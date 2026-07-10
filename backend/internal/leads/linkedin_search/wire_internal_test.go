package linkedinsearch

import "testing"

// parseSearch must pass through a real profileURL and synthesize one
// from `username` when the URL is absent — the dedup identifier depends
// on always having a stable linkedin_url.
func TestParseSearch_URLPassthroughAndUsernameSynth(t *testing.T) {
	raw := []byte(`{"data":{"items":[
		{"fullName":"Ada Byron","headline":"CEO","profileURL":"https://www.linkedin.com/in/ada","companyName":"Analytical","companyDomain":"analytical.test","location":"London"},
		{"fullName":"Carl Friedrich","headline":"CTO","username":"cfgauss","companyName":"Disquisitiones"}
	]}}`)
	profiles, err := parseSearch(raw)
	if err != nil {
		t.Fatalf("parseSearch: %v", err)
	}
	if len(profiles) != 2 {
		t.Fatalf("profiles=%d want 2", len(profiles))
	}
	if profiles[0].LinkedInURL != "https://www.linkedin.com/in/ada" {
		t.Errorf("profile[0].LinkedInURL=%q want passthrough", profiles[0].LinkedInURL)
	}
	if profiles[0].Title != "CEO" || profiles[0].Domain != "analytical.test" {
		t.Errorf("profile[0] fields wrong: title=%q domain=%q", profiles[0].Title, profiles[0].Domain)
	}
	if profiles[1].LinkedInURL != "https://www.linkedin.com/in/cfgauss" {
		t.Errorf("profile[1].LinkedInURL=%q want synth from username", profiles[1].LinkedInURL)
	}
}

func TestParseSearch_Malformed(t *testing.T) {
	if _, err := parseSearch([]byte(`not json`)); err == nil {
		t.Error("malformed body must error")
	}
}

// SyntheticURL gives demo profiles (scraper names, no real URL) a stable,
// collision-resistant linkedin_url namespaced by company.
func TestSyntheticURL(t *testing.T) {
	got := SyntheticURL("Jane Doe", "acme.com")
	if got != "https://www.linkedin.com/in/jane-doe-acme" {
		t.Errorf("SyntheticURL=%q want https://www.linkedin.com/in/jane-doe-acme", got)
	}
	// Same name, different company → different URL (no collision).
	other := SyntheticURL("Jane Doe", "initech.io")
	if other == got {
		t.Errorf("same-name people at different companies collided: %q", got)
	}
}
