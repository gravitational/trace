/*
Copyright 2016 Gravitational, Inc.

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

package trail

import (
	"strings"
	"testing"

	"github.com/gravitational/trace"

	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func TestTrail(t *testing.T) {
	suite.Run(t, new(TrailSuite))
}

type TrailSuite struct {
	suite.Suite
}

// TestConversion makes sure we convert all trace supported errors
// to and back from GRPC codes
func (s *TrailSuite) TestConversion() {
	testCases := []struct {
		Error     error
		Message   string
		Predicate func(error) bool
	}{
		{
			Error:     trace.AccessDenied("access denied"),
			Predicate: trace.IsAccessDenied,
		},
		{
			Error:     trace.ConnectionProblem(nil, "problem"),
			Predicate: trace.IsConnectionProblem,
		},
		{
			Error:     trace.NotFound("not found"),
			Predicate: trace.IsNotFound,
		},
		{
			Error:     trace.BadParameter("bad parameter"),
			Predicate: trace.IsBadParameter,
		},
		{
			Error:     trace.CompareFailed("compare failed"),
			Predicate: trace.IsCompareFailed,
		},
		{
			Error:     trace.AccessDenied("denied"),
			Predicate: trace.IsAccessDenied,
		},
		{
			Error:     trace.LimitExceeded("exceeded"),
			Predicate: trace.IsLimitExceeded,
		},
		{
			Error:     trace.NotImplemented("not implemented"),
			Predicate: trace.IsNotImplemented,
		},
	}

	for i, tc := range testCases {
		grpcError := ToGRPC(tc.Error)
		s.Equal(tc.Error.Error(), grpc.ErrorDesc(grpcError), "test case #v", i+1)
		out := FromGRPC(grpcError)
		s.True(tc.Predicate(out), "test case #v", i+1)
	}
}

// TestNil makes sure conversions of nil to and from GRPC are no-op
func (s *TrailSuite) TestNil() {
	out := FromGRPC(ToGRPC(nil))
	s.Nil(out)
}

// TestTraces makes sure we pass traces via metadata and can decode it back
func (s *TrailSuite) TestTraces() {
	err := trace.BadParameter("param")
	meta := metadata.New(nil)
	SetDebugInfo(err, meta)
	err2 := FromGRPC(ToGRPC(err), meta)
	s.Regexp(".*trail_test.go.*", line(trace.DebugReport(err)))
	s.Regexp(".*trail_test.go.*", line(trace.DebugReport(err2)))
}

func line(s string) string {
	return strings.Replace(s, "\n", "", -1)
}
