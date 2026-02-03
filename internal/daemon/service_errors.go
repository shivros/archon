package daemon

import "fmt"

type ServiceErrorKind string

const (
	ServiceErrorInvalid     ServiceErrorKind = "invalid"
	ServiceErrorNotFound    ServiceErrorKind = "not_found"
	ServiceErrorUnavailable ServiceErrorKind = "unavailable"
	ServiceErrorConflict    ServiceErrorKind = "conflict"
)

type ServiceError struct {
	Kind    ServiceErrorKind
	Message string
	Err     error
}

func (e *ServiceError) Error() string {
	switch {
	case e == nil:
		return ""
	case e.Message != "" && e.Err != nil:
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	case e.Message != "":
		return e.Message
	case e.Err != nil:
		return e.Err.Error()
	default:
		return string(e.Kind)
	}
}

func (e *ServiceError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func invalidError(message string, err error) *ServiceError {
	return &ServiceError{Kind: ServiceErrorInvalid, Message: message, Err: err}
}

func notFoundError(message string, err error) *ServiceError {
	return &ServiceError{Kind: ServiceErrorNotFound, Message: message, Err: err}
}

func unavailableError(message string, err error) *ServiceError {
	return &ServiceError{Kind: ServiceErrorUnavailable, Message: message, Err: err}
}

func conflictError(message string, err error) *ServiceError {
	return &ServiceError{Kind: ServiceErrorConflict, Message: message, Err: err}
}
