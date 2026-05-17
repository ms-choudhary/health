package handlers

import (
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
	Name   string `json:"name"`
	Avatar string `json:"avatar"`
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
	avatar := strings.TrimSpace(body.Avatar)
	if avatar == "" {
		avatar = strings.ToUpper(name)
		if len(avatar) > 2 {
			avatar = avatar[:2]
		}
	}
	u, err := h.Q.CreateUser(r.Context(), queries.CreateUserParams{Name: name, Avatar: avatar})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, u)
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
