package worker

// NoteCharLimit is LinkedIn's hard cap on a connection-request note
// (step 0 of a LinkedIn sequence). The campaign handler rejects a
// step-0 body longer than this before persisting.
const NoteCharLimit = 300

// LinkedinStep is one step of a LinkedIn outreach sequence. Unlike the
// email SequenceStep there is no Subject: step 0's Body is the
// connection-request note (capped at NoteCharLimit), steps 1+ are
// direct messages. The personalization tokens are the same as email.
type LinkedinStep struct {
	Step      int    `json:"step"`
	DelayDays int    `json:"delay_days"`
	Body      string `json:"body"`
}

// RenderLinkedinStep fills the personalization tokens in a LinkedIn
// step's body, reusing the same token table as the email renderer
// (personalize) so the two rails never drift.
func RenderLinkedinStep(step LinkedinStep, firstName, lastName, company, title string) string {
	return renderTokens(step.Body, firstName, lastName, company, title)
}
