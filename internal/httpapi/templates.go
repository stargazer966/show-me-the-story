package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"showmethestory/internal/config"
	"showmethestory/internal/fsutil"
	"showmethestory/internal/story"
	"strings"
)

type storyTemplate struct {
	Name     string                 `json:"name"`
	Config   *config.Config         `json:"config"`
	Settings *story.ProjectSettings `json:"settings"`
}

func (h *Handlers) templateDir() string { return filepath.Join(h.progDir, "templates") }
func (h *Handlers) readTemplate(name string) (storyTemplate, error) {
	var t storyTemplate
	if !validProjectName(name) {
		return t, fmt.Errorf("invalid template name")
	}
	data, err := os.ReadFile(filepath.Join(h.templateDir(), name+".json"))
	if err != nil {
		return t, err
	}
	err = json.Unmarshal(data, &t)
	if err == nil && (t.Config == nil || t.Settings == nil || t.Config.ProjectFormatVersion != config.ProjectFormatVersion) {
		err = fmt.Errorf("unsupported template")
	}
	return t, err
}
func (h *Handlers) listTemplates(w http.ResponseWriter, r *http.Request) {
	entries, err := os.ReadDir(h.templateDir())
	if err != nil && !os.IsNotExist(err) {
		h.writeErrorReq(w, r, 500, "invalid_request", err.Error())
		return
	}
	out := []storyTemplate{}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			t, err := h.readTemplate(strings.TrimSuffix(e.Name(), ".json"))
			if err != nil {
				h.writeErrorReq(w, r, 500, "invalid_request", err.Error())
				return
			}
			out = append(out, t)
		}
	}
	h.writeJSON(w, 200, out)
}
func (h *Handlers) saveTemplate(w http.ResponseWriter, r *http.Request) {
	if h.rejectIfTaskRunning(w, r) {
		return
	}
	var req struct {
		Name, Source          string
		Characters, Worldview bool
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || !validProjectName(req.Name) || !validProjectName(req.Source) {
		h.writeErrorReq(w, r, 400, "project_name_invalid_chars")
		return
	}
	source := filepath.Join(h.storysDir(), req.Source)
	if err := ensureProjectCompatible(source); err != nil {
		h.writeErrorReq(w, r, 400, "invalid_request", err.Error())
		return
	}
	cfg, err := config.LoadConfig(filepath.Join(source, "config.json"))
	if err != nil {
		h.writeErrorReq(w, r, 400, "invalid_request", err.Error())
		return
	}
	settings, err := story.LoadProjectSettings(filepath.Join(source, "settings.json"))
	if err != nil {
		h.writeErrorReq(w, r, 400, "invalid_request", err.Error())
		return
	}
	selected := story.ProjectSettings{}
	if req.Characters {
		selected.Characters = settings.Characters
	}
	if req.Worldview {
		selected.Worldview = settings.Worldview
	}
	cfg.Story.Title = ""
	path := filepath.Join(h.templateDir(), req.Name+".json")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		h.writeErrorReq(w, r, 409, "invalid_request", "Template name already exists")
		return
	}
	data, err := json.MarshalIndent(storyTemplate{req.Name, cfg, &selected}, "", "  ")
	if err == nil {
		err = os.MkdirAll(h.templateDir(), 0755)
	}
	if err == nil {
		err = fsutil.WriteFileAtomic(path, data)
	}
	if err != nil {
		h.writeErrorReq(w, r, 500, "init_project_config_failed", err)
		return
	}
	h.writeJSON(w, 201, map[string]string{"name": req.Name})
}
