package httpapi

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"showmethestory/internal/config"
	"showmethestory/internal/fsutil"
	"showmethestory/internal/story"
)

type workflowSnapshot struct {
	State    *story.Progress         `json:"state"`
	Settings *story.ProjectSettings  `json:"settings"`
	Config   *config.Config          `json:"config"`
	Proof    *story.PostProcessState `json:"proof"`
}

type historyEntry struct {
	ID     string    `json:"id"`
	At     time.Time `json:"at"`
	Action string    `json:"action"`
}
type historyRecord struct {
	historyEntry
	Snapshot workflowSnapshot `json:"snapshot"`
}

func historyDir(path string) string { return filepath.Join(filepath.Dir(path), "workflow-history") }

func (h *Handlers) checkpointWorkflow(action string) error {
	if h.progressPath == "" || h.projectName == "" {
		return nil
	}
	snapshot := workflowSnapshot{h.state, h.settings, h.cfg, h.postprocess}
	data, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(data))
	dir := historyDir(h.progressPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	index := filepath.Join(dir, "index.json")
	var list struct {
		Digest  string         `json:"digest"`
		Entries []historyEntry `json:"entries"`
	}
	prior, err := os.ReadFile(index)
	if err == nil {
		if err := json.Unmarshal(prior, &list); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if list.Digest == digest {
		return nil
	}
	now := time.Now().UTC()
	if action == "" {
		action = "Project update"
	}
	entry := historyEntry{ID: fmt.Sprint(now.UnixNano()), At: now, Action: action}
	record := historyRecord{entry, snapshot}
	blob, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if err := fsutil.WriteFileAtomic(filepath.Join(dir, entry.ID+".json"), blob); err != nil {
		return err
	}
	list.Digest = digest
	list.Entries = append(list.Entries, entry)
	blob, err = json.Marshal(list)
	if err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(index, blob)
}

func (h *Handlers) recordWorkflowHistory(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path := r.Method, r.URL.Path
		write := strings.HasPrefix(path, "/api/") && method != http.MethodGet && path != "/api/task/stop"
		if !write || h.isTaskRunning() {
			next.ServeHTTP(w, r)
			return
		}
		if err := h.checkpointWorkflow("Baseline"); err != nil {
			h.writeErrorReq(w, r, 500, "save_failed", err)
			return
		}
		action := method + " " + path
		if path == "/api/story-desk" {
			// The endpoint records an explicit action at the handler.
			action = "Story Desk"
		}
		h.workflowAction = action
		next.ServeHTTP(w, r)
		if !h.isTaskRunning() {
			if err := h.checkpointWorkflow(h.workflowAction); err != nil {
				h.logger.WarnKey("log.knowledge_failed", err)
			}
		}
	})
}

func (h *Handlers) workflowHistory(w http.ResponseWriter, r *http.Request) {
	if !h.ensureProject(w, r) {
		return
	}
	entries := []historyEntry{}
	data, err := os.ReadFile(filepath.Join(historyDir(h.progressPath), "index.json"))
	if err == nil {
		var index struct {
			Entries []historyEntry `json:"entries"`
		}
		if err := json.Unmarshal(data, &index); err != nil {
			h.writeErrorReq(w, r, 500, "save_failed", err)
			return
		}
		entries = index.Entries
	} else if !os.IsNotExist(err) {
		h.writeErrorReq(w, r, 500, "save_failed", err)
		return
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].At.After(entries[j].At) })
	h.writeJSON(w, 200, entries)
}

