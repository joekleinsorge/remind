package main

import (
	"bufio"
	"fmt"
	"log"
  "io"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
)

func main() {
	// Enable debugging and logging
	debug := true
	logFile := "log.txt"
	setupLogging(debug, logFile)

	// Read Kindle clippings file
	clippingsFilePath := os.Getenv("CLIPPINGS_FILE_PATH")
	clippings, err := readClippingsFile(clippingsFilePath)
	if err != nil {
		log.Fatalf("Failed to read clippings file: %s", err)
	}

	// Select random quotes
	numQuotes := 3
	quotes := selectRandomQuotes(clippings, numQuotes)

	// Send email with selected quotes
	sendgridAPIKey := os.Getenv("SENDGRID_API_KEY")
	senderEmail := os.Getenv("SENDER_EMAIL")
	recipientEmail := os.Getenv("RECIPIENT_EMAIL")
	err = sendEmail(sendgridAPIKey, senderEmail, recipientEmail, quotes)
	if err != nil {
		log.Fatalf("Failed to send email: %s", err)
	}

	log.Println("Email sent successfully!")
}

// setupLogging configures the logging settings
func setupLogging(debug bool, logFile string) {
	if debug {
		// Log to console and file
		logFile, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			log.Fatalf("Failed to open log file: %s", err)
		}
		log.SetOutput(io.MultiWriter(os.Stdout, logFile))
	} else {
		// Log to file only
		logFile, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			log.Fatalf("Failed to open log file: %s", err)
		}
		log.SetOutput(logFile)
	}
}

// readClippingsFile reads the Kindle clippings file and returns the extracted highlights and quotes.
func readClippingsFile(filePath string) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var clippings []string
	var clipping string

	for scanner.Scan() {
		line := scanner.Text()

		// Clippings start with "========" delimiter
		if strings.HasPrefix(line, "==========") {
			if len(clipping) > 0 {
				clippings = append(clippings, strings.TrimSpace(clipping)) // Trim leading/trailing whitespace
				clipping = ""
			}
		} else {
			// Append non-empty lines to the current clipping
			if len(strings.TrimSpace(line)) > 0 {
				clipping += line + "\n"
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if len(clipping) > 0 {
		clippings = append(clippings, strings.TrimSpace(clipping)) // Trim leading/trailing whitespace
	}

	return clippings, nil
}

// selectRandomQuotes selects a given number of random quotes from the clippings.
func selectRandomQuotes(clippings []string, numQuotes int) []string {
	// Set the random seed
	rand.Seed(time.Now().UnixNano())

	// Shuffle the clippings
	rand.Shuffle(len(clippings), func(i, j int) {
		clippings[i], clippings[j] = clippings[j], clippings[i]
	})

	// Select the desired number of quotes
	selectedQuotes := clippings[:numQuotes]

	return selectedQuotes
}

// sendEmail sends an email with the selected quotes.
func sendEmail(apiKey, senderEmail, recipientEmail string, quotes []string) error {
	fromEmail := senderEmail

	message := mail.NewSingleEmail(
		mail.NewEmail("Sender Name", fromEmail),
		"Randomly Selected Quotes",
		mail.NewEmail("Recipient Name", recipientEmail),
		"",
		strings.Join(quotes, "\n\n"),
	)

	client := sendgrid.NewSendClient(apiKey)
	response, err := client.Send(message)
	if err != nil {
		return fmt.Errorf("failed to send email: %s", err)
	}

	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return nil
	}

	return fmt.Errorf("failed to send email: status code %d", response.StatusCode)
}
