package handler

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	apierr "github.com/jcleira/magiklead/backend/pkg/errors"
)

type DeliverabilityHandler struct{}

func NewDeliverabilityHandler() *DeliverabilityHandler {
	return &DeliverabilityHandler{}
}

type DomainCheck struct {
	Domain string      `json:"domain"`
	Score  int         `json:"score"`  // 0-100
	Level  string      `json:"level"`  // "safe", "moderate", "risky"
	Limits SendLimits  `json:"limits"` // Unlocked sending limits based on score
	Checks []CheckItem `json:"checks"`
}

type CheckItem struct {
	Name        string `json:"name"`
	Status      string `json:"status"` // "pass", "fail", "warn"
	Description string `json:"description"`
	HowToFix    string `json:"how_to_fix,omitempty"`
}

type SendLimits struct {
	DailyMax       int    `json:"daily_max"`
	MinDelayMin    int    `json:"min_delay_minutes"`
	MaxDelayMin    int    `json:"max_delay_minutes"`
	WarmupDays     int    `json:"warmup_days"`
	WarmupSchedule []int  `json:"warmup_schedule"` // Emails per day during warmup
	Description    string `json:"description"`
}

// CheckDomain handles GET /api/v1/deliverability/check?domain=example.com
func (h *DeliverabilityHandler) CheckDomain(w http.ResponseWriter, r *http.Request) {
	domain := r.URL.Query().Get("domain")
	if domain == "" {
		apierr.WriteError(w, apierr.APIError{Status: 400, Code: "bad_request", Message: "domain parameter required"})
		return
	}

	// Strip email if full email passed
	if at := strings.Index(domain, "@"); at != -1 {
		domain = domain[at+1:]
	}

	result := checkDomainHealth(domain)
	apierr.WriteJSON(w, http.StatusOK, result)
}

