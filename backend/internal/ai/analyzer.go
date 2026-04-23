package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const analyzeSystemPrompt = `You are a sales intelligence AI. Analyze the following website content and extract a structured business profile.
Return ONLY valid JSON with this exact structure:
{
  "company_name": "string",
  "product_description": "string (2-3 sentences)",
  "features": ["feature1", "feature2", ...],
  "pricing": "string (e.g. '$29-199/mo')",
  "target_customers": ["customer1", "customer2", ...],
  "differentiators": ["diff1", "diff2", ...],
  "industry": "string"
}
Do not include any text outside the JSON object.`

// AnalyzeWebsite scrapes a URL and generates a business profile using Claude.
func (c *Client) AnalyzeWebsite(ctx context.Context, url string) (*BusinessProfile, error) {
	content, err := scrapeWebsite(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("scrape website: %w", err)
	}

	if strings.TrimSpace(content) == "" {
		return nil, fmt.Errorf("no content extracted from website")
	}

	resp, err := c.callClaude(ctx, analyzeSystemPrompt, fmt.Sprintf("Analyze this website content:\n\n%s", content))
	if err != nil {
		return nil, fmt.Errorf("claude analyze: %w", err)
	}

	// Extract JSON from response (handle markdown code blocks)
	resp = strings.TrimSpace(resp)
	resp = strings.TrimPrefix(resp, "```json")
	resp = strings.TrimPrefix(resp, "```")
	resp = strings.TrimSuffix(resp, "```")
	resp = strings.TrimSpace(resp)

	var profile BusinessProfile
	if err := json.Unmarshal([]byte(resp), &profile); err != nil {
		return nil, fmt.Errorf("unmarshal profile: %w (response: %s)", err, resp[:min(len(resp), 200)])
	}
	return &profile, nil
}
