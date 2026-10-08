package transport

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"slots/models"
	"slots/service"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	log.SetOutput(io.Discard) // silence "internal error" logs from the 500 case
	m.Run()
}

type fakeUserService struct {
	err        error
	gotName    string
	gotEmail   string
	createHit  bool
	gotHistory *[3]int // user, limit, offset
}

func (f *fakeUserService) GetById(ctx context.Context, id int) (models.User, error) {
	return models.User{ID: int64(id), Name: "Asha", Email: "asha@example.com"}, f.err
}

func (f *fakeUserService) Create(ctx context.Context, name, email string) (models.User, error) {
	f.createHit, f.gotName, f.gotEmail = true, name, email
	if f.err != nil {
		return models.User{}, f.err
	}
	return models.User{ID: 1, Name: name, Email: email}, nil
}

func (f *fakeUserService) History(ctx context.Context, userId, limit, offset int) ([]models.SlotHistory, error) {
	f.gotHistory = &[3]int{userId, limit, offset}
	if f.err != nil {
		return nil, f.err
	}
	prev := int64(1)
	return []models.SlotHistory{
		{ID: 2, UserID: int64(userId), SlotID: 2, PreviousSlotID: &prev, Action: models.SlotActionRescheduled},
		{ID: 1, UserID: int64(userId), SlotID: 1, Action: models.SlotActionBooked},
	}, nil
}

type fakeSlotsService struct {
	err                error
	gotSlot            *models.Slot
	gotBulk            []models.Slot
	gotSlotId, gotUser int
	gotToSlot          int
	called             bool
}

func (f *fakeSlotsService) Reschedule(ctx context.Context, fromSlotId, toSlotId, userId int) (models.Slot, error) {
	f.called, f.gotSlotId, f.gotToSlot, f.gotUser = true, fromSlotId, toSlotId, userId
	return models.Slot{ID: int64(toSlotId), Status: models.SlotStatusBooked}, f.err
}

func (f *fakeSlotsService) GetById(ctx context.Context, id int) (models.Slot, error) {
	return models.Slot{ID: int64(id)}, f.err
}

func (f *fakeSlotsService) GetAll(ctx context.Context) ([]models.Slot, error) {
	return []models.Slot{}, f.err
}

func (f *fakeSlotsService) Hold(ctx context.Context, slotId, userId int) (models.Slot, error) {
	f.called, f.gotSlotId, f.gotUser = true, slotId, userId
	return models.Slot{ID: int64(slotId), Status: models.SlotStatusOnHold}, f.err
}

func (f *fakeSlotsService) Book(ctx context.Context, slotId, userId int) (models.Slot, error) {
	f.called, f.gotSlotId, f.gotUser = true, slotId, userId
	return models.Slot{ID: int64(slotId), Status: models.SlotStatusBooked}, f.err
}

func (f *fakeSlotsService) Cancel(ctx context.Context, slotId, userId int) (models.Slot, error) {
	f.called, f.gotSlotId, f.gotUser = true, slotId, userId
	return models.Slot{ID: int64(slotId)}, f.err
}

func (f *fakeSlotsService) AddSlot(ctx context.Context, slot models.Slot) (models.Slot, error) {
	f.called, f.gotSlot = true, &slot
	return slot, f.err
}

func (f *fakeSlotsService) AddBulk(ctx context.Context, slots []models.Slot) ([]models.Slot, error) {
	f.called, f.gotBulk = true, slots
	return slots, f.err
}

func newTestRouter(users *fakeUserService, slots *fakeSlotsService) *gin.Engine {
	return NewRouter(NewUserHandler(users), NewSlotsHandler(slots))
}

func do(r http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func errorMessage(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not an error JSON: %s", w.Body.String())
	}
	return body.Error
}

