package domain

// Entry pairs one resume bullet with its source label (position, company,
// and employment dates). It is the grounding unit the 009 fit-gap coach
// matches adjacent evidence against: every claim it shows the user must
// trace back to one of these, verbatim.
type Entry struct {
	// SourceLabel is the human-readable entry header, e.g.
	// "Senior Backend Engineer, CloudScale (2022–2024)".
	SourceLabel string
	// Bullet is the verbatim resume highlight text.
	Bullet string
}
