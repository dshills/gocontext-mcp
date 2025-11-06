package sample

// This file has intentional syntax errors for testing error handling

func BrokenFunction( {
	// Missing closing parenthesis in parameters
	return "test"
}

type IncompleteStruct struct {
	Field1 string
	// Missing closing brace

func (i *IncompleteStruct) Method() {
	// This should cause parse errors
