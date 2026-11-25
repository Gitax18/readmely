// Package mail sends the few emails the product needs: sign-in codes, a welcome, and notices.
//
// Sending never happens on the request path: handlers put a message on a bounded Queue and
// answer at once, so a slow or failing provider cannot slow sign-in or reveal, through timing,
// whether an address has an account (audit F15).
//
// The only mailers so far are for development: LogMailer writes the email to the log and
// FileMailer writes it to a directory, where the browser tests read it. Both expose sign-in
// codes, and the configuration refuses them in production; a real provider is added before
// the MVP release.
package mail

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Message is one email: plain text always, and HTML when it has a designed version (see Brand).
type Message struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Text    string `json:"text"`
	HTML    string `json:"html,omitempty"`
}

// Mailer delivers one message.
type Mailer interface {
	Send(ctx context.Context, m Message) error
}

// LogMailer writes each email, body included, to the log. Development only.
type LogMailer struct{ Logger *slog.Logger }

// Send logs the message.
func (l LogMailer) Send(ctx context.Context, m Message) error {
	l.Logger.InfoContext(ctx, "email (development mailer, not sent)", "to", m.To, "subject", m.Subject, "text", m.Text)
	return nil
}

// FileMailer writes each email as a JSON file in Dir. Development and browser tests only.
type FileMailer struct{ Dir string }

type fileMessage struct {
	Message
	SentAt time.Time `json:"sent_at"`
}

// Send writes the message to a new file. The file appears complete or not at all (written
// under a temporary name, then renamed), and only the owner can read it.
func (f FileMailer) Send(_ context.Context, m Message) error {
	if err := os.MkdirAll(f.Dir, 0o700); err != nil {
		return fmt.Errorf("mail directory: %w", err)
	}
	body, err := json.MarshalIndent(fileMessage{Message: m, SentAt: time.Now().UTC()}, "", "  ")
	if err != nil {
		return err
	}
	name := strconv.FormatInt(time.Now().UnixNano(), 10) + "-" + rand.Text()[:8]
	tmp := filepath.Join(f.Dir, "."+name+".tmp")
	if err := os.WriteFile(tmp, body, 0o600); err != nil {
		return fmt.Errorf("write mail file: %w", err)
	}
	if err := os.Rename(tmp, filepath.Join(f.Dir, name+".json")); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("write mail file: %w", err)
	}
	return nil
}

// ErrUnknownMailer is returned by New for a name it does not know.
var ErrUnknownMailer = errors.New("unknown mailer")

// New returns the mailer named by the configuration ("log" or "file").
func New(name, dir string, logger *slog.Logger) (Mailer, error) {
	switch name {
	case "log":
		return LogMailer{Logger: logger}, nil
	case "file":
		if dir == "" {
			return nil, errors.New("the file mailer needs a directory")
		}
		return FileMailer{Dir: dir}, nil
	}
	return nil, ErrUnknownMailer
}
