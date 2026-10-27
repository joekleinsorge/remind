package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUndoRestoresScheduleAndRejectsStaleRequests(t *testing.T) {
	a := testWebApp(t)
	upload(t, a, testClipping)
	id := a.library[0].ID
	review := webRequest(a, "POST", "/api/review", `{"id":"`+id+`","action":"remembered"}`)
	var result struct {
		UndoToken string `json:"undoToken"`
	}
	if err := json.Unmarshal(review.Body.Bytes(), &result); err != nil || result.UndoToken == "" {
		t.Fatalf("missing undo: %s", review.Body)
	}
	body := `{"id":"` + id + `","token":"` + result.UndoToken + `"}`
	if w := webRequest(a, "POST", "/api/undo", body); w.Code != 200 {
		t.Fatal(w.Body)
	}
	if _, exists := a.state.Highlights[id]; exists {
		t.Fatal("undo did not restore absence of review")
	}
	if w := webRequest(a, "POST", "/api/undo", body); w.Code != 409 {
		t.Fatal("replayed undo accepted")
	}
	review = webRequest(a, "POST", "/api/review", `{"id":"`+id+`","action":"again"}`)
	json.Unmarshal(review.Body.Bytes(), &result)
	webRequest(a, "POST", "/api/curate", `{"id":"`+id+`","note":"New thought"}`)
	if w := webRequest(a, "POST", "/api/undo", `{"id":"`+id+`","token":"`+result.UndoToken+`"}`); w.Code != 409 {
		t.Fatal("undo overwrote newer edits")
	}
	if a.state.Highlights[id].Note != "New thought" {
		t.Fatal("lost note")
	}
}

func TestCurationPersistsAndExcludesArchivedFromSessionsAndEmail(t *testing.T) {
	a := testWebApp(t)
	upload(t, a, testClipping)
	id := a.library[0].ID
	w := webRequest(a, "POST", "/api/curate", `{"id":"`+id+`","favorite":true,"archived":true,"note":"My interpretation"}`)
	if w.Code != 200 {
		t.Fatal(w.Body)
	}
	b, err := newWebApp(a.cfg)
	if err != nil {
		t.Fatal(err)
	}
	s := b.state.Highlights[id]
	if !s.Favorite || !s.Archived || s.Note != "My interpretation" {
		t.Fatalf("lost curation: %+v", s)
	}
	if len(selectClippings(b.library, 5, b.state, time.Now())) != 0 || len(selectEmailClippings(b.library, 5, b.state, time.Now())) != 0 {
		t.Fatal("archived highlight selected")
	}
	webRequest(b, "POST", "/api/curate", `{"id":"`+id+`","archived":false}`)
	if !b.state.IsDue(b.library[0], time.Now()) {
		t.Fatal("unarchived highlight remains hidden")
	}
}

func TestDeliveryDoesNotReviewAndRotatesPassages(t *testing.T) {
	a := testWebApp(t)
	upload(t, a, testClipping+strings.ReplaceAll(testClipping, "A thought worth remembering.", "A second thought."))
	a.cfg.Count = 1
	selected := selectEmailClippings(a.library, 1, a.state, time.Now())[0]
	a.send = func(string, string, string, string, string, string) error { return nil }
	if err := a.sendDaily(time.Now()); err != nil {
		t.Fatal(err)
	}
	s := a.state.Highlights[selected.ID]
	if s.SendCount != 0 || s.NextDueAt != "" || s.LastSentAt != "" || s.EmailCount != 1 || s.LastEmailedAt == "" || !a.state.IsDue(selected, time.Now()) {
		t.Fatalf("delivery advanced review: %+v", s)
	}
	if next := selectEmailClippings(a.library, 1, a.state, time.Now())[0]; next.ID == selected.ID {
		t.Fatal("email failed to rotate passages")
	}
}

func TestImportReportsMalformedUnsupportedAndFileDuplicates(t *testing.T) {
	a := testWebApp(t)
	content := testClipping + testClipping + "invalid record\n==========\n" + strings.ReplaceAll(testClipping, "Your Highlight", "Your Note")
	w := upload(t, a, content)
	var got importResult
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &got) != nil || got.Added != 1 || got.Duplicates != 1 || got.Malformed != 1 || got.Unsupported != 1 {
		t.Fatalf("incorrect report: %s", w.Body)
	}
}

