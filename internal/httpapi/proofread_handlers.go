package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"showmethestory/internal/story"
)

func (h *Handlers) proofreadReady(w http.ResponseWriter, r *http.Request) bool {
	if !h.ensureProject(w, r) {
		return false
	}
	if h.postprocess.Copy == nil && !story.IsBookFullyAccepted(h.state) {
		h.writeErrorReq(w, r, http.StatusConflict, "book_not_complete")
		return false
	}
	return true
}

func (h *Handlers) GetProofread(w http.ResponseWriter, r *http.Request) {
	if !h.ensureProject(w, r) {
		return
	}
	h.writeJSON(w, http.StatusOK, proofreadView(h.postprocess, h.state))
}

func (h *Handlers) DeleteProofread(w http.ResponseWriter, r *http.Request) {
	if !h.ensureProject(w, r) || h.rejectIfTaskRunning(w, r) {
		return
	}
	keep := h.postprocess
	h.postprocess = story.NewProofreadState()
	h.postprocess.BackupAcknowledged = keep.BackupAcknowledged
	h.postprocess.ContentModified = keep.ContentModified
	h.postprocess.Revisions = keep.Revisions
	h.postprocess.Copy = keep.Copy
	h.postprocess.CopySettings = keep.CopySettings
	h.postprocess.Source = keep.Source
	h.postprocess.Proposed = keep.Proposed
	if err := story.SavePostProcess(h.postprocessPath, h.postprocess); err != nil {
		h.writeErrorReq(w, r, 500, "save_failed", err)
		return
	}
	h.writeJSON(w, 200, proofreadView(h.postprocess, h.state))
}

func (h *Handlers) PostProofreadBackup(w http.ResponseWriter, r *http.Request) {
	if !h.proofreadReady(w, r) || h.rejectIfTaskRunning(w, r) {
		return
	}
	next := *h.postprocess
	if next.Copy == nil {
		raw, _ := json.Marshal(h.state)
		if err := json.Unmarshal(raw, &next.Copy); err != nil {
			h.writeErrorReq(w, r, 500, "save_failed", err)
			return
		}
		raw, _ = json.Marshal(h.settings)
		if err := json.Unmarshal(raw, &next.CopySettings); err != nil {
			h.writeErrorReq(w, r, 500, "save_failed", err)
			return
		}
		next.Source = map[int]string{}
		for i := range next.Copy.Chapters {
			ch := &next.Copy.Chapters[i]
			story.SyncChapterBlocks(ch)
			next.Source[ch.Num] = proofSource(*ch)
		}
	}
	next.BackupAcknowledged = true
	if err := story.SavePostProcess(h.postprocessPath, &next); err != nil {
		h.writeErrorReq(w, r, 500, "save_failed", err)
		return
	}
	*h.postprocess = next
	h.writeJSON(w, 200, proofreadView(h.postprocess, h.state))
}

func (h *Handlers) beginProofreadTask(w http.ResponseWriter, r *http.Request) bool {
	if !h.proofreadReady(w, r) {
		return false
	}
	if !h.postprocess.BackupAcknowledged || h.postprocess.Copy == nil {
		h.writeErrorReq(w, r, http.StatusConflict, "proofread_backup_required")
		return false
	}
	if !h.tryStartTask() {
		h.writeErrorReq(w, r, http.StatusConflict, "task_running_wait")
		return false
	}
	h.taskMu.Lock()
	h.skipKnowledgeSync = true
	h.taskMu.Unlock()
	return true
}

