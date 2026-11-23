package story

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

// RemoveChapter publishes the renumbered story and its dependent records together.
func RemoveChapter(path string, state *Progress, settings *ProjectSettings, proof *PostProcessState, idx int, clearOnly bool) error {
	if idx < 0 || idx >= len(state.Chapters) || state.Chapters[idx].Inherited {
		return fmt.Errorf("chapter unavailable")
	}
	if clearOnly {
		if state.Chapters[idx].Content == "" {
			return ErrDeleteFrontierUnavailable
		}
	}
	dir := filepath.Dir(path)
	if LoadImportState(ImportStatePath(dir)) != nil {
		return fmt.Errorf("finish the active import before changing chapters")
	}
	// Work on detached copies: a failed transaction must leave live readers on the old story.
	type documents struct {
		State    *Progress
		Settings *ProjectSettings
		Proof    *PostProcessState
	}
	bytes, err := json.Marshal(documents{state, settings, proof})
	if err != nil {
		return err
	}
	var work documents
	if err = json.Unmarshal(bytes, &work); err != nil {
		return err
	}
	p, s, pp := work.State, work.Settings, work.Proof
	num := p.Chapters[idx].Num
	shift := func(n int) int {
		if !clearOnly && n > num {
			return n - 1
		}
		return n
	}
	oldChapters := slices.Clone(p.Chapters)
	if clearOnly {
		ch := &p.Chapters[idx]
		delete(p.ProseCandidates, num)
		ch.Content = ""
		ch.Summary = ""
		ch.Status = StatusPending
		ch.Blocks = nil
		ch.BlockSep = ""
		ch.NextBlockID = 0
		ch.MemoryRevision = ""
		ch.KnowledgeTracked = false
		ch.ContentRev = ""
		ch.WordCount = 0
		ch.DeskProseVersion = 0
		ch.DeskEarlierProse = false
	} else {
		p.ProseCandidates = nil
		p.Chapters = slices.Delete(p.Chapters, idx, idx+1)
		for i := range p.Chapters {
			p.Chapters[i].Num = shift(p.Chapters[i].Num)
		}
		batches := p.OutlineBatches[:0]
		for _, b := range p.OutlineBatches {
			if b.StartCh == num && b.EndCh == num {
				continue
			}
			if b.StartCh <= num && num <= b.EndCh {
				b.EndCh--
				b.Revision++
			} else {
				b.StartCh = shift(b.StartCh)
				b.EndCh = shift(b.EndCh)
			}
			batches = append(batches, b)
		}
		p.OutlineBatches = batches
	}
	p.CurrentChapterIndex = len(p.Chapters)
	for i, ch := range p.Chapters {
		if !ch.Inherited && ch.Status != StatusAccepted {
			p.CurrentChapterIndex = i
			break
		}
	}
	if clearOnly || len(p.Chapters) == 0 {
		p.BookStatus = BookStatusActive
		p.Phase = "writing"
	}
	purgeMemoryForChapter(p, num)
	for i, m := range p.MemoryEntries {
		if !m.Inherited {
			for j, r := range m.References {
				p.MemoryEntries[i].References[j].Chapter = shift(r.Chapter)
			}
		}
	}
	for i := range p.Foreshadows {
		f := &p.Foreshadows[i]
		if f.Inherited {
			continue
		}
		if !clearOnly && f.PlantChapter == num {
			f.PlantChapter = 0
		} else {
			f.PlantChapter = shift(f.PlantChapter)
		}
		if !clearOnly && f.TargetChapter == num {
			f.TargetChapter = 0
		} else {
			f.TargetChapter = shift(f.TargetChapter)
		}
		f.Events = slices.DeleteFunc(f.Events, func(e ForeshadowEvent) bool { return e.Chapter == num })
		for j := range f.Events {
			f.Events[j].Chapter = shift(f.Events[j].Chapter)
		}
	}
	p.PendingWritingConflict = nil
	p.LatestPlanningReview = nil
	p.LastOutlineCharacterReport = nil
	p.LastForeshadowOutlineReport = nil
	p.NarrativeCheckpoints = nil
	if !clearOnly {
		s.StoryChanges = slices.DeleteFunc(s.StoryChanges, func(c SettingChange) bool { return c.Source.Chapter == num })
		for i := range s.StoryChanges {
			s.StoryChanges[i].Source.Chapter = shift(s.StoryChanges[i].Source.Chapter)
		}
	}
	remap := func(m map[int]string) map[int]string {
		out := map[int]string{}
		for n, v := range m {
			if n != num {
				out[shift(n)] = v
			}
		}
		return out
	}
	if !clearOnly {
		s.StorySynced = remap(s.StorySynced)
	}
	if pp.Copy == nil && !clearOnly {
		pp.ApplyErrors = remap(pp.ApplyErrors)
		pp.Revisions = slices.DeleteFunc(pp.Revisions, func(r ProofreadRevision) bool { return r.ChapterNum == num })
		for i := range pp.Revisions {
			pp.Revisions[i].ChapterNum = shift(pp.Revisions[i].ChapterNum)
		}
		for i := range pp.Issues {
			issue := &pp.Issues[i]
			issue.Anchors = slices.DeleteFunc(issue.Anchors, func(a ProofreadAnchor) bool { return a.ChapterNum == num })
			for j := range issue.Anchors {
				issue.Anchors[j].ChapterNum = shift(issue.Anchors[j].ChapterNum)
			}
		}
		pp.Issues = slices.DeleteFunc(pp.Issues, func(i ProofreadIssue) bool { return len(i.Anchors) == 0 })
	}
	sb, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	pb, err := json.MarshalIndent(pp, "", "  ")
	if err != nil {
		return err
	}
	files := []progressFile{{Num: -1, Data: sb}, {Num: -2, Data: pb}}
	// The optional per-chapter Markdown exports follow their source chapter.
	for i := idx; i < len(oldChapters); i++ {
		n := oldChapters[i].Num
		_, statErr := os.Stat(ChapterMarkdownPath(dir, n))
		exists := statErr == nil
		if statErr != nil && !os.IsNotExist(statErr) {
			return statErr
		}
		var body []byte
		if !clearOnly && i+1 < len(oldChapters) {
			next := oldChapters[i+1]
			if _, err := os.Stat(ChapterMarkdownPath(dir, next.Num)); err == nil {
				body = []byte(fmt.Sprintf("# 第 %d 章: %s\n\n> **本章摘要**：%s\n\n---\n\n%s", n, next.Title, next.Summary, next.Content))
			} else if !os.IsNotExist(err) {
				return err
			}
		}
		if exists || body != nil {
			files = append(files, progressFile{Num: -2 - n, Data: body})
		}
		if clearOnly {
			break
		}
	}
	if err = saveProgressFiles(path, p, files); err != nil {
		return err
	}
	*state = *p
	*settings = *s
	*proof = *pp
	return nil
}
