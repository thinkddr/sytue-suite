// SPDX-License-Identifier: Apache-2.0
//go:build js && wasm

// Command pqc-wasm exposes the same maintained crypto core to a dedicated web
// worker. It performs no network or persistence and never implements JS crypto.
//
// Es el núcleo COMPLETO (`core`): las operaciones de la bóveda (kernelops) más las
// de Matrix, recursos, búsqueda de Rooms y los puertos del anfitrión. Las apps con
// bóveda arrancan con el núcleo ligero `vault` (cmd/vault-wasm); este solo lo
// descarga quien usa algo de lo demás (docs/decisions.md#0086).
package main

import (
	"syscall/js"

	"git.sytue.eu/sytue/one/apps/nucleo/cmd/internal/kernelops"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
	"git.sytue.eu/sytue/one/apps/nucleo/webport"
)

var domainOperations = map[string]func(js.Value) (any, error){}

func main() {
	webport.Serve(webport.Module{ID: "core", Version: 1, Execute: execute, Async: asyncDomainOperations, RejectedCode: "crypto_rejected", RevokePort: corePortRevocations.Revoke})
}

func bytesArg(v js.Value, maximum int) ([]byte, error) { return webport.BytesArg(v, maximum) }
func bytesValue(raw []byte) js.Value                   { return webport.BytesValue(raw) }
func scopeArg(req js.Value) storage.Scope              { return webport.ScopeArg(req) }
func identityArg(req js.Value) (*pqc.Identity, error)  { return kernelops.IdentityArg(req) }
func integerArg(value js.Value, maximum int) (int, error) {
	return kernelops.IntegerArg(value, maximum)
}

// Primero las operaciones de la bóveda, como antes iban primero en el switch; los
// nombres no se solapan con los de dominio.
func execute(operation string, req js.Value) (any, error) {
	if handler, ok := kernelops.Operations[operation]; ok {
		return handler(req)
	}
	if handler, ok := domainOperations[operation]; ok {
		return handler(req)
	}
	return nil, pqc.ErrEnvelope
}
