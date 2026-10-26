package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseClippings(t *testing.T) {
	data := "Book One (Doe, Jane)\r\n- Your Highlight on page 11 | Location 120-121 | Added on Monday, 2 January 2006 15:04:05\r\n\r\nHighlight 1\r\n==========\r\n" +
		"Book Two (Author 2)\r\n- Your Highlight at location 220-221 | Added on Tuesday, 3 January 2006 08:09:10\r\n\r\nHighlight 2\r\n==========\r\n"
	expected := []Clipping{
		{
			Title:     "Book One",
			Author:    "Jane Doe",
			Kind:      "highlight",
			Page:      "11",
			Location:  "120-121",
			When:      "Monday, January 2, 2006 3:04:05 PM",
			Highlight: "Highlight 1",
		},
		{
			Title:     "Book Two",
			Author:    "Author 2",
			Kind:      "highlight",
			Page:      "",
			Location:  "220-221",
			When:      "Tuesday, January 3, 2006 8:09:10 AM",
			Highlight: "Highlight 2",
		},
	}

	result := parseClippings(data)
	if len(result) != len(expected) {
		t.Fatalf("Unexpected number of clippings. Got: %d, Expected: %d", len(result), len(expected))
	}

	for i := range result {
		if result[i].Title != expected[i].Title ||
			result[i].Author != expected[i].Author ||
			result[i].Kind != expected[i].Kind ||
			result[i].Page != expected[i].Page ||
			result[i].Location != expected[i].Location ||
			result[i].When != expected[i].When ||
			result[i].Highlight != expected[i].Highlight {
			t.Errorf("Unexpected clipping at index %d. Got: %+v, Expected: %+v", i, result[i], expected[i])
		}
	}
}

func TestParseClippingsFixtureFile(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("testdata", "clippings_fixture.txt"))
	if err != nil {
		t.Fatalf("Failed to read fixture: %v", err)
	}

	result := parseClippings(string(content))
	if len(result) != 2 {
		t.Fatalf("Expected 2 parsed clippings, got %d", len(result))
	}
	if result[0].When != "Tuesday, June 12, 2018 7:29:31 PM" {
		t.Fatalf("Expected real fixture date to parse, got %q", result[0].When)
	}
	if result[1].Location != "203-204" {
		t.Fatalf("Expected location-only fixture to keep location, got %q", result[1].Location)
	}
}

func TestParseClippingsMultilineHighlight(t *testing.T) {
	data := "Book One (Author A)\r\n- Your Highlight on page 8 | Added on Monday, 2 January 2006 15:04:05\r\n\r\nFirst line of quote.\r\nSecond line of quote.\r\n==========\r\n"
	result := parseClippings(data)

	if len(result) != 1 {
		t.Fatalf("Expected 1 parsed clipping, got %d", len(result))
	}

	expectedHighlight := "First line of quote.\nSecond line of quote."
	if result[0].Highlight != expectedHighlight {
		t.Fatalf("Unexpected multiline highlight.\nGot: %q\nExpected: %q", result[0].Highlight, expectedHighlight)
	}
}

func TestParseClippingsUSKindleDate(t *testing.T) {
	data := "Sapiens: A Brief History of Humankind (Yuval Noah Harari)\r\n- Your Highlight on page 122 | Location 1867-1873 | Added on Tuesday, June 12, 2018 7:29:31 PM\r\n\r\nA real fixture-style highlight.\r\n==========\r\n"
	result := parseClippings(data)

	if len(result) != 1 {
		t.Fatalf("Expected 1 parsed clipping, got %d", len(result))
	}
	if result[0].When != "Tuesday, June 12, 2018 7:29:31 PM" {
		t.Fatalf("Expected U.S. Kindle date to be preserved. Got %q", result[0].When)
	}
}

func TestParseClippingsOmitsMissingMetadata(t *testing.T) {
	data := "Book Without Page (Author A)\r\n- Your Highlight at location 220-221 | Added on Tuesday, 3 January 2006 08:09:10\r\n\r\nHighlight without a page.\r\n==========\r\n"
	result := parseClippings(data)

	if len(result) != 1 {
		t.Fatalf("Expected 1 parsed clipping, got %d", len(result))
	}
	if result[0].Page != "" {
		t.Fatalf("Expected missing page to be empty, got %q", result[0].Page)
	}
	if result[0].When == "" {
		t.Fatal("Expected date to be parsed")
	}
}

