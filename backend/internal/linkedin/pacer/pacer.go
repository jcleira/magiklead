// Package pacer is the LinkedIn invite capacity policy — pure logic, no
// DB and no clock of its own. Given an account's rolling counters and a
// caller-supplied "now", it answers one question: how many connection
// invites may this account send right now without breaching its weekly
// ceiling or daily sub-cap?
//
// LinkedIn enforces a hard weekly invitation limit (≈100/week on a
// Standard account); blowing past it is the fastest way to get an
// account restricted. The daily sub-cap spreads those invites out so a
// week's budget isn't burned in one sitting. Issue #4 shipped those two
// flat caps; issue #7 layers the full Standard safety policy on top, all
// through this same pure Allowance call:
//
//   - a multi-week warmup ramp that eases a freshly warming account from
//     ~8 invites/day up to the full daily sub-cap by week 4
//     (WarmupStartedAt + warmupDailyCap), and
//   - an acceptance-rate circuit breaker that returns 0 — pausing the
//     account — while its trailing-7-day connection-acceptance rate sits
//     below BreakerThreshold (Counters.BreakerTripped).
//
// Stale-invite withdrawal is the worker's job, not the pacer's, since it
// acts on parked leads rather than computing an allowance.
//
// Per docs/2026-06-09-linkedin-only-outreach/issues/07-pacer-warmup-breaker-withdrawal.md.
package pacer

import "time"

const (
	// StandardWeeklyCap is LinkedIn's weekly connection-invite ceiling on
	// a Standard (free) account.
	StandardWeeklyCap = 100
	// DefaultDailyCap is the per-day sub-cap. It keeps a week's invites
	// from front-loading into a single day, which itself looks automated.
	// A conservative fraction of the weekly cap; the warmup ramp below
	// starts well under it and climbs to it.
	DefaultDailyCap = 20

	// WarmupWeek1DailyCap is the daily invite ceiling during an account's
	// first warmup week. The ramp climbs warmupStepPerWeek invites/day each
	// subsequent week until it reaches the full DailyCap — ~8/day in week 1
	// to ~20/day by week 4 on a Standard account (issue #7). A freshly
	// connected account that sends at full tilt is the fastest way to get
	// restricted, so it eases in.
	WarmupWeek1DailyCap = 8
	warmupStepPerWeek   = 4

	// BreakerThreshold is the trailing-7-day connection-acceptance rate
	// below which the acceptance circuit breaker pauses an account's invites
	// (issue #7). A persistently low accept rate is LinkedIn's signal that
	// the outreach reads as spam; backing off protects the account from a
	// restriction.
	BreakerThreshold = 0.20
	// BreakerMinSample is the minimum number of invites in the trailing
	// window before the breaker may trip. Acceptances lag invites by days,
	// so a young account always shows a low rate at first; requiring a
	// sample keeps the breaker from pausing an account that simply hasn't
	// had time to be accepted yet.
	BreakerMinSample = 20

	weeklyWindow = 7 * 24 * time.Hour
	dailyWindow  = 24 * time.Hour
)

// Limits is the capacity envelope for one account. Construct with
// Standard for the issue-#4 flat caps, or build one directly when a
// caller (issue #7's ramp) wants a tighter ceiling.
type Limits struct {
	WeeklyCap int
	DailyCap  int
}

// Standard returns the flat caps for a Standard-plan account.
func Standard() Limits {
	return Limits{WeeklyCap: StandardWeeklyCap, DailyCap: DefaultDailyCap}
}

// Counters is the rolling-window state read off a linkedin_accounts row.
// Both window fields are window-START markers: the weekly window runs
// [WeeklyWindowStart, +7d) and the daily window runs [DailyWindowStart,
// +24h). A zero-value (never-set) window, or one whose span has elapsed,
// means the matching count is stale and the budget for that period is
// full again. The increment query that writes these fields uses the same
// window semantics, so writer and reader agree.
type Counters struct {
	WeeklyCount       int
	WeeklyWindowStart time.Time
	DailyCount        int
	DailyWindowStart  time.Time

	// WarmupStartedAt is when the account began warming up (its first
	// invite). The daily allowance ramps from WarmupWeek1DailyCap to the
	// full DailyCap over the four weeks after this instant. The zero value
	// means "no warmup anchor" → the full DailyCap applies (no throttle) —
	// the pre-warmup behaviour the window-only cases rely on. The worker
	// stamps this on an account's first invite, so a real account always
	// ramps; only the degenerate never-sent input reads as unthrottled.
	WarmupStartedAt time.Time

	// InvitesSent7d and Accepted7d are the account's trailing-7-day invite
	// outcomes, counted from linkedin_events (invite_sent vs accepted) by
	// the worker. They feed the acceptance-rate breaker and are independent
	// of the weekly/daily window counts above, which pace raw volume.
	InvitesSent7d int
	Accepted7d    int
}

