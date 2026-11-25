// SPDX-License-Identifier: Apache-2.0
//go:build js && wasm

// Command mail-wasm loads the original Mail application only when Mail opens.
// SDK storage encryption remains owned by the suite's canonical crypto kernel;
// this module uses the same scoped host-port protocol and Go domain sources.
package main

import (
	"context"
	"syscall/js"

	"git.sytue.eu/sytue/one/apps/nucleo/webport"
)

var (
	coreSources           = webport.NewSources()
	corePolicies          = webport.NewPolicies()
	corePortRevocations   = webport.PortRevocations{corePolicies.RevokePort}
	asyncDomainOperations = map[string]func(context.Context, js.Value) (any, error){
		"core.source.read":           coreSources.ReadJS,
		"core.source.finalize":       coreSources.FinalizeJS,
		"core.publication.authorize": corePolicies.AuthorizeJS,
	}
)

// `mail.capabilities` también aquí, y no solo en el núcleo completo: el correo de la
// bóveda tiene este módulo abierto y no necesita arrancar un `core` para preguntarla.
func init() {
	asyncDomainOperations["mail.capabilities"] = func(ctx context.Context, req js.Value) (any, error) {
		return webport.MailCapabilitiesJS(ctx, req, coreSources, corePolicies)
	}
}

func main() {
	webport.Serve(webport.Module{ID: "mail", Version: 1, Execute: func(string, js.Value) (any, error) { return nil, webport.ErrRejected }, Async: asyncDomainOperations, RevokePort: corePortRevocations.Revoke})
}
