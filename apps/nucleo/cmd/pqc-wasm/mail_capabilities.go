//go:build js && wasm

// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"syscall/js"

	"git.sytue.eu/sytue/one/apps/nucleo/mail/limits"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/webport"
)

func init() { asyncDomainOperations["mail.capabilities"] = mailCapabilities }
func mailCapabilities(ctx context.Context, req js.Value) (any, error) {
	port, _, err := webport.AuthorizedJS(ctx, req)
	if err != nil {
		return nil, err
	}
	owner, err := webport.OpenStorage(ctx, port, coreSources, corePolicies)
	if err != nil || owner.FileScope().Product != "mail" || owner.StorageIdentityVersion() != storage.IsolatedIdentity {
		return nil, webport.ErrRejected
	}
	source, err := webport.NewSource(owner, false)
	if err != nil {
		return nil, err
	}
	state, err := source.PrivateStorageState(ctx, owner.FileScope())
	if err != nil || state.Shared || !state.PrivateIngressReady {
		return nil, webport.ErrRejected
	}
	maximum, err := limits.MessageBytes(state.MaxBodyBytes)
	if err != nil {
		return nil, err
	}
	return map[string]any{"v": 1, "scope": map[string]any{"tenant": state.Scope.Tenant, "subject": state.Scope.Subject, "product": state.Scope.Product}, "max_body_bytes": maximum}, nil
}
