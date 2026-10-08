package repo

// These tests need a real PostgreSQL database, because booking relies on
// SELECT ... FOR UPDATE row locks. They are skipped unless TEST_DATABASE_URL is set:
//
//	TEST_DATABASE_URL="postgres://postgres@localhost:5432/slots_test?sslmode=disable" go test ./repo/
//
// WARNING: every test truncates the users and slots tables. Never point this at real data.

import (
	"context"
	"errors"
	"os"
	"slots/models"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	testDB     *gorm.DB
	testDBErr  error
	testDBOnce sync.Once
)

func setupDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping Postgres repo tests")
	}
	testDBOnce.Do(func() {
		testDB, testDBErr = gorm.Open(postgres.Open(dsn), &gorm.Config{
			TranslateError: true,
			Logger:         logger.Default.LogMode(logger.Silent),
		})
		if testDBErr == nil {
			testDBErr = testDB.AutoMigrate(&models.User{}, &models.Slot{}, &models.SlotHistory{})
		}
	})
	if testDBErr != nil {
		t.Fatalf("test db: %v", testDBErr)
	}
	if err := testDB.Exec("TRUNCATE slot_history, slots, users RESTART IDENTITY CASCADE").Error; err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return testDB
}

var ctx = context.Background()

const testHold = 10 * time.Minute

func mustUser(t *testing.T, r UserRepo, email string) models.User {
	t.Helper()
	u, err := r.Create(ctx, models.User{Name: "u", Email: email})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	return u
}

func newSlot(hour int) models.Slot {
	day := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	return models.Slot{
		Date: day,
		From: day.Add(time.Duration(hour) * time.Hour),
		To:   day.Add(time.Duration(hour+1) * time.Hour),
	}
}

func mustSlot(t *testing.T, r SlotsRepo) models.Slot {
	t.Helper()
	s, err := r.AddSlot(ctx, newSlot(10))
	if err != nil {
		t.Fatalf("add slot: %v", err)
	}
	return s
}

func TestUserRepo(t *testing.T) {
	db := setupDB(t)
	users := NewUserRepo(db)

	created := mustUser(t, users, "a@b.com")
	if created.ID == 0 {
		t.Fatal("created user has no id")
	}

	got, err := users.GetById(ctx, int(created.ID))
	if err != nil || got != created {
		t.Errorf("GetById = %+v, %v; want %+v", got, err, created)
	}

	if _, err := users.GetById(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetById(missing) err = %v, want ErrNotFound", err)
	}

	if _, err := users.Create(ctx, models.User{Name: "dup", Email: "a@b.com"}); !errors.Is(err, ErrDuplicate) {
		t.Errorf("duplicate email err = %v, want ErrDuplicate", err)
	}
}

func TestSlotsAddAndGet(t *testing.T) {
	db := setupDB(t)
	slots := NewSlotsRepo(db, testHold)

	created := mustSlot(t, slots)
	if created.ID == 0 || created.CreatedAt.IsZero() {
		t.Errorf("created slot missing id/createdAt: %+v", created)
	}

	got, err := slots.GetById(ctx, int(created.ID))
	if err != nil {
		t.Fatalf("GetById: %v", err)
	}
	if !got.From.Equal(created.From) || !got.To.Equal(created.To) || got.Status != models.SlotStatusNotBooked {
		t.Errorf("GetById = %+v, want %+v", got, created)
	}

	if _, err := slots.GetById(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetById(missing) err = %v, want ErrNotFound", err)
	}

	bad := newSlot(10)
	bad.Status = 5
	if _, err := slots.AddSlot(ctx, bad); !errors.Is(err, ErrInvalidStatus) {
		t.Errorf("AddSlot(status 5) err = %v, want ErrInvalidStatus", err)
	}
}

func TestSlotsGetAll(t *testing.T) {
	db := setupDB(t)
	slots := NewSlotsRepo(db, testHold)

	all, err := slots.GetAll(ctx)
	if err != nil || all == nil || len(all) != 0 {
		t.Fatalf("GetAll on empty table = %v, %v; want empty non-nil slice", all, err)
	}

	// Insert out of order; GetAll must sort by date, then from.
	for _, h := range []int{14, 9, 11} {
		if _, err := slots.AddSlot(ctx, newSlot(h)); err != nil {
			t.Fatal(err)
		}
	}
	all, err = slots.GetAll(ctx)
	if err != nil || len(all) != 3 {
		t.Fatalf("GetAll = %d slots, %v; want 3", len(all), err)
	}
	for i := 1; i < len(all); i++ {
		if !all[i-1].From.Before(all[i].From) {
			t.Errorf("GetAll not ordered by from: %v then %v", all[i-1].From, all[i].From)
		}
	}
}

