package trace

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/codingllama/semerr"
)

// WriteError sets up HTTP error response and writes it to writer w
func WriteError(w http.ResponseWriter, err error) {
	if !IsAggregate(err) {
		replyJSON(w, ErrorToCode(err), err)
		return
	}
	for i := 0; i < maxHops; i++ {
		var aggErr Aggregate
		var ok bool
		if aggErr, ok = Unwrap(err).(Aggregate); !ok {
			break
		}
		errors := aggErr.Errors()
		if len(errors) == 0 {
			break
		}
		err = errors[0]
	}
	replyJSON(w, ErrorToCode(err), err)
}

// ErrorToCode returns an appropriate HTTP status code based on the provided error type
func ErrorToCode(err error) int {
	status, ok := semerr.HTTPStatus(err)
	if ok {
		return status
	}

	if IsAggregate(err) {
		return http.StatusGatewayTimeout
	}

	return status // 500/Internal.
}

// ReadError converts http error to internal error type
// based on HTTP response code and HTTP body contents
// if status code does not indicate error, it will return nil
func ReadError(statusCode int, respBytes []byte) error {
	if statusCode >= http.StatusOK && statusCode < http.StatusBadRequest {
		return nil
	}
	var err error
	switch statusCode {
	case http.StatusNotFound:
		err = &NotFoundError{}
	case http.StatusBadRequest:
		err = &BadParameterError{}
	case http.StatusNotImplemented:
		err = &NotImplementedError{}
	case http.StatusPreconditionFailed:
		err = &CompareFailedError{}
	case http.StatusForbidden:
		err = &AccessDeniedError{}
	case http.StatusConflict:
		err = &AlreadyExistsError{}
	case http.StatusTooManyRequests:
		err = &LimitExceededError{}
	case http.StatusGatewayTimeout:
		err = &ConnectionProblemError{}
	default:
		err = &RawTrace{}
	}
	return wrapProxy(unmarshalError(err, respBytes))
}

func replyJSON(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	var out []byte
	// wrap regular errors in order to achieve unification
	// and provide structurally consistent responses
	obj, ok := err.(*TraceErr)
	if !ok {
		obj = &TraceErr{Err: err}
	}
	out, err = json.MarshalIndent(obj, "", "    ")
	if err != nil {
		out = []byte(fmt.Sprintf(`{"error": {"message": "internal marshal error: %v"}}`, err))
	}
	_, _ = w.Write(out)
}

func unmarshalError(err error, responseBody []byte) error {
	if len(responseBody) == 0 {
		return err
	}
	var raw RawTrace
	if err2 := json.Unmarshal(responseBody, &raw); err2 != nil {
		return errorOnInvalidJSON(err, responseBody)
	}
	if len(raw.Err) != 0 {
		if err2 := json.Unmarshal(raw.Err, err); err2 != nil {
			return errorOnInvalidJSON(err, responseBody)
		}
		return &TraceErr{
			Traces:   raw.Traces,
			Err:      err,
			Message:  raw.Message,
			Messages: raw.Messages,
			Fields:   raw.Fields,
		}
	}
	if err2 := json.Unmarshal(responseBody, err); err2 != nil {
		return errorOnInvalidJSON(err, responseBody)
	}
	return err
}

// errorOnInvalidJSON is used to construct a TraceErr with the
// input error as Err and the responseBody as Messages.
// This function is used when the responseBody is not valid
// JSON or it contains an unexpected JSON.
func errorOnInvalidJSON(err error, responseBody []byte) error {
	return &TraceErr{
		Err:      err,
		Messages: []string{string(responseBody)},
	}
}
