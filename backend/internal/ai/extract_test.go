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
  <strong>Jane Doe</strong> — Chief Executive Officer<br>
  <em>John Smith</em> — CTO
</p>
<script>console.log('ignored')</script>
<noscript>Please enable JavaScript</noscript>
</body></html>`

	got, err := stripHTMLToText(raw)
	if err != nil {
		t.Fatalf("stripHTMLToText: %v", err)
	}
	for _, needle := range []string{"Our team", "Jane Doe", "Chief Executive Officer", "John Smith", "CTO"} {
		if !strings.Contains(got, needle) {
			t.Errorf("stripped text missing %q; got %q", needle, got)
		}
	}
	for _, forbidden := range []string{"alert", "xss", "color: red", "console.log", "JavaScript"} {
		if strings.Contains(got, forbidden) {
			t.Errorf("stripped text contains %q; got %q", forbidden, got)
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
