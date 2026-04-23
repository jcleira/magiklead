package leads

import (
	"fmt"
	"log"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// EmailResult is a discovered and verified email address.
type EmailResult struct {
	Email      string `json:"email"`
	Pattern    string `json:"pattern"` // e.g., "first.last", "flast"
	Verified   bool   `json:"verified"`
	Confidence int    `json:"confidence"` // 0-100
}

// FindEmail discovers a person's email by guessing patterns and verifying via SMTP.
func FindEmail(firstName, lastName, domain string) (*EmailResult, error) {
	firstName = strings.ToLower(strings.TrimSpace(firstName))
	lastName = strings.ToLower(strings.TrimSpace(lastName))
	domain = strings.ToLower(strings.TrimSpace(domain))

	if firstName == "" || lastName == "" || domain == "" {
		return nil, fmt.Errorf("need first name, last name, and domain")
	}

	// Remove common domain prefixes
	domain = strings.TrimPrefix(domain, "www.")

	// Generate candidate emails from common patterns
	firstInitial := string(firstName[0])
	lastInitial := string(lastName[0])

	candidates := []struct {
		email   string
		pattern string
		weight  int // higher = more common pattern
	}{
		{fmt.Sprintf("%s.%s@%s", firstName, lastName, domain), "first.last", 30},
		{fmt.Sprintf("%s@%s", firstName, domain), "first", 20},
		{fmt.Sprintf("%s%s@%s", firstInitial, lastName, domain), "flast", 15},
		{fmt.Sprintf("%s.%s@%s", firstInitial, lastName, domain), "f.last", 10},
		{fmt.Sprintf("%s%s@%s", firstName, lastInitial, domain), "firstl", 8},
		{fmt.Sprintf("%s_%s@%s", firstName, lastName, domain), "first_last", 8},
		{fmt.Sprintf("%s%s@%s", firstName, lastName, domain), "firstlast", 8},
		{fmt.Sprintf("%s@%s", lastName, domain), "last", 5},
		{fmt.Sprintf("%s.%s@%s", lastName, firstName, domain), "last.first", 5},
	}

	// Find MX records for the domain
	mxRecords, err := net.LookupMX(domain)
	if err != nil || len(mxRecords) == 0 {
		// No MX records — can't verify. Return best guess with low confidence.
		log.Printf("No MX records for %s — returning best guess", domain)
		return &EmailResult{
			Email:      candidates[0].email,
			Pattern:    candidates[0].pattern,
			Verified:   false,
			Confidence: 40,
		}, nil
	}

	mxHost := strings.TrimSuffix(mxRecords[0].Host, ".")

	// Try each candidate via SMTP
	for _, c := range candidates {
		verified, err := verifyEmailSMTP(c.email, mxHost)
		if err != nil {
			// SMTP connection failed — try next MX or return best guess
			log.Printf("SMTP check failed for %s: %v", c.email, err)
			continue
		}
		if verified {
			log.Printf("Verified email: %s (pattern: %s)", c.email, c.pattern)
			return &EmailResult{
				Email:      c.email,
				Pattern:    c.pattern,
				Verified:   true,
				Confidence: 90 + c.weight/10,
			}, nil
		}
	}

	// No SMTP verification worked — return best guess
	log.Printf("Could not verify any email for %s %s at %s — returning best guess", firstName, lastName, domain)
	return &EmailResult{
		Email:      candidates[0].email,
		Pattern:    candidates[0].pattern,
		Verified:   false,
		Confidence: 50,
	}, nil
}

// verifyEmailSMTP checks if an email address exists by talking to the SMTP server.
// Returns true if the server accepts the RCPT TO command.
func verifyEmailSMTP(email, mxHost string) (bool, error) {
	// Connect with timeout
	conn, err := net.DialTimeout("tcp", mxHost+":25", 10*time.Second)
	if err != nil {
		return false, fmt.Errorf("connect: %w", err)
	}

	client, err := smtp.NewClient(conn, mxHost)
	if err != nil {
		conn.Close()
		return false, fmt.Errorf("smtp client: %w", err)
	}
	defer client.Close()

	// Set deadline for the whole conversation
	conn.SetDeadline(time.Now().Add(15 * time.Second))

	// EHLO
	if err := client.Hello("magiklead.com"); err != nil {
		return false, fmt.Errorf("ehlo: %w", err)
	}

	// MAIL FROM
	if err := client.Mail("verify@magiklead.com"); err != nil {
		return false, fmt.Errorf("mail from: %w", err)
	}

	// RCPT TO — this is the actual check
	err = client.Rcpt(email)
	if err != nil {
		// Server rejected the recipient — email doesn't exist
		return false, nil
	}

	// Server accepted — email exists
	client.Reset()
	client.Quit()
	return true, nil
}

// FindEmailsForDomain discovers the email pattern for a domain by testing
// with a known person, then applies it to all other people at that domain.
func FindEmailsForDomain(people []ScrapedPerson) []ScrapedPerson {
	if len(people) == 0 {
		return people
	}

	domain := people[0].Domain
	var discoveredPattern string

	// Try to discover the pattern using the first few people
	for i := 0; i < min(len(people), 3); i++ {
		firstName, lastName := splitName(people[i].Name)
		if firstName == "" || lastName == "" {
			continue
		}

		result, err := FindEmail(firstName, lastName, domain)
		if err != nil {
			continue
		}

		if result.Verified {
			discoveredPattern = result.Pattern
			people[i].Email = result.Email
			people[i].EmailVerified = true
			people[i].EmailConfidence = result.Confidence
			log.Printf("Discovered email pattern for %s: %s", domain, discoveredPattern)
			break
		} else {
			// Use best guess for this person
			people[i].Email = result.Email
			people[i].EmailConfidence = result.Confidence
		}
	}

	// Apply discovered pattern to remaining people
	for i := range people {
		if people[i].Email != "" {
			continue
		}

		firstName, lastName := splitName(people[i].Name)
		if firstName == "" || lastName == "" {
			continue
		}

		if discoveredPattern != "" {
			// Apply the known pattern
			email := applyPattern(discoveredPattern, firstName, lastName, domain)
			people[i].Email = email
			people[i].EmailVerified = true
			people[i].EmailConfidence = 85
		} else {
			// No pattern found, use best guess
			result, _ := FindEmail(firstName, lastName, domain)
			if result != nil {
				people[i].Email = result.Email
				people[i].EmailConfidence = result.Confidence
			}
		}
	}

	return people
}

func applyPattern(pattern, firstName, lastName, domain string) string {
	firstInitial := string(firstName[0])
	lastInitial := string(lastName[0])

	switch pattern {
	case "first.last":
		return fmt.Sprintf("%s.%s@%s", firstName, lastName, domain)
	case "first":
		return fmt.Sprintf("%s@%s", firstName, domain)
	case "flast":
		return fmt.Sprintf("%s%s@%s", firstInitial, lastName, domain)
	case "f.last":
		return fmt.Sprintf("%s.%s@%s", firstInitial, lastName, domain)
	case "firstl":
		return fmt.Sprintf("%s%s@%s", firstName, lastInitial, domain)
	case "first_last":
		return fmt.Sprintf("%s_%s@%s", firstName, lastName, domain)
	case "firstlast":
		return fmt.Sprintf("%s%s@%s", firstName, lastName, domain)
	case "last":
		return fmt.Sprintf("%s@%s", lastName, domain)
	case "last.first":
		return fmt.Sprintf("%s.%s@%s", lastName, firstName, domain)
	default:
		return fmt.Sprintf("%s.%s@%s", firstName, lastName, domain)
	}
}

func splitName(fullName string) (string, string) {
	fullName = strings.TrimSpace(fullName)
	parts := strings.Fields(fullName)
	if len(parts) < 2 {
		return fullName, ""
	}
	firstName := strings.ToLower(parts[0])
	lastName := strings.ToLower(parts[len(parts)-1])
	return firstName, lastName
}
