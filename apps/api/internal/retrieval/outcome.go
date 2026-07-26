package retrieval

type PageStatus string

const (
	PageRead        PageStatus = "read"
	PageChallenged  PageStatus = "challenged"
	PageRefused     PageStatus = "refused"
	PageUnparseable PageStatus = "unparseable"
	PageDeferred    PageStatus = "deferred"
)

type PageOutcome struct {
	Status PageStatus `json:"status"`
	Method string     `json:"method"`
	Reason string     `json:"reason,omitempty"`
	URL    string     `json:"url"`
}

type RunVerdict string

const (
	VerdictSuccess RunVerdict = "success"
	VerdictPartial RunVerdict = "partial"
	VerdictBlocked RunVerdict = "blocked"
)
