package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"health/db/queries"
)

type exerciseBody struct {
	Name           string  `json:"name"`
	Notes          string  `json:"notes"`
	ExerciseTagIDs []int64 `json:"exercise_tag_ids"`
}

type exerciseListResponse struct {
	queries.Exercise
	Tags []queries.ExerciseTag `json:"tags"`
}

type exerciseDetailResponse struct {
	queries.Exercise
	Tags []queries.ExerciseTag `json:"tags"`
}

func validateExerciseBody(body exerciseBody) (string, string, []int64, error) {
	name := strings.TrimSpace(body.Name)
	if name == "" {
		return "", "", nil, errors.New("name required")
	}
	for _, id := range body.ExerciseTagIDs {
		if id <= 0 {
			return "", "", nil, errors.New("exercise_tag_id must be > 0")
		}
	}
	return name, strings.TrimSpace(body.Notes), body.ExerciseTagIDs, nil
}

func (h *Handler) applyExerciseTags(w http.ResponseWriter, r *http.Request, q *queries.Queries, exerciseID int64, tagIDs []int64) bool {
	for _, tid := range tagIDs {
		if _, err := q.GetExerciseTag(r.Context(), tid); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusBadRequest, "exercise tag not found — create it first")
				return false
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return false
		}
		if err := q.AddExerciseTag(r.Context(), queries.AddExerciseTagParams{ExerciseID: exerciseID, ExerciseTagID: tid}); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return false
		}
	}
	return true
}

func (h *Handler) ListExercises(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	rows, err := h.Q.ListExercises(r.Context(), &q)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	tagsByExercise, err := h.exerciseTagMap(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]exerciseListResponse, 0, len(rows))
	for _, row := range rows {
		tags := tagsByExercise[row.ID]
		if tags == nil {
			tags = []queries.ExerciseTag{}
		}
		out = append(out, exerciseListResponse{Exercise: row, Tags: tags})
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) GetExercise(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	exercise, err := h.Q.GetExercise(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "exercise not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	tags, err := h.Q.GetTagsForExercise(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tags == nil {
		tags = []queries.ExerciseTag{}
	}
	writeJSON(w, http.StatusOK, exerciseDetailResponse{Exercise: exercise, Tags: tags})
}

func (h *Handler) CreateExercise(w http.ResponseWriter, r *http.Request) {
	var body exerciseBody
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	name, notes, tagIDs, err := validateExerciseBody(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer func() { _ = tx.Rollback() }()
	q := h.Q.WithTx(tx)
	exercise, err := q.CreateExercise(r.Context(), queries.CreateExerciseParams{Name: name, Notes: notes})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !h.applyExerciseTags(w, r, q, exercise.ID, tagIDs) {
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, exercise)
}

func (h *Handler) UpdateExercise(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var body exerciseBody
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	name, notes, tagIDs, err := validateExerciseBody(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := h.Q.GetExercise(r.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "exercise not found")
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
	defer func() { _ = tx.Rollback() }()
	q := h.Q.WithTx(tx)
	exercise, err := q.UpdateExercise(r.Context(), queries.UpdateExerciseParams{ID: id, Name: name, Notes: notes})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := q.ClearExerciseTags(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !h.applyExerciseTags(w, r, q, id, tagIDs) {
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, exercise)
}

func (h *Handler) DeleteExercise(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.Q.DeleteExercise(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
