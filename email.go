package main

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"strings"
	"time"

	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
)

func buildEmail(clippings []Clipping, nowNow func() string, templatePath string) (string, string, string, error) {
	dateLabel := nowNow()
	subject := fmt.Sprintf("Your daily highlights for %s", dateLabel)
	emailData := EmailTemplateData{
		Preheader:  "A few highlights worth revisiting today.",
		DateLabel:  dateLabel,
		Highlights: clippings,
	}

	htmlContent, err := renderHTMLEmail(emailData, templatePath)
	if err != nil {
		return "", "", "", err
	}

	return subject, renderPlainTextEmail(emailData), htmlContent, nil
}

func renderHTMLEmail(data EmailTemplateData, templatePath string) (string, error) {
	tmpl, err := template.ParseFiles(templatePath)
	if err != nil {
		return "", err
	}

	var emailContent bytes.Buffer
	if err := tmpl.Execute(&emailContent, data); err != nil {
		return "", err
	}
	return emailContent.String(), nil
}

func renderPlainTextEmail(data EmailTemplateData) string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("Your daily highlights for %s\n", data.DateLabel))
	builder.WriteString(data.Preheader)
	builder.WriteString("\n")

	for idx, clipping := range data.Highlights {
		builder.WriteString(fmt.Sprintf("\n%d. From %s", idx+1, clipping.Title))
		if clipping.Author != "" {
			builder.WriteString(fmt.Sprintf(" by %s", clipping.Author))
		}
		builder.WriteString("\n")

		metadata := clippingMetadata(clipping)
		if metadata != "" {
			builder.WriteString(metadata)
			builder.WriteString("\n")
		}

		builder.WriteString(fmt.Sprintf("\"%s\"\n", strings.TrimSpace(clipping.Highlight)))
	}

	return builder.String()
}

func clippingMetadata(clipping Clipping) string {
	parts := []string{}
	if clipping.Page != "" {
		parts = append(parts, fmt.Sprintf("Page %s", clipping.Page))
	}
	if clipping.Location != "" {
		parts = append(parts, fmt.Sprintf("Location %s", clipping.Location))
	}
	if clipping.When != "" {
		parts = append(parts, fmt.Sprintf("Added on %s", clipping.When))
	}
	return strings.Join(parts, " | ")
}

func sendEmail(apiKey, senderEmail, recipientEmail, subject, plainTextContent, htmlContent string) error {
	message := mail.NewSingleEmail(
		mail.NewEmail("Remind Daily", senderEmail),
		subject,
		mail.NewEmail("You", recipientEmail),
		plainTextContent,
		htmlContent,
	)

	client := sendgrid.NewSendClient(apiKey)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	response, err := client.SendWithContext(ctx, message)
	if err != nil {
		return fmt.Errorf("failed to send email: %s", err)
	}

	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return nil
	}

	return fmt.Errorf("failed to send email: status code %d", response.StatusCode)
}
