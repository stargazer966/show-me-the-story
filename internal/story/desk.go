package story

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"showmethestory/internal/config"
	"showmethestory/internal/llm"
	"slices"
	"strings"
)

type StoryDesk struct {
	Revision int        `json:"revision"`
	Parts    []DeskPart `json:"parts"`
}
type DeskDraft struct {
	Title string `json:"title"`
	Beats string `json:"beats"`
	Focus string `json:"focus"`
}
type DeskVersion struct {
	Source  DeskDraft `json:"source"`
	Context string    `json:"context,omitempty"`
	Lines   []string  `json:"lines"`
}
type DeskPart struct {
	PreviouslyLinked bool `json:"previously_linked,omitempty"`
	DeskDraft
	Versions []DeskVersion `json:"versions"`
}
type DeskRequest struct {
	Project           string    `json:"project"`
	Action            string    `json:"action"`
	Revision          int       `json:"revision"`
	Part              int       `json:"part"`
	Version           int       `json:"version"`
	Mode              string    `json:"mode"`
	Beat              int       `json:"beat"` // zero means whole outline
	Comment           string    `json:"comment"`
	Direction         string    `json:"direction"`
	PreviousDirection string    `json:"previous_direction"`
	Draft             DeskDraft `json:"draft"`
}
type DeskPrompt struct {
	System string `json:"system"`
	User   string `json:"user"`
}

