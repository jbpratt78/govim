package main

import (
	"fmt"
	"net"
	"sync"

	"github.com/govim/govim"
)

type server struct {
	listener net.Listener
	wg       sync.WaitGroup
	quit     chan interface{}
}

func newserver(addr string) *server {

	s := &server{
		quit: make(chan interface{}),
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		panic(err)
	}

	s.listener = ln
	return s
}

func (v *vimstate) startSession(flags govim.CommandFlags, args ...string) error {
	v.Logf("starting session")
	v.ChannelExf(`echom "Starting session"`)

	addr := ":8000"
	if len(args) == 1 {
		addr = args[0]
	}

	v.server = newserver(addr)
	v.server.wg.Add(1)
	go v.serve()

	return nil
}

func (v *vimstate) endSesssion(flags govim.CommandFlags, args ...string) error {
	close(v.server.quit)
	v.server.listener.Close()
	v.server.wg.Wait()
	return nil
}

// TODO: add deadline
func (v *vimstate) serve() {
	defer v.server.wg.Done()

	conn, err := v.server.listener.Accept()
	if err != nil {
		select {
		case <-v.server.quit:
			return
		default:
			panic(err)
		}
	}

	// get current file and send it to connecting client
	cb, _, _ := v.cursorPos()
	if _, err = conn.Write([]byte(cb.URI().Filename())); err != nil {
		panic(fmt.Errorf("failed to write filename for client loading location: %v", err))
	}

	v.server.wg.Add(1)
	go v.handleServerUpdates(conn)
}

func (v *vimstate) handleServerUpdates(conn net.Conn) {
	defer v.server.wg.Done()

	for event := range v.bufferUpdates {
		_, err := conn.Write(event.contents)
		if err != nil {
			v.Logf(err.Error())
		}
	}
}