func TestSlotsAddBulk(t *testing.T) {
	db := setupDB(t)
	slots := NewSlotsRepo(db, testHold)

	created, err := slots.AddBulk(ctx, []models.Slot{newSlot(9), newSlot(10), newSlot(11)})
	if err != nil || len(created) != 3 {
		t.Fatalf("AddBulk = %d, %v; want 3 slots", len(created), err)
	}
	for _, s := range created {
		if s.ID == 0 {
			t.Error("bulk-created slot has no id")
		}
	}

	if got, err := slots.AddBulk(ctx, nil); err != nil || len(got) != 0 {
		t.Errorf("AddBulk(nil) = %v, %v; want empty, nil", got, err)
	}

	bad := newSlot(12)
	bad.Status = 1
	if _, err := slots.AddBulk(ctx, []models.Slot{newSlot(12), bad}); !errors.Is(err, ErrInvalidStatus) {
		t.Errorf("AddBulk with invalid status err = %v, want ErrInvalidStatus", err)
	}
	all, _ := slots.GetAll(ctx)
	if len(all) != 3 {
		t.Errorf("after rejected bulk: %d slots in db, want 3 (nothing inserted)", len(all))
	}
}

func TestBookSlot(t *testing.T) {
	db := setupDB(t)
	users, slots := NewUserRepo(db), NewSlotsRepo(db, testHold)
	alice := mustUser(t, users, "alice@x.com")
	bob := mustUser(t, users, "bob@x.com")
	slot := mustSlot(t, slots)

	booked, err := slots.UpdateSlotStatus(ctx, int(slot.ID), int(alice.ID), models.SlotStatusBooked)
	if err != nil {
		t.Fatalf("book: %v", err)
	}
	if booked.Status != models.SlotStatusBooked || booked.BookedBy == nil || *booked.BookedBy != alice.ID {
		t.Errorf("after book: %+v, want booked by %d", booked, alice.ID)
	}
	if !booked.UpdatedAt.After(slot.UpdatedAt) {
		t.Error("updatedAt was not bumped")
	}

	if _, err := slots.UpdateSlotStatus(ctx, int(slot.ID), int(bob.ID), models.SlotStatusBooked); !errors.Is(err, ErrSlotAlreadyBooked) {
		t.Errorf("second book err = %v, want ErrSlotAlreadyBooked", err)
	}
	if _, err := slots.UpdateSlotStatus(ctx, int(slot.ID), int(alice.ID), models.SlotStatusBooked); !errors.Is(err, ErrSlotAlreadyBooked) {
		t.Errorf("rebook by owner err = %v, want ErrSlotAlreadyBooked", err)
	}
}

func TestBookSlotErrors(t *testing.T) {
	db := setupDB(t)
	users, slots := NewUserRepo(db), NewSlotsRepo(db, testHold)
	alice := mustUser(t, users, "alice@x.com")
	slot := mustSlot(t, slots)

	tests := []struct {
		name           string
		slotId, userId int
		status         models.SlotStatus
		want           error
	}{
		{"missing slot", 999, int(alice.ID), models.SlotStatusBooked, ErrNotFound},
		{"missing user", int(slot.ID), 999, models.SlotStatusBooked, ErrNotFound},
		{"invalid status", int(slot.ID), int(alice.ID), 7, ErrInvalidStatus},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := slots.UpdateSlotStatus(ctx, tt.slotId, tt.userId, tt.status); !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}

	got, _ := slots.GetById(ctx, int(slot.ID))
	if got.IsBooked() {
		t.Error("slot became booked after failed attempts")
	}
}