func (h *Handlers) PostProofreadAnalyze(w http.ResponseWriter, r *http.Request) {
	if !h.beginProofreadTask(w, r) {
		return
	}
	go func() {
		defer h.endTask()
		h.logger.TaskStart("proofread_analyze")
		next, err := cloneProofread(h.postprocess)
		if err == nil {
			err = story.AnalyzeProofread(h.taskCtx, h.apiCfg, h.cfg, next.CopySettings, next.Copy, next, h.logger)
		}
		if err == nil {
			err = story.SavePostProcess(h.postprocessPath, next)
		}
		if err == nil {
			h.postprocess = next
		}
		if err != nil {
			h.logger.ErrorKey("log.postprocess_diagnose_failed", err)
		}
		h.logger.TaskEnd("proofread_analyze", err == nil)
		h.broadcastProgress()
	}()
	h.writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
}

func (h *Handlers) PostProofreadApply(w http.ResponseWriter, r *http.Request) {
	if !h.beginProofreadTask(w, r) {
		return
	}
	var body struct {
		Preferences string `json:"preferences"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	go func() {
		defer h.endTask()
		h.logger.TaskStart("proofread_apply")
		next, err := cloneProofread(h.postprocess)
		if err == nil {
			next.AuthorRequirements = strings.TrimSpace(body.Preferences)
			err = story.ProposeProofread(h.taskCtx, h.apiCfg, h.cfg, next, h.skills, h.logger)
		}
		if err == nil {
			err = story.SavePostProcess(h.postprocessPath, next)
		}
		if err == nil {
			h.postprocess = next
		}
		if err != nil {
			h.logger.ErrorKey("log.postprocess_execute_failed", err)
		}
		h.logger.TaskEnd("proofread_apply", err == nil)
		h.broadcastProgress()
	}()
	h.writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
}

func (h *Handlers) PutProofreadIssue(w http.ResponseWriter, r *http.Request) {
	if !h.ensureProject(w, r) {
		return
	}
	if h.rejectIfTaskRunning(w, r) {
		return
	}
	var body struct {
		Status string `json:"status"`
		Note   string `json:"note"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil || (body.Status != "pending" && body.Status != "resolved" && body.Status != "ignored") {
		h.writeErrorReq(w, r, 400, "invalid_json")
		return
	}
	h.changeProofread(w, r, func(pp *story.PostProcessState) error {
		for i := range pp.Issues {
			if pp.Issues[i].ID == r.PathValue("id") {
				pp.Issues[i].Status = body.Status
				pp.Issues[i].DecisionNote = strings.TrimSpace(body.Note)
				return nil
			}
		}
		return fmt.Errorf("issue unavailable")
	}, 0)
}

func (h *Handlers) PostProofreadUndo(w http.ResponseWriter, r *http.Request) {
	if !h.proofreadReady(w, r) {
		return
	}
	if h.rejectIfTaskRunning(w, r) {
		return
	}
	num, err := strconv.Atoi(r.PathValue("num"))
	if err != nil {
		h.writeErrorReq(w, r, 400, "invalid_chapter_num")
		return
	}
	if h.postprocess.Copy == nil {
		h.writeErrorReq(w, r, 409, "proofread_backup_required")
		return
	}
	h.changeProofread(w, r, func(pp *story.PostProcessState) error {
		return story.UndoProofreadChapter(pp.Copy, pp.CopySettings, pp, num)
	}, 0)
}

func cloneProofread(pp *story.PostProcessState) (*story.PostProcessState, error) {
	data, err := json.Marshal(pp)
	if err != nil {
		return nil, err
	}
	var copy story.PostProcessState
	err = json.Unmarshal(data, &copy)
	return &copy, err
}

func (h *Handlers) adoptProofreadChapter(w http.ResponseWriter, r *http.Request) {
	if !h.proofreadReady(w, r) || h.rejectIfTaskRunning(w, r) {
		return
	}
	if h.postprocess.Copy == nil {
		h.writeErrorReq(w, r, 409, "proofread_backup_required")
		return
	}
	num, _ := strconv.Atoi(r.PathValue("num"))
	h.changeProofread(w, r, func(pp *story.PostProcessState) error { return story.AdoptProofread(pp, num, r.Header.Get("If-Match")) }, 0)
}

func (h *Handlers) changeProofread(w http.ResponseWriter, r *http.Request, edit func(*story.PostProcessState) error, chapter int) {
	next, err := cloneProofread(h.postprocess)
	if err != nil {
		h.writeErrorReq(w, r, 500, "save_failed", err)
		return
	}
	if err := edit(next); err != nil {
		h.writeErrorReq(w, r, 409, "invalid_request", err.Error())
		return
	}
	if err := story.SavePostProcess(h.postprocessPath, next); err != nil {
		h.writeErrorReq(w, r, 500, "save_failed", err)
		return
	}
	h.postprocess = next
	if chapter == 0 {
		h.writeJSON(w, 200, proofreadView(next, h.state))
		return
	}
	ch := next.Copy.Chapters[story.FindChapterIdx(next.Copy, chapter)]
	ch.ContentRev = story.ChapterRevision(ch)
	h.writeJSON(w, 200, ch)
}

func (h *Handlers) proofreadBlockEdit(w http.ResponseWriter, r *http.Request, edit func(*story.ChapterState) error) {
	if !h.proofreadReady(w, r) || h.rejectIfTaskRunning(w, r) {
		return
	}
	num, _ := strconv.Atoi(r.PathValue("num"))
	if h.postprocess.Copy == nil {
		h.writeErrorReq(w, r, 409, "proofread_backup_required")
		return
	}
	if story.FindChapterIdx(h.postprocess.Copy, num) < 0 {
		h.writeErrorReq(w, r, 404, "chapter_not_found")
		return
	}
	h.changeProofread(w, r, func(pp *story.PostProcessState) error {
		ch := &pp.Copy.Chapters[story.FindChapterIdx(pp.Copy, num)]
		if err := edit(ch); err != nil {
			return err
		}
		story.RefreshProofreadAnchors(pp.Copy, pp.CopySettings, ch)
		pp.ContentModified = true
		return nil
	}, num)
}

func proofSource(ch story.ChapterState) string {
	return fmt.Sprintf("%d:%s", ch.DeskPart, story.ChapterRevision(ch))
}

func proofreadView(pp *story.PostProcessState, current *story.Progress) any {
	view := *pp
	view.Copy = nil
	view.CopySettings = nil
	view.Source = nil
	chapters := []map[string]any{}
	if pp.Copy != nil {
		for _, ch := range pp.Copy.Chapters {
			idx := story.FindChapterIdx(current, ch.Num)
			stale := idx < 0 || proofSource(current.Chapters[idx]) != pp.Source[ch.Num]
			chapters = append(chapters, map[string]any{"num": ch.Num, "title": ch.Title, "stale": stale})
		}
	}
	return struct {
		story.PostProcessState
		HasCopy  bool             `json:"has_copy"`
		Chapters []map[string]any `json:"chapters"`
	}{view, pp.Copy != nil, chapters}
}

func (h *Handlers) getProofreadChapter(w http.ResponseWriter, r *http.Request) {
	if !h.ensureProject(w, r) {
		return
	}
	num, _ := strconv.Atoi(r.PathValue("num"))
	if pp := h.postprocess; pp.Copy != nil {
		if idx := story.FindChapterIdx(pp.Copy, num); idx >= 0 {
			ch := pp.Copy.Chapters[idx]
			story.SyncChapterBlocks(&ch)
			ch.ContentRev = story.ChapterRevision(ch)
			h.writeJSON(w, 200, ch)
			return
		}
	}
	h.writeErrorReq(w, r, 404, "chapter_not_found")
}

func (h *Handlers) exportProofreadProse(w http.ResponseWriter, r *http.Request) {
	if !h.ensureProject(w, r) {
		return
	}
	if h.postprocess.Copy == nil {
		h.writeErrorReq(w, r, 409, "proofread_backup_required")
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	for _, ch := range h.postprocess.Copy.Chapters {
		if !ch.Inherited {
			fmt.Fprintf(w, "# %s\n\n%s\n\n", ch.Title, ch.Content)
		}
	}
}

func (h *Handlers) syncProofreadChapter(w http.ResponseWriter, r *http.Request) {
	if !h.ensureProject(w, r) || h.rejectIfTaskRunning(w, r) {
		return
	}
	num, _ := strconv.Atoi(r.PathValue("num"))
	i := story.FindChapterIdx(h.state, num)
	if h.postprocess.Copy == nil || i < 0 || h.state.Chapters[i].Content == "" {
		h.writeErrorReq(w, r, 404, "chapter_not_found")
		return
	}
	h.changeProofread(w, r, func(pp *story.PostProcessState) error {
		j := story.FindChapterIdx(pp.Copy, num)
		if j < 0 {
			return fmt.Errorf("chapter unavailable")
		}
		if pp.Copy.Chapters[j].DeskPart != 0 && pp.Copy.Chapters[j].DeskPart != h.state.Chapters[i].DeskPart {
			return fmt.Errorf("chapter identity has changed")
		}
		pp.Copy.Chapters[j] = h.state.Chapters[i]
		pp.Source[num] = proofSource(h.state.Chapters[i])
		return nil
	}, 0)
}

func (h *Handlers) PutProofreadBlock(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text string `json:"text"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil || strings.TrimSpace(body.Text) == "" {
		h.writeErrorReq(w, r, 400, "block_text_required")
		return
	}
	id, _ := strconv.Atoi(r.PathValue("id"))
	h.proofreadBlockEdit(w, r, func(ch *story.ChapterState) error { return story.UpdateBlock(ch, id, body.Text) })
}
func (h *Handlers) DeleteProofreadBlock(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.PathValue("id"))
	h.proofreadBlockEdit(w, r, func(ch *story.ChapterState) error { return story.DeleteBlock(ch, id) })
}
func (h *Handlers) PostProofreadBlock(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AfterID int    `json:"after_id"`
		Text    string `json:"text"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil || strings.TrimSpace(body.Text) == "" {
		h.writeErrorReq(w, r, 400, "block_text_required")
		return
	}
	h.proofreadBlockEdit(w, r, func(ch *story.ChapterState) error {
		_, err := story.InsertBlockAfter(ch, body.AfterID, body.Text)
		return err
	})
}

func (h *Handlers) GetProofreadExport(w http.ResponseWriter, r *http.Request) {
	if !h.ensureProject(w, r) {
		return
	}
	title := h.state.Title
	if h.cfg.Story.Title != "" {
		title = h.cfg.Story.Title
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="proofread-report-%d.md"`, time.Now().Unix()))
	english := h.cfg.Language == "en"
	if english {
		fmt.Fprintf(w, "# %s — Final Proofreading Report\n\n", title)
	} else {
		fmt.Fprintf(w, "# %s — 完稿校订报告\n\n", title)
	}
	for _, issue := range h.postprocess.Issues {
		if english {
			fmt.Fprintf(w, "## [%s/%s/%s] %s\n\n%s\n\nSuggestion: %s\n\n", issue.Status, issue.Severity, issue.Category, issue.Title, issue.Detail, issue.Suggestion)
		} else {
			fmt.Fprintf(w, "## [%s/%s/%s] %s\n\n%s\n\n建议：%s\n\n", issue.Status, issue.Severity, issue.Category, issue.Title, issue.Detail, issue.Suggestion)
		}
		for _, a := range issue.Anchors {
			if english {
				fmt.Fprintf(w, "- Chapter %d · block %d: %s\n", a.ChapterNum, a.BlockID, a.Excerpt)
			} else {
				fmt.Fprintf(w, "- 第 %d 章 · 段落 %d：%s\n", a.ChapterNum, a.BlockID, a.Excerpt)
			}
		}
		fmt.Fprintln(w)
	}
}
