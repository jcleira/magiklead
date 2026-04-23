// Package leads — see smtp_verify.go for the email-verification pipeline.
//
// Design follows research.md §3 "Email Verification Pipeline":
//
//  1. MX lookup → cached in `domains` table so we don't re-resolve daily.
//  2. Catch-all probe: send RCPT TO to a random local-part at the domain.
//     If accepted, the domain is catch-all and individual verification is
//     meaningless; mark the domain and skip per-address checks forever.
//  3. For non-catch-all domains, RCPT TO the real address. Interpret a
//     5xx response as "rejected" (user doesn't exist) and anything else
//     as a temporary failure (retry later).
//
// Pattern-guessed emails never enter the pipeline — this verifier only
// confirms addresses that already exist in the `emails` table.
package leads

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jcleira/magiklead/backend/internal/repository"
)

// SMTPVerifier implements the catch-all-aware email verification
// pipeline. It caches MX records and catch-all determinations in the
// `domains` table so repeat work is free.
type SMTPVerifier struct {
	queries *repository.Queries
	hello   string
	from    string
	timeout time.Duration
	domainCacheTTL time.Duration
}

// NewSMTPVerifier wires a verifier with the HELO hostname used on SMTP
// handshakes and the MAIL FROM envelope sender used for probes.
func NewSMTPVerifier(q *repository.Queries, helo, mailFrom string) *SMTPVerifier {
	return &SMTPVerifier{
		queries:        q,
		hello:          helo,
		from:           mailFrom,
		timeout:        20 * time.Second,
		domainCacheTTL: 30 * 24 * time.Hour,
	}
}

// Result is the outcome of a single Verify() call.
type Result struct {
	Email      string
	Verified   bool   // true when an MX server explicitly accepted RCPT TO <email>
	IsCatchAll bool   // true when the domain accepts any local-part; Verified is meaningless
	Method     string // "smtp-rcpt", "catchall", "no-mx", "invalid", "smtp-error"
	MXRecords  []string
	Reason     string // free-form detail for diagnostics / bounce logs
}

// Verify returns the verification outcome for one email. Never returns
// an error for "user doesn't exist" or "domain has no MX" — those are
// normal outcomes encoded in Result. Errors are reserved for DB /
// context failures that callers should actually handle.
func (v *SMTPVerifier) Verify(ctx context.Context, email string) (*Result, error) {
	addr, err := mail.ParseAddress(email)
	if err != nil {
		return &Result{Email: email, Method: "invalid", Reason: err.Error()}, nil
	}
	at := strings.LastIndexByte(addr.Address, '@')
	if at < 0 {
		return &Result{Email: email, Method: "invalid", Reason: "missing @"}, nil
	}
	domain := strings.ToLower(addr.Address[at+1:])

	d, err := v.ensureDomain(ctx, domain)
	if err != nil {
		return nil, err
	}
	if len(d.MxRecords) == 0 {
		return &Result{Email: email, Method: "no-mx", Reason: "domain has no MX records"}, nil
	}

	// Catch-all known and set → short-circuit without hitting SMTP again.
	if d.IsCatchall.Valid && d.IsCatchall.Bool {
		return &Result{
			Email: email, IsCatchAll: true, Method: "catchall",
			MXRecords: d.MxRecords,
			Reason:    "domain previously confirmed catch-all",
		}, nil
	}

	// Catch-all unknown → probe.
	if !d.IsCatchall.Valid {
		accepted, probeReason, probeErr := v.checkRCPT(ctx, d.MxRecords, randomLocalPart()+"@"+domain)
		if probeErr != nil {
			return &Result{Email: email, Method: "smtp-error", MXRecords: d.MxRecords, Reason: probeErr.Error()}, nil
		}
		if err := v.queries.UpdateDomainCatchall(ctx, repository.UpdateDomainCatchallParams{
			Domain:     domain,
			IsCatchall: pgtype.Bool{Bool: accepted, Valid: true},
		}); err != nil {
			return nil, err
		}
		if accepted {
			return &Result{
				Email: email, IsCatchAll: true, Method: "catchall",
				MXRecords: d.MxRecords,
				Reason:    "catch-all probe accepted: " + probeReason,
			}, nil
		}
	}

	// Non-catch-all → real RCPT TO.
	accepted, reason, err := v.checkRCPT(ctx, d.MxRecords, email)
	if err != nil {
		return &Result{Email: email, Method: "smtp-error", MXRecords: d.MxRecords, Reason: err.Error()}, nil
	}
	return &Result{
		Email:     email,
		Verified:  accepted,
		Method:    "smtp-rcpt",
		MXRecords: d.MxRecords,
		Reason:    reason,
	}, nil
}

