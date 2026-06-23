package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"health/db/queries"
)

func (h *Handler) GetLog(w http.ResponseWriter, r *http.Request) {
	userID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	entries, err := h.Q.GetLogHistory(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if entries == nil {
		entries = []queries.LogEntry{}
	}
	writeJSON(w, http.StatusOK, entries)
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

func (h *Handler) DeleteLogEntriesByGroup(w http.ResponseWriter, r *http.Request) {
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
	groupID, err := strconv.ParseInt(r.URL.Query().Get("recipe_group_id"), 10, 64)
	if err != nil || groupID == 0 {
		writeError(w, http.StatusBadRequest, "recipe_group_id required")
		return
	}
	rows, err := h.Q.DeleteLogEntriesByGroup(r.Context(), queries.DeleteLogEntriesByGroupParams{
		UserID:        userID,
		Date:          date,
		RecipeGroupID: &groupID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if rows == 0 {
		writeError(w, http.StatusNotFound, "no log entries found for that recipe group")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type customRecipeItem struct {
	IngredientID    *int64  `json:"ingredient_id"`
	IngredientName  string  `json:"ingredient_name"`
	IngredientUnit  string  `json:"ingredient_unit"`
	CaloriesPerUnit float64 `json:"calories_per_unit"`
	ProteinPerUnit  float64 `json:"protein_per_unit"`
	Quantity        float64 `json:"quantity"`
}

type logCustomRecipeBody struct {
	Name  string             `json:"name"`
	Date  string             `json:"date"`
	Items []customRecipeItem `json:"items"`
}

func (h *Handler) LogCustomRecipe(w http.ResponseWriter, r *http.Request) {
	userID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var body logCustomRecipeBody
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}
	if !validDate(body.Date) {
		writeError(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}
	if len(body.Items) == 0 {
		writeError(w, http.StatusBadRequest, "at least one ingredient required")
		return
	}
	for _, it := range body.Items {
		if strings.TrimSpace(it.IngredientName) == "" {
			writeError(w, http.StatusBadRequest, "ingredient_name required")
			return
		}
		if it.Quantity <= 0 {
			writeError(w, http.StatusBadRequest, "quantity must be > 0")
			return
		}
	}

	// Unique group id per log event (a unix millisecond timestamp), shared by every
	// ingredient of this custom recipe so they render as one group, distinct from
	// any other log event on the same day. Milliseconds keep the id within
	// JavaScript's safe integer range (nanoseconds overflow it).
	groupID := time.Now().UnixMilli()
	groupName := name

	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer func() { _ = tx.Rollback() }()
	q := h.Q.WithTx(tx)

	out := make([]queries.LogEntry, 0, len(body.Items))
	for _, it := range body.Items {
		entry, err := q.AddLogEntry(r.Context(), queries.AddLogEntryParams{
			UserID:               userID,
			IngredientID:         it.IngredientID,
			Date:                 body.Date,
			IngredientName:       it.IngredientName,
			IngredientUnit:       it.IngredientUnit,
			CaloriesPerUnit:      it.CaloriesPerUnit,
			ProteinPerUnit:       it.ProteinPerUnit,
			Quantity:             it.Quantity,
			Calories:             it.CaloriesPerUnit * it.Quantity,
			Protein:              it.ProteinPerUnit * it.Quantity,
			RecipeGroupID:        &groupID,
			SourceRecipeName:     &groupName,
			SourceRecipeServings: nil,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, entry)
	}

	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, out)
}
