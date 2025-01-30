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

package trace

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func forEachReporter(t *testing.T, test func(t *testing.T, reporter func(error) string)) {
	tests := []struct {
		desc     string
		reporter func(error) string
	}{
		{
			desc:     "DebugReport",
			reporter: DebugReport,
		},
		{
			desc:     "DebugReportHTML",
			reporter: DebugReportHTML,
		},
		{
			desc:     "DebugReportCLI",
			reporter: DebugReportCLI,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			test(t, tt.reporter)
		})
	}
}

func TestEmpty(t *testing.T) {
	forEachReporter(t, func(t *testing.T, reporter func(error) string) {
		assert.Equal(t, "", reporter(nil))
	})
	assert.Equal(t, "", UserMessage(nil))
	assert.Equal(t, "", UserMessageWithFields(nil))
	assert.Equal(t, map[string]interface{}{}, GetFields(nil))
}

func TestWrap(t *testing.T) {
	testErr := &testError{Param: "param"}
	err := Wrap(Wrap(testErr))

	forEachReporter(t, func(t *testing.T, reporter func(error) string) {
		assert.Regexp(t, ".*trace_test.go.*", line(reporter(err)))
		assert.NotRegexp(t, ".*trace.go.*", line(reporter(err)))
	})
	assert.Regexp(t, ".*param.*", line(UserMessage(err)))
	assert.NotRegexp(t, ".*trace_test.go.*", line(UserMessage(err)))
}

func TestOrigError(t *testing.T) {
	testErr := fmt.Errorf("some error")
	err := Wrap(Wrap(testErr))

	assert.Equal(t, testErr, err.OrigError())
}

func TestIsEOF(t *testing.T) {
	assert.True(t, IsEOF(io.EOF))
	assert.True(t, IsEOF(Wrap(io.EOF)))
}

func TestWrapUserMessage(t *testing.T) {
	testErr := fmt.Errorf("description")

	err := Wrap(testErr, "user message")
	forEachReporter(t, func(t *testing.T, reporter func(error) string) {
		assert.Regexp(t, ".*trace_test.go.*", line(reporter(err)))
		assert.NotRegexp(t, ".*trace.go.*", line(reporter(err)))
	})
	assert.Equal(t, "user message\tdescription", line(UserMessage(err)))

	err = Wrap(err, "user message 2")
	assert.Equal(t, "user message 2\tuser message\t\tdescription", line(UserMessage(err)))
}

func TestWrapWithMessage(t *testing.T) {
	testErr := fmt.Errorf("description")
	err := WrapWithMessage(testErr, "user message")
	assert.Equal(t, "user message\tdescription", line(UserMessage(err)))
	forEachReporter(t, func(t *testing.T, reporter func(error) string) {
		assert.Regexp(t, ".*trace_test.go.*", line(reporter(err)))
		assert.NotRegexp(t, ".*trace.go.*", line(reporter(err)))
	})
}

func TestUserMessageWithFields(t *testing.T) {
	testErr := fmt.Errorf("description")
	assert.Equal(t, testErr.Error(), UserMessageWithFields(testErr))

	err := Wrap(testErr, "user message")
	assert.Equal(t, "user message\tdescription", line(UserMessageWithFields(err)))

	err = WithField(err, "test_key", "test_value")
	assert.Equal(t, "test_key=\"test_value\" user message\tdescription", line(UserMessageWithFields(err)))
}

func TestGetFields(t *testing.T) {
	testErr := fmt.Errorf("description")
	assert.Equal(t, map[string]interface{}{}, GetFields(testErr))

	fields := map[string]interface{}{
		"test_key": "test_value",
	}
	err := WithFields(Wrap(testErr), fields)
	assert.Equal(t, fields, GetFields(err))

	// ensure that you can get fields from a proxyError
	e := roundtripError(err)
	assert.Equal(t, fields, GetFields(e))
}

func roundtripError(err error) error {
	w := newTestWriter()
	WriteError(w, err)

	outErr := ReadError(w.StatusCode, w.Body)
	return outErr
}

func TestWrapNil(t *testing.T) {
	err1 := Wrap(nil, "message: %v", "extra")
	assert.Nil(t, err1)

	var err2 error
	err2 = nil

	err3 := Wrap(err2)
	assert.Nil(t, err3)

	err4 := Wrap(err3)
	assert.Nil(t, err4)
}

