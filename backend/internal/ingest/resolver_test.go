package ingest

import "testing"

func TestSplitName(t *testing.T) {
	tests := []struct {
		in         string
		first      string
		last       string
		fieldsSeed map[string]any
	}{
		{in: "Cook, Timothy D.", first: "Timothy D", last: "Cook"},
		{in: "WALKER JOHN KENT", first: "JOHN KENT", last: "WALKER"},
		{in: "Musk Elon", first: "Elon", last: "Musk"},
		{in: "Le-Quoc, Alexis", first: "Alexis", last: "Le-Quoc"},
		{
			in:         "Someone Else",
			fieldsSeed: map[string]any{"first_name": "Timothy", "last_name": "Cook"},
			first:      "Timothy",
			last:       "Cook",
		},
		{in: "Madonna", first: "Madonna", last: ""},
		{in: "", first: "", last: ""},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			first, last := splitName(tc.in, tc.fieldsSeed)
			if first != tc.first || last != tc.last {
				t.Errorf("splitName(%q) = (%q, %q); want (%q, %q)",
					tc.in, first, last, tc.first, tc.last)
			}
		})
	}
}

func TestFirstString(t *testing.T) {
	m := map[string]any{
		"a": "",
		"b": "  hello  ",
		"c": "world",
	}
	if got := firstString(m, "a", "b"); got != "hello" {
		t.Errorf("got %q", got)
	}
	if got := firstString(m, "missing", "c"); got != "world" {
		t.Errorf("got %q", got)
	}
	if got := firstString(m, "missing"); got != "" {
		t.Errorf("got %q", got)
	}
}

func TestNormalizeName(t *testing.T) {
	tests := map[string]string{
		"Musk Elon":        "elon musk",
		"Elon Musk":        "elon musk",
		"Cook, Timothy D.": "cook d timothy",
		"WALKER JOHN KENT": "john kent walker",
		"Le-Quoc, Alexis":  "alexis le quoc",
		"":                 "",
		"   ":              "",
	}
	for in, want := range tests {
		if got := NormalizeName(in); got != want {
			t.Errorf("NormalizeName(%q)=%q want %q", in, got, want)
		}
	}
}

func TestNormalizeLinkedInURL(t *testing.T) {
	tests := map[string]string{
		"https://www.linkedin.com/in/timcook/":        "linkedin.com/in/timcook",
		"https://linkedin.com/in/timcook":             "linkedin.com/in/timcook",
		"HTTP://WWW.LinkedIn.COM/in/Tim-Cook-123":     "linkedin.com/in/tim-cook-123",
		"/in/timcook":                                 "linkedin.com/in/timcook",
		"linkedin.com/company/apple":                  "linkedin.com/company/apple",
		"https://twitter.com/timcook":                 "",
		"":                                            "",
		"not a url":                                   "",
	}
	for in, want := range tests {
		if got := NormalizeLinkedInURL(in); got != want {
			t.Errorf("NormalizeLinkedInURL(%q)=%q want %q", in, got, want)
		}
	}
}

func TestNormalizeDomain(t *testing.T) {
	tests := map[string]string{
		"apple.com":                    "apple.com",
		"www.apple.com":                "apple.com",
		"https://www.apple.com/path":   "apple.com",
		"HTTPS://Apple.COM":            "apple.com",
		"":                             "",
	}
	for in, want := range tests {
		if got := NormalizeDomain(in); got != want {
			t.Errorf("NormalizeDomain(%q)=%q want %q", in, got, want)
		}
	}
}

func TestPersonIdentifiers(t *testing.T) {
	ids := personIdentifiers(map[string]any{
		"person_cik":   "0001214156",
		"linkedin_url": "https://www.linkedin.com/in/timcook/",
		"title":        "CEO",
	})
	if len(ids) != 2 {
		t.Fatalf("expected 2 identifiers, got %d (%v)", len(ids), ids)
	}
	if ids[0].typ != "cik" || ids[0].value != "0001214156" {
		t.Errorf("ids[0]=%+v", ids[0])
	}
	if ids[1].typ != "linkedin" || ids[1].value != "linkedin.com/in/timcook" {
		t.Errorf("ids[1]=%+v", ids[1])
	}
}
