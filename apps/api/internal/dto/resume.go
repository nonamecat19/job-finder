package dto

// EntryType is one of the 9 canonical RenderCV entry types. A Section holds
// entries of exactly one EntryType (see spec 009 data-model.md).
type EntryType string

const (
	EntryEducation        EntryType = "education"
	EntryExperience       EntryType = "experience"
	EntryNormal           EntryType = "normal"
	EntryPublication      EntryType = "publication"
	EntryOneLine          EntryType = "one_line"
	EntryBullet           EntryType = "bullet"
	EntryNumbered         EntryType = "numbered"
	EntryReversedNumbered EntryType = "reversed_numbered"
	EntryText             EntryType = "text"
)

var EntryTypes = []EntryType{
	EntryEducation, EntryExperience, EntryNormal, EntryPublication,
	EntryOneLine, EntryBullet, EntryNumbered, EntryReversedNumbered, EntryText,
}

func IsValidEntryType(s string) bool {
	for _, v := range EntryTypes {
		if string(v) == s {
			return true
		}
	}
	return false
}

// SocialNetwork mirrors RenderCV's cv.social_networks entries.
type SocialNetwork struct {
	Network  string `json:"network"`
	Username string `json:"username"`
}

// CustomConnection mirrors RenderCV's cv.custom_connections entries: a
// display label (placeholder), a link, and a fontawesome icon name — used
// for networks not in RenderCV's built-in social_networks list (e.g.
// Telegram in some configs).
type CustomConnection struct {
	Placeholder string `json:"placeholder"`
	URL         string `json:"url"`
	Icon        string `json:"icon,omitempty"`
}

// Entry is a tagged-union-by-convention struct: which fields are meaningful
// depends on the parent Section's EntryType. Represented as one flat struct
// (rather than a Go interface) so it tygo-generates into a single, simple TS
// type the dashboard can pattern-match on `entryType`.
type Entry struct {
	// education
	Institution *string `json:"institution,omitempty"`
	Area        *string `json:"area,omitempty"`
	Degree      *string `json:"degree,omitempty"`

	// experience
	Company  *string `json:"company,omitempty"`
	Position *string `json:"position,omitempty"`

	// normal (projects)
	Name *string `json:"name,omitempty"`

	// publication
	Title   *string  `json:"title,omitempty"`
	Authors []string `json:"authors,omitempty"`
	Doi     *string  `json:"doi,omitempty"`
	Journal *string  `json:"journal,omitempty"`

	// publication, normal
	URL *string `json:"url,omitempty"`

	// one_line (skills)
	Label   *string `json:"label,omitempty"`
	Details *string `json:"details,omitempty"`

	// bullet
	Bullet *string `json:"bullet,omitempty"`

	// numbered
	Number *string `json:"number,omitempty"`

	// reversed_numbered — field is named "reversed_number" in RenderCV YAML;
	// its value is a talk title/description, not a number (RenderCV naming
	// quirk, preserved as-is for round-trip fidelity).
	ReversedNumber *string `json:"reversedNumber,omitempty"`

	// text
	Text *string `json:"text,omitempty"`

	// education, experience, normal, publication
	Date *string `json:"date,omitempty"`

	// education, experience, normal
	StartDate *string `json:"startDate,omitempty"`
	EndDate   *string `json:"endDate,omitempty"`
	Location  *string `json:"location,omitempty"`

	// education, experience, normal, publication
	Summary *string `json:"summary,omitempty"`

	// education, experience, normal
	Highlights []string `json:"highlights,omitempty"`

	// Unrecognized carries any fields present in imported data that don't map
	// to a field above, so nothing is silently dropped (FR-009).
	Unrecognized map[string]any `json:"unrecognized,omitempty"`
}

// Section is a named, ordered group of same-typed Entries (FR-005, FR-006).
type Section struct {
	Name      string    `json:"name"`
	EntryType EntryType `json:"entryType"`
	Entries   []Entry   `json:"entries"`
}

// Resume is the top-level structured, editable view of a Profile's resume
// content (maps to cv.* in the RendercvMaster, minus design/locale/settings,
// which stay out of scope for this feature).
type Resume struct {
	Name              string             `json:"name"`
	Headline          *string            `json:"headline,omitempty"`
	Location          *string            `json:"location,omitempty"`
	Email             *string            `json:"email,omitempty"`
	Phone             *string            `json:"phone,omitempty"`
	Website           *string            `json:"website,omitempty"`
	Photo             *string            `json:"photo,omitempty"`
	SocialNetworks    []SocialNetwork    `json:"socialNetworks,omitempty"`
	CustomConnections []CustomConnection `json:"customConnections,omitempty"`
	Sections          []Section          `json:"sections"`

	// Unrecognized carries any top-level cv.* keys not modeled above.
	Unrecognized map[string]any `json:"unrecognized,omitempty"`
}

// ResumeDto wraps Resume for the GET/PUT /profiles/{id}/resume endpoints.
type ResumeDto struct {
	Resume Resume `json:"resume"`
}
