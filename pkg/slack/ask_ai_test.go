package slack

import (
	"strings"
	"testing"

	"github.com/slack-go/slack"
)

func TestSplitForBlocksDoesNotSeverLinks(t *testing.T) {
	link := "<https://example.com/very/long/path|the label>"
	msg := strings.Repeat("x", 2890) + " " + link + " and a tail"
	for _, part := range splitForBlocks(msg, 2900) {
		if strings.Count(part, "<") != strings.Count(part, ">") {
			t.Errorf("link severed across a chunk boundary: %q", part)
		}
	}
}

func TestFollowupButtonElementsCapAndValidate(t *testing.T) {
	buttons := []AIButton{
		{Label: "A", Message: "do a"},
		{Label: "", Message: "skip: no label"},
		{Label: "no msg", Message: ""},
		{Label: "B", Message: "do b", Style: "primary"},
		{Label: "C", Message: "do c"},
		{Label: "D", Message: "do d"},
		{Label: "E", Message: "do e"},
		{Label: "F", Message: "over the cap of 5"},
	}
	els := followupButtonElements(buttons)
	if len(els) != 5 {
		t.Fatalf("expected 5 valid buttons (cap), got %d", len(els))
	}
}

func TestRunCommandElementsCapPrefixAndStyle(t *testing.T) {
	commands := []AICommand{
		{Command: "launch 4.19 gcp"},
		{Command: "", Label: "skip: empty command"},
		{Command: "test e2e 4.19 gcp", Label: "Run e2e"},
		{Command: "workflow-launch openshift-e2e-gcp 4.19"},
		{Command: "over the cap of 3"},
	}
	els := runCommandElements(commands)
	if len(els) != 3 {
		t.Fatalf("expected 3 run buttons (cap, empty skipped), got %d", len(els))
	}
	seen := map[string]bool{}
	for i, el := range els {
		btn, ok := el.(*slack.ButtonBlockElement)
		if !ok {
			t.Fatalf("element %d is not a ButtonBlockElement", i)
		}
		if !strings.HasPrefix(btn.ActionID, "ai_run_command_") {
			t.Errorf("element %d action_id = %q, want ai_run_command_ prefix", i, btn.ActionID)
		}
		if seen[btn.ActionID] {
			t.Errorf("duplicate action_id %q", btn.ActionID)
		}
		seen[btn.ActionID] = true
		if btn.Style != slack.StylePrimary {
			t.Errorf("element %d style = %q, want primary", i, btn.Style)
		}
		if !strings.HasPrefix(btn.Text.Text, "▶ ") {
			t.Errorf("element %d label = %q, want ▶ prefix", i, btn.Text.Text)
		}
	}
	// First button: default label is the command; value is the exact command.
	first := els[0].(*slack.ButtonBlockElement)
	if first.Value != "launch 4.19 gcp" {
		t.Errorf("first value = %q, want the command", first.Value)
	}
	if first.Text.Text != "▶ launch 4.19 gcp" {
		t.Errorf("first label = %q, want ▶ + command", first.Text.Text)
	}
	// Provided label is used (with the ▶ prefix) for the second valid command.
	if els[1].(*slack.ButtonBlockElement).Text.Text != "▶ Run e2e" {
		t.Errorf("second label = %q, want ▶ Run e2e", els[1].(*slack.ButtonBlockElement).Text.Text)
	}
}
