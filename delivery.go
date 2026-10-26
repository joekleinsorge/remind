package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// A successful delivery means SendGrid accepted the email, not that it reached
// the inbox. Keep the previous success when a later attempt fails.
type deliveryStatus struct {
	LastAttemptAt  string   `json:"last_attempt_at,omitempty"`
	FinishedAt     string   `json:"finished_at,omitempty"`
	Outcome        string   `json:"outcome,omitempty"`
	SelectedIDs    []string `json:"selected_ids,omitempty"`
	LastSuccessAt  string   `json:"last_success_at,omitempty"`
	LastSuccessIDs []string `json:"last_success_ids,omitempty"`
}

func deliveryStatusPath(statePath string) string {
	return filepath.Join(filepath.Dir(statePath), "delivery-status.json")
}

func loadDeliveryStatus(statePath string) (deliveryStatus, error) {
	var status deliveryStatus
	content, err := os.ReadFile(deliveryStatusPath(statePath))
	if errors.Is(err, os.ErrNotExist) {
		return status, nil
	}
	if err != nil {
		return status, err
	}
	err = json.Unmarshal(content, &status)
	return status, err
}

func (s *deliveryStatus) begin(selected []Clipping, now time.Time) {
	s.LastAttemptAt = now.UTC().Format(time.RFC3339)
	s.FinishedAt = ""
	s.Outcome = "in_progress"
	s.SelectedIDs = make([]string, 0, len(selected))
	for _, c := range selected {
		s.SelectedIDs = append(s.SelectedIDs, c.ID)
	}
}

func (s *deliveryStatus) finish(outcome string, now time.Time) {
	s.Outcome = outcome
	s.FinishedAt = now.UTC().Format(time.RFC3339)
	if outcome == "accepted" || outcome == "accepted_state_error" {
		s.LastSuccessAt = s.FinishedAt
		s.LastSuccessIDs = append([]string(nil), s.SelectedIDs...)
	}
}
