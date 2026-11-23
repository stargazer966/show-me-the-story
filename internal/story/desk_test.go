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
	answer := `{"lines":["First expanded","Second expanded"]}`
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
	if preview.User != promptSent || !strings.Contains(promptSent, "Keep it quiet") || state.Chapters[0].Outline != "Legacy outline" {
		t.Fatal("preview or isolation failed")
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
	must(DeskRequest{Action: "use", Version: 3})
	restored, err := LoadProgress(path)
	if err != nil {
		t.Fatal(err)
	}
	ch := restored.Chapters[0]
	if restored.Desk.Parts[0].DeskDraft != draft || ch.Content != "Already written." || ch.Outline != "First expanded\n\nSecond revised" || !ch.DeskEarlierProse {
		t.Fatal(ch)
	}
	before, _ := json.Marshal(state)
	_, err = RunDesk(context.Background(), path, state, cfg, nil, nil, nil, DeskRequest{Action: "save", Revision: 0, Part: 1})
	after, _ := json.Marshal(state)
	if !errors.Is(err, ErrDeskChanged) || string(after) != string(before) {
		t.Fatal("stale edit affected the desk")
	}
}
