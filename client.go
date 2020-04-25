package main

import (
	"fmt"
	"net"
	"strings"
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

	// Reading location to load from the server
	// I don't exactly like this solution but it works currently.
	// 255 bytes is the max file name size on most file systems
	recvBuf := make([]byte, 255)
	_, err = conn.Read(recvBuf[:])
	if err != nil {
		return fmt.Errorf("read error: %v", err)
	}

	v.client.wg.Add(1)
	go v.handleClientUpdates()

	// loadLocation
	return v.loadLocation(flags.Mods, string(recvBuf))
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

func (v *vimstate) loadLocation(mods govim.CommModList, file string) error {
	// We expect at most one argument that is the a string value appropriate
	// for &switchbuf. This will need parsing if supplied

	modesStr := v.ParseString(v.ChannelExpr("&switchbuf"))

	var modes []govim.SwitchBufMode
	if modesStr != "" {
		pmodes, err := govim.ParseSwitchBufModes(modesStr)
		if err != nil {
			source := "from Vim setting &switchbuf"
			return fmt.Errorf("got invalid SwitchBufMode setting %v: %q", source, modesStr)
		}
		modes = pmodes
	} else {
		modes = []govim.SwitchBufMode{govim.SwitchBufUseOpen}
	}

	modesMap := make(map[govim.SwitchBufMode]bool)
	for _, m := range modes {
		modesMap[m] = true
	}

	v.ChannelEx("normal! m'")

	vp := v.Viewport()
	tf := strings.TrimPrefix(file, "file://")

	bn := v.ParseInt(v.ChannelCall("bufnr", tf))
	if bn != -1 {
		if vp.Current.BufNr == bn {
			goto MovedToTargetWin
		}
		if modesMap[govim.SwitchBufUseOpen] {
			ctp := vp.Current.TabNr
			for _, w := range vp.Windows {
				if w.TabNr == ctp && w.BufNr == bn {
					v.ChannelCall("win_gotoid", w.WinID)
					goto MovedToTargetWin
				}
			}
		}
		if modesMap[govim.SwitchBufUseTag] {
			for _, w := range vp.Windows {
				if w.BufNr == bn {
					v.ChannelCall("win_gotoid", w.WinID)
					goto MovedToTargetWin
				}
			}
		}
	}
	for _, m := range modes {
		switch m {
		case govim.SwitchBufUseOpen, govim.SwitchBufUseTag:
			continue
		case govim.SwitchBufSplit:
			v.ChannelExf("%v split %v", mods, tf)
		case govim.SwitchBufVsplit:
			v.ChannelExf("%v vsplit %v", mods, tf)
		case govim.SwitchBufNewTab:
			v.ChannelExf("%v tabnew %v", mods, tf)
		}
		goto MovedToTargetWin
	}

	// I _think_ the default behaviour at this point is to use the
	// current window, i.e. simply edit
	v.ChannelExf("%v edit %v", mods, tf)

MovedToTargetWin:

	// now we _must_ have a valid buffer
	bn = v.ParseInt(v.ChannelCall("bufnr", tf))
	if bn == -1 {
		return fmt.Errorf("should have a valid buffer number by this point; we don't")
	}
	_, ok := v.buffers[bn]
	if !ok {
		return fmt.Errorf("should have resolved a buffer; we didn't")
	}

	v.ChannelEx("normal! zz")

	return nil
}
