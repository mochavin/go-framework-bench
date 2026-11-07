// labstack/echo without the request logger.
package main

import (
	"errors"
	"log"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/mochavin/go-framework-bench/internal/app"
)

func main() {
	a := app.New("echo")
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.Use(middleware.Recover())

	e.GET("/api/health", func(c echo.Context) error {
		return c.JSON(200, a.Health())
	})
	e.GET("/api/posts", func(c echo.Context) error {
		posts, err := a.LatestPosts(c.Request().Context())
		if err != nil {
			return c.JSON(500, app.ServerErrorBody)
		}
		return c.JSON(200, app.ListBody(posts))
	})
	e.GET("/api/posts/:id", func(c echo.Context) error {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			return c.JSON(404, app.NotFoundBody)
		}
		p, err := a.FindPost(c.Request().Context(), id)
		switch {
		case errors.Is(err, app.ErrNotFound):
			return c.JSON(404, app.NotFoundBody)
		case err != nil:
			return c.JSON(500, app.ServerErrorBody)
		}
		return c.JSON(200, app.PostBody(p))
	})
	e.POST("/api/posts", func(c echo.Context) error {
		var in app.PostInput
		if err := c.Bind(&in); err != nil {
			return c.JSON(400, app.InvalidJSONBody)
		}
		if errs := app.Validate(in); errs != nil {
			return c.JSON(422, errs)
		}
		p, err := a.CreatePost(c.Request().Context(), *in.Title, *in.Body)
		if err != nil {
			return c.JSON(500, app.ServerErrorBody)
		}
		return c.JSON(201, app.PostBody(p))
	})
	e.GET("/api/heavy", func(c echo.Context) error {
		return c.JSON(200, app.Heavy())
	})

	log.Printf("echo listening on %s", a.Addr)
	log.Fatal(e.Start(a.Addr))
}