func TestCancelSlot(t *testing.T) {
	db := setupDB(t)
	users, slots := NewUserRepo(db), NewSlotsRepo(db, testHold)
	alice := mustUser(t, users, "alice@x.com")
	bob := mustUser(t, users, "bob@x.com")
	slot := mustSlot(t, slots)
	id := int(slot.ID)

	if _, err := slots.UpdateSlotStatus(ctx, id, int(alice.ID), models.SlotStatusNotBooked); !errors.Is(err, ErrSlotNotBooked) {
		t.Errorf("cancel unbooked err = %v, want ErrSlotNotBooked", err)
	}

	if _, err := slots.UpdateSlotStatus(ctx, id, int(alice.ID), models.SlotStatusBooked); err != nil {
		t.Fatal(err)
	}

	if _, err := slots.UpdateSlotStatus(ctx, id, int(bob.ID), models.SlotStatusNotBooked); !errors.Is(err, ErrSlotNotOwned) {
		t.Errorf("cancel by non-owner err = %v, want ErrSlotNotOwned", err)
	}

	cancelled, err := slots.UpdateSlotStatus(ctx, id, int(alice.ID), models.SlotStatusNotBooked)
	if err != nil {
		t.Fatalf("cancel by owner: %v", err)
	}
	if cancelled.Status != models.SlotStatusNotBooked || cancelled.BookedBy != nil {
		t.Errorf("after cancel: %+v, want unbooked with no bookedBy", cancelled)
	}

	// Freed slot can be booked by someone else.
	if _, err := slots.UpdateSlotStatus(ctx, id, int(bob.ID), models.SlotStatusBooked); err != nil {
		t.Errorf("rebook after cancel: %v", err)
	}
}

func TestDeletingUserFreesBookedBy(t *testing.T) {
	db := setupDB(t)
	users, slots := NewUserRepo(db), NewSlotsRepo(db, testHold)
	alice := mustUser(t, users, "alice@x.com")
	slot := mustSlot(t, slots)
	if _, err := slots.UpdateSlotStatus(ctx, int(slot.ID), int(alice.ID), models.SlotStatusBooked); err != nil {
		t.Fatal(err)
	}

	if err := db.Delete(&models.User{}, alice.ID).Error; err != nil {
		t.Fatalf("delete user: %v", err)
	}
	got, _ := slots.GetById(ctx, int(slot.ID))
	if got.BookedBy != nil {
		t.Errorf("bookedBy = %d after user deleted, want nil (ON DELETE SET NULL)", *got.BookedBy)
	}
}

// Many users try to book the same slot at once: exactly one must win.
func TestConcurrentBookingOnlyOneWins(t *testing.T) {
	db := setupDB(t)
	users, slots := NewUserRepo(db), NewSlotsRepo(db, testHold)
	slot := mustSlot(t, slots)

	const n = 20
	userIds := make([]int, n)
	for i := range userIds {
		userIds[i] = int(mustUser(t, users, "user"+string(rune('a'+i))+"@x.com").ID)
	}

	// Widen the race window: pause after every read of the slots table so all goroutines
	// read the slot before any of them writes. Without the row lock this lets several
	// bookings through; with it, the others block on the lock and then see "booked".
	const slowRead = "test:slow_slot_read"
	if err := db.Callback().Query().After("gorm:query").Register(slowRead, func(tx *gorm.DB) {
		if tx.Statement.Table == "slots" {
			time.Sleep(20 * time.Millisecond)
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Callback().Query().Remove(slowRead) })

	var (
		wg      sync.WaitGroup
		start   = make(chan struct{})
		results = make([]error, n)
	)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, results[i] = slots.UpdateSlotStatus(ctx, int(slot.ID), userIds[i], models.SlotStatusBooked)
		}(i)
	}
	close(start)
	wg.Wait()

	winners := 0
	winner := -1
	for i, err := range results {
		switch {
		case err == nil:
			winners++
			winner = i
		case errors.Is(err, ErrSlotAlreadyBooked):
		default:
			t.Errorf("user %d: unexpected error %v", i, err)
		}
	}
	if winners != 1 {
		t.Fatalf("%d bookings succeeded, want exactly 1", winners)
	}

	got, _ := slots.GetById(ctx, int(slot.ID))
	if got.BookedBy == nil || *got.BookedBy != int64(userIds[winner]) {
		t.Errorf("slot bookedBy = %v, want winner %d", got.BookedBy, userIds[winner])
	}

	var historyRows int64
	db.Model(&models.SlotHistory{}).Count(&historyRows)
	if historyRows != 1 {
		t.Errorf("%d history rows, want 1 (only the winning booking)", historyRows)
	}
}

