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
	"net"
	"sync"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	log "github.com/sirupsen/logrus"
	. "gopkg.in/check.v1"
)

func TestHooks(t *testing.T) { TestingT(t) }

type HooksSuite struct{}

var _ = Suite(&HooksSuite{})

func (s *HooksSuite) TestSafeForConcurrentAccess(c *C) {
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

func TestClientNet(t *testing.T) {
	type args struct {
		clientNet string
	}
	tests := []struct {
		name string
		clientNet string
		want string
	}{
		{"Empty value", "", "", },
		{"UDP", "udp", "udp", },
		{"TCP", "tcp", "tcp", },
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := ClientNet(tt.clientNet)
			hook := UDPHook{}
			f(&hook)

			if hook.clientNet!= tt.want /*!reflect.DeepEqual(got, tt.want) */{
				t.Errorf("ClientNet() = %v, want %v", hook.clientNet, tt.want)
			}
		})
	}
}

func TestClientAddr(t *testing.T) {
	type args struct {
		clientNet string
	}
	tests := []struct {
		name string
		clientAddr string
		want string
	}{
		{"Empty value", "", "", },
		{"Localhost and another port", "127.0.0.1:9999", "127.0.0.1:9999", },
		{"Another host", "192.168.0.1:9999", "192.168.0.1:9999", },
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := ClientAddr(tt.clientAddr)
			hook := UDPHook{}
			f(&hook)

			if hook.clientAddr != tt.want /*!reflect.DeepEqual(got, tt.want) */{
				t.Errorf("ClientAddr() = %v, want %v", hook.clientNet, tt.want)
			}
		})
	}
}

func TestUDPHook_Fire(t *testing.T) {
	remoteAddr := "127.0.0.1:5501"
	raddr, err := net.ResolveUDPAddr("udp", remoteAddr)
	UDPConn, err := net.ListenUDP("udp", raddr)
	if err != nil {
		t.Errorf("Cannot listen given address %v, error: %v", remoteAddr, err)
		return
	}
	defer UDPConn.Close()

	var ok bool

	var (
		wg sync.WaitGroup
		bytesRead int
		serverReadError error
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		var b = make([]byte, 1024)
		UDPConn.SetDeadline(time.Now().Add(100*time.Millisecond))
		bytesRead, _, serverReadError = UDPConn.ReadFromUDP(b)
		if bytesRead > 0 {
			ok = true
		}
		serverReadError = err
	}()

	hook, err := NewUDPHook(ClientAddr(remoteAddr+""))
	logger := log.New()
	entry := log.NewEntry(logger)

	err = hook.Fire(entry)
	if err != nil {
		t.Errorf("Fire() error: %v", err)
	}
	wg.Wait()
	if !ok {
		t.Errorf("UDP server read error: %v. Bytes read: %v", serverReadError, bytesRead)
	}
}