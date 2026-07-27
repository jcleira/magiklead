// Package liurl defines the one canonical form for a LinkedIn profile URL
// and is the single place that form is defined. Every writer of a person's
// LinkedIn identity — the ingest resolver, cmd/seed, the live-search
// write-through, and the curated CSV loader — stores Canonical's output
// under identifier_type "linkedin_url", and every LinkedIn-rail reader
// filters on that type. Converging both the type and the value spelling is
// what makes the same human, found by two different sources, dedup to one
// person_identifiers row (identifier_value is globally UNIQUE, and dedup
// matches on the exact string).
//
// The canonical form is:
//
//	https://www.linkedin.com/<path>
//
// scheme always https, host always www.linkedin.com, the path lowercased
// with no trailing slash and any query string or fragment stripped.
package liurl

import (
	"net/url"
	"strings"
)

// canonicalPrefix is the scheme+host every canonical LinkedIn URL carries.
// It is also the exact string the 030 data migration concatenates onto a
// legacy scheme-less "linkedin.com/<path>" value, so the two must agree:
// Canonical("linkedin.com/in/x") == canonicalPrefix + "/in/x".
const canonicalPrefix = "https://www.linkedin.com"

// Canonical coerces any messy LinkedIn profile URL — scheme variants, a
// missing scheme, www./m./country-code hosts, a trailing slash, query
// strings or fragments, uppercase slugs, or a bare "/in/<slug>" path — to
// the single canonical form "https://www.linkedin.com/<path>". Input that
// isn't a linkedin.com URL returns "".
func Canonical(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	// Tolerate bare paths like "/in/timcook" and scheme-less hosts like
	// "linkedin.com/in/timcook" — both name a linkedin.com resource.
	switch {
	case strings.HasPrefix(raw, "/"):
		raw = canonicalPrefix + raw
	case !strings.Contains(raw, "://"):
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	host := strings.ToLower(u.Host)
	// Accept linkedin.com and any subdomain (www., m., ca., …) but reject
	// look-alikes such as notlinkedin.com.
	if host != "linkedin.com" && !strings.HasSuffix(host, ".linkedin.com") {
		return ""
	}
	path := strings.ToLower(strings.Trim(u.Path, "/"))
	if path == "" {
		return ""
	}
	return canonicalPrefix + "/" + path
}
