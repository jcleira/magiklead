package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/html"
)

const model = "claude-haiku-4-5-20251001"

// Client wraps the Anthropic API for MagikLead AI tasks.
type Client struct {
	apiKey     string
	httpClient *http.Client
}

func NewClient(apiKey string) *Client {
	return &Client{
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// BusinessProfile is the AI-extracted business analysis from a website.
type BusinessProfile struct {
	CompanyName     string   `json:"company_name"`
	Description     string   `json:"product_description"`
	Features        []string `json:"features"`
	Pricing         string   `json:"pricing"`
	TargetCustomers []string `json:"target_customers"`
	Differentiators []string `json:"differentiators"`
	Industry        string   `json:"industry"`
}

// Play is an AI-generated sales strategy targeting a specific ICP.
type Play struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	ICP         ICP         `json:"icp"`
	Signal      string      `json:"signal"`
	Channels    []string    `json:"channels"`
	SearchQuery SearchQuery `json:"search_query"`
}

type ICP struct {
	Titles      []string `json:"titles"`
	Industry    []string `json:"industry"`
	CompanySize string   `json:"company_size"`
	Geo         []string `json:"geo"`
}

type SearchQuery struct {
	Title       string `json:"title"`
	CompanySize string `json:"company_size"`
	Industry    string `json:"industry"`
	Location    string `json:"location"`
}

// Sequence is a multi-step email sequence for outreach.
type Sequence struct {
	Steps []SequenceStep `json:"steps"`
}

type SequenceStep struct {
	Step      int    `json:"step"`
	DelayDays int    `json:"delay_days"`
	Subject   string `json:"subject"`
	Body      string `json:"body"`
}

// LeadInfo is the minimal lead data needed for sequence generation.
type LeadInfo struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Title     string `json:"title"`
	Company   string `json:"company"`
}

// callClaude sends a message to Claude and returns the text response.
func (c *Client) callClaude(ctx context.Context, systemPrompt, userMessage string) (string, error) {
	reqBody := map[string]any{
		"model":      model,
		"max_tokens": 2048,
		"system":     systemPrompt,
		"messages": []map[string]string{
			{"role": "user", "content": userMessage},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.anthropic.com/v1/messages", strings.NewReader(string(body)))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("claude API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}
	if len(result.Content) == 0 {
		return "", fmt.Errorf("empty response from claude")
	}
	return result.Content[0].Text, nil
}

// scrapeWebsite fetches a URL and extracts text content from HTML.
func scrapeWebsite(ctx context.Context, url string) (string, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "MagikLead/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	doc, err := html.Parse(resp.Body)
	if err != nil {
		return "", err
	}

	var text strings.Builder
	var extract func(*html.Node)
	extract = func(n *html.Node) {
		if n.Type == html.TextNode {
			t := strings.TrimSpace(n.Data)
			if t != "" {
				text.WriteString(t)
				text.WriteString(" ")
			}
		}
		// Skip script and style tags
		if n.Type == html.ElementNode && (n.Data == "script" || n.Data == "style" || n.Data == "noscript") {
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			extract(c)
		}
	}
	extract(doc)

	result := text.String()
	if len(result) > 4000 {
		result = result[:4000]
	}
	return result, nil
}
