# MoneyPrinter Clone — Research Document

**Date:** 2026-04-10
**Goal:** Build an open-source/self-hosted alternative to [moneyprinter.me](https://moneyprinter.me/) — an AI-powered outbound sales automation platform.
**First tenant:** MagikShot (magikshot.com)
**Tech stack:** Go (Chi router) + Next.js 16 + PostgreSQL + Redis (Asynq) + Chrome Extension
**Repo:** Separate repository (monorepo: backend + frontend + extension)

---

## Table of Contents

1. [What MoneyPrinter Does](#1-what-moneyprinter-does)
2. [Competitive Analysis](#2-competitive-analysis) *(updated Apr 2026)*
3. [Architecture](#3-architecture)
4. [Lead Discovery Pipeline](#4-lead-discovery-pipeline)
5. [Email Infrastructure](#5-email-infrastructure)
6. [LinkedIn Automation (Chrome Extension)](#6-linkedin-automation-chrome-extension)
7. [AI Layer](#7-ai-layer)
8. [Multi-Tenant Model](#8-multi-tenant-model)
9. [Cost Analysis](#9-cost-analysis)
10. [Pricing Model](#10-pricing-model)
11. [Data Model](#11-data-model)
12. [API Design](#12-api-design)
13. [MVP Scope](#13-mvp-scope)
14. [Risk Analysis](#14-risk-analysis)
15. [Customer Acquisition Strategy](#15-customer-acquisition-strategy) *(new)*
16. [Differentiation Strategy](#16-differentiation-strategy) *(new)*
17. [Domain & Brand Strategy](#17-domain--brand-strategy) *(new)*
18. [Legal & Compliance](#18-legal--compliance) *(new)*
19. [Validation Framework & Kill Criteria](#19-validation-framework--kill-criteria) *(new)*
20. [Summary & Next Steps](#20-summary--next-steps) *(new)*
21. [Sources](#21-sources) *(updated)*

---

## 1. What MoneyPrinter Does

MoneyPrinter (by [HireRoger](https://www.hireroger.com/)) is a YC-backed (General Catalyst, Capital One Ventures) AI outbound sales platform. Core flow:

1. **Paste your website** — AI scrapes and analyzes your product, pricing, target customers
2. **Generate sales plays** — AI creates 3-5 ICPs with: who (titles), where (company type/size/geo), signal (hiring, growth, etc.), and channel mix (email/LinkedIn/phone)
3. **Discover leads** — system finds actual people matching each play using LinkedIn data, job postings, competitor followers
4. **Generate sequences** — AI writes personalized multi-step email and LinkedIn outreach sequences
5. **Execute outreach** — emails sent via managed mailboxes, LinkedIn via Chrome Extension ("Roger Companion")
6. **Track results** — opens, replies, interested leads, meetings booked, pipeline value

### MoneyPrinter Pricing

| Plan | $/mo | Sequences/mo | Leads/mo | Key Features |
|------|------|-------------|----------|-------------|
| Free | $0 | 300 | 100 | Basic targeting, 7-day trial |
| Lite | $99 | 450 | 150 | Job posting signals, full filters |
| Growth | $250 | 1,125 | 375 | AI personalization (Most Popular) |
| Scale | $800 | 3,600 | 1,200 | Research reports, competitor tracking |

Add-on: Managed mailboxes + warming at $3/mailbox/month.

### MoneyPrinter Dashboard Features (observed)

From hands-on trial (April 10, 2026):

**Left sidebar navigation:**
- AI Assistant
- Analytics
- Campaigns (manage outreach)
- Conversations
- Tasks
- Users
- Domains & Mailboxes
- LinkedIn
- CRM: Contacts, Find Leads, Find/Upload Leads
- Settings

**Campaign view:**
- Campaign name + status (Active)
- Metrics: Sent, Replied, Link Opens, Interested, Meetings, Pipeline value
- Contact list with name, title, company, date added, status (Active/Waiting)
- Email sequence preview panel showing multi-step follow-ups with configurable delays

**Email sequences observed:**
- Step 1: Initial outreach (immediate)
- Step 2: Follow-up (X days after previous)
- Step 3: Second follow-up (30 days after previous)
- Step 4: Final "breakup" email (100 days after previous)
- Each email personalized to lead's context (job search, company mentions, pain points)
- Signed with sender name + product + URL

**Chrome Extension ("Roger Companion"):**
- Shows upcoming LinkedIn interactions (connection requests, messages)
- Displays daily invite limit ("11 invites available today")
- Lists scheduled contacts with name, tag, LinkedIn URL, date, status (Scheduled)
- Executes actions using user's LinkedIn session in their browser

**Onboarding flow:**
- Enter website URL
- Platform scrapes and analyzes (struggled with app-only sites like magikshot.com)
- Manual override: short description, detailed summary, features, social proof
- Generates sales plays with ICP + signal + channel recommendations

---

## 2. Competitive Analysis

### Direct Competitors (AI Outbound Platforms)

| Product | Pricing (Apr 2026) | Channels | Lead Source | Differentiator |
|---------|-------------------|----------|-------------|----------------|
| MoneyPrinter/Roger | $0-800/mo (Free/Lite $99/Growth $250/Scale $800) | Email + LinkedIn + Phone | Built-in AutoFind | YC-backed, parallel dialer + teleprompter, $3/mo managed mailboxes |
| Apollo.io | $0-99/user/mo (Free/Basic $49/Pro $99/Enterprise custom) | Email + LinkedIn | 275M+ contact DB | Largest database, credit system ($0.025/credit), no built-in warmup |
| Instantly.ai | $47-358/mo (Growth $47/Hypergrowth $97/Light Speed $358) | Email only | 450M+ B2B DB (separate credits plan $47-197/mo) | Unlimited email accounts + warmup, SISR deliverability system |
| Lemlist | $63-109/user/mo annual (Email Pro $79/Multichannel Expert $109) | Email + LinkedIn | BYOL or enrichment credits (25-500/mo) | AI voice messages on LinkedIn, personalized images, per-seat model |
| Waalaxy | €0-69/user/mo (Free/Pro €19/Advanced €49/Business €69) | LinkedIn + Email (Business only) | LinkedIn search | Cheapest LinkedIn automation, 800 invites/mo on Advanced |
| Smartlead.ai | $39-94/mo (Basic $39/Pro $79/Custom $94) | Email | BYOL | Agency-focused, high deliverability, unlimited mailboxes |
| Saleshandy | $25-219/mo (Starter $25/Pro $99/Scale $199/Scale+ $219) | Email | 830M+ DB | Cheapest entry point, unlimited accounts, agency features |
| Outreach | Enterprise ($$$) | Email + LinkedIn + Phone | CRM integration | Enterprise-grade |
| Salesloft | Enterprise ($$$) | Email + LinkedIn + Phone | CRM integration | Enterprise-grade |
| Amplemarket | Enterprise ($$$) | Email + LinkedIn + WhatsApp + AI Voice + Phone | Built-in | Highest feature score (219/231), only platform with WhatsApp + AI voice |

### Our Positioning

- **Zero-marginal-cost lead discovery** via RapidAPI + caching (no expensive data provider lock-in)
- **Users bring their own Gmail** (no managed mailbox cost)
- **AI-first** — Claude Haiku for sequence generation at $0.0025/lead
- **Multi-tenant** — one platform for multiple products/brands
- **Open approach** — potential open-source core

---

## 3. Architecture

### System Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                         User Flow                                │
│                                                                  │
│  1. Paste website URL                                            │
│  2. AI analyzes product -> generates business profile            │
│  3. AI generates 3-5 sales plays (ICP, signal, channels)        │
│  4. User approves a play                                         │
│  5. System finds leads (LinkedIn API -> Email API -> verify)     │
│  6. AI generates personalized sequences (email + LinkedIn)       │
│  7. User connects Gmail (OAuth) -> emails sent automatically     │
│  8. User installs Chrome Extension -> LinkedIn outreach          │
│  9. Dashboard: opens, replies, meetings booked                   │
└─────────────────────────────────────────────────────────────────┘
```

### Tech Stack

| Layer | Technology | Notes |
|-------|-----------|-------|
| Backend API | Go + Chi v5 | Same stack as MagikShot |
| Async Worker | Go + Asynq (Redis) | Background lead discovery, email sending |
| Frontend | Next.js 16 + Tailwind + Shadcn | Same stack as MagikShot |
| Database | PostgreSQL 16 | Shared lead cache, tenant isolation |
| Queue | Redis (Asynq) | Task scheduling, campaign execution |
| Auth | Clerk | Same provider as MagikShot |
| AI | Claude API (Haiku 4.5 Batch) | Sequence generation, play generation |
| Email Sending | Gmail API (OAuth) | User's own account |
| LinkedIn Automation | Chrome Extension (Manifest V3) | User's browser |
| Lead Discovery | RapidAPI (LinkedIn + Email) | Cached in DB |

### Repository Structure

```
moneyprinter/
├── backend/
│   ├── cmd/
│   │   ├── api/            # HTTP server
│   │   └── worker/         # Asynq worker
│   ├── internal/
│   │   ├── handler/        # HTTP handlers
│   │   ├── service/        # Business logic
│   │   ├── repository/     # SQLC-generated DB access
│   │   ├── leads/          # Lead discovery pipeline
│   │   │   ├── linkedin.go # RapidAPI LinkedIn client
│   │   │   ├── email.go    # RapidAPI email finder
│   │   │   └── verify.go   # Email verification
│   │   ├── ai/             # Claude API client
│   │   │   ├── analyzer.go # Website analysis
│   │   │   ├── plays.go    # Sales play generation
│   │   │   └── sequences.go# Email/LinkedIn sequence generation
│   │   ├── gmail/          # Gmail OAuth + send
│   │   ├── campaign/       # Campaign orchestration
│   │   └── worker/         # Async task handlers
│   ├── migrations/
│   ├── queries/            # SQLC SQL files
│   └── pkg/
│       ├── dto/
│       └── errors/
├── frontend/
│   ├── src/
│   │   ├── app/
│   │   │   ├── dashboard/
│   │   │   ├── campaigns/
│   │   │   ├── leads/
│   │   │   ├── sequences/
│   │   │   ├── analytics/
│   │   │   ├── settings/
│   │   │   └── onboarding/
│   │   ├── components/
│   │   ├── hooks/
│   │   ├── stores/
│   │   └── lib/
│   └── public/
├── extension/
│   ├── manifest.json       # Manifest V3
│   ├── background.js       # Service worker (WebSocket to backend)
│   ├── content.js          # LinkedIn DOM interaction
│   ├── popup.html          # Extension popup UI
│   └── popup.js            # Popup logic (invites remaining, queue)
├── docker-compose.yml
├── Makefile
└── README.md
```

---

## 4. Lead Discovery Pipeline

### The Zero-Marginal-Cost Strategy

The key insight: **cache every lead in PostgreSQL**. Once a lead is fetched from an API, it's stored permanently. Subsequent searches that match cached leads cost $0. Over time, the database becomes an asset — more users = better cache = lower per-lead costs.

### Data Flow

```
User approves play
       │
       ▼
┌──────────────┐     cache     ┌───────────────┐
│ Search Query │ ──── hit? ──► │  leads table   │ ──► Return cached
│ (title,      │      │        │  (PostgreSQL)  │
│  company,    │      │ miss   └───────────────┘
│  location)   │      │
│              │      ▼
│              │  ┌────────────────────────┐
│              │  │ RapidAPI: LinkedIn     │
│              │  │ People Search          │
│              │  │ ($0.002/request)       │
│              │  └──────────┬─────────────┘
│              │             │ profiles
│              │             ▼
│              │  ┌────────────────────────┐
│              │  │ RapidAPI: Email Search │
│              │  │ (name + domain)        │
│              │  │ ($0.002/request)       │
│              │  └──────────┬─────────────┘
│              │             │ emails
│              │             ▼
│              │  ┌────────────────────────┐
│              │  │ Email Verification     │
│              │  │ ($0.0025/request)      │
│              │  └──────────┬─────────────┘
│              │             │ verified
│              │             ▼
│              │       Store in DB
│              │       (leads table)
│              │             │
└──────────────┘             ▼
                       Return leads
```

### RapidAPI Providers

#### LinkedIn People Search + Profile Data

**Provider:** [Fresh LinkedIn Profile Data](https://rapidapi.com/freshdata-freshdata-default/api/fresh-linkedin-profile-data) on RapidAPI

| Plan | $/mo | Requests/mo | Per request | Rate limit |
|------|------|-------------|-------------|------------|
| Free | $0 | 50 | — | basic |
| Ultra | $200 | 100,000 | $0.002 | 120/min |
| Mega | $500 | 500,000 | $0.001 | 300/min |

**Key endpoints:**
- **Search People** — by keyword, title, company, location. Returns 10 profiles per page.
- **Get Profile** — full profile data: name, title, company, location, summary, experience, education, LinkedIn URL.

**Data returned per lead:**
- Full name
- Current title
- Current company + company LinkedIn URL
- Location
- Profile summary
- LinkedIn profile URL

#### Email Finding

**Provider:** [Email Search API](https://rapidapi.com/letscrape-6bRBa3QguO5/api/email-search16) on RapidAPI

| Plan | $/mo | Requests/mo | Per request |
|------|------|-------------|-------------|
| Free | $0 | 50 | — |
| 1K | $3 | 1,000 | $0.003 |
| Pro | $25 | 10,000 | $0.0025 |
| Ultra | $75 | 50,000 | $0.0015 |
| Usage-based | $0 base | Unlimited | $0.002 |

**Input:** first name + last name + company domain
**Output:** email address (verified)

#### Email Verification

**Provider:** [MyEmailVerifier](https://rapidapi.com/MyEmailVerifier/api/myemailverifier1) on RapidAPI
- 100 free verifications/day
- $0.0025/verification after that

#### Alternative: Hunter.io

- Free: 25 searches + 50 verifications/month
- Starter: $34/mo
- API: domain search (1 credit per email found), email finder (1 credit per call)
- Rate limit: 15 req/sec
- Has both finding and verification in one platform

### Caching Strategy

```sql
-- Lead freshness TTL: 30 days
-- After 30 days, re-enrich on next access
-- Email verification: re-verify every 60 days

-- Cache hit logic:
-- 1. Hash the search query (title + company_size + location + industry)
-- 2. Check lead_searches table for matching hash with fresh TTL
-- 3. If hit: return cached lead IDs from DB
-- 4. If miss: call LinkedIn API, store results, return
```

### Future: Hiring Signal Detection

**Tool:** [JobSpy](https://github.com/speedyapply/JobSpy) (open-source Python library)
- Scrapes job listings from LinkedIn Jobs, Indeed, Glassdoor, Google, ZipRecruiter
- Use case: detect companies hiring SDRs, BDRs → buying signal for sales plays
- Implementation: Python sidecar service or Go wrapper, runs as daily cron
- **Not needed for MVP** — can be added as a signal source later

---

## 5. Email Infrastructure

### Approach: User Provides Their Own Gmail

Users connect their Google account via OAuth 2.0. We send emails on their behalf using the Gmail API. This means:

- **No managed mailbox cost** — $0 per user
- **No warmup needed** — it's their existing warmed account
- **Better deliverability** — emails come from a real person's inbox
- **Gmail API limits:** 2,000 emails/day (more than enough for outreach)

### Google OAuth Scopes Required

```
https://www.googleapis.com/auth/gmail.send        # Send emails
https://www.googleapis.com/auth/gmail.readonly     # Read replies/opens
https://www.googleapis.com/auth/userinfo.email     # User identity
```

### Email Tracking

- **Open tracking:** 1x1 pixel image hosted on our domain, unique per email
- **Click tracking:** URL redirect through our domain, unique per link
- **Reply detection:** Poll Gmail API for replies to sent messages (or webhook via Gmail push notifications)

### Email Sequence Engine

Each campaign has a sequence of steps with configurable delays:

```
Step 1: Initial outreach          (Day 0)
Step 2: Follow-up if no reply     (Day 3-7)
Step 3: Value-add follow-up       (Day 14-30)
Step 4: Breakup email             (Day 30-100)
```

The worker processes the sequence:
1. Asynq scheduled task checks: "which leads are due for their next step?"
2. For each due lead: generate email (if not pre-generated) → send via Gmail API
3. If lead replies → pause sequence, move to "Conversations"
4. If lead opens but no reply → continue sequence
5. If no activity after final step → mark as "Exhausted"

### Future: Additional Email Providers

- Outlook/Office 365 (Microsoft Graph API)
- Custom SMTP
- Managed mailboxes with warmup (for users without Gmail)

---

## 6. LinkedIn Automation (Chrome Extension)

### Why a Chrome Extension

LinkedIn has **no official API** for sending connection requests or direct messages. The only way to automate LinkedIn outreach is:

1. **Chrome Extension** — runs in user's browser, uses their session (this is what MoneyPrinter uses)
2. **Cloud-based browser automation** — runs headless browsers on servers with user's cookies
3. **Manual** — generate message text, user copies and pastes

Option 1 is the simplest to build and ship. Option 2 is more scalable but more complex and higher ban risk. We start with Option 1.

### How It Works

```
┌────────── Backend ──────────┐     ┌──── Chrome Extension ────┐
│                              │     │                          │
│  linkedin_tasks table:       │ WS  │  background.js:          │
│  - connect(lead, note)      │◄───►│  - Connects via WebSocket│
│  - message(lead, text)      │     │  - Pulls next task       │
│  - view_profile(lead)       │     │  - Reports result        │
│                              │     │                          │
│  Scheduling:                 │     │  content.js:             │
│  - Respect daily limits     │     │  - Injects into LinkedIn │
│  - Randomize timing         │     │  - Clicks "Connect"      │
│  - Warm-up new accounts     │     │  - Types personalized    │
│                              │     │    connection note       │
│  Analytics:                  │     │  - Sends messages        │
│  - Connection accepted      │     │  - Views profiles        │
│  - Message replied           │     │                          │
│  - Profile viewed            │     │  popup.html:             │
│                              │     │  - Shows queue status    │
│                              │     │  - "X invites remaining" │
│                              │     │  - Today's scheduled     │
└──────────────────────────────┘     └──────────────────────────┘
```

### LinkedIn Daily Limits (2026)

| Account Type | Connection Requests/Day | Messages/Day | Profile Views/Day |
|-------------|------------------------|-------------|------------------|
| Free LinkedIn | 15-20 | 50 | 80-100 |
| Sales Navigator | 30-50 | 100 | 150+ |

**Warm-up protocol:** New automation should start at 25% of limits and scale up over 4 weeks.

### Extension Features (MVP)

1. **Connection requests** with personalized note (up to 300 chars)
2. **Direct messages** to existing connections
3. **Profile viewing** as warm-up before connecting (makes connect more natural)
4. **Queue management** — shows upcoming tasks, daily limit status
5. **Human-like behavior** — random delays (30s-5min between actions), realistic typing speed

### Extension Distribution

Chrome Web Store (same as MoneyPrinter's "Roger Companion"):
- Public listing with review process
- Auto-updates
- Trust signal for users
- Review takes 1-3 business days

### Safety Measures

- Random delays between actions (not uniform — normal distribution around 2-3 min)
- Never exceed daily limits
- Pause on weekends (optional, configurable)
- Stop immediately if LinkedIn shows a warning/captcha
- Only run when user has LinkedIn tab open
- No DOM modifications that LinkedIn's detection can fingerprint

---

## 7. AI Layer

### Model: Claude Haiku 4.5 via Batch API

| Metric | Value |
|--------|-------|
| Input cost | $0.50/M tokens (with batch 50% discount) |
| Output cost | $2.50/M tokens (with batch 50% discount) |
| Latency | Async (up to 24h, typically minutes) |
| Quality | Sufficient for sales emails and analysis |

### AI Tasks

#### 1. Website Analysis

**Input:** Scraped website content (HTML → markdown)
**Output:** Structured business profile

```json
{
  "company_name": "MagikShot",
  "product_description": "AI photo studio that transforms selfies into professional photos",
  "features": ["AI headshots", "dating photos", "virtual try-on", "..."],
  "pricing": "$19-199/mo",
  "target_customers": ["job seekers", "dating app users", "real estate agents"],
  "differentiators": ["10 tools in one", "53% cheaper than PhotoAI"],
  "industry": "AI/SaaS/Photography"
}
```

**Estimated tokens:** ~2,000 input (website content), ~500 output = ~$0.002/analysis

#### 2. Sales Play Generation

**Input:** Business profile + product context
**Output:** 3-5 sales plays

```json
{
  "plays": [
    {
      "name": "HR Leaders at Hiring Tech Companies",
      "who": ["Head of People", "VP of Talent", "HR Manager"],
      "where": {
        "industry": ["Software", "Technology"],
        "company_size": "50-500 employees",
        "geo": ["US", "Europe"]
      },
      "signal": "Companies with active SDR/BDR job listings",
      "pitch": "New sales hires need professional LinkedIn presence immediately",
      "channels": ["email", "linkedin"],
      "search_query": {
        "title": "Head of People OR VP Talent Acquisition",
        "company_size": "51-500",
        "industry": "Technology",
        "location": "United States"
      }
    }
  ]
}
```

**Estimated tokens:** ~1,000 input, ~2,000 output = ~$0.006/generation

#### 3. Email Sequence Generation

**Input:** Business profile + play + lead profile
**Output:** 3-4 step email sequence

```json
{
  "steps": [
    {
      "step": 1,
      "delay_days": 0,
      "subject": "Quick question about onboarding headshots at {{company}}",
      "body": "Hi {{first_name}},\n\nI noticed {{company}} has 3 open SDR roles..."
    },
    {
      "step": 2,
      "delay_days": 5,
      "subject": "Re: Quick question about onboarding headshots at {{company}}",
      "body": "Hi {{first_name}},\n\nJust following up..."
    }
  ]
}
```

**Estimated tokens:** ~800 input, ~1,200 output = ~$0.004/sequence

#### 4. LinkedIn Connection Note

**Input:** Business profile + lead profile
**Output:** 300-char connection note

**Estimated tokens:** ~500 input, ~100 output = ~$0.0005/note

### Total AI Cost Per Lead

| Task | Cost |
|------|------|
| Website analysis | $0.002 (amortized across all leads) |
| Play generation | $0.006 (amortized across all leads in play) |
| Email sequence | $0.004 |
| LinkedIn note | $0.0005 |
| **Total per lead** | **~$0.005** |

---

## 8. Multi-Tenant Model

### Tenant Isolation

Each tenant (MagikShot, MagikText, future products) is a separate entity with its own:
- Business profile (website, features, pricing)
- Sales plays
- Campaigns
- Sequences (different product = different email copy)
- Gmail connection
- LinkedIn connection
- Analytics

### Shared Resources

- **Lead database** — a lead (person) exists once in the system. Multiple tenants can target the same lead with different campaigns.
- **Auth** — single Clerk account, tenant switching in UI
- **Infrastructure** — same Postgres, Redis, worker pool

### Data Model

```
tenants (id, name, domain, business_profile, created_at)
   │
   ├── plays (id, tenant_id, name, icp, signal, channels, search_query)
   │
   ├── campaigns (id, tenant_id, play_id, name, status, gmail_account_id)
   │      │
   │      └── campaign_leads (campaign_id, lead_id, sequence_step, status, next_send_at)
   │
   ├── sequences (id, tenant_id, play_id, steps_json)
   │
   └── gmail_accounts (id, tenant_id, email, oauth_token, refresh_token)

leads (id, linkedin_url, name, title, company, domain, email, verified_at, enriched_at)
   │
   └── lead_searches (id, query_hash, filters_json, lead_ids, fetched_at, ttl_days)
```

A lead record is tenant-agnostic. The `campaign_leads` junction table connects leads to tenant-specific campaigns.

---

## 9. Cost Analysis

### Per-Lead Cost Breakdown

| Component | Cost per lead | Notes |
|-----------|--------------|-------|
| LinkedIn search | $0.0002 | 10 results per $0.002 API call |
| Profile enrichment | $0.002 | 1 API call per lead |
| Email finding | $0.002 | 1 API call per lead |
| Email verification | $0.0025 | 1 API call per lead |
| AI sequence generation | $0.005 | Claude Haiku Batch |
| Email sending (Gmail) | $0.00 | User's own account |
| LinkedIn (Extension) | $0.00 | User's own browser |
| **Total (new lead)** | **$0.012** | |
| **Total (cached lead)** | **$0.005** | AI only, lead data from cache |

### Monthly Infrastructure Cost

| Component | Cost | Notes |
|-----------|------|-------|
| PostgreSQL | $0 | Shared with existing infra / self-hosted |
| Redis | $0 | Shared with existing infra / self-hosted |
| VPS/hosting | $20-50/mo | API + worker + frontend |
| Domain + SSL | ~$1/mo | moneyprinter domain |
| **Total fixed** | **~$20-50/mo** | |

### Cost Per Tier (at scale)

| Tier | Leads/mo | New lead cost | Cached % | Effective cost | Revenue | Margin |
|------|----------|--------------|----------|---------------|---------|--------|
| Free | 100 | $1.20 | 0% | $1.20 | $0 | -$1.20 |
| Free (mature) | 100 | $1.20 | 60% | $0.72 | $0 | -$0.72 |
| Starter | 500 | $6.00 | 30% | $4.50 | $49 | 91% |
| Growth | 2,000 | $24.00 | 50% | $14.00 | $149 | 91% |
| Scale | 10,000 | $120.00 | 60% | $56.00 | $399 | 86% |

**Key insight:** As the lead cache grows, the effective cost per lead drops. At 60% cache hit rate, margins exceed 85% on all paid tiers.

### RapidAPI Plan Selection by Stage

| Stage | LinkedIn API | Email API | Monthly cost |
|-------|-------------|-----------|-------------|
| Pre-launch (testing) | Free (50/mo) | Free (50/mo) | $0 |
| Launch (first 10 users) | Ultra ($200, 100K) | Pro ($25, 10K) | $225 |
| Growth (100 users) | Mega ($500, 500K) | Ultra ($75, 50K) | $575 |
| Scale (1000+ users) | Custom / self-hosted | Usage-based | Variable |

---

## 10. Pricing Model

### Proposed Tiers

| Plan | $/mo | Leads/mo | Sequences/mo | Campaigns | Features |
|------|------|----------|-------------|-----------|----------|
| Free | $0 | 100 | 300 | 1 | Email only, 1 Gmail account |
| Starter | $49 | 500 | 1,500 | 3 | Email + LinkedIn, 1 Gmail |
| Growth | $149 | 2,000 | 6,000 | 10 | Email + LinkedIn, 3 Gmail accounts, AI personalization |
| Scale | $399 | 10,000 | 30,000 | Unlimited | Everything + priority support, API access |

### Why These Numbers

- **Free tier at 100 leads** matches MoneyPrinter's free tier. Costs us ~$1.20/user/mo — sustainable as acquisition cost.
- **Starter at $49** undercuts MoneyPrinter's Lite ($99) by 50%. At $0.012/lead, 500 leads cost $6 → 88% margin.
- **Growth at $149** vs MoneyPrinter's Growth ($250). Our AI costs are much lower since we use Haiku batch.
- **Scale at $399** vs MoneyPrinter's Scale ($800). Half the price, same value.

### Revenue Projections

| Month | Free users | Paid users | MRR | API costs | Margin |
|-------|-----------|-----------|-----|-----------|--------|
| 1 | 20 | 2 | $98 | $50 | $48 |
| 3 | 100 | 10 | $690 | $250 | $440 |
| 6 | 500 | 50 | $4,500 | $600 | $3,900 |
| 12 | 2,000 | 200 | $20,000 | $2,000 | $18,000 |

---

## 11. Data Model

### Core Tables

```sql
-- Multi-tenant
CREATE TABLE tenants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    domain TEXT NOT NULL,
    business_profile JSONB,  -- AI-generated from website
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Shared lead cache (tenant-agnostic)
CREATE TABLE leads (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    linkedin_url TEXT UNIQUE,
    first_name TEXT NOT NULL,
    last_name TEXT NOT NULL,
    title TEXT,
    company TEXT,
    company_domain TEXT,
    location TEXT,
    email TEXT,
    email_verified BOOLEAN DEFAULT FALSE,
    email_verified_at TIMESTAMPTZ,
    enriched_at TIMESTAMPTZ,
    source TEXT,  -- 'linkedin_api', 'csv_upload', 'manual'
    raw_data JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Search cache for zero-cost repeat queries
CREATE TABLE lead_searches (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    query_hash TEXT NOT NULL,  -- SHA256 of normalized search params
    filters JSONB NOT NULL,    -- original search params
    result_lead_ids UUID[] NOT NULL,
    fetched_at TIMESTAMPTZ NOT NULL,
    ttl_days INT DEFAULT 30,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_lead_searches_hash ON lead_searches(query_hash);

-- Sales plays
CREATE TABLE plays (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    name TEXT NOT NULL,
    description TEXT,
    icp JSONB NOT NULL,        -- who, where, signal
    search_query JSONB NOT NULL, -- LinkedIn search params
    channels TEXT[] NOT NULL,   -- ['email', 'linkedin']
    status TEXT DEFAULT 'draft', -- draft, active, paused, archived
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Campaigns (a play being executed)
CREATE TABLE campaigns (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    play_id UUID NOT NULL REFERENCES plays(id),
    name TEXT NOT NULL,
    status TEXT DEFAULT 'draft', -- draft, active, paused, completed
    gmail_account_id UUID REFERENCES gmail_accounts(id),
    sequence JSONB NOT NULL,    -- email steps with delays
    linkedin_sequence JSONB,    -- LinkedIn steps
    stats JSONB DEFAULT '{}',   -- sent, opened, replied, etc.
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Campaign-Lead junction (tracks sequence progress per lead)
CREATE TABLE campaign_leads (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    campaign_id UUID NOT NULL REFERENCES campaigns(id),
    lead_id UUID NOT NULL REFERENCES leads(id),
    status TEXT DEFAULT 'queued', -- queued, active, replied, bounced, exhausted, unsubscribed
    current_step INT DEFAULT 0,
    next_send_at TIMESTAMPTZ,
    last_sent_at TIMESTAMPTZ,
    last_opened_at TIMESTAMPTZ,
    last_replied_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(campaign_id, lead_id)
);

-- Gmail OAuth connections
CREATE TABLE gmail_accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    user_id UUID NOT NULL,
    email TEXT NOT NULL,
    access_token TEXT NOT NULL,
    refresh_token TEXT NOT NULL,
    token_expiry TIMESTAMPTZ,
    daily_sent_count INT DEFAULT 0,
    daily_sent_reset_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Email tracking
CREATE TABLE email_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    campaign_lead_id UUID NOT NULL REFERENCES campaign_leads(id),
    event_type TEXT NOT NULL,  -- sent, opened, clicked, replied, bounced
    step INT NOT NULL,
    metadata JSONB,            -- click URL, bounce reason, etc.
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- LinkedIn tasks (queue for Chrome Extension)
CREATE TABLE linkedin_tasks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    campaign_lead_id UUID NOT NULL REFERENCES campaign_leads(id),
    task_type TEXT NOT NULL,   -- connect, message, view_profile
    payload JSONB NOT NULL,    -- note text, message text, profile URL
    status TEXT DEFAULT 'pending', -- pending, sent_to_extension, completed, failed
    scheduled_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    error TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Subscription/billing
CREATE TABLE subscriptions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    plan TEXT NOT NULL,         -- free, starter, growth, scale
    leads_used INT DEFAULT 0,
    leads_limit INT NOT NULL,
    sequences_used INT DEFAULT 0,
    sequences_limit INT NOT NULL,
    current_period_start TIMESTAMPTZ,
    current_period_end TIMESTAMPTZ,
    stripe_subscription_id TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
```

---

## 12. API Design

### Auth & Tenant

```
POST   /api/v1/auth/callback          # Clerk auth
GET    /api/v1/tenants                 # List user's tenants
POST   /api/v1/tenants                 # Create tenant
PUT    /api/v1/tenants/:id             # Update tenant
```

### Onboarding (Website Analysis)

```
POST   /api/v1/websites/analyze        # Submit URL for analysis
GET    /api/v1/websites/:id/status     # Poll analysis status
GET    /api/v1/websites/:id/profile    # Get business profile
PUT    /api/v1/websites/:id/profile    # Edit business profile
```

### Sales Plays

```
POST   /api/v1/plays/generate          # AI-generate plays from profile
GET    /api/v1/plays                    # List plays
GET    /api/v1/plays/:id               # Get play details
PUT    /api/v1/plays/:id               # Edit play
DELETE /api/v1/plays/:id               # Delete play
```

### Leads

```
GET    /api/v1/leads                    # List leads (with filters)
POST   /api/v1/leads/discover           # Trigger lead discovery for a play
GET    /api/v1/leads/discover/:id       # Poll discovery status
POST   /api/v1/leads/upload             # CSV upload
GET    /api/v1/leads/:id               # Get lead details
```

### Campaigns

```
POST   /api/v1/campaigns               # Create campaign
GET    /api/v1/campaigns               # List campaigns
GET    /api/v1/campaigns/:id           # Get campaign + leads + stats
PUT    /api/v1/campaigns/:id           # Update campaign
POST   /api/v1/campaigns/:id/start     # Start sending
POST   /api/v1/campaigns/:id/pause     # Pause campaign
POST   /api/v1/campaigns/:id/leads     # Add leads to campaign
DELETE /api/v1/campaigns/:id/leads/:lid # Remove lead from campaign
```

### Sequences

```
POST   /api/v1/sequences/generate      # AI-generate sequence for play
GET    /api/v1/sequences/:id           # Get sequence
PUT    /api/v1/sequences/:id           # Edit sequence steps
```

### Gmail

```
GET    /api/v1/gmail/auth-url          # Get OAuth URL
POST   /api/v1/gmail/callback          # OAuth callback
GET    /api/v1/gmail/accounts          # List connected accounts
DELETE /api/v1/gmail/accounts/:id      # Disconnect
```

### LinkedIn (Chrome Extension)

```
WS     /api/v1/linkedin/ws             # WebSocket for extension
GET    /api/v1/linkedin/tasks          # Fallback: poll for tasks
POST   /api/v1/linkedin/tasks/:id/done # Report task completed
POST   /api/v1/linkedin/tasks/:id/fail # Report task failed
GET    /api/v1/linkedin/status         # Extension connection status
```

### Analytics

```
GET    /api/v1/analytics/overview      # Dashboard stats
GET    /api/v1/analytics/campaigns/:id # Campaign-specific stats
GET    /api/v1/analytics/timeline      # Activity over time
```

---

## 13. MVP Scope

### Phase 1: Core Pipeline (Weeks 1-3)

**Goal:** Paste URL → get leads → send emails

- [ ] Tenant + auth setup (Clerk)
- [ ] Website analysis (scrape URL + Claude Haiku → business profile)
- [ ] Sales play generation (Claude Haiku → 3-5 plays)
- [ ] Lead discovery (RapidAPI LinkedIn search → email finding → verification)
- [ ] Lead caching in PostgreSQL
- [ ] Email sequence generation (Claude Haiku → 3-4 step sequence)
- [ ] Gmail OAuth integration
- [ ] Email sending via Gmail API
- [ ] Basic email tracking (open pixel, click redirect)
- [ ] Campaign CRUD + lead assignment
- [ ] Sequence execution engine (Asynq scheduled tasks)
- [ ] Basic dashboard (campaigns list, lead list, sent/opened/replied counts)

### Phase 2: LinkedIn + Polish (Weeks 4-6)

**Goal:** Full multi-channel outreach

- [ ] Chrome Extension MVP (connect + message + view profile)
- [ ] WebSocket connection between extension and backend
- [ ] LinkedIn daily limits + warm-up logic
- [ ] LinkedIn task scheduling
- [ ] Conversations view (aggregated replies from email + LinkedIn)
- [ ] Lead upload (CSV import)
- [ ] Campaign analytics (timeline charts, conversion funnel)
- [ ] Sequence editor (edit AI-generated emails before sending)

### Phase 3: Growth (Weeks 7-10)

**Goal:** Multi-tenant + billing + polish

- [ ] Tenant switching UI
- [ ] Stripe billing integration (reuse MagikShot patterns)
- [ ] Usage metering (leads used, sequences sent)
- [ ] Plan limits enforcement
- [ ] Reply detection (Gmail API polling for responses)
- [ ] Bounce handling
- [ ] Unsubscribe link + compliance (CAN-SPAM footer)
- [ ] Chrome Web Store submission
- [ ] Landing page + marketing site

### Phase 4: Scale (Weeks 11+)

- [ ] Hiring signal detection (JobSpy integration)
- [ ] A/B testing for email subject lines
- [ ] AI-powered reply classification (interested/not interested/meeting request)
- [ ] Multiple Gmail accounts per tenant
- [ ] Outlook/Office 365 support
- [ ] Domains & Mailboxes management (for managed sending)
- [ ] Email warmup integration
- [ ] API access for Scale tier customers
- [ ] Webhook notifications

---

## 14. Risk Analysis

### Technical Risks

| Risk | Impact | Likelihood | Mitigation |
|------|--------|-----------|------------|
| RapidAPI LinkedIn provider goes down | High | Medium | Abstract provider interface, have backup providers ready |
| LinkedIn detects Chrome Extension | High | Medium | Human-like delays, respect limits, no detectable DOM mods |
| Gmail OAuth scope rejection by Google | High | Low | Apply for verification early, have clear privacy policy |
| Email deliverability issues | Medium | Medium | SPF/DKIM/DMARC guidance in onboarding, monitor bounce rates |
| Claude API cost spike | Low | Low | Haiku batch is very cheap, can fall back to cheaper models |

### Legal/Compliance Risks

| Risk | Impact | Mitigation |
|------|--------|------------|
| LinkedIn ToS violation (extension) | Medium | Same approach as MoneyPrinter, Expandi, Waalaxy — industry standard |
| CAN-SPAM compliance | High | Include unsubscribe link in every email, honor opt-outs within 10 days |
| GDPR (EU leads) | High | Legitimate interest basis, opt-out in every email, data deletion on request |
| Google API policy | Medium | Clear privacy policy, OAuth consent screen, don't store emails longer than needed |

### Business Risks

| Risk | Impact | Mitigation |
|------|--------|------------|
| MoneyPrinter/Apollo/Instantly undercuts | Medium | Our zero-marginal-cost model + caching gives structural cost advantage |
| Low free-to-paid conversion | Medium | Free tier is generous enough to demonstrate value, paywall at scale |
| Lead data quality too low | High | Multiple verification steps, let users report bad leads, improve over time |

---

## 15. Customer Acquisition Strategy

### 15.1 SEO Strategy

#### Target Keywords

| Keyword Cluster | Examples | Intent | Priority |
|----------------|---------|--------|----------|
| **Primary (tool)** | "cold email software", "AI outreach tool", "sales automation tool" | High-intent, buying | P0 |
| **Comparison** | "apollo alternative", "instantly alternative", "lemlist alternative", "moneyprinter alternative" | High-intent, switching | P0 |
| **Category** | "best cold email tool 2026", "best AI SDR tool", "best outbound sales platform" | Research, comparison | P1 |
| **Problem** | "how to automate cold outreach", "cold email deliverability", "LinkedIn outreach automation" | Educational, top-of-funnel | P1 |
| **Long-tail** | "cheap cold email tool for startups", "open source outreach platform", "self-hosted sales automation", "AI cold email with LinkedIn" | Niche, low competition | P2 |
| **Versus** | "apollo vs instantly", "lemlist vs instantly", "smartlead vs instantly" | Comparison, high-intent | P2 |

**Market signal:** The email warmup tool market alone generates $200M in revenue (2024), projected to reach $600M by 2033. "AI SDR" search interest is at an all-time high.

#### Content Pages to Create at Launch (5-8 pages)

1. **Homepage** — target: "AI outreach tool", "cold email automation"
2. **Pricing page** — target: "[product] pricing", comparison hooks
3. **"MagikLead vs Apollo"** — comparison page targeting switching users
4. **"MagikLead vs Instantly"** — comparison page
5. **"MagikLead vs Lemlist"** — comparison page
6. **"Best Cold Email Tools 2026"** — listicle (include ourselves, honest rankings)
7. **"How to Set Up AI-Powered Cold Outreach"** — tutorial/guide targeting long-tail
8. **"Open Source Outreach: Why We Built MagikLead"** — story page for HN/indie audience

#### Backlink Strategy

| Channel | Action | Timeline |
|---------|--------|----------|
| Product Hunt | Launch with assets, aim for top 5 of the day | Week 1 |
| AlternativeTo | List as alternative to Apollo, Instantly, Lemlist | Week 1 |
| GitHub | Open-source core components, README with demo | Week 1 |
| Hacker News | "Show HN: We built an open-source alternative to Apollo" | Week 1-2 |
| Indie Hackers | Build-in-public updates | Ongoing |
| G2 / Capterra | Create listing, solicit early reviews | Week 2-3 |
| r/SaaS, r/startups, r/sales | Value posts (not spam), share learnings | Ongoing |
| Guest posts | Target SaaS blogs (Close.com blog, Saleshandy blog accept guest posts) | Month 2+ |

### 15.2 Paid Ads Strategy

#### Channel Selection

| Channel | Budget/2wk | Why | Expected CPA |
|---------|-----------|-----|-------------|
| **Google Ads** | $150 | High-intent searches: "cold email tool", "AI outreach software" | $15-40 |
| **Reddit Ads** | $100 | Target r/sales, r/SaaS, r/startups — niche, engaged | $10-25 |
| **Twitter/X Ads** | $50 | Tech/founder audience, retweet amplification | $20-50 |

**Total initial test budget: $300 over 2 weeks**

**Not recommended initially:** Meta Ads (too broad for B2B), LinkedIn Ads (too expensive, $5-10 CPC).

#### Ad Creative Concepts

**Concept 1: Price Undercut**
- Hook: "Paying $250/mo for MoneyPrinter? Try the same thing for $49."
- Body: AI-powered outreach. Email + LinkedIn. 500 leads/mo.
- CTA: "Start free — no credit card"
- Target: Founders, sales leaders searching for alternatives

**Concept 2: Open Source Angle**
- Hook: "The first open-source AI outreach platform"
- Body: Full pipeline: leads → emails → LinkedIn. Self-host or cloud.
- CTA: "See it on GitHub"
- Target: Technical founders, indie hackers

**Concept 3: Pain Point**
- Hook: "Tired of AI emails that sound like AI?"
- Body: MagikLead generates outreach that gets replies, not eye-rolls. Human-quality sequences powered by Claude AI.
- CTA: "Send your first campaign free"
- Target: SDRs and sales teams frustrated with "AI slop" from competitors

### 15.3 Organic / Launch Strategy

#### Product Hunt Launch Plan

- **Timing:** Tuesday or Wednesday, 12:01 AM PST
- **Prep (1 week before):** Teaser on Twitter/X, notify followers, prep assets (logo, screenshots, demo GIF, maker comment)
- **Launch day assets:** 5 product screenshots, 1 demo video (60s), tagline, maker story
- **Goal:** Top 5 Product of the Day (Mailgo hit #1 in June 2025 in this category — precedent exists)

#### Community Engagement

| Community | Approach |
|-----------|----------|
| r/SaaS (280K+ members) | Share build-in-public updates, respond to "looking for X" posts |
| r/sales (350K+ members) | Share cold outreach tips, mention tool when relevant |
| r/startups | Launch announcement, feedback request |
| Indie Hackers | Revenue milestone updates, transparent build log |
| Sales Hacker Slack | Participate in discussions, share expertise |
| LinkedIn (personal) | Weekly posts about building the product, cold outreach tips |

#### Build-in-Public Strategy

- Weekly Twitter/X thread: what we built, metrics, learnings
- Monthly Indie Hackers revenue update
- Open GitHub repo with star count as social proof
- Transparent pricing page showing cost breakdown (trust signal)

---

## 16. Differentiation Strategy

### Our Angle: Cheaper + Open

We can't beat Apollo's database (275M contacts) or Amplemarket's feature breadth (219/231 features). We win on **price** and **transparency**.

### The Three Pillars

| Pillar | What It Means | Why It Works |
|--------|--------------|-------------|
| **50% cheaper** | $49/mo vs MoneyPrinter $250, Lemlist $109, Apollo $99 | Lemlist's Feb 2026 price hike to $79/user pushed users to seek alternatives. Apollo credits run out fast. |
| **Zero-marginal-cost leads** | Cached lead database gets cheaper over time | Structural cost advantage — more users = better cache = lower per-lead cost |
| **Open approach** | Open-source core, self-hostable, transparent pricing | Trust signal. No vendor lock-in. Resonates with technical founders and indie hackers. Competitors like OutreachStud.io (open source) exist but are immature. |

### One-Sentence Positioning

> **"MagikLead is the affordable, AI-powered outreach platform for founders who refuse to pay enterprise prices for cold email."**

### Why Would Someone Switch?

| From | Why They'd Switch |
|------|-------------------|
| MoneyPrinter ($250/mo) | 5x cheaper for the same core flow (paste URL → leads → sequences → send) |
| Lemlist ($79-109/user) | Per-seat model is expensive for teams. We charge per account, not per seat. |
| Apollo ($49-99/user) | Credit system is confusing and runs out fast. We include leads in the plan. |
| Instantly ($47-97/mo) | Email only, no LinkedIn. We do both. |
| Waalaxy (€49-69/user) | LinkedIn only, no email (until Business tier at €69). We do both from $49. |

### Honest Risk Assessment

**When we lose:** Enterprises needing CRM integration, phone dialer, or WhatsApp → they'll pick Amplemarket or Outreach. Teams needing a 275M contact database → they'll stay on Apollo. We don't compete here, and that's fine.

---

## 17. Domain & Brand Strategy

### Domain Availability (Checked April 10, 2026)

| Domain | Status |
|--------|--------|
| **magiklead.com** | Available |
| **magiklead.ai** | Available |
| **magiklead.io** | Available |
| leadprinter.com | Taken |
| coldcraft.ai | Taken |
| outreachai.io | Taken |
| pipeprinter.com | Taken |
| prospectai.io | Taken |
| sellflow.ai | Taken |

**Recommendation:** Register **magiklead.com** (primary) and **magiklead.ai** (redirect). The name matches the repo, is memorable, and implies both "magic" and "lead generation."

### Brand Positioning

**Target Persona:**
Solo founders and small sales teams (1-5 people) at early-stage startups who are doing their own outbound. They've tried a free tier of Apollo or Instantly, hit limits, and are looking for something affordable that does email + LinkedIn in one tool. They're technical enough to appreciate an open-source approach and cost transparency.

**Tone of Voice:** Direct, no-BS, builder-friendly. Like talking to a fellow founder at a coffee shop. No corporate jargon, no "unlock your revenue potential" fluff.

**Landing Page:**
- **Headline:** "AI outreach that doesn't cost a fortune"
- **Subheadline:** "Paste your website. Get leads. Send personalized emails and LinkedIn messages. Starting at $49/mo."
- **3 Value Props:**
  1. **All-in-one pipeline** — Leads + email + LinkedIn in a single tool. No duct-taping 3 platforms together.
  2. **50% cheaper than competitors** — Same AI-powered outreach as MoneyPrinter at a fraction of the cost.
  3. **Open and transparent** — See our code, see our costs. No credit gotchas, no per-seat surprises.

---

## 18. Legal & Compliance

### Specific Risks for This Product

| Risk | Severity | Details |
|------|----------|---------|
| **LinkedIn ToS violation** | Medium | Chrome Extension automates LinkedIn actions, which violates LinkedIn's ToS. However, this is industry-standard practice (Waalaxy, Expandi, Lemlist, MoneyPrinter all do it). LinkedIn's enforcement is rate-limit based, not legal action against tool providers. |
| **CAN-SPAM compliance** | High | Every cold email must include: physical address, unsubscribe link, honest subject lines. Opt-outs must be honored within 10 business days. |
| **GDPR (EU leads)** | High | Legitimate interest basis for B2B outreach is defensible but requires: clear opt-out in every email, data deletion on request, privacy policy disclosing data sources. |
| **CCPA (California leads)** | Medium | Right to know, right to delete, right to opt out. Must provide mechanism for California residents. |
| **Google OAuth review** | Medium | Gmail send scope requires Google verification. Need: clear privacy policy, limited scope justification, HTTPS, and verified domain. Apply early — review takes 2-6 weeks. |
| **RapidAPI data usage** | Low | LinkedIn data via RapidAPI is scraped data. No direct contractual risk to us, but the data provider could be shut down. Abstract the provider interface. |

### Required Legal Pages

1. **Privacy Policy** — Disclose: what data we collect (lead data, user Gmail tokens), how we use it, third-party sharing (RapidAPI, Claude API), GDPR/CCPA rights, data retention periods, cookie usage
2. **Terms of Service** — Acceptable use policy (no spam, compliance with CAN-SPAM), account termination, liability limitations, arbitration clause
3. **Refund Policy** — 7-day refund window for paid plans. No refund after lead credits are consumed.
4. **CAN-SPAM Compliance Guide** — In-app guide for users explaining their obligations when sending cold email

### Mitigations

- Include unsubscribe link in every email automatically (non-removable)
- Add physical address field during onboarding (required for CAN-SPAM)
- Implement "data deletion request" endpoint for GDPR/CCPA
- Rate-limit LinkedIn automation well below detection thresholds
- Apply for Google OAuth verification in week 1 of development
- Store Gmail tokens encrypted at rest

---

## 19. Validation Framework & Kill Criteria

### 2-Week Validation Window

| Period | Focus | Actions |
|--------|-------|---------|
| **Week 1 (Days 1-7)** | Organic launch | Product Hunt launch, HN Show post, Reddit/IH posts, GitHub repo public, landing page live with waitlist |
| **Week 2 (Days 8-14)** | Paid validation | $300 ad spend (Google $150, Reddit $100, Twitter $50), retarget PH/HN visitors, email waitlist for conversions |

### Success Metrics with Kill Thresholds

| Metric | Success | Cautious | Kill |
|--------|---------|----------|------|
| **Signups (week 1)** | > 100 | 30-100 | < 30 |
| **Signups (week 2, with ads)** | > 250 total | 100-250 | < 100 total |
| **Free → Paid conversion (day 14)** | > 5% | 2-5% | < 2% |
| **Paying customers (day 14)** | > 5 | 2-5 | < 2 |
| **CAC from paid channel** | < $30 | $30-60 | > $60 |
| **GitHub stars (week 2)** | > 200 | 50-200 | < 50 |
| **Product Hunt upvotes** | > 300 | 100-300 | < 100 |

### Revenue Targets

| Milestone | Target | Timeline |
|-----------|--------|----------|
| First paying customer | $49 MRR | Day 7-10 |
| Break-even on ads | 7 paid customers ($343 MRR) | Day 14 |
| Ramen profitable | $1,000 MRR | Month 2 |
| Sustainable | $5,000 MRR | Month 6 |

### Break-Even Math

- Monthly fixed costs: ~$50 (infra) + $225 (APIs at launch stage) = **$275/mo**
- ARPU (blended): ~$70 (mix of $49 and $149 plans)
- Break-even: $275 / $70 = **4 paying customers**
- With $300/mo ad spend: ($275 + $300) / $70 = **9 paying customers**

### Decision Framework

**Kill if (day 14):**
- < 100 total signups AND < 2 paying customers
- No organic interest (0 GitHub stars, 0 inbound inquiries)
- Every ad creative has CPA > $60
- **Action:** Shut down ads, archive repo, move on. Total sunk cost: ~$300 ads + development time.

**Pivot if (day 14):**
- 100-250 signups but < 2% conversion
- Users sign up but don't complete onboarding
- Specific feature requests cluster around a different use case
- **Pivot options:**
  1. **Niche down:** Focus only on LinkedIn automation (Waalaxy competitor) — simpler, faster to build
  2. **Agency tool:** Pivot to white-label outreach for agencies (Smartlead competitor) — higher ARPU
  3. **Lead data only:** Strip outreach, sell just the cached lead database as an API (Apollo data competitor)
  4. **Email-only:** Drop LinkedIn Extension, compete purely on email (Instantly competitor at lower price)

**Double down if (day 14):**
- > 250 signups AND > 5 paying customers
- Organic traffic growing (HN/PH referrals converting)
- Users completing full flow (paste URL → discover leads → send emails)
- **Action:** Increase ad budget to $500/mo, accelerate LinkedIn Extension (Phase 2), hire first contractor for frontend polish

### Cost of Validation

| Item | Cost |
|------|------|
| Domain (magiklead.com) | ~$12 |
| Ads budget (2 weeks) | $300 |
| RapidAPI (free tier during dev) | $0 |
| Vercel (free tier) | $0 |
| Hetzner VPS (smallest) | ~$5 |
| **Total validation cost** | **~$317** |

---

## 20. Summary & Next Steps

### Go/No-Go Recommendation

**GO** — with caveats. The market is large ($9B+ sales automation), growing fast (20%+ YoY), and fragmented. Competitor pricing is rising (Lemlist's Feb 2026 hike). User complaints center on price, credit confusion, and "AI slop" quality — all addressable. The zero-marginal-cost caching model gives a structural advantage. Total validation cost is ~$317.

### Top 3 Risks to Monitor

1. **Google OAuth verification delay** — Apply in week 1 of development. If rejected, fall back to SMTP setup guide.
2. **RapidAPI LinkedIn provider reliability** — Abstract the provider interface from day 1. Have backup providers identified.
3. **"AI slop" perception** — Users are switching away from tools with low-quality AI emails. Our Claude Haiku sequences must be meaningfully better. Test with real recipients before launch.

### Top 3 Differentiators

1. **Price** — 50-80% cheaper than MoneyPrinter, Lemlist, and Apollo at equivalent tiers
2. **All-in-one** — Email + LinkedIn in one tool (Instantly = email only, Waalaxy = LinkedIn only)
3. **Open approach** — Open-source core, transparent costs, no credit gotchas

### Estimated Monthly Cost (at launch)

| Item | Cost |
|------|------|
| Hetzner VPS | $20 |
| Domain | $1 |
| RapidAPI (Ultra) | $225 |
| Claude API (Haiku batch) | ~$10 |
| Vercel (free) | $0 |
| Clerk (free tier) | $0 |
| Ads | $300 |
| **Total** | **~$556/mo** |

Break-even at **9 paying customers** (including ads) or **4 customers** (organic only).

### Immediate Actions for Planning Phase

1. Register `magiklead.com` + `magiklead.ai`
2. Set up monorepo (backend Go + frontend Next.js + extension)
3. Apply for Google OAuth verification
4. Build Phase 1 MVP: paste URL → discover leads → generate sequences → send via Gmail
5. Prepare Product Hunt launch assets

---

## 21. Sources

### Product Research
- [MoneyPrinter.me](https://moneyprinter.me/) — Product homepage, pricing, feature analysis
- [HireRoger.com](https://www.hireroger.com/) — Parent company, AI sales agent positioning
- [Roger Companion Chrome Extension](https://moneyprinter.me/) — LinkedIn automation via browser extension (observed during trial)

### Lead Discovery APIs
- [Fresh LinkedIn Profile Data — RapidAPI](https://rapidapi.com/freshdata-freshdata-default/api/fresh-linkedin-profile-data) — LinkedIn people search + profile enrichment
- [Email Search API — RapidAPI](https://rapidapi.com/letscrape-6bRBa3QguO5/api/email-search16/pricing) — Email finding from name + domain
- [MyEmailVerifier — RapidAPI](https://rapidapi.com/MyEmailVerifier/api/myemailverifier1) — Email verification
- [Hunter.io Pricing](https://hunter.io/pricing) — Alternative email finder
- [Netrows — LinkedIn Data API Comparison 2026](https://www.netrows.com/blog/best-linkedin-data-api-providers-2026) — Provider comparison

### Email Infrastructure
- [Instantly.ai — Email API for Cold Outreach](https://instantly.ai/blog/email-api-for-cold-outreach-unlimited-accounts-warmup-ramp-plans/) — Warmup + sending
- [Best Email Warm-Up APIs 2026](https://prospeo.io/s/email-warm-up-api) — Warmup service comparison

### LinkedIn Automation
- [LinkedIn Automation Daily Limits 2026](https://blog.linkboost.co/linkedin-automation-daily-limits-guidelines-2026/) — Safe daily limits
- [LinkedIn Automation Chrome Extensions 2026](https://blog.linkboost.co/best-linkedin-automation-chrome-extensions-2026/) — Extension landscape
- [LinkedIn Sales Navigator Pricing](https://business.linkedin.com/sales-solutions/compare-plans) — $99-160/mo per seat

### AI/LLM
- [Claude API Pricing](https://platform.claude.com/docs/en/about-claude/pricing) — Haiku $1/$5 per M tokens, Batch 50% discount
- [Claude Batch API](https://platform.claude.com/docs/en/about-claude/pricing) — Async processing, 50% discount

### Open Source Tools
- [JobSpy — GitHub](https://github.com/speedyapply/JobSpy) — Open-source job board scraper (LinkedIn, Indeed, Glassdoor)
- [Scrapy](https://scrapy.org/) — Python web scraping framework

### Competitive Landscape (Updated April 2026)
- [Apollo.io Pricing](https://www.apollo.io/pricing) — $0-99/user/mo, credit system
- [Instantly.ai Pricing](https://instantly.ai/pricing) — $47-358/mo, unlimited accounts + warmup
- [Lemlist Pricing](https://www.lemlist.com/pricing) — $63-109/user/mo, Feb 2026 price hike
- [Waalaxy Pricing](https://www.waalaxy.com/pricing/) — €0-69/user/mo, cheapest LinkedIn automation
- [Saleshandy Review of Lemlist](https://www.saleshandy.com/blog/lemlist-review/) — User complaints and limitations
- [Lemlist vs Apollo Comparison](https://www.saleshandy.com/blog/lemlist-vs-apollo/) — Feature comparison
- [2026 GTM Stack — Landbase](https://www.landbase.com/blog/2026-gtm-stack-replacing-apollo-salesloft-outreach) — Industry trends

### Market Research (April 2026)
- [AI in Sales Market Report — Grand View Research](https://www.grandviewresearch.com/industry-analysis/artificial-intelligence-ai-sales-market-report) — $9B+ market, 20%+ CAGR
- [Sales Automation Statistics 2026](https://utmost.agency/blogs/sales-automation-statistics/) — 83% of AI-using teams report revenue growth
- [Best AI Sales Engagement Platforms 2026 — Amplemarket](https://www.amplemarket.com/blog/best-ai-sales-engagement-platforms-2026) — Feature scoring (Amplemarket 219/231)
- [AI SDR Tools Comparison 2026 — Coldreach](https://coldreach.ai/blog/ai-sales-agents-for-b2b-outreach) — "AI slop" user complaints
- [Artisan AI Review 2026](https://coldreach.ai/blog/artisan-ai-review) — User feedback on autonomous AI SDRs
- [Future of Cold Email 2026-2027 — Instantly](https://instantly.ai/blog/future-of-cold-email-ai-personalization-automation-trends-shaping-2026-2027/) — Deliverability-first trends
- [Best AI Sales Tools — Product Hunt](https://www.producthunt.com/categories/ai-sales-tools) — Launch precedents
- [Mailgo Product Hunt Launch (June 2025)](https://www.globenewswire.com/news-release/2025/06/03/3093110/0/en/Mailgo-launches-AI-powered-Cold-Email-Outreach-Tool.html) — #1 Product of the Day

### Open Source Alternatives
- [OutreachStud.io — GitHub](https://github.com/OutreachStud-io/studio) — Open-source cold email outreach
- [Email-automation — GitHub](https://github.com/PaulleDemon/Email-automation) — Open-source email automation
- [Meteor-emails — GitHub](https://github.com/catin-black/meteor-emails) — Free open-source cold email tool

