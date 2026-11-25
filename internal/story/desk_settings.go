package story

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
)

type DeskSettingSuggestion struct {
	Kind     string `json:"kind"`
	EntityID string `json:"entity_id,omitempty"`
	Name     string `json:"name,omitempty"`
	Field    string `json:"field,omitempty"`
	Before   string `json:"before,omitempty"`
	After    string `json:"after"`
	Category string `json:"category,omitempty"`
	SourceID string `json:"source_id,omitempty"`
	TargetID string `json:"target_id,omitempty"`
	Beat     int    `json:"beat"`
	Origin   string `json:"origin"`
	Quote    string `json:"quote"`
}

type deskSettingDecision struct {
	Part    int `json:"part"`
	Version int `json:"version"`
	Index   int `json:"index"`
}

func deskSettingInstructions(lang string) string {
	schema := ` "setting_suggestions": [{"kind":"characters|worldview|relations","entity_id":"existing ID, or omit for new","name":"new entity name","field":"one existing field to update","after":"proposed full value","category":"new worldview category","source_id":"existing character ID","target_id":"existing character ID","beat":1,"origin":"draft|outline","quote":"exact excerpt"}]`
	if lang == "en" {
		return ` Optionally include this JSON field:` + schema + `. Suggest only durable setting facts grounded in an exact quote from the corresponding author_beats ("draft") or returned outline line ("outline"); prefer the author's beats and never use writing instructions as evidence. Use original chapter beat numbers even for a selected-beat revision. Match existing entities by ID from related_settings. Update a single field: character age, appearance, personality, background, motivation, abilities or notes; worldview description or tags; or relation label. Preserve every still-valid detail in the full proposed value. For a new character use after as background, for a new worldview entry use after as description, and for a new relation use after as the directional label from source_id to target_id. Relationships require two existing character IDs. If Lisa is Dave's mother, consider both a Lisa background update and a separate relationship. Do not create a duplicate of an existing card. New worldview categories: geography, faction, rule, history, knowledge, other. Return at most 8 suggestions.`
	}
	return ` 可选返回下列 JSON 字段：` + schema + `。只提取有逐字原文证据且适合长期记录的设定，优先作者节拍（draft），也可使用生成的大纲（outline）。不要把写作指示当作设定。单节拍修订时仍使用原章的节拍编号。用 related_settings 中的 ID 匹配已有条目，避免重复创建。每次更新一个字段，角色可更新 age、appearance、personality、background、motivation、abilities、notes，世界观可更新 description、tags，关系可更新 label。after 是保留所有仍有效细节的完整新值。新角色的 after 是背景，新世界观的 after 是描述，新关系的 after 是方向为 source_id 到 target_id 的标签，关系两端都必须是已存在的角色 ID。世界观分类为 geography、faction、rule、history、knowledge、other。最多返回 8 条。`
}

func deskSuggestionField(kind, field string) bool {
	switch kind {
	case "characters":
		switch field {
		case "age", "appearance", "personality", "background", "motivation", "abilities", "notes":
			return true
		}
	case "worldview":
		return field == "description" || field == "tags"
	case "relations":
		return field == "label"
	}
	return false
}

func deskSuggestionEntity(s *ProjectSettings, x DeskSettingSuggestion) (map[string]any, map[string]any, error) {
	if x.After == "" {
		return nil, nil, errors.New("Enter a setting value")
	}
	if x.EntityID != "" {
		entity := entityAt(s, x.Kind, x.EntityID)
		if entity == nil || !deskSuggestionField(x.Kind, x.Field) {
			return nil, nil, errors.New("Setting or field is unavailable")
		}
		before := entityAt(s, x.Kind, x.EntityID)
		entity[x.Field] = x.After
		return before, entity, nil
	}
	entity := map[string]any{}
	if x.Kind == "characters" || x.Kind == "worldview" {
		for _, current := range settingsEntities(s)[x.Kind] {
			if name, _ := current["name"].(string); strings.EqualFold(strings.TrimSpace(name), x.Name) {
				return nil, nil, errors.New("A setting with that name already exists")
			}
		}
		entity["name"] = x.Name
	}
	switch x.Kind {
	case "characters":
		entity["background"] = x.After
	case "worldview":
		switch x.Category {
		case "geography", "faction", "rule", "history", "knowledge", "other":
		default:
			return nil, nil, errors.New("Invalid worldview category")
		}
		entity["category"], entity["description"] = x.Category, x.After
	case "relations":
		if x.SourceID == x.TargetID {
			return nil, nil, errors.New("Choose two different characters")
		}
		for _, current := range s.Relations {
			if current.SourceID == x.SourceID && current.TargetID == x.TargetID && strings.EqualFold(current.Label, x.After) {
				return nil, nil, errors.New("That relationship already exists")
			}
		}
		entity["source_id"], entity["target_id"] = x.SourceID, x.TargetID
		entity["source_type"], entity["target_type"], entity["label"] = "character", "character", x.After
	default:
		return nil, nil, errors.New("Invalid setting category")
	}
	return nil, entity, nil
}

