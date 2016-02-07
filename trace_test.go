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
	err := Wrap(Wrap(&TestError{Param: "param"}))
	c.Assert(err, FitsTypeOf, &TestError{})
	t := err.(*TestError)
	c.Assert(len(t.Traces), Equals, 2)
	c.Assert(err.Error(), Matches, "*.trace_test.go.*")
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
