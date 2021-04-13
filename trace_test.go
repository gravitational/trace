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
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/gravitational/trace/internal"

	log "github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	. "gopkg.in/check.v1"
)

func TestTrace(t *testing.T) { TestingT(t) }

type TraceSuite struct{}

var _ = Suite(&TraceSuite{})

func (s *TraceSuite) TestEmpty(c *C) {
	c.Assert(DebugReport(nil), Equals, "")
	c.Assert(UserMessage(nil), Equals, "")
	c.Assert(UserMessageWithFields(nil), Equals, "")
	c.Assert(GetFields(nil), DeepEquals, map[string]interface{}{})
}

func (s *TraceSuite) TestWrap(c *C) {
	testErr := &testError{Param: "param"}
	err := Wrap(Wrap(testErr))

	c.Assert(line(DebugReport(err)), Matches, ".*trace_test.go.*")
	c.Assert(line(DebugReport(err)), Not(Matches), ".*trace.go.*")
	c.Assert(line(UserMessage(err)), Not(Matches), ".*trace_test.go.*")
	c.Assert(line(UserMessage(err)), Matches, ".*param.*")
}

func (s *TraceSuite) TestOrigError(c *C) {
	testErr := fmt.Errorf("some error")
	err := Wrap(Wrap(testErr))
	c.Assert(err.OrigError(), Equals, testErr)
}

func (s *TraceSuite) TestIsEOF(c *C) {
	c.Assert(IsEOF(io.EOF), Equals, true)
	c.Assert(IsEOF(Wrap(io.EOF)), Equals, true)
}

func (s *TraceSuite) TestWrapUserMessage(c *C) {
	testErr := fmt.Errorf("description")

	err := Wrap(testErr, "user message")
	c.Assert(line(DebugReport(err)), Matches, "*.trace_test.go.*")
	c.Assert(line(DebugReport(err)), Not(Matches), "*.trace.go.*")
	c.Assert(line(UserMessage(err)), Equals, "user message\tdescription")

	err = Wrap(err, "user message 2")
	c.Assert(line(UserMessage(err)), Equals, "user message 2\tuser message\t\tdescription")
}

func (s *TraceSuite) TestWrapWithMessage(c *C) {
	testErr := fmt.Errorf("description")
	err := WrapWithMessage(testErr, "user message")
	c.Assert(line(UserMessage(err)), Equals, "user message\tdescription")
	c.Assert(line(DebugReport(err)), Matches, "*.trace_test.go.*")
	c.Assert(line(DebugReport(err)), Not(Matches), "*.trace.go.*")
}

func (s *TraceSuite) TestUserMessageWithFields(c *C) {
	testErr := fmt.Errorf("description")
	c.Assert(UserMessageWithFields(testErr), Equals, testErr.Error())

	err := Wrap(testErr, "user message")
	c.Assert(line(UserMessageWithFields(err)), Equals, "user message\tdescription")

	err.AddField("test_key", "test_value")
	c.Assert(line(UserMessageWithFields(err)), Equals, "test_key=\"test_value\" user message\tdescription")
}

func (s *TraceSuite) TestGetFields(c *C) {
	testErr := fmt.Errorf("description")
	c.Assert(GetFields(testErr), DeepEquals, map[string]interface{}{})

	fields := map[string]interface{}{
		"test_key": "test_value",
	}
	err := Wrap(testErr).AddFields(fields)
	c.Assert(GetFields(err), DeepEquals, fields)
}

func (s *TraceSuite) TestWrapNil(c *C) {
	err1 := Wrap(nil, "message: %v", "extra")
	c.Assert(err1, IsNil)

	var err2 error
	err2 = nil

	err3 := Wrap(err2)
	c.Assert(err3, IsNil)

	err4 := Wrap(err3)
	c.Assert(err4, IsNil)
}

func (s *TraceSuite) TestWrapStdlibErrors(c *C) {
	c.Assert(IsNotFound(os.ErrNotExist), Equals, true)
}

