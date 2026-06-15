package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"health/db/queries"
)

type ingredientInput struct {
	IngredientID   int64   `json:"ingredient_id"`
	Quantity float64 `json:"quantity"`
}

type recipeBody struct {
	Name        string            `json:"name"`
	Ingredients []ingredientInput `json:"ingredients"`
	FoodTagIDs  []int64           `json:"food_tag_ids"`
}

type recipeDetailResponse struct {
	ID            int64                             `json:"id"`
	Name          string                            `json:"name"`
	CreatedAt     string                            `json:"created_at"`
	TotalCalories float64                           `json:"total_calories"`
	TotalProtein  float64                           `json:"total_protein"`
	Ingredients   []queries.GetRecipeIngredientsRow `json:"ingredients"`
	FoodTags      []queries.FoodTag                 `json:"food_tags"`
}

func validateRecipeBody(body recipeBody) (string, []ingredientInput, []int64, error) {
	name := strings.TrimSpace(body.Name)
	if name == "" {
		return "", nil, nil, errors.New("name required")
	}
	if len(body.Ingredients) == 0 {
		return "", nil, nil, errors.New("at least one ingredient required")
	}
	for _, ing := range body.Ingredients {
		if ing.IngredientID <= 0 {
			return "", nil, nil, errors.New("ingredient ingredient_id must be > 0")
		}
		if ing.Quantity <= 0 {
			return "", nil, nil, errors.New("ingredient quantity must be > 0")
		}
	}
	for _, id := range body.FoodTagIDs {
		if id <= 0 {
			return "", nil, nil, errors.New("food_tag_id must be > 0")
		}
	}
	return name, body.Ingredients, body.FoodTagIDs, nil
}

func (h *Handler) applyRecipeFoodTags(w http.ResponseWriter, r *http.Request, q *queries.Queries, recipeID int64, foodTagIDs []int64) bool {
	for _, tid := range foodTagIDs {
		if _, err := q.GetFoodTag(r.Context(), tid); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusBadRequest, "food tag not found — create it first")
				return false
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return false
		}
		if err := q.AddRecipeFoodTag(r.Context(), queries.AddRecipeFoodTagParams{RecipeID: recipeID, FoodTagID: tid}); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return false
		}
	}
	return true
}

type recipeListResponse struct {
	queries.ListRecipesRow
	FoodTags []queries.FoodTag `json:"food_tags"`
}

func (h *Handler) ListRecipes(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	rows, err := h.Q.ListRecipes(r.Context(), &q)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	tagsByRecipe, err := h.recipeFoodTagMap(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]recipeListResponse, 0, len(rows))
	for _, row := range rows {
		tags := tagsByRecipe[row.ID]
		if tags == nil {
			tags = []queries.FoodTag{}
		}
		out = append(out, recipeListResponse{ListRecipesRow: row, FoodTags: tags})
	}
	writeJSON(w, http.StatusOK, out)
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
	tags, err := h.Q.GetRecipeFoodTags(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tags == nil {
		tags = []queries.FoodTag{}
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
		FoodTags:      tags,
	})
}

func (h *Handler) CreateRecipe(w http.ResponseWriter, r *http.Request) {
	var body recipeBody
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	name, ingredients, foodTagIDs, err := validateRecipeBody(body)
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
			IngredientID:   ing.IngredientID,
			Quantity: ing.Quantity,
		}); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if !h.applyRecipeFoodTags(w, r, q, recipe.ID, foodTagIDs) {
		return
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
	name, ingredients, foodTagIDs, err := validateRecipeBody(body)
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
			IngredientID:   ing.IngredientID,
			Quantity: ing.Quantity,
		}); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if err := q.ClearRecipeFoodTags(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !h.applyRecipeFoodTags(w, r, q, id, foodTagIDs) {
		return
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
	Servings float64 `json:"servings"`
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
	if body.Servings <= 0 {
		writeError(w, http.StatusBadRequest, "servings must be > 0")
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
		ingredientID := ing.IngredientID
		recipeID := recipe.ID
		recipeName := recipe.Name
		qty := ing.Quantity * body.Servings
		entry, err := q.AddLogEntry(r.Context(), queries.AddLogEntryParams{
			UserID:               userID,
			IngredientID:               &ingredientID,
			Date:                 body.Date,
			IngredientName:             ing.IngredientName,
			IngredientUnit:             ing.IngredientUnit,
			CaloriesPerUnit:      ing.CaloriesPerUnit,
			ProteinPerUnit:       ing.ProteinPerUnit,
			Quantity:             qty,
			Calories:             ing.CaloriesPerUnit * qty,
			Protein:              ing.ProteinPerUnit * qty,
			SourceRecipeID:       &recipeID,
			SourceRecipeName:     &recipeName,
			SourceRecipeServings: &body.Servings,
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
