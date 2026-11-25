package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"showmethestory/internal/config"
	"showmethestory/internal/story"
)

func TestSkillOptimizationPreservesEnglishPackage(t *testing.T) {
	responses := []string{
		`{"wrong_key":"missing Markdown"}`,
		`{"content":"Keep the 24-hour limit. See [rules](refs/rules.md)."}`,
		`{"status":"passed","summary":"Ready."}`,
	}
	var requests atomic.Int32
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i := int(requests.Add(1)) - 1
		if i >= len(responses) {
			t.Error("unexpected extra model request")
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		var request struct {
			Messages []struct{ Content string }
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.Messages) != 2 {
			t.Errorf("unexpected model request: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if !strings.Contains(request.Messages[1].Content, "24 hours") {
			t.Error("reference was not supplied")
		}
		if i == 2 && !strings.Contains(request.Messages[1].Content, "applies_to=[outline.generate]") {
			t.Error("validator did not receive the corrected scope")
		}
		data, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{
			"delta": map[string]string{"content": responses[i]}, "finish_reason": "stop",
		}}})
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", data)
	}))
	defer model.Close()

	dir := t.TempDir()
	id := strings.Repeat("s", 64)
	source, err := story.InstallSkillFiles(dir, map[string][]byte{
		"skill.json":    []byte(fmt.Sprintf(`{"schema_version":1,"id":%q,"name":"Rules","languages":["en"],"applies_to":["chapter.generate"],"entrypoint":"custom.md"}`, id)),
		"custom.md":     []byte("Original guidance"),
		"refs/rules.md": []byte("The maximum is 24 hours."),
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	source.Validation = &story.SkillValidationReport{Status: "needs_optimization", RecommendedAppliesTo: []string{story.SkillScopeOutlineGenerate}}
	optimizedID := story.NextOptimizedSkillID(dir, id)
	api := config.DefaultAPIConfig()
	api.BaseURL, api.Model = model.URL, "test"
	h := &Handlers{progDir: dir, apiCfg: api, cfg: config.DefaultConfigForLang("en")}
	if _, report, err := h.createOptimizedSkill(context.Background(), optimizedID, source); err != nil || report.Status != "passed" {
		t.Fatalf("optimization failed: %v, %+v", err, report)
	}
	if requests.Load() != 3 {
		t.Fatalf("expected a format retry and one validation, got %d requests", requests.Load())
	}
	var optimized, original story.Skill
	for _, s := range story.LoadGlobalSkills(dir) {
		switch s.ID {
		case id:
			original = s
		case optimizedID:
			optimized = s
		}
	}
	if len(optimizedID) > 64 || original.Content != "Original guidance" || optimized.Lang != "en" ||
		optimized.EntryPoint != "custom.md" || !reflect.DeepEqual(optimized.Resources, source.Resources) ||
		!strings.Contains(optimized.ReferenceContent, "24 hours") || validationStatus(optimized) != "passed" {
		t.Fatalf("unexpected optimized package: %+v", optimized)
	}
	if filepath.Base(optimizedID) != optimizedID || story.NextOptimizedSkillID(dir, id) == optimizedID {
		t.Fatal("next optimized ID did not advance")
	}
}
