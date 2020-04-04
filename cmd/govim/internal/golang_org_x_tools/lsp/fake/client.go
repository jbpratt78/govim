// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fake

import (
	"context"
	"fmt"

	"github.com/govim/govim/cmd/govim/internal/golang_org_x_tools/lsp/protocol"
)

// Client is an adapter that converts a *Client into an LSP Client.
type Client struct {
	*Editor

	// Hooks for testing. Add additional hooks here as needed for testing.
	onLogMessage  func(context.Context, *protocol.LogMessageParams) error
	onDiagnostics func(context.Context, *protocol.PublishDiagnosticsParams) error
}

// OnLogMessage sets the hook to run when the editor receives a log message.
func (c *Client) OnLogMessage(hook func(context.Context, *protocol.LogMessageParams) error) {
	c.mu.Lock()
	c.onLogMessage = hook
	c.mu.Unlock()
}

// OnDiagnostics sets the hook to run when the editor receives diagnostics
// published from the language server.
func (c *Client) OnDiagnostics(hook func(context.Context, *protocol.PublishDiagnosticsParams) error) {
	c.mu.Lock()
	c.onDiagnostics = hook
	c.mu.Unlock()
}

func (c *Client) ShowMessage(ctx context.Context, params *protocol.ShowMessageParams) error {
	c.mu.Lock()
	c.lastMessage = params
	c.mu.Unlock()
	return nil
}

func (c *Client) ShowMessageRequest(ctx context.Context, params *protocol.ShowMessageRequestParams) (*protocol.MessageActionItem, error) {
	return nil, nil
}

func (c *Client) LogMessage(ctx context.Context, params *protocol.LogMessageParams) error {
	c.mu.Lock()
	c.logs = append(c.logs, params)
	onLogMessage := c.onLogMessage
	c.mu.Unlock()
	if onLogMessage != nil {
		return onLogMessage(ctx, params)
	}
	return nil
}

func (c *Client) Event(ctx context.Context, event *interface{}) error {
	c.mu.Lock()
	c.events = append(c.events, event)
	c.mu.Unlock()
	return nil
}

func (c *Client) PublishDiagnostics(ctx context.Context, params *protocol.PublishDiagnosticsParams) error {
	c.mu.Lock()
	c.diagnostics = params
	onPublishDiagnostics := c.onDiagnostics
	c.mu.Unlock()
	if onPublishDiagnostics != nil {
		return onPublishDiagnostics(ctx, params)
	}
	return nil
}

func (c *Client) WorkspaceFolders(context.Context) ([]protocol.WorkspaceFolder, error) {
	return []protocol.WorkspaceFolder{}, nil
}

func (c *Client) Configuration(ctx context.Context, params *protocol.ParamConfiguration) ([]interface{}, error) {
	// We should always receive at least one ConfigurationItem corresponding to
	// the "gopls" Section. The Editor's configuration is associated with that
	// section. Assert that that is the case so that we are forced to make
	// changes here if that situation changes
	minLen := 1
	if l := len(params.Items); l < minLen {
		panic(fmt.Errorf("got %v items; expected at least %v", l, minLen))
	}
	wantSec := "gopls"
	if sec := params.Items[0].Section; sec != "gopls" {
		panic(fmt.Errorf("the first ConfigurationItem has section %q; expected %q", sec, wantSec))
	}
	return []interface{}{c.Editor.configuration()}, nil
}

func (c *Client) RegisterCapability(context.Context, *protocol.RegistrationParams) error {
	return nil
}

func (c *Client) UnregisterCapability(context.Context, *protocol.UnregistrationParams) error {
	return nil
}

func (c *Client) Progress(context.Context, *protocol.ProgressParams) error {
	return nil
}

func (c *Client) WorkDoneProgressCreate(context.Context, *protocol.WorkDoneProgressCreateParams) error {
	return nil
}

// ApplyEdit applies edits sent from the server. Note that as of writing gopls
// doesn't use this feature, so it is untested.
func (c *Client) ApplyEdit(ctx context.Context, params *protocol.ApplyWorkspaceEditParams) (*protocol.ApplyWorkspaceEditResponse, error) {
	if len(params.Edit.Changes) != 0 {
		return &protocol.ApplyWorkspaceEditResponse{FailureReason: "Edit.Changes is unsupported"}, nil
	}
	for _, change := range params.Edit.DocumentChanges {
		path := c.ws.URIToPath(change.TextDocument.URI)
		edits := convertEdits(change.Edits)
		c.EditBuffer(ctx, path, edits)
	}
	return &protocol.ApplyWorkspaceEditResponse{Applied: true}, nil
}
