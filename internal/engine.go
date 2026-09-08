package internal

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Muxcore-Media/contracts-ai/infer"
)

const (
	SceneSexual          = "sexual"
	SceneGraphicViolence = "graphic_violence"
	SceneDrugUse         = "drug_use"
	SceneProfanity       = "profanity"
	SceneJumpScare       = "jump_scare"
	SceneSelfHarm        = "self_harm"
)

// Profile is an admin-configured filter.
type Profile struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	TargetRating    string   `json:"target_rating,omitempty"`
	Words           []string `json:"words"`
	SceneCategories []string `json:"scene_categories"`
	BleepToken      string   `json:"bleep_token"`
	Enabled         bool     `json:"enabled"`
}

// SceneTag is an admin- or classifier-supplied timed span.
type SceneTag struct {
	StartMs  int64  `json:"start_ms"`
	EndMs    int64  `json:"end_ms"`
	Category string `json:"category"`
	Note     string `json:"note,omitempty"`
}

// Cut is a span to drop from playback/transcode.
type Cut struct {
	StartMs  int64  `json:"start_ms"`
	EndMs    int64  `json:"end_ms"`
	Category string `json:"category"`
}

// Bleep is a word replacement inside a cue.
type Bleep struct {
	StartMs int64  `json:"start_ms"`
	Word    string `json:"word"`
}

// Plan is the computed filter for one media item + profile.
type Plan struct {
	PlanID      string                `json:"plan_id"`
	MediaID     string                `json:"media_id"`
	ProfileID   string                `json:"profile_id"`
	Bleeps      []Bleep               `json:"bleeps"`
	Cuts        []Cut                 `json:"cuts"`
	FilteredSRT string                `json:"filtered_srt"`
	EDL         []string              `json:"edl"`
	Cues        []infer.TranscriptCue `json:"cues"`
}

func builtinProfiles() []Profile {
	return []Profile{
		{
			ID: "r-to-pg13", Name: "R to PG-13", TargetRating: "PG-13",
			Words:           []string{"fuck", "shit", "cunt", "cock", "motherfucker"},
			SceneCategories: []string{SceneSexual, SceneGraphicViolence, SceneSelfHarm},
			BleepToken:      "bleep", Enabled: true,
		},
		{
			ID: "pg13-to-pg", Name: "PG-13 to PG", TargetRating: "PG",
			Words:           []string{"fuck", "shit", "asshole", "bitch", "damn", "hell"},
			SceneCategories: []string{SceneSexual, SceneGraphicViolence, SceneDrugUse, SceneSelfHarm},
			BleepToken:      "bleep", Enabled: true,
		},
	}
}

func applyFilter(profile Profile, mediaID string, cues []infer.TranscriptCue, scenes []SceneTag) Plan {
	token := profile.BleepToken
	if token == "" {
		token = "bleep"
	}
	wordSet := compileWords(profile.Words)
	var bleeps []Bleep
	filtered := make([]infer.TranscriptCue, 0, len(cues))
	for _, c := range cues {
		text, hits := bleepText(c.Text, wordSet, token)
		for _, w := range hits {
			bleeps = append(bleeps, Bleep{StartMs: c.StartMs, Word: w})
		}
		keep := true
		for _, sc := range scenes {
			if overlaps(c.StartMs, c.EndMs, sc.StartMs, sc.EndMs) && categoryEnabled(profile, sc.Category) {
				keep = false
				break
			}
		}
		if keep {
			filtered = append(filtered, infer.TranscriptCue{StartMs: c.StartMs, EndMs: c.EndMs, Text: text})
		}
	}
	var cuts []Cut
	for _, sc := range scenes {
		if categoryEnabled(profile, sc.Category) {
			cuts = append(cuts, Cut{StartMs: sc.StartMs, EndMs: sc.EndMs, Category: sc.Category})
		}
	}
	edl := make([]string, 0, len(cuts))
	for i, c := range cuts {
		edl = append(edl, fmt.Sprintf("%d  %s  %s", i+1, formatEDL(c.StartMs), formatEDL(c.EndMs)))
	}
	return Plan{
		MediaID: mediaID, ProfileID: profile.ID,
		Bleeps: bleeps, Cuts: cuts,
		FilteredSRT: formatSimpleSRT(filtered),
		EDL:         edl, Cues: filtered,
	}
}

func compileWords(words []string) []*regexp.Regexp {
	out := make([]*regexp.Regexp, 0, len(words))
	for _, w := range words {
		w = strings.TrimSpace(w)
		if w == "" {
			continue
		}
		out = append(out, regexp.MustCompile(`(?i)\b`+regexp.QuoteMeta(w)+`\b`))
	}
	return out
}

func bleepText(text string, words []*regexp.Regexp, token string) (string, []string) {
	var hits []string
	out := text
	for _, re := range words {
		if re.MatchString(out) {
			hits = append(hits, re.FindString(out))
			out = re.ReplaceAllString(out, token)
		}
	}
	return out, hits
}

func categoryEnabled(p Profile, cat string) bool {
	for _, c := range p.SceneCategories {
		if strings.EqualFold(c, cat) {
			return true
		}
	}
	return false
}

func overlaps(a0, a1, b0, b1 int64) bool {
	return a0 < b1 && b0 < a1
}

func formatEDL(ms int64) string {
	h := ms / 3_600_000
	ms %= 3_600_000
	m := ms / 60_000
	ms %= 60_000
	s := ms / 1000
	f := (ms % 1000) * 24 / 1000
	return fmt.Sprintf("%02d:%02d:%02d:%02d", h, m, s, f)
}

func formatSimpleSRT(cues []infer.TranscriptCue) string {
	var b strings.Builder
	for i, c := range cues {
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "%d\n%s --> %s\n%s\n", i+1, clock(c.StartMs), clock(c.EndMs), c.Text)
	}
	return b.String()
}

func clock(ms int64) string {
	h := ms / 3_600_000
	ms %= 3_600_000
	m := ms / 60_000
	ms %= 60_000
	s := ms / 1000
	frac := ms % 1000
	return fmt.Sprintf("%02d:%02d:%02d,%03d", h, m, s, frac)
}

func inferScenesFromCues(cues []infer.TranscriptCue) []SceneTag {
	var out []SceneTag
	for _, c := range cues {
		lower := strings.ToLower(c.Text)
		switch {
		case strings.Contains(lower, "sex") || strings.Contains(lower, "nude") || strings.Contains(lower, "naked"):
			out = append(out, SceneTag{StartMs: c.StartMs, EndMs: c.EndMs, Category: SceneSexual})
		case strings.Contains(lower, "blood") || strings.Contains(lower, "decapitat") || strings.Contains(lower, "gore"):
			out = append(out, SceneTag{StartMs: c.StartMs, EndMs: c.EndMs, Category: SceneGraphicViolence})
		case strings.Contains(lower, "heroin") || strings.Contains(lower, "cocaine") || strings.Contains(lower, "inject"):
			out = append(out, SceneTag{StartMs: c.StartMs, EndMs: c.EndMs, Category: SceneDrugUse})
		}
	}
	return out
}
