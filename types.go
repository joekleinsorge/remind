package main

import "time"

type Clipping struct {
	ID        string
	Title     string
	Author    string
	Kind      string
	Page      string
	Location  string
	When      string
	AddedAt   time.Time
	Highlight string
}

type EmailTemplateData struct {
	Preheader  string
	DateLabel  string
	Highlights []Clipping
}
