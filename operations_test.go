package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestImportClientMergesThroughRunningServer(t *testing.T) {
	a := testWebApp(t)
	srv := httptest.NewServer(a.handler())
	defer srv.Close()
	for i, wantAdded := range []int{1, 0} {
		result, err := importClippings(strings.NewReader(testClipping), srv.URL+"/api/import")
		if err != nil || result.Added != wantAdded || result.Total != 1 || result.Duplicates != i {
			t.Fatalf("import %d: %+v, %v", i, result, err)
		}
	}
	b, err := newWebApp(a.cfg)
	if err != nil || len(b.library) != 1 {
		t.Fatalf("import did not survive restart: %v", err)
	}
	if _, err := importClippings(strings.NewReader("invalid export"), srv.URL+"/api/import"); err == nil {
		t.Fatal("invalid clippings reported as a successful import")
	}
	if len(a.library) != 1 {
		t.Fatal("rejected import changed library")
	}
}

func TestImportClientRejectsOversizeFilesAndRedirects(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
	}))
	defer srv.Close()
	if _, err := importClippings(strings.NewReader(strings.Repeat("x", (10<<20)+1)), srv.URL); err == nil || calls != 0 {
		t.Fatal("oversized file reached server")
	}
	if _, err := importClippings(strings.NewReader(testClipping), srv.URL); err == nil || calls != 1 {
		t.Fatal("import followed authentication redirect or reported success")
	}
}

func TestDeliveryStatusPersistsSuccessAndFailure(t *testing.T) {
	a := testWebApp(t)
	upload(t, a, testClipping)
	now := time.Now()
	a.send = func(string, string, string, string, string, string) error { return nil }
	if err := a.sendDaily(now); err != nil {
		t.Fatal(err)
	}
	firstSuccess := a.delivery.LastSuccessAt
	id := a.library[0].ID
	if a.delivery.Outcome != "accepted" || firstSuccess == "" || len(a.delivery.SelectedIDs) != 1 || a.delivery.SelectedIDs[0] != id {
		t.Fatalf("missing acceptance details: %+v", a.delivery)
	}
	a.send = func(string, string, string, string, string, string) error { return errors.New("provider failed") }
	if err := a.sendDaily(now.Add(8 * 24 * time.Hour)); err == nil {
		t.Fatal("provider error was lost")
	}
	b, err := newWebApp(a.cfg)
	if err != nil {
		t.Fatal(err)
	}
	w := webRequest(b, "GET", "/api/status", "")
	var status deliveryStatus
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &status) != nil {
		t.Fatalf("invalid status response: %s", w.Body)
	}
	if status.Outcome != "failed" || status.LastSuccessAt != firstSuccess || status.LastSuccessIDs[0] != id || status.FinishedAt == "" {
		t.Fatalf("restart lost delivery history: %+v", status)
	}
	if w := webRequest(b, "POST", "/api/status", ""); w.Code != 405 {
		t.Fatalf("status endpoint accepted mutation: %d", w.Code)
	}
}

func TestDeliveryStatusDistinguishesAcceptedButUnsavedState(t *testing.T) {
	a := testWebApp(t)
	upload(t, a, testClipping)
	originalStatePath := a.cfg.StatePath
	a.send = func(string, string, string, string, string, string) error {
		a.cfg.StatePath = filepath.Join(t.TempDir(), "missing", "state.json")
		return nil
	}
	if err := a.sendDaily(time.Now()); err == nil {
		t.Fatal("missing final state write error")
	}
	status, err := loadDeliveryStatus(originalStatePath)
	if err != nil || status.Outcome != "accepted_state_error" || status.LastSuccessAt == "" {
		t.Fatalf("acceptance was incorrectly reported: %+v, %v", status, err)
	}
}

func TestDeliveryStatusWriteFailurePreventsSend(t *testing.T) {
	a := testWebApp(t)
	upload(t, a, testClipping)
	if err := os.Mkdir(deliveryStatusPath(a.cfg.StatePath), 0o700); err != nil {
		t.Fatal(err)
	}
	calls := 0
	a.send = func(string, string, string, string, string, string) error { calls++; return nil }
	if err := a.sendDaily(time.Now()); err == nil || calls != 0 {
		t.Fatalf("sent without durable attempt status: calls=%d err=%v", calls, err)
	}
}

func TestPreviewDoesNotCreateDeliveryStatus(t *testing.T) {
	a := testWebApp(t)
	upload(t, a, testClipping)
	if w := webRequest(a, "GET", "/api/email-preview", ""); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if _, err := os.Stat(deliveryStatusPath(a.cfg.StatePath)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("preview created delivery status: %v", err)
	}
}
