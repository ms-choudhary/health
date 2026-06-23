package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"health/db/queries"
)

func (h *Handler) ListIngredients(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	ingredients, err := h.Q.ListIngredients(r.Context(), &q)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if ingredients == nil {
		ingredients = []queries.Ingredient{}
	}
	writeJSON(w, http.StatusOK, ingredients)
}

type createIngredientBody struct {
	Name            string  `json:"name"`
	Unit            string  `json:"unit"`
	CaloriesPerUnit float64 `json:"calories_per_unit"`
	ProteinPerUnit  float64 `json:"protein_per_unit"`
}

func (h *Handler) CreateIngredient(w http.ResponseWriter, r *http.Request) {
	var body createIngredientBody
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
	if body.ProteinPerUnit < 0 {
		writeError(w, http.StatusBadRequest, "protein_per_unit must be >= 0")
		return
	}
	ingredient, err := h.Q.CreateIngredient(r.Context(), queries.CreateIngredientParams{
		Name:            name,
		Unit:            unit,
		CaloriesPerUnit: body.CaloriesPerUnit,
		ProteinPerUnit:  body.ProteinPerUnit,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, ingredient)
}

type updateIngredientBody struct {
	CaloriesPerUnit float64 `json:"calories_per_unit"`
	ProteinPerUnit  float64 `json:"protein_per_unit"`
}

func (h *Handler) UpdateIngredient(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := h.Q.GetIngredient(r.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "ingredient not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var body updateIngredientBody
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.CaloriesPerUnit < 0 {
		writeError(w, http.StatusBadRequest, "calories_per_unit must be >= 0")
		return
	}
	if body.ProteinPerUnit < 0 {
		writeError(w, http.StatusBadRequest, "protein_per_unit must be >= 0")
		return
	}

	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer func() { _ = tx.Rollback() }()
	q := h.Q.WithTx(tx)

	ingredient, err := q.UpdateIngredientNutrition(r.Context(), queries.UpdateIngredientNutritionParams{
		ID:              id,
		CaloriesPerUnit: body.CaloriesPerUnit,
		ProteinPerUnit:  body.ProteinPerUnit,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	ingredientID := id
	if err := q.RestampLogEntriesForIngredient(r.Context(), queries.RestampLogEntriesForIngredientParams{
		CaloriesPerUnit: body.CaloriesPerUnit,
		ProteinPerUnit:  body.ProteinPerUnit,
		IngredientID:    &ingredientID,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ingredient)
}

func (h *Handler) GetRecipesByIngredient(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	recipes, err := h.Q.GetRecipesByIngredient(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if recipes == nil {
		recipes = []queries.GetRecipesByIngredientRow{}
	}
	writeJSON(w, http.StatusOK, recipes)
}

func (h *Handler) DeleteIngredient(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.Q.DeleteIngredient(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
