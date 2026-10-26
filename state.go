package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

type ReminderState struct {
	Highlights map[string]HighlightState `json:"highlights"`
}

type HighlightState struct {
	LastSentAt string `json:"last_sent_at,omitempty"`
	NextDueAt  string `json:"next_due_at,omitempty"`
	PendingAt  string `json:"pending_at,omitempty"`
	CreatedAt  string `json:"created_at,omitempty"`
	UpdatedAt  string `json:"updated_at,omitempty"`
	Title      string `json:"title,omitempty"`
	Author     string `json:"author,omitempty"`
	Snippet    string `json:"snippet,omitempty"`
	SendCount  int    `json:"send_count"`
}

func loadReminderState(path string) (ReminderState, error) {
	state := ReminderState{Highlights: map[string]HighlightState{}}
	if path == "" {
		return state, nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return state, nil
		}
		return state, err
	}

	if len(content) == 0 {
		return state, nil
	}

	if err := json.Unmarshal(content, &state); err != nil {
		return ReminderState{Highlights: map[string]HighlightState{}}, err
	}
	if state.Highlights == nil {
		state.Highlights = map[string]HighlightState{}
	}

	return state, nil
}

func saveReminderState(path string, state ReminderState) error {
	if path == "" {
		return nil
	}
	if state.Highlights == nil {
		state.Highlights = map[string]HighlightState{}
	}

	content, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	content = append(content, '\n')

	dir := filepath.Dir(path)
	tmpFile, err := os.CreateTemp(dir, ".remind-state-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.Write(content); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Chmod(0o644); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}

	return os.Rename(tmpPath, path)
}

func (state ReminderState) IsDue(clipping Clipping, now time.Time) bool {
	if clipping.ID == "" {
		return true
	}

	review, ok := state.Highlights[clipping.ID]
	if !ok {
		return true
	}
	if review.PendingAt != "" && !pendingReviewIsStale(review.PendingAt, now) {
		return false
	}
	if review.NextDueAt == "" {
		return true
	}

	nextDue, err := time.Parse(time.RFC3339, review.NextDueAt)
	if err != nil {
		return true
	}

	return !nextDue.After(now.UTC())
}

func (state ReminderState) Review(clipping Clipping) (HighlightState, bool) {
	if clipping.ID == "" || state.Highlights == nil {
		return HighlightState{}, false
	}
	review, ok := state.Highlights[clipping.ID]
	return review, ok
}

func (state *ReminderState) Prune(clippings []Clipping) {
	if state.Highlights == nil {
		return
	}

	currentIDs := map[string]bool{}
	for _, clipping := range clippings {
		if clipping.ID != "" {
			currentIDs[clipping.ID] = true
		}
	}

	for id := range state.Highlights {
		if !currentIDs[id] {
			delete(state.Highlights, id)
		}
	}
}

func (state *ReminderState) MarkPending(clippings []Clipping, now time.Time) {
	if state.Highlights == nil {
		state.Highlights = map[string]HighlightState{}
	}

	pendingAt := now.UTC().Format(time.RFC3339)
	for _, clipping := range clippings {
		if clipping.ID == "" {
			continue
		}

		review := state.Highlights[clipping.ID]
		if review.CreatedAt == "" {
			review.CreatedAt = pendingAt
		}
		review.Title = clipping.Title
		review.Author = clipping.Author
		review.Snippet = highlightSnippet(clipping.Highlight)
		review.PendingAt = pendingAt
		review.UpdatedAt = pendingAt
		state.Highlights[clipping.ID] = review
	}
}

func (state *ReminderState) MarkReviewed(clippings []Clipping, now time.Time, intervals []time.Duration) {
	if state.Highlights == nil {
		state.Highlights = map[string]HighlightState{}
	}

	sentAt := now.UTC()
	for _, clipping := range clippings {
		if clipping.ID == "" {
			continue
		}

		review := state.Highlights[clipping.ID]
		if review.CreatedAt == "" {
			review.CreatedAt = sentAt.Format(time.RFC3339)
		}
		review.SendCount++
		review.Title = clipping.Title
		review.Author = clipping.Author
		review.Snippet = highlightSnippet(clipping.Highlight)
		review.PendingAt = ""
		review.LastSentAt = sentAt.Format(time.RFC3339)
		review.UpdatedAt = sentAt.Format(time.RFC3339)
		review.NextDueAt = sentAt.Add(nextReviewInterval(review.SendCount, intervals)).Format(time.RFC3339)
		state.Highlights[clipping.ID] = review
	}
}

func (state *ReminderState) MarkFailed(clippings []Clipping, now time.Time) {
	if state.Highlights == nil {
		return
	}

	updatedAt := now.UTC().Format(time.RFC3339)
	for _, clipping := range clippings {
		if clipping.ID == "" {
			continue
		}
		review := state.Highlights[clipping.ID]
		review.PendingAt = ""
		review.UpdatedAt = updatedAt
		state.Highlights[clipping.ID] = review
	}
}

func nextReviewInterval(sendCount int, intervals []time.Duration) time.Duration {
	if len(intervals) == 0 {
		intervals = reviewIntervals(nil)
	}
	if sendCount <= 0 {
		return intervals[0]
	}
	if sendCount > len(intervals) {
		return intervals[len(intervals)-1]
	}
	return intervals[sendCount-1]
}

func highlightSnippet(highlight string) string {
	words := []rune(highlight)
	if len(words) <= 120 {
		return highlight
	}
	return string(words[:120])
}

func pendingReviewIsStale(pendingAt string, now time.Time) bool {
	parsed, err := time.Parse(time.RFC3339, pendingAt)
	if err != nil {
		return true
	}
	return parsed.Add(24 * time.Hour).Before(now.UTC())
}