func deskBeats(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if v := strings.TrimSpace(line); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// Populate missing links and preserve externally edited chapter outlines in history.
func syncDesk(p *Progress) bool {
	changed := false
	for i := range p.Chapters {
		ch := &p.Chapters[i]
		if ch.Inherited {
			continue
		}
		if ch.DeskPart < 1 || ch.DeskPart > len(p.Desk.Parts) {
			p.Desk.Parts = append(p.Desk.Parts, DeskPart{DeskDraft: DeskDraft{Title: ch.Title}})
			ch.DeskPart = len(p.Desk.Parts)
			changed = true
		}
		part := &p.Desk.Parts[ch.DeskPart-1]
		part.PreviouslyLinked = true
		if ch.DeskVersion > 0 && ch.DeskVersion <= len(part.Versions) && strings.Join(part.Versions[ch.DeskVersion-1].Lines, "\n\n") == ch.Outline {
			continue
		}
		part.Versions = append(slices.Clone(part.Versions), DeskVersion{Lines: []string{ch.Outline}, Source: part.DeskDraft})
		ch.DeskVersion = len(part.Versions)
		changed = true
	}
	return changed
}
func deskContext(cfg *config.Config, p *Progress, settings *ProjectSettings, draft DeskDraft) string {
	refs, _ := retrieveSettings(settings, draft.Beats+"\n"+draft.Focus, settingsContextRunes)
	b, _ := json.Marshal([]any{cfg.Language, cfg.Story, p.CorePrompt, p.LongTermDirection, refs})
	return string(b)
}

func deskPrompt(cfg *config.Config, state *Progress, settings *ProjectSettings, skills []Skill, part DeskPart, req DeskRequest) (DeskPrompt, int, error) {
	beats := deskBeats(part.Beats)
	scope := SkillScopeOutlineGenerate
	instructions := "Write a chapter outline in English. Expand each supplied beat into complete sentences, in order. Give more space to the author's focus. Begin at the first beat and stop at the last. Only brief introductions, endings and transitions are allowed. Never invent a preceding or following scene. Return JSON with a lines array of strings, one per requested beat."
	if cfg.Language != "en" {
		instructions = "用中文按顺序将每个节拍扩写为完整的章纲句子，优先照顾作者重点。以首个节拍开始、末个节拍结束，只允许简短过渡，不添加前后场景。返回 JSON，lines 数组每个节拍一个字符串。"
	}
	history := *state
	history.Chapters = nil
	for _, ch := range state.Chapters {
		if ch.DeskPart == req.Part {
			break
		}
		history.Chapters = append(history.Chapters, ch)
	}
	query := part.Beats + "\n" + part.Focus
	refs, _ := retrieveSettings(settings, query, settingsContextRunes)
	lo, hi := outlineRange(cfg)
	data := map[string]any{"title": state.Title, "premise": state.CorePrompt, "story": cfg.Story, "long_term_direction": state.LongTermDirection, "part": part.Title, "author_beats": beats, "focus": part.Focus, "previous_chapters": BuildPlanningHistory(&history, query, cfg.Language), "related_settings": refs, "chapter_outline_length": formatOutlineLengthRequirementBlock(lo, hi, cfg.Language)}
	count := len(beats)
	if req.Mode == "update" {
		if req.Version < 1 || req.Version > len(part.Versions) {
			return DeskPrompt{}, 0, errors.New("Select the outline to update")
		}
		old := part.Versions[req.Version-1]
		if old.Source == part.DeskDraft && (old.Context == "" || old.Context == deskContext(cfg, state, settings, part.DeskDraft)) {
			return DeskPrompt{}, 0, errors.New("The draft has not changed")
		}
		data["previous_author_beats"] = deskBeats(old.Source.Beats)
		data["previous_focus"] = old.Source.Focus
		data["previous_outline"] = old.Lines
		if old.Context != "" {
			data["previous_story_context"] = json.RawMessage(old.Context)
		}
		if cfg.Language == "en" {
			instructions += " Compare the previous and current inputs. Make only necessary changes."
		} else {
			instructions += " 比较新旧输入，只作必要修改。"
		}
	}
	if req.Comment != "" {
		scope = SkillScopeOutlineRevise
		if req.Version < 1 || req.Version > len(part.Versions) {
			return DeskPrompt{}, 0, errors.New("Select an outline version")
		}
		lines := part.Versions[req.Version-1].Lines
		if req.Beat < 0 || req.Beat > len(lines) {
			return DeskPrompt{}, 0, errors.New("Invalid beat")
		}
		data["current_outline"] = lines
		data["revision_request"] = req.Comment
		count = len(lines)
		if req.Beat > 0 {
			data["revise_only_beat_number"] = req.Beat
			count = 1
		}
	}
	if count == 0 {
		return DeskPrompt{}, 0, errors.New("Add a beat first")
	}
	data["required_output_lines"] = count
	system := instructions
	if req.Comment != "" && req.Beat > 0 {
		if cfg.Language == "en" {
			system += " Return only the revised paragraph for the selected beat."
		} else {
			system += " 只返回选中节拍的修订段落。"
		}
	}
	if addon := FormatSkillsContent(ResolveSkills(skills, cfg.SkillConfig, scope, cfg.Language)); addon != "" {
		system += "\n\n## Active user skills\n\n" + addon
	}
	body, err := json.MarshalIndent(data, "", "  ")
	return DeskPrompt{system, string(body)}, count, err
}

var ErrDeskChanged = errors.New("The project changed. Reload the desk before saving.")

func RunDesk(ctx context.Context, path string, state *Progress, cfg *config.Config, settings *ProjectSettings, skills []Skill, model *config.APIConfig, req DeskRequest) (any, error) {
	next := *state
	next.Chapters = slices.Clone(state.Chapters)
	next.Desk.Parts = slices.Clone(state.Desk.Parts)
	changed := syncDesk(&next)
	if req.Action != "open" && req.Action != "direction" && (req.Revision != state.Desk.Revision || changed) {
		return nil, ErrDeskChanged
	}
	if req.Action == "open" && len(next.Desk.Parts) == 0 {
		next.Desk.Parts = []DeskPart{{}}
		changed = true
	}
	switch req.Action {
	case "open":
	case "add":
		next.Desk.Parts = append(next.Desk.Parts, DeskPart{})
		changed = true
	case "direction":
		if req.PreviousDirection != next.LongTermDirection {
			return nil, ErrDeskChanged
		}
		next.LongTermDirection = req.Direction
		changed = true
	default:
		if req.Part < 1 || req.Part > len(next.Desk.Parts) {
			return nil, errors.New("Select a part")
		}
		part := &next.Desk.Parts[req.Part-1]
		switch req.Action {
		case "manual_outline":
			outline := strings.TrimSpace(req.Comment)
			if outline == "" {
				return nil, errors.New("Add outline text")
			}
			part.Versions = append(slices.Clone(part.Versions), DeskVersion{Source: part.DeskDraft, Context: deskContext(cfg, &next, settings, part.DeskDraft), Lines: strings.Split(outline, "\n\n")})
			changed = true
		case "save":
			part.DeskDraft = req.Draft
			changed = true
		case "use":
			if req.Version < 1 || req.Version > len(part.Versions) {
				return nil, errors.New("Outline unavailable")
			}
			idx := slices.IndexFunc(next.Chapters, func(c ChapterState) bool { return c.DeskPart == req.Part })
			if idx < 0 {
				for _, ch := range next.Chapters {
					if ch.DeskPart > req.Part {
						return nil, errors.New("An earlier part cannot be appended after a later chapter")
					}
				}
				for n := 1; n < req.Part; n++ {
					if slices.IndexFunc(next.Chapters, func(c ChapterState) bool { return c.DeskPart == n }) < 0 && !next.Desk.Parts[n-1].PreviouslyLinked {
						return nil, errors.New("Adopt earlier parts first")
					}
				}
				num := 1
				for _, ch := range next.Chapters {
					num = max(num, ch.Num+1)
				}
				next.Chapters = append(next.Chapters, ChapterState{Num: num, Status: StatusPending, DeskPart: req.Part})
				idx = len(next.Chapters) - 1
			}
			part.PreviouslyLinked = true
			ch := &next.Chapters[idx]
			v := part.Versions[req.Version-1]
			if ch.Content != "" && ch.DeskProseVersion == 0 {
				ch.DeskProseVersion = ch.DeskVersion
			}
			ch.DeskEarlierProse = ch.Content != "" && ch.DeskProseVersion != req.Version
			ch.DeskVersion = req.Version
			ch.Outline = strings.Join(v.Lines, "\n\n")
			if v.Source.Title != "" {
				ch.Title = v.Source.Title
			}
			if ch.Title == "" {
				ch.Title = fmt.Sprintf("Chapter %d", ch.Num)
			}
			next.Phase = "writing"
			changed = true
		case "preview", "generate":
			input := *part
			if req.Comment != "" && req.Version > 0 && req.Version <= len(part.Versions) {
				input.DeskDraft = part.Versions[req.Version-1].Source
			}
			prompt, count, err := deskPrompt(cfg, &next, settings, skills, input, req)
			if err != nil {
				return nil, err
			}
			if req.Action == "preview" {
				return prompt, nil
			}
			raw, err := llm.CallAPI(ctx, model, prompt.System, prompt.User)
			if err != nil {
				return nil, err
			}
			var reply struct {
				Lines []string `json:"lines"`
			}
			if err := json.Unmarshal([]byte(cleanJSONResponse(raw)), &reply); err != nil || len(reply.Lines) != count {
				return nil, errors.New("Model returned an incomplete outline. Your draft and earlier versions are saved.")
			}
			for i, line := range reply.Lines {
				reply.Lines[i] = strings.TrimSpace(line)
				if reply.Lines[i] == "" {
					return nil, errors.New("Model returned an empty beat")
				}
			}
			if req.Mode == "update" {
				old := part.Versions[req.Version-1]
				previous, current := deskBeats(old.Source.Beats), deskBeats(input.Beats)
				if len(previous) == len(old.Lines) && old.Source.Focus == input.Focus && (old.Context == "" || old.Context == deskContext(cfg, &next, settings, input.DeskDraft)) {
					for i, j := range unchangedBeats(previous, current) {
						reply.Lines[i] = old.Lines[j]
					}
				}
			}
			if req.Comment != "" && req.Beat > 0 {
				lines := slices.Clone(part.Versions[req.Version-1].Lines)
				lines[req.Beat-1] = reply.Lines[0]
				reply.Lines = lines
			}
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			part.Versions = append(slices.Clone(part.Versions), DeskVersion{Source: input.DeskDraft, Context: deskContext(cfg, &next, settings, input.DeskDraft), Lines: reply.Lines})
			changed = true
		default:
			return nil, errors.New("Unknown desk action")
		}
	}
	if changed {
		next.Desk.Revision++
		if err := SaveProgress(path, &next); err != nil {
			return nil, err
		}
		*state = next
	}
	contexts := make([]string, len(next.Desk.Parts))
	for i, p := range next.Desk.Parts {
		contexts[i] = deskContext(cfg, &next, settings, p.DeskDraft)
	}
	return map[string]any{"desk": next.Desk, "chapters": ProgressView(&next).Chapters, "direction": next.LongTermDirection, "contexts": contexts}, nil
}

// Match equal beats in reading order, keeping duplicate beats deterministic.
func unchangedBeats(old, current []string) map[int]int {
	matches := map[int]int{}
	counts := map[string][]int{}
	for i, text := range old {
		counts[text] = append(counts[text], i)
	}
	for i, text := range current {
		if positions := counts[text]; len(positions) > 0 {
			matches[i] = positions[0]
			counts[text] = positions[1:]
		}
	}
	return matches
}
