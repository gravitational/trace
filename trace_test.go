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
	"bytes"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"

	log "github.com/Sirupsen/logrus"

	. "gopkg.in/check.v1"
)

func TestTrace(t *testing.T) { TestingT(t) }

type TraceSuite struct {
}

var _ = Suite(&TraceSuite{})

func (s *TraceSuite) TestWrap(c *C) {
	testErr := &TestError{Param: "param"}
	err := Wrap(Wrap(testErr))
	c.Assert(err, Equals, testErr)

	t := err.(*TestError)
	c.Assert(len(t.Traces), Equals, 2)
	c.Assert(err.Error(), Matches, ".*trace_test.go.*")
}

func (s *TraceSuite) TestOrigError(c *C) {
	testErr := fmt.Errorf("some error")
	err := Wrap(Wrap(testErr))
	c.Assert(err.OrigError(), Equals, testErr)
}

func (s *TraceSuite) TestWrapMessage(c *C) {
	testErr := fmt.Errorf("description")

	err := Wrap(testErr)

	SetDebug(true)
	c.Assert(err.Error(), Matches, ".*trace_test.go.*")
	c.Assert(err.Error(), Matches, ".*description.*")

	SetDebug(false)
	c.Assert(err.Error(), Not(Matches), ".*trace_test.go.*")
	c.Assert(err.Error(), Matches, ".*description.*")
}

func (s *TraceSuite) TestWrapNil(c *C) {
	err1 := Wrap(nil)
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
		buf := &bytes.Buffer{}
		log.SetOutput(buf)
		log.Infof("hello")
		c.Assert(strings.TrimSpace(buf.String()), Matches, ".*trace_test.go.*")

		// check case with embedded Infof
		buf = &bytes.Buffer{}
		log.SetOutput(buf)
		log.WithFields(log.Fields{"a": "b"}).Infof("hello")
		c.Assert(strings.TrimSpace(buf.String()), Matches, ".*trace_test.go.*")
	}
}

func (s *TraceSuite) TestGenericErrors(c *C) {
	testCases := []struct {
		Err        error
		Predicate  func(error) bool
		StatusCode int
	}{
		{
			Err:        NotFound("not found"),
			Predicate:  IsNotFound,
			StatusCode: http.StatusNotFound,
		},
		{
			Err:        AlreadyExists("already exists"),
			Predicate:  IsAlreadyExists,
			StatusCode: http.StatusConflict,
		},
		{
			Err:        BadParameter("is bad"),
			Predicate:  IsBadParameter,
			StatusCode: http.StatusBadRequest,
		},
		{
			Err:        CompareFailed("is bad"),
			Predicate:  IsCompareFailed,
			StatusCode: http.StatusPreconditionFailed,
		},
		{
			Err:        AccessDenied("denied"),
			Predicate:  IsAccessDenied,
			StatusCode: http.StatusForbidden,
		},
		{
			Err:        ConnectionProblem(nil, "prob"),
			Predicate:  IsConnectionProblem,
			StatusCode: http.StatusRequestTimeout,
		},
		{
			Err:        LimitExceeded("limit exceeded"),
			Predicate:  IsLimitExceeded,
			StatusCode: statusTooManyRequests,
		},
	}

	for i, testCase := range testCases {
		comment := Commentf("test case #%v", i+1)
		SetDebug(true)
		err := testCase.Err

		t := err.(*TraceErr)
		c.Assert(len(t.Traces), Equals, 1, comment)
		c.Assert(err.Error(), Matches, "*.trace_test.go.*", comment)
		c.Assert(testCase.Predicate(err), Equals, true, comment)

		w := newTestWriter()
		WriteError(w, err)
		outerr := ReadError(w.StatusCode, w.Body)
		c.Assert(testCase.Predicate(outerr), Equals, true, comment)
		t = outerr.(*TraceErr)
		c.Assert(len(t.Traces), Equals, 2, comment)

		SetDebug(false)
		w = newTestWriter()
		WriteError(w, err)
		outerr = ReadError(w.StatusCode, w.Body)
		c.Assert(testCase.Predicate(outerr), Equals, true, comment)
		t = outerr.(*TraceErr)
		c.Assert(len(t.Traces), Equals, 1, comment)
	}
}

// Make sure we write some output produced by standard errors
func (s *TraceSuite) TestWriteExternalErrors(c *C) {
	err := fmt.Errorf("snap!")

	SetDebug(true)
	w := newTestWriter()
	WriteError(w, err)
	c.Assert(w.StatusCode, Equals, http.StatusInternalServerError)
	c.Assert(strings.Replace(string(w.Body), "\n", "", -1), Matches, "*.snap.*")

	SetDebug(false)
	w = newTestWriter()
	WriteError(w, err)
	c.Assert(w.StatusCode, Equals, http.StatusInternalServerError)
	c.Assert(strings.Replace(string(w.Body), "\n", "", -1), Matches, "*.snap.*")
}

type TestError struct {
	Traces
	Param string
}

func (n *TestError) Error() string {
	return fmt.Sprintf("TestError(param=%v,trace=%v)", n.Param, n.Traces)
}

func (n *TestError) OrigError() error {
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
