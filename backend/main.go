package main

import (
	"bytes"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"health/db"
	"health/handlers"
)

func main() {
	dbPath := os.Getenv("HEALTH_DB")
	if dbPath == "" {
		dbPath = "health.db"
	}

	if os.Getenv("GEMINI_API_KEY") == "" {
		log.Println("warning: GEMINI_API_KEY not set — AI calorie hints disabled")
	}

	database, err := db.Init(dbPath)
	if err != nil {
		log.Fatalf("db init: %v", err)
	}
	defer database.Close()

	h := handlers.New(database.Conn, database.Queries)
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	mux.HandleFunc("GET /api/users", h.ListUsers)
	mux.HandleFunc("POST /api/users", h.CreateUser)
	mux.HandleFunc("PUT /api/users/{id}", h.UpdateUser)
	mux.HandleFunc("DELETE /api/users/{id}", h.DeleteUser)
	mux.HandleFunc("GET /api/users/{id}/today", h.GetTodaySummary)

	mux.HandleFunc("GET /api/ingredients", h.ListIngredients)
	mux.HandleFunc("POST /api/ingredients", h.CreateIngredient)
	mux.HandleFunc("PUT /api/ingredients/{id}", h.UpdateIngredient)
	mux.HandleFunc("DELETE /api/ingredients/{id}", h.DeleteIngredient)

	mux.HandleFunc("GET /api/users/{id}/log", h.GetLog)
	mux.HandleFunc("DELETE /api/users/{id}/log/recipe", h.DeleteLogEntriesByGroup)
	mux.HandleFunc("POST /api/users/{id}/log/recipe", h.LogRecipe)
	mux.HandleFunc("POST /api/users/{id}/log/custom-recipe", h.LogCustomRecipe)
	mux.HandleFunc("DELETE /api/users/{id}/log/{eid}", h.DeleteLogEntry)

	mux.HandleFunc("GET /api/recipes", h.ListRecipes)
	mux.HandleFunc("POST /api/recipes", h.CreateRecipe)
	mux.HandleFunc("GET /api/recipes/{id}", h.GetRecipe)
	mux.HandleFunc("PUT /api/recipes/{id}", h.UpdateRecipe)
	mux.HandleFunc("DELETE /api/recipes/{id}", h.DeleteRecipe)

	mux.HandleFunc("GET /api/food-tags", h.ListFoodTags)
	mux.HandleFunc("POST /api/food-tags", h.CreateFoodTag)
	mux.HandleFunc("DELETE /api/food-tags/{id}", h.DeleteFoodTag)
	mux.HandleFunc("GET /api/food-tags/{id}/recipes", h.GetRecipesByFoodTag)

	mux.HandleFunc("GET /api/users/{id}/metrics", h.GetMetrics)
	mux.HandleFunc("PUT /api/users/{id}/metrics", h.UpsertMetrics)

	mux.HandleFunc("GET /api/exercises", h.ListExercises)
	mux.HandleFunc("POST /api/exercises", h.CreateExercise)
	mux.HandleFunc("GET /api/exercises/{id}", h.GetExercise)
	mux.HandleFunc("PUT /api/exercises/{id}", h.UpdateExercise)
	mux.HandleFunc("DELETE /api/exercises/{id}", h.DeleteExercise)

	mux.HandleFunc("GET /api/exercise-tags", h.ListExerciseTags)
	mux.HandleFunc("POST /api/exercise-tags", h.CreateExerciseTag)
	mux.HandleFunc("DELETE /api/exercise-tags/{id}", h.DeleteExerciseTag)
	mux.HandleFunc("GET /api/exercise-tags/{id}/exercises", h.GetExercisesByTag)

	mux.HandleFunc("GET /api/users/{id}/sets", h.GetSets)
	mux.HandleFunc("POST /api/users/{id}/sets", h.AddSets)
	mux.HandleFunc("PUT /api/users/{id}/sets/{sid}", h.UpdateSet)
	mux.HandleFunc("DELETE /api/users/{id}/sets/{sid}", h.DeleteSet)
	mux.HandleFunc("GET /api/users/{id}/exercises/{eid}/last-sets", h.GetLastSets)
	mux.HandleFunc("GET /api/users/{id}/exercise-progress", h.GetExerciseProgress)

	mux.HandleFunc("POST /api/ai/calorie-hint", h.CalorieHint)

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
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not found"}`))
			return
		}
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

// respRecorder wraps http.ResponseWriter to capture the status code and, for
// error responses, a copy of the response body so failures can be logged.
type respRecorder struct {
	http.ResponseWriter
	status  int
	capture bool
	body    bytes.Buffer
}

func (rr *respRecorder) WriteHeader(code int) {
	rr.status = code
	rr.capture = code >= http.StatusBadRequest
	rr.ResponseWriter.WriteHeader(code)
}

func (rr *respRecorder) Write(b []byte) (int, error) {
	if rr.status == 0 {
		rr.status = http.StatusOK
	}
	if rr.capture {
		rr.body.Write(b)
	}
	return rr.ResponseWriter.Write(b)
}

func logRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &respRecorder{ResponseWriter: w}

		defer func() {
			if v := recover(); v != nil {
				log.Printf("PANIC %s %s: %v\n%s", r.Method, r.URL.Path, v, debug.Stack())
				if rec.status == 0 {
					rec.ResponseWriter.Header().Set("Content-Type", "application/json")
					rec.ResponseWriter.WriteHeader(http.StatusInternalServerError)
					_, _ = rec.ResponseWriter.Write([]byte(`{"error":"internal server error"}`))
				}
			}
		}()

		next.ServeHTTP(rec, r)

		dur := time.Since(start)
		if rec.status >= http.StatusBadRequest {
			log.Printf("ERROR %s %s -> %d (%s): %s",
				r.Method, r.URL.Path, rec.status, dur, strings.TrimSpace(rec.body.String()))
		} else {
			log.Printf("%s %s %d %s", r.Method, r.URL.Path, rec.status, dur)
		}
	})
}
