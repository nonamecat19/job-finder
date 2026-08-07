package domain

import "fmt"

type SourceNotFoundError struct{ Key string }

func (e SourceNotFoundError) Error() string {
	return fmt.Sprintf("source '%s' not found", e.Key)
}

type AdapterNotRegisteredError struct{ Key string }

func (e AdapterNotRegisteredError) Error() string {
	return fmt.Sprintf("no job source adapter registered for key '%s'", e.Key)
}