func (h *Handlers) restoreHistory(w http.ResponseWriter, r *http.Request) {
	if !h.ensureProject(w, r) || h.rejectIfTaskRunning(w, r) {
		return
	}
	id := r.PathValue("id")
	if len(id) != 19 || strings.Trim(id, "0123456789") != "" {
		h.writeErrorReq(w, r, 400, "invalid_request", "Invalid history ID")
		return
	}
	data, err := os.ReadFile(filepath.Join(historyDir(h.progressPath), id+".json"))
	if err != nil {
		h.writeErrorReq(w, r, 404, "invalid_request", "History entry unavailable")
		return
	}
	var record historyRecord
	if err := json.Unmarshal(data, &record); err != nil {
		h.writeErrorReq(w, r, 500, "save_failed", err)
		return
	}
	s := record.Snapshot
	var selection struct {
		Stage   string `json:"stage"`
		Part    int    `json:"part"`
		Chapter int    `json:"chapter"`
		Project string `json:"project"`
	}
	if err := json.NewDecoder(r.Body).Decode(&selection); err != nil {
		h.writeErrorReq(w, r, 400, "invalid_json")
		return
	}
	if selection.Project != h.projectName {
		h.writeErrorReq(w, r, 409, "invalid_request", "Project has changed")
		return
	}
	switch selection.Stage {
	case "all":
	case "proofreading":
		s.State = h.state
		s.Settings = h.settings
		s.Config = h.cfg
	case "settings":
		s.State = h.state
		s.Proof = h.postprocess
	case "draft", "prose", "outline":
		if s.State == nil {
			h.writeErrorReq(w, r, 400, "invalid_request", "Invalid history entry")
			return
		}
		old := s.State
		next := *h.state
		if selection.Stage == "draft" {
			i := selection.Part - 1
			if i < 0 || i >= len(old.Desk.Parts) || i >= len(next.Desk.Parts) {
				h.writeErrorReq(w, r, 404, "invalid_request", "Draft unavailable in this version")
				return
			}
			next.Desk.Parts = slices.Clone(next.Desk.Parts)
			next.Desk.Parts[i].DeskDraft = old.Desk.Parts[i].DeskDraft
			next.Desk.Revision++
		} else {
			i := story.FindChapterIdx(&next, selection.Chapter)
			j := story.FindChapterIdx(old, selection.Chapter)
			if i < 0 || j < 0 || next.Chapters[i].DeskPart != old.Chapters[j].DeskPart {
				h.writeErrorReq(w, r, 404, "chapter_not_found")
				return
			}
			next.Chapters = slices.Clone(next.Chapters)
			current, prior := &next.Chapters[i], old.Chapters[j]
			if selection.Stage == "outline" {
				p := current.DeskPart - 1
				current.Title = prior.Title
				current.Outline = prior.Outline
				current.Characters = prior.Characters
				if p >= 0 && p < len(next.Desk.Parts) && p < len(old.Desk.Parts) {
					next.Desk.Parts = slices.Clone(next.Desk.Parts)
					part := &next.Desk.Parts[p]
					rev := story.DeskVersion{Lines: []string{prior.Outline}, Source: old.Desk.Parts[p].DeskDraft}
					versions := old.Desk.Parts[p].Versions
					if prior.DeskVersion > 0 && prior.DeskVersion <= len(versions) {
						rev = versions[prior.DeskVersion-1]
					}
					part.Versions = append(slices.Clone(part.Versions), rev)
					current.DeskVersion = len(part.Versions)
				}
				current.DeskEarlierProse = current.Content != "" && current.DeskProseVersion != current.DeskVersion
				next.Desk.Revision++
			} else {
				current.Content = prior.Content
				current.Summary = prior.Summary
				current.Status = prior.Status
				if current.Status == story.StatusWriting {
					current.Status = story.StatusPending
					if current.Content != "" {
						current.Status = story.StatusReview
					}
				}
				current.Blocks = prior.Blocks
				current.BlockSep = prior.BlockSep
				current.NextBlockID = prior.NextBlockID
				current.DeskProseVersion = prior.DeskProseVersion
				current.DeskEarlierProse = current.Content != "" && current.DeskProseVersion != current.DeskVersion
				current.MemoryRevision = ""
				current.KnowledgeTracked = current.Content != ""
				next.BookStatus = story.BookStatusActive
				next.PendingWritingConflict = nil
				next.CurrentChapterIndex = len(next.Chapters)
				for k, ch := range next.Chapters {
					if !ch.Inherited && ch.Status != story.StatusAccepted {
						next.CurrentChapterIndex = k
						break
					}
				}
			}
		}
		s = workflowSnapshot{&next, h.settings, h.cfg, h.postprocess}
	default:
		h.writeErrorReq(w, r, 400, "invalid_request", "Unknown restore scope")
		return
	}
	if s.State == nil || s.Config == nil || s.Settings == nil || s.Proof == nil {
		h.writeErrorReq(w, r, 400, "invalid_request", "Invalid history entry")
		return
	}
	if selection.Stage == "all" {
		s.State.Desk.Revision = max(s.State.Desk.Revision, h.state.Desk.Revision) + 1
	}
	if err := story.RestoreWorkflow(h.progressPath, s.State, s.Config, s.Settings, s.Proof); err != nil {
		h.writeErrorReq(w, r, 500, "save_progress_failed", err)
		return
	}
	h.state = s.State
	h.cfg = s.Config
	h.settings = s.Settings
	h.postprocess = s.Proof
	h.reloadSkills()
	for _, ch := range h.state.Chapters {
		if ch.Content != "" {
			story.SaveChapterMarkdown(filepath.Dir(h.progressPath), ch, h.state.Title)
		}
	}
	h.broadcastProgress()
	h.logger.SettingsUpdated()
	h.writeJSON(w, 200, map[string]string{"status": "restored"})
}
