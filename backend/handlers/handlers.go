package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"

	"health/db/queries"
)

type Handler struct {
	Q *queries.Queries
}

func New(q *queries.Queries) *Handler {
	return &Handler{Q: q}
}

type errorBody struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, errorBody{Error: msg})
}

func readJSON(r *http.Request, v interface{}) error {
	return json.NewDecoder(r.Body).Decode(v)
}

func parseID(r *http.Request, key string) (int64, error) {
	raw := r.PathValue(key)
	if raw == "" {
		return 0, errors.New("missing id")
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid id")
	}
	return id, nil
}

var dateRE = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

func validDate(s string) bool {
	return dateRE.MatchString(s)
}
