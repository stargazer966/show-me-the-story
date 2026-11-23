package httpapi

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"showmethestory/internal/story"
	"strconv"
)

func (h *Handlers) chapterRemoval(w http.ResponseWriter, r *http.Request) {
	if !h.ensureProject(w, r) || h.rejectIfTaskRunning(w, r) {
		return
	}
	n, _ := strconv.Atoi(r.PathValue("num"))
	idx := -1
	for i, ch := range h.state.Chapters {
		if ch.Num == n && !ch.Inherited {
			idx = i
			break
		}
	}
	if idx < 0 {
		h.writeErrorReq(w, r, 404, "invalid_request", "Chapter unavailable")
		return
	}
	hash := sha256.New()
	enc := json.NewEncoder(hash)
	for _, v := range []any{h.projectName, h.state, h.settings, h.postprocess} {
		if err := enc.Encode(v); err != nil {
			h.writeErrorReq(w, r, 500, "invalid_request", err.Error())
			return
		}
	}
	token := fmt.Sprintf("%x", hash.Sum(nil))
	if r.Method == "POST" {
		count := 0
		for _, m := range h.state.MemoryEntries {
			for _, ref := range m.References {
				if !m.Inherited && ref.Chapter == n {
					count++
				}
			}
		}

		h.writeJSON(w, 200, map[string]any{"token": token, "title": h.state.Chapters[idx].Title, "references": count, "later": len(h.state.Chapters) - idx - 1, "can_clear": h.state.Chapters[idx].Content != ""})
		return
	}
	if r.Header.Get("If-Match") != token {
		h.writeErrorReq(w, r, 409, "invalid_request", "Story changed. Review the chapter again.")
		return
	}
	if err := story.RemoveChapter(h.progressPath, h.state, h.settings, h.postprocess, idx, true); err != nil {
		h.writeErrorReq(w, r, 400, "save_progress_failed", err)
		return
	}
	h.broadcastProgress()
	h.logger.SettingsUpdated()
	h.writeJSON(w, 200, story.ProgressView(h.state))
}
