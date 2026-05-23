package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"health/db/queries"
)

type ingredientInput struct {
	FoodID   int64   `json:"food_id"`
	Quantity float64 `json:"quantity"`
}

type recipeBody struct {
	Name        string            `json:"name"`
	Ingredients []ingredientInput `json:"ingredients"`
}

type recipeDetailResponse struct {
	ID            int64                             `json:"id"`
	Name          string                            `json:"name"`
	CreatedAt     string                            `json:"created_at"`
	TotalCalories float64                           `json:"total_calories"`
	TotalProtein  float64                           `json:"total_protein"`
	Ingredients   []queries.GetRecipeIngredientsRow `json:"ingredients"`
}

func validateRecipeBody(body recipeBody) (string, []ingredientInput, error) {
	name := strings.TrimSpace(body.Name)
	if name == "" {
		return "", nil, errors.New("name required")
	}
	if len(body.Ingredients) == 0 {
		return "", nil, errors.New("at least one ingredient required")
	}
	for _, ing := range body.Ingredients {
		if ing.FoodID <= 0 {
			return "", nil, errors.New("ingredient food_id must be > 0")
		}
		if ing.Quantity <= 0 {
			return "", nil, errors.New("ingredient quantity must be > 0")
		}
	}
	return name, body.Ingredients, nil
}

func (h *Handler) ListRecipes(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	rows, err := h.Q.ListRecipes(r.Context(), &q)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if rows == nil {
		rows = []queries.ListRecipesRow{}
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *Handler) GetRecipe(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	recipe, err := h.Q.GetRecipe(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "recipe not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	ings, err := h.Q.GetRecipeIngredients(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if ings == nil {
		ings = []queries.GetRecipeIngredientsRow{}
	}
	var totalCal, totalProt float64
	for _, ing := range ings {
		totalCal += ing.CaloriesPerUnit * ing.Quantity
		totalProt += ing.ProteinPerUnit * ing.Quantity
	}
	writeJSON(w, http.StatusOK, recipeDetailResponse{
		ID:            recipe.ID,
		Name:          recipe.Name,
		CreatedAt:     recipe.CreatedAt,
		TotalCalories: totalCal,
		TotalProtein:  totalProt,
		Ingredients:   ings,
	})
}

func (h *Handler) CreateRecipe(w http.ResponseWriter, r *http.Request) {
	var body recipeBody
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	name, ingredients, err := validateRecipeBody(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer func() {
		_ = tx.Rollback()
	}()
	q := h.Q.WithTx(tx)
	recipe, err := q.CreateRecipe(r.Context(), name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, ing := range ingredients {
		if _, err := q.AddRecipeIngredient(r.Context(), queries.AddRecipeIngredientParams{
			RecipeID: recipe.ID,
			FoodID:   ing.FoodID,
			Quantity: ing.Quantity,
		}); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, recipe)
}

func (h *Handler) UpdateRecipe(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var body recipeBody
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	name, ingredients, err := validateRecipeBody(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := h.Q.GetRecipe(r.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "recipe not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer func() {
		_ = tx.Rollback()
	}()
	q := h.Q.WithTx(tx)
	if err := q.UpdateRecipeName(r.Context(), queries.UpdateRecipeNameParams{
		ID:   id,
		Name: name,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := q.ClearRecipeIngredients(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, ing := range ingredients {
		if _, err := q.AddRecipeIngredient(r.Context(), queries.AddRecipeIngredientParams{
			RecipeID: id,
			FoodID:   ing.FoodID,
			Quantity: ing.Quantity,
		}); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, queries.Recipe{ID: id, Name: name})
}

func (h *Handler) DeleteRecipe(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.Q.DeleteRecipe(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type logRecipeBody struct {
	RecipeID int64   `json:"recipe_id"`
	Scale    float64 `json:"scale"`
	Date     string  `json:"date"`
}

func (h *Handler) LogRecipe(w http.ResponseWriter, r *http.Request) {
	userID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var body logRecipeBody
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.RecipeID <= 0 {
		writeError(w, http.StatusBadRequest, "recipe_id required")
		return
	}
	if body.Scale <= 0 {
		writeError(w, http.StatusBadRequest, "scale must be > 0")
		return
	}
	if !validDate(body.Date) {
		writeError(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}
	recipe, err := h.Q.GetRecipe(r.Context(), body.RecipeID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "recipe not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	ings, err := h.Q.GetRecipeIngredients(r.Context(), body.RecipeID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(ings) == 0 {
		writeError(w, http.StatusBadRequest, "recipe has no ingredients")
		return
	}
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer func() {
		_ = tx.Rollback()
	}()
	q := h.Q.WithTx(tx)
	out := make([]queries.LogEntry, 0, len(ings))
	for _, ing := range ings {
		foodID := ing.FoodID
		recipeID := recipe.ID
		recipeName := recipe.Name
		qty := ing.Quantity * body.Scale
		entry, err := q.AddLogEntry(r.Context(), queries.AddLogEntryParams{
			UserID:           userID,
			FoodID:           &foodID,
			Date:             body.Date,
			FoodName:         ing.FoodName,
			FoodUnit:         ing.FoodUnit,
			CaloriesPerUnit:  ing.CaloriesPerUnit,
			ProteinPerUnit:   ing.ProteinPerUnit,
			Quantity:         qty,
			Calories:         ing.CaloriesPerUnit * qty,
			Protein:          ing.ProteinPerUnit * qty,
			SourceRecipeID:   &recipeID,
			SourceRecipeName: &recipeName,
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
