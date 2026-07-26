package domain

import "time"

type Profile struct {
	ID             string
	Name           string
	RendercvConfig []byte
	RendercvYaml   *string
	ExtraNotes     *string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
