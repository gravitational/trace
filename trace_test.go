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
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmpty(t *testing.T) {
	assert.Equal(t, "", DebugReport(nil))
	assert.Equal(t, "", UserMessage(nil))
	assert.Equal(t, "", UserMessageWithFields(nil))
	assert.Equal(t, map[string]interface{}{}, GetFields(nil))
}

func TestWrap(t *testing.T) {
	testErr := &testError{Param: "param"}
	err := Wrap(Wrap(testErr))

	assert.Regexp(t, ".*trace_test.go.*", line(DebugReport(err)))
	assert.NotRegexp(t, ".*trace.go.*", line(DebugReport(err)))
	assert.NotRegexp(t, ".*trace_test.go.*", line(UserMessage(err)))
	assert.Regexp(t, ".*param.*", line(UserMessage(err)))
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
	assert.Regexp(t, ".*trace_test.go.*", line(DebugReport(err)))
	assert.NotRegexp(t, ".*trace.go.*", line(DebugReport(err)))
	assert.Equal(t, "user message\tdescription", line(UserMessage(err)))

	err = Wrap(err, "user message 2")
	assert.Equal(t, "user message 2\tuser message\t\tdescription", line(UserMessage(err)))
}

func TestWrapWithMessage(t *testing.T) {
	testErr := fmt.Errorf("description")
	err := WrapWithMessage(testErr, "user message")
	assert.Equal(t, "user message\tdescription", line(UserMessage(err)))
	assert.Regexp(t, ".*trace_test.go.*", line(DebugReport(err)))
	assert.NotRegexp(t, ".*trace.go.*", line(DebugReport(err)))
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

func TestLogFormatter(t *testing.T) {
	for _, f := range []log.Formatter{&TextFormatter{}, &JSONFormatter{}} {
		log.SetFormatter(f)

		// check case with global Infof
		var buf bytes.Buffer
		log.SetOutput(&buf)
		log.Infof("hello")
		assert.Regexp(t, ".*trace_test.go.*", line(buf.String()))

		// check case with embedded Infof
		buf.Reset()
		log.WithFields(log.Fields{"a": "b"}).Infof("hello")
		assert.Regexp(t, ".*trace_test.go.*", line(buf.String()))
	}
}

type panicker string

func (p panicker) String() string {
	panic(p)
}

func TestTextFormatter(t *testing.T) {
	padding := 6
	f := &TextFormatter{
		DisableTimestamp: true,
		ComponentPadding: padding,
	}
	log.SetFormatter(f)

	type testCase struct {
		log     func()
		match   string
		comment string
	}

	testCases := []testCase{
		{
			comment: "padding fits in",
			log: func() {
				log.WithFields(log.Fields{
					Component: "test",
				}).Infof("hello")
			},
			match: `^INFO \[TEST\] hello.*`,
		},
		{
			comment: "padding overflow",
			log: func() {
				log.WithFields(log.Fields{
					Component: "longline",
				}).Infof("hello")
			},
			match: `^INFO \[LONG\] hello.*`,
		},
		{
			comment: "padded with extra spaces",
			log: func() {
				log.WithFields(log.Fields{
					Component: "abc",
				}).Infof("hello")
			},
			match: `^INFO \[ABC\]  hello.*`,
		},
		{
			comment: "missing component will be padded",
			log: func() {
				log.Infof("hello")
			},
			match: `^INFO        hello.*`,
		},
		{
			comment: "panic in component is handled",
			log: func() {
				log.WithFields(log.Fields{
					Component: panicker("panic"),
				}).Infof("hello")
			},
			match: `.*panic.*`,
		},
		{
			comment: "nested fields are reflected",
			log: func() {
				log.WithFields(log.Fields{
					ComponentFields: log.Fields{"key": "value"},
				}).Infof("hello")
			},
			match: `.*key:value.*`,
		},
		{
			comment: "fields are reflected",
			log: func() {
				log.WithFields(log.Fields{
					"a": "b",
				}).Infof("hello")
			},
			match: `.*a:b.*`,
		},
		{
			comment: "non control characters are quoted",
			log: func() {
				log.Infof("\n")
			},
			match: `.*"\\n".*`,
		},
		{
			comment: "printable strings are not quoted",
			log: func() {
				log.Infof("printable string")
			},
			match: `.*[^"]printable string[^"].*`,
		},
	}

	for i, tc := range testCases {
		buf := &bytes.Buffer{}
		log.SetOutput(buf)
		tc.log()
		assert.Regexp(t, tc.match, line(buf.String()), "test case %v %v, expected match: %v", i+1, tc.comment, tc.match)
	}
}

func TestTextFormatterWithColors(t *testing.T) {
	padding := 6
	f := &TextFormatter{
		DisableTimestamp: true,
		ComponentPadding: padding,
		EnableColors:     true,
	}
	log.SetFormatter(f)

	type testCase struct {
		log     func()
		match   string
		comment string
	}

	testCases := []testCase{
		{
			comment: "test info color",
			log: func() {
				log.WithFields(log.Fields{
					Component: "test",
				}).Info("hello")
			},
			match: `^\x1b\[36mINFO\x1b\[0m \[TEST\] hello.*`,
		},
		{
			comment: "info color padding overflow",
			log: func() {
				log.WithFields(log.Fields{
					Component: "longline",
				}).Info("hello")
			},
			match: `^\x1b\[36mINFO\x1b\[0m \[LONG\] hello.*`,
		},
		{
			comment: "test debug color",
			log: func() {
				log.WithFields(log.Fields{
					Component: "test",
				}).Debug("hello")
			},
			match: `^\x1b\[37mDEBU\x1b\[0m \[TEST\] hello.*`,
		},
		{
			comment: "test warn color",
			log: func() {
				log.WithFields(log.Fields{
					Component: "test",
				}).Warning("hello")
			},
			match: `^\x1b\[33mWARN\x1b\[0m \[TEST\] hello.*`,
		},
		{
			comment: "test error color",
			log: func() {
				log.WithFields(log.Fields{
					Component: "test",
				}).Error("hello")
			},
			match: `^\x1b\[31mERRO\x1b\[0m \[TEST\] hello.*`,
		},
	}

	for i, tc := range testCases {
		buf := &bytes.Buffer{}
		log.SetOutput(buf)
		log.SetLevel(log.DebugLevel)
		tc.log()
		assert.Regexpf(t, tc.match, line(buf.String()), "test case %v %v, expected match: %v", i+1, tc.comment, tc.match)
	}
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
		assert.Regexp(t, ".*.trace_test\\.go.*", line(DebugReport(err)), testCase.comment)
		assert.NotRegexp(t, ".*.errors\\.go.*", line(DebugReport(err)), testCase.comment)
		assert.NotRegexp(t, ".*.trace\\.go.*", line(DebugReport(err)), testCase.comment)
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
	assert.Regexp(t, ".*.trace_test.go.*", line(DebugReport(err)))
	assert.NotRegexp(t, ".*.Fields.*", line(DebugReport(err)))
	assert.Equal(t, []string(nil), err.(*TraceErr).Messages)
}

func TestWithField(t *testing.T) {
	err := WithField(Wrap(Errorf("error")), "testfield", true)
	assert.Regexp(t, ".*.testfield.*", line(DebugReport(err)))
}

func TestWithFields(t *testing.T) {
	err := WithFields(Wrap(Errorf("error")), map[string]interface{}{
		"testfield1": true,
		"testfield2": "value2",
	})
	assert.Regexp(t, ".*.Fields.*", line(DebugReport(err)))
	assert.Regexp(t, ".*.testfield1: true.*", line(DebugReport(err)))
	assert.Regexp(t, ".*.testfield2: value2.*", line(DebugReport(err)))
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

		assert.Regexp(t, ".*.trace_test.go.*", line(DebugReport(err)), testCase.comment)
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

// TestStdLibCompat_Aggregate runs through a scenario which ensures that
// Aggregate behaves well with errors.Is/errors.As in cases with trace
// wrapped errors and stdlib errors
func TestStdlibCompat_Aggregate(t *testing.T) {
	randomErr := fmt.Errorf("random")
	bpMsg := "bad param"
	badParamErr := BadParameter(bpMsg)
	fooErr := fmt.Errorf("foo")

	agg := Wrap(NewAggregate(Wrap(badParamErr), fooErr))

	require.ErrorIs(t, agg, badParamErr)
	require.ErrorIs(t, agg, fooErr)
	require.NotErrorIs(t, agg, randomErr)

	var badParamErrTarget *BadParameterError
	require.ErrorAs(t, agg, &badParamErrTarget)
	require.Equal(t, bpMsg, badParamErrTarget.Message)
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
