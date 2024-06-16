package main

import (
	"database/sql"
	"log"
	"sync"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/session"
	"github.com/gofiber/template/html/v2"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

var db *sql.DB
var dbOnce sync.Once

func initDB() {
	var err error
	db, err = sql.Open("sqlite3", "./data.db")
	if err != nil {
		log.Fatal(err)
	}

	// Drop the URLs table if it exists to ensure schema updates are applied
	_, err = db.Exec(`DROP TABLE IF EXISTS urls`)
	if err != nil {
		log.Fatal(err)
	}

	createUsersTable := `CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT NOT NULL UNIQUE,
		password TEXT NOT NULL
	);`

	createUrlsTable := `CREATE TABLE IF NOT EXISTS urls (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		short_id TEXT NOT NULL UNIQUE,
		original_url TEXT NOT NULL,
		username TEXT NOT NULL
	);`

	_, err = db.Exec(createUsersTable)
	if err != nil {
		log.Fatal(err)
	}

	_, err = db.Exec(createUrlsTable)
	if err != nil {
		log.Fatal(err)
	}
}

func main() {
	dbOnce.Do(initDB)

	engine := html.New("./templates", ".html")

	app := fiber.New(fiber.Config{
		Views: engine,
	})

	store := session.New()

	app.Static("/", "./static")

	app.Get("/", func(c *fiber.Ctx) error {
		sess, _ := store.Get(c)
		user := sess.Get("user")

		var urls []fiber.Map
		if user != nil {
			rows, err := db.Query("SELECT short_id, original_url FROM urls WHERE username = ?", user)
			if err != nil {
				return c.Status(fiber.StatusInternalServerError).SendString("Could not retrieve URLs: " + err.Error())
			}
			defer rows.Close()

			for rows.Next() {
				var shortID, originalURL string
				if err := rows.Scan(&shortID, &originalURL); err != nil {
					return c.Status(fiber.StatusInternalServerError).SendString("Could not scan URL: " + err.Error())
				}
				urls = append(urls, fiber.Map{
					"ShortID":     shortID,
					"OriginalURL": originalURL,
				})
			}
			if err := rows.Err(); err != nil {
				return c.Status(fiber.StatusInternalServerError).SendString("Row iteration error: " + err.Error())
			}
		}

		return c.Render("index", fiber.Map{
			"User": user,
			"URLs": urls,
		})
	})

	app.Get("/register", func(c *fiber.Ctx) error {
		return c.Render("register", nil)
	})

	app.Post("/register", func(c *fiber.Ctx) error {
		username := c.FormValue("username")
		password := c.FormValue("password")

		if username == "" || password == "" {
			return c.Status(fiber.StatusBadRequest).SendString("Username and Password are required")
		}

		_, err := db.Exec("INSERT INTO users (username, password) VALUES (?, ?)", username, password)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Could not register user: " + err.Error())
		}

		return c.Redirect("/login")
	})

	app.Get("/login", func(c *fiber.Ctx) error {
		return c.Render("login", nil)
	})

	app.Post("/login", func(c *fiber.Ctx) error {
		username := c.FormValue("username")
		password := c.FormValue("password")

		var storedPassword string
		err := db.QueryRow("SELECT password FROM users WHERE username = ?", username).Scan(&storedPassword)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).SendString("Invalid credentials")
		}

		if storedPassword != password {
			return c.Status(fiber.StatusUnauthorized).SendString("Invalid credentials")
		}

		sess, _ := store.Get(c)
		sess.Set("user", username)
		sess.Save()

		return c.Redirect("/")
	})

	app.Get("/logout", func(c *fiber.Ctx) error {
		sess, _ := store.Get(c)
		sess.Destroy()
		return c.Redirect("/")
	})

	app.Post("/shorten", func(c *fiber.Ctx) error {
		originalURL := c.FormValue("url")
		if originalURL == "" {
			return c.Status(fiber.StatusBadRequest).SendString("URL is required")
		}

		sess, _ := store.Get(c)
		user := sess.Get("user")
		if user == nil {
			return c.Status(fiber.StatusUnauthorized).SendString("You must be logged in to shorten a URL")
		}

		shortID := uuid.New().String()[:8]
		_, err := db.Exec("INSERT INTO urls (short_id, original_url, username) VALUES (?, ?, ?)", shortID, originalURL, user)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Could not shorten URL: " + err.Error())
		}

		shortURL := c.BaseURL() + "/" + shortID
		return c.Render("index", fiber.Map{
			"ShortURL": shortURL,
			"User":     user,
		})
	})

	app.Get("/:id", func(c *fiber.Ctx) error {
		id := c.Params("id")

		var originalURL string
		err := db.QueryRow("SELECT original_url FROM urls WHERE short_id = ?", id).Scan(&originalURL)
		if err != nil {
			return c.Status(fiber.StatusNotFound).SendString("URL not found")
		}

		return c.Redirect(originalURL)
	})

	app.Listen(":3000")
}

// TODO: NEED TO FIX SOME BUG
