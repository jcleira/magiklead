package ai

import (
	"strings"
	"testing"
)

func TestStripHTMLToText(t *testing.T) {
	raw := `<html><head>
<title>Team — Acme</title>
<script>alert('xss')</script>
<style>body {color: red}</style>
</head>
<body>
<h1>Our team</h1>
<p>
  <strong>Jane Doe</strong> — Chief Executive Officer
  (<a href="mailto:jane.doe@acme.com?subject=Hi">email Jane</a>)<br>
  <em>John Smith</em> — CTO
</p>
<script>console.log('ignored')</script>
<noscript>Please enable JavaScript</noscript>
</body></html>`

	got, err := stripHTMLToText(raw)
	if err != nil {
		t.Fatalf("stripHTMLToText: %v", err)
	}
	for _, needle := range []string{"Our team", "Jane Doe", "Chief Executive Officer", "John Smith", "CTO", "jane.doe@acme.com"} {
		if !strings.Contains(got, needle) {
			t.Errorf("stripped text missing %q; got %q", needle, got)
		}
	}
	for _, forbidden := range []string{"alert", "xss", "color: red", "console.log", "JavaScript", "?subject="} {
		if strings.Contains(got, forbidden) {
			t.Errorf("stripped text contains %q; got %q", forbidden, got)
		}
	}
}

func TestMailtoAddress(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"mailto:jane@acme.com", "jane@acme.com"},
		{"MailTo:Jane@Acme.com", "Jane@Acme.com"},
		{"mailto:jane@acme.com?subject=hi", "jane@acme.com"},
		{"mailto:jane@acme.com#frag", "jane@acme.com"},
		{"https://acme.com/contact", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := mailtoAddress(c.in); got != c.want {
			t.Errorf("mailtoAddress(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("abcde", 3); got != "abc…" {
		t.Errorf("truncate(\"abcde\",3)=%q", got)
	}
	if got := truncate("ab", 3); got != "ab" {
		t.Errorf("truncate(\"ab\",3)=%q", got)
	}
}

func TestCleanExtractedEmail(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"trim and lowercase", "  Jane.Doe@Acme.COM ", "jane.doe@acme.com"},
		{"empty stays empty", "", ""},
		{"no @ rejected", "not-an-email", ""},
		{"empty local part rejected", "@acme.com", ""},
		{"empty domain rejected", "jane@", ""},
		{"info shared inbox dropped", "info@acme.com", ""},
		{"contact shared inbox dropped", "Contact@Acme.com", ""},
		{"no-reply dropped", "no-reply@acme.com", ""},
		{"hr dropped", "hr@acme.com", ""},
		{"named address kept", "ceo.tim@acme.com", "ceo.tim@acme.com"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := cleanExtractedEmail(c.in); got != c.want {
				t.Errorf("cleanExtractedEmail(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