func (s *TraceSuite) TestLogFormatter(c *C) {
	for _, f := range []log.Formatter{&TextFormatter{}, &JSONFormatter{}} {
		log.SetFormatter(f)

		// check case with global Infof
		var buf bytes.Buffer
		log.SetOutput(&buf)
		log.Infof("hello")
		c.Assert(line(buf.String()), Matches, ".*trace_test.go.*")

		// check case with embedded Infof
		buf.Reset()
		log.WithFields(log.Fields{"a": "b"}).Infof("hello")
		c.Assert(line(buf.String()), Matches, ".*trace_test.go.*")
	}
}

type panicker string

func (p panicker) String() string {
	panic(p)
}

func (s *TraceSuite) TestTextFormatter(c *C) {
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
		comment := Commentf("test case %v %v, expected match: %v", i+1, tc.comment, tc.match)
		buf := &bytes.Buffer{}
		log.SetOutput(buf)
		tc.log()
		c.Assert(line(buf.String()), Matches, tc.match, comment)
	}
}

func (s *TraceSuite) TestTextFormatterWithColors(c *C) {
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
		comment := Commentf("test case %v %v, expected match: %v", i+1, tc.comment, tc.match)
		buf := &bytes.Buffer{}
		log.SetOutput(buf)
		log.SetLevel(log.DebugLevel)
		tc.log()
		c.Assert(line(buf.String()), Matches, tc.match, comment)
	}
}

func (s *TraceSuite) TestGenericErrors(c *C) {
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
		comment := Commentf(testCase.comment)
		SetDebug(true)
		err := testCase.Err

		if _, ok := err.(*TraceErr); !ok {
			c.Fatal("Expected error to be of type *TraceErr")
		}
		c.Assert(line(DebugReport(err)), Matches, "*.trace_test.go.*", comment)
		c.Assert(line(DebugReport(err)), Not(Matches), "*.errors.go.*", comment)
		c.Assert(line(DebugReport(err)), Not(Matches), "*.trace.go.*", comment)
		c.Assert(testCase.Predicate(err), Equals, true, comment)

		w := newTestWriter()
		WriteError(w, err)

		outErr := ReadError(w.StatusCode, w.Body)
		if _, ok := outErr.(*internal.ProxyError); !ok {
			c.Fatal("Expected error to be of type internal.ProxyError")
		}
		c.Assert(testCase.Predicate(outErr), Equals, true, comment)

		SetDebug(false)
		w = newTestWriter()
		WriteError(w, err)
		outErr = ReadError(w.StatusCode, w.Body)
		c.Assert(testCase.Predicate(outErr), Equals, true, comment)
	}
}

// Make sure we write some output produced by standard errors
func (s *TraceSuite) TestWriteExternalErrors(c *C) {
	err := Wrap(fmt.Errorf("snap!"))

	SetDebug(true)
	w := newTestWriter()
	WriteError(w, err)
	extErr := ReadError(w.StatusCode, w.Body)
	c.Assert(w.StatusCode, Equals, http.StatusInternalServerError)
	c.Assert(strings.Replace(string(w.Body), "\n", "", -1), Matches, "*.snap.*")
	c.Assert(err.Error(), Equals, extErr.Error())

	SetDebug(false)
	w = newTestWriter()
	WriteError(w, err)
	extErr = ReadError(w.StatusCode, w.Body)
	c.Assert(w.StatusCode, Equals, http.StatusInternalServerError)
	c.Assert(strings.Replace(string(w.Body), "\n", "", -1), Matches, "*.snap.*")
	c.Assert(err.Error(), Equals, extErr.Error())
}

type netError struct{}

func (e *netError) Error() string   { return "net error" }
func (e *netError) Timeout() bool   { return true }
func (e *netError) Temporary() bool { return true }

func (s *TraceSuite) TestConvert(c *C) {
	err := ConvertSystemError(&netError{})
	c.Assert(IsConnectionProblem(err), Equals, true, Commentf("failed to detect network error"))

	dir := c.MkDir()
	err = os.Mkdir(dir, 0770)
	err = ConvertSystemError(err)
	c.Assert(IsAlreadyExists(err), Equals, true, Commentf("expected AlreadyExists error, got %T", err))
}