func deskSuggestions(settings *ProjectSettings, draft DeskDraft, lines []string, output json.RawMessage, selectedBeat int) []DeskSettingSuggestion {
	var entries []json.RawMessage
	if settings == nil || json.Unmarshal(output, &entries) != nil {
		return nil
	}
	beats := deskBeats(draft.Beats)
	var suggestions []DeskSettingSuggestion
	seen := map[[5]string]bool{}
	for _, entry := range entries {
		if len(suggestions) == 8 {
			break
		}
		var x DeskSettingSuggestion
		if json.Unmarshal(entry, &x) != nil || x.Beat < 1 || x.Beat > len(lines) || selectedBeat > 0 && x.Beat != selectedBeat {
			continue
		}
		x.Name = strings.TrimSpace(StripNameMarks(strings.TrimSpace(x.Name)))
		x.After, x.Quote = strings.TrimSpace(x.After), strings.TrimSpace(x.Quote)
		if x.Origin != "draft" && x.Origin != "outline" || x.Quote == "" {
			continue
		}
		if x.Origin == "draft" && !strings.Contains(deskTextAt(beats, x.Beat-1), x.Quote) {
			continue
		}
		if x.Origin == "outline" {
			if !strings.Contains(lines[x.Beat-1], x.Quote) {
				continue
			}
			if strings.Contains(deskTextAt(beats, x.Beat-1), x.Quote) {
				x.Origin = "draft"
			}
		}
		before, after, err := deskSuggestionEntity(settings, x)
		if err != nil || validateEntity(settings, x.Kind, after) != nil {
			continue
		}
		x.Before = ""
		if before != nil {
			x.Before, _ = before[x.Field].(string)
			x.Name, _ = before["name"].(string)
			if x.Kind == "relations" {
				x.SourceID, _ = before["source_id"].(string)
				x.TargetID, _ = before["target_id"].(string)
			}
			if x.Before == x.After {
				continue
			}
		}
		key := [5]string{x.Kind, x.EntityID, x.Field, x.Name, x.SourceID + "→" + x.TargetID + ":" + x.After}
		if !seen[key] {
			suggestions = append(suggestions, x)
			seen[key] = true
		}
	}
	return suggestions
}

func deskSettingDecisions(s *ProjectSettings) map[string]string {
	decisions := map[string]string{}
	if s != nil {
		for _, c := range s.StoryChanges {
			if d := c.Desk; d != nil {
				decisions[fmt.Sprintf("%d:%d:%d", d.Part, d.Version, d.Index)] = c.Status
			}
		}
	}
	return decisions
}

func decideDeskSetting(path string, state *Progress, settings *ProjectSettings, part DeskPart, req DeskRequest) error {
	if settings == nil || req.Version < 1 || req.Version > len(part.Versions) {
		return errors.New("Select an outline version")
	}
	version := part.Versions[req.Version-1]
	adopted := false
	for _, chapter := range state.Chapters {
		if chapter.DeskPart == req.Part && chapter.DeskVersion == req.Version {
			adopted = chapter.Outline == strings.Join(version.Lines, "\n\n")
			break
		}
	}
	if (!adopted && req.Action != "undo_setting") || req.Suggestion < 1 || req.Suggestion > len(version.SettingSuggestions) {
		return errors.New("Select a suggestion from the adopted outline")
	}
	x := version.SettingSuggestions[req.Suggestion-1]
	if req.Action == "apply_setting" && x.Origin == "draft" && !strings.Contains(deskTextAt(deskBeats(part.Beats), x.Beat-1), x.Quote) {
		return errors.New("The source beat changed. Update the outline to refresh the suggestion")
	}
	decision := deskSettingDecision{Part: req.Part, Version: req.Version, Index: req.Suggestion}
	next := cloneSettings(settings)
	for i, change := range next.StoryChanges {
		if change.Desk == nil || *change.Desk != decision {
			continue
		}
		if req.Action != "undo_setting" || change.Status != "applied" {
			return nil
		}
		current := entityAt(next, change.Kind, change.EntityID)
		if change.Before == nil {
			if entityHasDependents(next, change.EntityID) || !reflect.DeepEqual(current, change.After) {
				return errors.New("The setting has changed or is in use and cannot be removed")
			}
			current = nil
		} else {
			if current == nil || !reflect.DeepEqual(current[x.Field], change.After[x.Field]) {
				return errors.New("This field changed after the suggestion was applied")
			}
			if value, ok := change.Before[x.Field]; ok {
				current[x.Field] = value
			} else {
				delete(current, x.Field)
			}
		}
		if err := putEntity(next, change.Kind, change.EntityID, current); err != nil {
			return err
		}
		next.StoryChanges[i].Status = "reverted"
		return saveDeskSettings(path, settings, next)
	}
	if req.Action == "undo_setting" {
		return errors.New("This suggestion has not been applied")
	}
	change := SettingChange{ID: len(next.StoryChanges) + 1, Kind: x.Kind, EntityID: x.EntityID,
		Source: MemoryReference{Quote: x.Quote}, Desk: &decision, Status: "rejected"}
	if req.Action == "apply_setting" {
		if req.SettingValue != nil {
			x.After = strings.TrimSpace(*req.SettingValue)
		}
		before, after, err := deskSuggestionEntity(next, x)
		if err != nil {
			return err
		}
		current, _ := before[x.Field].(string)
		if current != x.Before {
			return errors.New("The setting changed. Refresh the outline before applying this suggestion")
		}
		if x.EntityID == "" {
			x.EntityID = newEntityID(next, x.Kind)
		}
		after["id"] = x.EntityID
		if err := validateEntity(next, x.Kind, after); err != nil {
			return err
		}
		if err := putEntity(next, x.Kind, x.EntityID, after); err != nil {
			return err
		}
		change.Before, change.After, change.EntityID, change.Status = before, after, x.EntityID, "applied"
	}
	next.StoryChanges = append(next.StoryChanges, change)
	return saveDeskSettings(path, settings, next)
}

func saveDeskSettings(path string, settings, next *ProjectSettings) error {
	if err := SaveProjectSettings(filepath.Join(filepath.Dir(path), "settings.json"), next); err != nil {
		return err
	}
	*settings = *next
	return nil
}
