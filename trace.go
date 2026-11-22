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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/gravitational/trace/internal"
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
		trace = newTrace(err)
	}
	if len(args) > 0 {
		trace = WithUserMessage(trace, args[0], args[1:]...)
	}
	return trace
}

// Unwrap returns the original error the given error wraps. It is equivalent to
// removing all trace layers until the original error is exposed.
//
// Unwrap works mainly on trace errors, so it won't unwrap errors wrapped via
// Go's `%w`.
//
// Prefer using higher-level comparison methods, such as the various "IsError"
// functions in this package or [errors.Is] and [errors.As]. If you want a
// general error-unwrapping mechanism, consider using [errors.Unwrap] instead.
func Unwrap(err error) error {
	if err, ok := err.(ErrorWrapper); ok {
		return err.OrigError()
	}
	return err
}

// UserMessager returns a user message associated with the error
type UserMessager interface {
	// UserMessage returns the user message associated with the error if any
	UserMessage() string
}

// ErrorWrapper wraps another error
type ErrorWrapper interface {
	// OrigError returns the wrapped error
	OrigError() error
}

// DebugReporter formats an error for display
type DebugReporter interface {
	// DebugReport formats an error for display
	DebugReport() string
}

// UserMessage returns user-friendly part of the error
func UserMessage(err error) string {
	if err == nil {
		return ""
	}
	if wrap, ok := err.(UserMessager); ok {
		return wrap.UserMessage()
	}
	return err.Error()
}

// UserMessageWithFields returns user-friendly error with key-pairs as part of the message
func UserMessageWithFields(err error) string {
	if err == nil {
		return ""
	}
	if wrap, ok := err.(Error); ok {
		if len(wrap.GetFields()) == 0 {
			return wrap.UserMessage()
		}

		var sb strings.Builder
		for k, v := range wrap.GetFields() {
			fmt.Fprintf(&sb, "%v=%q ", k, v)
		}
		sb.WriteString(wrap.UserMessage())
		return sb.String()
	}
	return err.Error()
}

// DebugReport returns debug report with all known information
// about the error including stack trace if it was captured
func DebugReport(err error) string {
	if err == nil {
		return ""
	}
	if reporter, ok := err.(DebugReporter); ok {
		return reporter.DebugReport()
	}
	return err.Error()
}

// GetFields returns any fields that have been added to the error message
func GetFields(err error) map[string]interface{} {
	if err == nil {
		return map[string]interface{}{}
	}

	// for proxyError combine top-level and nested fields.
	if proxy, ok := err.(proxyError); ok {
		fields := map[string]interface{}{}

		// nested
		for field, value := range GetFields(proxy.Err) {
			fields[field] = value
		}

		// top-level
		for field, value := range proxy.GetFields() {
			fields[field] = value
		}

		return fields
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
		trace = newTrace(err)
	}
	return WithUserMessage(trace, message, args...)
}

// Errorf is similar to fmt.Errorf except that it captures
// more information about the origin of error, such as
// callee, line number and function that simplifies debugging
func Errorf(format string, args ...interface{}) (err error) {
	err = fmt.Errorf(format, args...)
	return newTrace(err)
}

// Fatalf - If debug is false Fatalf calls Errorf. If debug is
// true Fatalf calls panic
func Fatalf(format string, args ...interface{}) error {
	if IsDebug() {
		panic(fmt.Sprintf(format, args...))
	} else {
		return Errorf(format, args...)
	}
}

func newTrace(err error) *TraceErr {
	// newTrace does not call newTraceWithDepth so the depth value is consistent
	// between both methods.
	const depth = 2
	return &TraceErr{Err: err, stack: internal.CaptureStack(depth)}
}

func newTraceWithDepth(err error, depth int) *TraceErr {
	return &TraceErr{Err: err, stack: internal.CaptureStack(depth)}
}

type Traces = internal.Traces

type Trace = internal.Trace

// NewTraceErr returns an error with the provided stack traces. Most callers
// should use Wrap, which captures the current stack. NewTraceErr is intended
// for errors whose traces were captured elsewhere, such as decoded errors.
func NewTraceErr(err error, traces Traces) *TraceErr {
	return &TraceErr{Err: err, traces: traces}
}

// MarshalJSON marshals this error as JSON-encoded payload
func (e *TraceErr) MarshalJSON() ([]byte, error) {
	if e == nil {
		return nil, nil
	}
	type marshalableError TraceErr
	err := marshalableError(*e)
	err.Err = &RawTrace{Message: e.Err.Error()}
	return json.Marshal(err)
}

