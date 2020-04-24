package main

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/jbpratt78/vimcollab/internal/plugin"
	"github.com/jbpratt78/vimcollab/internal/types"
)

type vimstate struct {
	plugin.Driver
	*govimplugin

	// buffers represents the current state of all buffers in Vim. It is only safe to
	// write and read to/from this map in the callback for a defined function, command
	// or autocommand.
	buffers map[int]*types.Buffer

	configLock sync.Mutex

	// userBusy indicates the user is moving the cusor doing something
	userBusy bool

	// currBatch represents the batch we are collecting
	currBatch *batch

	// working directory (when govim was started)
	// TODO: handle changes to current working directory during runtime
	workingDirectory string

	// tcp server
	server *server

	client *client
}

func (v *vimstate) setUserBusy(args ...json.RawMessage) (interface{}, error) {
	v.userBusy = v.ParseInt(args[0]) != 0
	var pos cursorPosition
	v.Parse(args[1], &pos)
	return nil, nil
}

type batch struct {
	calls   []interface{}
	results []json.RawMessage
}

func (b *batch) result() batchResult {
	i := len(b.calls) - 1
	return func() json.RawMessage {
		if b.results == nil {
			panic(fmt.Errorf("tried to get result from incomplete Batch"))
		}
		return b.results[i]
	}
}

func (v *vimstate) BatchStart() {
	if v.currBatch != nil {
		panic(fmt.Errorf("called BatchStart whilst in a batch"))
	}
	v.currBatch = &batch{}
}

func (v *vimstate) BatchStartIfNeeded() bool {
	if v.currBatch != nil {
		return false
	}
	v.currBatch = &batch{}
	return true
}

type batchResult func() json.RawMessage

type AssertExpr struct {
	Fn   string
	Args []interface{}
}

func AssertNoError() AssertExpr {
	return AssertExpr{
		Fn: "s:mustNoError",
	}
}

func AssertIsZero() AssertExpr {
	return AssertExpr{
		Fn: "s:mustBeZero",
	}
}

func AssertIsErrorOrNil(patterns ...string) AssertExpr {
	args := make([]interface{}, 0, len(patterns))
	for _, v := range patterns {
		args = append(args, v)
	}
	return AssertExpr{
		Fn:   "s:mustBeErrorOrNil",
		Args: args,
	}
}

func (v *vimstate) BatchChannelExprf(format string, args ...interface{}) batchResult {
	return v.BatchAssertChannelExprf(AssertNoError(), format, args...)
}

func (v *vimstate) BatchAssertChannelExprf(a AssertExpr, format string, args ...interface{}) batchResult {
	if v.currBatch == nil {
		panic(fmt.Errorf("cannot call BatchChannelExprf: not in batch"))
	}
	v.currBatch.calls = append(v.currBatch.calls, []interface{}{
		"expr",
		[2]interface{}{a.Fn, a.Args},
		fmt.Sprintf(format, args...),
	})
	return v.currBatch.result()
}
func (v *vimstate) BatchChannelCall(name string, args ...interface{}) batchResult {
	return v.BatchAssertChannelCall(AssertNoError(), name, args...)
}

func (v *vimstate) BatchAssertChannelCall(a AssertExpr, name string, args ...interface{}) batchResult {
	if v.currBatch == nil {
		panic(fmt.Errorf("cannot call BatchChannelCall: not in batch"))
	}
	callargs := []interface{}{
		"call",
		[2]interface{}{a.Fn, a.Args},
		name,
	}
	callargs = append(callargs, args...)
	v.currBatch.calls = append(v.currBatch.calls, callargs)
	return v.currBatch.result()
}

func (v *vimstate) BatchCancelIfNotEnded() {
	v.currBatch = nil
}

func (v *vimstate) BatchEnd() ([]json.RawMessage, error) {
	return v.batchEndImpl(false)
}

func (v *vimstate) MustBatchEnd() (res []json.RawMessage) {
	res, _ = v.batchEndImpl(true)
	return
}

func (v *vimstate) batchEndImpl(must bool) (res []json.RawMessage, err error) {
	if v.currBatch == nil {
		panic(fmt.Errorf("called BatchEnd but not in a batch"))
	}
	b := v.currBatch
	v.currBatch = nil
	if len(b.calls) == 0 {
		return
	}
	var vs json.RawMessage
	if must {
		vs = v.ChannelCall("s:batchCall", b.calls)
	} else {
		vs, err = v.Driver.Govim.ChannelCall("s:batchCall", b.calls)
		if err != nil {
			return
		}
	}
	v.Parse(vs, &res)
	b.results = res
	return
}

func (v *vimstate) ChannelCall(name string, args ...interface{}) json.RawMessage {
	if v.currBatch != nil {
		panic(fmt.Errorf("called ChannelCall when in batch"))
	}
	return v.Driver.ChannelCall(name, args...)
}

func (v *vimstate) ChannelEx(expr string) {
	if v.currBatch != nil {
		panic(fmt.Errorf("called ChannelEx when in batch"))
	}
	v.Driver.ChannelEx(expr)
}

func (v *vimstate) ChannelExf(format string, args ...interface{}) {
	if v.currBatch != nil {
		panic(fmt.Errorf("called ChannelExf when in batch"))
	}
	v.Driver.ChannelExf(format, args...)
}

func (v *vimstate) ChannelExpr(expr string) json.RawMessage {
	if v.currBatch != nil {
		panic(fmt.Errorf("called ChannelExpr when in batch"))
	}
	return v.Driver.ChannelExpr(expr)
}

func (v *vimstate) ChannelExprf(format string, args ...interface{}) json.RawMessage {
	if v.currBatch != nil {
		panic(fmt.Errorf("called ChannelExprf when in batch"))
	}
	return v.Driver.ChannelExprf(format, args...)
}

func (v *vimstate) ChannelNormal(expr string) {
	if v.currBatch != nil {
		panic(fmt.Errorf("called ChannelNormal when in batch"))
	}
	v.Driver.ChannelNormal(expr)
}

func (v *vimstate) ChannelRedraw(force bool) {
	if v.currBatch != nil {
		panic(fmt.Errorf("called ChannelRedraw when in batch"))
	}
	v.Driver.ChannelRedraw(force)
}
