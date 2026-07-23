package domain

import "errors"

// ErrContactNotFound is returned when a caller passes a contactID that does
// not match any resolved contact for the job.
var ErrContactNotFound = errors.New("outreach: contact not found for job")

// ErrContactRequired is returned when more than one contact is resolved for
// the job and the caller did not choose one (FR-008): the draft must
// address exactly one recipient, never silently merge or guess among them.
var ErrContactRequired = errors.New("outreach: multiple contacts resolved — choose one")