// TraceErr contains error message and some additional
// information about the error origin
type TraceErr struct {
	// Err is the underlying error that TraceErr wraps
	Err error `json:"error"`
	// traces holds stack trace entries provided to NewTraceErr.
	traces Traces
	// stack holds the stack captured when the error was created by this
	// package. It is resolved into traces on first use by GetTraces.
	stack *internal.Stack
	// Message is an optional message that can be wrapped with the original error.
	//
	// This field is obsolete, replaced by messages list below.
	Message string `json:"message,omitempty"`
	// Messages is a list of user messages added to this error.
	Messages []string `json:"messages,omitempty"`
	// Fields is a list of key-value-pairs that can be wrapped with the error to give additional context
	Fields map[string]interface{} `json:"fields,omitempty"`
}

// Fields maps arbitrary keys to values inside an error
type Fields map[string]interface{}

// Error returns the error message this trace describes.
// Implements error
func (r *RawTrace) Error() string {
	return r.Message
}

// RawTrace describes the error trace on the wire
type RawTrace struct {
	// Err specifies the original error
	Err json.RawMessage `json:"error,omitempty"`
	// Traces lists the stack traces at the moment the error was recorded
	Traces `json:"traces,omitempty"`
	// Message specifies the optional user-facing message
	Message string `json:"message,omitempty"`
	// Messages is a list of user messages added to this error.
	Messages []string `json:"messages,omitempty"`
	// Fields is a list of key-value-pairs that can be wrapped with the error to give additional context
	Fields map[string]interface{} `json:"fields,omitempty"`
}

func (e *TraceErr) Clone() *TraceErr {
	if e == nil {
		return nil
	}

	tr := &TraceErr{
		Err:     e.Err,
		traces:  e.traces,
		stack:   e.stack,
		Message: e.Message,
	}

	if e.Messages != nil {
		tr.Messages = append([]string{}, e.Messages...)
	}

	if e.Fields != nil {
		tr.Fields = make(map[string]interface{}, len(e.Fields))
		for k, v := range e.Fields {
			tr.Fields[k] = v
		}
	}

	return tr
}

// UserMessage returns user-friendly error message
func (e *TraceErr) UserMessage() string {
	if len(e.Messages) > 0 {
		// Format all collected messages in the reverse order, with each error
		// on its own line with appropriate indentation so they form a tree and
		// it's easy to see the cause and effect.
		cause := UserMessage(e.Err)
		// Each message is followed by a newline and an increasing number of tabs.
		size := len(cause) + len(e.Messages) + len(e.Messages)*(len(e.Messages)+1)/2
		for _, message := range e.Messages {
			size += len(message)
		}

		var sb strings.Builder
		sb.Grow(size)
		for i := len(e.Messages) - 1; i >= 0; i-- {
			sb.WriteString(e.Messages[i])
			sb.WriteByte('\n')
			for range len(e.Messages) - i {
				sb.WriteByte('\t')
			}
		}
		sb.WriteString(cause)
		return sb.String()
	}
	if e.Message != "" {
		// For backwards compatibility return the old user message if it's present.
		return e.Message
	}
	return UserMessage(e.Err)
}

// writeEscapedUserMessage writes the HTML-escaped equivalent of e.UserMessage
// to buf without building an intermediate string.
func writeEscapedUserMessage(buf *bytes.Buffer, e *TraceErr) {
	if len(e.Messages) == 0 && e.Message != "" {
		writeEscaped(buf, e.Message)
		return
	}
	for i := len(e.Messages) - 1; i >= 0; i-- {
		writeEscaped(buf, e.Messages[i])
		buf.WriteByte('\n')
		for range len(e.Messages) - i {
			buf.WriteByte('\t')
		}
	}
	if cause, ok := e.Err.(*TraceErr); ok {
		writeEscapedUserMessage(buf, cause)
	} else {
		writeEscaped(buf, UserMessage(e.Err))
	}
}

// DebugReport returns developer-friendly error report
func (e *TraceErr) DebugReport() string {
	return debugReport(e, nil)
}

// Error returns user-friendly error message when not in debug mode
func (e *TraceErr) Error() string {
	return e.UserMessage()
}

func (e *TraceErr) GetFields() map[string]interface{} {
	return e.Fields
}

// GetTraces returns the stack trace entries for the error.
func (e *TraceErr) GetTraces() Traces {
	if e.stack != nil {
		return e.stack.Traces()
	}
	return e.traces
}

