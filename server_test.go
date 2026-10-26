package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func testWebApp(t *testing.T) *webApp {
	t.Helper()
	t.Setenv("REMIND_EMAIL_TIME", "")
	t.Setenv("REMIND_TIMEZONE", "America/New_York")
	cfg := defaultAppConfig()
	cfg.ClippingsPath = filepath.Join(t.TempDir(), "missing.txt")
	cfg.StatePath = filepath.Join(t.TempDir(), "state.json")
	a, err := newWebApp(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func webRequest(a *webApp, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("X-Remind-Request", "1")
	w := httptest.NewRecorder()
	a.handler().ServeHTTP(w, r)
	return w
}

func upload(t *testing.T, a *webApp, content string) *httptest.ResponseRecorder {
	t.Helper()
	var b bytes.Buffer
	m := multipart.NewWriter(&b)
	f, err := m.CreateFormFile("file", "My Clippings.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err = m.Close(); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/api/import", &b)
	r.Header.Set("Content-Type", m.FormDataContentType())
	r.Header.Set("X-Remind-Request", "1")
	w := httptest.NewRecorder()
	a.handler().ServeHTTP(w, r)
	return w
}

const testClipping = "A Book (An Author)\n- Your Highlight on page 1 | Location 1-2 | Added on Monday, January 1, 2024 12:00:00 PM\n\nA thought worth remembering.\n==========\n"

func TestWebImportReviewAndRestart(t *testing.T) {
	a := testWebApp(t)
	if got := webRequest(a, "GET", "/api/library", ""); got.Code != 200 || !strings.Contains(got.Body.String(), `"highlights":[]`) {
		t.Fatalf("empty library: %s", got.Body)
	}
	if w := upload(t, a, testClipping); w.Code != 200 {
		t.Fatalf("import: %d %s", w.Code, w.Body)
	}
	if len(a.library) != 1 {
		t.Fatalf("highlights: %d", len(a.library))
	}
	id := a.library[0].ID
	if w := webRequest(a, "POST", "/api/review", fmt.Sprintf(`{"id":%q,"action":"remembered"}`, id)); w.Code != 200 {
		t.Fatalf("review: %s", w.Body)
	}
	if w := webRequest(a, "POST", "/api/review", fmt.Sprintf(`{"id":%q,"action":"remembered"}`, id)); w.Code != 409 {
		t.Fatalf("duplicate review accepted: %d", w.Code)
	}
	if w := upload(t, a, testClipping); w.Code != 200 || !strings.Contains(w.Body.String(), `"added":0`) {
		t.Fatalf("duplicate import: %s", w.Body)
	}
	b, err := newWebApp(a.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.library) != 1 || b.state.Highlights[id].SendCount != 1 || b.state.IsDue(b.library[0], time.Now()) {
		t.Fatal("restart lost import or review state")
	}
	due, _ := time.Parse(time.RFC3339, b.state.Highlights[id].NextDueAt)
	if d := time.Until(due); d < 6*24*time.Hour || d > 7*24*time.Hour {
		t.Fatalf("first review interval: %s", d)
	}
	if w := webRequest(b, "GET", "/api/export", ""); w.Code != 200 || !strings.Contains(w.Body.String(), id) {
		t.Fatal("export missing persisted data")
	}
}

func TestWebImportFailureKeepsLibrary(t *testing.T) {
	a := testWebApp(t)
	upload(t, a, testClipping)
	before, err := os.ReadFile(a.libraryPath)
	if err != nil {
		t.Fatal(err)
	}
	if w := upload(t, a, "this is not a Kindle export"); w.Code != 400 {
		t.Fatalf("invalid import accepted: %s", w.Body)
	}
	after, _ := os.ReadFile(a.libraryPath)
	if !bytes.Equal(before, after) || len(a.library) != 1 {
		t.Fatal("failed import changed library")
	}
	// A storage failure must not update memory or report success.
	a.libraryPath = filepath.Join(a.libraryPath, "invalid", "library.json")
	if w := upload(t, a, strings.ReplaceAll(testClipping, "A thought worth remembering.", "Another thought.")); w.Code != 500 {
		t.Fatalf("write failure: %d", w.Code)
	}
	if len(a.library) != 1 {
		t.Fatal("failed write updated in-memory library")
	}
}

func TestWebAgainDoesNotAdvanceInterval(t *testing.T) {
	a := testWebApp(t)
	upload(t, a, testClipping)
	id := a.library[0].ID
	w := webRequest(a, "POST", "/api/review", fmt.Sprintf(`{"id":%q,"action":"again"}`, id))
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	s := a.state.Highlights[id]
	due, _ := time.Parse(time.RFC3339, s.NextDueAt)
	if s.SendCount != 0 || time.Until(due) < 23*time.Hour || time.Until(due) > 24*time.Hour {
		t.Fatalf("again state: %+v", s)
	}
}

func TestWebRejectsCrossOriginAndInvalidRequests(t *testing.T) {
	a := testWebApp(t)
	for _, headers := range []map[string]string{
		{},
		{"X-Remind-Request": "1", "Origin": "https://attacker.test"},
		{"X-Remind-Request": "1", "Sec-Fetch-Site": "cross-site"},
	} {
		r := httptest.NewRequest("POST", "http://remind.test/api/review", strings.NewReader(`{}`))
		for key, value := range headers {
			r.Header.Set(key, value)
		}
		w := httptest.NewRecorder()
		a.handler().ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatalf("accepted cross-site request: %v: %d", headers, w.Code)
		}
	}
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{
		{"POST", "/api/review", `{"id":"x","action":"invalid"}`, 400},
		{"POST", "/api/review", `{"id":"x","action":"remembered"}`, 404},
		{"GET", "/api/review", "", 405},
		{"GET", "/missing", "", 404},
		{"GET", "/healthz", "", 200},
		{"GET", "/", "", 200},
		{"GET", "/app.js", "", 200},
		{"GET", "/app.css", "", 200},
	} {
		if w := webRequest(a, tc.method, tc.path, tc.body); w.Code != tc.status {
			t.Fatalf("%s %s: %d, want %d", tc.method, tc.path, w.Code, tc.status)
		}
	}
}

func TestWebPreviewDoesNotChangeReviewState(t *testing.T) {
	a := testWebApp(t)
	upload(t, a, testClipping)
	w := webRequest(a, "GET", "/api/email-preview", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "A thought worth remembering.") {
		t.Fatalf("preview: %s", w.Body)
	}
	if len(a.state.Highlights) != 0 {
		t.Fatal("preview changed review state")
	}
}

func TestWebConcurrentReviewsAreIdempotent(t *testing.T) {
	a := testWebApp(t)
	upload(t, a, testClipping)
	id := a.library[0].ID
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			webRequest(a, "POST", "/api/review", fmt.Sprintf(`{"id":%q,"action":"remembered"}`, id))
		}()
	}
	wg.Wait()
	if a.state.Highlights[id].SendCount != 1 {
		t.Fatal("concurrent requests advanced schedule more than once")
	}
}

func TestDailyDeliveryIsNotRepeatedAfterRestart(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			a := testWebApp(t)
			upload(t, a, testClipping)
			calls := 0
			a.send = func(string, string, string, string, string, string) error {
				calls++
				if fail {
					return errors.New("delivery failed")
				}
				return nil
			}
			now := time.Now()
			err := a.sendDaily(now)
			if (err != nil) != fail {
				t.Fatalf("delivery error: %v", err)
			}
			if err := a.sendDaily(now); err != nil {
				t.Fatal(err)
			}
			b, err := newWebApp(a.cfg)
			if err != nil {
				t.Fatal(err)
			}
			b.send = a.send
			if err = b.sendDaily(now); err != nil {
				t.Fatal(err)
			}
			if calls != 1 {
				t.Fatalf("sent %d times", calls)
			}
			var state map[string]interface{}
			if w := webRequest(b, "GET", "/api/library", ""); json.Unmarshal(w.Body.Bytes(), &state) != nil {
				t.Fatal("invalid library response")
			}
		})
	}
}