func historyOf(t *testing.T, db *gorm.DB, userId int64) []models.SlotHistory {
	t.Helper()
	h, err := NewHistoryRepo(db).ListByUser(ctx, int(userId), 100, 0)
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	return h
}

func TestBookAndCancelWriteHistory(t *testing.T) {
	db := setupDB(t)
	users, slots := NewUserRepo(db), NewSlotsRepo(db, testHold)
	alice := mustUser(t, users, "alice@x.com")
	bob := mustUser(t, users, "bob@x.com")
	slot := mustSlot(t, slots)
	id := int(slot.ID)

	if _, err := slots.UpdateSlotStatus(ctx, id, int(alice.ID), models.SlotStatusBooked); err != nil {
		t.Fatal(err)
	}
	// Failed attempts must not leave history behind.
	slots.UpdateSlotStatus(ctx, id, int(bob.ID), models.SlotStatusBooked)
	slots.UpdateSlotStatus(ctx, id, int(bob.ID), models.SlotStatusNotBooked)
	if _, err := slots.UpdateSlotStatus(ctx, id, int(alice.ID), models.SlotStatusNotBooked); err != nil {
		t.Fatal(err)
	}

	h := historyOf(t, db, alice.ID)
	if len(h) != 2 {
		t.Fatalf("alice has %d history rows, want 2", len(h))
	}
	// Newest first.
	if h[0].Action != models.SlotActionUnbooked || h[1].Action != models.SlotActionBooked {
		t.Errorf("actions = [%s %s], want [unbooked booked]", h[0].Action, h[1].Action)
	}
	for _, e := range h {
		if e.SlotID != slot.ID || e.PreviousSlotID != nil || e.Slot == nil || e.Slot.ID != slot.ID || e.CreatedAt.IsZero() {
			t.Errorf("bad history entry: %+v", e)
		}
	}
	if len(historyOf(t, db, bob.ID)) != 0 {
		t.Error("bob's failed attempts were recorded")
	}
}

func TestReschedule(t *testing.T) {
	db := setupDB(t)
	users, slots := NewUserRepo(db), NewSlotsRepo(db, testHold)
	alice := mustUser(t, users, "alice@x.com")
	from, _ := slots.AddSlot(ctx, newSlot(9))
	to, _ := slots.AddSlot(ctx, newSlot(10))

	if _, err := slots.UpdateSlotStatus(ctx, int(from.ID), int(alice.ID), models.SlotStatusBooked); err != nil {
		t.Fatal(err)
	}

	got, err := slots.Reschedule(ctx, int(from.ID), int(to.ID), int(alice.ID))
	if err != nil {
		t.Fatalf("Reschedule: %v", err)
	}
	if got.ID != to.ID || !got.IsBooked() || got.BookedBy == nil || *got.BookedBy != alice.ID {
		t.Errorf("returned %+v, want new slot booked by alice", got)
	}
	old, _ := slots.GetById(ctx, int(from.ID))
	if old.IsBooked() || old.BookedBy != nil {
		t.Errorf("old slot still booked: %+v", old)
	}

	h := historyOf(t, db, alice.ID)
	if len(h) != 2 {
		t.Fatalf("%d history rows, want 2 (booked, rescheduled)", len(h))
	}
	e := h[0]
	if e.Action != models.SlotActionRescheduled || e.SlotID != to.ID ||
		e.PreviousSlotID == nil || *e.PreviousSlotID != from.ID ||
		e.Slot == nil || e.Slot.ID != to.ID || e.PreviousSlot == nil || e.PreviousSlot.ID != from.ID {
		t.Errorf("reschedule entry = %+v", e)
	}
}

