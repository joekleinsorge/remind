package main

import (
	"bytes"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
)

// Define the struct for the clipping
type Clipping struct {
	Title     string
	Author    string
	Page      string
	When      string
	Highlight string
}

func main() {
	// Read the contents of the clippings.txt file
	filePath := "clippings.txt"
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		fmt.Printf("Failed to read the file: %v", err)
		return
	}

	// Parse the clippings
	clippingSlice := parseClippings(string(content))

	// Select 3 random quotes
	selectedClippings := selectRandomClippings(clippingSlice, 3)

	// pull in environment variables
	sendgridAPIKey := os.Getenv("SENDGRID_API_KEY")
	senderEmail := os.Getenv("SENDER_EMAIL")
	recipientEmail := os.Getenv("RECIPIENT_EMAIL")

	// Use the template to generate the email
	var tmplFile = "email.tmpl"

	tmpl, err := template.New(tmplFile).ParseFiles(tmplFile)
	if err != nil {
		panic(err)
	}

	var emailContent bytes.Buffer
	err = tmpl.Execute(&emailContent, selectedClippings)
	if err != nil {
		log.Fatalf("Failed to execute template: %s", err)
	}

	err = sendEmail(sendgridAPIKey, senderEmail, recipientEmail, emailContent.String())
	if err != nil {
		log.Fatalf("Failed to send email: %s", err)
	}
	log.Println("Email sent successfully!")
}

// Parse the clippings from the Kindle file
func parseClippings(data string) []Clipping {
	// Split the data into individual clippings
	delimiter := "=========="
	clippings := strings.Split(data, delimiter)

	// Define the regular expression patterns to extract the required information
	titlePattern := `^([^\n(]+)`                      //string up until the first parenthesis
	authorPattern := `\((.*?)\)`                      //string between the first and second parenthesis
	pagePattern := `on page (\d+)`                    //string after the word "page"
	whenPattern := `Added on\s+(.+)`                  //string after the word "Added"
	highlightPattern := `Added on[^\n]+\n?\r?\n?(.*)` //string after the word "Added" and the newline

	// Create a slice to store the clippings
	clippingSlice := []Clipping{}

	// Extract the information for each clipping
	for _, clipping := range clippings {
		// Trim leading and trailing whitespace from the clipping
		clipping = strings.TrimSpace(clipping)

		// Skip empty clippings
		if clipping == "" {
			continue
		}

		// Extract the information using regex
		titleRegex := regexp.MustCompile(titlePattern)
		authorRegex := regexp.MustCompile(authorPattern)
		pageRegex := regexp.MustCompile(pagePattern)
		whenRegex := regexp.MustCompile(whenPattern)
		highlightRegex := regexp.MustCompile(highlightPattern)

		title := extractSubmatch(clipping, titleRegex, "title")
		author := extractSubmatch(clipping, authorRegex, "author")
		page := extractSubmatch(clipping, pageRegex, "page")
		when := extractSubmatch(clipping, whenRegex, "when")
		highlight := extractSubmatch(clipping, highlightRegex, "highlight")

		// Create a new Clipping struct and append it to the slice
		newClipping := Clipping{
			Title:     title,
			Author:    author,
			Page:      page,
			When:      when,
			Highlight: highlight,
		}
		clippingSlice = append(clippingSlice, newClipping)
	}
	return clippingSlice
}

// Selects a given number of random clips from the clippings
func selectRandomClippings(clippings []Clipping, count int) []Clipping {
	// Set the random seed
	rand.Seed(time.Now().UnixNano())

	// Shuffle the clippings
	rand.Shuffle(len(clippings), func(i, j int) {
		clippings[i], clippings[j] = clippings[j], clippings[i]
	})

	// Select the desired number of clips
	clips := clippings[:count]

	return clips
}

// Function to extract submatch and handle errors
func extractSubmatch(clipping string, regex *regexp.Regexp, label string) string {
	match := regex.FindStringSubmatch(clipping)
	if len(match) < 2 {
		log.Fatalf("Failed to extract %s from clipping", label)
	}
	return match[1]
}

// Send an email with the selected clippings
func sendEmail(apiKey, senderEmail, recipientEmail, emailContent string) error {

	message := mail.NewSingleEmail(
		mail.NewEmail("Remind App", senderEmail),
		"Here is your REMINDer",
		mail.NewEmail("You", recipientEmail),
		"",
		emailContent,
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
