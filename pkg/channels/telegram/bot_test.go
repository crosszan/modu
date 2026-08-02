package telegram

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestSenderDisplayName(t *testing.T) {
	tests := []struct {
		name string
		user *tgbotapi.User
		want string
	}{
		{"nil user", nil, "unknown"},
		{"username preferred", &tgbotapi.User{UserName: "alice", FirstName: "Alice"}, "@alice"},
		{"first and last name", &tgbotapi.User{FirstName: "Alice", LastName: "Smith"}, "Alice Smith"},
		{"first name only", &tgbotapi.User{FirstName: "Alice"}, "Alice"},
		{"no name falls back to user id", &tgbotapi.User{ID: 12345}, "user12345"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := senderDisplayName(tt.user); got != tt.want {
				t.Errorf("senderDisplayName(%+v) = %q, want %q", tt.user, got, tt.want)
			}
		})
	}
}

func TestEscapeMarkdownEscapesReservedCharacters(t *testing.T) {
	// Per Telegram's MarkdownV2 spec, these characters must be escaped with
	// a preceding backslash wherever they appear outside an intended entity.
	reserved := "_*[]()~`>#+-=|{}.!"
	for _, r := range reserved {
		in := "a" + string(r) + "b"
		want := "a\\" + string(r) + "b"
		if got := escapeMarkdown(in); got != want {
			t.Errorf("escapeMarkdown(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEscapeMarkdownLeavesPlainTextUnchanged(t *testing.T) {
	in := "hello world 123"
	if got := escapeMarkdown(in); got != in {
		t.Errorf("escapeMarkdown(%q) = %q, want unchanged", in, got)
	}
}

func TestStripMarkdownRemovesFormattingMarkers(t *testing.T) {
	in := "*bold* _italic_ `code` ~strike~"
	got := stripMarkdown(in)
	if strings.ContainsAny(got, "*_`~") {
		t.Errorf("stripMarkdown(%q) = %q, still contains formatting markers", in, got)
	}
}

func TestStripMarkdownPreservesPlainPunctuation(t *testing.T) {
	// stripMarkdown is a plain-text fallback for failed MarkdownV2 sends —
	// it should remove formatting syntax, not mangle ordinary punctuation
	// like periods and hyphens that are part of the actual message text.
	in := "version 1.2.3 - a fix"
	got := stripMarkdown(in)
	if got != in {
		t.Errorf("stripMarkdown(%q) = %q, want unchanged (only formatting markers should be stripped)", in, got)
	}
}

func TestAwaitApprovalRegistersAndDeliversToChat(t *testing.T) {
	b := &Bot{approvalCh: make(map[int64]chan string)}
	ch := b.AwaitApproval(42)

	b.approvalMu.Lock()
	waiter, ok := b.approvalCh[42]
	b.approvalMu.Unlock()
	if !ok {
		t.Fatal("AwaitApproval should register a channel for the chat")
	}
	waiter <- "y"

	select {
	case got := <-ch:
		if got != "y" {
			t.Errorf("received %q, want %q", got, "y")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for delivered approval")
	}
}

func TestCancelApprovalRemovesRegistration(t *testing.T) {
	b := &Bot{approvalCh: make(map[int64]chan string)}
	b.AwaitApproval(42)
	b.CancelApproval(42)

	b.approvalMu.Lock()
	_, ok := b.approvalCh[42]
	b.approvalMu.Unlock()
	if ok {
		t.Error("CancelApproval should remove the chat's registration")
	}
}

func TestCancelApprovalOnUnregisteredChatIsSafe(t *testing.T) {
	b := &Bot{approvalCh: make(map[int64]chan string)}
	b.CancelApproval(999) // must not panic
}

func TestApprovalChannelConcurrentAccess(t *testing.T) {
	b := &Bot{approvalCh: make(map[int64]chan string)}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int64) {
			defer wg.Done()
			b.AwaitApproval(i)
			b.CancelApproval(i)
		}(int64(i))
	}
	wg.Wait()
}

func TestTgContextGetters(t *testing.T) {
	c := &tgContext{
		chatID:      7,
		userID:      "user-1",
		messageText: "hello",
		messageTS:   "123",
		senderName:  "@alice",
		attachments: []string{"/tmp/a.txt"},
	}
	if c.ChatID() != 7 {
		t.Errorf("ChatID() = %d, want 7", c.ChatID())
	}
	if c.UserID() != "user-1" {
		t.Errorf("UserID() = %q, want %q", c.UserID(), "user-1")
	}
	if c.MessageText() != "hello" {
		t.Errorf("MessageText() = %q, want %q", c.MessageText(), "hello")
	}
	if c.MessageTS() != "123" {
		t.Errorf("MessageTS() = %q, want %q", c.MessageTS(), "123")
	}
	if c.SenderName() != "@alice" {
		t.Errorf("SenderName() = %q, want %q", c.SenderName(), "@alice")
	}
	if len(c.Attachments()) != 1 || c.Attachments()[0] != "/tmp/a.txt" {
		t.Errorf("Attachments() = %v, want [/tmp/a.txt]", c.Attachments())
	}
}

func TestTgContextImagesEncodesKnownImageExtensions(t *testing.T) {
	dir := t.TempDir()
	imgPath := filepath.Join(dir, "photo.png")
	data := []byte{0x89, 'P', 'N', 'G'}
	if err := os.WriteFile(imgPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	txtPath := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(txtPath, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := &tgContext{attachments: []string{imgPath, txtPath}}
	images := c.Images()
	if len(images) != 1 {
		t.Fatalf("expected 1 image (the .txt should be skipped), got %d", len(images))
	}
	if images[0].MimeType != "image/png" {
		t.Errorf("MimeType = %q, want image/png", images[0].MimeType)
	}
	wantData := base64.StdEncoding.EncodeToString(data)
	if images[0].Data != wantData {
		t.Errorf("Data = %q, want %q", images[0].Data, wantData)
	}
}

func TestTgContextImagesSkipsUnreadableFile(t *testing.T) {
	c := &tgContext{attachments: []string{"/nonexistent/path/photo.jpg"}}
	images := c.Images()
	if len(images) != 0 {
		t.Errorf("expected no images for an unreadable path, got %d", len(images))
	}
}

func TestDownloadFileSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("file contents"))
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "sub", "out.bin")
	if err := downloadFile(dest, srv.URL); err != nil {
		t.Fatalf("downloadFile: %v", err)
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("reading downloaded file: %v", err)
	}
	if string(data) != "file contents" {
		t.Errorf("downloaded content = %q, want %q", string(data), "file contents")
	}
}

func TestDownloadFileNonOKStatusReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "out.bin")
	if err := downloadFile(dest, srv.URL); err == nil {
		t.Fatal("expected an error for a non-200 response")
	}
	if _, err := os.Stat(dest); err == nil {
		t.Error("no file should be left behind for a failed download")
	}
}

func TestDownloadFileUnreachableURLReturnsError(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "out.bin")
	if err := downloadFile(dest, "http://127.0.0.1:1/unreachable"); err == nil {
		t.Fatal("expected an error for an unreachable URL")
	}
}