func TestRescheduleErrors(t *testing.T) {
	db := setupDB(t)
	users, slots := NewUserRepo(db), NewSlotsRepo(db, testHold)
	alice := mustUser(t, users, "alice@x.com")
	bob := mustUser(t, users, "bob@x.com")
	aliceSlot, _ := slots.AddSlot(ctx, newSlot(9))
	bobSlot, _ := slots.AddSlot(ctx, newSlot(10))
	free, _ := slots.AddSlot(ctx, newSlot(11))
	free2, _ := slots.AddSlot(ctx, newSlot(12))
	slots.UpdateSlotStatus(ctx, int(aliceSlot.ID), int(alice.ID), models.SlotStatusBooked)
	slots.UpdateSlotStatus(ctx, int(bobSlot.ID), int(bob.ID), models.SlotStatusBooked)

	tests := []struct {
		name             string
		from, to, userId int64
		want             error
	}{
		{"same slot", aliceSlot.ID, aliceSlot.ID, alice.ID, ErrSameSlot},
		{"source slot missing", 999, free.ID, alice.ID, ErrNotFound},
		{"target slot missing", aliceSlot.ID, 999, alice.ID, ErrNotFound},
		{"source not booked", free.ID, free2.ID, alice.ID, ErrSlotNotBooked},
		{"source booked by someone else", bobSlot.ID, free.ID, alice.ID, ErrSlotNotOwned},
		{"target already booked", aliceSlot.ID, bobSlot.ID, alice.ID, ErrSlotAlreadyBooked},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := slots.Reschedule(ctx, int(tt.from), int(tt.to), int(tt.userId)); !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}

	// Nothing changed.
	for _, s := range []models.Slot{aliceSlot, bobSlot} {
		got, _ := slots.GetById(ctx, int(s.ID))
		if !got.IsBooked() {
			t.Errorf("slot %d lost its booking", s.ID)
		}
	}
	for _, s := range []models.Slot{free, free2} {
		got, _ := slots.GetById(ctx, int(s.ID))
		if got.IsBooked() {
			t.Errorf("slot %d became booked", s.ID)
		}
	}
	if n := len(historyOf(t, db, alice.ID)); n != 1 {
		t.Errorf("alice has %d history rows, want 1 (just the booking)", n)
	}
}

// Two users each move their booking into the same free slot at once: exactly one wins,
// and the loser keeps their original slot.
func TestConcurrentRescheduleIntoSameSlot(t *testing.T) {
	db := setupDB(t)
	users, slots := NewUserRepo(db), NewSlotsRepo(db, testHold)
	alice := mustUser(t, users, "alice@x.com")
	bob := mustUser(t, users, "bob@x.com")
	aliceSlot, _ := slots.AddSlot(ctx, newSlot(9))
	bobSlot, _ := slots.AddSlot(ctx, newSlot(10))
	target, _ := slots.AddSlot(ctx, newSlot(11))
	slots.UpdateSlotStatus(ctx, int(aliceSlot.ID), int(alice.ID), models.SlotStatusBooked)
	slots.UpdateSlotStatus(ctx, int(bobSlot.ID), int(bob.ID), models.SlotStatusBooked)

	const slowRead = "test:slow_slot_read_reschedule"
	db.Callback().Query().After("gorm:query").Register(slowRead, func(tx *gorm.DB) {
		if tx.Statement.Table == "slots" {
			time.Sleep(20 * time.Millisecond)
		}
	})
	t.Cleanup(func() { db.Callback().Query().Remove(slowRead) })

	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make([]error, 2)
	moves := [2][3]int64{{aliceSlot.ID, target.ID, alice.ID}, {bobSlot.ID, target.ID, bob.ID}}
	for i, m := range moves {
		wg.Add(1)
		go func(i int, m [3]int64) {
			defer wg.Done()
			<-start
			_, errs[i] = slots.Reschedule(ctx, int(m[0]), int(m[1]), int(m[2]))
		}(i, m)
	}
	close(start)
	wg.Wait()

	winners := 0
	for i, err := range errs {
		if err == nil {
			winners++
		} else if !errors.Is(err, ErrSlotAlreadyBooked) {
			t.Errorf("move %d: unexpected error %v", i, err)
		}
	}
	if winners != 1 {
		t.Fatalf("%d reschedules succeeded, want exactly 1", winners)
	}

	booked := 0
	all, _ := slots.GetAll(ctx)
	for _, s := range all {
		if s.IsBooked() {
			booked++
		}
	}
	if booked != 2 {
		t.Errorf("%d slots booked after race, want 2 (winner's new slot + loser's original)", booked)
	}
}