func (s *TraceSuite) TestAggregates(c *C) {
	err1 := Errorf("failed one")
	err2 := Errorf("failed two")
	err := NewAggregate(err1, err2)
	c.Assert(IsAggregate(err), Equals, true)
	agg := Unwrap(err).(Aggregate)
	c.Assert(agg.Errors(), DeepEquals, []error{err1, err2})
	c.Assert(err.Error(), DeepEquals, "failed one, failed two")
}

func (s *TraceSuite) TestErrorf(c *C) {
	err := Errorf("error")
	c.Assert(line(DebugReport(err)), Matches, "*.trace_test.go.*")
	c.Assert(line(DebugReport(err)), Not(Matches), "*.Fields.*")
	c.Assert(err.(*TraceErr).Messages, DeepEquals, []string(nil))
}

func (s *TraceSuite) TestWithField(c *C) {
	err := Wrap(Errorf("error")).AddField("testfield", true)
	c.Assert(line(DebugReport(err)), Matches, "*.testfield.*")
}

func (s *TraceSuite) TestWithFields(c *C) {
	err := Wrap(Errorf("error")).AddFields(map[string]interface{}{
		"testfield1": true,
		"testfield2": "value2",
	})
	c.Assert(line(DebugReport(err)), Matches, "*.Fields.*")
	c.Assert(line(DebugReport(err)), Matches, "*.testfield1: true.*")
	c.Assert(line(DebugReport(err)), Matches, "*.testfield2: value2.*")
}

func (s *TraceSuite) TestAggregateConvertsToCommonErrors(c *C) {
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
		comment := Commentf(testCase.comment)
		SetDebug(true)
		err := testCase.Err

		c.Assert(line(DebugReport(err)), Matches, "*.trace_test.go.*", comment)
		c.Assert(testCase.Predicate(err), Equals, true, comment)

		w := newTestWriter()
		WriteError(w, err)
		outErr := ReadError(w.StatusCode, w.Body)
		c.Assert(testCase.RoundtripPredicate(outErr), Equals, true, comment)

		SetDebug(false)
		w = newTestWriter()
		WriteError(w, err)
		outErr = ReadError(w.StatusCode, w.Body)
		c.Assert(testCase.RoundtripPredicate(outErr), Equals, true, comment)
	}
}

func (s *TraceSuite) TestAggregateThrowAwayNils(c *C) {
	err := NewAggregate(fmt.Errorf("error1"), nil, fmt.Errorf("error2"))
	c.Assert(err.Error(), Not(Matches), ".*nil.*")
}

func (s *TraceSuite) TestAggregateAllNils(c *C) {
	c.Assert(NewAggregate(nil, nil, nil), IsNil)
}

func (s *TraceSuite) TestAggregateFromChannel(c *C) {
	errCh := make(chan error, 3)
	errCh <- fmt.Errorf("Snap!")
	errCh <- fmt.Errorf("BAM")
	errCh <- fmt.Errorf("omg")
	close(errCh)
	err := NewAggregateFromChannel(errCh, context.Background())
	c.Assert(err.Error(), Matches, ".*Snap!.*")
	c.Assert(err.Error(), Matches, ".*BAM.*")
	c.Assert(err.Error(), Matches, ".*omg.*")
}

func (s *TraceSuite) TestAggregateFromChannelCancel(c *C) {
	errCh := make(chan error, 3)
	errCh <- fmt.Errorf("Snap!")
	errCh <- fmt.Errorf("BAM")
	errCh <- fmt.Errorf("omg")
	ctx, cancel := context.WithCancel(context.Background())
	// we never closed the channel so we just need to make sure
	// the function exits when we cancel it
	cancel()
	NewAggregateFromChannel(errCh, ctx)
}

func (s *TraceSuite) TestCompositeErrorsCanProperlyUnwrap(c *C) {
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
		c.Assert(tt.err.Error(), Equals, tt.message)
		c.Assert(Unwrap(tt.err), Implements, &wrapper)
		c.Assert(Unwrap(tt.err).(ErrorWrapper).OrigError().Error(), Equals, tt.wrappedMessage)
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

func BenchmarkStacktraceAllocs(b *testing.B) {
	b.ReportAllocs()
	for n := 0; n < b.N; n++ {
		Wrap(Errorf("example error"))
	}
}
