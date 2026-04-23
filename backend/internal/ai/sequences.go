package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const sequenceSystemPrompt = `You are an expert cold email copywriter. Write a 3-4 step email sequence for B2B outreach.
Each email should be personalized, concise (under 150 words), and have a clear CTA.
Use {{first_name}}, {{company}}, {{title}} as merge variables that will be replaced with real values.
Return ONLY valid JSON with this structure:
{
  "steps": [
    {
      "step": 1,
      "delay_days": 0,
      "subject": "Subject line with {{company}}",
      "body": "Email body with {{first_name}} and {{company}}"
    }
  ]
}
Do not include any text outside the JSON object.`

// GenerateSequence creates a personalized email sequence using Claude.
func (c *Client) GenerateSequence(ctx context.Context, profile *BusinessProfile, play *Play, lead *LeadInfo) (*Sequence, error) {
	input := map[string]any{
		"business":    profile,
		"play":        play,
		"lead_sample": lead,
	}
	inputJSON, _ := json.Marshal(input)

	resp, err := c.callClaude(ctx, sequenceSystemPrompt, fmt.Sprintf("Generate a cold email sequence for this outreach:\n\n%s", string(inputJSON)))
	if err != nil {
		return nil, fmt.Errorf("claude sequence: %w", err)
	}

	resp = strings.TrimSpace(resp)
	resp = strings.TrimPrefix(resp, "```json")
	resp = strings.TrimPrefix(resp, "```")
	resp = strings.TrimSuffix(resp, "```")
	resp = strings.TrimSpace(resp)

	var seq Sequence
	if err := json.Unmarshal([]byte(resp), &seq); err != nil {
		return nil, fmt.Errorf("unmarshal sequence: %w", err)
	}
	return &seq, nil
}
