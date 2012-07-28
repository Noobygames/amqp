// Copyright (c) 2012, Sean Treadway, SoundCloud Ltd.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
// Source code and contact info at http://github.com/streadway/amqp

package amqp

import (
	"bytes"
	"io"
	"reflect"
	"testing"
	"time"
)

type server struct {
	*testing.T
	r reader        // framer <- client
	w writer        // framer -> client
	S io.ReadWriter // Server IO
	C io.ReadWriter // Client IO
}

func defaultConfig() Config {
	return Config{SASL: []Authentication{&PlainAuth{"guest", "guest"}}, Vhost: "/"}
}

func newSession(t *testing.T) (io.ReadWriteCloser, *server) {
	type pipe struct {
		io.Reader
		io.WriteCloser
	}

	rs, wc := io.Pipe()
	rc, ws := io.Pipe()

	rws := &logIO{t, "server", pipe{rs, ws}}
	rwc := &logIO{t, "client", pipe{rc, wc}}

	server := server{
		T: t,
		r: reader{rws},
		w: writer{rws},
		S: rws,
		C: rwc,
	}

	return rwc, &server
}

	if _, ok = f.(*methodFrame).Method.(*connectionTuneOk); !ok {
		t.Fatalf("expected ConnectionTuneOk")
	}

func (t *server) expectBytes(b []byte) {
	in := make([]byte, len(b))
	if _, err := io.ReadFull(t.S, in); err != nil {
		t.Fatalf("io error expecting bytes: %v", err)
	}

	if bytes.Compare(b, in) != 0 {
		t.Fatalf("failed bytes: expected: %s got: %s", string(b), string(in))
	}
}

func (t *server) send(channel int, m message) {
	defer time.AfterFunc(10*time.Millisecond, func() { panic("send deadlock") }).Stop()

	if err := t.w.WriteFrame(&methodFrame{
		ChannelId: uint16(channel),
		Method:    m,
	}); err != nil {
		t.Fatalf("frame err, write: %s", err)
	}
}

// Currently drops all but method frames expected on the given channel
func (t *server) recv(channel int, m message) message {
	defer time.AfterFunc(10*time.Millisecond, func() { panic("recv deadlock") }).Stop()

	var remaining int
	var header *headerFrame
	var body []byte

	for {
		frame, err := t.r.ReadFrame()
		if err != nil {
			t.Fatalf("frame err, read: %s", err)
		}

		if frame.channel() != uint16(channel) {
			t.Fatalf("expected frame on channel %d, got channel %d", channel, frame.channel())
		}

		switch f := frame.(type) {
		case *heartbeatFrame:
			// drop

		case *headerFrame:
			// start content state
			header = f
			remaining = int(header.Size)

		case *bodyFrame:
			// continue until terminated
			body = append(body, f.Body...)
			remaining -= len(f.Body)
			if remaining <= 0 {
				m.(messageWithContent).setContent(header.Properties, body)
				return m
			}

		case *methodFrame:
			if reflect.TypeOf(m) == reflect.TypeOf(f.Method) {
				wantv := reflect.ValueOf(m).Elem()
				havev := reflect.ValueOf(f.Method).Elem()
				wantv.Set(havev)
				if _, ok := m.(messageWithContent); !ok {
					return m
				}
			} else {
				t.Fatalf("expected method type: %T, got: %T", m, f.Method)
			}
		}
	}

	panic("unreachable")
}

func (t *server) expectAMQP() {
	t.expectBytes([]byte{'A', 'M', 'Q', 'P', 0, 0, 9, 1})
}

func (t *server) connectionOpen() {
	t.expectAMQP()

	t.send(0, &connectionStart{
		VersionMajor: 0,
		VersionMinor: 9,
		Mechanisms:   "PLAIN",
		Locales:      "en-us",
	})

	t.recv(0, &connectionStartOk{})

	t.send(0, &connectionTune{
		ChannelMax: 11,
		FrameMax:   20000,
		Heartbeat:  10,
	})

	t.recv(0, &connectionTuneOk{})
	t.recv(0, &connectionOpen{})
	t.send(0, &connectionOpenOk{})
}

func (t *server) connectionClose() {
	t.recv(0, &connectionClose{})
	t.send(0, &connectionCloseOk{})
}

func (t *server) channelOpen(id int) {
	t.recv(id, &channelOpen{})
	t.send(id, &channelOpenOk{})
}

func TestNewConnectionOpen(t *testing.T) {
	rwc, srv := newSession(t)
	go srv.connectionOpen()

	if c, err := NewConnection(rwc, defaultConfig()); err != nil {
		t.Fatalf("could not create connection: %s (%s)", c, err)
	}
}

func TestNewConnectionChannelOpen(t *testing.T) {
	rwc, srv := newSession(t)

	go func() {
		srv.connectionOpen()
		srv.channelOpen(1)
	}()

	c, err := NewConnection(rwc, defaultConfig())
	if err != nil {
		t.Fatalf("could not create connection: %s (%s)", c, err)
	}

	go driveChannelOpen(t, server)

	ch, err := c.Channel()
	if err != nil {
		t.Fatalf("could not open channel: %s (%s)", ch, err)
	}
}