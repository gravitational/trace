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
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/gravitational/trace"

	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
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
			Error: io.EOF,
			Predicate: func(err error) bool {
				return errors.Is(err, io.EOF)
			},
		},
		{
			Error:     trace.AccessDenied("access denied"),
			Predicate: trace.IsAccessDenied,
		},
		{
			Error:     trace.AlreadyExists("already exists"),
			Predicate: trace.IsAlreadyExists,
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
			Error:     trace.ConnectionProblem(nil, "problem"),
			Predicate: trace.IsConnectionProblem,
		},
		{
			Error:     trace.LimitExceeded("exceeded"),
			Predicate: trace.IsLimitExceeded,
		},
		{
			Error:     trace.NotFound("not found"),
			Predicate: trace.IsNotFound,
		},
		{
			Error:     trace.NotImplemented("not implemented"),
			Predicate: trace.IsNotImplemented,
		},
	}

	for i, tc := range testCases {
		grpcError := ToGRPC(tc.Error)
		s.Equal(tc.Error.Error(), status.Convert(grpcError).Message(), "test case %v", i+1)
		out := FromGRPC(grpcError)
		s.True(tc.Predicate(out), "test case %v", i+1)
		s.Regexp(".*trail_test.go.*", line(trace.DebugReport(out)))
		s.NotRegexp(".*trail.go.*", line(trace.DebugReport(out)))
	}
}

// TestNil makes sure conversions of nil to and from GRPC are no-op
func (s *TrailSuite) TestNil() {
	out := FromGRPC(ToGRPC(nil))
	s.Nil(out)
}

// TestFromEOF makes sure that non-grpc error such as io.EOF is preserved well.
func (s *TrailSuite) TestFromEOF() {
	out := FromGRPC(trace.Wrap(io.EOF))
	s.True(trace.IsEOF(out))
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
	return strings.ReplaceAll(s, "\n", "")
}

func TestToGRPCKeepCode(t *testing.T) {
	err := status.Errorf(codes.PermissionDenied, "denied")
	err = ToGRPC(err)
	if code := status.Code(err); code != codes.PermissionDenied {
		t.Errorf("after ToGRPC, got error code %v, want %v, error: %v", code, codes.PermissionDenied, err)
	}
	err = FromGRPC(err)
	if !trace.IsAccessDenied(err) {
		t.Errorf("after FromGRPC, trace.IsAccessDenied is false, want true, error: %v", err)
	}
}

func TestToGRPC_statusError(t *testing.T) {
	err1 := status.Errorf(codes.NotFound, "not found")
	err2 := fmt.Errorf("go wrap: %w", trace.Wrap(err1))

	tests := []struct {
		name string
		err  error
		want error
	}{
		{
			name: "unwrapped status",
			err:  err1,
			want: err1, // Exact same error.
		},
		{
			name: "wrapped status",
			err:  err2,
			want: status.Errorf(codes.NotFound, err2.Error()),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ToGRPC(test.err)

			got, ok := status.FromError(err)
			if !ok {
				t.Fatalf("Failed to convert `got` to a status.Status: %#v", err)
			}
			want, ok := status.FromError(test.want)
			if !ok {
				t.Fatalf("Failed to convert `want` to a status.Status: %#v", err)
			}

			if got.Code() != want.Code() || got.Message() != want.Message() {
				t.Errorf("ToGRPC = %#v, want %#v", got, test.want)
			}
		})
	}
}
