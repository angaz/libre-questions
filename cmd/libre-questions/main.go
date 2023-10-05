package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"html/template"
	"io/fs"
	"net/http"
	"os"
	"time"

	"log/slog"

	"github.com/angaz/libre-questions/public"
	_ "modernc.org/sqlite"
)

type Data struct {
	ID    string
	Name  string
	Total int
}

type Server struct {
	Template *template.Template
	DB       *sql.DB
}

func (s *Server) NameHander(w http.ResponseWriter, r *http.Request, id string) {
	name := r.FormValue("name")

	if name == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	rows, err := s.DB.QueryContext(
		r.Context(), `
			INSERT INTO
				users (id, name, total)
				VALUES (?, ?, 0)
			ON CONFLICT (id) DO UPDATE
			SET
				name = excluded.name
			RETURNING id, name, total
		`,
		id,
		name,
	)
	if err != nil {
		slog.Error("failed to upsert user name", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	data := Data{}
	for rows.Next() {
		rows.Scan(
			&data.ID,
			&data.Name,
			&data.Total,
		)
	}

	s.Template.ExecuteTemplate(w, "index.html", data)
}

func (s *Server) IncreaseCount(w http.ResponseWriter, r *http.Request, id string) {
	rows, err := s.DB.QueryContext(
		r.Context(), `
			UPDATE users
			SET
				total = total + 1
			WHERE id = ?
			RETURNING id, name, total
		`,
		id,
	)
	if err != nil {
		slog.Error("failed to update total", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	data := Data{}
	for rows.Next() {
		rows.Scan(
			&data.ID,
			&data.Name,
			&data.Total,
		)
	}

	s.Template.ExecuteTemplate(w, "counter", data)
}

func sessionGenerator() string {
	randSession := make([]byte, 32)
	_, err := rand.Reader.Read(randSession)
	if err != nil {
		slog.Error("session generator rand read failed", "err", err)
		panic(nil)
	}

	outSession := make([]byte, hex.EncodedLen(len(randSession)))
	_ = hex.Encode(outSession, randSession)

	return string(outSession)
}

func dbSetup() *sql.DB {
	db, err := sql.Open("sqlite", "libre-questions.db")
	if err != nil {
		slog.Error("failed to open db", "err", err)
		os.Exit(1)
	}

	_, err = db.Exec(`PRAGMA journal_mode=WAL`)
	if err != nil {
		slog.Error("failed to set WAL mode", "err", err)
		os.Exit(1)
	}

	dbTables(db)

	return db
}

func dbTables(db *sql.DB) {
	_, err := db.Exec(
		`
			CREATE TABLE IF NOT EXISTS users (
				id		TEXT	PRIMARY KEY,
				name	TEXT	NOT NULL,
				total	INT		NOT NULL DEFAULT 0
			)
		`,
	)
	if err != nil {
		slog.Error("creating user table failed", "err", err)
		os.Exit(1)
	}
}

func main() {
	templates, err := template.ParseFS(public.Templates, "*.tmpl")
	if err != nil {
		slog.Error("failed to read public templates", "err", err)
		os.Exit(1)
	}

	staticFiles, err := fs.Sub(public.Static, "static")
	fileServer := http.FileServer(http.FS(staticFiles))

	db := dbSetup()

	server := Server{
		Template: templates,
		DB:       db,
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		isHX := r.Header.Get("HX-Request") != ""

		defer func() {
			slog.Info("request", "path", r.URL.Path, "is_hx", isHX, "duration", time.Now().Sub(start))
		}()

		idCookie, _ := r.Cookie("id")
		if idCookie == nil {
			idCookie = &http.Cookie{
				Name:     "id",
				Value:    sessionGenerator(),
				SameSite: http.SameSiteStrictMode,
				HttpOnly: true,
				Expires:  time.Now().Add(time.Hour * 24 * 7),
			}
			w.Header().Add("Set-Cookie", idCookie.String())
		}

		id := idCookie.Value

		path := r.URL.Path
		if path == "/" {
			path = "index.html"
		} else if path[0] == '/' {
			path = path[1:]
		}

		switch path {
		case "name":
			server.NameHander(w, r, id)
		case "increase_count":
			server.IncreaseCount(w, r, id)
		default:
			t := templates.Lookup(path)
			if t == nil {
				fileServer.ServeHTTP(w, r)
				return
			}

			rows, err := server.DB.QueryContext(
				r.Context(),
				`
					SELECT id, name, total
					FROM users
					WHERE id = ?
				`,
				id,
			)
			if err != nil {
				slog.Error("failed to update total", "err", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			defer rows.Close()

			data := Data{}
			for rows.Next() {
				rows.Scan(
					&data.ID,
					&data.Name,
					&data.Total,
				)
			}

			t.Execute(w, data)
		}
	})

	listenAddress := "127.0.0.1:8080"
	slog.Info("starting server", "address", listenAddress)
	err = http.ListenAndServe(listenAddress, nil)
	if err != nil {
		slog.Error("http server failed", "err", err)
		os.Exit(1)
	}
}
