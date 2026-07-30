package domain

import "fmt"

// SourceNotFoundError is returned when a key does not correspond to a
// registered adapter. Source identity is code-defined, so an unknown key is a
// "not found" regardless of what rows exist in the database.
type SourceNotFoundError struct{ Key string }

func (e SourceNotFoundError) Error() string {
	return fmt.Sprintf("source '%s' not found", e.Key)
}

// AdapterNotRegisteredError is returned by Registry.Get for a key with no
// registered adapter.
type AdapterNotRegisteredError struct{ Key string }

func (e AdapterNotRegisteredError) Error() string {
	return fmt.Sprintf("no job source adapter registered for key '%s'", e.Key)
}
