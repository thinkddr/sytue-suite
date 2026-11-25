// SPDX-License-Identifier: Apache-2.0
//go:build js && wasm

package kernelops

import (
	"encoding/json"
	"syscall/js"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/vaultproof"
)

// Las acciones de entrada del correo sobre la bóveda (ADR 0087): el módulo de
// correo pide firmar `{expected, action}` por el puerto `ingress.action.sign`, y
// el navegador pasa esos bytes tal cual, con la clave de firma de la bóveda, a
// esta operación. El navegador no interpreta la acción ni firma nada.
func init() {
	Operations["vault.ingress.sign"] = vaultIngressSign
}

const maxVaultIngressRequest = 16 << 10

func vaultIngressSign(req js.Value) (any, error) {
	signer, err := vaultSignerArg(req)
	if err != nil {
		return nil, err
	}
	raw, err := bytesArg(req.Get("request"), maxVaultIngressRequest)
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	request, err := vaultproof.DecodeSignRequest(raw)
	if err != nil {
		return nil, err
	}
	proof, err := vaultproof.SignIntent(signer, request.Expected.Intent(), request.Action, time.Now())
	if err != nil {
		return nil, err
	}
	out, err := json.Marshal(proof)
	if err != nil {
		return nil, err
	}
	return map[string]any{"proof": bytesValue(out)}, nil
}
