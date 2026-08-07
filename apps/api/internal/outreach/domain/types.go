package domain

import "time"

type Tone string

const (
	ToneWarm   Tone = "warm"
	ToneDirect Tone = "direct"
	ToneFormal Tone = "formal"
)

const DefaultTone = ToneWarm

func AllTones() []Tone {
	return []Tone{ToneWarm, ToneDirect, ToneFormal}
}

func isValidTone(t Tone) bool {
	for _, v := range AllTones() {
		if v == t {
			return true
		}
	}
	return false
}

func NormalizeTone(raw string) Tone {
	t := Tone(raw)
	if isValidTone(t) {
		return t
	}
	return DefaultTone
}

type Fact struct {
	Kind  string
	Value string
}

type GroundingTrace struct {
	Claim       string
	SignalKind  string
	SignalValue string
}

type OutreachDraft struct {
	JobID           string
	ContactID       string
	ContactName     string
	Tone            Tone
	Text            string
	GroundingTraces []GroundingTrace
	GeneratedAt     time.Time
}