func TestSplitKindleRecordsAllowsLongHighlightLines(t *testing.T) {
	longHighlight := strings.Repeat("a", 70*1024)
	data := "Long Book (Author A)\r\n- Your Highlight on page 1 | Added on Tuesday, June 12, 2018 7:29:31 PM\r\n\r\n" + longHighlight + "\r\n==========\r\n"

	result := parseClippings(data)
	if len(result) != 1 {
		t.Fatalf("Expected 1 parsed clipping, got %d", len(result))
	}
	if result[0].Highlight != longHighlight {
		t.Fatalf("Expected long highlight to survive parsing. Got length %d", len(result[0].Highlight))
	}
}

func TestParseClippingsSkipsMalformedEntries(t *testing.T) {
	data := "Valid Book (Author A)\r\n- Your Highlight on page 21 | Added on Monday, 2 January 2006 15:04:05\r\n\r\nValid highlight\r\n==========\r\n" +
		"Malformed clipping without metadata line\r\n==========\r\n" +
		"Trailing Book (Author B)\r\n- Your Highlight on page 40 | Added on Wednesday, 4 January 2006 06:07:08\r\n\r\nTrailing highlight\r\n==========\r\n"
	result := parseClippings(data)

	if len(result) != 2 {
		t.Fatalf("Expected malformed clipping to be skipped while parsing later records. Got %d parsed clippings", len(result))
	}
	if result[0].Title != "Valid Book" {
		t.Fatalf("Unexpected parsed clipping title: %q", result[0].Title)
	}
	if result[1].Title != "Trailing Book" {
		t.Fatalf("Expected parser to continue after malformed record. Got trailing title %q", result[1].Title)
	}
}

func TestParseClippingsDedupesNormalizedHighlights(t *testing.T) {
	data := "Book One (Author A)\r\n- Your Highlight on page 8 | Location 10-11 | Added on Monday, 2 January 2006 15:04:05\r\n\r\nSame highlight text.\r\n==========\r\n" +
		"Book One (Author A)\r\n- Your Highlight on page 9 | Location 12-13 | Added on Tuesday, 3 January 2006 08:09:10\r\n\r\nSame    highlight text.\r\n==========\r\n"

	result := parseClippings(data)
	if len(result) != 1 {
		t.Fatalf("Expected duplicate highlights to collapse, got %d", len(result))
	}
}

func TestSelectClippingsCapsRequestedCount(t *testing.T) {
	clippings := []Clipping{
		{
			Title:     "Book A",
			Author:    "Author 1",
			Page:      "1",
			When:      "2023-06-01",
			Highlight: "Highlight 1",
		},
		{
			Title:     "Book B",
			Author:    "Author 2",
			Page:      "2",
			When:      "2023-06-02",
			Highlight: "Highlight 2",
		},
	}

	selected := selectClippings(clippings, 10, ReminderState{}, time.Now())
	if len(selected) != 2 {
		t.Fatalf("Expected capped selection to return all available clippings. Got %d", len(selected))
	}
}

func TestSelectClippingsDiversifiesAcrossTitles(t *testing.T) {
	clippings := []Clipping{
		{
			Title:     "Book A",
			Author:    "Author 1",
			Page:      "1",
			When:      "Monday, January 1, 2001 1:00:00 PM",
			Highlight: "A1",
		},
		{
			Title:     "Book A",
			Author:    "Author 1",
			Page:      "2",
			When:      "Monday, January 2, 2001 1:00:00 PM",
			Highlight: "A2",
		},
		{
			Title:     "Book B",
			Author:    "Author 2",
			Page:      "1",
			When:      "Saturday, January 1, 2000 1:00:00 PM",
			Highlight: "B1",
		},
	}

	selected := selectClippings(clippings, 2, ReminderState{}, time.Now())
	if len(selected) != 2 {
		t.Fatalf("Expected 2 selected clippings. Got %d", len(selected))
	}
	if selected[0].Title == selected[1].Title {
		t.Fatalf("Expected diversified selection across titles. Got %q and %q", selected[0].Title, selected[1].Title)
	}
}

