package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"showmethestory/internal/config"
	"showmethestory/internal/sse"
	"showmethestory/internal/story"
)

func TestWorkflowDeleteProseAndRestoreDoesNotTouchOutlineOrProof(t *testing.T) {
	root := t.TempDir()
	h := NewHandlers(&config.APIConfig{}, "", sse.NewLogBroadcaster(), root, "test")
	h.projectName = "Workflow"
	h.progressPath = filepath.Join(root, "progress.json")
	h.postprocessPath = filepath.Join(root, "postprocess.json")
	ch := story.ChapterState{Num: 1, Title: "First", Outline: "Outline v1", DeskPart: 1, DeskVersion: 1, DeskProseVersion: 1, Content: "The original prose.", Status: story.StatusAccepted}
	h.state.Chapters = []story.ChapterState{ch, {Num: 2, Title: "Second", Outline: "Next outline", Content: "The second chapter.", Status: story.StatusAccepted}}
	h.state.Desk.Parts = []story.DeskPart{{DeskDraft: story.DeskDraft{Beats: "Enter the station"}}}
	h.state.BookStatus = story.BookStatusCompleted
	if err := story.SaveProgress(h.progressPath, h.state); err != nil {
		t.Fatal(err)
	}
	backup := httptest.NewRecorder()
	h.PostProofreadBackup(backup, httptest.NewRequest("POST", "/api/proofread/backup", nil))
	if backup.Code != 200 {
		t.Fatal(backup.Body.String())
	}
	if err := h.checkpointWorkflow("Baseline"); err != nil {
		t.Fatal(err)
	}
	ls := httptest.NewRecorder()
	h.workflowHistory(ls, httptest.NewRequest("GET", "/api/history", nil))
	var entries []historyEntry
	if err := json.Unmarshal(ls.Body.Bytes(), &entries); err != nil || len(entries) != 1 {
		t.Fatalf("%s %v", ls.Body.String(), err)
	}
	request := func(method string) *http.Request {
		r := httptest.NewRequest(method, "/api/chapters/1", nil)
		r.SetPathValue("num", "1")
		return r
	}
	preview := httptest.NewRecorder()
	h.chapterRemoval(preview, request("POST"))
	var token struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(preview.Body.Bytes(), &token); err != nil || token.Token == "" {
		t.Fatal(preview.Body.String())
	}
	remove := request("DELETE")
	remove.Header.Set("If-Match", token.Token)
	result := httptest.NewRecorder()
	h.chapterRemoval(result, remove)
	if result.Code != 200 {
		t.Fatal(result.Body.String())
	}
	if h.state.Chapters[0].Content != "" || h.state.Chapters[0].Outline != "Outline v1" || h.state.Chapters[1].Num != 2 || h.postprocess.Copy.Chapters[0].Content != ch.Content {
		t.Fatalf("deletion escaped its stage: %+v", h.state.Chapters)
	}
	h.state.Chapters[0].Outline = "Outline v2"
	restore := httptest.NewRequest("POST", "/api/history/"+entries[0].ID, strings.NewReader(`{"stage":"prose","chapter":1,"project":"Workflow"}`))
	restore.SetPathValue("id", entries[0].ID)
	rec := httptest.NewRecorder()
	h.restoreHistory(rec, restore)
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	durable, err := story.LoadProgress(h.progressPath)
	if err != nil || durable.Chapters[0].Content != ch.Content || durable.Chapters[0].Outline != "Outline v2" {
		t.Fatalf("restore crossed workspace boundaries: %+v %v", durable, err)
	}
	r := httptest.NewRequest("PUT", "/api/proofread/chapters/1/blocks/1", strings.NewReader(`{"text":"Copyedit the first sentence."}`))
	r.SetPathValue("num", "1")
	r.SetPathValue("id", "1")
	rec = httptest.NewRecorder()
	h.PutProofreadBlock(rec, r)
	if rec.Code != 200 || h.state.Chapters[0].Content != ch.Content || h.postprocess.Copy.Chapters[0].Content == ch.Content {
		t.Fatalf("proofread isolation failed: %s", rec.Body.String())
	}
}