// AcceptanceRate is the account's trailing-7-day connection-acceptance rate
// (accepted ÷ invites sent) and whether the window held enough invites to
// trust it. Below BreakerMinSample the second return is false and the rate
// must be ignored — too small a sample to judge.
func (c Counters) AcceptanceRate() (float64, bool) {
	if c.InvitesSent7d < BreakerMinSample {
		return 0, false
	}
	return float64(c.Accepted7d) / float64(c.InvitesSent7d), true
}

// BreakerTripped reports whether the acceptance circuit breaker should pause
// this account: a trustworthy trailing-7-day acceptance rate that has fallen
// below BreakerThreshold. It abstains (false) until the window holds at
// least BreakerMinSample invites, so a young, lagging account is never paused.
func (c Counters) BreakerTripped() bool {
	rate, ok := c.AcceptanceRate()
	return ok && rate < BreakerThreshold
}

// Allowance returns how many invites the account may send right now: the
// smaller of the remaining weekly and daily budgets, never negative. An
// expired or unset window contributes its full cap (the stale count is
// ignored), which is exactly what happens the first send after a window
// rolls over.
func (l Limits) Allowance(c Counters, now time.Time) int {
	// Acceptance circuit breaker: a struggling account sends nothing until
	// its rate recovers, no matter how much window/warmup headroom it has.
	if c.BreakerTripped() {
		return 0
	}
	dailyCap := l.warmupDailyCap(c.WarmupStartedAt, now)
	weeklyRemaining := remaining(l.WeeklyCap, c.WeeklyCount, c.WeeklyWindowStart, weeklyWindow, now)
	dailyRemaining := remaining(dailyCap, c.DailyCount, c.DailyWindowStart, dailyWindow, now)
	return max(0, min(weeklyRemaining, dailyRemaining))
}

// warmupDailyCap is the daily invite ceiling under the warmup ramp for an
// account that started warming at start, as of now. A zero start means no
// warmup anchor → the full DailyCap (no throttle). Otherwise the cap climbs
// from WarmupWeek1DailyCap by warmupStepPerWeek per elapsed week, never above
// the configured DailyCap: ~8/day in week 1 reaching the full ~20/day by
// week 4. It only ever tightens the daily cap, so the weekly ceiling in
// Allowance still binds independently.
func (l Limits) warmupDailyCap(start, now time.Time) int {
	if start.IsZero() {
		return l.DailyCap
	}
	// max(0, …) guards clock skew (a warmup anchored in the future): never
	// ramp above week 1. min(…, DailyCap) keeps the ramp from exceeding the
	// configured ceiling once it tops out.
	weeks := max(0, int(now.Sub(start)/weeklyWindow))
	return min(WarmupWeek1DailyCap+weeks*warmupStepPerWeek, l.DailyCap)
}

// EffectiveWeeklyCap is the weekly invite ceiling actually reachable as
// of now, accounting for the warmup ramp. LinkedIn's flat ceiling is
// WeeklyCap, but during warmup the daily sub-cap throttles a week to at
// most 7×(warmup daily cap), so the effective weekly cap is the smaller
// of the two: ~56 in week 1, ~84 in week 2, the full WeeklyCap from week
// 3 on. With no warmup anchor the full WeeklyCap applies. This is the
// ceiling the capacity indicator reports against (issue #9); Allowance
// keeps treating the weekly and daily caps as independent constraints.
func (l Limits) EffectiveWeeklyCap(warmupStartedAt, now time.Time) int {
	return min(l.WeeklyCap, 7*l.warmupDailyCap(warmupStartedAt, now))
}

// WeeklyRemaining is how many invites the account may still fit in this
// week's budget: the effective weekly cap minus the invites already used
// in the current weekly window, never negative. An unset or elapsed
// weekly window means the stored count is stale and the full effective
// cap is available again — the same window semantics Allowance uses.
// Unlike Allowance it ignores the daily sub-cap and the acceptance
// breaker: it answers "how much weekly headroom remains" for the
// dashboard's capacity indicator (issue #9), not "how many may go out
// right now," which is the worker's per-send gate.
func (l Limits) WeeklyRemaining(c Counters, now time.Time) int {
	return max(0, remaining(l.EffectiveWeeklyCap(c.WarmupStartedAt, now), c.WeeklyCount, c.WeeklyWindowStart, weeklyWindow, now))
}

// remaining is the budget left in one window. If the window is unset or
// has elapsed, the count is stale and the whole cap is available.
func remaining(cap, count int, windowStart time.Time, window time.Duration, now time.Time) int {
	if windowStart.IsZero() || now.Sub(windowStart) >= window {
		return cap
	}
	return cap - count
}
