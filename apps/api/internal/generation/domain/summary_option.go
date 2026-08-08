package domain

// The summary model catalogue (034).
//
// 034 was specified as a choice of "the model that writes my resume". 035 then
// split generation into five stages served by different models, so there is no
// single model to choose. The choice this catalogue offers is the **summary
// stage only**, deliberately:
//
//   - analyze and select are mechanical. 035 measured the economy model doing
//     them as well as the premium one at a fraction of the price, so offering a
//     dearer option there sells the user a way to spend more for no gain.
//   - the summary is the part the economy model actually failed at, and the
//     part a human reads first.
//
// The catalogue is Go rather than a database table because an option is a
// gateway task key plus prose, and the task key must exist in
// gateway/config.yaml — a deployment artifact reviewed in code. A row in a
// table could name a key the gateway has never heard of, and the mismatch would
// surface only as a silent fallback to the local model. Only the user's
// *choice* is persisted; the menu is code, and a test asserts every key here is
// a declared, chained model group.

// SummaryOption is one entry on the menu the user picks from.
type SummaryOption struct {
	// ID is the stable identifier persisted in the setting and sent by the
	// dashboard. Never reuse one for a different option.
	ID string
	// Label is what the user reads.
	Label string
	// Description says what this option is good at, in one line.
	Description string
	// Cost is a relative indicator, not a price. A figure typed here would be
	// wrong within a month and cannot be reproduced; 038's comparison artifact
	// is where real per-run cost lives.
	Cost string
	// TaskKey is the gateway model group this option routes to. Empty means the
	// self-hosted model, reached by giving the router no gateway at all.
	TaskKey string
	// Default marks the option a user who never opens the selector gets.
	// Exactly one option has it.
	Default bool
}

// SelfHosted reports whether this option runs on the local model.
func (o SummaryOption) SelfHosted() bool { return o.TaskKey == "" }

// SummaryOptionStandard is the default, and its task key is the one the
// pipeline already used before this feature existed. That is what makes "a user
// who never touches the selector sees today's behaviour" true by construction
// rather than by a test that has to be kept honest: it is the same key, sent to
// the same chain. Giving the default a new key would have made that property
// depend on two chains being kept in sync forever.
const SummaryOptionStandard = "standard"

var summaryOptions = []SummaryOption{
	{
		ID:          SummaryOptionStandard,
		Label:       "Standard",
		Description: "The balanced default. Writes a summary that reads well and stays grounded in your profile.",
		Cost:        "moderate",
		TaskKey:     "generation-summary",
		Default:     true,
	},
	{
		ID:          "premium",
		Label:       "Premium",
		Description: "The strongest writer available. Best when the summary has to carry a career change or an unusual profile.",
		Cost:        "highest",
		TaskKey:     "generation-summary-premium",
	},
	{
		// Named `fast` before it was measured, and the measurement said
		// otherwise — 038's comparison of 2026-08-08 put it at a 57.8s median
		// against Standard's 19.1s, because the provider queues requests rather
		// than because the model is slow. Its quality was identical to
		// Standard's on all six scorers and it cost a seventh as much. The id
		// is kept because ids are persisted and reusing one reroutes a user;
		// the label and description say what is actually true.
		ID:          "fast",
		Label:       "Economy",
		Description: "A seventh of the cost, and measured no worse. Often slower to come back, so pick it when you are not waiting on the result.",
		Cost:        "lowest",
		TaskKey:     "generation-summary-fast",
	},
	{
		ID:          "local",
		Label:       "Self-hosted",
		Description: "Runs entirely on your own machine. No provider sees your profile, and it costs nothing to run.",
		Cost:        "free",
		TaskKey:     "",
	},
}

// SummaryOptions returns the catalogue in menu order.
func SummaryOptions() []SummaryOption {
	out := make([]SummaryOption, len(summaryOptions))
	copy(out, summaryOptions)
	return out
}

// DefaultSummaryOption returns the option a user who has never chosen gets.
func DefaultSummaryOption() SummaryOption {
	for _, o := range summaryOptions {
		if o.Default {
			return o
		}
	}
	// Unreachable while TestSummaryCatalogueHasExactlyOneDefault passes, and a
	// panic here would take down a resume run over a menu, so the first entry
	// is the safe answer.
	return summaryOptions[0]
}

// LookupSummaryOption resolves an id to its option. An unknown id — a stale
// dashboard, a hand-edited setting, an option removed between releases —
// returns the default and false, so a caller can choose between reporting the
// mismatch and quietly carrying on. A resume run should carry on: an option is
// a routing preference, and a preference that can fail a run is a liability.
func LookupSummaryOption(id string) (SummaryOption, bool) {
	for _, o := range summaryOptions {
		if o.ID == id {
			return o, true
		}
	}
	return DefaultSummaryOption(), false
}
