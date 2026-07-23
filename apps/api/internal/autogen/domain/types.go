// Package autogen holds the "auto-generate resume on high match score"
// setting: a single enabled/threshold pair, configurable from Settings, and
// read by matching/handler.go right after a job's score is computed to
// decide whether to auto-enqueue a resume for it.
package domain

type State struct {
	Enabled   bool
	Threshold int
}
