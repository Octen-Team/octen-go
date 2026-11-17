package octen

// Ptr returns a pointer to v, for setting optional request fields.
func Ptr[T any](v T) *T { return &v }

// Int returns a pointer to v.
func Int(v int) *int { return &v }

// String returns a pointer to v.
func String(v string) *string { return &v }

// Bool returns a pointer to v.
func Bool(v bool) *bool { return &v }

// Float64 returns a pointer to v.
func Float64(v float64) *float64 { return &v }
