package pacer_test

import (
	"testing"
	"time"

	"github.com/jcleira/magiklead/backend/internal/linkedin/pacer"
)

// now is a fixed reference instant. The pacer takes the clock as an
// argument (never reads the wall clock itself), so every case is
// deterministic — windows are expressed relative to this anchor.
var now = time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)

func TestAllowance(t *testing.T) {
	std := pacer.Standard() // WeeklyCap 100, DailyCap 20

	cases := []struct {
		name     string
		limits   pacer.Limits
		counters pacer.Counters
		want     int
	}{
		{
			// A freshly connected account with no windows yet: both
			// budgets are full, so the daily sub-cap is the binding
			// constraint.
			name:     "fresh account, no windows",
			limits:   std,
			counters: pacer.Counters{},
			want:     20,
		},
		{
			// Weekly ceiling reached inside the current week → nothing,
			// even though the daily window is fresh.
			name:   "weekly cap reached this week",
			limits: std,
			counters: pacer.Counters{
				WeeklyCount:       100,
				WeeklyWindowStart: now.Add(-2 * 24 * time.Hour),
			},
			want: 0,
		},
		{
			// Daily sub-cap reached today blocks sending even with weekly
			// headroom.
			name:   "daily cap reached today",
			limits: std,
			counters: pacer.Counters{
				WeeklyCount:       30,
				WeeklyWindowStart: now.Add(-2 * 24 * time.Hour),
				DailyCount:        20,
				DailyWindowStart:  now.Add(-3 * time.Hour),
			},
			want: 0,
		},
		{
			// Weekly window older than 7 days → the stored weekly count is
			// stale and counts as a fresh full week. Daily sub-cap binds.
			name:   "expired weekly window resets to full budget",
			limits: std,
			counters: pacer.Counters{
				WeeklyCount:       100,
				WeeklyWindowStart: now.Add(-8 * 24 * time.Hour),
			},
			want: 20,
		},
		{
			// Daily window older than 24h → today's count is stale; daily
			// budget is full again. Weekly headroom is the binding cap here.
			name:   "expired daily window resets daily budget",
			limits: std,
			counters: pacer.Counters{
				WeeklyCount:       95,
				WeeklyWindowStart: now.Add(-1 * 24 * time.Hour),
				DailyCount:        20,
				DailyWindowStart:  now.Add(-25 * time.Hour),
			},
			want: 5, // 100-95 weekly remaining < 20 daily
		},
		{
			// Partial week, fresh day: min(weekly remaining, daily cap).
			name:   "weekly remaining below daily cap",
			limits: std,
			counters: pacer.Counters{
				WeeklyCount:       98,
				WeeklyWindowStart: now.Add(-1 * time.Hour),
			},
			want: 2,
		},
		{
			// Counts somehow above the cap (clock skew / manual edit) must
			// never produce a negative allowance.
			name:   "over-cap never goes negative",
			limits: std,
			counters: pacer.Counters{
				WeeklyCount:       150,
				WeeklyWindowStart: now.Add(-1 * time.Hour),
			},
			want: 0,
		},
		{
			// Custom limits are honoured (warmup ramp in issue #7 passes a
			// smaller weekly cap through this same path).
			name:   "custom limits honoured",
			limits: pacer.Limits{WeeklyCap: 10, DailyCap: 5},
			counters: pacer.Counters{
				WeeklyCount:       8,
				WeeklyWindowStart: now.Add(-1 * time.Hour),
			},
			want: 2,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.limits.Allowance(tc.counters, now)
			if got != tc.want {
				t.Errorf("Allowance() = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestWarmupRamp pins the multi-week warmup curve (issue #7): a freshly
// warming account's daily allowance ramps from ~8/day in week 1 to the full
// ~20/day sub-cap by week 4, driven off how long ago warmup_started_at was.
// Windows are fresh in every case so the ramp is the binding cap — except
// the last, which proves the 100/week ceiling still wins when the ramp would
// allow more. The clock is injected (now), as everywhere in this suite.
func TestWarmupRamp(t *testing.T) {
	std := pacer.Standard() // WeeklyCap 100, DailyCap 20
	const day = 24 * time.Hour

	cases := []struct {
		name   string
		warmup time.Time      // warmup_started_at
		extra  pacer.Counters // window counters layered under the warmup anchor
		want   int
	}{
		{"week 1, day 0", now, pacer.Counters{}, 8},
		{"week 1, day 3", now.Add(-3 * day), pacer.Counters{}, 8},
		{"week 2", now.Add(-7 * day), pacer.Counters{}, 12},
		{"week 3", now.Add(-14 * day), pacer.Counters{}, 16},
		{"week 4 reaches the full daily cap", now.Add(-21 * day), pacer.Counters{}, 20},
		{"fully warmed past week 4", now.Add(-60 * day), pacer.Counters{}, 20},
		{
			// The ramp would allow 20/day in week 4, but only 5 remain on the
			// weekly ceiling — the 100/week cap still binds over the ramp.
			name:   "weekly ceiling caps the ramp",
			warmup: now.Add(-21 * day),
			extra:  pacer.Counters{WeeklyCount: 95, WeeklyWindowStart: now.Add(-1 * time.Hour)},
			want:   5,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := tc.extra
			c.WarmupStartedAt = tc.warmup
			if got := std.Allowance(c, now); got != tc.want {
				t.Errorf("Allowance() = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestAcceptanceBreaker pins the acceptance-rate circuit breaker (issue #7):
// once an account has a meaningful sample of trailing-7-day invites, a
// sub-20% acceptance rate trips the breaker — Allowance returns 0 and
// BreakerTripped reports true so the worker can flag the account paused —
// and a recovery to ≥20% clears it. Below the minimum sample the breaker
// abstains, so a young account whose invites simply haven't been accepted
// yet is never paused.
func TestAcceptanceBreaker(t *testing.T) {
	std := pacer.Standard()

	cases := []struct {
		name        string
		sent        int
		accepted    int
		wantTripped bool
	}{
		{"healthy acceptance", 40, 20, false},                // 50%
		{"exactly at the 20% floor stays up", 50, 10, false}, // 20% — not below
		{"below the floor trips", 50, 5, true},               // 10%
		{"zero acceptance with enough sample trips", 30, 0, true},
		{"tiny sample never trips (acceptances lag)", 5, 0, false},
		{"just under min sample never trips", pacer.BreakerMinSample - 1, 0, false},
		{"at min sample, below floor trips", pacer.BreakerMinSample, 0, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := pacer.Counters{InvitesSent7d: tc.sent, Accepted7d: tc.accepted}
			if got := c.BreakerTripped(); got != tc.wantTripped {
				t.Errorf("BreakerTripped() = %v, want %v", got, tc.wantTripped)
			}
			// A tripped breaker forces Allowance to 0 regardless of headroom;
			// otherwise the window/warmup caps apply (here: fresh windows, no
			// warmup anchor → the full daily cap, so a positive allowance).
			got := std.Allowance(c, now)
			if tc.wantTripped && got != 0 {
				t.Errorf("Allowance() = %d, want 0 when breaker tripped", got)
			}
			if !tc.wantTripped && got == 0 {
				t.Errorf("Allowance() = 0, want >0 when breaker not tripped")
			}
		})
	}
}

// TestAcceptanceRate pins the rate math and the enough-data gate the breaker
// leans on: below the minimum sample the rate is untrustworthy (ok=false);
// at or above it the ratio is exact.
func TestAcceptanceRate(t *testing.T) {
	if _, ok := (pacer.Counters{InvitesSent7d: 3, Accepted7d: 0}).AcceptanceRate(); ok {
		t.Error("below min sample must report enoughData=false")
	}
	rate, ok := (pacer.Counters{InvitesSent7d: 40, Accepted7d: 10}).AcceptanceRate()
	if !ok {
		t.Fatal("with enough sample must report enoughData=true")
	}
	if rate != 0.25 {
		t.Errorf("rate=%v want 0.25", rate)
	}
}

// TestEffectiveWeeklyCap pins how the warmup ramp lowers the weekly
// invite ceiling actually reachable as of now (issue #9's capacity
// read). LinkedIn's flat ceiling is 100/week, but a young account's
// daily warmup sub-cap throttles a week to at most 7×(daily cap): ~56 in
// week 1, ~84 in week 2, then the full 100 from week 3 on once the ramp
// would allow more than the weekly ceiling. No warmup anchor → the full 100.
func TestEffectiveWeeklyCap(t *testing.T) {
	std := pacer.Standard()
	const day = 24 * time.Hour
	cases := []struct {
		name   string
		warmup time.Time
		want   int
	}{
		{"no warmup anchor → full weekly cap", time.Time{}, 100},
		{"week 1 throttles to 7×8", now, 56},
		{"week 2 throttles to 7×12", now.Add(-7 * day), 84},
		{"week 3 ramp exceeds weekly cap → 100", now.Add(-14 * day), 100},
		{"week 4 fully warmed → 100", now.Add(-21 * day), 100},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := std.EffectiveWeeklyCap(tc.warmup, now); got != tc.want {
				t.Errorf("EffectiveWeeklyCap() = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestWeeklyRemaining pins the capacity-indicator read behind the
// dashboard (issue #9): the effective weekly cap (warmup-adjusted) minus
// the invites used in the current weekly window, never negative, and
// resetting to the full cap once the weekly window has elapsed. Unlike
// Allowance it ignores the daily sub-cap and the acceptance breaker — it
// answers "how many more invites fit in this week's budget," not "how
// many may go out right now."
func TestWeeklyRemaining(t *testing.T) {
	std := pacer.Standard()
	const day = 24 * time.Hour
	cases := []struct {
		name     string
		counters pacer.Counters
		want     int
	}{
		{
			name:     "fresh account, no warmup, no window → full 100",
			counters: pacer.Counters{},
			want:     100,
		},
		{
			name: "warmed account mid-week → 100 − used",
			counters: pacer.Counters{
				WeeklyCount:       30,
				WeeklyWindowStart: now.Add(-1 * day),
				WarmupStartedAt:   now.Add(-60 * day),
			},
			want: 70,
		},
		{
			name: "week-1 warmup caps the week at 56 → 56 − used",
			counters: pacer.Counters{
				WeeklyCount:       10,
				WeeklyWindowStart: now.Add(-1 * time.Hour),
				WarmupStartedAt:   now,
			},
			want: 46,
		},
		{
			name: "expired weekly window resets to the full effective cap",
			counters: pacer.Counters{
				WeeklyCount:       80,
				WeeklyWindowStart: now.Add(-8 * day),
				WarmupStartedAt:   now.Add(-60 * day),
			},
			want: 100,
		},
		{
			name: "over-cap (skew / manual edit) never goes negative",
			counters: pacer.Counters{
				WeeklyCount:       120,
				WeeklyWindowStart: now.Add(-1 * time.Hour),
			},
			want: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := std.WeeklyRemaining(tc.counters, now); got != tc.want {
				t.Errorf("WeeklyRemaining() = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestStandard pins the published Standard-plan caps so a careless edit
// to the constants is caught — issue #4 ships weekly 100 + a daily sub-cap.
func TestStandard(t *testing.T) {
	l := pacer.Standard()
	if l.WeeklyCap != 100 {
		t.Errorf("Standard weekly cap = %d, want 100", l.WeeklyCap)
	}
	if l.DailyCap <= 0 || l.DailyCap > l.WeeklyCap {
		t.Errorf("Standard daily cap = %d, want a positive sub-cap below the weekly cap", l.DailyCap)
	}
}
