//go:build !gravitational_trace.nocrypto

// Copyright 2026 Gravitational, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package trace

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
		assert.Regexp(t, ".*.trace_crypto_test\\.go.*", line(DebugReport(err)), testCase.comment)
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

		assert.Regexp(t, ".*.trace_crypto_test.go.*", line(DebugReport(err)), testCase.comment)
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