func checkDomainHealth(domain string) DomainCheck {
	var checks []CheckItem
	score := 0

	// 1. MX Records
	mxRecords, err := net.LookupMX(domain)
	if err != nil || len(mxRecords) == 0 {
		checks = append(checks, CheckItem{
			Name:        "MX Records",
			Status:      "fail",
			Description: "No mail server found for this domain.",
			HowToFix:    "Add MX records in your DNS provider. If using Google Workspace, add Google's MX records.",
		})
	} else {
		mxHosts := make([]string, len(mxRecords))
		for i, mx := range mxRecords {
			mxHosts[i] = strings.TrimSuffix(mx.Host, ".")
		}
		checks = append(checks, CheckItem{
			Name:        "MX Records",
			Status:      "pass",
			Description: fmt.Sprintf("Mail server found: %s", strings.Join(mxHosts[:min(len(mxHosts), 2)], ", ")),
		})
		score += 20
	}

	// 2. SPF Record
	spfFound := false
	txtRecords, _ := net.LookupTXT(domain)
	for _, txt := range txtRecords {
		if strings.HasPrefix(txt, "v=spf1") {
			spfFound = true
			checks = append(checks, CheckItem{
				Name:        "SPF Record",
				Status:      "pass",
				Description: fmt.Sprintf("SPF configured: %s", truncate(txt, 80)),
			})
			score += 20
			break
		}
	}
	if !spfFound {
		checks = append(checks, CheckItem{
			Name:        "SPF Record",
			Status:      "fail",
			Description: "No SPF record found. Email servers can't verify you're authorized to send.",
			HowToFix:    "Add a TXT record to your DNS: v=spf1 include:_spf.google.com ~all (for Google Workspace) or v=spf1 include:spf.protection.outlook.com ~all (for Outlook).",
		})
	}

	// 3. DKIM (check common selectors)
	dkimFound := false
	for _, selector := range []string{"google", "default", "selector1", "selector2", "k1", "dkim"} {
		cname, _ := net.LookupCNAME(selector + "._domainkey." + domain)
		txt, _ := net.LookupTXT(selector + "._domainkey." + domain)
		if cname != "" || len(txt) > 0 {
			dkimFound = true
			checks = append(checks, CheckItem{
				Name:        "DKIM Record",
				Status:      "pass",
				Description: fmt.Sprintf("DKIM configured (selector: %s)", selector),
			})
			score += 20
			break
		}
	}
	if !dkimFound {
		checks = append(checks, CheckItem{
			Name:        "DKIM Record",
			Status:      "warn",
			Description: "DKIM not detected (checked common selectors). Emails may land in spam.",
			HowToFix:    "Enable DKIM in your email provider settings. Google Workspace: Admin Console → Apps → Gmail → Authenticate email. Outlook: Microsoft 365 admin center → Settings → Domains.",
		})
	}

	// 4. DMARC Record
	dmarcRecords, _ := net.LookupTXT("_dmarc." + domain)
	dmarcFound := false
	for _, txt := range dmarcRecords {
		if strings.HasPrefix(txt, "v=DMARC1") {
			dmarcFound = true
			checks = append(checks, CheckItem{
				Name:        "DMARC Record",
				Status:      "pass",
				Description: fmt.Sprintf("DMARC configured: %s", truncate(txt, 80)),
			})
			score += 20
			break
		}
	}
	if !dmarcFound {
		checks = append(checks, CheckItem{
			Name:        "DMARC Record",
			Status:      "warn",
			Description: "No DMARC policy found. Recommended for better deliverability.",
			HowToFix:    "Add a TXT record for _dmarc.yourdomain.com with value: v=DMARC1; p=none; rua=mailto:dmarc@yourdomain.com",
		})
	}

	// 5. Domain age (check if it resolves — older domains are more trusted)
	_, err = net.LookupHost(domain)
	if err == nil {
		checks = append(checks, CheckItem{
			Name:        "Domain Active",
			Status:      "pass",
			Description: "Domain resolves and is active.",
		})
		score += 10
	} else {
		checks = append(checks, CheckItem{
			Name:        "Domain Active",
			Status:      "fail",
			Description: "Domain does not resolve.",
			HowToFix:    "Make sure your domain has A/AAAA records and is pointing to a web server.",
		})
	}

	// 6. Reverse DNS / PTR (bonus)
	if len(mxRecords) > 0 {
		mxHost := strings.TrimSuffix(mxRecords[0].Host, ".")
		addrs, _ := net.LookupHost(mxHost)
		if len(addrs) > 0 {
			names, _ := net.LookupAddr(addrs[0])
			if len(names) > 0 {
				checks = append(checks, CheckItem{
					Name:        "Reverse DNS",
					Status:      "pass",
					Description: fmt.Sprintf("MX server has valid reverse DNS: %s", strings.TrimSuffix(names[0], ".")),
				})
				score += 10
			}
		}
	}

	// Calculate level and limits
	level := "risky"
	if score >= 70 {
		level = "safe"
	} else if score >= 40 {
		level = "moderate"
	}

	limits := calculateLimits(score, level)

	return DomainCheck{
		Domain: domain,
		Score:  score,
		Level:  level,
		Limits: limits,
		Checks: checks,
	}
}

func calculateLimits(score int, level string) SendLimits {
	switch level {
	case "safe":
		return SendLimits{
			DailyMax:       100,
			MinDelayMin:    2,
			MaxDelayMin:    5,
			WarmupDays:     14,
			WarmupSchedule: []int{10, 15, 20, 25, 30, 35, 40, 50, 60, 70, 80, 90, 100, 100},
			Description:    "Your domain is well configured. Full sending power unlocked after 14-day warmup.",
		}
	case "moderate":
		return SendLimits{
			DailyMax:       50,
			MinDelayMin:    3,
			MaxDelayMin:    8,
			WarmupDays:     21,
			WarmupSchedule: []int{5, 8, 10, 12, 15, 18, 20, 25, 28, 30, 33, 36, 40, 43, 45, 47, 48, 49, 50, 50, 50},
			Description:    "Your domain is missing some DNS records. Fix them to unlock higher limits.",
		}
	default:
		return SendLimits{
			DailyMax:       20,
			MinDelayMin:    5,
			MaxDelayMin:    12,
			WarmupDays:     28,
			WarmupSchedule: []int{3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 20, 20, 20, 20, 20, 20, 20, 20, 20, 20},
			Description:    "Your domain has issues. Sending is limited. Fix the problems below to unlock full power.",
		}
	}
}

// GetWarmupDay returns the current warmup day for an email account based on its creation date.
func GetWarmupDay(createdAt time.Time) int {
	days := int(time.Since(createdAt).Hours() / 24)
	return days
}

// GetDailyLimit returns today's send limit based on warmup schedule and domain health.
func GetDailyLimit(limits SendLimits, warmupDay int) int {
	if warmupDay < len(limits.WarmupSchedule) {
		return limits.WarmupSchedule[warmupDay]
	}
	return limits.DailyMax
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