// ensureDomain loads the cached domain row, refreshing from DNS when
// the row is missing or stale.
func (v *SMTPVerifier) ensureDomain(ctx context.Context, domain string) (repository.Domain, error) {
	existing, err := v.queries.GetDomainByName(ctx, domain)
	if err == nil && len(existing.MxRecords) > 0 && existing.LastVerifiedAt.Valid {
		if time.Since(existing.LastVerifiedAt.Time) < v.domainCacheTTL {
			return existing, nil
		}
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return repository.Domain{}, err
	}

	mxs, _ := net.LookupMX(domain)
	hosts := make([]string, 0, len(mxs))
	// LookupMX returns hosts sorted by preference; persist in the same
	// order so checkRCPT tries the preferred one first.
	sort.Slice(mxs, func(i, j int) bool { return mxs[i].Pref < mxs[j].Pref })
	for _, m := range mxs {
		hosts = append(hosts, strings.TrimSuffix(m.Host, "."))
	}

	// Preserve a previously-stored catch-all verdict; a fresh MX lookup
	// doesn't invalidate it.
	catchall := pgtype.Bool{Valid: false}
	if existing.IsCatchall.Valid {
		catchall = existing.IsCatchall
	}

	updated, err := v.queries.UpsertDomain(ctx, repository.UpsertDomainParams{
		Domain:     domain,
		MxRecords:  hosts,
		IsCatchall: catchall,
	})
	return updated, err
}

// checkRCPT tries each MX host in order, returning (accepted, reason,
// err). accepted=true means the server explicitly accepted RCPT TO for
// the given address. A 5xx response means explicit rejection
// (accepted=false, no error). Transport errors keep trying other MX
// hosts before giving up.
func (v *SMTPVerifier) checkRCPT(ctx context.Context, mxHosts []string, target string) (bool, string, error) {
	var lastErr error
	for _, host := range mxHosts {
		addr := host + ":25"
		conn, err := (&net.Dialer{Timeout: v.timeout}).DialContext(ctx, "tcp", addr)
		if err != nil {
			lastErr = err
			continue
		}
		accepted, reason, smtpErr := rcptOnConn(conn, host, v.hello, v.from, target, v.timeout)
		_ = conn.Close()
		if smtpErr == nil {
			return accepted, reason, nil
		}
		// 4xx temp failures on one MX often work on another; keep trying.
		lastErr = smtpErr
	}
	if lastErr == nil {
		lastErr = errors.New("no MX hosts available")
	}
	return false, "", lastErr
}

// rcptOnConn runs HELO → MAIL FROM → RCPT TO on an open connection.
// Returns (accepted, reason, err). A 5xx on RCPT TO is a clean
// "not accepted" (err is nil). Anything else is a transport/protocol
// error the caller should retry or give up on.
func rcptOnConn(conn net.Conn, host, helo, from, target string, timeout time.Duration) (bool, string, error) {
	_ = conn.SetDeadline(time.Now().Add(timeout))
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return false, "", fmt.Errorf("smtp handshake: %w", err)
	}
	defer func() { _ = client.Close() }()

	if err := client.Hello(helo); err != nil {
		return false, "", fmt.Errorf("HELO: %w", err)
	}
	if err := client.Mail(from); err != nil {
		return false, "", fmt.Errorf("MAIL FROM: %w", err)
	}
	if err := client.Rcpt(target); err != nil {
		msg := err.Error()
		// net/smtp stringifies 5xx errors with the code in position 0-2.
		if len(msg) >= 3 && msg[0] == '5' {
			return false, msg, nil
		}
		return false, msg, fmt.Errorf("RCPT TO: %w", err)
	}
	return true, "accepted", nil
}

// randomLocalPart returns a 12-char hex string used as the local-part
// of catch-all probe addresses. crypto/rand gives us enough entropy
// that collisions with real mailboxes are astronomically unlikely.
func randomLocalPart() string {
	var buf [6]byte
	_, _ = rand.Read(buf[:])
	return "probe-" + hex.EncodeToString(buf[:])
}
