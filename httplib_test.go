/*
Copyright 2020 Gravitational, Inc.

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
	"errors"
	"net/http/httptest"

	. "gopkg.in/check.v1"
)

type HttplibSuite struct{}

var _ = Suite(&HttplibSuite{})

var (
	errCode               = 400
	errText               = "test error"
	expectedErrorResponse = "" +
		"{\n" +
		"    \"error\": {\n" +
		"        \"message\": \"" + errText + "\"\n" +
		"    }\n" +
		"}"
)

func (s *HttplibSuite) TestRegularErrorResponseJSON(c *C) {
	recorder := httptest.NewRecorder()
	replyJSON(recorder, errCode, errors.New(errText))
	c.Assert(recorder.Body.String(), Equals, expectedErrorResponse)
}

func (s *HttplibSuite) TestTraceErrorResponseJSON(c *C) {
	recorder := httptest.NewRecorder()
	replyJSON(recorder, errCode, &TraceErr{Err: errors.New(errText)})
	c.Assert(recorder.Body.String(), Equals, expectedErrorResponse)
}
