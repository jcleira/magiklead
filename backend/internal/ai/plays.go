package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const playsSystemPrompt = `You are a B2B sales strategist. Given a business profile, generate 3-5 sales plays.
Each play targets a specific ICP (Ideal Customer Profile) with a buying signal and outreach channel.
Return ONLY a valid JSON array with this structure:
[
  {
    "name": "Descriptive play name",
    "description": "Why this ICP would buy",
    "icp": {
      "titles": ["VP Sales", "Head of Sales"],
      "industry": ["Software", "Technology"],
      "company_size": "50-500 employees",
      "geo": ["United States"]
    },
    "signal": "Companies actively hiring for this role",
    "channels": ["email", "linkedin"],
    "search_query": {
      "title": "VP Sales OR Head of Sales",
      "company_size": "51-500",
      "industry": "Technology",
      "location": "United States"
    }
  }
]
Do not include any text outside the JSON array.`

// GeneratePlays creates sales plays from a business profile using Claude.
func (c *Client) GeneratePlays(ctx context.Context, profile *BusinessProfile) ([]Play, error) {
	profileJSON, _ := json.Marshal(profile)

	resp, err := c.callClaude(ctx, playsSystemPrompt, fmt.Sprintf("Generate sales plays for this business:\n\n%s", string(profileJSON)))
	if err != nil {
		return nil, fmt.Errorf("claude plays: %w", err)
	}

	resp = strings.TrimSpace(resp)
	resp = strings.TrimPrefix(resp, "```json")
	resp = strings.TrimPrefix(resp, "```")
	resp = strings.TrimSuffix(resp, "```")
	resp = strings.TrimSpace(resp)

	var plays []Play
	if err := json.Unmarshal([]byte(resp), &plays); err != nil {
		return nil, fmt.Errorf("unmarshal plays: %w", err)
	}
	return plays, nil
}
