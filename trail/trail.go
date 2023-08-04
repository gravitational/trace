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

// Package trail integrates trace errors with GRPC
//
// Example server that sends the GRPC error and attaches metadata:
//
//	func (s *server) Echo(ctx context.Context, message *gw.StringMessage) (*gw.StringMessage, error) {
//		trace.SetDebug(true) // to tell trace to start attaching metadata
//		// Send sends metadata via grpc header and converts error to GRPC compatible one
//		return nil, trail.Send(ctx, trace.AccessDenied("missing authorization"))
//	}
//
// Example client reading error and trace debug info:
//
//	var header metadata.MD
//	r, err := c.Echo(context.Background(), &gw.StringMessage{Value: message}, grpc.Header(&header))
//	if err != nil {
//		// FromGRPC reads error, converts it back to trace error and attaches debug metadata
//		// like stack trace of the error origin back to the error
//		err = trail.FromGRPC(err, header)
//
//		// this line will log original trace of the error
//		log.Errorf("error saying echo: %v", trace.DebugReport(err))
//		return
//	}
package trail

import (
	"container/list"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"

	"github.com/gravitational/trace"
	"github.com/gravitational/trace/internal"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Send is a high level function that:
// * converts error to GRPC error
// * attaches debug metadata to existing metadata if possible
// * sends the header to GRPC
func Send(ctx context.Context, err error) error {
	meta, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		meta = metadata.New(nil)
	}
	if trace.IsDebug() {
		SetDebugInfo(err, meta)
	}
	if len(meta) != 0 {
		sendErr := grpc.SendHeader(ctx, meta)
		if sendErr != nil {
			return trace.NewAggregate(err, sendErr)
		}
	}
	return ToGRPC(err)
}

// DebugReportMetadata is a key in metadata holding debug information
// about the error - stack traces and original error
const DebugReportMetadata = "trace-debug-report"

// ToGRPC converts error to GRPC-compatible error
func ToGRPC(originalErr error) error {
	if originalErr == nil {
		return nil
	}

	// Avoid modifying top-level gRPC errors.
	if _, ok := status.FromError(originalErr); ok {
		return originalErr
	}

	errFromCode := func(code codes.Code) error {
		return status.Errorf(code, trace.UserMessage(originalErr))
	}

	l := list.New()
	l.PushBack(originalErr)
	for l.Len() > 0 {
		elem := l.Front()
		l.Remove(elem)
		e := elem.Value.(error)

		if e == io.EOF {
			// Keep legacy semantics and return the original error.
			return originalErr
		}

		if s, ok := status.FromError(e); ok {
			return errFromCode(s.Code())
		}

		// Duplicate check from trace.IsNotFound.
		if os.IsNotExist(e) {
			return errFromCode(codes.NotFound)
		}

		switch e := e.(type) {
		case *trace.AccessDeniedError:
			return errFromCode(codes.PermissionDenied)
		case *trace.AlreadyExistsError:
			return errFromCode(codes.AlreadyExists)
		case *trace.BadParameterError:
			return errFromCode(codes.InvalidArgument)
		case *trace.CompareFailedError:
			return errFromCode(codes.FailedPrecondition)
		case *trace.ConnectionProblemError:
			return errFromCode(codes.Unavailable)
		case *trace.LimitExceededError:
			return errFromCode(codes.ResourceExhausted)
		case *trace.NotFoundError:
			return errFromCode(codes.NotFound)
		case *trace.NotImplementedError:
			return errFromCode(codes.Unimplemented)
		case *trace.OAuth2Error:
			return errFromCode(codes.InvalidArgument)
		case *trace.RetryError: // Not mapped.
		case *trace.TrustError: // Not mapped.

		case interface{ Unwrap() error }:
			if err := e.Unwrap(); err != nil {
				l.PushBack(err)
			}

		case interface{ Unwrap() []error }:
			for _, err := range e.Unwrap() {
				if err != nil {
					l.PushBack(err)
				}
			}
		}
	}

	return errFromCode(codes.Unknown)
}

// FromGRPC converts error from GRPC error back to trace.Error
// Debug information will be retrieved from the metadata if specified in args
func FromGRPC(err error, args ...interface{}) error {
	if err == nil {
		return nil
	}

	statusErr := status.Convert(err)
	code := statusErr.Code()
	message := statusErr.Message()

	var e error
	switch code {
	case codes.OK:
		return nil
	case codes.NotFound:
		e = &trace.NotFoundError{Message: message}
	case codes.AlreadyExists:
		e = &trace.AlreadyExistsError{Message: message}
	case codes.PermissionDenied:
		e = &trace.AccessDeniedError{Message: message}
	case codes.FailedPrecondition:
		e = &trace.CompareFailedError{Message: message}
	case codes.InvalidArgument:
		e = &trace.BadParameterError{Message: message}
	case codes.ResourceExhausted:
		e = &trace.LimitExceededError{Message: message}
	case codes.Unavailable:
		e = &trace.ConnectionProblemError{Message: message}
	case codes.Unimplemented:
		e = &trace.NotImplementedError{Message: message}
	default:
		e = err
	}
	if len(args) != 0 {
		if meta, ok := args[0].(metadata.MD); ok {
			e = DecodeDebugInfo(e, meta)
			// We return here because if it's a trace.Error then
			// frames was already extracted from metadata so
			// there's no need to capture frames once again.
			if _, ok := e.(trace.Error); ok {
				return e
			}
		}
	}
	traces := internal.CaptureTraces(1)
	return &trace.TraceErr{Err: e, Traces: traces}
}

// SetDebugInfo adds debug metadata about error (traces, original error)
// to request metadata as encoded property
func SetDebugInfo(err error, meta metadata.MD) {
	if _, ok := err.(*trace.TraceErr); !ok {
		return
	}
	out, err := json.Marshal(err)
	if err != nil {
		return
	}
	meta[DebugReportMetadata] = []string{
		base64.StdEncoding.EncodeToString(out),
	}
}

// DecodeDebugInfo decodes debug information about error
// from the metadata and returns error with enriched metadata about it
func DecodeDebugInfo(err error, meta metadata.MD) error {
	if len(meta) == 0 {
		return err
	}
	encoded, ok := meta[DebugReportMetadata]
	if !ok || len(encoded) != 1 {
		return err
	}
	data, decodeErr := base64.StdEncoding.DecodeString(encoded[0])
	if decodeErr != nil {
		return err
	}
	var raw trace.RawTrace
	if unmarshalErr := json.Unmarshal(data, &raw); unmarshalErr != nil {
		return err
	}
	if len(raw.Traces) != 0 && len(raw.Err) != 0 {
		return &trace.TraceErr{Traces: raw.Traces, Err: err, Message: raw.Message}
	}
	return err
}