func TestRaceErrorWrap(t *testing.T) {
	baseErr := BadParameter("foo")

	iters := 100_000

	wg := sync.WaitGroup{}
	wg.Add(3)

	// trace.Wrap with format arguments
	go func() {
		for i := 0; i < iters; i++ {
			_ = Wrap(baseErr, "foo bar %q", "baz")
		}
		wg.Done()
	}()

	// trace.WrapWithMessage
	go func() {
		for i := 0; i < iters; i++ {
			_ = WrapWithMessage(baseErr, "foo bar %q", "baz")
		}
		wg.Done()
	}()

	// plain Error() call
	go func() {
		for i := 0; i < iters; i++ {
			_ = baseErr.Error()
		}
		wg.Done()
	}()

	wg.Wait()
}

func TestWrapStdlibErrors(t *testing.T) {
	assert.True(t, IsNotFound(os.ErrNotExist))
}

func TestGenericErrors(t *testing.T) {
	testCases := []struct {
		Err        Error
		Predicate  func(error) bool
		StatusCode int
		comment    string
	}{
		{
			Err:        NotFound("not found"),
			Predicate:  IsNotFound,
			StatusCode: http.StatusNotFound,
			comment:    "not found error",
		},
		{
			Err:        AlreadyExists("already exists"),
			Predicate:  IsAlreadyExists,
			StatusCode: http.StatusConflict,
			comment:    "already exists error",
		},
		{
			Err:        BadParameter("is bad"),
			Predicate:  IsBadParameter,
			StatusCode: http.StatusBadRequest,
			comment:    "bad parameter error",
		},
		{
			Err:        CompareFailed("is bad"),
			Predicate:  IsCompareFailed,
			StatusCode: http.StatusPreconditionFailed,
			comment:    "comparison failed error",
		},
		{
			Err:        AccessDenied("denied"),
			Predicate:  IsAccessDenied,
			StatusCode: http.StatusForbidden,
			comment:    "access denied error",
		},
		{
			Err:        ConnectionProblem(nil, "prob"),
			Predicate:  IsConnectionProblem,
			StatusCode: http.StatusRequestTimeout,
			comment:    "connection error",
		},
		{
			Err:        LimitExceeded("limit exceeded"),
			Predicate:  IsLimitExceeded,
			StatusCode: http.StatusTooManyRequests,
			comment:    "limit exceeded error",
		},
		{
			Err:        NotImplemented("not implemented"),
			Predicate:  IsNotImplemented,
			StatusCode: http.StatusNotImplemented,
			comment:    "not implemented error",
		},
	}

	for _, testCase := range testCases {
		SetDebug(true)
		err := testCase.Err

		var traceErr *TraceErr
		var ok bool
		if traceErr, ok = err.(*TraceErr); !ok {
			t.Fatalf("Expected error to be of type *TraceErr: %#v", err)
		}

		assert.NotEmpty(t, traceErr.Traces, testCase.comment)
		forEachReporter(t, func(t *testing.T, reporter func(error) string) {
			assert.Regexp(t, ".*.trace_test\\.go.*", line(reporter(err)), testCase.comment)
			assert.NotRegexp(t, ".*.errors\\.go.*", line(reporter(err)), testCase.comment)
			assert.NotRegexp(t, ".*.trace\\.go.*", line(reporter(err)), testCase.comment)
		})
		assert.True(t, testCase.Predicate(err), testCase.comment)

		w := newTestWriter()
		WriteError(w, err)

		outErr := ReadError(w.StatusCode, w.Body)
		if _, ok := outErr.(proxyError); !ok {
			t.Fatalf("Expected error to be of type proxyError: %#v", outErr)
		}
		assert.True(t, testCase.Predicate(outErr), testCase.comment)

		SetDebug(false)
		w = newTestWriter()
		WriteError(w, err)
		outErr = ReadError(w.StatusCode, w.Body)
		assert.True(t, testCase.Predicate(outErr), testCase.comment)
	}
}

// Make sure we write some output produced by standard errors
func TestWriteExternalErrors(t *testing.T) {
	err := Wrap(fmt.Errorf("snap!"))

	SetDebug(true)
	w := newTestWriter()
	WriteError(w, err)
	extErr := ReadError(w.StatusCode, w.Body)
	assert.Equal(t, http.StatusInternalServerError, w.StatusCode)
	assert.Regexp(t, ".*.snap.*", strings.Replace(string(w.Body), "\n", "", -1))
	require.NotNil(t, extErr)
	assert.EqualError(t, err, extErr.Error())

	SetDebug(false)
	w = newTestWriter()
	WriteError(w, err)
	extErr = ReadError(w.StatusCode, w.Body)
	assert.Equal(t, http.StatusInternalServerError, w.StatusCode)
	assert.Regexp(t, ".*.snap.*", strings.Replace(string(w.Body), "\n", "", -1))
	require.NotNil(t, extErr)
	assert.EqualError(t, err, extErr.Error())
}

