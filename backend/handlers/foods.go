package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"health/db/queries"
)

func (h *Handler) ListFoods(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	foods, err := h.Q.ListFoods(r.Context(), &q)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if foods == nil {
		foods = []queries.Food{}
	}
	writeJSON(w, http.StatusOK, foods)
}

type createFoodBody struct {
	Name            string  `json:"name"`
	Unit            string  `json:"unit"`
	CaloriesPerUnit float64 `json:"calories_per_unit"`
}

func (h *Handler) CreateFood(w http.ResponseWriter, r *http.Request) {
	var body createFoodBody
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}
	unit := strings.TrimSpace(body.Unit)
	if unit == "" {
		unit = "g"
	}
	if body.CaloriesPerUnit < 0 {
		writeError(w, http.StatusBadRequest, "calories_per_unit must be >= 0")
		return
	}
	food, err := h.Q.CreateFood(r.Context(), queries.CreateFoodParams{
		Name:            name,
		Unit:            unit,
		CaloriesPerUnit: body.CaloriesPerUnit,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, food)
}

type updateFoodBody struct {
	CaloriesPerUnit float64 `json:"calories_per_unit"`
}

func (h *Handler) UpdateFood(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := h.Q.GetFood(r.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "food not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var body updateFoodBody
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.CaloriesPerUnit < 0 {
		writeError(w, http.StatusBadRequest, "calories_per_unit must be >= 0")
		return
	}

	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer func() { _ = tx.Rollback() }()
	q := h.Q.WithTx(tx)

	food, err := q.UpdateFoodCalories(r.Context(), queries.UpdateFoodCaloriesParams{
		ID:              id,
		CaloriesPerUnit: body.CaloriesPerUnit,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	foodID := id
	if err := q.RestampLogEntriesForFood(r.Context(), queries.RestampLogEntriesForFoodParams{
		CaloriesPerUnit: body.CaloriesPerUnit,
		FoodID:          &foodID,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, food)
}

func (h *Handler) DeleteFood(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.Q.DeleteFood(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
