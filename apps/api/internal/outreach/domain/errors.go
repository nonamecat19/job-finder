package domain

import "errors"

var ErrContactNotFound = errors.New("outreach: contact not found for job")

var ErrContactRequired = errors.New("outreach: multiple contacts resolved — choose one")