// Func returns the function name where the error was created.
func (e *TraceErr) Func() string {
	return e.GetTraces().Func()
}

// FuncName returns the unqualified function name where the error was created.
func (e *TraceErr) FuncName() string {
	return e.GetTraces().FuncName()
}

// Loc returns the file and line where the error was created.
func (e *TraceErr) Loc() string {
	return e.GetTraces().Loc()
}

// String returns a debug-friendly representation of the stack trace.
func (e *TraceErr) String() string {
	return e.GetTraces().String()
}

// Unwrap returns the error this TraceErr wraps. The returned error may also
// wrap another one, Unwrap doesn't recursively get the inner-most error like
// OrigError does.
func (e *TraceErr) Unwrap() error {
	return e.Err
}

// OrigError returns original wrapped error
func (e *TraceErr) OrigError() error {
	err := e.Err
	// this is not an endless loop because I'm being
	// paranoid, this is a safe protection against endless
	// loops
	for i := 0; i < maxHops; i++ {
		newerr, ok := err.(Error)
		if !ok {
			break
		}
		next := newerr.OrigError()
		if next == nil || next == err {
			break
		}
		err = next
	}
	return err
}

// GoString formats this trace object for use with
// the "%#v" format string
func (e *TraceErr) GoString() string {
	return e.DebugReport()
}

// maxHops is a max supported nested depth for errors
const maxHops = 50

// Error is an interface that helps to adapt usage of trace in the code
// When applications define new error types, they can implement the interface
//
// Error handlers can use Unwrap() to retrieve error from the wrapper, or
// errors.Is()/As() to compare it to another value.
type Error interface {
	error
	ErrorWrapper
	DebugReporter
	UserMessager

	// GetFields returns any fields that have been added to the error
	GetFields() map[string]interface{}

	// Clone returns a copy of the current Error.
	Clone() *TraceErr
}

// WithUserMessage adds formatted user-facing message
// to the error, depends on the implementation,
// usually works as fmt.Sprintf(formatArg, rest...)
// but implementations can choose another way, e.g. treat
// arguments as structured args.
func WithUserMessage(err Error, formatArg interface{}, rest ...interface{}) *TraceErr {
	errCopy := err.Clone()

	format, ok := formatArg.(string)
	if !ok {
		format = fmt.Sprint(formatArg)
	}
	errCopy.Messages = append(errCopy.Messages, formatString(format, rest))
	return errCopy
}

// formatString avoids invoking the formatter for the common case of a literal
// message while retaining fmt.Sprintf's behavior for strings containing verbs.
func formatString(format string, args []any) string {
	if len(args) == 0 && !strings.ContainsRune(format, '%') {
		return format
	}
	return fmt.Sprintf(format, args...)
}

// WithField adds additional field information to the error.
func WithField(err Error, key string, value interface{}) *TraceErr {
	errCopy := err.Clone()

	if errCopy.Fields == nil {
		errCopy.Fields = make(map[string]interface{}, 1)
	}
	errCopy.Fields[key] = value
	return errCopy
}

// WithFields adds a map of additional fields to the error
func WithFields(err Error, fields map[string]interface{}) *TraceErr {
	errCopy := err.Clone()

	if errCopy.Fields == nil {
		errCopy.Fields = make(map[string]interface{}, len(fields))
	}
	for k, v := range fields {
		errCopy.Fields[k] = v
	}
	return errCopy
}

// NewAggregate creates a new aggregate instance from the specified
// list of errors
func NewAggregate(errs ...error) error {
	nonNils := make([]error, 0, len(errs))
	for _, err := range errs {
		if err != nil {
			nonNils = append(nonNils, err)
		}
	}
	if len(nonNils) == 0 {
		return nil
	}
	return newTrace(aggregate(nonNils))
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
	buf := &strings.Builder{}
	for i, e := range r {
		if i > 0 {
			buf.WriteString(", ")
		}
		buf.WriteString(e.Error())
	}
	return buf.String()
}

// Is implements the `Is` interface, by iterating through each error in the
// aggregate and invoking `errors.Is`.
// Required for Go versions < 1.20 (newer releases support "Unwrap() []error").
func (r aggregate) Is(t error) bool {
	for _, err := range r {
		if errors.Is(err, t) {
			return true
		}
	}
	return false
}

// As implements the `As` interface, by iterating through each error in the
// aggregate and invoking `errors.As`.
// Required for Go versions < 1.20 (newer releases support "Unwrap() []error").
func (r aggregate) As(t interface{}) bool {
	for _, err := range r {
		if errors.As(err, t) {
			return true
		}
	}
	return false
}

