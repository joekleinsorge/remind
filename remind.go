package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

type config struct {
	configPath string
	app        appConfig
	serve      string
	importPath string
	importURL  string
}

func main() {
	cfg := parseConfig()
	if cfg.importPath != "" {
		if cfg.serve != "" {
			log.Fatal("--import and --serve cannot be used together")
		}
		if err := runImport(cfg.importPath, cfg.importURL); err != nil {
			log.Fatal(err)
		}
		return
	}
	if cfg.serve != "" {
		if err := serveApp(cfg.app, cfg.serve); err != nil {
			log.Fatal(err)
		}
		return
	}

	content, err := os.ReadFile(cfg.app.ClippingsPath)
	if err != nil {
		log.Fatalf("Failed to read the file: %v", err)
	}

	clippingSlice := parseClippings(string(content))
	if len(clippingSlice) == 0 {
		log.Fatal("No valid clippings found")
	}
	log.Printf("Parsed %d unique highlights from %s", len(clippingSlice), cfg.app.ClippingsPath)

	state, err := loadReminderState(cfg.app.StatePath)
	if err != nil {
		log.Fatalf("Failed to load reminder state: %v", err)
	}
	if cfg.app.PruneMissing {
		state.Prune(clippingSlice)
		log.Println("Pruned state entries that are no longer present in clippings")
	}

	now := time.Now()
	dueCount := len(filterDueClippings(clippingSlice, state, now))
	selectedClippings := selectClippings(clippingSlice, cfg.app.Count, state, now)
	log.Printf("Found %d due highlights; selected %d", dueCount, len(selectedClippings))
	if len(selectedClippings) == 0 {
		log.Println("No highlights are due for review.")
		return
	}

	subject, plainTextContent, htmlContent, err := buildEmail(selectedClippings, func() string {
		return now.Format("Monday, January 2, 2006")
	}, cfg.app.TemplatePath)
	if err != nil {
		log.Fatalf("Failed to render email: %v", err)
	}

	if cfg.app.PreviewPath != "" {
		if err := os.WriteFile(cfg.app.PreviewPath, []byte(htmlContent), 0o644); err != nil {
			log.Fatalf("Failed to write preview: %v", err)
		}
		log.Printf("Preview written to %s", cfg.app.PreviewPath)
		return
	}

	if cfg.app.DryRun {
		printDryRun(subject, selectedClippings, state, now)
		return
	}

	sendgridAPIKey := os.Getenv("SENDGRID_API_KEY")
	senderEmail := os.Getenv("SENDER_EMAIL")
	recipientEmail := os.Getenv("RECIPIENT_EMAIL")
	if sendgridAPIKey == "" || senderEmail == "" || recipientEmail == "" {
		log.Fatal("Missing one or more required environment variables: SENDGRID_API_KEY, SENDER_EMAIL, RECIPIENT_EMAIL")
	}

	state.MarkPending(selectedClippings, now)
	if err := saveReminderState(cfg.app.StatePath, state); err != nil {
		log.Fatalf("Failed to save pending reminder state: %v", err)
	}

	if err := sendEmail(sendgridAPIKey, senderEmail, recipientEmail, subject, plainTextContent, htmlContent); err != nil {
		state.MarkFailed(selectedClippings, time.Now())
		if saveErr := saveReminderState(cfg.app.StatePath, state); saveErr != nil {
			log.Printf("Failed to clear pending reminder state after send failure: %v", saveErr)
		}
		log.Fatalf("Failed to send email: %s", err)
	}

	state.MarkReviewed(selectedClippings, now, reviewIntervals(cfg.app.ReviewIntervalsDays))
	if err := saveReminderState(cfg.app.StatePath, state); err != nil {
		log.Fatalf("Failed to save reminder state: %v", err)
	}

	log.Println("Email sent successfully!")
}

func parseConfig() config {
	cfg := config{}
	cfg.configPath = configPathFromArgs(os.Args[1:])

	if cfg.configPath != "" {
		fileConfig, err := loadAppConfig(cfg.configPath)
		if err != nil {
			log.Fatalf("Failed to load config: %v", err)
		}
		cfg.app = fileConfig
	} else {
		cfg.app = defaultAppConfig()
	}

	flag.StringVar(&cfg.configPath, "config", cfg.configPath, "path to JSON config file")
	flag.StringVar(&cfg.app.ClippingsPath, "clippings", cfg.app.ClippingsPath, "path to Kindle clippings file")
	flag.IntVar(&cfg.app.Count, "count", cfg.app.Count, "number of highlights to include")
	flag.StringVar(&cfg.app.StatePath, "state", cfg.app.StatePath, "path to reminder state file")
	flag.StringVar(&cfg.app.TemplatePath, "template", cfg.app.TemplatePath, "path to HTML email template")
	flag.StringVar(&cfg.app.PreviewPath, "preview", cfg.app.PreviewPath, "write rendered HTML email to this path instead of sending")
	flag.BoolVar(&cfg.app.DryRun, "dry-run", cfg.app.DryRun, "print selected highlights without sending or updating state")
	flag.StringVar(&cfg.serve, "serve", "", "run the web app at this address (for example :8080)")
	flag.StringVar(&cfg.importPath, "import", "", "import clippings through a running server; use - for stdin")
	flag.StringVar(&cfg.importURL, "import-url", "http://127.0.0.1:8080/api/import", "running server import endpoint")
	flag.Parse()
	return cfg
}

func configPathFromArgs(args []string) string {
	for idx, arg := range args {
		if arg == "--config" && idx+1 < len(args) {
			return args[idx+1]
		}
		if strings.HasPrefix(arg, "--config=") {
			return strings.TrimPrefix(arg, "--config=")
		}
	}
	return ""
}

func printDryRun(subject string, selected []Clipping, state ReminderState, now time.Time) {
	fmt.Printf("Subject: %s\n", subject)
	fmt.Printf("Due highlights selected at %s:\n", now.UTC().Format(time.RFC3339))
	for idx, clipping := range selected {
		review, _ := state.Review(clipping)
		fmt.Printf("%d. %s - %s\n", idx+1, clipping.Title, clipping.Author)
		fmt.Printf("   id: %s\n", clipping.ID)
		fmt.Printf("   sent: %d\n", review.SendCount)
		if review.NextDueAt != "" {
			fmt.Printf("   next due: %s\n", review.NextDueAt)
		}
		if metadata := clippingMetadata(clipping); metadata != "" {
			fmt.Printf("   %s\n", metadata)
		}
	}
}
