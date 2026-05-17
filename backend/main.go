package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"health/db"
	"health/handlers"
)

func main() {
	dbPath := os.Getenv("HEALTH_DB")
	if dbPath == "" {
		dbPath = "health.db"
	}

	database, err := db.Init(dbPath)
	if err != nil {
		log.Fatalf("db init: %v", err)
	}
	defer database.Close()

	h := handlers.New(database.Queries)
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	mux.HandleFunc("GET /api/users", h.ListUsers)
	mux.HandleFunc("POST /api/users", h.CreateUser)
	mux.HandleFunc("DELETE /api/users/{id}", h.DeleteUser)
	mux.HandleFunc("GET /api/users/{id}/today", h.GetTodaySummary)

	mux.HandleFunc("GET /api/foods", h.ListFoods)
	mux.HandleFunc("POST /api/foods", h.CreateFood)
	mux.HandleFunc("DELETE /api/foods/{id}", h.DeleteFood)

	mux.HandleFunc("GET /api/users/{id}/log", h.GetLog)
	mux.HandleFunc("POST /api/users/{id}/log", h.AddLogEntry)
	mux.HandleFunc("DELETE /api/users/{id}/log/{eid}", h.DeleteLogEntry)
	mux.HandleFunc("GET /api/users/{id}/recent-foods", h.GetRecentFoods)

	mux.HandleFunc("GET /api/users/{id}/metrics", h.GetMetrics)
	mux.HandleFunc("PUT /api/users/{id}/metrics", h.UpsertMetrics)

	distDir := "../frontend/dist"
	if abs, err := filepath.Abs(distDir); err == nil {
		if info, err := os.Stat(abs); err == nil && info.IsDir() {
			fs := http.FileServer(http.Dir(abs))
			mux.Handle("/", spaHandler(abs, fs))
		}
	}

	addr := ":8080"
	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, withMiddleware(mux)); err != nil {
		log.Fatal(err)
	}
}

func spaHandler(root string, fs http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Join(root, filepath.Clean(r.URL.Path))
		if info, err := os.Stat(path); err != nil || info.IsDir() {
			http.ServeFile(w, r, filepath.Join(root, "index.html"))
			return
		}
		fs.ServeHTTP(w, r)
	})
}

func withMiddleware(next http.Handler) http.Handler {
	return cors(logRequest(next))
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func logRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}
