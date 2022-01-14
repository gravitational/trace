/*
Copyright 2015 Gravitational, Inc.

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
	"io/ioutil"
	"testing"

	"github.com/jonboulle/clockwork"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/suite"
)

type HooksSuite struct {
	suite.Suite
}

func TestHooks(t *testing.T) {
	suite.Run(t, new(HooksSuite))
}

func (s *HooksSuite) TestSafeForConcurrentAccess() {
	logger := log.New()
	logger.Out = ioutil.Discard
	entry := logger.WithFields(log.Fields{"foo": "bar"})
	logger.Hooks.Add(&UDPHook{Clock: clockwork.NewFakeClock()})
	for i := 0; i < 3; i++ {
		go func(entry *log.Entry) {
			for i := 0; i < 1000; i++ {
				entry.Infof("test")
			}
		}(entry)
	}
}
