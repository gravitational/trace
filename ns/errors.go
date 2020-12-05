/*
Copyright 2020 Gravitational, Inc.

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

// Package ns implements No Stacktrace(ns) versions of
// the utility functions from the trace package.
// To be used when information hiding for security purposes is needed.
package ns

import (
	"crypto/x509"
	"fmt"
	"net"
	"os"

	"github.com/gravitational/trace"
)

// AccessDenied returns new instance of AccessDeniedError without stacktrace.
func AccessDenied(message string, args ...interface{}) trace.Error {
	return &trace.TraceErr{
		Err: &trace.AccessDeniedError{
			Message: fmt.Sprintf(message, args...),
		},
	}
}

// BadParameter returns a new instance of BadParameterError without stacktrace.
func BadParameter(message string, args ...interface{}) trace.Error {
	return &trace.TraceErr{
		Err: &trace.BadParameterError{
			Message: fmt.Sprintf(message, args...),
		},
	}
}

// NotFound returns new instance of not found error without stacktrace.
func NotFound(message string, args ...interface{}) trace.Error {
	return &trace.TraceErr{
		Err: &trace.NotFoundError{
			Message: fmt.Sprintf(message, args...),
		},
	}
}

// Wrap takes the original error and wraps it into the Trace struct.
// memorizing the context of the error.
// Does not return the stack traces.
func Wrap(err error, args ...interface{}) trace.Error {
	if err == nil {
		return nil
	}

	var t trace.Error
	if traceErr, ok := err.(trace.Error); ok {
		t = traceErr
	} else {
		t = &trace.TraceErr{
			Err: err,
		}
	}

	if len(args) > 0 {
		format := args[0]
		args = args[1:]

		t.AddUserMessage(format, args...)
	}

	return t
}

// ConvertSystemError converts system error to appropriate trace error.
// if it is possible, otherwise, returns original error.
// Does not return the stack traces.
func ConvertSystemError(err error) error {
	innerError := trace.Unwrap(err)

	if os.IsExist(innerError) {
		return &trace.AlreadyExistsError{
			Message: innerError.Error(),
		}
	}
	if os.IsNotExist(innerError) {
		return &trace.NotFoundError{
			Message: innerError.Error(),
		}
	}
	if os.IsPermission(innerError) {
		return &trace.AccessDeniedError{
			Message: innerError.Error(),
		}
	}

	switch realErr := innerError.(type) {
	case *net.OpError:
		return &trace.ConnectionProblemError{
			Err: realErr,
		}
	case *os.PathError:
		message := fmt.Sprintf("failed to execute command %v error:  %v", realErr.Path, realErr.Err)
		return &trace.AccessDeniedError{
			Message: message,
		}
	case x509.SystemRootsError, x509.UnknownAuthorityError:
		return &trace.TrustError{Err: innerError}
	}

	if _, ok := innerError.(net.Error); ok {
		return &trace.ConnectionProblemError{
			Err: innerError,
		}
	}

	return err
}

// Errorf is similar to trace.Errorf but without the stack trace.
func Errorf(format string, args ...interface{}) (err error) {
	return fmt.Errorf(format, args...)
}
