package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/crosszan/modu/repos/notebooklm"
	vo "github.com/crosszan/modu/vo/notebooklm_vo"
)

const usage = `NotebookLM CLI - Unofficial Google NotebookLM client

Usage: notebooklm <command> [options]

Commands:
  login              Login to Google account (browser-based)
  status             Check authentication status
  list               List all notebooks
  create <title>     Create a new notebook
  delete <id>        Delete a notebook
  rename <id> <name> Rename a notebook

  source add <nb> <url>       Add URL source to notebook
  source text <nb> <title>    Add text source (reads from stdin)
  source delete <nb> <src>    Delete source from notebook

  ask <nb> <question>         Ask a question to the notebook

  audio <nb>                  Generate audio podcast
  video <nb>                  Generate video

Options:
  -format string    Output format: text, json (default "text")
  -storage string   Path to storage file
  -help             Show this help
`

func main() {
	// Global flags
	format := flag.String("format", "text", "Output format: text, json")
	storagePath := flag.String("storage", "", "Path to storage file")
	help := flag.Bool("help", false, "Show help")

	flag.Parse()

	if *help || flag.NArg() == 0 {
		fmt.Print(usage)
		os.Exit(0)
	}

	args := flag.Args()
	cmd := args[0]

	switch cmd {
	case "login":
		doLogin()
	case "status":
		doStatus(*storagePath)
	case "list":
		doList(*storagePath, *format)
	case "create":
		if len(args) < 2 {
			fatal("Usage: notebooklm create <title>")
		}
		doCreate(*storagePath, args[1], *format)
	case "delete":
		if len(args) < 2 {
			fatal("Usage: notebooklm delete <notebook_id>")
		}
		doDelete(*storagePath, args[1])
	case "rename":
		if len(args) < 3 {
			fatal("Usage: notebooklm rename <notebook_id> <new_title>")
		}
		doRename(*storagePath, args[1], strings.Join(args[2:], " "))
	case "source":
		if len(args) < 2 {
			fatal("Usage: notebooklm source <add|text|delete> ...")
		}
		doSource(*storagePath, args[1:], *format)
	case "ask":
		if len(args) < 3 {
			fatal("Usage: notebooklm ask <notebook_id> <question>")
		}
		doAsk(*storagePath, args[1], strings.Join(args[2:], " "), *format)
	case "audio":
		if len(args) < 2 {
			fatal("Usage: notebooklm audio <notebook_id>")
		}
		doAudio(*storagePath, args[1], *format)
	case "video":
		if len(args) < 2 {
			fatal("Usage: notebooklm video <notebook_id>")
		}
		doVideo(*storagePath, args[1], *format)
	default:
		fatal("Unknown command: " + cmd)
	}
}

func fatal(msg string) {
	fmt.Fprintln(os.Stderr, "Error:", msg)
	os.Exit(1)
}

func getClient(storagePath string) *notebooklm.Client {
	client, err := notebooklm.NewClientFromStorage(storagePath)
	if err != nil {
		fatal("Not logged in. Run 'notebooklm login' first.\n" + err.Error())
	}
	return client
}

func doLogin() {
	if err := notebooklm.Login(); err != nil {
		fatal(err.Error())
	}
	fmt.Println("Login successful!")
}

func doStatus(storagePath string) {
	if !notebooklm.StorageExists() {
		fmt.Println("Status: Not logged in")
		fmt.Printf("Storage path: %s (not found)\n", notebooklm.GetStoragePath())
		return
	}

	client := getClient(storagePath)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := client.RefreshTokens(ctx); err != nil {
		fmt.Println("Status: Session expired")
		fmt.Println("Run 'notebooklm login' to re-authenticate")
		return
	}

	fmt.Println("Status: Authenticated")
	fmt.Printf("Storage path: %s\n", notebooklm.GetStoragePath())
}

func doList(storagePath, format string) {
	client := getClient(storagePath)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	notebooks, err := client.ListNotebooks(ctx)
	if err != nil {
		fatal(err.Error())
	}

	if format == "json" {
		data, _ := json.MarshalIndent(notebooks, "", "  ")
		fmt.Println(string(data))
		return
	}

	if len(notebooks) == 0 {
		fmt.Println("No notebooks found")
		return
	}

	fmt.Printf("Found %d notebook(s):\n\n", len(notebooks))
	for _, nb := range notebooks {
		fmt.Printf("  ID:      %s\n", nb.ID)
		fmt.Printf("  Title:   %s\n", nb.Title)
		if nb.SourceCount > 0 {
			fmt.Printf("  Sources: %d\n", nb.SourceCount)
		}
		fmt.Println()
	}
}

func doCreate(storagePath, title, format string) {
	client := getClient(storagePath)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	nb, err := client.CreateNotebook(ctx, title)
	if err != nil {
		fatal(err.Error())
	}

	if format == "json" {
		data, _ := json.MarshalIndent(nb, "", "  ")
		fmt.Println(string(data))
		return
	}

	fmt.Printf("Created notebook:\n")
	fmt.Printf("  ID:    %s\n", nb.ID)
	fmt.Printf("  Title: %s\n", nb.Title)
}

func doDelete(storagePath, notebookID string) {
	client := getClient(storagePath)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := client.DeleteNotebook(ctx, notebookID); err != nil {
		fatal(err.Error())
	}

	fmt.Println("Notebook deleted")
}

