package main

import (
	"io/ioutil"
	"os"
	"testing"
)

func TestReadClippingsFile(t *testing.T) {
	// Create a temporary clippings file
	file, err := ioutil.TempFile("", "clippings*.txt")
	if err != nil {
		t.Fatalf("Failed to create temporary file: %s", err)
	}
	defer os.Remove(file.Name())

	// Write test data to the clippings file
	testData := "==========\nQuote 1\n==========\nQuote 2\n==========\nQuote 3\n"
	err = ioutil.WriteFile(file.Name(), []byte(testData), 0644)
	if err != nil {
		t.Fatalf("Failed to write test data to clippings file: %s", err)
	}

	// Test reading the clippings file
	clippings, err := readClippingsFile(file.Name())
	if err != nil {
		t.Fatalf("Failed to read clippings file: %s", err)
	}

	// Verify the extracted clippings
	expectedClippings := []string{"Quote 1", "Quote 2", "Quote 3"}
	if len(clippings) != len(expectedClippings) {
		t.Errorf("Expected %d clippings, got %d", len(expectedClippings), len(clippings))
	}

	for i, clipping := range clippings {
		if clipping != expectedClippings[i] {
			t.Errorf("Expected clipping %d to be %q, got %q", i+1, expectedClippings[i], clipping)
		}
	}
}

