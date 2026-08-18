package domain

import "fmt"

type FailureKind string

const (
	KindInvalidURL FailureKind = "invalid_url"

	KindNotAPosting FailureKind = "not_a_posting"

	KindNoReader FailureKind = "no_reader"

	KindUnreachable FailureKind = "unreachable"

	KindBlocked FailureKind = "blocked"

	KindTimedOut FailureKind = "timed_out"

	KindIncomplete FailureKind = "incomplete"
)

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
