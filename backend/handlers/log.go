package handlers

import (
	"net/http"
	"strings"

	"health/db/queries"
)

func (h *Handler) GetLog(w http.ResponseWriter, r *http.Request) {
	userID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	date := r.URL.Query().Get("date")
	if !validDate(date) {
		writeError(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}
	entries, err := h.Q.GetLogForDate(r.Context(), queries.GetLogForDateParams{
		UserID: userID,
		Date:   date,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if entries == nil {
		entries = []queries.LogEntry{}
	}
	writeJSON(w, http.StatusOK, entries)
}

type addLogBody struct {
	FoodID          *int64  `json:"food_id"`
	FoodName        string  `json:"food_name"`
	FoodUnit        string  `json:"food_unit"`
	CaloriesPerUnit float64 `json:"calories_per_unit"`
	Quantity        float64 `json:"quantity"`
	Date            string  `json:"date"`
}

func (h *Handler) AddLogEntry(w http.ResponseWriter, r *http.Request) {
	userID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var body addLogBody
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	body.FoodName = strings.TrimSpace(body.FoodName)
	body.FoodUnit = strings.TrimSpace(body.FoodUnit)
	if body.FoodName == "" {
		writeError(w, http.StatusBadRequest, "food_name required")
		return
	}
	if body.FoodUnit == "" {
		writeError(w, http.StatusBadRequest, "food_unit required")
		return
	}
	if body.Quantity <= 0 {
		writeError(w, http.StatusBadRequest, "quantity must be > 0")
		return
	}
	if body.CaloriesPerUnit < 0 {
		writeError(w, http.StatusBadRequest, "calories_per_unit must be >= 0")
		return
	}
	if !validDate(body.Date) {
		writeError(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}
	entry, err := h.Q.AddLogEntry(r.Context(), queries.AddLogEntryParams{
		UserID:          userID,
		FoodID:          body.FoodID,
		Date:            body.Date,
		FoodName:        body.FoodName,
		FoodUnit:        body.FoodUnit,
		CaloriesPerUnit: body.CaloriesPerUnit,
		Quantity:        body.Quantity,
		Calories:        body.CaloriesPerUnit * body.Quantity,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, entry)
}

func (h *Handler) DeleteLogEntry(w http.ResponseWriter, r *http.Request) {
	userID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	entryID, err := parseID(r, "eid")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.Q.DeleteLogEntry(r.Context(), queries.DeleteLogEntryParams{
		ID:     entryID,
		UserID: userID,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) GetRecentFoods(w http.ResponseWriter, r *http.Request) {
	userID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	rows, err := h.Q.GetRecentLoggedFoods(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if rows == nil {
		rows = []queries.GetRecentLoggedFoodsRow{}
	}
	writeJSON(w, http.StatusOK, rows)
}
