/*
   Copyright 2021 Gravitational, Inc.

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
	"fmt"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

// Trace stores structured trace entry, including file line and path
type Trace struct {
	// Path is a full file path
	Path string `json:"path"`
	// Func is a function name
	Func string `json:"func"`
	// Line is a code line number
	Line int `json:"line"`
}

// FrameCursor stores the position in a call stack
type FrameCursor struct {
	// Current specifies the current stack frame.
	// if omitted, rest contains the complete stack
	Current *runtime.Frame
	// Rest specifies the rest of stack frames to explore
	Rest *runtime.Frames
	// N specifies the total number of stack frames
	N int
}

// Traces is a list of trace entries
type Traces []Trace

// maxStackDepth is the maximum number of frames captured for an error.
const maxStackDepth = 32

// Stack is a captured call stack. Program counters are captured eagerly so
// they reflect the call site, but resolving them into file names, function
// names, and line numbers is deferred until Traces because it is comparatively
// expensive and most errors never have their traces inspected.
type Stack struct {
	pcs    [maxStackDepth]uintptr
	n      int
	once   sync.Once
	traces Traces
}

// CaptureStack captures the current call stack with some deep frames skipped.
func CaptureStack(skip int) *Stack {
	s := &Stack{}
	// +2 means that we also skip `CaptureStack` and `runtime.Callers` frames.
	s.n = runtime.Callers(skip+2, s.pcs[:])
	return s
}

// Traces resolves the captured stack on first use and returns the cached
// result on subsequent calls. It is safe for concurrent use.
func (s *Stack) Traces() Traces {
	s.once.Do(func() {
		s.traces = GetTracesFromCursor(FrameCursor{
			Rest: runtime.CallersFrames(s.pcs[:s.n]),
			N:    s.n,
		})
	})
	return s.traces
}

// CaptureTraces gets the current stack trace with some deep frames skipped
func CaptureTraces(skip int) Traces {
	// +1 skips this function in addition to CaptureStack's own frames.
	return CaptureStack(skip + 1).Traces()
}

// GetTracesFromCursor gets the current stack trace from a given cursor
func GetTracesFromCursor(cursor FrameCursor) Traces {
	traces := make(Traces, 0, cursor.N)
	if cursor.Current != nil {
		traces = append(traces, frameToTrace(*cursor.Current))
	}
	for i := 0; i < cursor.N; i++ {
		frame, more := cursor.Rest.Next()
		traces = append(traces, frameToTrace(frame))
		if !more {
			break
		}
	}
	return traces
}

func frameToTrace(frame runtime.Frame) Trace {
	return Trace{
		Func: frame.Function,
		Path: frame.File,
		Line: frame.Line,
	}
}

// Func returns first function in trace list
func (s Traces) Func() string {
	if len(s) == 0 {
		return ""
	}
	return s[0].Func
}

// Func returns just function name
func (s Traces) FuncName() string {
	if len(s) == 0 {
		return ""
	}
	fn := filepath.ToSlash(s[0].Func)
	idx := strings.LastIndex(fn, "/")
	if idx == -1 || idx == len(fn)-1 {
		return fn
	}
	return fn[idx+1:]
}

// Loc points to file/line location in the code
func (s Traces) Loc() string {
	if len(s) == 0 {
		return ""
	}
	return s[0].String()
}

// String returns debug-friendly representaton of trace stack
func (s Traces) String() string {
	if len(s) == 0 {
		return ""
	}
	// Reserve room for each frame's separators and the longest int64 line number.
	const maxLineLen = len("-9223372036854775808")
	const maxFrameOverhead = len("\n\t: ") + maxLineLen
	size := 0
	for _, t := range s {
		size += len(t.Path) + len(t.Func) + maxFrameOverhead
	}

	var sb strings.Builder
	sb.Grow(size)
	var line [maxLineLen]byte
	for i, t := range s {
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteByte('\t')
		sb.WriteString(t.Path)
		sb.WriteByte(':')
		sb.Write(strconv.AppendInt(line[:0], int64(t.Line), 10))
		sb.WriteByte(' ')
		sb.WriteString(t.Func)
	}
	return sb.String()
}

// String returns debug-friendly representation of this trace
func (t *Trace) String() string {
	dir, file := filepath.Split(t.Path)
	dirs := strings.Split(filepath.ToSlash(filepath.Clean(dir)), "/")
	if len(dirs) != 0 {
		file = filepath.Join(dirs[len(dirs)-1], file)
	}
	return fmt.Sprintf("%v:%v", file, t.Line)
}