func TestHistoryPagination(t *testing.T) {
	db := setupDB(t)
	users, slots := NewUserRepo(db), NewSlotsRepo(db, testHold)
	alice := mustUser(t, users, "alice@x.com")
	slot := mustSlot(t, slots)
	for i := 0; i < 3; i++ {
		slots.UpdateSlotStatus(ctx, int(slot.ID), int(alice.ID), models.SlotStatusBooked)
		slots.UpdateSlotStatus(ctx, int(slot.ID), int(alice.ID), models.SlotStatusNotBooked)
	}

	history := NewHistoryRepo(db)
	all, _ := history.ListByUser(ctx, int(alice.ID), 100, 0)
	if len(all) != 6 {
		t.Fatalf("%d rows, want 6", len(all))
	}
	page, _ := history.ListByUser(ctx, int(alice.ID), 2, 2)
	if len(page) != 2 || page[0].ID != all[2].ID || page[1].ID != all[3].ID {
		t.Errorf("page(limit 2, offset 2) = ids %d,%d; want %d,%d", page[0].ID, page[1].ID, all[2].ID, all[3].ID)
	}
	empty, err := history.ListByUser(ctx, 999, 10, 0)
	if err != nil || empty == nil || len(empty) != 0 {
		t.Errorf("unknown user = %v, %v; want empty non-nil slice", empty, err)
	}
}

func TestHistoryCascadesOnUserDelete(t *testing.T) {
	db := setupDB(t)
	users, slots := NewUserRepo(db), NewSlotsRepo(db, testHold)
	alice := mustUser(t, users, "alice@x.com")
	slot := mustSlot(t, slots)
	slots.UpdateSlotStatus(ctx, int(slot.ID), int(alice.ID), models.SlotStatusBooked)

	if err := db.Delete(&models.User{}, alice.ID).Error; err != nil {
		t.Fatalf("delete user: %v", err)
	}
	var n int64
	db.Model(&models.SlotHistory{}).Where("user_id = ?", alice.ID).Count(&n)
	if n != 0 {
		t.Errorf("%d history rows left after user delete, want 0", n)
	}
}

// expireHold backdates the slot's last update past the hold window.
func expireHold(t *testing.T, db *gorm.DB, slotId int64) {
	t.Helper()
	if err := db.Exec("UPDATE slots SET updated_at = ? WHERE id = ?", time.Now().Add(-testHold-time.Minute), slotId).Error; err != nil {
		t.Fatal(err)
	}
}

func TestHoldSlot(t *testing.T) {
	db := setupDB(t)
	users, slots := NewUserRepo(db), NewSlotsRepo(db, testHold)
	alice := mustUser(t, users, "alice@x.com")
	bob := mustUser(t, users, "bob@x.com")
	slot := mustSlot(t, slots)
	id := int(slot.ID)

	held, err := slots.UpdateSlotStatus(ctx, id, int(alice.ID), models.SlotStatusOnHold)
	if err != nil {
		t.Fatalf("hold: %v", err)
	}
	if held.Status != models.SlotStatusOnHold || !held.HeldOrBookedBy(alice.ID) {
		t.Errorf("after hold: %+v, want on hold by alice", held)
	}

	// An active hold blocks everyone else, and the holder can't re-hold to extend it.
	for name, tt := range map[string]struct {
		userId int64
		status models.SlotStatus
		want   error
	}{
		"bob holds":     {bob.ID, models.SlotStatusOnHold, ErrSlotOnHold},
		"bob books":     {bob.ID, models.SlotStatusBooked, ErrSlotOnHold},
		"bob cancels":   {bob.ID, models.SlotStatusNotBooked, ErrSlotNotOwned},
		"alice re-hold": {alice.ID, models.SlotStatusOnHold, ErrSlotOnHold},
	} {
		if _, err := slots.UpdateSlotStatus(ctx, id, int(tt.userId), tt.status); !errors.Is(err, tt.want) {
			t.Errorf("%s: err = %v, want %v", name, err, tt.want)
		}
	}

	// The holder confirms by booking.
	booked, err := slots.UpdateSlotStatus(ctx, id, int(alice.ID), models.SlotStatusBooked)
	if err != nil {
		t.Fatalf("holder book: %v", err)
	}
	if !booked.IsBooked() || !booked.HeldOrBookedBy(alice.ID) {
		t.Errorf("after confirm: %+v, want booked by alice", booked)
	}
	if _, err := slots.UpdateSlotStatus(ctx, id, int(bob.ID), models.SlotStatusOnHold); !errors.Is(err, ErrSlotAlreadyBooked) {
		t.Errorf("hold booked slot err = %v, want ErrSlotAlreadyBooked", err)
	}

	h := historyOf(t, db, alice.ID)
	if len(h) != 2 || h[0].Action != models.SlotActionBooked || h[1].Action != models.SlotActionHeld {
		t.Errorf("alice history = %v, want [booked held]", actions(h))
	}
	if n := len(historyOf(t, db, bob.ID)); n != 0 {
		t.Errorf("bob has %d history rows, want 0", n)
	}
}

