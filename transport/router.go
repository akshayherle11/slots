package transport

import (
	"slots/docs"

	"github.com/gin-gonic/gin"
)

func NewRouter(users *UserHandler, slots *SlotsHandler) *gin.Engine {
	r := gin.Default()

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	r.GET("/openapi.yaml", func(c *gin.Context) {
		c.Data(200, "application/yaml", docs.OpenAPISpec)
	})
	r.GET("/docs", func(c *gin.Context) {
		c.Data(200, "text/html; charset=utf-8", []byte(docs.SwaggerUIPage))
	})

	u := r.Group("/users")
	u.POST("", users.Create)
	u.GET("/:id", users.GetById)
	u.GET("/:id/history", users.History)

	s := r.Group("/slots")
	s.GET("", slots.GetAll)
	s.POST("", slots.Create)
	s.POST("/bulk", slots.CreateBulk)
	s.GET("/:id", slots.GetById)
	s.POST("/:id/hold", slots.Hold)
	s.POST("/:id/book", slots.Book)
	s.POST("/:id/cancel", slots.Cancel)
	s.POST("/:id/reschedule", slots.Reschedule)

	return r
}
