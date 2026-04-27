package jobsources

// Ptr returns a pointer to v — shorthand used across adapters when building
// dto.NormalizedJob's optional string fields.
func Ptr[T any](v T) *T { return &v }

// NilIfEmpty returns nil for an empty string, else a pointer to it —
// mirrors the TS `foo || undefined` pattern.
func NilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// StringOr returns v as a string if it's a non-empty string, else fallback —
// mirrors `(config.x as string) || process.env.X`.
func StringOr(v any, fallback string) string {
	if s, ok := v.(string); ok && s != "" {
		return s
	}
	return fallback
}
