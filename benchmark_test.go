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

package trace

import (
	"errors"
	"fmt"
	"testing"
)

func BenchmarkWithUserMessage(b *testing.B) {
	baseErr := Wrap(errors.New("failure"))

	b.Run("literal", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			WithUserMessage(baseErr, "additional context")
		}
	})

	b.Run("formatted", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			WithUserMessage(baseErr, "additional context %d", 42)
		}
	})
}

func BenchmarkWithFieldChain(b *testing.B) {
	baseErr := Wrap(errors.New("failure"))
	for _, count := range []int{1, 8, 32} {
		keys := make([]string, count)
		for i := range keys {
			keys[i] = fmt.Sprintf("field-%d", i)
		}
		b.Run(fmt.Sprintf("fields-%d", count), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				err := baseErr
				for field, key := range keys {
					err = WithField(err, key, field)
				}
			}
		})
	}
}

func BenchmarkWithFields(b *testing.B) {
	baseErr := Wrap(errors.New("failure"))
	for _, count := range []int{1, 8, 32} {
		fields := make(map[string]any, count)
		for i := range count {
			fields[fmt.Sprintf("field-%d", i)] = i
		}
		b.Run(fmt.Sprintf("fields-%d", count), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				WithFields(baseErr, fields)
			}
		})
	}
}

func BenchmarkUserMessage(b *testing.B) {
	for _, count := range []int{1, 8, 32} {
		messages := make([]string, count)
		for i := range messages {
			messages[i] = fmt.Sprintf("message-%d", i)
		}
		err := &TraceErr{Err: errors.New("failure"), Messages: messages}
		b.Run(fmt.Sprintf("messages-%d", count), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				err.UserMessage()
			}
		})
	}
}

func BenchmarkUserMessageWithFields(b *testing.B) {
	for _, count := range []int{1, 8, 32} {
		fields := make(map[string]any, count)
		for i := range count {
			fields[fmt.Sprintf("field-%d", i)] = i
		}
		err := &TraceErr{Err: errors.New("failure"), Fields: fields}
		b.Run(fmt.Sprintf("fields-%d", count), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				UserMessageWithFields(err)
			}
		})
	}
}

func BenchmarkTraceDebugReport(b *testing.B) {
	for _, count := range []int{1, 8, 32} {
		traces := benchmarkTraceEntries(count)
		err := &TraceErr{
			Err:      errors.New(`a < b & c > d "e"`),
			Traces:   traces,
			Fields:   map[string]any{"k<ey": "v<al&ue>"},
			Messages: []string{`<script>alert("xss")</script>`},
		}
		b.Run(fmt.Sprintf("traces-%d", count), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				err.DebugReport()
			}
		})
	}
}

func BenchmarkTracesString(b *testing.B) {
	for _, count := range []int{1, 8, 32} {
		traces := benchmarkTraceEntries(count)
		b.Run(fmt.Sprintf("traces-%d", count), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_ = traces.String()
			}
		})
	}
}

func benchmarkTraceEntries(count int) Traces {
	traces := make(Traces, count)
	for i := range traces {
		traces[i] = Trace{Path: "/a/b/c/file.go", Line: 100 + i, Func: "example.function"}
	}
	return traces
}

func BenchmarkAggregate(b *testing.B) {
	for _, count := range []int{1, 8, 32} {
		errs := make(aggregate, count)
		for i := range errs {
			errs[i] = errors.New("failure")
		}
		b.Run(fmt.Sprintf("error-string-%d", count), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_ = errs.Error()
			}
		})
	}
}
