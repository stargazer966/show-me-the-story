package story

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"showmethestory/internal/config"
	"strings"
	"testing"
)

func TestDeskCandidatesAndExistingProse(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Language = "en"
	state := &Progress{Title: "Novel", Phase: "writing", Chapters: []ChapterState{{Num: 1, Title: "Old", Outline: "Legacy outline", Status: StatusAccepted, Content: "Already written."}}}
	path := filepath.Join(t.TempDir(), "progress.json")
	answer := `{"title":"Suggested name","lines":["First expanded","Second expanded"]}`
	var promptSent string
	model := batchAPI(t, func(user string) string { promptSent = user; return answer })
	run := func(r DeskRequest) (any, error) {
		r.Part = 1
		r.Revision = state.Desk.Revision
		return RunDesk(context.Background(), path, state, cfg, &ProjectSettings{}, nil, model, r)
	}
	must := func(r DeskRequest) any {
		v, e := run(r)
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	must(DeskRequest{Action: "open"})
	draft := DeskDraft{"New", "Enter\nLeave", "Keep it quiet"}
	must(DeskRequest{Action: "save", Draft: draft})
	preview := must(DeskRequest{Action: "preview"}).(DeskPrompt)
	must(DeskRequest{Action: "generate"})
	if preview.User != promptSent || !strings.Contains(preview.System, "50–100 English words per beat") || !strings.Contains(promptSent, "Keep it quiet") || state.Chapters[0].Outline != "First expanded\n\nSecond expanded" || state.Chapters[0].DeskVersion != 2 || state.Desk.Parts[0].Versions[0].Lines[0] != "Legacy outline" {
		t.Fatal("preview, automatic adoption or history failed")
	}
	answer = `{"lines":["Second revised"]}`
	must(DeskRequest{Action: "generate", Version: 2, Beat: 2, Comment: "Slower"})
	part := state.Desk.Parts[0]
	if !reflect.DeepEqual(part.Versions[2].Lines, []string{"First expanded", "Second revised"}) || part.Versions[1].Lines[1] != "Second expanded" {
		t.Fatal(part)
	}
	if _, err := run(DeskRequest{Action: "generate"}); err == nil || len(state.Desk.Parts[0].Versions) != 3 {
		t.Fatal("partial candidate was accepted")
	}
	must(DeskRequest{Action: "use", Version: 1})
	if state.Chapters[0].Outline != "Legacy outline" {
		t.Fatal("existing version cannot be restored")
	}
	must(DeskRequest{Action: "use", Version: 3})
	restored, err := LoadProgress(path)
	if err != nil {
		t.Fatal(err)
	}
	ch := restored.Chapters[0]
	if ch.Title != "New" || restored.Desk.Parts[0].DeskDraft != draft || ch.Content != "Already written." || ch.Outline != "First expanded\n\nSecond revised" || !ch.DeskEarlierProse {
		t.Fatal(ch)
	}
	before, _ := json.Marshal(state)
	_, err = RunDesk(context.Background(), path, state, cfg, nil, nil, nil, DeskRequest{Action: "save", Revision: 0, Part: 1})
	after, _ := json.Marshal(state)
	if !errors.Is(err, ErrDeskChanged) || string(after) != string(before) {
		t.Fatal("stale edit affected the desk")
	}
}

func TestGeneratedPartTitleAndProgressOnAdoption(t *testing.T) {
	cfg := config.DefaultConfigForLang("en")
	p := &Progress{Phase: "outline"}
	path := filepath.Join(t.TempDir(), "progress.json")
	model := batchAPI(t, func(string) string { return `{"title":"The Last Train","lines":["Mara waits on the platform."]}` })
	run := func(r DeskRequest) any {
		t.Helper()
		r.Part = 1
		r.Revision = p.Desk.Revision
		result, err := RunDesk(context.Background(), path, p, cfg, nil, nil, model, r)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	run(DeskRequest{Action: "open"})
	run(DeskRequest{Action: "save", Draft: DeskDraft{Beats: "Wait"}})
	result := run(DeskRequest{Action: "generate"}).(map[string]any)
	view := result["progress"].(*Progress)
	if view.Phase != "writing" || len(view.Chapters) != 1 || view.Chapters[0].Title != "The Last Train" || view.Chapters[0].Status != StatusPending {
		t.Fatalf("writing did not receive the adopted chapter: %+v", view.Chapters)
	}
}

func TestDeskAdoptsLaterPartWithoutChangingChapterNumbers(t *testing.T) {
	cfg := config.DefaultConfigForLang("en")
	p := &Progress{Phase: "outline", Desk: StoryDesk{Parts: []DeskPart{{DeskDraft: DeskDraft{Beats: "Arrive"}}, {DeskDraft: DeskDraft{Beats: "Leave"}}}}}
	path := filepath.Join(t.TempDir(), "progress.json")
	model := batchAPI(t, func(string) string { return `{"title":"Station","lines":["A character crosses the platform."]}` })
	for _, part := range []int{2, 1} {
		_, err := RunDesk(context.Background(), path, p, cfg, nil, nil, model, DeskRequest{Action: "generate", Part: part, Revision: p.Desk.Revision})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(p.Chapters) != 2 || p.Chapters[0].Num != 1 || p.Chapters[0].DeskPart != 1 || p.Chapters[1].Num != 2 || p.Chapters[1].DeskPart != 2 || p.Chapters[0].DeskVersion != 1 || p.Chapters[1].DeskVersion != 1 {
		t.Fatal(p.Chapters)
	}
}
