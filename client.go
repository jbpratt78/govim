package main

import (
	"fmt"
	"net"
	"sync"

	"github.com/govim/govim"
)

type client struct {
	conn net.Conn
	wg   sync.WaitGroup
}

func (v *vimstate) joinSession(flags govim.CommandFlags, args ...string) error {
	v.ChannelExf(`echom "Joining session"`)

	if len(args) > 1 {
		return fmt.Errorf("must supply an address to dial")
	}

	v.client = &client{}

	// TODO: timeout
	conn, err := net.Dial("tcp", args[0])
	if err != nil {
		return fmt.Errorf("failed to dial server: %v", err)
	}

	v.client.conn = conn
	v.client.wg.Add(1)
	go v.handleClientUpdates()

	return nil
}

func (v *vimstate) leaveSession(flags govim.CommandFlags, args ...string) error {
	v.client.conn.Close()
	v.client.wg.Wait()
	return nil
}

func (v *vimstate) handleClientUpdates() {
	defer v.client.wg.Done()
	v.ChannelExf(`echom "Session connected"`)
	for {
		reply := make([]byte, 1024)
		_, err := v.client.conn.Read(reply)
		if err != nil {
			v.ChannelExf(`echom "Connection closed"`)
			v.Logf("failed to read from conn: %v", err)
			return
		}

		v.Logf(string(reply))
	}
}
