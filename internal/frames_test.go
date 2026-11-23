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

import (
	"runtime"
	"testing"
)

func TestGetTracesCapacityDoesNotLimitFrames(t *testing.T) {
	var pcs [32]uintptr
	n := runtime.Callers(0, pcs[:])

	frames := runtime.CallersFrames(pcs[:n])
	want := 0
	for more := true; more; want++ {
		_, more = frames.Next()
	}

	traces := getTraces(runtime.CallersFrames(pcs[:n]), 1)
	if len(traces) != want {
		t.Fatalf("got %d traces, want %d", len(traces), want)
	}
}

func TestCaptureStackDepth(t *testing.T) {
	baseDepth := len(captureDeepStack(0))
	tests := []struct {
		name  string
		depth int
	}{
		{
			name:  "no recursion",
			depth: 0,
		},
		{
			name:  "shallow",
			depth: 1,
		},
		{
			name:  "initial capacity",
			depth: initialStackDepth,
		},
		{
			name:  "maximum capacity",
			depth: maxStackDepth,
		},
	}

	for _, test := range tests {
		stack := captureDeepStack(test.depth)
		want := min(baseDepth+test.depth, maxStackDepth)
		if len(stack) != want {
			t.Errorf("%s: captured %d frames, want %d", test.name, len(stack), want)
		}
	}
}

func captureDeepStack(depth int) Stack {
	if depth == 0 {
		return CaptureStack(0)
	}
	return captureDeepStack(depth - 1)
}

func TestCapturedStackTraces(t *testing.T) {
	stack := CaptureStack(0)
	if len(stack) == 0 {
		t.Fatal("stack is empty")
	}

	traces := stack.Traces()
	if got := traces.FuncName(); got != "internal.TestCapturedStackTraces" {
		t.Fatalf("first frame = %q, want internal.TestCapturedStackTraces", got)
	}
}
