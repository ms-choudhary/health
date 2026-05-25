package handlers

import (
	"database/sql"
	"errors"
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

type logFoodBody struct {
	FoodID   *int64  `json:"food_id"`
	Quantity float64 `json:"quantity"`
	Date     string  `json:"date"`
}

func (h *Handler) AddLogEntry(w http.ResponseWriter, r *http.Request) {
	userID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var body logFoodBody
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.FoodID == nil || *body.FoodID <= 0 {
		writeError(w, http.StatusBadRequest, "food_id required")
		return
	}
	if body.Quantity <= 0 {
		writeError(w, http.StatusBadRequest, "quantity must be > 0")
		return
	}
	if !validDate(body.Date) {
		writeError(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}
	food, err := h.Q.GetFood(r.Context(), *body.FoodID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "food not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	entry, err := h.Q.AddLogEntry(r.Context(), queries.AddLogEntryParams{
		UserID:               userID,
		FoodID:               &food.ID,
		Date:                 body.Date,
		FoodName:             food.Name,
		FoodUnit:             food.Unit,
		CaloriesPerUnit:      food.CaloriesPerUnit,
		ProteinPerUnit:       food.ProteinPerUnit,
		Quantity:             body.Quantity,
		Calories:             food.CaloriesPerUnit * body.Quantity,
		Protein:              food.ProteinPerUnit * body.Quantity,
		SourceRecipeID:       nil,
		SourceRecipeName:     nil,
		SourceRecipeServings: nil,
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
	Kind            string  `json:"kind"`
	FoodID          *int64  `json:"food_id,omitempty"`
	FoodName        string  `json:"food_name,omitempty"`
	FoodUnit        string  `json:"food_unit,omitempty"`
	CaloriesPerUnit float64 `json:"calories_per_unit"`
	ProteinPerUnit  float64 `json:"protein_per_unit"`
	LastQuantity    float64 `json:"last_quantity"`
	RecipeID        *int64  `json:"recipe_id,omitempty"`
	RecipeName      string  `json:"recipe_name,omitempty"`
	TotalCalories   float64 `json:"total_calories"`
	TotalProtein    float64 `json:"total_protein"`
	LastServings    float64 `json:"last_servings"`
	maxID           int64
}

func (h *Handler) GetRecentFoods(w http.ResponseWriter, r *http.Request) {
	userID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	floor := time.Now().AddDate(0, 0, -recentWindowDays).Format("2006-01-02")

	foods, err := h.Q.GetRecentLoggedFoods(r.Context(), queries.GetRecentLoggedFoodsParams{
		UserID:    userID,
		DateFloor: floor,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	recipes, err := h.Q.GetRecentLoggedRecipes(r.Context(), queries.GetRecentLoggedRecipesParams{
		UserID:    userID,
		DateFloor: floor,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	items := make([]recentItem, 0, len(foods)+len(recipes))
	for _, f := range foods {
		items = append(items, recentItem{
			Kind:            "food",
			FoodID:          f.FoodID,
			FoodName:        f.FoodName,
			FoodUnit:        f.FoodUnit,
			CaloriesPerUnit: f.CaloriesPerUnit,
			ProteinPerUnit:  f.ProteinPerUnit,
			LastQuantity:    f.LastQuantity,
			maxID:           f.MaxID,
		})
	}
	for _, rec := range recipes {
		rid := rec.RecipeID
		items = append(items, recentItem{
			Kind:          "recipe",
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
