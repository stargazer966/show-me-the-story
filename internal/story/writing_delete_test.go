package story

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveDeleteChapterTarget(t *testing.T) {
	ch := func(num int, status, content string) ChapterState {
		return ChapterState{Num: num, Status: status, Content: content}
	}

	tests := []struct {
		name    string
		state   Progress
		wantIdx int
		wantErr error
	}{
		{
			name:    "empty",
			state:   Progress{},
			wantErr: ErrNoChaptersToDelete,
		},
		{
			name: "review at frontier",
			state: Progress{
				Phase:               "writing",
				CurrentChapterIndex: 2,
				Chapters: []ChapterState{
					ch(1, StatusAccepted, "a"),
					ch(2, StatusAccepted, "b"),
					ch(3, StatusReview, "c"),
					ch(4, StatusPending, ""),
				},
			},
			wantIdx: 2,
		},
		{
			name: "accepted frontier when next pending",
			state: Progress{
				Phase:               "writing",
				CurrentChapterIndex: 3,
				Chapters: []ChapterState{
					ch(1, StatusAccepted, "a"),
					ch(2, StatusAccepted, "b"),
					ch(3, StatusAccepted, "c"),
					ch(4, StatusPending, ""),
				},
			},
			wantIdx: 2,
		},
		{
			name: "book complete deletes last",
			state: Progress{
				Phase:               "writing",
				CurrentChapterIndex: 3,
				Chapters: []ChapterState{
					ch(1, StatusAccepted, "a"),
					ch(2, StatusAccepted, "b"),
					ch(3, StatusAccepted, "c"),
				},
			},
			wantIdx: 2,
		},
		{
			name: "writing blocks delete",
			state: Progress{
				Phase:               "writing",
				CurrentChapterIndex: 2,
				Chapters: []ChapterState{
					ch(1, StatusAccepted, "a"),
					ch(2, StatusAccepted, "b"),
					ch(3, StatusWriting, "c"),
				},
			},
			wantErr: ErrWritingChapterCannotDelete,
		},
		{
			name: "nothing written yet",
			state: Progress{
				Phase:               "writing",
				CurrentChapterIndex: 0,
				Chapters: []ChapterState{
					ch(1, StatusPending, ""),
					ch(2, StatusPending, ""),
				},
			},
			wantErr: ErrDeleteFrontierUnavailable,
		},
		{
			name: "review at frontier not earlier accepted",
			state: Progress{
				Phase:               "writing",
				CurrentChapterIndex: 2,
				Chapters: []ChapterState{
					ch(1, StatusAccepted, "a"),
					ch(2, StatusAccepted, "b"),
					ch(3, StatusReview, "c"),
				},
			},
			wantIdx: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idx, err := ResolveDeleteChapterTarget(&tt.state)
			if tt.wantErr != nil {
				if err != tt.wantErr {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if idx != tt.wantIdx {
				t.Fatalf("idx = %d, want %d", idx, tt.wantIdx)
			}
		})
	}
}

func TestDeleteFrontierChapterAdjustsPointer(t *testing.T) {
	state := &Progress{
		Phase:               "writing",
		CurrentChapterIndex: 3,
		Chapters: []ChapterState{
			{Num: 1, Status: StatusAccepted, Content: "a"},
			{Num: 2, Status: StatusAccepted, Content: "b"},
			{Num: 3, Status: StatusAccepted, Content: "c", Summary: "s"},
			{Num: 4, Status: StatusPending},
		},
		MemoryEntries: []MemoryEntry{{ID: 1, Content: "detail", References: []MemoryReference{{Chapter: 3}}}},
	}

	num, err := DeleteFrontierChapter(state, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if num != 3 {
		t.Fatalf("deleted num = %d, want 3", num)
	}
	if state.CurrentChapterIndex != 2 {
		t.Fatalf("pointer = %d, want 2", state.CurrentChapterIndex)
	}
	if state.Chapters[2].Status != StatusPending || state.Chapters[2].Content != "" {
		t.Fatalf("chapter 3 not cleared: %+v", state.Chapters[2])
	}
	if len(state.MemoryEntries) != 0 {
		t.Fatalf("memory not purged: %+v", state.MemoryEntries)
	}
}

func TestRemoveMiddleChapterRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.json")
	p := &Progress{Phase: "writing", Chapters: []ChapterState{
		{Num: 1, Title: "One", Status: StatusAccepted, Content: "One."},
		{Num: 2, Title: "Two", Status: StatusAccepted, Content: "Repeated."},
		{Num: 3, Title: "Three", Status: StatusReview, Content: "Repeated.", Blocks: []Block{{ID: 7, Text: "Repeated.", Type: "paragraph"}}, NextBlockID: 8},
	}, MemoryEntries: []MemoryEntry{{ID: 4, References: []MemoryReference{{Chapter: 2}, {Chapter: 3, BlockID: 7}}}}, OutlineBatches: []OutlineBatch{{ID: 1, StartCh: 1, EndCh: 3}}, Foreshadows: []Foreshadow{{PlantChapter: 1, TargetChapter: 3, Events: []ForeshadowEvent{{Chapter: 2}, {Chapter: 3}}}}}
	if err := SaveProgress(path, p); err != nil {
		t.Fatal(err)
	}
	for _, ch := range p.Chapters {
		SaveChapterMarkdown(dir, ch, "")
	}
	s := &ProjectSettings{StoryChanges: []SettingChange{{ID: 4, Source: MemoryReference{Chapter: 3}}}, StorySynced: map[int]string{2: "gone", 3: "keep"}}
	pp := NewProofreadState()
	pp.Revisions = []ProofreadRevision{{ChapterNum: 3}}
	pp.Issues = []ProofreadIssue{{Anchors: []ProofreadAnchor{{ChapterNum: 2}, {ChapterNum: 3, BlockID: 7}}}}
	if err := RemoveChapter(path, p, s, pp, 1, false); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadProgress(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Chapters) != 2 || loaded.CurrentChapterIndex != 1 || loaded.Chapters[1].Content != "Repeated." || loaded.Chapters[1].Title != "Three" || loaded.Chapters[1].Blocks[0].ID != 7 {
		t.Fatalf("chapter identity lost: %+v", loaded)
	}
	refs := loaded.MemoryEntries[0].References
	if len(refs) != 1 || refs[0].Chapter != 2 || refs[0].BlockID != 7 || loaded.OutlineBatches[0].EndCh != 2 || loaded.Foreshadows[0].Events[0].Chapter != 2 {
		t.Fatalf("wrong references: %+v", loaded)
	}
	stored, err := LoadProjectSettings(filepath.Join(dir, "settings.json"))
	if err != nil || stored.StorySynced[2] != "keep" || stored.StoryChanges[0].Source.Chapter != 2 {
		t.Fatal(stored, err)
	}
	report, err := LoadPostProcess(filepath.Join(dir, "postprocess.json"))
	if err != nil || report.Revisions[0].ChapterNum != 2 || len(report.Issues[0].Anchors) != 1 || report.Issues[0].Anchors[0].BlockID != 7 {
		t.Fatal(report, err)
	}
	md, err := os.ReadFile(ChapterMarkdownPath(dir, 2))
	if err != nil || !strings.Contains(string(md), "Three") {
		t.Fatal(string(md), err)
	}
	if _, err := os.Stat(ChapterMarkdownPath(dir, 3)); !os.IsNotExist(err) {
		t.Fatal("orphan Markdown")
	}
}