func TestErrorMapping(t *testing.T) {
	tests := []struct {
		err      error
		wantCode int
		wantMsg  string
	}{
		{&service.ValidationError{Msg: "bad input"}, http.StatusBadRequest, "bad input"},
		{service.ErrInvalidStatus, http.StatusBadRequest, service.ErrInvalidStatus.Error()},
		{service.ErrSameSlot, http.StatusBadRequest, service.ErrSameSlot.Error()},
		{service.ErrNotFound, http.StatusNotFound, service.ErrNotFound.Error()},
		{service.ErrSlotNotOwned, http.StatusForbidden, service.ErrSlotNotOwned.Error()},
		{service.ErrDuplicate, http.StatusConflict, service.ErrDuplicate.Error()},
		{service.ErrSlotAlreadyBooked, http.StatusConflict, service.ErrSlotAlreadyBooked.Error()},
		{service.ErrSlotOnHold, http.StatusConflict, service.ErrSlotOnHold.Error()},
		{service.ErrSlotNotBooked, http.StatusConflict, service.ErrSlotNotBooked.Error()},
		{service.ErrConcurrentUpdate, http.StatusConflict, service.ErrConcurrentUpdate.Error()},
		{errors.New("db exploded"), http.StatusInternalServerError, "internal server error"},
	}
	for _, tt := range tests {
		t.Run(tt.wantMsg, func(t *testing.T) {
			r := newTestRouter(&fakeUserService{}, &fakeSlotsService{err: tt.err})
			w := do(r, http.MethodPost, "/slots/1/book", `{"userId":1}`)
			if w.Code != tt.wantCode {
				t.Errorf("status = %d, want %d", w.Code, tt.wantCode)
			}
			if msg := errorMessage(t, w); msg != tt.wantMsg {
				t.Errorf("error = %q, want %q", msg, tt.wantMsg)
			}
		})
	}
}

func TestInvalidPathID(t *testing.T) {
	for _, path := range []string{"/users/abc", "/users/0", "/slots/-1", "/slots/x"} {
		w := do(newTestRouter(&fakeUserService{}, &fakeSlotsService{}), http.MethodGet, path, "")
		if w.Code != http.StatusBadRequest || errorMessage(t, w) != "invalid id" {
			t.Errorf("GET %s: got %d %s, want 400 invalid id", path, w.Code, w.Body.String())
		}
	}
}

func TestCreateUser(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		users := &fakeUserService{}
		w := do(newTestRouter(users, &fakeSlotsService{}), http.MethodPost, "/users", `{"name":"Asha","email":"asha@example.com"}`)
		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201: %s", w.Code, w.Body.String())
		}
		if users.gotName != "Asha" || users.gotEmail != "asha@example.com" {
			t.Errorf("service got %q %q", users.gotName, users.gotEmail)
		}
		var u models.User
		if err := json.Unmarshal(w.Body.Bytes(), &u); err != nil || u.ID != 1 {
			t.Errorf("body = %s, want created user", w.Body.String())
		}
	})

	for name, body := range map[string]string{
		"missing name":  `{"email":"a@b.com"}`,
		"missing email": `{"name":"Asha"}`,
		"malformed":     `{"name":`,
	} {
		t.Run(name, func(t *testing.T) {
			users := &fakeUserService{}
			w := do(newTestRouter(users, &fakeSlotsService{}), http.MethodPost, "/users", body)
			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", w.Code)
			}
			if users.createHit {
				t.Error("service called despite bad body")
			}
		})
	}
}

func TestGetUser(t *testing.T) {
	w := do(newTestRouter(&fakeUserService{}, &fakeSlotsService{}), http.MethodGet, "/users/5", "")
	var u models.User
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &u) != nil || u.ID != 5 {
		t.Errorf("got %d %s, want 200 user 5", w.Code, w.Body.String())
	}

	w = do(newTestRouter(&fakeUserService{err: service.ErrNotFound}, &fakeSlotsService{}), http.MethodGet, "/users/5", "")
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestCreateSlot(t *testing.T) {
	t.Run("parses date and times", func(t *testing.T) {
		slots := &fakeSlotsService{}
		w := do(newTestRouter(&fakeUserService{}, slots), http.MethodPost, "/slots",
			`{"date":"2026-10-10","from":"2026-10-10T10:00:00Z","to":"2026-10-10T11:00:00Z"}`)
		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201: %s", w.Code, w.Body.String())
		}
		got := slots.gotSlot
		if !got.Date.Equal(time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)) ||
			!got.From.Equal(time.Date(2026, 10, 10, 10, 0, 0, 0, time.UTC)) ||
			!got.To.Equal(time.Date(2026, 10, 10, 11, 0, 0, 0, time.UTC)) {
			t.Errorf("service got %+v", *got)
		}
	})

	tests := map[string]struct{ body, msg string }{
		"bad date format": {`{"date":"10-10-2026","from":"2026-10-10T10:00:00Z","to":"2026-10-10T11:00:00Z"}`, "date must be in YYYY-MM-DD format"},
		"missing to":      {`{"date":"2026-10-10","from":"2026-10-10T10:00:00Z"}`, ""},
		"bad timestamp":   {`{"date":"2026-10-10","from":"10am","to":"2026-10-10T11:00:00Z"}`, ""},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			slots := &fakeSlotsService{}
			w := do(newTestRouter(&fakeUserService{}, slots), http.MethodPost, "/slots", tt.body)
			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", w.Code)
			}
			if tt.msg != "" && errorMessage(t, w) != tt.msg {
				t.Errorf("error = %q, want %q", errorMessage(t, w), tt.msg)
			}
			if slots.called {
				t.Error("service called despite bad body")
			}
		})
	}
}

