/*
Copyright 2022 Gravitational, Inc.

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
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_Is(t *testing.T) {

	type testError struct {
		err  error
		name string
	}

	cases := map[error][]testError{
		&NotFoundError{}: {
			{
				err:  NotFound("test"),
				name: "NotFound",
			}, {
				err:  os.ErrNotExist,
				name: "os.ErrNotExist",
			},
		},
		&BadParameterError{}: {
			{
				err:  BadParameter("test"),
				name: "BadParameter",
			},
		},
		&RetryError{}: {
			{
				err:  Retry(io.EOF, "test"),
				name: "Retry",
			},
		},
		&OAuth2Error{}: {
			{
				err:  OAuth2("test", "test", url.Values{}),
				name: "OAuth2",
			},
		},
		&TrustError{}: {
			{
				err:  Trust(io.EOF, "test"),
				name: "Trust",
			},
		},
		&LimitExceededError{}: {
			{
				err:  LimitExceeded("test"),
				name: "LimitExceeded",
			},
		},
		&ConnectionProblemError{}: {
			{
				err:  ConnectionProblem(io.EOF, "test"),
				name: "ConnectionProblem",
			},
		},
		&AccessDeniedError{}: {
			{
				err:  AccessDenied("test"),
				name: "AccessDenied",
			},
		},
		&CompareFailedError{}: {
			{
				err:  CompareFailed("test"),
				name: "CompareFailed",
			},
		},
		&NotImplementedError{}: {
			{
				err:  NotImplemented("test"),
				name: "NotImplemented",
			},
		},
		&AlreadyExistsError{}: {
			{
				err:  AlreadyExists("test"),
				name: "AlreadyExists",
			},
		},
	}

	// none of the target errors should Is for these and many more...
	not := []testError{
		{
			err:  nil,
			name: "nil",
		},
		{
			err:  NewAggregate(Wrap(AccessDenied("test"), AccessDenied("test"), ConnectionProblem(io.EOF, "fail"), Wrap(NotFound("test")))),
			name: "Aggregate",
		},
		{
			err:  errors.New("test"),
			name: "test",
		},
		{
			err:  io.EOF,
			name: "io.EOF",
		},
		{
			err:  os.ErrPermission,
			name: "os.ErrPermission",
		},
		{
			err:  context.Canceled,
			name: "context.Canceled",
		},
	}

	// for each target error in the case check if it Is
	for k := range cases {
		t.Run(fmt.Sprintf("%T", k), func(t *testing.T) {
			for _, te := range not {
				t.Run(fmt.Sprintf("is not %s", te.name), func(t *testing.T) {
					require.NotErrorIs(t, k, te.err)
					require.NotErrorIs(t, k, Wrap(te.err))

				})
			}

			// check that the target error only Is for itself and not any other trace.Error
			for kk, v := range cases {
				for _, te := range v {
					if k == kk { // iterating on the target error - require Is
						t.Run(fmt.Sprintf("is %s", te.name), func(t *testing.T) {
							require.ErrorIs(t, k, te.err)
						})
					} else { // iterating on another error - require not Is
						t.Run(fmt.Sprintf("is not %s", te.name), func(t *testing.T) {
							require.NotErrorIs(t, k, te.err)
						})
					}
				}
			}
		})
	}

}
