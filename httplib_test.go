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
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReplyJSON(t *testing.T) {
	t.Parallel()
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

	testCase := func(t *testing.T, err error) {
		recorder := httptest.NewRecorder()
		replyJSON(recorder, errCode, err)
		require.Equal(t, expectedErrorResponse, recorder.Body.String())
	}

	testCase(t, errors.New("test error"))
	testCase(t, &TraceErr{Err: errors.New("test error")})
}

func TestUnmarshalError(t *testing.T) {
	t.Parallel()
	testCase := func(t *testing.T, err error, response string, isExpectedErr func(error) bool, expectedMsg string) {
		readErr := unmarshalError(err, []byte(response))
		require.True(t, isExpectedErr(readErr))
		require.EqualError(t, readErr, expectedMsg)
	}

	testCase(t, &NotFoundError{}, `{"error": {"message": "ABC"}}`, IsNotFound, "ABC")
	testCase(t, &AccessDeniedError{}, `{"error": {"message": "ABC"}}`, IsAccessDenied, "ABC")
}
