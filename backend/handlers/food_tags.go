package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"health/db/queries"
)

func (h *Handler) ListFoodTags(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Q.ListFoodTags(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if rows == nil {
		rows = []queries.ListFoodTagsRow{}
	}
	writeJSON(w, http.StatusOK, rows)
}

type createFoodTagBody struct {
	Name string `json:"name"`
}

func (h *Handler) CreateFoodTag(w http.ResponseWriter, r *http.Request) {
	var body createFoodTagBody
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}
	if _, err := h.Q.GetFoodTagByName(r.Context(), name); err == nil {
		writeError(w, http.StatusConflict, "food tag already exists")
		return
	} else if !errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	tag, err := h.Q.CreateFoodTag(r.Context(), name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, tag)
}

func (h *Handler) DeleteFoodTag(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.Q.DeleteFoodTag(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type recipeByFoodTagResponse struct {
	queries.ListRecipesByFoodTagRow
	FoodTags []queries.FoodTag `json:"food_tags"`
}

func (h *Handler) GetRecipesByFoodTag(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := h.Q.GetFoodTag(r.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "food tag not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	rows, err := h.Q.ListRecipesByFoodTag(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	tagsByRecipe, err := h.recipeFoodTagMap(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]recipeByFoodTagResponse, 0, len(rows))
	for _, row := range rows {
		tags := tagsByRecipe[row.ID]
		if tags == nil {
			tags = []queries.FoodTag{}
		}
		out = append(out, recipeByFoodTagResponse{ListRecipesByFoodTagRow: row, FoodTags: tags})
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) recipeFoodTagMap(r *http.Request) (map[int64][]queries.FoodTag, error) {
	all, err := h.Q.ListAllRecipeFoodTags(r.Context())
	if err != nil {
		return nil, err
	}
	m := make(map[int64][]queries.FoodTag)
	for _, t := range all {
		m[t.RecipeID] = append(m[t.RecipeID], queries.FoodTag{ID: t.ID, Name: t.Name, CreatedAt: t.CreatedAt})
	}
	return m, nil
}
