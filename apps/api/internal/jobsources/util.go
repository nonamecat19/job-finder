package jobsources

func Ptr[T any](v T) *T { return &v }

func NilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func StringOr(v any, fallback string) string {
	if s, ok := v.(string); ok && s != "" {
		return s
	}
	return fallback
}
