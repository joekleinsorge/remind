package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	_ "time/tzdata"
)

//go:embed web/*
var webFiles embed.FS

// One server owns the data volume. The CLI must not write to it concurrently.
type webApp struct {
	mu            sync.Mutex
	cfg           appConfig
	library       []Clipping
	state         ReminderState
	delivery      deliveryStatus
	libraryPath   string
	emailTime     string
	emailLocation *time.Location
	send          func(string, string, string, string, string, string) error
}

func newWebApp(cfg appConfig) (*webApp, error) {
	if err := os.MkdirAll(filepath.Dir(cfg.StatePath), 0o750); err != nil {
		return nil, err
	}
	state, err := loadReminderState(cfg.StatePath)
	if err != nil {
		return nil, fmt.Errorf("load review state: %w", err)
	}
	a := &webApp{cfg: cfg, state: state, libraryPath: filepath.Join(filepath.Dir(cfg.StatePath), "library.json"), send: sendEmail}
	a.delivery, err = loadDeliveryStatus(cfg.StatePath)
	if err != nil {
		return nil, fmt.Errorf("load delivery status: %w", err)
	}
	content, err := os.ReadFile(a.libraryPath)
	if err == nil {
		if err := json.Unmarshal(content, &a.library); err != nil {
			return nil, fmt.Errorf("load library: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	} else if content, err := os.ReadFile(cfg.ClippingsPath); err == nil {
		a.library = parseClippings(string(content))
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	a.emailTime = os.Getenv("REMIND_EMAIL_TIME")
	zone := os.Getenv("REMIND_TIMEZONE")
	if zone == "" {
		zone = "America/New_York"
	}
	a.emailLocation, err = time.LoadLocation(zone)
	if err != nil {
		return nil, fmt.Errorf("invalid REMIND_TIMEZONE: %w", err)
	}
	if a.emailTime != "" {
		if _, err := time.Parse("15:04", a.emailTime); err != nil {
			return nil, fmt.Errorf("REMIND_EMAIL_TIME must be HH:MM: %w", err)
		}
		if !emailConfigured() {
			return nil, errors.New("daily email requires SENDGRID_API_KEY, SENDER_EMAIL and RECIPIENT_EMAIL")
		}
	}
	return a, nil
}

func serveApp(cfg appConfig, addr string) error {
	a, err := newWebApp(cfg)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	srv := &http.Server{Addr: addr, Handler: a.handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 45 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	go a.schedule(ctx)
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown: %v", err)
		}
	}()
	log.Printf("Remind listening on %s (%d highlights)", addr, len(a.library))
	err = srv.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (a *webApp) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok\n")
	})
	mux.HandleFunc("/api/library", a.getLibrary)
	mux.HandleFunc("/api/import", a.importLibrary)
	mux.HandleFunc("/api/review", a.review)
	mux.HandleFunc("/api/email-preview", a.preview)
	mux.HandleFunc("/api/export", a.export)
	mux.HandleFunc("/api/status", a.status)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		name := "web/index.html"
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
		case "/app.js":
			name = "web/app.js"
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		case "/app.css":
			name = "web/app.css"
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
		default:
			http.NotFound(w, r)
			return
		}
		if !method(w, r, http.MethodGet) {
			return
		}
		b, err := webFiles.ReadFile(name)
		if err != nil {
			http.Error(w, "asset unavailable", 500)
			return
		}
		_, _ = w.Write(b)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; frame-ancestors 'self'; form-action 'self'; base-uri 'none'")
		w.Header().Set("Cache-Control", "no-store")
		// Requests behind the auth proxy preserve Host. Never trust a caller's
		// forwarded headers when deciding whether a browser mutation is same-origin.
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
				http.Error(w, "cross-site request denied", 403)
				return
			}
			if origin := r.Header.Get("Origin"); origin != "" {
				u, err := url.Parse(origin)
				if err != nil || u.Host != r.Host || (u.Scheme != "http" && u.Scheme != "https") {
					http.Error(w, "cross-origin request denied", 403)
					return
				}
			}
			if r.Header.Get("X-Remind-Request") != "1" {
				http.Error(w, "missing request header", 403)
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}