func actions(h []models.SlotHistory) []string {
	out := make([]string, len(h))
	for i, e := range h {
		out[i] = e.Action.String()
	}
	return out
}

func TestHoldMissingUser(t *testing.T) {
	db := setupDB(t)
	slots := NewSlotsRepo(db, testHold)
	slot := mustSlot(t, slots)
	if _, err := slots.UpdateSlotStatus(ctx, int(slot.ID), 999, models.SlotStatusOnHold); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestReleaseHold(t *testing.T) {
	db := setupDB(t)
	users, slots := NewUserRepo(db), NewSlotsRepo(db, testHold)
	alice := mustUser(t, users, "alice@x.com")
	slot := mustSlot(t, slots)
	slots.UpdateSlotStatus(ctx, int(slot.ID), int(alice.ID), models.SlotStatusOnHold)

	released, err := slots.UpdateSlotStatus(ctx, int(slot.ID), int(alice.ID), models.SlotStatusNotBooked)
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if released.Status != models.SlotStatusNotBooked || released.BookedBy != nil {
		t.Errorf("after release: %+v", released)
	}
	if h := historyOf(t, db, alice.ID); len(h) != 2 || h[0].Action != models.SlotActionUnbooked {
		t.Errorf("history = %v, want [unbooked held]", actions(h))
	}
}

func TestExpiredHold(t *testing.T) {
	db := setupDB(t)
	users, slots := NewUserRepo(db), NewSlotsRepo(db, testHold)
	alice := mustUser(t, users, "alice@x.com")
	bob := mustUser(t, users, "bob@x.com")

	heldThenExpired := func(t *testing.T) int {
		t.Helper()
		slot := mustSlot(t, slots)
		if _, err := slots.UpdateSlotStatus(ctx, int(slot.ID), int(alice.ID), models.SlotStatusOnHold); err != nil {
			t.Fatal(err)
		}
		expireHold(t, db, slot.ID)
		return int(slot.ID)
	}

	t.Run("db row is not changed", func(t *testing.T) {
		id := heldThenExpired(t)
		raw, _ := slots.GetById(ctx, id)
		if raw.Status != models.SlotStatusOnHold || !raw.HeldOrBookedBy(alice.ID) {
			t.Errorf("raw row = %+v, want still on hold by alice in the db", raw)
		}
	})
	t.Run("someone else can hold it", func(t *testing.T) {
		id := heldThenExpired(t)
		got, err := slots.UpdateSlotStatus(ctx, id, int(bob.ID), models.SlotStatusOnHold)
		if err != nil || !got.HeldOrBookedBy(bob.ID) {
			t.Errorf("bob hold = %+v, %v", got, err)
		}
	})
	t.Run("someone else can book it", func(t *testing.T) {
		id := heldThenExpired(t)
		got, err := slots.UpdateSlotStatus(ctx, id, int(bob.ID), models.SlotStatusBooked)
		if err != nil || !got.IsBooked() || !got.HeldOrBookedBy(bob.ID) {
			t.Errorf("bob book = %+v, %v", got, err)
		}
	})
	t.Run("former holder can hold again", func(t *testing.T) {
		id := heldThenExpired(t)
		if _, err := slots.UpdateSlotStatus(ctx, id, int(alice.ID), models.SlotStatusOnHold); err != nil {
			t.Errorf("alice re-hold after expiry: %v", err)
		}
	})
	t.Run("former holder cannot cancel it", func(t *testing.T) {
		id := heldThenExpired(t)
		if _, err := slots.UpdateSlotStatus(ctx, id, int(alice.ID), models.SlotStatusNotBooked); !errors.Is(err, ErrSlotNotBooked) {
			t.Errorf("err = %v, want ErrSlotNotBooked", err)
		}
	})
}

func TestRescheduleWithHolds(t *testing.T) {
	db := setupDB(t)
	users, slots := NewUserRepo(db), NewSlotsRepo(db, testHold)
	alice := mustUser(t, users, "alice@x.com")
	bob := mustUser(t, users, "bob@x.com")
	newBooked := func(hour int) int {
		s, _ := slots.AddSlot(ctx, newSlot(hour))
		if _, err := slots.UpdateSlotStatus(ctx, int(s.ID), int(alice.ID), models.SlotStatusBooked); err != nil {
			t.Fatal(err)
		}
		return int(s.ID)
	}
	newHeld := func(hour int, by int64) models.Slot {
		s, _ := slots.AddSlot(ctx, newSlot(hour))
		if _, err := slots.UpdateSlotStatus(ctx, int(s.ID), int(by), models.SlotStatusOnHold); err != nil {
			t.Fatal(err)
		}
		return s
	}

	t.Run("into another user's active hold", func(t *testing.T) {
		from, to := newBooked(1), newHeld(2, bob.ID)
		if _, err := slots.Reschedule(ctx, from, int(to.ID), int(alice.ID)); !errors.Is(err, ErrSlotOnHold) {
			t.Errorf("err = %v, want ErrSlotOnHold", err)
		}
	})
	t.Run("into another user's expired hold", func(t *testing.T) {
		from, to := newBooked(3), newHeld(4, bob.ID)
		expireHold(t, db, to.ID)
		got, err := slots.Reschedule(ctx, from, int(to.ID), int(alice.ID))
		if err != nil || !got.IsBooked() || !got.HeldOrBookedBy(alice.ID) {
			t.Errorf("reschedule = %+v, %v", got, err)
		}
	})
	t.Run("into own active hold", func(t *testing.T) {
		from, to := newBooked(5), newHeld(6, alice.ID)
		got, err := slots.Reschedule(ctx, from, int(to.ID), int(alice.ID))
		if err != nil || !got.IsBooked() {
			t.Errorf("reschedule = %+v, %v", got, err)
		}
	})
	t.Run("from a hold (not a booking)", func(t *testing.T) {
		from := newHeld(7, alice.ID)
		to, _ := slots.AddSlot(ctx, newSlot(8))
		if _, err := slots.Reschedule(ctx, int(from.ID), int(to.ID), int(alice.ID)); !errors.Is(err, ErrSlotNotBooked) {
			t.Errorf("err = %v, want ErrSlotNotBooked", err)
		}
	})
}

// Many users race for the same slot; exactly one must win. Run for holding a free slot and
// for booking a slot whose hold has expired (raw status 50 in the db).
func TestConcurrentHoldsOnlyOneWins(t *testing.T) {
	cases := []struct {
		name    string
		status  models.SlotStatus
		expired bool
		loseErr error
	}{
		{"hold free slot", models.SlotStatusOnHold, false, ErrSlotOnHold},
		{"book expired hold", models.SlotStatusBooked, true, ErrSlotAlreadyBooked},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := setupDB(t)
			users, slots := NewUserRepo(db), NewSlotsRepo(db, testHold)
			slot := mustSlot(t, slots)

			const n = 10
			userIds := make([]int, n)
			for i := range userIds {
				userIds[i] = int(mustUser(t, users, "u"+string(rune('a'+i))+"@x.com").ID)
			}
			if tc.expired {
				slots.UpdateSlotStatus(ctx, int(slot.ID), userIds[0], models.SlotStatusOnHold)
				expireHold(t, db, slot.ID)
			}

			const slowRead = "test:slow_slot_read_hold"
			db.Callback().Query().After("gorm:query").Register(slowRead, func(tx *gorm.DB) {
				if tx.Statement.Table == "slots" {
					time.Sleep(20 * time.Millisecond)
				}
			})
			t.Cleanup(func() { db.Callback().Query().Remove(slowRead) })

			var wg sync.WaitGroup
			start := make(chan struct{})
			errs := make([]error, n)
			for i := 0; i < n; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					<-start
					_, errs[i] = slots.UpdateSlotStatus(ctx, int(slot.ID), userIds[i], tc.status)
				}(i)
			}
			close(start)
			wg.Wait()

			winners := 0
			for i, err := range errs {
				if err == nil {
					winners++
				} else if !errors.Is(err, tc.loseErr) {
					t.Errorf("user %d: unexpected error %v", i, err)
				}
			}
			if winners != 1 {
				t.Errorf("%d succeeded, want exactly 1", winners)
			}
		})
	}
}
