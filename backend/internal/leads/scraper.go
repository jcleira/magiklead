package leads

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// ScrapedPerson is a person found on a company website.
type ScrapedPerson struct {
	Name            string `json:"name"`
	Title           string `json:"title"`
	Company         string `json:"company"`
	Domain          string `json:"domain"`
	PageURL         string `json:"page_url"`
	LinkedIN        string `json:"linkedin"`
	Email           string `json:"email"`
	EmailVerified   bool   `json:"email_verified"`
	EmailConfidence int    `json:"email_confidence"`
}

// ScrapeCompanyPeople finds people on a company's website by:
// 1. Fetching the website
// 2. Finding team/about/leadership pages
// 3. Extracting names and titles using Claude
func ScrapeCompanyPeople(ctx context.Context, anthropicKey string, company CompanyTarget, targetTitles string) ([]ScrapedPerson, error) {
	domain := company.Domain
	if !strings.HasPrefix(domain, "http") {
		domain = "https://" + domain
	}

	log.Printf("Scraping %s for team members...", company.Domain)

	// Step 1: Try common team page URLs
	teamPages := []string{
		domain + "/about",
		domain + "/about-us",
		domain + "/team",
		domain + "/our-team",
		domain + "/about/team",
		domain + "/company",
		domain + "/leadership",
		domain + "/about/leadership",
		domain + "/people",
	}

	var bestContent string
	var bestURL string

	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	for _, pageURL := range teamPages {
		content, err := fetchPageText(ctx, client, pageURL)
		if err != nil {
			continue
		}
		// Check if this page likely has people on it
		if looksLikeTeamPage(content) && len(content) > len(bestContent) {
			bestContent = content
			bestURL = pageURL
		}
	}

	// If no team page found, try the homepage
	if bestContent == "" {
		content, err := fetchPageText(ctx, client, domain)
		if err != nil {
			return nil, fmt.Errorf("could not fetch %s: %w", domain, err)
		}
		bestContent = content
		bestURL = domain
	}

	if bestContent == "" {
		return nil, fmt.Errorf("no content found at %s", company.Domain)
	}

	// Truncate for Claude
	if len(bestContent) > 6000 {
		bestContent = bestContent[:6000]
	}

	log.Printf("Found content at %s (%d chars), extracting people with AI...", bestURL, len(bestContent))

	// Step 2: Use Claude to extract people from the page content
	people, err := extractPeopleWithAI(ctx, anthropicKey, bestContent, company, targetTitles)
	if err != nil {
		return nil, fmt.Errorf("AI extraction: %w", err)
	}

	return people, nil
}

// fetchPageText fetches a URL and extracts visible text content.
func fetchPageText(ctx context.Context, client *http.Client, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 500_000))
	if err != nil {
		return "", err
	}

	return stripHTML(string(body)), nil
}

// looksLikeTeamPage checks if page content likely contains team/people info.
func looksLikeTeamPage(content string) bool {
	lower := strings.ToLower(content)
	teamSignals := []string{
		"our team", "meet the team", "leadership", "founders",
		"co-founder", "ceo", "cto", "vp of", "head of",
		"director of", "our people", "the team behind",
		"management team", "executive team",
	}
	hits := 0
	for _, signal := range teamSignals {
		if strings.Contains(lower, signal) {
			hits++
		}
	}
	return hits >= 2
}

// stripHTML removes HTML tags and extracts visible text.
var tagRegex = regexp.MustCompile(`<script[^>]*>[\s\S]*?</script>|<style[^>]*>[\s\S]*?</style>|<[^>]+>`)
var spaceRegex = regexp.MustCompile(`\s+`)

func stripHTML(html string) string {
	text := tagRegex.ReplaceAllString(html, " ")
	text = spaceRegex.ReplaceAllString(text, " ")
	return strings.TrimSpace(text)
}

// extractPeopleWithAI uses Claude to find people in website text.
func extractPeopleWithAI(ctx context.Context, anthropicKey, pageContent string, company CompanyTarget, targetTitles string) ([]ScrapedPerson, error) {
	prompt := fmt.Sprintf(`Extract all people mentioned on this company webpage. The company is "%s" (domain: %s).

I'm looking for people with titles like: %s
But include ALL people you find with any business title — I'll filter later.

For each person, extract their full name and job title exactly as shown on the page.

Website content:
---
%s
---

Return ONLY a valid JSON array. If no people are found, return an empty array [].
Format: [{"name": "Full Name", "title": "Job Title"}, ...]
No explanations, no markdown, just the JSON array.`, company.Name, company.Domain, targetTitles, pageContent)

	reqBody, _ := json.Marshal(map[string]any{
		"model":      "claude-haiku-4-5-20251001",
		"max_tokens": 1024,
		"messages":   []map[string]string{{"role": "user", "content": prompt}},
	})

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.anthropic.com/v1/messages", strings.NewReader(string(reqBody)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", anthropicKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("claude %d: %s", resp.StatusCode, string(body[:min(len(body), 200)]))
	}

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &result); err != nil || len(result.Content) == 0 {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	text := strings.TrimSpace(result.Content[0].Text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	var raw []struct {
		Name  string `json:"name"`
		Title string `json:"title"`
	}
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		return nil, fmt.Errorf("parse people JSON: %w", err)
	}

	var people []ScrapedPerson
	for _, r := range raw {
		if r.Name == "" {
			continue
		}
		people = append(people, ScrapedPerson{
			Name:    r.Name,
			Title:   r.Title,
			Company: company.Name,
			Domain:  company.Domain,
		})
	}

	log.Printf("Extracted %d people from %s", len(people), company.Domain)
	return people, nil
}
