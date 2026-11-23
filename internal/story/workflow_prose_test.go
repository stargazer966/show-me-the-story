package story

import (
	"context"
	"showmethestory/internal/config"
	"showmethestory/internal/sse"
	"strings"
	"testing"
)

func TestUpdateProseMakesOnlyReturnedBlockChanges(t *testing.T) {
	cfg := config.DefaultConfigForLang("en")
	ch := ChapterState{Num: 1, Title: "Opening", Outline: "Enter the station. Close the door.", Content: "Rain fell outside the station.\n\nMara left the door open.", DeskPart: 1, DeskVersion: 2, DeskProseVersion: 1, DeskEarlierProse: true}
	SyncChapterBlocks(&ch)
	p := &Progress{Chapters: []ChapterState{ch}, Desk: StoryDesk{Parts: []DeskPart{{Versions: []DeskVersion{{Lines: []string{"Enter the station. Leave the door open."}}, {Lines: []string{ch.Outline}}}}}}}
	model := batchAPI(t, func(prompt string) string {
		if strings.Contains(prompt, "old_outline") {
			if !strings.Contains(prompt, "Leave the door open.") {
				t.Error("old outline missing")
			}
			return `{"changes":[{"block_id":2,"text":"Mara closed the door."}]}`
		}
		return "Mara closed the station door."
	})
	updated, err := DraftProseCandidate(context.Background(), model, cfg, p, &ProjectSettings{}, 0, true, sse.NewLogBroadcaster())
	if err != nil {
		t.Fatal(err)
	}
	if updated.Content != "Rain fell outside the station.\n\nMara closed the door." || p.Chapters[0].Blocks[1].Text != "Mara left the door open." {
		t.Fatalf("unexpected candidate: %q", updated.Content)
	}
}

func TestAdoptProofreadRejectsStaleProposal(t *testing.T) {
	ch := ChapterState{Num: 1, Content: "At teh station."}
	SyncChapterBlocks(&ch)
	pp := NewProofreadState()
	pp.Copy = &Progress{Chapters: []ChapterState{ch}}
	pp.CopySettings = &ProjectSettings{}
	base := ChapterRevision(ch)
	pp.Proposed = []ProofreadRevision{{ChapterNum: 1, AfterRev: base, Changes: []ProofreadBlockChange{{BlockID: 1, Before: ch.Content, After: "At the station."}}}}
	if err := AdoptProofread(pp, 1, "stale"); err == nil {
		t.Fatal("accepted stale proposal")
	}
	if err := AdoptProofread(pp, 1, base); err != nil {
		t.Fatal(err)
	}
	if pp.Copy.Chapters[0].Content != "At the station." || len(pp.Proposed) != 0 {
		t.Fatal("adoption did not update copy")
	}
}
