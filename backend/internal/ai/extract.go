package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"golang.org/x/net/html"
)

// ExtractedPerson is one name/title/company tuple pulled out of a
// company team or about page.
type ExtractedPerson struct {
	Name    string `json:"name"`
	Title   string `json:"title"`
	Company string `json:"company"`
}

const extractPeopleSystemPrompt = `You read a company's team or about page and return the executives and staff mentioned.

Return ONLY a JSON array with this shape:
[{"name": "Full Name", "title": "Job Title", "company": "Company Name"}, ...]

Rules:
- Include only people with both a clear name AND a clear job title.
- Skip generic placeholders like "The Team", "Our People", "Join us", etc.
- Skip photo captions without a title.
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
	// empty.
	out := people[:0]
	for _, p := range people {
		p.Name = strings.TrimSpace(p.Name)
		p.Title = strings.TrimSpace(p.Title)
		p.Company = strings.TrimSpace(p.Company)
		if p.Name == "" || p.Title == "" {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

// stripHTMLToText is a standalone version of scrapeWebsite's inner
// text extractor — same semantics, but accepts a raw HTML string so
// the caller can fetch the document however they want (custom headers,
// proxies, retries).
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

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
