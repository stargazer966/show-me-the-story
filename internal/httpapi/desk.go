package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"showmethestory/internal/fsutil"
	"showmethestory/internal/story"
)

func (h *Handlers) storyDesk(w http.ResponseWriter, r *http.Request) {
	var req story.DeskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeErrorReq(w, r, 400, "invalid_request_body")
		return
	}
	if !h.ensureProject(w, r) {
		return
	}
	if !h.tryStartTask() {
		h.writeErrorReq(w, r, 409, "task_running_wait")
		return
	}
	defer h.endTask()
	h.workflowAction = fmt.Sprintf("Story Desk · %s · part %d", req.Action, req.Part)
	if h.projectName != req.Project {
		h.writeErrorReq(w, r, 409, "invalid_request", "Project changed")
		return
	}
	ctx, cancel := context.WithCancel(h.taskCtx)
	defer cancel()
	stop := context.AfterFunc(r.Context(), cancel)
	defer stop()
	if req.Action == "generate" {
		h.logger.TaskStart("story_desk")
	}
	result, err := story.RunDesk(ctx, h.progressPath, h.state, h.cfg, h.settings, h.skills, h.apiCfg, req)
	if req.Action == "generate" {
		h.logger.TaskEnd("story_desk", err == nil)
	}
	if err != nil {
		var save *fsutil.SaveError
		if errors.As(err, &save) {
			h.writeErrorReq(w, r, 500, "save_progress_failed", err)
			return
		}
		code := 400
		if errors.Is(err, story.ErrDeskChanged) {
			code = 409
		}
		h.writeErrorReq(w, r, code, "invalid_request", err.Error())
		return
	}
	if req.Action == "use" || req.Action == "generate" || req.Action == "manual_outline" {
		h.broadcastProgress()
	}
	h.writeJSON(w, 200, result)
}
