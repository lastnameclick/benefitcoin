// Package mail sends monthly statement emails over SMTP. It's a best-effort
// notification layer only: the in-app Inbox (see internal/statement and the
// statements store) is the guaranteed delivery channel, so a deployment with
// no SMTP configured still works end to end.
package mail

import (
	"bytes"
	"fmt"
	"strings"

	"cpal/internal/config"

	gomail "github.com/wneessen/go-mail"
)

// Sender delivers statement PDFs by email.
type Sender struct {
	cfg config.SMTPConfig
}

// New returns a Sender, or (nil, false) if SMTP isn't configured — callers
// should treat that as "skip emailing," not an error.
func New(cfg config.SMTPConfig) (*Sender, bool) {
	if !cfg.Configured() {
		return nil, false
	}
	return &Sender{cfg: cfg}, true
}

// SendStatement emails a PDF statement to one recipient.
func (s *Sender) SendStatement(to, subject, body, filename string, pdf []byte) error {
	msg := gomail.NewMsg()
	if err := msg.From(s.cfg.FromAddress); err != nil {
		return fmt.Errorf("from address: %w", err)
	}
	if err := msg.To(to); err != nil {
		return fmt.Errorf("to address: %w", err)
	}
	msg.Subject(subject)
	msg.SetBodyString(gomail.TypeTextPlain, body)
	msg.AttachReadSeeker(filename, bytes.NewReader(pdf))

	opts := []gomail.Option{gomail.WithPort(s.cfg.Port)}
	if s.cfg.Username != "" {
		opts = append(opts,
			gomail.WithSMTPAuth(gomail.SMTPAuthAutoDiscover),
			gomail.WithUsername(s.cfg.Username),
			gomail.WithPassword(s.cfg.Password),
		)
	}
	client, err := gomail.NewClient(s.cfg.Host, opts...)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	return client.DialAndSend(msg)
}

// LooksLikeAddress is a light sanity check for "is this identity username
// even worth trying to email" — not full RFC validation, which happens at
// delivery time. Holders' usernames are plain operator-chosen strings, so
// most won't pass this; only identities with an email-shaped username (in
// practice, operators — signup requires one) are considered emailable.
func LooksLikeAddress(s string) bool {
	at := strings.IndexByte(s, '@')
	if at <= 0 || at == len(s)-1 {
		return false
	}
	return strings.IndexByte(s[at+1:], '.') > 0
}
