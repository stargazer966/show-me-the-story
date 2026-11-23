package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"showmethestory/internal/config"
	"showmethestory/internal/sse"
	"showmethestory/internal/story"
	"strings"
	"testing"
)

func TestTemplateStartsIndependentEnglishStory(t *testing.T) {
	h := NewHandlers(config.DefaultAPIConfig(), "", sse.NewLogBroadcaster(), t.TempDir(), "test")
	defer h.logger.Close()
	post := func(handler http.HandlerFunc, body string) {
		t.Helper()
		rec := httptest.NewRecorder()
		handler(rec, httptest.NewRequest("POST", "/", strings.NewReader(body)))
		if rec.Code >= 300 {
			t.Fatalf("%d: %s", rec.Code, rec.Body.String())
		}
	}
	post(h.PostProject, `{"name":"source","language":"en"}`)
	if err := h.switchProject("source"); err != nil {
		t.Fatal(err)
	}
	h.settings.Characters = []story.Character{{ID: "char_1", Name: "Kai"}}
	h.settings.Worldview = []story.WorldviewEntry{{ID: "world_1", Name: "Station"}}
	if err := story.SaveProjectSettings(h.settingsPath, h.settings); err != nil {
		t.Fatal(err)
	}
	h.state.Chapters = []story.ChapterState{{Num: 1, Title: "Old", Content: "Private draft", Status: story.StatusAccepted}}
	if err := story.SaveProgress(h.progressPath, h.state); err != nil {
		t.Fatal(err)
	}
	post(h.saveTemplate, `{"name":"Mystery","source":"source","characters":true,"worldview":false}`)
	for _, name := range []string{"First", "Second"} {
		post(h.PostProject, fmt.Sprintf(`{"name":%q,"template":"Mystery","premise":"A letter arrives."}`, name))
		if err := h.switchProject(name); err != nil {
			t.Fatal(err)
		}
		if h.cfg.Language != "en" || !h.cfg.SkillConfig.EnabledSkills["english-word-count-control"] || len(h.state.Chapters) != 0 || h.state.CorePrompt != "A letter arrives." || len(h.settings.Characters) != 1 || h.settings.Characters[0].Name != "Kai" || len(h.settings.Worldview) != 0 {
			t.Fatal("unexpected template result")
		}
		h.settings.Characters[0].Name = "Edited"
		if err := story.SaveProjectSettings(h.settingsPath, h.settings); err != nil {
			t.Fatal(err)
		}
	}
	source, err := story.LoadProjectSettings(filepath.Join(h.storysDir(), "source", "settings.json"))
	if err != nil || source.Characters[0].Name != "Kai" {
		t.Fatal(source, err)
	}
}

func TestVisionDraftUsesDedicatedModelAndLeavesRecordsUnsaved(t *testing.T) {
	var seen bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model    string
			Messages []struct {
				Content []struct {
					Text     string
					ImageURL struct{ URL string } `json:"image_url"`
				}
			}
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		if r.Header.Get("Authorization") != "Bearer vision-test" || body.Model != "vision-test-model" || len(body.Messages) != 1 || len(body.Messages[0].Content) != 2 || !strings.Contains(body.Messages[0].Content[0].Text, "bottom-right") || !strings.HasPrefix(body.Messages[0].Content[1].ImageURL.URL, "data:image/png;base64,") {
			t.Error("wrong vision request")
			return
		}
		seen = true
		fmt.Fprint(w, `{"choices":[{"finish_reason":"stop","message":{"content":"{\"appearance\":\"Blue coat\",\"location_name\":\"Station\",\"scenery\":\"Iron roof\"}"}}]}`)
	}))
	defer server.Close()
	cfg := config.DefaultAPIConfig()
	cfg.APIKey = "writing-test"
	cfg.VisionURL = server.URL
	cfg.VisionModel = "vision-test-model"
	cfg.VisionKey = "vision-test"
	h := NewHandlers(cfg, "", sse.NewLogBroadcaster(), t.TempDir(), "test")
	defer h.logger.Close()
	h.projectName = "book"
	h.cfg = config.DefaultConfigForLang("en")
	var form bytes.Buffer
	mp := multipart.NewWriter(&form)
	file, _ := mp.CreateFormFile("image", "sheet.png")
	file.Write([]byte("\x89PNG\r\n\x1a\n"))
	mp.WriteField("panel", "bottom-right")
	mp.Close()
	req := httptest.NewRequest("POST", "/api/image-draft", &form)
	req.Header.Set("Content-Type", mp.FormDataContentType())
	res := httptest.NewRecorder()
	h.imageDraft(res, req)
	if res.Code != 200 || !seen || !strings.Contains(res.Body.String(), "Iron roof") || len(h.settings.Characters)+len(h.settings.Worldview) != 0 {
		t.Fatal(res.Code, res.Body.String())
	}
}
