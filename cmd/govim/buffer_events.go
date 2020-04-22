package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/jbpratt78/vimcollab/cmd/govim/config"
	"github.com/jbpratt78/vimcollab/cmd/govim/internal/types"
)

func (v *vimstate) bufReadPost(args ...json.RawMessage) error {
	nb := v.currentBufferInfo(args[0])

	if cb, ok := v.buffers[nb.Num]; ok {
		// reload of buffer, e.v. e!
		cb.Loaded = nb.Loaded

		// If the contents are the same we probably just re-loaded a currently
		// unloaded buffer.  We shouldn't increase version in that case, but we
		// have to re-place signs and redefine highlights since text properties
		// are removed when a buffer is unloaded.
		if bytes.Equal(nb.Contents(), cb.Contents()) {
			return nil
		}
		cb.SetContents(nb.Contents())
		cb.Version++
		return v.handleBufferEvent(cb)
	}

	v.buffers[nb.Num] = nb
	nb.Version = 1
	nb.Listener = v.ParseInt(v.ChannelCall("listener_add", v.Prefix()+string(config.FunctionEnrichDelta), nb.Num))

	return v.handleBufferEvent(nb)
}

type bufChangedChange struct {
	Lnum  int      `json:"lnum"`
	Col   int      `json:"col"`
	Added int      `json:"added"`
	End   int      `json:"end"`
	Type  string   `json:"type"`
	Lines []string `json:"lines"`
}

// bufChanged is fired as a result of the listener_add callback for a buffer; it is mutually
// exclusive with bufTextChanged. args are:
//
// bufChanged(bufnr, start, end, added, changes)
//
func (v *vimstate) bufChanged(args ...json.RawMessage) (interface{}, error) {
	bufnr := v.ParseInt(args[0])
	b, ok := v.buffers[bufnr]
	if !ok {
		return nil, fmt.Errorf("failed to resolve buffer %v in bufChanged callback", bufnr)
	}
	var changes []bufChangedChange
	v.Parse(args[4], &changes)
	if len(changes) == 0 {
		v.Logf("bufChanged: no changes to apply for %v", b.Name)
		return nil, nil
	}

	contents := bytes.Split(b.Contents()[:len(b.Contents())-1], []byte("\n"))
	b.Version++

	for _, c := range changes {
		var newcontents [][]byte

		newcontents = append(newcontents, contents[:c.Lnum-1]...)
		for _, l := range c.Lines {
			newcontents = append(newcontents, []byte(l))
		}
		if len(c.Lines) > 0 {
		}
		newcontents = append(newcontents, contents[c.End-1:]...)

		contents = newcontents
	}
	// add back trailing newline
	b.SetContents(append(bytes.Join(contents, []byte("\n")), '\n'))
	v.triggerBufferASTUpdate(b)
	return nil, nil
}

func (v *vimstate) bufUnload(args ...json.RawMessage) error {
	bufnr := v.ParseInt(args[0])
	if _, ok := v.buffers[bufnr]; !ok {
		return nil
	}
	v.buffers[bufnr].Loaded = false
	return nil
}

func (v *vimstate) handleBufferEvent(b *types.Buffer) error {
	v.triggerBufferASTUpdate(b)
	if b.Version == 1 {
		return nil
	}

	return nil
}

func (v *vimstate) deleteCurrentBuffer(args ...json.RawMessage) error {
	currBufNr := v.ParseInt(args[0])
	cb, ok := v.buffers[currBufNr]
	if !ok {
		return fmt.Errorf("tried to remove buffer %v; but we have no record of it", currBufNr)
	}

	v.ChannelCall("listener_remove", cb.Listener)
	delete(v.buffers, cb.Num)

	return nil
}

func (v *vimstate) bufWritePost(args ...json.RawMessage) error {
	currBufNr := v.ParseInt(args[0])
	_, ok := v.buffers[currBufNr]
	if !ok {
		return fmt.Errorf("tried to handle BufWritePost for buffer %v; but we have no record of it", currBufNr)
	}
	return nil
}

type bufferUpdate struct {
	buffer   *types.Buffer
	wait     chan bool
	name     string
	version  int
	contents []byte
}

func (g *govimplugin) startProcessBufferUpdates() {
	g.bufferUpdates = make(chan *bufferUpdate)
	g.tomb.Go(func() error {
		latest := make(map[*types.Buffer]int)
		var lock sync.Mutex
		for upd := range g.bufferUpdates {
			upd := upd
			lock.Lock()
			latest[upd.buffer] = upd.version
			lock.Unlock()

			// Note we are not restricting the number of concurrent parses here.
			// This is simply because we are unlikely to ever get a sufficiently
			// high number of concurrent updates from Vim to make this necessary.
			// Like the Vim <-> govim <-> gopls "channel" would get
			// flooded/overloaded first
			g.tomb.Go(func() error {
				lock.Lock()
				if latest[upd.buffer] == upd.version {
					delete(latest, upd.buffer)
				}
				lock.Unlock()
				close(upd.wait)
				return nil
			})
		}
		return nil
	})
}

func (v *vimstate) triggerBufferASTUpdate(b *types.Buffer) {
	b.ASTWait = make(chan bool)
	v.bufferUpdates <- &bufferUpdate{
		buffer:   b,
		wait:     b.ASTWait,
		name:     b.Name,
		version:  b.Version,
		contents: b.Contents(),
	}
}
