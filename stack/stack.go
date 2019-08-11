/*
Copyright 2019 Gravitational, Inc.

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

package stack

import (
	"context"
	"encoding/json"
	"fmt"

	"cloud.google.com/go/logging"
	"github.com/gravitational/trace"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/option"
	logpb "google.golang.org/genproto/googleapis/logging/v2"
)

// Hook is a stackdriver logrus hook,
// this hook implements direct async logging
// to stackdriver
type Hook struct {
	client *logging.Client
	logger *logging.Logger
	ctx    context.Context
}

// Config specifies stackdriver logrus hook configuration
type Config struct {
	// Context is a global context to
	// cancel all hook operations
	Context context.Context
	// Creds contains credentials to authenticate
	// with the stackdriver
	Creds []byte
	// ServiceName is a service name to log
	ServiceName string
}

// CheckAndSetDefaults checks and sets default values
func (c *Config) CheckAndSetDefaults() error {
	if c.Context == nil {
		return trace.BadParameter("missing parameter Context")
	}
	if len(c.Creds) == 0 {
		return trace.BadParameter("missing parameter Creds")
	}
	if c.ServiceName == "" {
		c.ServiceName = DefaultServiceName
	}
	return nil
}

// NewHook returns a new stackdriver hook
func NewHook(cfg Config) (*Hook, error) {
	if err := cfg.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}

	type creds struct {
		ProjectID string `json:"project_id"`
	}
	var cred creds
	err := json.Unmarshal(cfg.Creds, &cred)
	if err != nil {
		return nil, trace.BadParameter(
			"failed to parse credentials, unsupported format, expected JSON: %v", err)
	}
	if cred.ProjectID == "" {
		return nil, trace.BadParameter("credentials are missing project id")
	}
	client, err := logging.NewClient(
		cfg.Context, cred.ProjectID, option.WithCredentialsJSON(cfg.Creds))
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return &Hook{
		ctx:    cfg.Context,
		client: client,
		logger: client.Logger(cfg.ServiceName),
	}, nil
}

// Fire is invoked by logrus and sends log to Stackdriver.
func (h *Hook) Fire(entry *logrus.Entry) error {
	if entry == nil {
		return trace.BadParameter("missing parameter entry")
	}
	h.logger.Log(convertEntry(entry))
	return nil
}

// Levels returns hook's registered logging levels
func (h *Hook) Levels() []logrus.Level {
	return logrus.AllLevels
}

// DefaultServiceName is a default service name
const DefaultServiceName = "default"

func convertEntry(entry *logrus.Entry) logging.Entry {
	e := logging.Entry{
		Timestamp: entry.Time,
		Severity:  convertLevel(entry.Level),
		Labels:    convertLabels(entry.Data),
	}
	err, ok := extractError(entry)
	if ok {
		e.Payload = entry.Message + " " + trace.UserMessage(err)
		traceErr, ok := err.(*trace.TraceErr)
		if ok && len(traceErr.Traces) > 0 {
			t := traceErr.Traces[0]
			e.SourceLocation = &logpb.LogEntrySourceLocation{
				File:     t.Path,
				Line:     int64(t.Line),
				Function: t.Func,
			}
		}
		if len(traceErr.Fields) > 0 {
			for key, val := range convertLabels(traceErr.Fields) {
				e.Labels[key] = val
			}
		}

	} else {
		e.Payload = entry.Message
	}
	return e
}

func extractError(entry *logrus.Entry) (error, bool) {
	errI, ok := entry.Data[logrus.ErrorKey]
	if !ok {
		return nil, false
	}
	err, ok := errI.(error)
	if !ok {
		return nil, false
	}
	return err, true
}

func convertLabels(data map[string]interface{}) map[string]string {
	labels := make(map[string]string, len(data))
	for k, v := range data {
		// do not format error as a label
		if k == logrus.ErrorKey {
			continue
		}
		switch x := v.(type) {
		case string:
			labels[k] = x
		default:
			labels[k] = fmt.Sprintf("%v", v)
		}
	}
	return labels
}

func convertLevel(level logrus.Level) logging.Severity {
	switch level {
	case logrus.TraceLevel, logrus.DebugLevel:
		return logging.Debug
	case logrus.InfoLevel:
		return logging.Info
	case logrus.WarnLevel:
		return logging.Warning
	case logrus.ErrorLevel, logrus.FatalLevel, logrus.PanicLevel:
		return logging.Error
	default:
		return logging.Default
	}
}
