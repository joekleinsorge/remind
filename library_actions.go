package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type reviewUndo struct {
	Token         string
	Before, After HighlightState
	Existed       bool
}

func decodeRequest(w http.ResponseWriter, r *http.Request, limit int64, value interface{}) bool {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(value); err != nil {
		http.Error(w, "Invalid or oversized JSON file.", 400)
		return false
	}
	if err := decoder.Decode(new(interface{})); err != io.EOF {
		http.Error(w, "Expected one JSON object.", 400)
		return false
	}
	return true
}

func (a *webApp) undoReview(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	var input struct {
		ID    string `json:"id"`
		Token string `json:"token"`
	}
	if !decodeRequest(w, r, 4096, &input) {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.restorePending {
		http.Error(w, "Restore needs recovery. Restart Remind.", 503)
		return
	}
	undo, ok := a.undo[input.ID]
	if !ok || undo.Token != input.Token || a.state.Highlights[input.ID] != undo.After {
		http.Error(w, "This review has changed. Refresh your library before continuing.", 409)
		return
	}
	next := cloneState(a.state)
	if undo.Existed {
		next.Highlights[input.ID] = undo.Before
	} else {
		delete(next.Highlights, input.ID)
	}
	if err := saveReminderState(a.cfg.StatePath, next); err != nil {
		http.Error(w, "Could not undo review.", 500)
		return
	}
	a.state = next
	delete(a.undo, input.ID)
	writeJSON(w, map[string]bool{"undone": true})
}

func (a *webApp) curate(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	var input struct {
		ID       string  `json:"id"`
		Favorite *bool   `json:"favorite"`
		Archived *bool   `json:"archived"`
		Note     *string `json:"note"`
	}
	if !decodeRequest(w, r, 32<<10, &input) {
		return
	}
	if (input.Favorite == nil && input.Archived == nil && input.Note == nil) || (input.Note != nil && len(*input.Note) > 10000) {
		http.Error(w, "Choose a change; notes can contain up to 10,000 bytes.", 400)
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.restorePending {
		http.Error(w, "Restore needs recovery. Restart Remind.", 503)
		return
	}
	for _, c := range a.library {
		if c.ID != input.ID {
			continue
		}
		next := cloneState(a.state)
		review := next.Highlights[c.ID]
		if input.Favorite != nil {
			review.Favorite = *input.Favorite
		}
		if input.Archived != nil {
			review.Archived = *input.Archived
		}
		if input.Note != nil {
			review.Note = *input.Note
		}
		next.Highlights[c.ID] = review
		if err := saveReminderState(a.cfg.StatePath, next); err != nil {
			http.Error(w, "Could not save highlight.", 500)
			return
		}
		a.state = next
		writeJSON(w, review)
		return
	}
	http.Error(w, "Highlight not found.", 404)
}

type libraryBackup struct {
	Version    int           `json:"version"`
	Highlights []Clipping    `json:"highlights"`
	State      ReminderState `json:"state"`
}

func validateBackup(backup libraryBackup) error {
	if backup.Version != 1 || backup.Highlights == nil || backup.State.Highlights == nil {
		return errors.New("Choose a version 1 Remind JSON export with highlights and review state.")
	}
	ids := map[string]bool{}
	for _, c := range backup.Highlights {
		if strings.TrimSpace(c.Highlight) == "" || c.ID != clippingID(c) || ids[c.ID] {
			return errors.New("Backup contains invalid or duplicate highlights.")
		}
		ids[c.ID] = true
	}
	for _, s := range backup.State.Highlights {
		if s.SendCount < 0 || s.EmailCount < 0 || len(s.Note) > 10000 {
			return errors.New("Backup contains invalid review state.")
		}
		for _, date := range []string{s.LastSentAt, s.NextDueAt, s.PendingAt, s.CreatedAt, s.UpdatedAt, s.LastEmailedAt} {
			if date != "" {
				if _, err := time.Parse(time.RFC3339, date); err != nil {
					return errors.New("Backup contains an invalid review date.")
				}
			}
		}
	}
	return nil
}

func restoreJournalPath(statePath string) string {
	return filepath.Join(filepath.Dir(statePath), "restore-pending.json")
}

// A durable journal makes the two-file merge recoverable after interruption.
// No other mutations may run until the journal has been fully applied.
func recoverRestore(statePath string) error {
	path := restoreJournalPath(statePath)
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var backup libraryBackup
	if err = json.Unmarshal(content, &backup); err != nil {
		return err
	}
	if err = validateBackup(backup); err != nil {
		return err
	}
	if err = saveJSON(filepath.Join(filepath.Dir(statePath), "library.json"), backup.Highlights); err != nil {
		return err
	}
	if err = saveReminderState(statePath, backup.State); err != nil {
		return err
	}
	return os.Remove(path)
}

func (a *webApp) restore(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	var backup libraryBackup
	if !decodeRequest(w, r, 50<<20, &backup) {
		return
	}
	if err := validateBackup(backup); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.restorePending {
		http.Error(w, "Restore needs recovery. Restart Remind.", 503)
		return
	}
	merged := libraryBackup{Version: 1, Highlights: append([]Clipping{}, a.library...), State: cloneState(a.state)}
	ids := map[string]bool{}
	for _, c := range merged.Highlights {
		ids[c.ID] = true
	}
	added, reviews := 0, 0
	for _, c := range backup.Highlights {
		if !ids[c.ID] {
			merged.Highlights = append(merged.Highlights, c)
			ids[c.ID] = true
			added++
		}
		if _, exists := merged.State.Highlights[c.ID]; !exists {
			if state, ok := backup.State.Highlights[c.ID]; ok {
				merged.State.Highlights[c.ID] = state
				reviews++
			}
		}
	}
	if err := saveJSON(restoreJournalPath(a.cfg.StatePath), merged); err != nil {
		http.Error(w, "Could not save restore. Your library has not changed.", 500)
		return
	}
	a.restorePending = true
	if err := recoverRestore(a.cfg.StatePath); err != nil {
		http.Error(w, fmt.Sprintf("Restore is pending recovery. Fix storage and restart Remind: %v", err), 503)
		return
	}
	a.library = merged.Highlights
	a.state = merged.State
	a.undo = nil
	a.restorePending = false
	writeJSON(w, map[string]int{"added": added, "reviews": reviews, "total": len(merged.Highlights)})
}
