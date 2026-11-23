package story

import (
	"encoding/json"
	"strings"
)

func deskWritingGuidance(state *Progress, ch ChapterState, version int, lang string) string {
	if ch.DeskPart < 1 || ch.DeskPart > len(state.Desk.Parts) {
		return ""
	}
	versions := state.Desk.Parts[ch.DeskPart-1].Versions
	if version < 1 || version > len(versions) {
		return ""
	}
	v := versions[version-1]
	if version == ch.DeskVersion && strings.Join(v.Lines, "\n\n") != ch.Outline {
		return ""
	}
	type beat struct {
		Number      int    `json:"beat"`
		Outline     string `json:"outline"`
		Instruction string `json:"writing_instruction,omitempty"`
		Remark      string `json:"author_remark,omitempty"`
	}
	var beats []beat
	hasGuidance := strings.TrimSpace(v.Source.Focus) != ""
	for i, line := range v.Lines {
		entry := beat{i + 1, line, deskTextAt(v.Instructions, i), ""}
		if len(deskBeats(v.Source.Beats)) == len(v.Lines) {
			entry.Remark = deskTextAt(v.Source.Remarks, i)
		}
		hasGuidance = hasGuidance || strings.TrimSpace(entry.Instruction+entry.Remark) != ""
		beats = append(beats, entry)
	}
	if !hasGuidance {
		return ""
	}
	data, _ := json.Marshal(struct {
		ChapterRemarks string `json:"chapter_remarks,omitempty"`
		Beats          []beat `json:"beats"`
	}{v.Source.Focus, beats})
	if lang == "zh" {
		return "\n\n【已采用章纲的正文指示】\n按节拍顺序写故事；篇幅比例按整章正文计算。每个节拍的写作指示优先于该节拍的原始备注和章节备注和全章默认视角、文风。节拍结束后恢复全章默认值。只输出小说正文，不输出这些指示或编号。\n" + string(data)
	}
	return "\n\n[Prose guidance from the outline version]\nFollow the beats in order. Treat requested proportions as shares of the entire prose chapter. Within each beat, apply its writing instructions first, then its author remarks, then the chapter remarks and global defaults. A beat-specific POV or style exception applies only to that beat. Output story prose only.\n" + string(data)
}
