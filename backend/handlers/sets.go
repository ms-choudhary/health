package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"health/db/queries"
)

type setInput struct {
	Weight float64 `json:"weight"`
	Reps   int64   `json:"reps"`
}

type addSetsBody struct {
	ExerciseID int64      `json:"exercise_id"`
	Date       string     `json:"date"`
	Unit       string     `json:"unit"`
	Sets       []setInput `json:"sets"`
}

func (h *Handler) GetSets(w http.ResponseWriter, r *http.Request) {
	userID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	sets, err := h.Q.GetSetHistory(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if sets == nil {
		sets = []queries.ExerciseSet{}
	}
	writeJSON(w, http.StatusOK, sets)
}

func (h *Handler) AddSets(w http.ResponseWriter, r *http.Request) {
	userID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var body addSetsBody
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.ExerciseID <= 0 {
		writeError(w, http.StatusBadRequest, "exercise_id required")
		return
	}
	if !validDate(body.Date) {
		writeError(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}
	if len(body.Sets) == 0 {
		writeError(w, http.StatusBadRequest, "at least one set required")
		return
	}
	for _, s := range body.Sets {
		if s.Weight < 0 || s.Reps < 0 {
			writeError(w, http.StatusBadRequest, "weight/reps must be >= 0")
			return
		}
	}
	ex, err := h.Q.GetExercise(r.Context(), body.ExerciseID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "exercise not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	unit := strings.TrimSpace(body.Unit)
	if unit == "" {
		unit = "kg"
	}

	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer func() { _ = tx.Rollback() }()
	q := h.Q.WithTx(tx)

	out := make([]queries.ExerciseSet, 0, len(body.Sets))
	for _, s := range body.Sets {
		exID := ex.ID
		row, err := q.AddSet(r.Context(), queries.AddSetParams{
			UserID:       userID,
			ExerciseID:   &exID,
			ExerciseName: ex.Name,
			Date:         body.Date,
			Weight:       s.Weight,
			Reps:         s.Reps,
			Unit:         unit,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, row)
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (h *Handler) GetLastSets(w http.ResponseWriter, r *http.Request) {
	userID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	exerciseID, err := parseID(r, "eid")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	rows, err := h.Q.GetLastSetsForExercise(r.Context(), queries.GetLastSetsForExerciseParams{
		UserID:     userID,
		ExerciseID: &exerciseID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]setInput, 0, len(rows))
	for _, s := range rows {
		out = append(out, setInput{Weight: s.Weight, Reps: s.Reps})
	}
	writeJSON(w, http.StatusOK, out)
}

type updateSetBody struct {
	Weight float64 `json:"weight"`
	Reps   int64   `json:"reps"`
}

func (h *Handler) UpdateSet(w http.ResponseWriter, r *http.Request) {
	userID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	setID, err := parseID(r, "sid")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var body updateSetBody
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.Weight < 0 || body.Reps < 0 {
		writeError(w, http.StatusBadRequest, "weight/reps must be >= 0")
		return
	}
	set, err := h.Q.UpdateSet(r.Context(), queries.UpdateSetParams{
		Weight: body.Weight,
		Reps:   body.Reps,
		ID:     setID,
		UserID: userID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "set not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, set)
}

func (h *Handler) DeleteSet(w http.ResponseWriter, r *http.Request) {
	userID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	setID, err := parseID(r, "sid")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.Q.DeleteSet(r.Context(), queries.DeleteSetParams{ID: setID, UserID: userID}); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type progressPoint struct {
	Date        string  `json:"date"`
	TotalVolume float64 `json:"total_volume"`
	Breakdown   string  `json:"breakdown"`
}

type exerciseProgress struct {
	ExerciseID   *int64          `json:"exercise_id"`
	ExerciseName string          `json:"exercise_name"`
	Points       []progressPoint `json:"points"`
}

func trimNum(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func (h *Handler) GetExerciseProgress(w http.ResponseWriter, r *http.Request) {
	userID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	if !validDate(from) || !validDate(to) {
		writeError(w, http.StatusBadRequest, "from/to must be YYYY-MM-DD")
		return
	}
	rows, err := h.Q.GetProgressSets(r.Context(), queries.GetProgressSetsParams{
		UserID:   userID,
		FromDate: from,
		ToDate:   to,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	out := make([]exerciseProgress, 0)
	exIdx := make(map[string]int)
	for _, row := range rows {
		ei, ok := exIdx[row.ExerciseName]
		if !ok {
			exIdx[row.ExerciseName] = len(out)
			out = append(out, exerciseProgress{ExerciseID: row.ExerciseID, ExerciseName: row.ExerciseName})
			ei = exIdx[row.ExerciseName]
		}
		pts := out[ei].Points
		if n := len(pts); n == 0 || pts[n-1].Date != row.Date {
			out[ei].Points = append(pts, progressPoint{Date: row.Date})
		}
		p := &out[ei].Points[len(out[ei].Points)-1]
		p.TotalVolume += row.Weight * float64(row.Reps)
		piece := trimNum(row.Weight) + "x" + strconv.FormatInt(row.Reps, 10)
		if p.Breakdown == "" {
			p.Breakdown = piece
		} else {
			p.Breakdown += " " + piece
		}
	}
	writeJSON(w, http.StatusOK, out)
}
