/*
Copyright 2015-2019 Gravitational, Inc.

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

// Package trace implements utility functions for capturing debugging
// information about file and line in error reports and logs.
package trace

import (
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/gravitational/trace/internal"

	"golang.org/x/net/context"
)

var debug int32

// SetDebug turns on/off debugging mode, that causes Fatalf to panic
func SetDebug(enabled bool) {
	if enabled {
		atomic.StoreInt32(&debug, 1)
	} else {
		atomic.StoreInt32(&debug, 0)
	}
}

// IsDebug returns true if debug mode is on
func IsDebug() bool {
	return atomic.LoadInt32(&debug) == 1
}

// Wrap takes the original error and wraps it into the Trace struct
// memorizing the context of the error.
func Wrap(err error, args ...interface{}) Error {
	if err == nil {
		return nil
	}
	var trace Error
	if traceErr, ok := err.(Error); ok {
		trace = traceErr
	} else {
		trace = newTrace(err, 2)
	}
	if len(args) > 0 {
		trace = trace.AddUserMessage(args[0], args[1:]...)
	}
	return trace
}

// Unwrap returns the original error the given error wraps
func Unwrap(err error) error {
	if err, ok := err.(ErrorWrapper); ok {
		return err.OrigError()
	}
	return err
}

var UserMessage = internal.UserMessage
var DebugReport = internal.DebugReport

type UserMessager = internal.UserMessager
type ErrorWrapper = internal.ErrorWrapper
type DebugReporter = internal.DebugReporter

// UserMessageWithFields returns user-friendly error with key-pairs as part of the message
func UserMessageWithFields(err error) string {
	if err == nil {
		return ""
	}
	if wrap, ok := err.(Error); ok {
		if len(wrap.GetFields()) == 0 {
			return wrap.UserMessage()
		}

		var kvps []string
		for k, v := range wrap.GetFields() {
			kvps = append(kvps, fmt.Sprintf("%v=%q", k, v))
		}
		return fmt.Sprintf("%v %v", strings.Join(kvps, " "), wrap.UserMessage())
	}
	return err.Error()
}

// GetFields returns any fields that have been added to the error message
func GetFields(err error) map[string]interface{} {
	if err == nil {
		return map[string]interface{}{}
	}
	if wrap, ok := err.(Error); ok {
		return wrap.GetFields()
	}
	return map[string]interface{}{}
}

// WrapWithMessage wraps the original error into Error and adds user message if any
func WrapWithMessage(err error, message interface{}, args ...interface{}) Error {
	var trace Error
	if traceErr, ok := err.(Error); ok {
		trace = traceErr
	} else {
		trace = newTrace(err, 2)
	}
	trace.AddUserMessage(message, args...)
	return trace
}

// Errorf is similar to fmt.Errorf except that it captures
// more information about the origin of error, such as
// callee, line number and function that simplifies debugging
func Errorf(format string, args ...interface{}) (err error) {
	err = fmt.Errorf(format, args...)
	return newTrace(err, 2)
}

// Fatalf - If debug is false Fatalf calls Errorf. If debug is
// true Fatalf calls panic
func Fatalf(format string, args ...interface{}) error {
	if IsDebug() {
		panic(fmt.Sprintf(format, args...))
	}
	return Errorf(format, args...)
}

func newTrace(err error, depth int) *TraceErr {
	return internal.Wrap(err, depth+1)
}

type Traces = internal.Traces

type TraceErr = internal.TraceErr

type Error = internal.Error

// Fields maps arbitrary keys to values inside an error
type Fields map[string]interface{}

// NewAggregate creates a new aggregate instance from the specified
// list of errors
func NewAggregate(errs ...error) error {
	// filter out possible nil values
	var nonNils []error
	for _, err := range errs {
		if err != nil {
			nonNils = append(nonNils, err)
		}
	}
	if len(nonNils) == 0 {
		return nil
	}
	return newTrace(aggregate(nonNils), 2)
}

// NewAggregateFromChannel creates a new aggregate instance from the provided
// errors channel.
//
// A context.Context can be passed in so the caller has the ability to cancel
// the operation. If this is not desired, simply pass context.Background().
func NewAggregateFromChannel(errCh chan error, ctx context.Context) error {
	var errs []error

Loop:
	for {
		select {
		case err, ok := <-errCh:
			if !ok { // the channel is closed, time to exit
				break Loop
			}
			errs = append(errs, err)
		case <-ctx.Done():
			break Loop
		}
	}

	return NewAggregate(errs...)
}

// Aggregate interface combines several errors into one error
type Aggregate interface {
	error
	// Errors obtains the list of errors this aggregate combines
	Errors() []error
}

// aggregate implements Aggregate
type aggregate []error

// Error implements the error interface
func (r aggregate) Error() string {
	if len(r) == 0 {
		return ""
	}
	output := r[0].Error()
	for i := 1; i < len(r); i++ {
		output = fmt.Sprintf("%v, %v", output, r[i])
	}
	return output
}

// Errors obtains the list of errors this aggregate combines
func (r aggregate) Errors() []error {
	return []error(r)
}

// IsAggregate returns whether this error of Aggregate error type
func IsAggregate(err error) bool {
	_, ok := Unwrap(err).(Aggregate)
	return ok
}
