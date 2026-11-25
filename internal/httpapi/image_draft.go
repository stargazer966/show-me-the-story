package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"showmethestory/internal/llm"
	"strings"
)

func (h *Handlers) imageDraft(w http.ResponseWriter, r *http.Request) {
	if !h.ensureProject(w, r) || h.rejectIfTaskRunning(w, r) {
		return
	}
	const limit = 8 << 20
	r.Body = http.MaxBytesReader(w, r.Body, limit+(64<<10))
	file, _, err := r.FormFile("image")
	if err != nil {
		h.writeErrorReq(w, r, 400, "invalid_request", err.Error())
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || len(data) > limit {
		h.writeErrorReq(w, r, 400, "invalid_request", "Image exceeds 8 MiB")
		return
	}
	mime := http.DetectContentType(data)
	if mime != "image/png" && mime != "image/jpeg" && mime != "image/webp" {
		h.writeErrorReq(w, r, 400, "invalid_request", "Choose a PNG, JPEG or WebP image")
		return
	}
	cfg := *h.apiCfg
	if cfg.VisionURL != "" {
		cfg.BaseURL = cfg.VisionURL
		cfg.APIKey = cfg.VisionKey
		cfg.URLStrict = false
	} else if cfg.VisionKey != "" {
		cfg.APIKey = cfg.VisionKey
	}
	if cfg.VisionModel != "" {
		cfg.Model = cfg.VisionModel
	}
	if cfg.Model == "" || cfg.BaseURL == "" {
		h.writeErrorReq(w, r, 400, "invalid_request", "Configure a vision model and endpoint")
		return
	}
	cfg.MaxTokens = 1500
	prompt := "Describe the visible character and setting in English. Multiple portraits/front/back views of the same subject represent one character. Distinguish scenery from a studio backdrop or transparency grid. Suggest a fictional name for the character in character_name. The name is a creative suggestion and must never claim or infer the identity of the person shown. Return a JSON object with string fields character_name, appearance, location_name and scenery. Describe only visible details. Use empty strings for character_name and appearance if there is no character."
	if h.cfg.Language == "zh" {
		prompt += " Write the field values in Chinese."
	}
	switch cell := r.FormValue("panel"); cell {
	case "top-left", "top-right", "bottom-left", "bottom-right":
		prompt += " Use only the " + cell + " panel for scenery."
	}
	if !h.tryStartTask() {
		h.writeErrorReq(w, r, 409, "task_running_wait")
		return
	}
	defer h.endTask()
	ctx, cancel := context.WithCancel(h.taskCtx)
	stop := context.AfterFunc(r.Context(), cancel)
	defer stop()
	defer cancel()
	res, err := llm.CallAPIMessagesSync(ctx, &cfg, []llm.Message{{Role: "user", Content: prompt, ImageURL: "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)}})
	if err != nil {
		h.writeErrorReq(w, r, 502, "invalid_request", "Vision request failed. Check the endpoint, model, API key and image support.")
		return
	}
	var out struct {
		Name       string `json:"character_name"`
		Appearance string `json:"appearance"`
		Location   string `json:"location_name"`
		Scenery    string `json:"scenery"`
	}
	if res.FinishReason == "length" || json.Unmarshal([]byte(llm.ExtractJSON(res.Content)), &out) != nil || strings.TrimSpace(out.Appearance+out.Scenery) == "" {
		h.writeErrorReq(w, r, 502, "invalid_request", "The model returned an incomplete image description")
		return
	}
	out.Name = strings.TrimSpace(out.Name)
	if strings.TrimSpace(out.Appearance) == "" {
		out.Name = ""
	}
	h.writeJSON(w, 200, out)
}
