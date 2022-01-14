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
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/suite"
)

func TestTrace(t *testing.T) {
	suite.Run(t, new(TraceSuite))
}

type TraceSuite struct {
	suite.Suite
}

func (s *TraceSuite) TestEmpty() {
	s.Equal("", DebugReport(nil))
	s.Equal("", UserMessage(nil))
	s.Equal("", UserMessageWithFields(nil))
	s.Equal(map[string]interface{}{}, GetFields(nil))
}

func (s *TraceSuite) TestWrap() {
	testErr := &testError{Param: "param"}
	err := Wrap(Wrap(testErr))

	s.Regexp(".*trace_test.go.*", line(DebugReport(err)))
	s.NotRegexp(".*trace.go.*", line(DebugReport(err)))
	s.NotRegexp(".*trace_test.go.*", line(UserMessage(err)))
	s.Regexp(".*param.*", line(UserMessage(err)))
}

func (s *TraceSuite) TestOrigError() {
	testErr := fmt.Errorf("some error")
	err := Wrap(Wrap(testErr))

	s.Equal(testErr, err.OrigError())
}

func (s *TraceSuite) TestIsEOF() {
	s.True(IsEOF(io.EOF))
	s.True(IsEOF(Wrap(io.EOF)))
}

func (s *TraceSuite) TestWrapUserMessage() {
	testErr := fmt.Errorf("description")

	err := Wrap(testErr, "user message")
	s.Regexp(".*trace_test.go.*", line(DebugReport(err)))
	s.NotRegexp(".*trace.go.*", line(DebugReport(err)))
	s.Equal("user message\tdescription", line(UserMessage(err)))

	err = Wrap(err, "user message 2")
	s.Equal("user message 2\tuser message\t\tdescription", line(UserMessage(err)))
}

func (s *TraceSuite) TestWrapWithMessage() {
	testErr := fmt.Errorf("description")
	err := WrapWithMessage(testErr, "user message")
	s.Equal("user message\tdescription", line(UserMessage(err)))
	s.Regexp(".*trace_test.go.*", line(DebugReport(err)))
	s.NotRegexp(".*trace.go.*", line(DebugReport(err)))
}

func (s *TraceSuite) TestUserMessageWithFields() {
	testErr := fmt.Errorf("description")
	s.Equal(testErr.Error(), UserMessageWithFields(testErr))

	err := Wrap(testErr, "user message")
	s.Equal("user message\tdescription", line(UserMessageWithFields(err)))

	_ = err.AddField("test_key", "test_value")
	s.Equal("test_key=\"test_value\" user message\tdescription", line(UserMessageWithFields(err)))
}

func (s *TraceSuite) TestGetFields() {
	testErr := fmt.Errorf("description")
	s.Equal(map[string]interface{}{}, GetFields(testErr))

	fields := map[string]interface{}{
		"test_key": "test_value",
	}
	err := Wrap(testErr).AddFields(fields)
	s.Equal(fields, GetFields(err))
}

func (s *TraceSuite) TestWrapNil() {
	err1 := Wrap(nil, "message: %v", "extra")
	s.Nil(err1)

	var err2 error
	err2 = nil

	err3 := Wrap(err2)
	s.Nil(err3)

	err4 := Wrap(err3)
	s.Nil(err4)
}

func (s *TraceSuite) TestWrapStdlibErrors() {
	s.True(IsNotFound(os.ErrNotExist))
}

func (s *TraceSuite) TestLogFormatter() {
	for _, f := range []log.Formatter{&TextFormatter{}, &JSONFormatter{}} {
		log.SetFormatter(f)

		// check case with global Infof
		var buf bytes.Buffer
		log.SetOutput(&buf)
		log.Infof("hello")
		s.Regexp(".*trace_test.go.*", line(buf.String()))

		// check case with embedded Infof
		buf.Reset()
		log.WithFields(log.Fields{"a": "b"}).Infof("hello")
		s.Regexp(".*trace_test.go.*", line(buf.String()))
	}
}

type panicker string

func (p panicker) String() string {
	panic(p)
}

func (s *TraceSuite) TestTextFormatter() {
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
		s.Regexp(tc.match, line(buf.String()), "test case %v %v, expected match: %v", i+1, tc.comment, tc.match)
	}
}

func (s *TraceSuite) TestTextFormatterWithColors() {
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
		s.Regexpf(tc.match, line(buf.String()), "test case %v %v, expected match: %v", i+1, tc.comment, tc.match)
	}
}

func (s *TraceSuite) TestGenericErrors() {
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
			s.Fail("Expected error to be of type *TraceErr")
		}

		s.NotEmpty(traceErr.Traces, testCase.comment)
		s.Regexp(".*.trace_test\\.go.*", line(DebugReport(err)), testCase.comment)
		s.NotRegexp(".*.errors\\.go.*", line(DebugReport(err)), testCase.comment)
		s.NotRegexp(".*.trace\\.go.*", line(DebugReport(err)), testCase.comment)
		s.True(testCase.Predicate(err), testCase.comment)

		w := newTestWriter()
		WriteError(w, err)

		outErr := ReadError(w.StatusCode, w.Body)
		if _, ok := outErr.(proxyError); !ok {
			s.Fail("Expected error to be of type proxyError")
		}
		s.True(testCase.Predicate(outErr), testCase.comment)

		SetDebug(false)
		w = newTestWriter()
		WriteError(w, err)
		outErr = ReadError(w.StatusCode, w.Body)
		s.True(testCase.Predicate(outErr), testCase.comment)
	}
}