// Errors obtains the list of errors this aggregate combines
func (r aggregate) Errors() []error {
	cp := make([]error, len(r))
	copy(cp, r)
	return cp
}

// Unwrap returns the underlying aggregated errors.
func (r aggregate) Unwrap() []error {
	return r.Errors()
}

// IsAggregate returns true if `err` contains an [Aggregate] error in its
// chain.
func IsAggregate(err error) bool {
	var other Aggregate
	return errors.As(err, &other)
}

// wrapProxy wraps the specified error as a new error trace
func wrapProxy(err error) Error {
	if err == nil {
		return nil
	}
	return proxyError{
		// Do not include ReadError in the trace
		TraceErr: newTraceWithDepth(err, 3),
	}
}

// htmlEscaper matches the escaping done in [html/template] for text and quoted
// attributes, which is slightly more than what [html.EscapeString] does.
var htmlEscaper = strings.NewReplacer(
	"\x00", "\uFFFD",
	`"`, "&#34;", // "&#34;" is shorter than "&quot;".
	`&`, "&amp;",
	`'`, "&#39;", // "&#39;" is shorter than "&apos;" and apos was not in HTML until HTML5.
	`+`, "&#43;",
	`<`, "&lt;",
	`>`, "&gt;",
)

// DebugReport formats the underlying error for display
// Implements DebugReporter
func (r proxyError) DebugReport() string {
	wrappedErr, ok := r.TraceErr.Err.(*TraceErr)
	if !ok {
		return DebugReport(r.TraceErr)
	}
	return debugReport(wrappedErr, r.TraceErr.GetTraces())
}

func debugReport(err *TraceErr, caught Traces) string {
	buf := getBuffer()
	defer putBuffer(buf)

	buf.WriteString("\nERROR REPORT:\nOriginal Error: ")
	buf.WriteString(reflect.TypeOf(err.Err).String())
	buf.WriteByte(' ')
	writeEscaped(buf, err.Err.Error())
	buf.WriteByte('\n')
	if len(err.Fields) > 0 {
		buf.WriteString("Fields:\n")
		for k, v := range err.Fields {
			buf.WriteString("  ")
			writeEscaped(buf, k)
			buf.WriteString(": ")
			value, ok := v.(string)
			if !ok {
				value = fmt.Sprint(v)
			}
			writeEscaped(buf, value)
			buf.WriteByte('\n')
		}
	}
	buf.WriteString("Stack Trace:\n")
	writeEscapedTraces(buf, err.GetTraces())
	buf.WriteByte('\n')
	if len(caught) > 0 {
		buf.WriteString("Caught:\n")
		writeEscapedTraces(buf, caught)
		buf.WriteByte('\n')
	}
	buf.WriteString("User Message: ")
	writeEscapedUserMessage(buf, err)
	if len(caught) > 0 {
		buf.WriteByte('\n')
	}

	return buf.String()
}

// writeEscapedTraces writes traces in the same format as Traces.String,
// HTML-escaping paths and function names.
func writeEscapedTraces(buf *bytes.Buffer, traces Traces) {
	for i, t := range traces {
		if i > 0 {
			buf.WriteByte('\n')
		}
		buf.WriteByte('\t')
		writeEscaped(buf, t.Path)
		buf.WriteByte(':')
		buf.Write(strconv.AppendInt(buf.AvailableBuffer(), int64(t.Line), 10))
		buf.WriteByte(' ')
		writeEscaped(buf, t.Func)
	}
}

func writeEscaped(buf *bytes.Buffer, s string) {
	htmlEscaper.WriteString(buf, s)
}

// buffers holds reusable buffers so debug reports need only one allocation for
// the returned string.
var buffers = sync.Pool{New: func() any { return new(bytes.Buffer) }}

// maxPooledBufferSize prevents unusually large reports from keeping large
// buffers alive in the pool.
const maxPooledBufferSize = 64 << 10

func getBuffer() *bytes.Buffer {
	return buffers.Get().(*bytes.Buffer)
}

func putBuffer(buf *bytes.Buffer) {
	if buf.Cap() > maxPooledBufferSize {
		return
	}
	buf.Reset()
	buffers.Put(buf)
}

// GoString formats this trace object for use with
// with the "%#v" format string
func (r proxyError) GoString() string {
	return r.DebugReport()
}

// proxyError wraps another error
type proxyError struct {
	*TraceErr
}
