//go:build js && wasm

// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"syscall/js"

	"git.sytue.eu/sytue/one/apps/nucleo/webport"
)

// Se queda también aquí mientras el correo de la bóveda la pida a un `core` aparte
// (packages/offline/src/vault/mail-private.ts); el módulo `mail` ya la sirve.
func init() {
	asyncDomainOperations["mail.capabilities"] = func(ctx context.Context, req js.Value) (any, error) {
		return webport.MailCapabilitiesJS(ctx, req, coreSources, corePolicies)
	}
}
