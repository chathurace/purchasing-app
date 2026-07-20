// Package email sends notification mail for the purchasing app. It is
// deliberately small: a Mailer interface with an SMTP implementation for real
// deployments and a logging no-op for local development, where no SMTP server
// is configured. Callers send best-effort — a failed send must never block or
// fail the user action that triggered it.
package email

import (
	"context"
	"fmt"
	"net/smtp"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

// Mailer sends a plain-text email to a recipient (with optional Cc). Implementations
// must be safe for concurrent use.
type Mailer interface {
	// Send delivers a message. It blocks until the send completes (or fails), so
	// callers that don't want to wait should invoke it in a goroutine.
	Send(ctx context.Context, to, subject, body string) error
	// SendCC is Send with additional Cc recipients (empty/nil cc == Send). The
	// message is delivered to the primary recipient and every cc address.
	SendCC(ctx context.Context, to string, cc []string, subject, body string) error
}

// Config is the email section of the app config.
type Config struct {
	Enabled     bool   `yaml:"enabled"`
	SMTPHost    string `yaml:"smtp_host"`
	SMTPPort    int    `yaml:"smtp_port"`
	Username    string `yaml:"username"`
	Password    string `yaml:"password"`
	FromAddress string `yaml:"from_address"`
}

// New returns an SMTP mailer when email is enabled and a host is configured;
// otherwise a LogMailer that records what *would* have been sent. This lets the
// app run end-to-end in development without an SMTP server.
func New(cfg Config, log zerolog.Logger) Mailer {
	if cfg.Enabled && strings.TrimSpace(cfg.SMTPHost) != "" {
		log.Info().Str("host", cfg.SMTPHost).Int("port", cfg.SMTPPort).Msg("email: SMTP mailer enabled")
		return &SMTPMailer{cfg: cfg, log: log}
	}
	log.Info().Msg("email: disabled — notifications will be logged, not sent")
	return &LogMailer{log: log}
}

// LogMailer logs each message instead of sending it. Used in development.
type LogMailer struct {
	log zerolog.Logger
}

func (m *LogMailer) Send(ctx context.Context, to, subject, body string) error {
	return m.SendCC(ctx, to, nil, subject, body)
}

func (m *LogMailer) SendCC(_ context.Context, to string, cc []string, subject, body string) error {
	m.log.Info().Str("to", to).Strs("cc", cc).Str("subject", subject).Msg("email (not sent — mailer disabled)")
	return nil
}

// SMTPMailer sends real mail via net/smtp.
type SMTPMailer struct {
	cfg Config
	log zerolog.Logger
}

func (m *SMTPMailer) Send(ctx context.Context, to, subject, body string) error {
	return m.SendCC(ctx, to, nil, subject, body)
}

func (m *SMTPMailer) SendCC(ctx context.Context, to string, cc []string, subject, body string) error {
	addr := fmt.Sprintf("%s:%d", m.cfg.SMTPHost, m.cfg.SMTPPort)
	from := m.cfg.FromAddress
	if from == "" {
		from = m.cfg.Username
	}
	cc = cleanRecipients(to, cc)
	msg := buildMessage(from, to, cc, subject, body)
	recipients := append([]string{to}, cc...)

	var auth smtp.Auth
	if m.cfg.Username != "" {
		auth = smtp.PlainAuth("", m.cfg.Username, m.cfg.Password, m.cfg.SMTPHost)
	}

	// net/smtp has no context support; bound the attempt with a goroutine so a
	// hung server can't pin the caller indefinitely.
	done := make(chan error, 1)
	go func() { done <- smtp.SendMail(addr, auth, from, recipients, msg) }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(15 * time.Second):
		return fmt.Errorf("smtp send to %s timed out", to)
	}
}

// cleanRecipients trims/deduplicates the cc list and drops blanks and any address
// equal to the primary recipient (case-insensitive), so nobody is listed twice.
func cleanRecipients(to string, cc []string) []string {
	seen := map[string]bool{strings.ToLower(strings.TrimSpace(to)): true}
	out := []string{}
	for _, c := range cc {
		c = strings.TrimSpace(c)
		key := strings.ToLower(c)
		if c == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, c)
	}
	return out
}

func buildMessage(from, to string, cc []string, subject, body string) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", to)
	if len(cc) > 0 {
		fmt.Fprintf(&b, "Cc: %s\r\n", strings.Join(cc, ", "))
	}
	fmt.Fprintf(&b, "Subject: %s\r\n", subject)
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
	b.WriteString("\r\n")
	b.WriteString(body)
	return []byte(b.String())
}
