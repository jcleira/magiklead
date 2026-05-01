// Package email provides a minimal SMTP-based transactional sender
// for system-originated emails (e.g. GDPR erasure confirmation). It's
// intentionally separate from internal/gmail and internal/worker's
// per-tenant sender: those deliver on behalf of a customer; this one
// delivers on behalf of MagikLead itself.
package email

import (
	"fmt"
	"log"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is resolved from env at construction. An empty Host means
// "not configured" — in that case Send logs the message and returns
// nil so local dev doesn't crash.
type Config struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
}

type Sender struct {
	cfg Config
}

func NewFromEnv() *Sender {
	port, _ := strconv.Atoi(os.Getenv("SYSTEM_SMTP_PORT"))
	if port == 0 {
		port = 587
	}
	return &Sender{cfg: Config{
		Host:     strings.TrimSpace(os.Getenv("SYSTEM_SMTP_HOST")),
		Port:     port,
		Username: strings.TrimSpace(os.Getenv("SYSTEM_SMTP_USER")),
		Password: os.Getenv("SYSTEM_SMTP_PASS"),
		From:     strings.TrimSpace(os.Getenv("SYSTEM_SMTP_FROM")),
	}}
}

// Send delivers a plain-text + light HTML message to `to`.
// Returns an error only on SMTP failure when configured; an
// unconfigured sender logs the mail and returns nil.
func (s *Sender) Send(to, subject, body string) error {
	if s.cfg.Host == "" || s.cfg.From == "" {
		log.Printf("[email] (dev) would send to=%q subject=%q body=%q", to, subject, body)
		return nil
	}

	var msg strings.Builder
	msg.WriteString(fmt.Sprintf("From: %s\r\n", s.cfg.From))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", to))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	msg.WriteString(fmt.Sprintf("Date: %s\r\n", time.Now().Format(time.RFC1123Z)))
	msg.WriteString("\r\n")
	msg.WriteString(body)

	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	auth := smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)
	return smtp.SendMail(addr, auth, s.cfg.From, []string{to}, []byte(msg.String()))
}
