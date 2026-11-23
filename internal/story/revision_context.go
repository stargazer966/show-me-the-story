package story

import (
	"encoding/json"
	"strings"

	"showmethestory/internal/i18n"
	"showmethestory/internal/prose"
)

// Chapter revision addresses blocks by reading order, matching the Writing editor.
func revisionBlocks(ch ChapterState) string {
	SyncChapterBlocks(&ch)
	type block struct {
		Number int    `json:"block"`
		ID     int    `json:"block_id"`
		Words  int    `json:"word_count"`
		Text   string `json:"text"`
	}
	blocks := make([]block, len(ch.Blocks))
	for i, b := range ch.Blocks {
		blocks[i] = block{Number: i + 1, ID: b.ID, Words: prose.CountProseUnits(b.Text), Text: b.Text}
	}
	data, _ := json.Marshal(blocks)
	return string(data)
}

func revisionPacingContext(state *Progress, ch ChapterState, lang string) string {
	type beat struct {
		Number  int    `json:"beat"`
		Draft   string `json:"author_beat,omitempty"`
		Outline string `json:"outline"`
	}
	var beats []beat
	if ch.DeskPart > 0 && ch.DeskPart <= len(state.Desk.Parts) {
		versions := state.Desk.Parts[ch.DeskPart-1].Versions
		if ch.DeskVersion > 0 && ch.DeskVersion <= len(versions) {
			adopted := versions[ch.DeskVersion-1]
			if strings.Join(adopted.Lines, "\n\n") == ch.Outline {
				source := deskBeats(adopted.Source.Beats)
				for i, line := range adopted.Lines {
					item := beat{Number: i + 1, Outline: line}
					if len(source) == len(adopted.Lines) {
						item.Draft = source[i]
					}
					beats = append(beats, item)
				}
			}
		}
	}
	outline := ""
	if len(beats) == 0 {
		outline = ch.Outline
	}
	data, _ := json.Marshal(struct {
		Beats   []beat `json:"beats,omitempty"`
		Outline string `json:"chapter_outline,omitempty"`
	}{Beats: beats, Outline: outline})
	if i18n.NormalizeLanguage(lang) == i18n.LangZH {
		return "\n\n[章纲与节奏]\n" + string(data) + "\n正文段落按位置编号，章纲节拍单独编号。段落范围（如第1到4段）包含两端。扩写某个节拍时，按情节定位对应正文，可以涉及多段。按作者意见分配篇幅。先压缩指定段落，在指定节拍补充实质发展。数值要求用于指定范围的合计；没有数值时明显改变比例。允许在指定范围内合并或拆分段落，保留范围外文字和原有叙事顺序。输出前检查比例，仅输出正文，不带编号或统计。"
	}
	return "\n\n[Adopted outline and pacing instructions]\n" + string(data) +
		"\nA block number is the paragraph's position in the current prose. A beat number is the position in the adopted outline. Block ranges are inclusive. Locate a requested beat by matching its events to the prose; it may span several blocks. " +
		"When the author requests a redistribution, plan the new allocation before drafting. Compress the specified blocks and spend the freed words developing the requested beat. " +
		"Apply numeric budgets to the named span as a whole. If no counts are provided, make the proportion visibly different. When the feedback requests both changes, satisfy both before you finish. " +
		"Merge or split paragraphs within the affected spans if needed. Preserve the sequence of events and preserve unrelated prose. Before responding, verify the allocations. Output only the revised prose, without block labels, word counts, or the allocation plan."
}
