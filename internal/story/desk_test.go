package story

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"showmethestory/internal/config"
	"showmethestory/internal/sse"
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
	draft := DeskDraft{Title: "New", Beats: "Enter\nLeave", Focus: "Keep it quiet"}
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
	if ch.Title != "New" || !sameDeskDraft(restored.Desk.Parts[0].DeskDraft, draft) || ch.Content != "Already written." || ch.Outline != "First expanded\n\nSecond revised" || !ch.DeskEarlierProse {
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

func TestDeskBeatGuidanceFlowsToWriting(t *testing.T) {
	cfg := config.DefaultConfigForLang("en")
	p := &Progress{Desk: StoryDesk{Parts: []DeskPart{{DeskDraft: DeskDraft{Beats: "Arrive\nDiscover the letter", Focus: "Keep the ending quiet", Remarks: []string{"Brief transition", "Third person through Mara, 40% of chapter"}}}}}}
	path := filepath.Join(t.TempDir(), "progress.json")
	model := batchAPI(t, func(prompt string) string {
		if !strings.Contains(prompt, "Third person through Mara") {
			t.Error("outline request lost beat remark")
		}
		return `{"lines":["Mara arrives.","Mara finds the letter."],"instructions":["Use a brief transition.","Third person through Mara, 40% of chapter."]}`
	})
	run := func(r DeskRequest) {
		t.Helper()
		r.Part = 1
		r.Revision = p.Desk.Revision
		if _, err := RunDesk(context.Background(), path, p, cfg, nil, nil, model, r); err != nil {
			t.Fatal(err)
		}
	}
	run(DeskRequest{Action: "generate"})
	run(DeskRequest{Action: "manual_outline", Version: 1, Lines: []string{"Mara arrives.", "Mara finds the letter."}, Instructions: []string{"Use a brief transition.", "Third person through Mara, 45% of chapter."}})
	ch := p.Chapters[0]
	if ch.DeskVersion != 2 || !strings.Contains(deskWritingGuidance(p, ch, 2, "en"), "45%") || !strings.Contains(deskWritingGuidance(p, ch, 1, "en"), "40%") {
		t.Fatal("guidance was not versioned")
	}
	shown := ProgressView(p).Chapters[0].WritingGuidance
	if shown == nil || len(shown.Beats) != 2 ||
		!strings.Contains(shown.Beats[1].Instruction, "45%") ||
		!strings.Contains(shown.ChapterRemarks, "Keep the ending quiet") {
		t.Fatal("progress must show the guidance from the adopted outline")
	}
	model = batchAPI(t, func(prompt string) string {
		for _, want := range []string{"Prose guidance from the outline version", "45%", "Keep the ending quiet", "Brief transition"} {
			if !strings.Contains(prompt, want) {
				t.Errorf("writing request missing %q", want)
			}
		}
		return "Mara arrived and found the letter."
	})
	if _, err := generateChapterContentStream(context.Background(), model, cfg, p, 0, &ProjectSettings{}, "", sse.NewLogBroadcaster()); err != nil {
		t.Fatal(err)
	}
}

func TestDeskDeleteAndRestorePreserveDraftAndWriting(t *testing.T) {
	draft := DeskDraft{Title: "Station", Beats: "Find the letter", Focus: "Build tension"}
	version := DeskVersion{Source: draft, Lines: []string{"Mara finds the letter."}}
	chapter := ChapterState{Num: 1, Title: draft.Title, DeskPart: 1, DeskVersion: 1, Outline: version.Lines[0], Content: "Mara saw her name.", Status: StatusAccepted}
	state := &Progress{
		Phase: "writing",
		Desk: StoryDesk{Parts: []DeskPart{
			{DeskDraft: draft, Versions: []DeskVersion{version}},
			{DeskDraft: DeskDraft{Title: "Home"}},
		}},
		Chapters: []ChapterState{chapter},
	}
	cfg := config.DefaultConfigForLang("en")
	path := filepath.Join(t.TempDir(), "progress.json")
	run := func(action string) error {
		_, err := RunDesk(context.Background(), path, state, cfg, nil, nil, nil, DeskRequest{Action: action, Part: 1, Revision: state.Desk.Revision})
		return err
	}
	if err := run("delete"); err != nil {
		t.Fatal(err)
	}
	if err := run("save"); err == nil {
		t.Fatal("a deleted part accepted a draft edit")
	}
	if err := run("open"); err != nil {
		t.Fatal(err)
	}
	persisted, err := LoadProgress(path)
	if err != nil {
		t.Fatal(err)
	}
	state = persisted
	if !state.Desk.Parts[0].Deleted || state.Desk.Parts[1].Title != "Home" || state.Chapters[0].Content != chapter.Content || state.Chapters[0].DeskPart != 1 {
		t.Fatal("deletion changed the linked Writing chapter or another part")
	}
	if err := run("restore"); err != nil {
		t.Fatal(err)
	}
	part := state.Desk.Parts[0]
	if part.Deleted || !sameDeskDraft(part.DeskDraft, draft) || !reflect.DeepEqual(part.Versions, []DeskVersion{version}) {
		t.Fatal("restore did not recover the draft and outline")
	}
}
