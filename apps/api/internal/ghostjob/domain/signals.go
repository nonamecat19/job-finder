package domain

import (
	"strings"
	"unicode"
)

type GhostSignals struct {
	RepostCount       int
	DaysOpen          *int
	CrossBoardCount   *int
	AlwaysHiringCount *int
	Notes             map[string]string
}

func (g GhostSignals) AllOptionalSignalsUnknown() bool {
	return g.RepostCount <= 1 && g.DaysOpen == nil && g.CrossBoardCount == nil && g.AlwaysHiringCount == nil
}

func (g GhostSignals) HasUnknownSignal() bool {
	return g.DaysOpen == nil || g.CrossBoardCount == nil || g.AlwaysHiringCount == nil
}

func IsUsableCompanyName(company string) bool {
	if company == "" {
		return false
	}
	if strings.EqualFold(company, "unknown") {
		return false
	}
	for _, r := range company {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}
