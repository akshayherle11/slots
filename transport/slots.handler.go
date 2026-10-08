package transport

import (
	"context"
	"fmt"
	"net/http"
	"slots/models"
	"slots/service"
	"time"

	"github.com/gin-gonic/gin"
)

const dateLayout = "2006-01-02"

type SlotsHandler struct {
	slots service.SlotsService
}

func NewSlotsHandler(slots service.SlotsService) *SlotsHandler {
	return &SlotsHandler{slots: slots}
}

type createSlotRequest struct {
	Date string    `json:"date" binding:"required"` // YYYY-MM-DD
	From time.Time `json:"from" binding:"required"` // RFC3339
	To   time.Time `json:"to" binding:"required"`   // RFC3339
}

func (r createSlotRequest) toModel() (models.Slot, error) {
	date, err := time.Parse(dateLayout, r.Date)
	if err != nil {
		return models.Slot{}, fmt.Errorf("date must be in YYYY-MM-DD format")
	}
	return models.Slot{Date: date, From: r.From, To: r.To}, nil
}

type createBulkRequest struct {
	Slots []createSlotRequest `json:"slots" binding:"required,min=1,dive"`
}

type bookingRequest struct {
	UserId int `json:"userId" binding:"required,gt=0"`
}

type rescheduleRequest struct {
	UserId   int `json:"userId" binding:"required,gt=0"`
	ToSlotId int `json:"toSlotId" binding:"required,gt=0"`
}

// GET /slots
func (h *SlotsHandler) GetAll(c *gin.Context) {
	slots, err := h.slots.GetAll(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, slots)
}

// GET /slots/:id
func (h *SlotsHandler) GetById(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	slot, err := h.slots.GetById(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, slot)
}

// POST /slots
func (h *SlotsHandler) Create(c *gin.Context) {
	var req createSlotRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err)
		return
	}
	slot, err := req.toModel()
	if err != nil {
		badRequest(c, err)
		return
	}
	created, err := h.slots.AddSlot(c.Request.Context(), slot)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, created)
}

// POST /slots/bulk
func (h *SlotsHandler) CreateBulk(c *gin.Context) {
	var req createBulkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err)
		return
	}
	slots := make([]models.Slot, 0, len(req.Slots))
	for i, r := range req.Slots {
		slot, err := r.toModel()
		if err != nil {
			badRequest(c, fmt.Errorf("slots[%d]: %w", i, err))
			return
		}
		slots = append(slots, slot)
	}
	created, err := h.slots.AddBulk(c.Request.Context(), slots)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, created)
}

// POST /slots/:id/hold
func (h *SlotsHandler) Hold(c *gin.Context) {
	h.changeBooking(c, h.slots.Hold)
}

// POST /slots/:id/book
func (h *SlotsHandler) Book(c *gin.Context) {
	h.changeBooking(c, h.slots.Book)
}

// POST /slots/:id/cancel
func (h *SlotsHandler) Cancel(c *gin.Context) {
	h.changeBooking(c, h.slots.Cancel)
}

// POST /slots/:id/reschedule
func (h *SlotsHandler) Reschedule(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var req rescheduleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err)
		return
	}
	slot, err := h.slots.Reschedule(c.Request.Context(), id, req.ToSlotId, req.UserId)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, slot)
}

func (h *SlotsHandler) changeBooking(c *gin.Context, op func(ctx context.Context, slotId, userId int) (models.Slot, error)) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var req bookingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err)
		return
	}
	slot, err := op(c.Request.Context(), id, req.UserId)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, slot)
}
