/*
Copyright 2026 Gravitational, Inc.

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

package internal

import "testing"

func TestCaptureStackResolvesLazily(t *testing.T) {
	stack := CaptureStack(0)
	if stack.traces != nil {
		t.Fatal("traces resolved before use")
	}

	traces := stack.Traces()
	if got := traces.FuncName(); got != "internal.TestCaptureStackResolvesLazily" {
		t.Fatalf("first frame = %q, want internal.TestCaptureStackResolvesLazily", got)
	}
	if &stack.Traces()[0] != &traces[0] {
		t.Fatal("resolved traces were not cached")
	}
}
