package domain

type SummaryOption struct {
	ID string

	Label string

	Description string

	Cost string

	TaskKey string

	Default bool
}

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

		ID:          "fast",
		Label:       "Economy",
		Description: "A seventh of the cost, and measured no worse. Often slower to come back, so pick it when you are not waiting on the result.",
		Cost:        "lowest",
		TaskKey:     "generation-summary-fast",
	},
}

func SummaryOptions() []SummaryOption {
	out := make([]SummaryOption, len(summaryOptions))
	copy(out, summaryOptions)
	return out
}

func DefaultSummaryOption() SummaryOption {
	for _, o := range summaryOptions {
		if o.Default {
			return o
		}
	}

	return summaryOptions[0]
}

func LookupSummaryOption(id string) (SummaryOption, bool) {
	for _, o := range summaryOptions {
		if o.ID == id {
			return o, true
		}
	}
	return DefaultSummaryOption(), false
}