// Make sure we write some output produced by standard errors
func (s *TraceSuite) TestWriteExternalErrors() {
	err := Wrap(fmt.Errorf("snap!"))

	SetDebug(true)
	w := newTestWriter()
	WriteError(w, err)
	extErr := ReadError(w.StatusCode, w.Body)
	s.Equal(http.StatusInternalServerError, w.StatusCode)
	s.Regexp(".*.snap.*", strings.Replace(string(w.Body), "\n", "", -1))
	s.Require().NotNil(extErr)
	s.EqualError(err, extErr.Error())

	SetDebug(false)
	w = newTestWriter()
	WriteError(w, err)
	extErr = ReadError(w.StatusCode, w.Body)
	s.Equal(http.StatusInternalServerError, w.StatusCode)
	s.Regexp(".*.snap.*", strings.Replace(string(w.Body), "\n", "", -1))
	s.Require().NotNil(extErr)
	s.EqualError(err, extErr.Error())
}

type netError struct{}

func (e *netError) Error() string   { return "net" }
func (e *netError) Timeout() bool   { return true }
func (e *netError) Temporary() bool { return true }

func (s *TraceSuite) TestConvert() {
	err := ConvertSystemError(&netError{})
	s.True(IsConnectionProblem(err), "failed to detect network error")

	dir := s.T().TempDir()
	err = os.Mkdir(dir, 0770)
	err = ConvertSystemError(err)
	s.True(IsAlreadyExists(err), "expected AlreadyExists error, got %T", err)
}

func (s *TraceSuite) TestAggregates() {
	err1 := Errorf("failed one")
	err2 := Errorf("failed two")

	err := NewAggregate(err1, err2)
	s.True(IsAggregate(err))

	agg := Unwrap(err).(Aggregate)
	s.Equal([]error{err1, err2}, agg.Errors())
	s.Equal("failed one, failed two", err.Error())
}

func (s *TraceSuite) TestErrorf() {
	err := Errorf("error")
	s.Regexp(".*.trace_test.go.*", line(DebugReport(err)))
	s.NotRegexp(".*.Fields.*", line(DebugReport(err)))
	s.Equal([]string(nil), err.(*TraceErr).Messages)
}

func (s *TraceSuite) TestWithField() {
	err := Wrap(Errorf("error")).AddField("testfield", true)
	s.Regexp(".*.testfield.*", line(DebugReport(err)))
}

func (s *TraceSuite) TestWithFields() {
	err := Wrap(Errorf("error")).AddFields(map[string]interface{}{
		"testfield1": true,
		"testfield2": "value2",
	})
	s.Regexp(".*.Fields.*", line(DebugReport(err)))
	s.Regexp(".*.testfield1: true.*", line(DebugReport(err)))
	s.Regexp(".*.testfield2: value2.*", line(DebugReport(err)))
}

func (s *TraceSuite) TestAggregateConvertsToCommonErrors() {
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

		s.Regexp(".*.trace_test.go.*", line(DebugReport(err)), testCase.comment)
		s.True(testCase.Predicate(err), testCase.comment)

		w := newTestWriter()
		WriteError(w, err)
		outErr := ReadError(w.StatusCode, w.Body)
		s.True(testCase.RoundtripPredicate(outErr), testCase.comment)

		SetDebug(false)
		w = newTestWriter()
		WriteError(w, err)
		outErr = ReadError(w.StatusCode, w.Body)
		s.True(testCase.RoundtripPredicate(outErr), testCase.comment)
	}
}

func (s *TraceSuite) TestAggregateThrowAwayNils() {
	err := NewAggregate(fmt.Errorf("error1"), nil, fmt.Errorf("error2"))
	s.Require().NotNil(err)
	s.NotRegexp(".*nil.*", err.Error())
}

func (s *TraceSuite) TestAggregateAllNils() {
	s.Nil(NewAggregate(nil, nil, nil))
}

func (s *TraceSuite) TestAggregateFromChannel() {
	errCh := make(chan error, 3)
	errCh <- fmt.Errorf("Snap!")
	errCh <- fmt.Errorf("BAM")
	errCh <- fmt.Errorf("omg")
	close(errCh)

	err := NewAggregateFromChannel(errCh, context.Background())
	s.Require().NotNil(err)
	s.Regexp(".*Snap!.*", err.Error())
	s.Regexp(".*BAM.*", err.Error())
	s.Regexp(".*omg.*", err.Error())
}

func (s *TraceSuite) TestAggregateFromChannelCancel() {
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
	s.Error(err)
}

func (s *TraceSuite) TestCompositeErrorsCanProperlyUnwrap() {
	var testCases = []struct {
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
		s.Equal(tt.message, tt.err.Error())
		s.Implements(&wrapper, Unwrap(tt.err))
		s.Equal(tt.wrappedMessage, Unwrap(tt.err).(ErrorWrapper).OrigError().Error())
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
	return strings.Replace(s, "\n", "", -1)
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
