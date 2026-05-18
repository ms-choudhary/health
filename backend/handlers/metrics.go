package handlers

import (
	"net/http"

	"health/db/queries"
)

type metricsResponse struct {
	ID               int64    `json:"id"`
	UserID           int64    `json:"user_id"`
	Date             string   `json:"date"`
	Weight           *float64 `json:"weight"`
	Steps            *int64   `json:"steps"`
	CaloriesConsumed float64  `json:"calories_consumed"`
}

func (h *Handler) GetMetrics(w http.ResponseWriter, r *http.Request) {
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
	metrics, err := h.Q.GetMetricsRange(r.Context(), queries.GetMetricsRangeParams{
		UserID:   userID,
		FromDate: from,
		ToDate:   to,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	calRows, err := h.Q.SumCaloriesByDateRange(r.Context(), queries.SumCaloriesByDateRangeParams{
		UserID:   userID,
		FromDate: from,
		ToDate:   to,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	calByDate := make(map[string]float64, len(calRows))
	for _, c := range calRows {
		calByDate[c.Date] = c.TotalCalories
	}
	out := make([]metricsResponse, 0, len(metrics))
	for _, m := range metrics {
		out = append(out, metricsResponse{
			ID:               m.ID,
			UserID:           m.UserID,
			Date:             m.Date,
			Weight:           m.Weight,
			Steps:            m.Steps,
			CaloriesConsumed: calByDate[m.Date],
		})
	}
	writeJSON(w, http.StatusOK, out)
}

type upsertMetricsBody struct {
	Date   string   `json:"date"`
	Weight *float64 `json:"weight"`
	Steps  *int64   `json:"steps"`
}

func (h *Handler) UpsertMetrics(w http.ResponseWriter, r *http.Request) {
	userID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var body upsertMetricsBody
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !validDate(body.Date) {
		writeError(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}
	if body.Weight != nil && *body.Weight < 0 {
		writeError(w, http.StatusBadRequest, "weight must be >= 0")
		return
	}
	if body.Steps != nil && *body.Steps < 0 {
		writeError(w, http.StatusBadRequest, "steps must be >= 0")
		return
	}
	m, err := h.Q.UpsertMetrics(r.Context(), queries.UpsertMetricsParams{
		UserID: userID,
		Date:   body.Date,
		Weight: body.Weight,
		Steps:  body.Steps,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (h *Handler) GetTodaySummary(w http.ResponseWriter, r *http.Request) {
	userID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	row, err := h.Q.GetTodaySummary(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]float64{
		"consumed": row.Consumed,
		"target":   float64(row.Target),
	})
}
