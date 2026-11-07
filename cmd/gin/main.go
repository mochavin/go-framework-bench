// gin-gonic/gin in release mode, without the request logger.
package main

import (
	"errors"
	"log"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/mochavin/go-framework-bench/internal/app"
)

func main() {
	a := app.New("gin")
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	r.GET("/api/health", func(c *gin.Context) {
		c.JSON(200, a.Health())
	})
	r.GET("/api/posts", func(c *gin.Context) {
		posts, err := a.LatestPosts(c.Request.Context())
		if err != nil {
			c.JSON(500, app.ServerErrorBody)
			return
		}
		c.JSON(200, app.ListBody(posts))
	})
	r.GET("/api/posts/:id", func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(404, app.NotFoundBody)
			return
		}
		p, err := a.FindPost(c.Request.Context(), id)
		switch {
		case errors.Is(err, app.ErrNotFound):
			c.JSON(404, app.NotFoundBody)
		case err != nil:
			c.JSON(500, app.ServerErrorBody)
		default:
			c.JSON(200, app.PostBody(p))
		}
	})
	r.POST("/api/posts", func(c *gin.Context) {
		var in app.PostInput
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(400, app.InvalidJSONBody)
			return
		}
		if errs := app.Validate(in); errs != nil {
			c.JSON(422, errs)
			return
		}
		p, err := a.CreatePost(c.Request.Context(), *in.Title, *in.Body)
		if err != nil {
			c.JSON(500, app.ServerErrorBody)
			return
		}
		c.JSON(201, app.PostBody(p))
	})
	r.GET("/api/heavy", func(c *gin.Context) {
		c.JSON(200, app.Heavy())
	})

	log.Printf("gin listening on %s", a.Addr)
	log.Fatal(r.Run(a.Addr))
}
