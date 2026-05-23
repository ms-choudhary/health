package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

const geminiURL = "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent"

type calorieHintRequest struct {
	Name string `json:"name"`
}

type calorieHintResponse struct {
	Hint string `json:"hint"`
}

func (h *Handler) CalorieHint(w http.ResponseWriter, r *http.Request) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "AI not configured"})
		return
	}

	var req calorieHintRequest
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.Name) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}

	prompt := fmt.Sprintf("ANSWER IN ONE LINE: how much calories and protein (g) in %s ?", req.Name)

	body, _ := json.Marshal(map[string]any{
		"contents": []map[string]any{
			{"parts": []map[string]any{{"text": prompt}}},
		},
	})

	httpReq, err := http.NewRequestWithContext(
		r.Context(), http.MethodPost, geminiURL+"?key="+apiKey, bytes.NewReader(body),
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to build request"})
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to reach Gemini"})
		return
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)

	var geminiResp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}

	if err := json.Unmarshal(raw, &geminiResp); err != nil ||
		len(geminiResp.Candidates) == 0 ||
		len(geminiResp.Candidates[0].Content.Parts) == 0 {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "unexpected Gemini response"})
		return
	}

	hint := strings.TrimSpace(geminiResp.Candidates[0].Content.Parts[0].Text)
	writeJSON(w, http.StatusOK, calorieHintResponse{Hint: hint})
}
