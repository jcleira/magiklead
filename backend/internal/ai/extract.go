package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"golang.org/x/net/html"
)

// ExtractedPerson is one record pulled out of a company team or about
// page. Email is optional — present only when the page lists a real
// per-person address; pattern-guessing is banned (research §3) so a
// missing email here means we don't know one.
type ExtractedPerson struct {
	Name    string `json:"name"`
	Title   string `json:"title"`
	Company string `json:"company"`
	Email   string `json:"email,omitempty"`
}

const extractPeopleSystemPrompt = `You read a company's team or about page and return the executives and staff mentioned.

Return ONLY a JSON array with this shape:
[{"name": "Full Name", "title": "Job Title", "company": "Company Name", "email": "person@domain.com"}, ...]

Rules:
- Include only people with both a clear name AND a clear job title.
- Skip generic placeholders like "The Team", "Our People", "Join us", etc.
- Skip photo captions without a title.
- Include "email" ONLY when the page shows an address tied to a specific named individual (mailto: link in their bio, byline, footer attribution, etc.). Omit the field entirely otherwise. NEVER guess or pattern-construct an email.
- Skip generic shared inboxes — info@, contact@, sales@, hello@, support@, press@, careers@, jobs@, hr@, admin@, noreply@, no-reply@, marketing@, team@, ops@, office@, help@, general@.
- If no people are mentioned, return [].
- No markdown, no commentary, just the JSON array.`

// ExtractPeopleFromHTML strips the HTML, caps the size to Haiku-friendly
// tokens, and asks Claude for a list of people mentioned on the page.
// `domain` is passed through to the prompt as context (so Claude knows
// which company's page it's reading).
//
// The caller is expected to know which company's page this is and to
// override the `Company` field on the returned records if the
// extraction got it wrong — we trust the seed domain more than a
// sentence somewhere on the page.
func (c *Client) ExtractPeopleFromHTML(ctx context.Context, rawHTML, domain string) ([]ExtractedPerson, error) {
	text, err := stripHTMLToText(rawHTML)
	if err != nil {
		return nil, fmt.Errorf("strip html: %w", err)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil
	}
	// Claude Haiku has ample context but input tokens cost money; cap
	// at ~15k chars which comfortably fits most team pages while
	// keeping per-page cost predictable.
	const maxChars = 15000
	if len(text) > maxChars {
		text = text[:maxChars]
	}

	userMessage := fmt.Sprintf("Domain: %s\n\nPage content:\n%s", domain, text)
	resp, err := c.callClaude(ctx, extractPeopleSystemPrompt, userMessage)
	if err != nil {
		return nil, err
	}

	// Tolerate Claude prefixing/suffixing with whitespace or a code
	// fence even though the prompt forbids it.
	resp = strings.TrimSpace(resp)
	resp = strings.TrimPrefix(resp, "```json")
	resp = strings.TrimPrefix(resp, "```")
	resp = strings.TrimSuffix(resp, "```")
	resp = strings.TrimSpace(resp)

	var people []ExtractedPerson
	if err := json.Unmarshal([]byte(resp), &people); err != nil {
		return nil, fmt.Errorf("parse extraction JSON (response was %q): %w", truncate(resp, 200), err)
	}

	// Clean up common noise: trim whitespace, drop rows that ended up
	// empty, normalise emails and discard generic shared inboxes that
	// slip past the prompt.
	out := people[:0]
	for _, p := range people {
		p.Name = strings.TrimSpace(p.Name)
		p.Title = strings.TrimSpace(p.Title)
		p.Company = strings.TrimSpace(p.Company)
		p.Email = cleanExtractedEmail(p.Email)
		if p.Name == "" || p.Title == "" {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

// genericEmailLocalParts are local parts we always discard. These are
// shared inboxes rather than person-tied addresses; the architecture's
// "real source" rule (research §3) applies per person, so a record
// carrying info@ tells us nothing about a specific human.
var genericEmailLocalParts = map[string]struct{}{
	"info":      {},
	"contact":   {},
	"sales":     {},
	"hello":     {},
	"support":   {},
	"press":     {},
	"careers":   {},
	"jobs":      {},
	"hr":        {},
	"admin":     {},
	"noreply":   {},
	"no-reply":  {},
	"marketing": {},
	"team":      {},
	"ops":       {},
	"office":    {},
	"help":      {},
	"general":   {},
}

// cleanExtractedEmail trims, lowercases, and rejects the common shared-
// inbox patterns. Returns "" for any input that isn't a clear person-
// tied email; callers treat "" as "no email known".
func cleanExtractedEmail(raw string) string {
	e := strings.ToLower(strings.TrimSpace(raw))
	if e == "" {
		return ""
	}
	at := strings.LastIndex(e, "@")
	if at < 1 || at == len(e)-1 {
		return ""
	}
	local := e[:at]
	if _, generic := genericEmailLocalParts[local]; generic {
		return ""
	}
	return e
}

// stripHTMLToText is a standalone version of scrapeWebsite's inner
// text extractor — same semantics, but accepts a raw HTML string so
// the caller can fetch the document however they want (custom headers,
// proxies, retries).
//
// Element-attribute exception: the bare text walk loses `mailto:` hrefs
// because email addresses on team pages almost always live in the
// `<a href="mailto:...">` rather than in visible text. We surface the
// address inline next to the link's anchor text so the downstream LLM
// can associate "Jane Doe" with "jane@acme.com" without any HTML
// awareness of its own.
func stripHTMLToText(raw string) (string, error) {
	doc, err := html.Parse(strings.NewReader(raw))
	if err != nil {
		return "", err
	}
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && (n.Data == "script" || n.Data == "style" || n.Data == "noscript") {
			return
		}
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, attr := range n.Attr {
				if attr.Key != "href" {
					continue
				}
				if email := mailtoAddress(attr.Val); email != "" {
					b.WriteString(email)
					b.WriteByte(' ')
				}
			}
		}
		if n.Type == html.TextNode {
			if t := strings.TrimSpace(n.Data); t != "" {
				b.WriteString(t)
				b.WriteByte(' ')
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return b.String(), nil
}

// mailtoAddress returns the bare email from a `mailto:` href, stripping
// any `?subject=`/`?body=` query string. Returns "" for any non-mailto
// href so callers can use the empty string as a "skip" signal.
func mailtoAddress(href string) string {
	const prefix = "mailto:"
	if !strings.HasPrefix(strings.ToLower(href), prefix) {
		return ""
	}
	rest := href[len(prefix):]
	if i := strings.IndexAny(rest, "?#"); i >= 0 {
		rest = rest[:i]
	}
	return strings.TrimSpace(rest)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
