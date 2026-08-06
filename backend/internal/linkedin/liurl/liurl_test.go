package liurl

import "testing"

func TestCanonical(t *testing.T) {
	const want = "https://www.linkedin.com/in/tim-cook"
	cases := map[string]string{
		// Scheme variants — https, http, uppercase, and none — all normalize
		// to https and drop to the same slug.
		"https://www.linkedin.com/in/tim-cook": want,
		"http://www.linkedin.com/in/tim-cook":  want,
		"HTTP://WWW.LinkedIn.COM/in/Tim-Cook":  want,
		"www.linkedin.com/in/tim-cook":         want,
		"linkedin.com/in/tim-cook":             want,

		// Host variants — bare, www., mobile, country-code — collapse to www.
		"https://linkedin.com/in/tim-cook":    want,
		"https://m.linkedin.com/in/tim-cook":  want,
		"https://ca.linkedin.com/in/tim-cook": want,

		// Trailing slash, query string, fragment — all stripped.
		"https://www.linkedin.com/in/tim-cook/":           want,
		"https://www.linkedin.com/in/tim-cook?utm=x":      want,
		"https://www.linkedin.com/in/tim-cook/?trk=abc":   want,
		"https://www.linkedin.com/in/tim-cook#experience": want,
		"  https://www.linkedin.com/in/tim-cook/  ":       want,

		// Uppercase slug lowercased.
		"https://www.linkedin.com/in/TIM-COOK": want,

		// Bare path — treated as a linkedin.com resource.
		"/in/tim-cook": want,

		// A company path still canonicalizes (the helper is generic; person
		// writers just never feed it one).
		"https://www.linkedin.com/company/apple": "https://www.linkedin.com/company/apple",

		// Non-LinkedIn and junk inputs return "".
		"https://twitter.com/tim-cook": "",
		"https://notlinkedin.com/in/x": "",
		"not a url":                    "",
		"":                             "",
		"   ":                          "",
		"https://www.linkedin.com":     "",
		"https://www.linkedin.com/":    "",
	}

	for in, exp := range cases {
		if got := Canonical(in); got != exp {
			t.Errorf("Canonical(%q) = %q, want %q", in, got, exp)
		}
	}
}

// TestCanonicalMatchesMigrationConcat pins the tie between the helper and the
// 030 data migration: legacy 'linkedin' rows hold a scheme-less
// "linkedin.com/<path>" value, and the migration rewrites them with
// 'https://www.' || value. That output must be byte-identical to what
// Canonical produces for the same legacy value, or migrated rows would not
// dedup against freshly-written ones.
func TestCanonicalMatchesMigrationConcat(t *testing.T) {
	legacyValues := []string{
		"linkedin.com/in/tim-cook",
		"linkedin.com/in/ada-lovelace",
		"linkedin.com/company/apple",
	}
	for _, legacy := range legacyValues {
		if got, want := Canonical(legacy), "https://www."+legacy; got != want {
			t.Errorf("Canonical(%q) = %q, migration concat = %q", legacy, got, want)
		}
	}
}

// TestCanonicalIdempotent guards the "one canonical form" invariant: feeding
// Canonical's own output back in must return it unchanged.
func TestCanonicalIdempotent(t *testing.T) {
	inputs := []string{
		"HTTP://WWW.LinkedIn.COM/in/Tim-Cook/?utm=x",
		"linkedin.com/in/ada-lovelace",
		"/in/grace-hopper",
		"https://m.linkedin.com/company/pied-piper",
	}
	for _, in := range inputs {
		once := Canonical(in)
		if once == "" {
			t.Fatalf("Canonical(%q) unexpectedly empty", in)
		}
		if twice := Canonical(once); twice != once {
			t.Errorf("not idempotent: Canonical(%q)=%q, Canonical(that)=%q", in, once, twice)
		}
	}
}