type netError struct{}

func (e *netError) Error() string   { return "net" }
func (e *netError) Timeout() bool   { return true }
func (e *netError) Temporary() bool { return true }

func TestConvert(t *testing.T) {
	err := ConvertSystemError(&netError{})
	assert.True(t, IsConnectionProblem(err), "failed to detect network error")

	dir := t.TempDir()
	err = os.Mkdir(dir, 0o770)
	err = ConvertSystemError(err)
	assert.True(t, IsAlreadyExists(err), "expected AlreadyExists error, got %T", err)
}

func TestAggregates(t *testing.T) {
	err1 := Errorf("failed one")
	err2 := Errorf("failed two")

	err := NewAggregate(err1, err2)
	assert.True(t, IsAggregate(err))

	agg := Unwrap(err).(Aggregate)
	assert.Equal(t, []error{err1, err2}, agg.Errors())
	assert.Equal(t, "failed one, failed two", err.Error())
}

func TestErrorf(t *testing.T) {
	err := Errorf("error")
	forEachReporter(t, func(t *testing.T, reporter func(error) string) {
		assert.Regexp(t, ".*.trace_test.go.*", line(reporter(err)))
		assert.NotRegexp(t, ".*.Fields.*", line(reporter(err)))
	})
	assert.Equal(t, []string(nil), err.(*TraceErr).Messages)
}

func TestWithField(t *testing.T) {
	err := WithField(Wrap(Errorf("error")), "testfield", true)
	forEachReporter(t, func(t *testing.T, reporter func(error) string) {
		assert.Regexp(t, ".*.testfield.*", line(reporter(err)))
	})
}

func TestWithFields(t *testing.T) {
	err := WithFields(Wrap(Errorf("error")), map[string]interface{}{
		"testfield1": true,
		"testfield2": "value2",
	})
	forEachReporter(t, func(t *testing.T, reporter func(error) string) {
		assert.Regexp(t, ".*.Fields.*", line(reporter(err)))
		assert.Regexp(t, ".*.testfield1: true.*", line(reporter(err)))
		assert.Regexp(t, ".*.testfield2: value2.*", line(reporter(err)))
	})
}

// Needed for backwards compat
func TestDebugReportMatchesDebugReportHTML(t *testing.T) {
	rawErr := Errorf("inner error")
	err := Wrap(rawErr, "middle error")
	err = Wrap(err, "outer error")
	err = WithFields(err, map[string]interface{}{
		"key1": "\"<>&'azAZ1,./ string field with special characters",
		"key2": Errorf("non-string in second field"),
	})
	err = WithUserMessage(err, "some multiline user error\n    line 2")

	assert.Equal(t, DebugReport(err), DebugReportHTML(err))
}

