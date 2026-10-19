package room

// InvalidError says which input was rejected and how to fix it. It never repeats the input.
type InvalidError struct {
	Field   string
	Problem string
}

func (e *InvalidError) Error() string {
	return e.Field + ": " + e.Problem
}

func invalid(field, problem string) error {
	return &InvalidError{Field: field, Problem: problem}
}
