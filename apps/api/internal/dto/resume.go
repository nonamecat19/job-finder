package dto

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

type SocialNetwork struct {
	Network  string `json:"network"`
	Username string `json:"username"`
}

type CustomConnection struct {
	Placeholder string `json:"placeholder"`
	URL         string `json:"url"`
	Icon        string `json:"icon,omitempty"`
}

type Entry struct {
	Institution *string `json:"institution,omitempty"`
	Area        *string `json:"area,omitempty"`
	Degree      *string `json:"degree,omitempty"`

	Company  *string `json:"company,omitempty"`
	Position *string `json:"position,omitempty"`

	Name *string `json:"name,omitempty"`

	Title   *string  `json:"title,omitempty"`
	Authors []string `json:"authors,omitempty"`
	Doi     *string  `json:"doi,omitempty"`
	Journal *string  `json:"journal,omitempty"`

	URL *string `json:"url,omitempty"`

	Label   *string `json:"label,omitempty"`
	Details *string `json:"details,omitempty"`

	SkillLevel *string `json:"skillLevel,omitempty"`

	ProjectLevel *string `json:"projectLevel,omitempty"`

	Bullet *string `json:"bullet,omitempty"`

	Number *string `json:"number,omitempty"`

	ReversedNumber *string `json:"reversedNumber,omitempty"`

	Text *string `json:"text,omitempty"`

	Date *string `json:"date,omitempty"`

	StartDate *string `json:"startDate,omitempty"`
	EndDate   *string `json:"endDate,omitempty"`
	Location  *string `json:"location,omitempty"`

	Summary *string `json:"summary,omitempty"`

	Highlights []string `json:"highlights,omitempty"`

	Unrecognized map[string]any `json:"unrecognized,omitempty"`
}

type Section struct {
	Name      string    `json:"name"`
	EntryType EntryType `json:"entryType"`
	Entries   []Entry   `json:"entries"`
}

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

	Unrecognized map[string]any `json:"unrecognized,omitempty"`
}

type ResumeDto struct {
	Resume Resume `json:"resume"`
}
