package handlers

import (
	"net/http"
	"sort"
	"strconv"
	"time"

	"health/db/queries"
)

const recentWindowDays = 7
const recentItemsCap = 20

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

func (h *Handler) DeleteLogEntriesByRecipe(w http.ResponseWriter, r *http.Request) {
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
	srid, err := strconv.ParseInt(r.URL.Query().Get("source_recipe_id"), 10, 64)
	if err != nil || srid <= 0 {
		writeError(w, http.StatusBadRequest, "source_recipe_id required")
		return
	}
	if err := h.Q.DeleteLogEntriesByRecipe(r.Context(), queries.DeleteLogEntriesByRecipeParams{
		UserID:         userID,
		Date:           date,
		SourceRecipeID: &srid,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type recentItem struct {
	RecipeID      *int64  `json:"recipe_id"`
	RecipeName    string  `json:"recipe_name"`
	TotalCalories float64 `json:"total_calories"`
	TotalProtein  float64 `json:"total_protein"`
	LastServings  float64 `json:"last_servings"`
	maxID         int64
}

func (h *Handler) GetRecentRecipes(w http.ResponseWriter, r *http.Request) {
	userID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	floor := time.Now().AddDate(0, 0, -recentWindowDays).Format("2006-01-02")

	recipes, err := h.Q.GetRecentLoggedRecipes(r.Context(), queries.GetRecentLoggedRecipesParams{
		UserID:    userID,
		DateFloor: floor,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	items := make([]recentItem, 0, len(recipes))
	for _, rec := range recipes {
		rid := rec.RecipeID
		items = append(items, recentItem{
			RecipeID:      &rid,
			RecipeName:    rec.RecipeName,
			TotalCalories: rec.TotalCalories,
			TotalProtein:  rec.TotalProtein,
			LastServings:  rec.LastServings,
			maxID:         rec.MaxID,
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].maxID > items[j].maxID })
	if len(items) > recentItemsCap {
		items = items[:recentItemsCap]
	}

	writeJSON(w, http.StatusOK, items)
}
