package httpapi

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"showmethestory/internal/config"
	"showmethestory/internal/sse"
	"showmethestory/internal/story"
)

func TestSkillScopeOverrideControlsActivation(t *testing.T) {
	h := NewHandlers(config.DefaultAPIConfig(), "", sse.NewLogBroadcaster(), t.TempDir(), "test")
	defer h.logger.Close()
	project := httptest.NewRecorder()
	h.PostProject(project, httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"test","language":"en"}`)))
	if project.Code >= 300 {
		t.Fatal(project.Body.String())
	}
	if err := h.switchProject("test"); err != nil {
		t.Fatal(err)
	}
	const id = "english-word-count-control"
	hasScope := func(scope string) bool {
		return slices.ContainsFunc(story.ResolveSkills(h.skills, h.cfg.SkillConfig, scope, "en"), func(s story.Skill) bool { return s.ID == id })
	}
	put := func(body string, code int) {
		t.Helper()
		req := httptest.NewRequest("PUT", "/", strings.NewReader(body))
		req.SetPathValue("id", id)
		rec := httptest.NewRecorder()
		h.PutSkillScopes(rec, req)
		if rec.Code != code {
			t.Fatalf("got %d, want %d: %s", rec.Code, code, rec.Body.String())
		}
	}
	put(`{"scopes":[]}`, http.StatusBadRequest)
	put(`{"scopes":["unknown"]}`, http.StatusBadRequest)
	put(`{"scopes":["outline.generate","outline.generate"]}`, http.StatusOK)
	cfg, err := config.LoadConfig(h.cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	h.cfg = cfg
	if !hasScope(story.SkillScopeOutlineGenerate) || hasScope(story.SkillScopeChapterGenerate) ||
		!slices.Equal(cfg.SkillConfig.ScopeOverrides[id], []string{story.SkillScopeOutlineGenerate}) {
		t.Fatal("saved override did not control activation")
	}
	put(`{"reset":true}`, http.StatusOK)
	if !hasScope(story.SkillScopeChapterGenerate) || hasScope(story.SkillScopeOutlineGenerate) {
		t.Fatal("reset did not restore the default scopes")
	}
}
