package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"time"
)

type importResult struct {
	Added      int `json:"added"`
	Duplicates int `json:"duplicates"`
	Total      int `json:"total"`
}

func importClippings(source io.Reader, endpoint string) (importResult, error) {
	var result importResult
	content, err := io.ReadAll(io.LimitReader(source, (10<<20)+1))
	if err != nil {
		return result, fmt.Errorf("read clippings: %w", err)
	}
	if len(content) == 0 || len(content) > 10<<20 {
		return result, fmt.Errorf("clippings must contain between 1 byte and 10 MB")
	}
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	file, err := form.CreateFormFile("file", "My Clippings.txt")
	if err != nil {
		return result, err
	}
	if _, err = file.Write(content); err != nil {
		return result, err
	}
	if err = form.Close(); err != nil {
		return result, err
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, &body)
	if err != nil {
		return result, err
	}
	req.Header.Set("Content-Type", form.FormDataContentType())
	req.Header.Set("X-Remind-Request", "1")
	client := &http.Client{
		Timeout: 45 * time.Second,
		// An authentication redirect must not forward private clippings elsewhere.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	response, err := client.Do(req)
	if err != nil {
		return result, fmt.Errorf("reach running Remind server: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return result, fmt.Errorf("import rejected (HTTP %d); use the local server or the Kubernetes import script", response.StatusCode)
	}
	if err = json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&result); err != nil {
		return result, fmt.Errorf("invalid import response: %w", err)
	}
	return result, nil
}

func runImport(path, endpoint string) error {
	var source io.Reader = os.Stdin
	if path != "-" {
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		source = file
	}
	result, err := importClippings(source, endpoint)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}
