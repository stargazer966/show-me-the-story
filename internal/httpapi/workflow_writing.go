package httpapi

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"showmethestory/internal/sse"
	"showmethestory/internal/story"
)

func proseBase(ch story.ChapterState) string {
	data, _ := json.Marshal([]any{ch.DeskPart, ch.DeskVersion, ch.Title, ch.Outline, story.ChapterRevision(ch)})
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func candidateToken(c story.ProseCandidate) string {
	data, _ := json.Marshal(c)
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func (h *Handlers) proseCandidate(w http.ResponseWriter, r *http.Request) {
	if !h.ensureProject(w, r) {
		return
	}
	if h.rejectIfTaskRunning(w, r) {
		return
	}
	num, _ := strconv.Atoi(r.PathValue("num"))
	idx := story.FindChapterIdx(h.state, num)
	if idx < 0 || h.state.Chapters[idx].Inherited {
		h.writeErrorReq(w, r, 404, "chapter_not_found")
		return
	}
	ch := h.state.Chapters[idx]
	if r.Method == http.MethodGet {
		h.writeJSON(w, 200, map[string]any{"candidate": h.state.ProseCandidates[num], "base": proseBase(ch), "token": candidateToken(h.state.ProseCandidates[num])})
		return
	}
	if strings.TrimSpace(ch.Outline) == "" {
		h.writeErrorReq(w, r, 409, "invalid_request", "Adopt a chapter outline first")
		return
	}
	next := *h.state
	next.Chapters = slices.Clone(h.state.Chapters)
	next.ProseCandidates = maps.Clone(h.state.ProseCandidates)
	if next.ProseCandidates == nil {
		next.ProseCandidates = map[int]story.ProseCandidate{}
	}
	if r.Method == http.MethodPut {
		candidate, ok := next.ProseCandidates[num]
		if !ok || candidate.Base != proseBase(ch) || candidateToken(candidate) != r.Header.Get("If-Match") {
			h.writeErrorReq(w, r, 409, "invalid_request", "The chapter changed. Generate a fresh candidate.")
			return
		}
		next.Chapters[idx] = candidate.Chapter
		next.BookStatus = story.BookStatusActive
		next.Phase = "writing"
		if next.CurrentChapterIndex > idx {
			next.CurrentChapterIndex = idx
		}
		delete(next.ProseCandidates, num)
	} else {
		if !h.tryStartTask() {
			h.writeErrorReq(w, r, 409, "task_running_wait")
			return
		}
		defer h.endTask()
		h.taskMu.Lock()
		h.skipKnowledgeSync = true
		h.taskMu.Unlock()
		h.logger.TaskStart("prose_candidate")
		scope := story.SkillScopeChapterGenerate
		update := r.URL.Query().Get("mode") == "update"
		h.workflowAction = fmt.Sprintf("Writing · Chapter %d · %s candidate", num, map[bool]string{true: "Update", false: "Rewrite"}[update])
		if ctx := r.Context(); ctx.Err() != nil {
			h.logger.TaskEnd("prose_candidate", false)
			return
		}
		if !update && ch.Content != "" && ch.DeskEarlierProse {
			h.logger.TaskEnd("prose_candidate", false)
			h.writeErrorReq(w, r, 409, "invalid_request", "Update the prose against the changed outline first")
			return
		}
		if update {
			scope = story.SkillScopeChapterRevise
		}
		ctx := h.activateSkills(h.taskCtx, scope, true)
		chapter, err := story.DraftProseCandidate(ctx, h.apiCfg, h.cfg, h.state, h.settings, idx, update, sse.NewLogBroadcaster())
		if err != nil {
			h.logger.TaskEnd("prose_candidate", false)
			h.writeErrorReq(w, r, 400, "invalid_request", err.Error())
			return
		}
		next.ProseCandidates[num] = story.ProseCandidate{Base: proseBase(ch), Chapter: chapter}
	}
	err := story.SaveProgress(h.progressPath, &next)
	if r.Method == http.MethodPost {
		h.logger.TaskEnd("prose_candidate", err == nil)
	}
	if err != nil {
		h.writeErrorReq(w, r, 500, "save_progress_failed", err)
		return
	}
	*h.state = next
	if r.Method == http.MethodPut {
		story.SaveChapterMarkdown(filepath.Dir(h.progressPath), next.Chapters[idx], next.Title)
		h.broadcastProgress()
		h.writeJSON(w, 200, story.ProgressView(h.state))
	} else {
		h.writeJSON(w, 200, map[string]any{"candidate": next.ProseCandidates[num], "base": proseBase(ch), "token": candidateToken(next.ProseCandidates[num])})
	}
}

func (h *Handlers) confirmSelectedChapter(w http.ResponseWriter, r *http.Request) {
	if !h.ensureProject(w, r) || h.rejectIfTaskRunning(w, r) {
		return
	}
	num, _ := strconv.Atoi(r.PathValue("num"))
	idx := story.FindChapterIdx(h.state, num)
	if idx < 0 || h.state.Chapters[idx].Inherited || h.state.Chapters[idx].Status != story.StatusReview {
		h.writeErrorReq(w, r, 409, "invalid_request", "Only reviewed prose can be confirmed")
		return
	}
	next := *h.state
	next.Chapters = slices.Clone(h.state.Chapters)
	next.Chapters[idx].Status = story.StatusAccepted
	next.Chapters[idx].KnowledgeTracked = true
	next.CurrentChapterIndex = len(next.Chapters)
	for i, ch := range next.Chapters {
		if !ch.Inherited && ch.Status != story.StatusAccepted {
			next.CurrentChapterIndex = i
			break
		}
	}
	if err := story.SaveProgress(h.progressPath, &next); err != nil {
		h.writeErrorReq(w, r, 500, "save_progress_failed", err)
		return
	}
	*h.state = next
	h.broadcastProgress()
	h.writeJSON(w, 200, story.ProgressView(h.state))
	h.startKnowledgeSync()
}
