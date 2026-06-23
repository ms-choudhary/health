package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"health/db/queries"
)

func (h *Handler) ListExerciseTags(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Q.ListExerciseTags(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if rows == nil {
		rows = []queries.ListExerciseTagsRow{}
	}
	writeJSON(w, http.StatusOK, rows)
}

type createExerciseTagBody struct {
	Name string `json:"name"`
}

func (h *Handler) CreateExerciseTag(w http.ResponseWriter, r *http.Request) {
	var body createExerciseTagBody
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}
	if _, err := h.Q.GetExerciseTagByName(r.Context(), name); err == nil {
		writeError(w, http.StatusConflict, "exercise tag already exists")
		return
	} else if !errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	tag, err := h.Q.CreateExerciseTag(r.Context(), name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, tag)
}

func (h *Handler) DeleteExerciseTag(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.Q.DeleteExerciseTag(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type exerciseByTagResponse struct {
	queries.Exercise
	Tags []queries.ExerciseTag `json:"tags"`
}

func (h *Handler) GetExercisesByTag(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := h.Q.GetExerciseTag(r.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "exercise tag not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	rows, err := h.Q.ListExercisesByTag(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	tagsByExercise, err := h.exerciseTagMap(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]exerciseByTagResponse, 0, len(rows))
	for _, row := range rows {
		tags := tagsByExercise[row.ID]
		if tags == nil {
			tags = []queries.ExerciseTag{}
		}
		out = append(out, exerciseByTagResponse{Exercise: row, Tags: tags})
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) exerciseTagMap(r *http.Request) (map[int64][]queries.ExerciseTag, error) {
	all, err := h.Q.ListAllExerciseTagLinks(r.Context())
	if err != nil {
		return nil, err
	}
	m := make(map[int64][]queries.ExerciseTag)
	for _, t := range all {
		m[t.ExerciseID] = append(m[t.ExerciseID], queries.ExerciseTag{ID: t.ID, Name: t.Name, CreatedAt: t.CreatedAt})
	}
	return m, nil
}
