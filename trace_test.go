package trace

import (
	"fmt"
	"testing"

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