func method(w http.ResponseWriter, r *http.Request, allowed string) bool {
	if r.Method == allowed {
		return true
	}
	w.Header().Set("Allow", allowed)
	http.Error(w, "method not allowed", 405)
	return false
}

func writeJSON(w http.ResponseWriter, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("encode response: %v", err)
	}
}

func (a *webApp) getLibrary(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	type item struct {
		Clipping
		Review HighlightState
		Due    bool
	}
	items := make([]item, 0, len(a.library))
	books := map[string]bool{}
	due := 0
	for _, c := range a.library {
		review, _ := a.state.Review(c)
		isDue := a.state.IsDue(c, now)
		if isDue {
			due++
		}
		books[c.Title] = true
		items = append(items, item{c, review, isDue})
	}
	writeJSON(w, map[string]interface{}{"highlights": items, "books": len(books), "due": due, "dailyCount": a.cfg.Count, "emailConfigured": emailConfigured(), "emailTime": a.emailTime, "timezone": a.emailLocation.String(), "delivery": a.delivery, "session": selectClippings(a.library, a.cfg.Count, a.state, now)})
}

func (a *webApp) importLibrary(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, (10<<20)+(64<<10))
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, "Upload a Kindle clippings file smaller than 10 MB.", 400)
		return
	}
	defer r.MultipartForm.RemoveAll()
	f, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Choose My Clippings.txt from your Kindle.", 400)
		return
	}
	defer f.Close()
	content, err := io.ReadAll(io.LimitReader(f, (10<<20)+1))
	if err != nil || len(content) > 10<<20 {
		http.Error(w, "Could not read upload.", 400)
		return
	}
	imported := parseClippings(string(content))
	if len(imported) == 0 {
		http.Error(w, "No Kindle highlights found. Your library has not changed.", 400)
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	merged := append([]Clipping{}, a.library...)
	seen := map[string]bool{}
	for _, c := range merged {
		seen[c.ID] = true
	}
	added := 0
	for _, c := range imported {
		if !seen[c.ID] {
			merged = append(merged, c)
			seen[c.ID] = true
			added++
		}
	}
	if err := saveJSON(a.libraryPath, merged); err != nil {
		log.Printf("import: %v", err)
		http.Error(w, "Could not save library.", 500)
		return
	}
	a.library = merged
	writeJSON(w, map[string]int{"added": added, "duplicates": len(imported) - added, "total": len(merged)})
}

