package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"health/db/queries"
)

func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.Q.ListUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if users == nil {
		users = []queries.User{}
	}
	writeJSON(w, http.StatusOK, users)
}

type createUserBody struct {
	Name           string `json:"name"`
	Avatar         string `json:"avatar"`
	TargetCalories int64  `json:"target_calories"`
}

func (h *Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var body createUserBody
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}
	if body.TargetCalories <= 0 {
		writeError(w, http.StatusBadRequest, "target_calories must be > 0")
		return
	}
	avatar := strings.TrimSpace(body.Avatar)
	if avatar == "" {
		avatar = strings.ToUpper(name)
		if len(avatar) > 2 {
			avatar = avatar[:2]
		}
	}
	u, err := h.Q.CreateUser(r.Context(), queries.CreateUserParams{
		Name:           name,
		Avatar:         avatar,
		TargetCalories: body.TargetCalories,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, u)
}

type updateUserBody struct {
	Name           *string `json:"name"`
	TargetCalories *int64  `json:"target_calories"`
}

func (h *Handler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	current, err := h.Q.GetUser(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var body updateUserBody
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	name := current.Name
	target := current.TargetCalories
	if body.Name != nil {
		trimmed := strings.TrimSpace(*body.Name)
		if trimmed == "" {
			writeError(w, http.StatusBadRequest, "name must be non-empty")
			return
		}
		name = trimmed
	}
	if body.TargetCalories != nil {
		if *body.TargetCalories <= 0 {
			writeError(w, http.StatusBadRequest, "target_calories must be > 0")
			return
		}
		target = *body.TargetCalories
	}
	u, err := h.Q.UpdateUser(r.Context(), queries.UpdateUserParams{
		ID:             id,
		Name:           name,
		TargetCalories: target,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (h *Handler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.Q.DeleteUser(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
