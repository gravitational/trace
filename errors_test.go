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
				err:  NotFound(""),
				name: "NotFound",
			}, {
				err:  os.ErrNotExist,
				name: "os.ErrNotExist",
			},
		},
		&BadParameterError{}: {
			{
				err:  BadParameter(""),
				name: "BadParameter",
			},
		},
		&RetryError{}: {
			{
				err:  Retry(nil, ""),
				name: "Retry",
			},
		},
		&OAuth2Error{}: {
			{
				err:  OAuth2("", "", url.Values{}),
				name: "OAuth2",
			},
		},
		&TrustError{}: {
			{
				err:  Trust(nil, ""),
				name: "Trust",
			},
		},
		&LimitExceededError{}: {
			{
				err:  LimitExceeded(""),
				name: "LimitExceeded",
			},
		},
		&ConnectionProblemError{}: {
			{
				err:  ConnectionProblem(nil, ""),
				name: "ConnectionProblem",
			},
		},
		&AccessDeniedError{}: {
			{
				err:  AccessDenied(""),
				name: "AccessDenied",
			},
		},
		&CompareFailedError{}: {
			{
				err:  CompareFailed(""),
				name: "CompareFailed",
			},
		},
		&NotImplementedError{}: {
			{
				err:  NotImplemented(""),
				name: "NotImplemented",
			},
		},
		&AlreadyExistsError{}: {
			{
				err:  AlreadyExists(""),
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

	require.ErrorIs(t, &CompareFailedError{}, CompareFailed(""))

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

func TestNotFoundError_Is(t *testing.T) {
	errs := []error{
		NotFound("one"),
		NotFound("two"),
		Wrap(NotFound("three")),
	}

	for i := range errs {
		for j := range errs {
			if i == j {
				require.ErrorIs(t, errs[i], errs[j])
				require.ErrorIs(t, Wrap(errs[i]), errs[j])
				require.ErrorIs(t, Wrap(errs[i]), Wrap(errs[j]))
			} else {
				require.NotErrorIs(t, errs[i], errs[j])
				require.NotErrorIs(t, errs[j], errs[i])
			}
		}
		require.ErrorIs(t, errs[i], os.ErrNotExist)
		require.NotErrorIs(t, os.ErrNotExist, errs[i])
	}
}

func TestAlreadyExistsError_Is(t *testing.T) {
	errs := []error{
		AlreadyExists("one"),
		AlreadyExists("two"),
	}

	for i := range errs {
		for j := range errs {
			if i == j {
				require.ErrorIs(t, errs[i], errs[j])
				require.ErrorIs(t, Wrap(errs[i]), errs[j])
				require.ErrorIs(t, Wrap(errs[i]), Wrap(errs[j]))
			} else {
				require.NotErrorIs(t, errs[i], errs[j])
				require.NotErrorIs(t, errs[j], errs[i])
			}
		}
	}
}

func TestBadParameterError_Is(t *testing.T) {
	errs := []error{
		BadParameter("one"),
		BadParameter("two"),
	}

	for i := range errs {
		for j := range errs {
			if i == j {
				require.ErrorIs(t, errs[i], errs[j])
				require.ErrorIs(t, Wrap(errs[i]), errs[j])
				require.ErrorIs(t, Wrap(errs[i]), Wrap(errs[j]))
			} else {
				require.NotErrorIs(t, errs[i], errs[j])
				require.NotErrorIs(t, errs[j], errs[i])
			}
		}
	}
}

func TestNotImplementedError_Is(t *testing.T) {
	errs := []error{
		NotImplemented("one"),
		NotImplemented("two"),
	}

	for i := range errs {
		for j := range errs {
			if i == j {
				require.ErrorIs(t, errs[i], errs[j])
				require.ErrorIs(t, Wrap(errs[i]), errs[j])
				require.ErrorIs(t, Wrap(errs[i]), Wrap(errs[j]))
			} else {
				require.NotErrorIs(t, errs[i], errs[j])
				require.NotErrorIs(t, errs[j], errs[i])
			}
		}
	}
}

func TestCompareFailedError_Is(t *testing.T) {
	errs := []error{
		CompareFailed("one"),
		CompareFailed("two"),
	}

	for i := range errs {
		for j := range errs {
			if i == j {
				require.ErrorIs(t, errs[i], errs[j])
				require.ErrorIs(t, errs[i], Wrap(errs[j]))
				require.ErrorIs(t, Wrap(errs[i]), errs[j])
				require.ErrorIs(t, Wrap(errs[i]), Wrap(errs[j]))
			} else {
				require.NotErrorIs(t, errs[i], errs[j])
				require.NotErrorIs(t, errs[j], errs[i])
			}
		}
	}
}

func TestAccessDeniedError_Is(t *testing.T) {
	errs := []error{
		AccessDenied("one"),
		AccessDenied("two"),
	}

	for i := range errs {
		for j := range errs {
			if i == j {
				require.ErrorIs(t, errs[i], errs[j])
				require.ErrorIs(t, Wrap(errs[i]), errs[j])
				require.ErrorIs(t, Wrap(errs[i]), Wrap(errs[j]))
			} else {
				require.NotErrorIs(t, errs[i], errs[j])
				require.NotErrorIs(t, errs[j], errs[i])
			}
		}
	}
}

func TestConnectionProblemError_Is(t *testing.T) {
	errs := []error{
		ConnectionProblem(io.EOF, "one"),
		ConnectionProblem(os.ErrNotExist, "one"),
		ConnectionProblem(io.EOF, "two"),
		ConnectionProblem(os.ErrNotExist, "two"),
	}

	for i := range errs {
		for j := range errs {
			if i == j {
				require.ErrorIs(t, errs[i], errs[j])
				require.ErrorIs(t, Wrap(errs[i]), errs[j])
				require.ErrorIs(t, Wrap(errs[i]), Wrap(errs[j]))
			} else {
				require.NotErrorIs(t, errs[i], errs[j])
				require.NotErrorIs(t, errs[j], errs[i])
			}
		}
	}
}

func TestLimitExceededError_Is(t *testing.T) {
	errs := []error{
		LimitExceeded("one"),
		LimitExceeded("two"),
	}

	for i := range errs {
		for j := range errs {
			if i == j {
				require.ErrorIs(t, errs[i], errs[j])
				require.ErrorIs(t, Wrap(errs[i]), errs[j])
				require.ErrorIs(t, Wrap(errs[i]), Wrap(errs[j]))
			} else {
				require.NotErrorIs(t, errs[i], errs[j])
				require.NotErrorIs(t, errs[j], errs[i])
			}
		}
	}
}

func TestTrustError_Is(t *testing.T) {
	errs := []error{
		Trust(io.EOF, "one"),
		Trust(os.ErrNotExist, "one"),
		Trust(io.EOF, "two"),
		Trust(os.ErrNotExist, "two"),
	}

	for i := range errs {
		for j := range errs {
			if i == j {
				require.ErrorIs(t, errs[i], errs[j])
				require.ErrorIs(t, Wrap(errs[i]), errs[j])
				require.ErrorIs(t, Wrap(errs[i]), Wrap(errs[j]))
			} else {
				require.NotErrorIs(t, errs[i], errs[j])
				require.NotErrorIs(t, errs[j], errs[i])
			}
		}
	}
}

func TestOauth2Error_Is(t *testing.T) {
	errs := []error{
		OAuth2("a", "one", nil),
		OAuth2("b", "one", url.Values{}),
		OAuth2("c", "one", url.Values{"test": []string{"test"}}),
		OAuth2("d", "one", url.Values{"test": []string{"error"}}),
		OAuth2("d", "one", url.Values{"test": []string{"test", "test"}}),
		OAuth2("a", "two", nil),
		OAuth2("b", "two", url.Values{}),
		OAuth2("c", "two", url.Values{"test": []string{"test"}}),
	}

	for i := range errs {
		for j := range errs {
			if i == j {
				require.ErrorIs(t, errs[i], errs[j])
				require.ErrorIs(t, Wrap(errs[i]), errs[j])
				require.ErrorIs(t, Wrap(errs[i]), Wrap(errs[j]))
			} else {
				require.NotErrorIs(t, errs[i], errs[j])
				require.NotErrorIs(t, errs[j], errs[i])
			}
		}
	}
}

func TestRetryError_Is(t *testing.T) {
	errs := []error{
		Retry(io.EOF, "one"),
		Retry(os.ErrNotExist, "one"),
		Retry(io.EOF, "two"),
		Retry(os.ErrNotExist, "two"),
	}

	for i := range errs {
		for j := range errs {
			if i == j {
				require.ErrorIs(t, errs[i], errs[j])
				require.ErrorIs(t, Wrap(errs[i]), errs[j])
				require.ErrorIs(t, Wrap(errs[i]), Wrap(errs[j]))
			} else {
				require.NotErrorIs(t, errs[i], errs[j])
				require.NotErrorIs(t, errs[j], errs[i])
			}
		}
	}
}

func TestGoErrorWrap_IsError_allTypes(t *testing.T) {
	tests := []struct {
		name          string
		instance      error            // eg, BadParameter("foo")
		instanceForAs interface{}      // eg, &BadParameterError{}
		isError       func(error) bool // eg, IsBadParameter
	}{
		{
			name:          "BadParameter",
			instance:      BadParameter("message"),
			instanceForAs: &BadParameterError{},
			isError:       IsBadParameter,
		},

		{
			name:          "NotFound",
			instance:      NotFound("message"),
			instanceForAs: &NotFoundError{},
			isError:       IsNotFound,
		},
		{
			name:          "AlreadyExists",
			instance:      AlreadyExists("message"),
			instanceForAs: &AlreadyExistsError{},
			isError:       IsAlreadyExists,
		},
		{
			name:          "NotImplemented",
			instance:      NotImplemented("message"),
			instanceForAs: &NotImplementedError{},
			isError:       IsNotImplemented,
		},
		{
			name:          "CompareFailed",
			instance:      CompareFailed("message"),
			instanceForAs: &CompareFailedError{},
			isError:       IsCompareFailed,
		},
		{
			name:          "AccessDenied",
			instance:      AccessDenied("message"),
			instanceForAs: &AccessDeniedError{},
			isError:       IsAccessDenied,
		},
		{
			name:          "ConnectionProblem",
			instance:      ConnectionProblem(errors.New("underlying error"), "message"),
			instanceForAs: &ConnectionProblemError{},
			isError:       IsConnectionProblem,
		},
		{
			name:          "LimitExceeded",
			instance:      LimitExceeded("message"),
			instanceForAs: &LimitExceededError{},
			isError:       IsLimitExceeded,
		},
		{
			name:          "TrustError",
			instance:      Trust(errors.New("underlying error"), "message"),
			instanceForAs: &TrustError{},
			isError:       IsTrustError,
		},
		{
			name:          "OAuth2",
			instance:      OAuth2("code", "message", nil /* query */),
			instanceForAs: &OAuth2Error{},
			isError:       IsOAuth2,
		},
		{
			name:          "RetryError",
			instance:      Retry(errors.New("underyling error"), "message"),
			instanceForAs: &RetryError{},
			isError:       IsRetryError,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err1 := test.instance
			err2 := Wrap(err1)
			err3 := fmt.Errorf("go wrap: %w", err1)
			err4 := fmt.Errorf("go plus trace wrap: %w", err2)

			for _, testErr := range []error{err1, err2, err3, err4} {
				if !test.isError(testErr) {
					t.Errorf("Is%v failed, err=%#v", test.name, testErr)
				}

				otherErr := test.instanceForAs
				if !errors.As(testErr, &otherErr) {
					t.Errorf("errors.As failed, err=%#v", testErr)
				}
			}
		})
	}
}