func TestExportRestoreRoundTripMergeAndValidation(t *testing.T) {
	source := testWebApp(t)
	upload(t, source, testClipping)
	id := source.library[0].ID
	webRequest(source, "POST", "/api/curate", `{"id":"`+id+`","favorite":true,"note":"Keep this"}`)
	webRequest(source, "POST", "/api/review", `{"id":"`+id+`","action":"remembered"}`)
	backup := webRequest(source, "GET", "/api/export", "").Body.String()
	target := testWebApp(t)
	if w := webRequest(target, "POST", "/api/restore", backup); w.Code != 200 {
		t.Fatal(w.Body)
	}
	restarted, err := newWebApp(target.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(restarted.library) != 1 || restarted.state.Highlights[id] != source.state.Highlights[id] {
		t.Fatal("restore round trip lost data")
	}
	webRequest(restarted, "POST", "/api/curate", `{"id":"`+id+`","note":"Newer note"}`)
	upload(t, restarted, strings.ReplaceAll(testClipping, "A thought worth remembering.", "Another thought."))
	if w := webRequest(restarted, "POST", "/api/restore", backup); w.Code != 200 || len(restarted.library) != 2 || restarted.state.Highlights[id].Note != "Newer note" {
		t.Fatal("merge overwrote existing data")
	}
	for _, invalid := range []string{`{}`, backup + `{}`, strings.Replace(backup, `"version":1`, `"version":2`, 1), strings.Replace(backup, id, "bad-id", 1)} {
		if w := webRequest(restarted, "POST", "/api/restore", invalid); w.Code != 400 {
			t.Fatalf("invalid backup accepted: %d %s", w.Code, w.Body)
		}
	}
	if len(restarted.library) != 2 {
		t.Fatal("invalid restore changed library")
	}
}

func TestRestoreJournalRecoversAcrossRestart(t *testing.T) {
	a := testWebApp(t)
	upload(t, a, testClipping)
	backup := libraryBackup{Version: 1, Highlights: a.library, State: cloneState(a.state)}
	backup.State.Highlights[a.library[0].ID] = HighlightState{Favorite: true, Note: "Recovered"}
	if err := saveJSON(restoreJournalPath(a.cfg.StatePath), backup); err != nil {
		t.Fatal(err)
	}
	// Simulate an interrupted restore after the library write but before state.
	if err := os.Remove(a.libraryPath); err != nil {
		t.Fatal(err)
	}
	b, err := newWebApp(a.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.library) != 1 || b.state.Highlights[b.library[0].ID].Note != "Recovered" {
		t.Fatal("incomplete restore was lost")
	}
	if _, err := os.Stat(restoreJournalPath(a.cfg.StatePath)); !os.IsNotExist(err) {
		t.Fatal("completed journal was not removed")
	}
}

func TestRestoreWriteFailureBlocksFurtherMutationUntilRecovery(t *testing.T) {
	source := testWebApp(t)
	upload(t, source, testClipping)
	backup := webRequest(source, "GET", "/api/export", "").Body.String()
	a := testWebApp(t)
	// Library persistence succeeds but review persistence fails.
	if err := os.Mkdir(a.cfg.StatePath, 0700); err != nil {
		t.Fatal(err)
	}
	if w := webRequest(a, "POST", "/api/restore", backup); w.Code != 503 {
		t.Fatalf("expected recoverable failure: %s", w.Body)
	}
	if w := upload(t, a, testClipping); w.Code != 503 {
		t.Fatal("mutation allowed during pending recovery")
	}
	if err := os.Remove(a.cfg.StatePath); err != nil {
		t.Fatal(err)
	}
	b, err := newWebApp(a.cfg)
	if err != nil || len(b.library) != 1 {
		t.Fatalf("restart failed to recover: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(a.cfg.StatePath), "restore-pending.json")); !os.IsNotExist(err) {
		t.Fatal("journal remains")
	}
}

func TestFavoritePriorityAndUndoWriteFailure(t *testing.T) {
	a := testWebApp(t)
	upload(t, a, testClipping+strings.ReplaceAll(testClipping, "A thought worth remembering.", "A favorite passage."))
	id := a.library[1].ID
	webRequest(a, "POST", "/api/curate", `{"id":"`+id+`","favorite":true}`)
	if got := selectClippings(a.library, 1, a.state, time.Now())[0]; got.ID != id {
		t.Fatal("favorite did not receive priority")
	}
	before := a.state.Highlights[id]
	review := webRequest(a, "POST", "/api/review", `{"id":"`+id+`","action":"remembered"}`)
	var result struct {
		Token string `json:"undoToken"`
	}
	json.Unmarshal(review.Body.Bytes(), &result)
	statePath := a.cfg.StatePath
	a.cfg.StatePath = filepath.Join(t.TempDir(), "missing", "state.json")
	body := `{"id":"` + id + `","token":"` + result.Token + `"}`
	if w := webRequest(a, "POST", "/api/undo", body); w.Code != 500 || a.state.Highlights[id].SendCount != 1 {
		t.Fatal("failed undo changed memory")
	}
	a.cfg.StatePath = statePath
	if w := webRequest(a, "POST", "/api/undo", body); w.Code != 200 || a.state.Highlights[id] != before {
		t.Fatal("retry did not restore prior curation and schedule")
	}
}

func TestOversizeClippingLineDoesNotPartiallyImport(t *testing.T) {
	a := testWebApp(t)
	if w := upload(t, a, testClipping+strings.Repeat("x", (1<<20)+1)); w.Code != 400 || len(a.library) != 0 {
		t.Fatal("truncated file partially imported")
	}
}
