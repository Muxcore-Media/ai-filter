package internal

import (
	"strings"
	"testing"

	"github.com/Muxcore-Media/contracts-ai/infer"
)

func TestRToPG13BleepsAndCutsSexScene(t *testing.T) {
	p := builtinProfiles()[0]
	cues := []infer.TranscriptCue{
		{StartMs: 0, EndMs: 2000, Text: "What the fuck was that"},
		{StartMs: 2000, EndMs: 8000, Text: "They have sex on the floor"},
		{StartMs: 8000, EndMs: 10000, Text: "Let's go home"},
	}
	scenes := inferScenesFromCues(cues)
	plan := applyFilter(p, "m1", cues, scenes)
	if len(plan.Bleeps) == 0 || plan.Bleeps[0].Word == "" {
		t.Fatalf("bleeps = %#v", plan.Bleeps)
	}
	if !strings.Contains(plan.FilteredSRT, "bleep") {
		t.Fatalf("srt = %s", plan.FilteredSRT)
	}
	if !strings.Contains(plan.FilteredSRT, "Let's go home") {
		t.Fatalf("kept dialogue missing: %s", plan.FilteredSRT)
	}
	if strings.Contains(plan.FilteredSRT, "sex on the floor") {
		t.Fatalf("sexual cue should be cut: %s", plan.FilteredSRT)
	}
	if len(plan.Cuts) == 0 || plan.Cuts[0].Category != SceneSexual {
		t.Fatalf("cuts = %#v", plan.Cuts)
	}
}

func TestCustomWordList(t *testing.T) {
	p := Profile{ID: "custom", Words: []string{"banana"}, BleepToken: "***", SceneCategories: nil, Enabled: true}
	plan := applyFilter(p, "m1", []infer.TranscriptCue{{StartMs: 0, EndMs: 1000, Text: "A banana split"}}, nil)
	if plan.Cues[0].Text != "A *** split" {
		t.Fatalf("text = %q", plan.Cues[0].Text)
	}
}
