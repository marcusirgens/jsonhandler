package jsonhandler

import (
	"errors"
	"fmt"
)

func (h HandlerErr) Error() string {
	return fmt.Sprintf("%d: %s", h.code, h.message)
}

func (h HandlerErr) Unwrap() error {
	return h.err
}

// Error constructs a new error with a user defined status code.
func Error(code int, message string) HandlerErr {
	return HandlerErr{
		code:    code,
		message: message,
	}
}

// Errorf creates a new formatted error using the same syntax as fmt.Errorf.
func Errorf(code int, format string, a ...interface{}) HandlerErr {
	err := fmt.Errorf(format, a...)
	return HandlerErr{
		code:    code,
		message: err.Error(),
		err:     errors.Unwrap(err),
	}
}


