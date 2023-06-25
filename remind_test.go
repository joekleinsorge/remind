package remind

import (
	"reflect"
	"testing"
)

func TestParseClippings(t *testing.T) {
	data := "clipping 1(Author 1) on page 1 Added on 2023-06-01\nHighlight 1\n==========\nclipping 2(Author 2) on page 2 Added on 2023-06-02\nHighlight 2\n"
	expected := []Clipping{
		{
			Title:     "clipping 1",
			Author:    "Author 1",
			Page:      "1",
			When:      "2023-06-01",
			Highlight: "Highlight 1",
		},
		{
			Title:     "clipping 2",
			Author:    "Author 2",
			Page:      "2",
			When:      "2023-06-02",
			Highlight: "Highlight 2",
		},
	}
	result := parseClippings(data)

	if len(result) != len(expected) {
		t.Errorf("Unexpected number of clippings. Got: %d, Expected: %d", len(result), len(expected))
	}

	for i := range result {
		if result[i].Title != expected[i].Title ||
			result[i].Author != expected[i].Author ||
			result[i].Page != expected[i].Page ||
			result[i].When != expected[i].When ||
			result[i].Highlight != expected[i].Highlight {
			t.Errorf("Unexpected clipping at index %d. Got: %+v, Expected: %+v", i, result[i], expected[i])
		}
	}
}

func TestSelectRandomClippings(t *testing.T) {
	clippings := []Clipping{
		{
			Title:     "clipping 1",
			Author:    "Author 1",
			Page:      "1",
			When:      "2023-06-01",
			Highlight: "Highlight 1",
		},
		{
			Title:     "clipping 2",
			Author:    "Author 2",
			Page:      "2",
			When:      "2023-06-02",
			Highlight: "Highlight 2",
		},
		{
			Title:     "clipping 3",
			Author:    "Author 3",
			Page:      "3",
			When:      "2023-06-03",
			Highlight: "Highlight 3",
		},
		{
			Title:     "clipping 4",
			Author:    "Author 4",
			Page:      "4",
			When:      "2023-06-04",
			Highlight: "Highlight 4",
		},
	}

	selectedClippings := selectRandomClippings(clippings, 2)

	if len(selectedClippings) != 2 {
		t.Errorf("Unexpected number of selected clippings. Got: %d, Expected: 2", len(selectedClippings))
	}

	for _, clip := range selectedClippings {
		found := false
		for _, c := range clippings {
			if reflect.DeepEqual(clip, c) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Selected clipping not found in the original clippings. Clipping: %v", clip)
		}
	}
}