func TestCreateSlotsBulk(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		slots := &fakeSlotsService{}
		w := do(newTestRouter(&fakeUserService{}, slots), http.MethodPost, "/slots/bulk", `{"slots":[
			{"date":"2026-10-11","from":"2026-10-11T10:00:00Z","to":"2026-10-11T11:00:00Z"},
			{"date":"2026-10-11","from":"2026-10-11T11:00:00Z","to":"2026-10-11T12:00:00Z"}]}`)
		if w.Code != http.StatusCreated || len(slots.gotBulk) != 2 {
			t.Errorf("got %d with %d slots, want 201 with 2", w.Code, len(slots.gotBulk))
		}
	})

	t.Run("empty list", func(t *testing.T) {
		slots := &fakeSlotsService{}
		w := do(newTestRouter(&fakeUserService{}, slots), http.MethodPost, "/slots/bulk", `{"slots":[]}`)
		if w.Code != http.StatusBadRequest || slots.called {
			t.Errorf("got %d (service called=%v), want 400 without calling service", w.Code, slots.called)
		}
	})

	t.Run("bad date names the item", func(t *testing.T) {
		slots := &fakeSlotsService{}
		w := do(newTestRouter(&fakeUserService{}, slots), http.MethodPost, "/slots/bulk", `{"slots":[
			{"date":"2026-10-11","from":"2026-10-11T10:00:00Z","to":"2026-10-11T11:00:00Z"},
			{"date":"11/10/2026","from":"2026-10-11T11:00:00Z","to":"2026-10-11T12:00:00Z"}]}`)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
		if msg := errorMessage(t, w); !strings.HasPrefix(msg, "slots[1]:") {
			t.Errorf("error = %q, want prefix slots[1]:", msg)
		}
	})
}

func TestBookAndCancel(t *testing.T) {
	for _, action := range []string{"hold", "book", "cancel"} {
		t.Run(action+" passes ids", func(t *testing.T) {
			slots := &fakeSlotsService{}
			w := do(newTestRouter(&fakeUserService{}, slots), http.MethodPost, "/slots/3/"+action, `{"userId":4}`)
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
			}
			if slots.gotSlotId != 3 || slots.gotUser != 4 {
				t.Errorf("service got slot %d user %d, want 3 and 4", slots.gotSlotId, slots.gotUser)
			}
		})

		for name, body := range map[string]string{
			"missing userId":  `{}`,
			"zero userId":     `{"userId":0}`,
			"negative userId": `{"userId":-2}`,
			"string userId":   `{"userId":"4"}`,
		} {
			t.Run(action+" "+name, func(t *testing.T) {
				slots := &fakeSlotsService{}
				w := do(newTestRouter(&fakeUserService{}, slots), http.MethodPost, "/slots/3/"+action, body)
				if w.Code != http.StatusBadRequest || slots.called {
					t.Errorf("got %d (service called=%v), want 400 without calling service", w.Code, slots.called)
				}
			})
		}
	}
}

func TestListSlotsReturnsEmptyArray(t *testing.T) {
	w := do(newTestRouter(&fakeUserService{}, &fakeSlotsService{}), http.MethodGet, "/slots", "")
	if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != "[]" {
		t.Errorf("got %d %q, want 200 []", w.Code, w.Body.String())
	}
}

