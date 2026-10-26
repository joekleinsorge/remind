package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	kindleclippings "github.com/hawx/kindle-clippings/clippings"
)

func parseClippings(data string) []Clipping {
	records := splitKindleRecords(data)
	clippingSlice := []Clipping{}
	seen := map[string]bool{}

	for idx, record := range records {
		recordNumber := idx + 1
		parsed, err := parseKindleRecord(record)
		if err != nil {
			log.Printf("Skipping clipping %d due to parse error: %v", recordNumber, err)
			continue
		}

		kind := strings.TrimSpace(strings.ToLower(parsed.Type))
		if kind != "" && kind != "highlight" {
			log.Printf("Skipping clipping %d (%q): unsupported type %q", recordNumber, parsed.Title, parsed.Type)
			continue
		}

		highlight := strings.TrimSpace(strings.ReplaceAll(parsed.Content, "\r\n", "\n"))
		if highlight == "" {
			log.Printf("Skipping clipping %d (%q): missing highlight body", recordNumber, parsed.Title)
			continue
		}

		title := strings.TrimSpace(parsed.Title)
		if title == "" {
			title = "Untitled"
		}

		author := strings.TrimSpace(parsed.Author)
		if author == "" {
			author = "Unknown author"
		}

		page := ""
		if parsed.Page != nil {
			page = strconv.Itoa(*parsed.Page)
		}

		location := formatLocation(parsed.Location)

		addedAt := parsed.Datetime
		if addedAt.IsZero() {
			addedAt, _ = parseWhen(extractAddedOnDate(record))
		}

		when := ""
		if !addedAt.IsZero() {
			when = addedAt.Format("Monday, January 2, 2006 3:04:05 PM")
		}

		newClipping := Clipping{
			Title:     title,
			Author:    author,
			Kind:      kind,
			Page:      page,
			Location:  location,
			When:      when,
			AddedAt:   addedAt,
			Highlight: highlight,
		}
		newClipping.ID = clippingID(newClipping)
		if seen[newClipping.ID] {
			continue
		}
		seen[newClipping.ID] = true
		clippingSlice = append(clippingSlice, newClipping)
	}

	return clippingSlice
}

func splitKindleRecords(data string) []string {
	normalized := strings.TrimPrefix(data, "\uFEFF")
	normalized = strings.ReplaceAll(normalized, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")

	records := []string{}
	current := []string{}
	scanner := bufio.NewScanner(strings.NewReader(normalized))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "==========" {
			record := strings.TrimSpace(strings.Join(current, "\n"))
			if record != "" {
				records = append(records, record)
			}
			current = current[:0]
			continue
		}
		current = append(current, line)
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Error while scanning clippings input: %v", err)
	}

	record := strings.TrimSpace(strings.Join(current, "\n"))
	if record != "" {
		records = append(records, record)
	}

	return records
}

func extractAddedOnDate(record string) string {
	for _, line := range strings.Split(record, "\n") {
		for _, part := range strings.Split(line, "|") {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(part, "Added on") {
				return strings.TrimSpace(strings.TrimPrefix(part, "Added on"))
			}
		}
	}
	return ""
}

func parseKindleRecord(record string) (kindleclippings.Clipping, error) {
	trimmedRecord := strings.TrimSpace(record)
	if trimmedRecord == "" {
		return kindleclippings.Clipping{}, fmt.Errorf("empty clipping record")
	}

	payload := trimmedRecord + "\n==========\n"
	reader := kindleclippings.NewReader(strings.NewReader(normalizeForKindleParser(payload)))
	return readKindleRecord(reader)
}

func readKindleRecord(reader *kindleclippings.Reader) (parsed kindleclippings.Clipping, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("kindle parser panic: %v", recovered)
		}
	}()
	return reader.Read()
}

func normalizeForKindleParser(data string) string {
	normalized := strings.TrimPrefix(data, "\uFEFF")
	normalized = strings.ReplaceAll(normalized, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	normalized = strings.ReplaceAll(normalized, "\n", "\r\n")
	return normalized
}

func formatLocation(location *kindleclippings.Range) string {
	if location == nil {
		return ""
	}
	if location.From == location.To {
		return strconv.Itoa(location.From)
	}
	return fmt.Sprintf("%d-%d", location.From, location.To)
}

func parseWhen(when string) (time.Time, bool) {
	cleanedWhen := strings.TrimSpace(when)
	if cleanedWhen == "" {
		return time.Time{}, false
	}

	layouts := []string{
		"Monday, January 2, 2006 3:04:05 PM",
		"Monday, January 2, 2006 3:04 PM",
		"Monday, 2 January 2006 15:04:05",
		"Monday, 2 January 06 15:04:05",
		"2006-01-02",
	}
	for _, layout := range layouts {
		parsed, err := time.Parse(layout, cleanedWhen)
		if err == nil {
			return parsed, true
		}
	}

	return time.Time{}, false
}

func clippingID(clipping Clipping) string {
	hash := sha256.Sum256([]byte(strings.Join([]string{
		normalizeIDPart(clipping.Title),
		normalizeIDPart(clipping.Author),
		normalizeIDPart(clipping.Highlight),
	}, "\x00")))
	return hex.EncodeToString(hash[:])
}

func normalizeIDPart(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}
