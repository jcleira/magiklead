package worker

import "testing"

// The LinkedIn rail reuses the email renderer's token table against the
// LinkedIn step shape ({step, delay_days, body} — no subject). A step
// body with {{first_name}}/{{company}}/{{title}} must come out fully
// personalised; an unknown token is left untouched rather than blanked.
func TestRenderLinkedinStep_FillsTokens(t *testing.T) {
	step := LinkedinStep{
		Step:      1,
		DelayDays: 2,
		Body:      "Hi {{first_name}}, saw {{company}} is hiring a {{title}}. {{unknown}}",
	}
	got := RenderLinkedinStep(step, "Ada", "Lovelace", "Analytical Engines", "Head of Eng")
	want := "Hi Ada, saw Analytical Engines is hiring a Head of Eng. {{unknown}}"
	if got != want {
		t.Errorf("RenderLinkedinStep() = %q, want %q", got, want)
	}
}