func TestStaticRoutes(t *testing.T) {
	r := newTestRouter(&fakeUserService{}, &fakeSlotsService{})
	tests := []struct{ path, contentType, contains string }{
		{"/health", "application/json", `"ok"`},
		{"/openapi.yaml", "application/yaml", "openapi: 3.0.3"},
		{"/docs", "text/html", "swagger-ui"},
	}
	for _, tt := range tests {
		w := do(r, http.MethodGet, tt.path, "")
		if w.Code != http.StatusOK ||
			!strings.HasPrefix(w.Header().Get("Content-Type"), tt.contentType) ||
			!strings.Contains(w.Body.String(), tt.contains) {
			t.Errorf("GET %s: got %d %q", tt.path, w.Code, w.Header().Get("Content-Type"))
		}
	}
}

func TestReschedule(t *testing.T) {
	t.Run("passes ids", func(t *testing.T) {
		slots := &fakeSlotsService{}
		w := do(newTestRouter(&fakeUserService{}, slots), http.MethodPost, "/slots/2/reschedule", `{"userId":4,"toSlotId":5}`)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
		}
		if slots.gotSlotId != 2 || slots.gotToSlot != 5 || slots.gotUser != 4 {
			t.Errorf("service got from=%d to=%d user=%d, want 2, 5, 4", slots.gotSlotId, slots.gotToSlot, slots.gotUser)
		}
	})

	for name, body := range map[string]string{
		"missing toSlotId": `{"userId":4}`,
		"missing userId":   `{"toSlotId":5}`,
		"zero toSlotId":    `{"userId":4,"toSlotId":0}`,
	} {
		t.Run(name, func(t *testing.T) {
			slots := &fakeSlotsService{}
			w := do(newTestRouter(&fakeUserService{}, slots), http.MethodPost, "/slots/2/reschedule", body)
			if w.Code != http.StatusBadRequest || slots.called {
				t.Errorf("got %d (service called=%v), want 400 without calling service", w.Code, slots.called)
			}
		})
	}
}

func TestHistory(t *testing.T) {
	t.Run("returns history and passes paging", func(t *testing.T) {
		users := &fakeUserService{}
		w := do(newTestRouter(users, &fakeSlotsService{}), http.MethodGet, "/users/3/history?limit=10&offset=5", "")
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
		}
		if users.gotHistory == nil || *users.gotHistory != [3]int{3, 10, 5} {
			t.Errorf("service got %v, want [3 10 5]", users.gotHistory)
		}
		var got []map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || len(got) != 2 {
			t.Fatalf("body = %s", w.Body.String())
		}
		if got[0]["action"] != float64(3) || got[0]["previousSlotId"] != float64(1) {
			t.Errorf("first entry = %v, want rescheduled with previousSlotId 1", got[0])
		}
		if _, ok := got[1]["previousSlotId"]; ok {
			t.Errorf("previousSlotId should be omitted for a booking: %v", got[1])
		}
	})

	t.Run("no paging params", func(t *testing.T) {
		users := &fakeUserService{}
		do(newTestRouter(users, &fakeSlotsService{}), http.MethodGet, "/users/3/history", "")
		if users.gotHistory == nil || *users.gotHistory != [3]int{3, 0, 0} {
			t.Errorf("service got %v, want [3 0 0]", users.gotHistory)
		}
	})

	for _, q := range []string{"limit=abc", "offset=1.5"} {
		t.Run("bad "+q, func(t *testing.T) {
			users := &fakeUserService{}
			w := do(newTestRouter(users, &fakeSlotsService{}), http.MethodGet, "/users/3/history?"+q, "")
			if w.Code != http.StatusBadRequest || users.gotHistory != nil {
				t.Errorf("got %d, want 400 without calling service", w.Code)
			}
		})
	}

	t.Run("unknown user", func(t *testing.T) {
		w := do(newTestRouter(&fakeUserService{err: service.ErrNotFound}, &fakeSlotsService{}), http.MethodGet, "/users/3/history", "")
		if w.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", w.Code)
		}
	})
}