func TestSelectClippingsUsesDueState(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	clippings := []Clipping{
		{
			ID:        "sent",
			Title:     "Book A",
			Author:    "Author 1",
			When:      "Monday, January 1, 2001 1:00:00 PM",
			Highlight: "A1",
		},
		{
			ID:        "fresh",
			Title:     "Book B",
			Author:    "Author 2",
			When:      "Monday, January 2, 2001 1:00:00 PM",
			Highlight: "B1",
		},
	}
	state := ReminderState{Highlights: map[string]HighlightState{
		"sent": {
			LastSentAt: "2026-05-30T00:00:00Z",
			NextDueAt:  "2026-06-06T00:00:00Z",
			SendCount:  1,
		},
	}}

	selected := selectClippings(clippings, 1, state, now)
	if len(selected) != 1 {
		t.Fatalf("Expected one selected clipping, got %d", len(selected))
	}
	if selected[0].ID != "fresh" {
		t.Fatalf("Expected sent clipping to be skipped, got %q", selected[0].ID)
	}
}

func TestSelectClippingsReturnsNoneWhenNothingIsDue(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	clippings := []Clipping{
		{
			ID:        "sent",
			Title:     "Book A",
			Author:    "Author 1",
			When:      "Monday, January 1, 2001 1:00:00 PM",
			Highlight: "A1",
		},
	}
	state := ReminderState{Highlights: map[string]HighlightState{
		"sent": {
			LastSentAt: "2026-05-30T00:00:00Z",
			NextDueAt:  "2026-06-06T00:00:00Z",
			SendCount:  1,
		},
	}}

	selected := selectClippings(clippings, 1, state, now)
	if len(selected) != 0 {
		t.Fatalf("Expected no selected clippings, got %d", len(selected))
	}
}

func TestReminderStateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	state := ReminderState{}
	state.MarkReviewed([]Clipping{{ID: "abc", Title: "Book", Author: "Author", Highlight: "Important highlight"}}, time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC), reviewIntervals([]int{7, 30, 90, 180}))

	if err := saveReminderState(path, state); err != nil {
		t.Fatalf("Failed to save state: %v", err)
	}

	loaded, err := loadReminderState(path)
	if err != nil {
		t.Fatalf("Failed to load state: %v", err)
	}
	review, ok := loaded.Review(Clipping{ID: "abc"})
	if !ok {
		t.Fatal("Expected loaded state to contain reviewed clipping")
	}
	if review.SendCount != 1 {
		t.Fatalf("Expected send count 1, got %d", review.SendCount)
	}
	if review.NextDueAt != "2026-06-06T12:00:00Z" {
		t.Fatalf("Expected next due one week later, got %q", review.NextDueAt)
	}
	if review.Title != "Book" {
		t.Fatalf("Expected title metadata to be stored, got %q", review.Title)
	}
}

func TestReminderStatePendingSuppressesDuplicateSelection(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	clipping := Clipping{ID: "abc", Title: "Book", Author: "Author", Highlight: "Quote"}
	state := ReminderState{}
	state.MarkPending([]Clipping{clipping}, now)

	selected := selectClippings([]Clipping{clipping}, 1, state, now.Add(time.Hour))
	if len(selected) != 0 {
		t.Fatalf("Expected pending clipping to be suppressed, got %d", len(selected))
	}
}

func TestReminderStateStalePendingBecomesDue(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	clipping := Clipping{ID: "abc", Title: "Book", Author: "Author", Highlight: "Quote"}
	state := ReminderState{}
	state.MarkPending([]Clipping{clipping}, now)

	selected := selectClippings([]Clipping{clipping}, 1, state, now.Add(25*time.Hour))
	if len(selected) != 1 {
		t.Fatalf("Expected stale pending clipping to become due, got %d", len(selected))
	}
}

func TestRenderPlainTextEmailOmitsMissingMetadata(t *testing.T) {
	body := renderPlainTextEmail(EmailTemplateData{
		DateLabel: "Sunday, May 31, 2026",
		Preheader: "Review these.",
		Highlights: []Clipping{
			{Title: "Book", Author: "Author", Highlight: "Quote."},
		},
	})

	if strings.Contains(body, "Unknown") || strings.Contains(body, "Page ") {
		t.Fatalf("Expected missing metadata to be omitted, got:\n%s", body)
	}
}