func doRename(storagePath, notebookID, newTitle string) {
	client := getClient(storagePath)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := client.RenameNotebook(ctx, notebookID, newTitle); err != nil {
		fatal(err.Error())
	}

	fmt.Printf("Notebook renamed to: %s\n", newTitle)
}

func doSource(storagePath string, args []string, format string) {
	if len(args) == 0 {
		fatal("Usage: notebooklm source <add|text|delete> ...")
	}

	client := getClient(storagePath)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	switch args[0] {
	case "add":
		if len(args) < 3 {
			fatal("Usage: notebooklm source add <notebook_id> <url>")
		}
		source, err := client.AddSourceURL(ctx, args[1], args[2])
		if err != nil {
			fatal(err.Error())
		}
		printSource(source, format)

	case "text":
		if len(args) < 3 {
			fatal("Usage: notebooklm source text <notebook_id> <title>\nContent is read from stdin")
		}
		// Read content from stdin
		var content strings.Builder
		buf := make([]byte, 4096)
		for {
			n, err := os.Stdin.Read(buf)
			if n > 0 {
				content.Write(buf[:n])
			}
			if err != nil {
				break
			}
		}
		source, err := client.AddSourceText(ctx, args[1], args[2], content.String())
		if err != nil {
			fatal(err.Error())
		}
		printSource(source, format)

	case "delete":
		if len(args) < 3 {
			fatal("Usage: notebooklm source delete <notebook_id> <source_id>")
		}
		if err := client.DeleteSource(ctx, args[1], args[2]); err != nil {
			fatal(err.Error())
		}
		fmt.Println("Source deleted")

	default:
		fatal("Unknown source command: " + args[0])
	}
}

func printSource(source *vo.Source, format string) {
	if format == "json" {
		data, _ := json.MarshalIndent(source, "", "  ")
		fmt.Println(string(data))
		return
	}

	fmt.Printf("Source added:\n")
	fmt.Printf("  ID:    %s\n", source.ID)
	fmt.Printf("  Title: %s\n", source.Title)
	fmt.Printf("  Type:  %s\n", source.SourceType)
	if source.URL != "" {
		fmt.Printf("  URL:   %s\n", source.URL)
	}
}

func doAsk(storagePath, notebookID, question, format string) {
	client := getClient(storagePath)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	result, err := client.Ask(ctx, notebookID, question, nil)
	if err != nil {
		fatal(err.Error())
	}

	if format == "json" {
		data, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(data))
		return
	}

	fmt.Println(result.Answer)
}

func doAudio(storagePath, notebookID, format string) {
	client := getClient(storagePath)
	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Second)
	defer cancel()

	fmt.Fprintln(os.Stderr, "Starting audio generation...")

	status, err := client.GenerateAudio(ctx, notebookID, vo.AudioFormatDeepDive, vo.AudioLengthDefault)
	if err != nil {
		fatal(err.Error())
	}

	fmt.Fprintf(os.Stderr, "Task ID: %s\n", status.TaskID)
	fmt.Fprintln(os.Stderr, "Polling for completion...")

	// Poll until complete
	for status.Status != "completed" && status.Status != "failed" {
		time.Sleep(5 * time.Second)
		status, err = client.PollGeneration(ctx, notebookID, status.TaskID)
		if err != nil {
			fatal(err.Error())
		}
		fmt.Fprintf(os.Stderr, "Status: %s\n", status.Status)
	}

	if format == "json" {
		data, _ := json.MarshalIndent(status, "", "  ")
		fmt.Println(string(data))
		return
	}

	if status.Status == "completed" {
		fmt.Println("Audio generation completed!")
		if status.DownloadURL != "" {
			fmt.Printf("Download URL: %s\n", status.DownloadURL)
		}
	} else {
		fmt.Println("Audio generation failed")
		if status.Error != "" {
			fmt.Printf("Error: %s\n", status.Error)
		}
	}
}

func doVideo(storagePath, notebookID, format string) {
	client := getClient(storagePath)
	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Second)
	defer cancel()

	fmt.Fprintln(os.Stderr, "Starting video generation...")

	status, err := client.GenerateVideo(ctx, notebookID, vo.VideoFormatBriefing, vo.VideoStyleClassroom)
	if err != nil {
		fatal(err.Error())
	}

	fmt.Fprintf(os.Stderr, "Task ID: %s\n", status.TaskID)
	fmt.Fprintln(os.Stderr, "Polling for completion...")

	// Poll until complete
	for status.Status != "completed" && status.Status != "failed" {
		time.Sleep(5 * time.Second)
		status, err = client.PollGeneration(ctx, notebookID, status.TaskID)
		if err != nil {
			fatal(err.Error())
		}
		fmt.Fprintf(os.Stderr, "Status: %s\n", status.Status)
	}

	if format == "json" {
		data, _ := json.MarshalIndent(status, "", "  ")
		fmt.Println(string(data))
		return
	}

	if status.Status == "completed" {
		fmt.Println("Video generation completed!")
		if status.DownloadURL != "" {
			fmt.Printf("Download URL: %s\n", status.DownloadURL)
		}
	} else {
		fmt.Println("Video generation failed")
		if status.Error != "" {
			fmt.Printf("Error: %s\n", status.Error)
		}
	}
}
