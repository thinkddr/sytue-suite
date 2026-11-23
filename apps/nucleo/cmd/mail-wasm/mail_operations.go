//go:build js && wasm

// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"syscall/js"

	webMail "git.sytue.eu/sytue/one/apps/nucleo/mail/web"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/webport"
)

var coreMail = func() *webMail.Manager {
	manager, err := webMail.NewManager(coreSources, corePolicies)
	if err != nil {
		panic("invalid Mail host registries")
	}
	return manager
}()

func init() {
	asyncDomainOperations["mail.open"] = mailOpen
	asyncDomainOperations["mail.pending"] = mailPending
	asyncDomainOperations["mail.pause"] = mailPause
	asyncDomainOperations["mail.resume"] = mailResume
	asyncDomainOperations["mail.call"] = mailCall
	asyncDomainOperations["mail.compose"] = mailCompose
	corePortRevocations = append(corePortRevocations, coreMail.ClosePort)
}

func mailInput(req js.Value, maximum int, out any) error {
	raw, err := webport.BytesArg(req.Get("input"), maximum)
	if err != nil {
		return err
	}
	defer clear(raw)
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(out) != nil || decoder.Decode(new(any)) != io.EOF {
		return webport.ErrRejected
	}
	return nil
}

func mailScope(scope storage.Scope) map[string]any {
	return map[string]any{"tenant": scope.Tenant, "subject": scope.Subject, "product": scope.Product}
}

func mailOpen(ctx context.Context, req js.Value) (any, error) {
	port, identity, err := webport.AuthorizedJS(ctx, req)
	if err != nil || identity.Scope.Product != "mail" {
		return nil, webport.ErrRejected
	}
	var input struct {
		Legacy string `json:"legacy_port"`
		Writer string `json:"migration_writer_port"`
		// El anfitrión elige el almacén: "" (Matrix, lo de siempre) o "vault".
		Store string `json:"store"`
	}
	if err = mailInput(req, 8192, &input); err != nil {
		return nil, err
	}
	store := webMail.StoreMatrix
	switch input.Store {
	case "":
	case "vault":
		store = webMail.StoreVault
	default:
		return nil, webport.ErrRejected
	}
	ports := webMail.Ports{Isolated: port}
	if input.Legacy != "" {
		ports.LegacyReader, err = webport.NewJS(input.Legacy)
		if err != nil {
			return nil, err
		}
	}
	if input.Writer != "" {
		ports.MigrationWriter, err = webport.NewJS(input.Writer)
		if err != nil {
			return nil, err
		}
	}
	session, err := coreMail.OpenStore(ctx, ports, store)
	if err != nil {
		return nil, err
	}
	return map[string]any{"session": session.ID(), "scope": mailScope(session.Scope()), "ready": true}, nil
}

func mailSession(ctx context.Context, req js.Value) (*webMail.Session, error) {
	port, identity, err := webport.AuthorizedJS(ctx, req)
	if err != nil {
		return nil, err
	}
	var input struct {
		Session string `json:"session"`
	}
	if err = mailInput(req, 4096, &input); err != nil {
		return nil, err
	}
	session, err := coreMail.Lookup(ctx, input.Session, port.ID())
	if err != nil {
		return nil, err
	}
	if session.Scope() != identity.Scope {
		return nil, webport.ErrRejected
	}
	return session, nil
}

func mailPending(ctx context.Context, req js.Value) (any, error) {
	session, err := mailSession(ctx, req)
	if err != nil {
		return nil, err
	}
	value, err := session.Pending(ctx)
	if err != nil {
		return nil, err
	}
	if value.Scope != session.Scope() || value.Count < 0 {
		return nil, webport.ErrRejected
	}
	return map[string]any{"scope": mailScope(value.Scope), "count": value.Count}, nil
}

func mailPause(ctx context.Context, req js.Value) (any, error) {
	session, err := mailSession(ctx, req)
	if err != nil {
		return nil, err
	}
	if err = session.Pause(ctx); err != nil {
		return nil, err
	}
	return map[string]any{"paused": true}, nil
}

func mailResume(ctx context.Context, req js.Value) (any, error) {
	session, err := mailSession(ctx, req)
	if err != nil {
		return nil, err
	}
	if err = session.Resume(ctx); err != nil {
		return nil, err
	}
	return map[string]any{"ready": true}, nil
}

func mailCall(ctx context.Context, req js.Value) (any, error) {
	port, identity, err := webport.AuthorizedJS(ctx, req)
	if err != nil {
		return nil, err
	}
	var input struct {
		Session string          `json:"session"`
		Action  string          `json:"action"`
		Params  json.RawMessage `json:"params"`
	}
	if err = mailInput(req, 2<<20, &input); err != nil {
		return nil, err
	}
	session, err := coreMail.Lookup(ctx, input.Session, port.ID())
	if err != nil {
		return nil, err
	}
	if session.Scope() != identity.Scope {
		return nil, webport.ErrRejected
	}
	var payload []byte
	if value := req.Get("payload"); value.Type() != js.TypeUndefined {
		payload, err = webport.BytesArg(value, int(^uint(0)>>1))
		if err != nil {
			return nil, err
		}
		defer clear(payload)
	}
	var result map[string]any
	err = session.Perform(ctx, webMail.Call{Action: input.Action, Params: input.Params}, payload, func(reply webMail.Reply) error {
		result = map[string]any{"data": webport.BytesValue(reply.Data)}
		if reply.Body != nil {
			result["body"] = webport.BytesValue(reply.Body)
		}
		return nil
	})
	if err != nil {
		for _, value := range result {
			if bytes, ok := value.(js.Value); ok {
				bytes.Call("fill", 0)
			}
		}
		return nil, err
	}
	return result, nil
}

// Larger attachments travel as an owned byte frame, bounded by the SAME native
// Mail capability. This avoids imposing mail.call's metadata limit on MIME.
func mailCompose(ctx context.Context, req js.Value) (any, error) {
	session, err := mailSession(ctx, req)
	if err != nil {
		return nil, err
	}
	maximum, err := session.ComposeInputLimit(ctx)
	if err != nil {
		return nil, err
	}
	raw, err := webport.BytesArg(req.Get("payload"), maximum)
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	var result map[string]any
	err = session.Perform(ctx, webMail.Call{Action: "mime.compose", Params: json.RawMessage(raw)}, nil, func(reply webMail.Reply) error {
		result = map[string]any{"data": webport.BytesValue(reply.Data), "body": webport.BytesValue(reply.Body)}
		return nil
	})
	if err != nil {
		for _, value := range result {
			if bytes, ok := value.(js.Value); ok {
				bytes.Call("fill", 0)
			}
		}
		return nil, err
	}
	return result, nil
}
