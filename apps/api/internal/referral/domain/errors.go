package domain

import "errors"

var ErrContactNotFound = errors.New("referral: contact not found")

var ErrNoGithubUsername = errors.New("referral: contact has no github username")
