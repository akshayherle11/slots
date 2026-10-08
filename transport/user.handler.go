package transport

import (
	"net/http"
	"slots/service"
	"strconv"

	"github.com/gin-gonic/gin"
)

type UserHandler struct {
	users service.UserService
}

func NewUserHandler(users service.UserService) *UserHandler {
	return &UserHandler{users: users}
}

type createUserRequest struct {
	Name  string `json:"name" binding:"required"`
	Email string `json:"email" binding:"required"`
}

// POST /users
func (h *UserHandler) Create(c *gin.Context) {
	var req createUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err)
		return
	}
	user, err := h.users.Create(c.Request.Context(), req.Name, req.Email)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, user)
}

// GET /users/:id
func (h *UserHandler) GetById(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	user, err := h.users.GetById(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, user)
}

// GET /users/:id/history?limit=50&offset=0
func (h *UserHandler) History(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	limit, ok := queryInt(c, "limit")
	if !ok {
		return
	}
	offset, ok := queryInt(c, "offset")
	if !ok {
		return
	}
	history, err := h.users.History(c.Request.Context(), id, limit, offset)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, history)
}

// queryInt reads an optional integer query parameter; absent means 0.
func queryInt(c *gin.Context, name string) (int, bool) {
	raw := c.Query(name)
	if raw == "" {
		return 0, true
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid " + name})
		return 0, false
	}
	return v, true
}
