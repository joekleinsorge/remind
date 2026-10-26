package main

import (
	"encoding/json"
	"os"
	"time"
)

type appConfig struct {
	ClippingsPath       string `json:"clippings_path"`
	Count               int    `json:"count"`
	StatePath           string `json:"state_path"`
	TemplatePath        string `json:"template_path"`
	PreviewPath         string `json:"preview_path"`
	DryRun              bool   `json:"dry_run"`
	PruneMissing        bool   `json:"prune_missing"`
	ReviewIntervalsDays []int  `json:"review_intervals_days"`
}

func defaultAppConfig() appConfig {
	return appConfig{
		ClippingsPath:       "clippings.txt",
		Count:               3,
		StatePath:           ".remind-state.json",
		TemplatePath:        "email.tmpl",
		ReviewIntervalsDays: []int{7, 30, 90, 180},
	}
}

func loadAppConfig(path string) (appConfig, error) {
	cfg := defaultAppConfig()
	if path == "" {
		return cfg, nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(content, &cfg); err != nil {
		return cfg, err
	}

	if cfg.Count <= 0 {
		cfg.Count = 3
	}
	if cfg.ClippingsPath == "" {
		cfg.ClippingsPath = "clippings.txt"
	}
	if cfg.StatePath == "" {
		cfg.StatePath = ".remind-state.json"
	}
	if cfg.TemplatePath == "" {
		cfg.TemplatePath = "email.tmpl"
	}
	if len(cfg.ReviewIntervalsDays) == 0 {
		cfg.ReviewIntervalsDays = []int{7, 30, 90, 180}
	}

	return cfg, nil
}

func reviewIntervals(days []int) []time.Duration {
	intervals := make([]time.Duration, 0, len(days))
	for _, day := range days {
		if day > 0 {
			intervals = append(intervals, time.Duration(day)*24*time.Hour)
		}
	}
	if len(intervals) == 0 {
		return []time.Duration{7 * 24 * time.Hour, 30 * 24 * time.Hour, 90 * 24 * time.Hour, 180 * 24 * time.Hour}
	}
	return intervals
}
