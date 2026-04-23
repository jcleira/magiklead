package leads

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// CompanyTarget is a company identified by AI as a good prospecting target.
type CompanyTarget struct {
	Name   string `json:"name"`
	Domain string `json:"domain"`
}

// FindTargetCompanies uses Claude to identify companies whose employees match the ICP.
func FindTargetCompanies(ctx context.Context, anthropicKey, product, titles, industry, location string) ([]CompanyTarget, error) {
	prompt := fmt.Sprintf(`I'm selling: %s

I want to cold-email people with these titles: %s
Market/industry: %s
Location: %s

List 10 specific real companies I should target. For each, give the company name and their website domain.

Rules:
- These must be companies whose EMPLOYEES would BUY my product
- NOT software platforms or tools my ICP uses
- Real companies with 20-5000 employees
- No mega-corps (no Google, Apple, Microsoft, Amazon, Meta, etc)
- Mix of well-known and mid-market companies
- Must be real companies that exist right now

Return ONLY valid JSON array, no other text:
[{"name": "Company Name", "domain": "company.com"}, ...]`, product, titles, industry, location)

	reqBody, _ := json.Marshal(map[string]any{
		"model":      "claude-haiku-4-5-20251001",
		"max_tokens": 512,
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
		return nil, fmt.Errorf("claude request: %w", err)
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
		return nil, fmt.Errorf("parse claude response: %w", err)
	}

	text := strings.TrimSpace(result.Content[0].Text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	var companies []CompanyTarget
	if err := json.Unmarshal([]byte(text), &companies); err != nil {
		return nil, fmt.Errorf("parse companies JSON: %w (text: %s)", err, text[:min(len(text), 200)])
	}

	// Filter out empty entries
	var valid []CompanyTarget
	for _, c := range companies {
		if c.Domain != "" && c.Name != "" {
			valid = append(valid, c)
		}
	}

	log.Printf("AI selected %d target companies", len(valid))
	return valid, nil
}
