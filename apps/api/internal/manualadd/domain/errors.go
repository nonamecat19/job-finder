package domain

import "fmt"

// FailureKind is the FR-018 taxonomy. The client switches on the kind, never
// on the prose, so the set is closed and each member has one meaning.
type FailureKind string

const (
	// KindInvalidURL — unparseable, or not http/https. No network request made.
	KindInvalidURL FailureKind = "invalid_url"
	// KindNotAPosting — a reader claims the host but the URL is a search or listing page.
	KindNotAPosting FailureKind = "not_a_posting"
	// KindNoReader — no adapter reads this host.
	KindNoReader FailureKind = "no_reader"
	// KindUnreachable — DNS failure, connection refused, 404, 410.
	KindUnreachable FailureKind = "unreachable"
	// KindBlocked — bot challenge, login wall, 403/429 after the ladder exhausted its rungs.
	KindBlocked FailureKind = "blocked"
	// KindTimedOut — the 30 s budget elapsed, including any pacing wait.
	KindTimedOut FailureKind = "timed_out"
	// KindIncomplete — the page was read but is missing title, company or description.
	KindIncomplete FailureKind = "incomplete"
)

// Failure carries an operator-facing reason alongside the machine-readable kind.
type Failure struct {
	Kind   FailureKind
	Reason string
	Cause  error
}

func (f *Failure) Error() string { return string(f.Kind) + ": " + f.Reason }

func (f *Failure) Unwrap() error { return f.Cause }

func NewFailure(kind FailureKind, reason string) *Failure {
	return &Failure{Kind: kind, Reason: reason}
}

func WrapFailure(kind FailureKind, cause error, reason string) *Failure {
	return &Failure{Kind: kind, Reason: reason, Cause: cause}
}

func InvalidURL(rawURL string) *Failure {
	return NewFailure(KindInvalidURL, fmt.Sprintf("%q is not a valid http(s) URL.", rawURL))
}

func NotAPosting(host string) *Failure {
	return NewFailure(KindNotAPosting, fmt.Sprintf("%s is a known source, but this URL is a search or listing page, not a single posting. Create a subscription for it instead.", host))
}

func NoReader(host string) *Failure {
	return NewFailure(KindNoReader, fmt.Sprintf("No source can read postings on %s yet.", host))
}

func Unreachable(host string, cause error) *Failure {
	return WrapFailure(KindUnreachable, cause, fmt.Sprintf("%s could not be reached, or the posting no longer exists.", host))
}

func Blocked(host string, cause error) *Failure {
	return WrapFailure(KindBlocked, cause, fmt.Sprintf("%s returned a bot challenge or login wall; the posting could not be read.", host))
}

func TimedOut(host string, cause error) *Failure {
	return WrapFailure(KindTimedOut, cause, fmt.Sprintf("Reading the posting on %s took longer than 30 seconds.", host))
}

func Incomplete(missing []string) *Failure {
	return NewFailure(KindIncomplete, "The page was read but is missing "+joinFields(missing)+".")
}

func joinFields(fields []string) string {
	switch len(fields) {
	case 0:
		return "required fields"
	case 1:
		return fields[0]
	case 2:
		return fields[0] + " and " + fields[1]
	default:
		out := ""
		for i, f := range fields[:len(fields)-1] {
			if i > 0 {
				out += ", "
			}
			out += f
		}
		return out + " and " + fields[len(fields)-1]
	}
}