// Produce a debug report using a reflection-backed template for historical reasons
func oldDebugReport(traceErr *TraceErr) string {
	reportTemplateText := `
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
	reportTemplate := template.Must(template.New("debugReport").Parse(reportTemplateText))

	var buf bytes.Buffer
	//nolint:errcheck
	reportTemplate.Execute(&buf, traceErr.toErrorReport())

	return buf.String()
}

// Needed for backwards compat
func TestDebugReportMatchesTemplate(t *testing.T) {
	dummyVal := struct {
		key1 string
		key2 bool
		key3 int
	}{
		key1: "val 1",
		key2: true,
		key3: 123456,
	}

	rawErr := Errorf("inner error %q", "quoted value")
	err := Wrap(rawErr, "middle error %#v", dummyVal)
	err = Wrap(err, "outer error")
	err = WithField(err, "key", "\"<>&'azAZ1,./ string field with special characters")
	traceErr := WithUserMessage(err, "some multiline user error\n    line 2")

	debugReportMessage := DebugReport(traceErr)
	oldTemplatedMessage := oldDebugReport(traceErr)
	require.Equal(t, oldTemplatedMessage, debugReportMessage)
}

func TestCLIReportDoesNotEscapeHTML(t *testing.T) {
	rawErr := Errorf("inner error <>&'azAZ1,./")
	err := Wrap(rawErr, "middle error <>&'azAZ1,./")
	err = Wrap(err, "outer error <>&'azAZ1,./")
	err = WithField(err, "key", "\"<>&'azAZ1,./ string field 1 with special characters")
	err = WithUserMessage(err, "some multiline user error\n    line 2 <>&'azAZ1,./")

	message := DebugReportCLI(err)

	// Escaped HTML charts always end in `;`. For this test to be effective,
	// a semicolon character should not appear in the error.
	assert.NotContains(t, message, ';')
}

func TestAggregateConvertsToCommonErrors(t *testing.T) {
	testCases := []struct {
		Err                error
		Predicate          func(error) bool
		RoundtripPredicate func(error) bool
		StatusCode         int
		comment            string
	}{
		{
			comment: "Aggregate unwraps to first aggregated error",
			Err: NewAggregate(
				BadParameter("invalid value of foo"),
				LimitExceeded("limit exceeded"),
			),
			Predicate:          IsAggregate,
			RoundtripPredicate: IsBadParameter,
			StatusCode:         http.StatusBadRequest,
		},
		{
			comment: "Nested aggregate unwraps recursively",
			Err: NewAggregate(
				NewAggregate(
					BadParameter("invalid value of foo"),
					LimitExceeded("limit exceeded"),
				),
			),
			Predicate:          IsAggregate,
			RoundtripPredicate: IsBadParameter,
			StatusCode:         http.StatusBadRequest,
		},
	}
	for _, testCase := range testCases {
		SetDebug(true)
		err := testCase.Err

		forEachReporter(t, func(t *testing.T, reporter func(error) string) {
			assert.Regexp(t, ".*.trace_test.go.*", line(DebugReport(err)), testCase.comment)
		})
		assert.True(t, testCase.Predicate(err), testCase.comment)

		w := newTestWriter()
		WriteError(w, err)
		outErr := ReadError(w.StatusCode, w.Body)
		assert.True(t, testCase.RoundtripPredicate(outErr), testCase.comment)

		SetDebug(false)
		w = newTestWriter()
		WriteError(w, err)
		outErr = ReadError(w.StatusCode, w.Body)
		assert.True(t, testCase.RoundtripPredicate(outErr), testCase.comment)
	}
}

func TestAggregateThrowAwayNils(t *testing.T) {
	err := NewAggregate(fmt.Errorf("error1"), nil, fmt.Errorf("error2"))
	require.NotNil(t, err)
	assert.NotRegexp(t, ".*nil.*", err.Error())
}

func TestAggregateAllNils(t *testing.T) {
	assert.Nil(t, NewAggregate(nil, nil, nil))
}

func TestAggregateFromChannel(t *testing.T) {
	errCh := make(chan error, 3)
	errCh <- fmt.Errorf("Snap!")
	errCh <- fmt.Errorf("BAM")
	errCh <- fmt.Errorf("omg")
	close(errCh)

	err := NewAggregateFromChannel(errCh, context.Background())
	require.NotNil(t, err)
	assert.Regexp(t, ".*Snap!.*", err.Error())
	assert.Regexp(t, ".*BAM.*", err.Error())
	assert.Regexp(t, ".*omg.*", err.Error())
}

func TestAggregateFromChannelCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error)
	outCh := make(chan error)
	go func() {
		outCh <- NewAggregateFromChannel(errCh, ctx)
	}()
	errCh <- fmt.Errorf("Snap!")
	errCh <- fmt.Errorf("BAM")
	errCh <- fmt.Errorf("omg")
	// we never closed the channel, so we just need to make sure
	// the function exits when we cancel it
	cancel()

	err := <-outCh
	assert.Error(t, err)
}

func TestCompositeErrorsCanProperlyUnwrap(t *testing.T) {
	testCases := []struct {
		err            error
		message        string
		wrappedMessage string
	}{
		{
			err:            ConnectionProblem(fmt.Errorf("internal error"), "failed to connect"),
			message:        "failed to connect",
			wrappedMessage: "internal error",
		},
		{
			err:            Retry(fmt.Errorf("transient error"), "connection refused"),
			message:        "connection refused",
			wrappedMessage: "transient error",
		},
		{
			err:            Trust(fmt.Errorf("access denied"), "failed to validate"),
			message:        "failed to validate",
			wrappedMessage: "access denied",
		},
	}
	var wrapper ErrorWrapper
	for _, tt := range testCases {
		assert.Equal(t, tt.message, tt.err.Error())
		assert.Implements(t, &wrapper, Unwrap(tt.err))
		assert.Equal(t, tt.wrappedMessage, Unwrap(tt.err).(ErrorWrapper).OrigError().Error())
	}
}

type testError struct {
	Param string
}

func (n *testError) Error() string {
	return fmt.Sprintf("TestError(param=%v)", n.Param)
}

func (n *testError) OrigError() error {
	return n
}

func newTestWriter() *testWriter {
	return &testWriter{
		H: make(http.Header),
	}
}

type testWriter struct {
	H          http.Header
	Body       []byte
	StatusCode int
}

func (tw *testWriter) Header() http.Header {
	return tw.H
}

func (tw *testWriter) Write(body []byte) (int, error) {
	tw.Body = body
	return len(tw.Body), nil
}

func (tw *testWriter) WriteHeader(code int) {
	tw.StatusCode = code
}

func line(s string) string {
	return strings.ReplaceAll(s, "\n", "")
}

func TestStdlibCompat(t *testing.T) {
	rootErr := BadParameter("root error")

	var err error = rootErr
	for i := 0; i < 10; i++ {
		err = Wrap(err)
	}
	for i := 0; i < 10; i++ {
		err = WrapWithMessage(err, "wrap message %d", i)
	}

	if !errors.Is(err, rootErr) {
		t.Error("trace.Is(err, rootErr): got false, want true")
	}
	otherErr := CompareFailed("other error")
	if errors.Is(err, otherErr) {
		t.Error("trace.Is(err, otherErr): got true, want false")
	}

	var bpErr *BadParameterError
	if !errors.As(err, &bpErr) {
		t.Error("trace.As(err, BadParameterEror): got false, want true")
	}
	var cpErr *ConnectionProblemError
	if errors.As(err, &cpErr) {
		t.Error("trace.As(err, ConnectivityProblemError): got true, want false")
	}

	expectedErr := errors.New("wrapped error message")
	err = &ConnectionProblemError{Err: expectedErr, Message: "error message"}
	wrappedErr := errors.Unwrap(err)
	if wrappedErr == nil {
		t.Errorf("trace.Unwrap(err): got nil, want %v", expectedErr)
	}
	wrappedErrorMessage := wrappedErr.Error()
	if wrappedErrorMessage != expectedErr.Error() {
		t.Errorf("got %q, want %q", wrappedErrorMessage, expectedErr.Error())
	}
}

// TestAggregate_StdLibCompat runs through a scenario which ensures that
// Aggregate behaves well with errors.Is/errors.As in cases with trace wrapped
// errors and stdlib errors
func TestAggregate_StdlibCompat(t *testing.T) {
	randomErr := errors.New("random")
	bpMsg := "bad param"
	bpErr := BadParameter(bpMsg)
	fooErr := errors.New("foo")

	agg := Wrap(NewAggregate(Wrap(bpErr), fooErr))

	assert.ErrorIs(t, agg, bpErr)
	assert.ErrorIs(t, agg, fooErr)
	assert.NotErrorIs(t, agg, randomErr)

	var badParamErrTarget *BadParameterError
	require.ErrorAs(t, agg, &badParamErrTarget)
	assert.Equal(t, bpMsg, badParamErrTarget.Message, "BadParameter message mismatch")

	var notFoundTarget *NotFoundError
	require.False(t, errors.As(agg, &notFoundTarget), "Aggregate does not contain a NotFoundError")
}

func TestIsAggregate(t *testing.T) {
	err1 := errors.New("foo")
	err2 := errors.New("bar")
	errAggregate := Wrap(NewAggregate(err1, err2))
	errGo := fmt.Errorf("go wrap: %w", errAggregate)

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "plain Go error is not aggregate",
			err:  err1,
		},
		{
			name: "Aggregate returns true",
			err:  errAggregate,
			want: true,
		},
		{
			name: "Aggregate unwrapped returns true",
			err:  Unwrap(errAggregate),
			want: true,
		},
		{
			name: "Aggregate Go-wrapped returns true",
			err:  errGo,
			want: true,
		},
		{
			name: "unrelated wrapped error is not aggregate",
			err:  Wrap(err1),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := IsAggregate(test.err); got != test.want {
				t.Errorf("IsAggregate = %v, want %v", got, test.want)
			}
		})
	}
}

func TestAggregate_IsError(t *testing.T) {
	err1 := BadParameter("bad")
	err2 := NotFound("not found")
	err3 := errors.New("some other error")
	errAggregate := NewAggregate(err1, err2, err3)
	errUnrelated := errors.New("unrelated error")

	assert.True(t, IsBadParameter(errAggregate), "IsBadParameter aggregate mismatch")
	assert.True(t, IsNotFound(errAggregate), "IsNotFound aggregate mismatch")
	assert.False(t, IsConnectionProblem(errAggregate), "IsConnectionProblem aggregate mismatch")

	assert.ErrorIs(t, errAggregate, err1)
	assert.ErrorIs(t, errAggregate, err2)
	assert.ErrorIs(t, errAggregate, err3)
	assert.NotErrorIs(t, errAggregate, errUnrelated)
}
