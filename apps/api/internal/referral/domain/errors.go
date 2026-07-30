package domain

import "errors"

// ErrContactNotFound is returned when a referenced contact does not exist.
var ErrContactNotFound = errors.New("referral: contact not found")

// ErrNoGithubUsername is returned when GitHub sync is requested for a
// contact that has no githubUsername on file.
var ErrNoGithubUsername = errors.New("referral: contact has no github username")
