/*
   Copyright 2021 Gravitational, Inc.

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

package internal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"
)

// MarshalJSON marshals this error as JSON-encoded payload
func (e *TraceErr) MarshalJSON() ([]byte, error) {
	if e == nil {
		return nil, nil
	}
	type marshalableError TraceErr
	err := marshalableError(*e)
	err.Err = &RawTrace{Message: e.Err.Error()}
	err.Traces = e.Decode()
	return json.Marshal(err)
}

// AddUserMessage adds user-friendly message describing the error nature
func (e *TraceErr) AddUserMessage(formatArg interface{}, rest ...interface{}) *TraceErr {
	newMessage := fmt.Sprintf(fmt.Sprintf("%v", formatArg), rest...)
	e.Messages = append(e.Messages, newMessage)
	return e
}

// AddFields adds the given map of fields to the error being reported
func (e *TraceErr) AddFields(fields map[string]interface{}) *TraceErr {
	if e.Fields == nil {
		e.Fields = make(map[string]interface{}, len(fields))
	}
	for k, v := range fields {
		e.Fields[k] = v
	}
	return e
}

// AddField adds a single field to the error wrapper as context for the error
func (e *TraceErr) AddField(k string, v interface{}) *TraceErr {
	if e.Fields == nil {
		e.Fields = make(map[string]interface{}, 1)
	}
	e.Fields[k] = v
	return e
}

// UserMessage returns user-friendly error message
func (e *TraceErr) UserMessage() string {
	if len(e.Messages) > 0 {
		// Format all collected messages in the reverse order, with each error
		// on its own line with appropriate indentation so they form a tree and
		// it's easy to see the cause and effect.
		var buf bytes.Buffer
		fmt.Fprintln(&buf, e.Messages[len(e.Messages)-1])
		index, indent := len(e.Messages)-1, 1
		for ; index > 0; index, indent = index-1, indent+1 {
			fmt.Fprintf(&buf, "%v%v\n", strings.Repeat("\t", indent), e.Messages[index-1])
		}
		fmt.Fprintf(&buf, "%v%v", strings.Repeat("\t", indent), UserMessage(e.Err))
		return buf.String()
	}
	if e.Message != "" {
		// For backwards compatibility return the old user message if it's present.
		return e.Message
	}
	return UserMessage(e.Err)
}

// DebugReport returns developer-friendly error report
func (e *TraceErr) DebugReport() string {
	var buf bytes.Buffer
	err := reportTemplate.Execute(&buf, errorReport{
		OrigErrType:    fmt.Sprintf("%T", e.Err),
		OrigErrMessage: e.Err.Error(),
		Fields:         e.Fields,
		StackTrace:     e.Decode().String(),
		UserMessage:    e.UserMessage(),
	})
	if err != nil {
		return fmt.Sprint("error generating debug report: ", err.Error())
	}
	return buf.String()
}

// Error returns user-friendly error message when not in debug mode
func (e *TraceErr) Error() string {
	return e.UserMessage()
}

func (e *TraceErr) GetFields() map[string]interface{} {
	return e.Fields
}

// Unwrap returns the error this TraceErr wraps. The returned error may also
// wrap another one, Unwrap doesn't recursively get the inner-most error like
// OrigError does.
func (e *TraceErr) Unwrap() error {
	return e.Err
}

// OrigError returns original wrapped error
func (e *TraceErr) OrigError() error {
	return e.Err
}

// GoString formats this trace object for use with
// with the "%#v" format string
func (e *TraceErr) GoString() string {
	return e.DebugReport()
}

// Decode decodes symbolic debugging information from this error value
func (e *TraceErr) Decode() Traces {
	if len(e.Traces) != 0 {
		return e.Traces
	}
	return e.traces.decode()
}

// TraceErr contains error message and some additional
// information about the error origin
type TraceErr struct {
	// Err is the underlying error that TraceErr wraps
	Err error `json:"error"`
	// Traces optionally specified the decoded stack trace entries for the error.
	Traces Traces `json:"traces,omitempty"`
	// Message is an optional message that can be wrapped with the original error.
	//
	// This field is obsolete, replaced by messages list below.
	Message string `json:"message,omitempty"`
	// Messages is a list of user messages added to this error.
	Messages []string `json:"messages,omitempty"`
	// Fields is a list of key-value-pairs that can be wrapped with the error to give additional context
	Fields map[string]interface{} `json:"fields,omitempty"`

	traces traces `json:"-"`
}

// Trace stores structured trace entry: source file line, file path and function name
type Trace struct {
	// Path is the absolute file path
	Path string `json:"path"`
	// Func is the function name
	Func string `json:"func"`
	// Line is the code line number
	Line int `json:"line"`
}

// Error returns the error message this trace describes.
// Implements error
func (r *RawTrace) Error() string {
	return r.Message
}

// Wrap translates this error value to an instance of TraceErr
// wrapping the specified error
func (r *RawTrace) Wrap(err error) *TraceErr {
	return &TraceErr{
		Traces:   r.Traces,
		Err:      err,
		Message:  r.Message,
		Messages: r.Messages,
		Fields:   r.Fields,
	}
}

// RawTrace describes the error trace on the wire
type RawTrace struct {
	// Err specifies the original error
	Err json.RawMessage `json:"error,omitempty"`
	// Traces lists the stack traces at the moment the error was recorded
	Traces Traces `json:"traces,omitempty"`
	// Message specifies the optional user-facing message
	Message string `json:"message,omitempty"`
	// Messages is a list of user messages added to this error.
	Messages []string `json:"messages,omitempty"`
	// Fields is a list of key-value-pairs that can be wrapped with the error to give additional context
	Fields map[string]interface{} `json:"fields,omitempty"`
}

// FrameCursor stores the position in a call stack
type FrameCursor struct {
	// Current specifies the current stack frame.
	// if omitted, rest contains the complete stack
	Current *runtime.Frame
	// Rest specifies the rest of stack frames to explore
	Rest *runtime.Frames
	// N specifies the total number of stack frames
	N int
}

// Traces is a list of trace entries
type Traces []Trace

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

// UserMessager returns a user message associated with the error
type UserMessager interface {
	// UserMessage returns the user message associated with the error if any
	UserMessage() string
}

// Wrap returns a new instance of trace error at specified stack depth
// for the given error
func Wrap(err error, depth int) *TraceErr {
	if err, ok := err.(*TraceErr); ok {
		return err
	}
	return &TraceErr{Err: err, traces: captureTraces(depth)}
}

// DecodeTracesFromCursor decodes the stack trace from the given cursor
func DecodeTracesFromCursor(cursor FrameCursor) Traces {
	traces := make(Traces, 0, cursor.N)
	if cursor.Current != nil {
		traces = append(traces, frameToTrace(*cursor.Current))
	}
	for i := 0; i < cursor.N; i++ {
		frame, more := cursor.Rest.Next()
		traces = append(traces, frameToTrace(frame))
		if !more {
			break
		}
	}
	return traces
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

// captureTraces captures the stack trace with skip frames omitted
func captureTraces(skip int) traces {
	var buf [32]uintptr
	// +2 means that we also skip `captureTraces` and `runtime.Callers` frames.
	n := runtime.Callers(skip+2, buf[:])
	return buf[:n]
}

// decode decodes symbolic debugging information from this traces value
func (r traces) decode() Traces {
	frames := runtime.CallersFrames(r)
	cursor := FrameCursor{
		Rest: frames,
		N:    len(r),
	}
	return DecodeTracesFromCursor(cursor)
}

// traces describes the call stack of an error
type traces []uintptr

func frameToTrace(frame runtime.Frame) Trace {
	return Trace{
		Func: frame.Function,
		Path: frame.File,
		Line: frame.Line,
	}
}

// SetTraces adds new traces to the list
func (s Traces) SetTraces(traces ...Trace) {
	s = append(s, traces...)
}

// Func returns first function in trace list
func (s Traces) Func() string {
	if len(s) == 0 {
		return ""
	}
	return s[0].Func
}

// Func returns just function name
func (s Traces) FuncName() string {
	if len(s) == 0 {
		return ""
	}
	fn := filepath.ToSlash(s[0].Func)
	idx := strings.LastIndex(fn, "/")
	if idx == -1 || idx == len(fn)-1 {
		return fn
	}
	return fn[idx+1:]
}

// Loc points to file/line location in the code
func (s Traces) Loc() string {
	if len(s) == 0 {
		return ""
	}
	return s[0].String()
}

// String returns debug-friendly representaton of trace stack
func (s Traces) String() string {
	if len(s) == 0 {
		return ""
	}
	out := make([]string, len(s))
	for i, t := range s {
		out[i] = fmt.Sprintf("\t%v:%v %v", t.Path, t.Line, t.Func)
	}
	return strings.Join(out, "\n")
}

// String returns debug-friendly representation of this trace
func (t *Trace) String() string {
	dir, file := filepath.Split(t.Path)
	dirs := strings.Split(filepath.ToSlash(filepath.Clean(dir)), "/")
	if len(dirs) != 0 {
		file = filepath.Join(dirs[len(dirs)-1], file)
	}
	return fmt.Sprintf("%v:%v", file, t.Line)
}

// WrapProxy wraps the specified error as a new error trace
func WrapProxy(err error) *ProxyError {
	if err == nil {
		return nil
	}
	if err, ok := err.(*TraceErr); ok {
		return &ProxyError{
			TraceErr: err,
		}
	}
	return &ProxyError{
		// Do not include ReadError in the trace
		TraceErr: Wrap(err, 3),
	}
}

// DebugReport formats the underlying error for display
// Implements DebugReporter
func (r *ProxyError) DebugReport() string {
	var wrappedErr *TraceErr
	var ok bool
	if wrappedErr, ok = r.TraceErr.Err.(*TraceErr); !ok {
		return DebugReport(r.TraceErr)
	}
	var buf bytes.Buffer
	//nolint:errcheck
	reportTemplate.Execute(&buf, errorReport{
		OrigErrType:    fmt.Sprintf("%T", wrappedErr.Err),
		OrigErrMessage: wrappedErr.Err.Error(),
		Fields:         wrappedErr.Fields,
		StackTrace:     wrappedErr.Decode().String(),
		UserMessage:    wrappedErr.UserMessage(),
		Caught:         r.TraceErr.Decode().String(),
	})
	return buf.String()
}

// GoString formats this trace object for use with
// with the "%#v" format string
func (r *ProxyError) GoString() string {
	return r.DebugReport()
}

func (r *ProxyError) OrigError() error {
	return r.TraceErr.Err
}

// ProxyError wraps another error
type ProxyError struct {
	*TraceErr
}

type errorReport struct {
	// OrigErrType specifies the error type as text
	OrigErrType string
	// OrigErrMessage specifies the original error's message
	OrigErrMessage string
	// Fields lists any additional fields attached to the error
	Fields map[string]interface{}
	// StackTrace specifies the call stack
	StackTrace string
	// UserMessage is the user-facing message (if any)
	UserMessage string
	// Caught optionally specifies the stack trace where the error
	// has been recorded after coming over the wire
	Caught string
}

var reportTemplate = template.Must(template.New("debugReport").Parse(reportTemplateText))
var reportTemplateText = `
ERROR REPORT:
Original Error: {{.OrigErrType}} {{.OrigErrMessage}}
{{if .Fields}}Fields:
{{range $key, $value := .Fields}}  {{$key}}: {{$value}}
{{end}}{{end}}Stack Trace:
{{.StackTrace}}
{{if .Caught}}Caught:
{{.Caught}}
User Message: {{.UserMessage}}
{{else}}User Message: {{.UserMessage}}{{end}}`