func (a *webApp) review(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	var input struct {
		ID     string `json:"id"`
		Action string `json:"action"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil || (input.Action != "remembered" && input.Action != "again") {
		http.Error(w, "Expected a highlight id and remembered or again action.", 400)
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, c := range a.library {
		if c.ID != input.ID {
			continue
		}
		if !a.state.IsDue(c, time.Now()) {
			http.Error(w, "This highlight has already been reviewed.", 409)
			return
		}
		next := cloneState(a.state)
		now := time.Now()
		if input.Action == "remembered" {
			next.MarkReviewed([]Clipping{c}, now, reviewIntervals(a.cfg.ReviewIntervalsDays))
		} else {
			s := next.Highlights[c.ID]
			s.Title = c.Title
			s.Author = c.Author
			s.Snippet = highlightSnippet(c.Highlight)
			if s.CreatedAt == "" {
				s.CreatedAt = now.UTC().Format(time.RFC3339)
			}
			s.PendingAt = ""
			s.UpdatedAt = now.UTC().Format(time.RFC3339)
			s.NextDueAt = now.Add(24 * time.Hour).UTC().Format(time.RFC3339)
			next.Highlights[c.ID] = s
		}
		if err := saveReminderState(a.cfg.StatePath, next); err != nil {
			http.Error(w, "Could not save review.", 500)
			return
		}
		a.state = next
		writeJSON(w, next.Highlights[c.ID])
		return
	}
	http.Error(w, "Highlight not found.", 404)
}

func (a *webApp) preview(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	selected := selectClippings(a.library, a.cfg.Count, a.state, time.Now())
	_, _, html, err := buildEmail(selected, func() string { return time.Now().In(a.emailLocation).Format("Monday, January 2, 2006") }, a.cfg.TemplatePath)
	if err != nil {
		http.Error(w, "Could not render email.", 500)
		return
	}
	// Email templates contain inline styles.
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; frame-ancestors 'self'; base-uri 'none'")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, html)
}

func (a *webApp) export(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	w.Header().Set("Content-Disposition", `attachment; filename="remind-export.json"`)
	writeJSON(w, map[string]interface{}{"version": 1, "highlights": a.library, "state": a.state})
}

func (a *webApp) status(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	writeJSON(w, a.delivery)
}

func cloneState(state ReminderState) ReminderState {
	next := ReminderState{Highlights: map[string]HighlightState{}}
	for id, s := range state.Highlights {
		next.Highlights[id] = s
	}
	return next
}

func saveJSON(path string, value interface{}) error {
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".remind-import-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(content); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func emailConfigured() bool {
	return os.Getenv("SENDGRID_API_KEY") != "" && os.Getenv("SENDER_EMAIL") != "" && os.Getenv("RECIPIENT_EMAIL") != ""
}

// Persist the attempted day before delivery: restarts never resend a daily
// digest. An ambiguous delivery must be investigated rather than retried blind.
func (a *webApp) sendDaily(now time.Time) (deliveryErr error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	day := now.In(a.emailLocation).Format("2006-01-02")
	marker := filepath.Join(filepath.Dir(a.cfg.StatePath), "email-day.json")
	var lastDay string
	b, err := os.ReadFile(marker)
	if err == nil {
		if err = json.Unmarshal(b, &lastDay); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if lastDay == day {
		return nil
	}
	selected := selectClippings(a.library, a.cfg.Count, a.state, now)
	if len(selected) == 0 {
		return nil
	}
	subject, plain, html, err := buildEmail(selected, func() string { return now.In(a.emailLocation).Format("Monday, January 2, 2006") }, a.cfg.TemplatePath)
	if err != nil {
		return err
	}
	a.delivery.begin(selected, now)
	statusPath := deliveryStatusPath(a.cfg.StatePath)
	if err = saveJSON(statusPath, a.delivery); err != nil {
		return fmt.Errorf("save delivery status: %w", err)
	}
	accepted := false
	defer func() {
		outcome := "failed"
		if accepted {
			outcome = "accepted"
			if deliveryErr != nil {
				outcome = "accepted_state_error"
			}
		}
		a.delivery.finish(outcome, time.Now())
		if err := saveJSON(statusPath, a.delivery); err != nil {
			deliveryErr = errors.Join(deliveryErr, fmt.Errorf("save delivery outcome: %w", err))
		}
	}()
	if err = saveJSON(marker, day); err != nil {
		return err
	}
	next := cloneState(a.state)
	next.MarkPending(selected, now)
	if err = saveReminderState(a.cfg.StatePath, next); err != nil {
		return err
	}
	a.state = next
	if err = a.send(os.Getenv("SENDGRID_API_KEY"), os.Getenv("SENDER_EMAIL"), os.Getenv("RECIPIENT_EMAIL"), subject, plain, html); err != nil {
		next.MarkFailed(selected, now)
		if saveErr := saveReminderState(a.cfg.StatePath, next); saveErr != nil {
			return fmt.Errorf("delivery failed (%v), saving failure state: %w", err, saveErr)
		}
		a.state = next
		return err
	}
	accepted = true
	next.MarkReviewed(selected, now, reviewIntervals(a.cfg.ReviewIntervalsDays))
	if err = saveReminderState(a.cfg.StatePath, next); err != nil {
		return err
	}
	a.state = next
	return nil
}

func (a *webApp) schedule(ctx context.Context) {
	if a.emailTime == "" {
		return
	}
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if now.In(a.emailLocation).Format("15:04") >= a.emailTime {
				if err := a.sendDaily(now); err != nil {
					log.Printf("daily email: %v", err)
				}
			}
		}
	}
}
