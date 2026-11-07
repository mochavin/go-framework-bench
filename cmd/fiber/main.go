// gofiber/fiber: built on fasthttp rather than net/http.
package main

import (
	"errors"
	"log"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"github.com/mochavin/go-framework-bench/internal/app"
)

func main() {
	a := app.New("fiber")
	f := fiber.New(fiber.Config{
		DisableStartupMessage: true,
		// fasthttp's default limit is 256k connections; keep the default
		// but make sure the 5000-VU scenario is not capped below it.
		Concurrency: 256 * 1024,
	})
	f.Use(recover.New())

	f.Get("/api/health", func(c *fiber.Ctx) error {
		return c.Status(200).JSON(a.Health())
	})
	f.Get("/api/posts", func(c *fiber.Ctx) error {
		posts, err := a.LatestPosts(c.Context())
		if err != nil {
			return c.Status(500).JSON(app.ServerErrorBody)
		}
		return c.Status(200).JSON(app.ListBody(posts))
	})
	f.Get("/api/posts/:id", func(c *fiber.Ctx) error {
		id, err := strconv.ParseInt(c.Params("id"), 10, 64)
		if err != nil {
			return c.Status(404).JSON(app.NotFoundBody)
		}
		p, err := a.FindPost(c.Context(), id)
		switch {
		case errors.Is(err, app.ErrNotFound):
			return c.Status(404).JSON(app.NotFoundBody)
		case err != nil:
			return c.Status(500).JSON(app.ServerErrorBody)
		}
		return c.Status(200).JSON(app.PostBody(p))
	})
	f.Post("/api/posts", func(c *fiber.Ctx) error {
		var in app.PostInput
		if err := c.BodyParser(&in); err != nil {
			return c.Status(400).JSON(app.InvalidJSONBody)
		}
		if errs := app.Validate(in); errs != nil {
			return c.Status(422).JSON(errs)
		}
		p, err := a.CreatePost(c.Context(), *in.Title, *in.Body)
		if err != nil {
			return c.Status(500).JSON(app.ServerErrorBody)
		}
		return c.Status(201).JSON(app.PostBody(p))
	})
	f.Get("/api/heavy", func(c *fiber.Ctx) error {
		return c.Status(200).JSON(app.Heavy())
	})

	log.Printf("fiber listening on %s", a.Addr)
	log.Fatal(f.Listen(a.Addr))
}
