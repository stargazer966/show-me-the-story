package story

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"showmethestory/internal/config"
	"showmethestory/internal/llm"
	"showmethestory/internal/sse"
)

// DraftProseCandidate builds a candidate against the selected chapter without changing the project.
func DraftProseCandidate(ctx context.Context, model *config.APIConfig, cfg *config.Config, state *Progress, settings *ProjectSettings, idx int, update bool, logger *sse.LogBroadcaster) (ChapterState, error) {
	ch := state.Chapters[idx]
	if !update {
		prose, err := generateChapterContentWithLengthControl(ctx, model, cfg, state, idx, settings, "", logger)
		if err != nil {
			return ch, err
		}
		ch.Content = prose
		SyncChapterBlocks(&ch)
	} else {
		if ch.Content == "" || !ch.DeskEarlierProse || ch.DeskPart < 1 || ch.DeskPart > len(state.Desk.Parts) {
			return ch, errors.New("This chapter needs a changed outline and an existing prose version")
		}
		versions := state.Desk.Parts[ch.DeskPart-1].Versions
		if ch.DeskProseVersion < 1 || ch.DeskProseVersion > len(versions) {
			return ch, errors.New("The source outline for this prose is unavailable")
		}
		ch.Blocks = append([]Block(nil), ch.Blocks...)
		SyncChapterBlocks(&ch)
		refs, _ := retrieveSettings(settings, ch.Outline, settingsContextRunes)
		data, _ := json.Marshal(map[string]any{"old_outline": versions[ch.DeskProseVersion-1].Lines, "new_outline": ch.Outline, "blocks": ch.Blocks, "story_so_far": buildHistorySummaryForLang(state, idx, cfg.Language), "settings": refs, "style": cfg.Story.WritingStyle, "pov": cfg.Story.WritingPOV})
		system := "Update the existing chapter for the outline differences only. Preserve voice and paragraph boundaries. Return JSON {\"changes\":[{\"block_id\":1,\"text\":\"replacement paragraph\"}]}. Return only changed blocks. Use the manuscript's language."
		raw, err := llm.CallAPI(ctx, model, system, string(data)+factProtection(state, ch.Num, cfg.Language))
		if err != nil {
			return ch, err
		}
		var result struct {
			Changes *[]struct {
				ID   int    `json:"block_id"`
				Text string `json:"text"`
			} `json:"changes"`
		}
		if err := json.Unmarshal([]byte(cleanJSONResponse(raw)), &result); err != nil || result.Changes == nil {
			return ch, errors.New("Invalid prose update response")
		}
		seen := map[int]bool{}
		for _, change := range *result.Changes {
			i := FindBlockIdx(&ch, change.ID)
			if i < 0 || seen[change.ID] || ch.Blocks[i].Type == BlockSceneBreak || strings.TrimSpace(change.Text) == "" || strings.Contains(change.Text, ch.BlockSep) {
				return ch, fmt.Errorf("Invalid change to block %d", change.ID)
			}
			seen[change.ID] = true
			ch.Blocks[i].Text = change.Text
		}
		rebuildContentFromBlocks(&ch)
	}
	if strings.TrimSpace(ch.Content) == "" {
		return ch, errors.New("The model returned empty prose")
	}
	summary, err := generateChapterSummary(ctx, model, cfg, ch.Content)
	if err != nil {
		return ch, err
	}
	if strings.TrimSpace(summary) == "" {
		return ch, errors.New("The model returned an empty summary")
	}
	ch.Summary = summary
	ch.Status = StatusReview
	ch.DeskEarlierProse = false
	ch.DeskProseVersion = ch.DeskVersion
	ch.KnowledgeTracked = true
	ch.MemoryRevision = ""
	return ch, nil
}

type ProseCandidate struct {
	Base    string       `json:"base"`
	Chapter ChapterState `json:"chapter"`
}
