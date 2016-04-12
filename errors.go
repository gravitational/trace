/*
Copyright 2015 Gravitational, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package trace

import (
	"crypto/x509"
	"fmt"
	"net"
	"net/url"
	"os"
)

// NotFound returns new instance of not found error
func NotFound(message string, args ...interface{}) error {
	return wrap(&NotFoundError{
		Message: fmt.Sprintf(message, args...),
	}, 2)
}

// NotFoundError indicates that object has not been found
type NotFoundError struct {
	Message string `json:"message"`
}

// IsNotFoundError returns true to indicate that is NotFoundError
func (e *NotFoundError) IsNotFoundError() bool {
	return true
}

// Error returns log friendly description of an error
func (e *NotFoundError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "object not found"
}

// OrigError returns original error (in this case this is the error itself)
func (e *NotFoundError) OrigError() error {
	return e
}

// IsNotFound returns whether this error indicates that the object is not found
func IsNotFound(e error) bool {
	type nf interface {
		IsNotFoundError() bool
	}
	_, ok := Unwrap(e).(nf)
	return ok
}

// AlreadyExists returns new AlreadyExists error
func AlreadyExists(message string, args ...interface{}) error {
	return wrap(&AlreadyExistsError{
		Message: fmt.Sprintf(message, args...),
	}, 2)
}

// AlreadyExistsError indicates that there's a duplicate object that already
// exists in the storage/system
type AlreadyExistsError struct {
	// Message is user-friendly error message
	Message string `json:"message"`
}

// Error returns log-friendly error description
func (n *AlreadyExistsError) Error() string {
	if n.Message != "" {
		return n.Message
	}
	return "object already exists"
}

// IsAlreadyExistsError indicates that this error is of AlreadyExists kind
func (AlreadyExistsError) IsAlreadyExistsError() bool {
	return true
}

// OrigError returns original error (in this case this is the error itself)
func (e *AlreadyExistsError) OrigError() error {
	return e
}

// IsAlreadyExists returns if this is error indicating that object
// already exists
func IsAlreadyExists(e error) bool {
	type ae interface {
		IsAlreadyExistsError() bool
	}
	_, ok := Unwrap(e).(ae)
	return ok
}

// BadParameter returns a new instance of BadParameterError
func BadParameter(message string, args ...interface{}) error {
	return wrap(&BadParameterError{
		Message: fmt.Sprintf(message, args...),
	}, 2)
}

// BadParameterError indicates that something is wrong with passed
// parameter to API method
type BadParameterError struct {
	Message string `json:"message"`
}

// Error returrns debug friendly message
func (b *BadParameterError) Error() string {
	return b.Message
}

// OrigError returns original error (in this case this is the error itself)
func (b *BadParameterError) OrigError() error {
	return b
}

// IsBadParameterError indicates that error is of bad parameter type
func (b *BadParameterError) IsBadParameterError() bool {
	return true
}

// IsBadParameter detects if this error is of BadParameter kind
func IsBadParameter(e error) bool {
	type bp interface {
		IsBadParameterError() bool
	}
	_, ok := Unwrap(e).(bp)
	return ok
}

// CompareFailed returns new instance of CompareFailed error
func CompareFailed(message string, args ...interface{}) error {
	return wrap(&CompareFailedError{Message: fmt.Sprintf(message, args...)}, 2)
}

// CompareFailedError indicates that compare failed (e.g wrong password or hash)
type CompareFailedError struct {
	// Message is user-friendly error message
	Message string `json:"message"`
}

// Error is debug - friendly message
func (e *CompareFailedError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "compare failed"
}

// OrigError returns original error (in this case this is the error itself)
func (e *CompareFailedError) OrigError() error {
	return e
}

// IsCompareFailedError indicates that this is compare failed error
func (e *CompareFailedError) IsCompareFailedError() bool {
	return true
}

// IsCompareFailed detects if this error is of CompareFailed kind
func IsCompareFailed(e error) bool {
	type cf interface {
		IsCompareFailedError() bool
	}
	_, ok := Unwrap(e).(cf)
	return ok
}

// AccessDenied returns new access denied error
func AccessDenied(message string, args ...interface{}) error {
	return wrap(&AccessDeniedError{
		Message: fmt.Sprintf(message, args...),
	}, 2)
}

// AccessDeniedError indicates denied access
type AccessDeniedError struct {
	Message string `json:"message"`
}

// Error is debug - friendly error message
func (e *AccessDeniedError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "access denied"
}

// IsAccessDeniedError indicates that this error is of AccessDenied type
func (e *AccessDeniedError) IsAccessDeniedError() bool {
	return true
}

// OrigError returns original error (in this case this is the error itself)
func (e *AccessDeniedError) OrigError() error {
	return e
}

// IsAccessDenied detects if this error is of AccessDeniedError
func IsAccessDenied(e error) bool {
	type ad interface {
		IsAccessDeniedError() bool
	}
	_, ok := Unwrap(e).(ad)
	return ok
}

// ConvertSystemError converts system error to appropriate teleport error
// if it is possible, otherwise, returns original error
func ConvertSystemError(err error) error {
	innerError := Unwrap(err)

	if os.IsNotExist(innerError) {
		return wrap(&NotFoundError{Message: innerError.Error()}, 2)
	}
	if os.IsPermission(innerError) {
		return wrap(&AccessDeniedError{Message: innerError.Error()}, 2)
	}
	switch realErr := innerError.(type) {
	case *net.OpError:
		return wrap(&ConnectionProblemError{
			Message: fmt.Sprintf("failed to connect to server %v", realErr.Addr),
			Err:     realErr}, 2)
	case *os.PathError:
		return wrap(&AccessDeniedError{
			Message: fmt.Sprintf("failed to execute command %v error:  %v", realErr.Path, realErr.Err),
		}, 2)
	case x509.SystemRootsError, x509.UnknownAuthorityError:
		return wrap(&TrustError{Err: innerError}, 2)
	default:
		return err
	}
}

// ConnectionProblem returns ConnectionProblem
func ConnectionProblem(err error, message string, args ...interface{}) error {
	return wrap(&ConnectionProblemError{
		Message: fmt.Sprintf(message, args...),
		Err:     err,
	}, 2)
}

// ConnectionProblemError indicates any network error that has occured
type ConnectionProblemError struct {
	Message string `json:"message"`
	Err     error  `json:"-"`
}

// Error is debug - friendly error message
func (c *ConnectionProblemError) Error() string {
	return fmt.Sprintf("%v: %v", c.Message, c.Err)
}

// IsConnectionProblemError indicates that this error is of ConnectionProblem
func (c *ConnectionProblemError) IsConnectionProblemError() bool {
	return true
}

// OrigError returns original error (in this case this is the error itself)
func (c *ConnectionProblemError) OrigError() error {
	return c
}

// IsConnectionProblem detects if this error is of ConnectionProblemError
func IsConnectionProblem(e error) bool {
	type ad interface {
		IsConnectionProblemError() bool
	}
	_, ok := Unwrap(e).(ad)
	return ok
}

// LimitExceeded returns new limit exceeded error
func LimitExceeded(message string, args ...interface{}) error {
	return wrap(&LimitExceededError{
		Message: fmt.Sprintf(message, args...),
	}, 2)
}

// LimitExceededError indicates rate limit or connection limit problem
type LimitExceededError struct {
	Message string `json:"message"`
}

// Error is debug - friendly error message
func (c *LimitExceededError) Error() string {
	return c.Message
}

// IsLimitExceededError indicates that this error is of ConnectionProblem
func (c *LimitExceededError) IsLimitExceededError() bool {
	return true
}

// OrigError returns original error (in this case this is the error itself)
func (c *LimitExceededError) OrigError() error {
	return c
}

// IsLimitExceeded detects if this error is of LimitExceededError
func IsLimitExceeded(e error) bool {
	type ad interface {
		IsLimitExceededError() bool
	}
	_, ok := Unwrap(e).(ad)
	return ok
}

// TrustError means that we can not trust remote party
type TrustError struct {
	// Err is original error
	Err     error `json:"error"`
	Message string
}

// Error returns log-friendly error description
func (t *TrustError) Error() string {
	return t.Err.Error()
}

// IsTrustError indicates that this error is of trust error kind
func (*TrustError) IsTrustError() bool {
	return true
}

// OrigError returns original error (in this case this is the error itself)
func (t *TrustError) OrigError() error {
	return t
}

// IsTrustError returns if this is a trust error
func IsTrustError(e error) bool {
	type te interface {
		IsTrustError() bool
	}
	_, ok := Unwrap(e).(te)
	return ok
}

// OAuth2 returns OAuth2 related error
func OAuth2(code, message string, query url.Values) Error {
	return wrap(&OAuth2Error{
		Code:    code,
		Message: message,
		Query:   query,
	}, 2)
}

// OAuth2Error is error returned during OAuth2 authentication
// currently used in OIDC (OpenID Connect flow)
type OAuth2Error struct {
	Code    string     `json:"code"`
	Message string     `json:"message"`
	Query   url.Values `json:"query"`
}

// Error returns debug friendly error message
func (o *OAuth2Error) Error() string {
	return fmt.Sprintf("OAuth2 error code=%v, message=%v", o.Code, o.Message)
}

// IsOAuth2Error indicates that this error of OAuth2 type
func (o *OAuth2Error) IsOAuth2Error() bool {
	return true
}

// IsOAuth2 returns if this is a OAuth2-related error
func IsOAuth2(e error) bool {
	type oe interface {
		IsOAuth2Error() bool
	}
	_, ok := Unwrap(e).(oe)
	return ok
}
