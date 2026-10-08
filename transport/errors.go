package transport

import (
	"errors"
	"log"
	"net/http"
	"slots/service"
	"strconv"

	"github.com/gin-gonic/gin"
)

func writeError(c *gin.Context, err error) {
	var vErr *service.ValidationError
	switch {
	case errors.As(err, &vErr):
		c.JSON(http.StatusBadRequest, gin.H{"error": vErr.Msg})
	case errors.Is(err, service.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrInvalidStatus),
		errors.Is(err, service.ErrSameSlot):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrSlotNotOwned):
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrDuplicate),
		errors.Is(err, service.ErrSlotAlreadyBooked),
		errors.Is(err, service.ErrSlotOnHold),
		errors.Is(err, service.ErrSlotNotBooked),
		errors.Is(err, service.ErrConcurrentUpdate):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	default:
		log.Printf("internal error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	}
}

func badRequest(c *gin.Context, err error) {
	c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
}

func pathID(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return 0, false
	}
	return id, true
}
