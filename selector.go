package main

import (
	"sort"
	"strings"
	"time"
)

func selectClippings(clippings []Clipping, count int, state ReminderState, now time.Time) []Clipping {
	if count <= 0 || len(clippings) == 0 {
		return nil
	}

	candidates := filterDueClippings(clippings, state, now)
	if count > len(candidates) {
		count = len(candidates)
	}

	byTitle := make(map[string][]Clipping)
	for _, clipping := range candidates {
		key := strings.ToLower(strings.TrimSpace(clipping.Title))
		if key == "" {
			key = "untitled"
		}
		byTitle[key] = append(byTitle[key], clipping)
	}

	type titledClippings struct {
		Key     string
		Clips   []Clipping
		Oldest  time.Time
		HasDate bool
	}

	titleBuckets := make([]titledClippings, 0, len(byTitle))
	for key, clips := range byTitle {
		sort.SliceStable(clips, func(i, j int) bool {
			return compareCandidates(clips[i], clips[j], state)
		})

		oldest, hasDate := clippingDate(clips[0])
		titleBuckets = append(titleBuckets, titledClippings{
			Key:     key,
			Clips:   clips,
			Oldest:  oldest,
			HasDate: hasDate,
		})
	}

	sort.SliceStable(titleBuckets, func(i, j int) bool {
		left := titleBuckets[i]
		right := titleBuckets[j]
		if len(left.Clips) > 0 && len(right.Clips) > 0 {
			return compareCandidates(left.Clips[0], right.Clips[0], state)
		}
		return titleBuckets[i].Key < titleBuckets[j].Key
	})

	selected := make([]Clipping, 0, count)
	for len(selected) < count {
		addedThisPass := false
		for i := range titleBuckets {
			if len(titleBuckets[i].Clips) == 0 {
				continue
			}
			selected = append(selected, titleBuckets[i].Clips[0])
			titleBuckets[i].Clips = titleBuckets[i].Clips[1:]
			addedThisPass = true
			if len(selected) == count {
				break
			}
		}
		if !addedThisPass {
			break
		}
	}

	return selected
}

func filterDueClippings(clippings []Clipping, state ReminderState, now time.Time) []Clipping {
	filtered := make([]Clipping, 0, len(clippings))
	for _, clipping := range clippings {
		if state.IsDue(clipping, now) {
			filtered = append(filtered, clipping)
		}
	}
	return filtered
}

func clippingDate(clipping Clipping) (time.Time, bool) {
	if !clipping.AddedAt.IsZero() {
		return clipping.AddedAt, true
	}
	return parseWhen(clipping.When)
}

func compareCandidates(left, right Clipping, state ReminderState) bool {
	leftReview, _ := state.Review(left)
	rightReview, _ := state.Review(right)
	if leftReview.SendCount != rightReview.SendCount {
		return leftReview.SendCount < rightReview.SendCount
	}

	if leftReview.Favorite != rightReview.Favorite {
		return leftReview.Favorite
	}

	leftDue, leftDueOK := reviewDueDate(leftReview)
	rightDue, rightDueOK := reviewDueDate(rightReview)
	if leftDueOK && rightDueOK && !leftDue.Equal(rightDue) {
		return leftDue.Before(rightDue)
	}
	if leftDueOK != rightDueOK {
		return leftDueOK
	}

	leftDate, leftDateOK := clippingDate(left)
	rightDate, rightDateOK := clippingDate(right)
	if leftDateOK && rightDateOK && !leftDate.Equal(rightDate) {
		return leftDate.Before(rightDate)
	}
	if leftDateOK != rightDateOK {
		return leftDateOK
	}

	return left.ID < right.ID
}

func reviewDueDate(review HighlightState) (time.Time, bool) {
	if review.NextDueAt == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339, review.NextDueAt)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

// Rotate emailed passages without treating delivery as a review.
func selectEmailClippings(clippings []Clipping, count int, state ReminderState, now time.Time) []Clipping {
	candidates := filterDueClippings(clippings, state, now)
	deliveryState := cloneState(state)
	for _, c := range candidates {
		review := deliveryState.Highlights[c.ID]
		review.SendCount = review.EmailCount
		review.NextDueAt = review.LastEmailedAt
		review.PendingAt = ""
		deliveryState.Highlights[c.ID] = review
	}
	return selectClippings(candidates, count, deliveryState, now)
}
