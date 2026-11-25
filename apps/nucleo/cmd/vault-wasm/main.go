// SPDX-License-Identifier: Apache-2.0
//go:build js && wasm

// Command vault-wasm es el núcleo LIGERO de la bóveda (`module_id: "vault"`): lo que
// toda app con bóveda descarga al arrancar. Lleva solo las operaciones de kernelops
// —identidad híbrida, sobres, registros por fragmentos, la clave de firma y la firma
// de acciones de entrada— y el transporte rpc, sin webport ni el SDK de Matrix.
//
// Lo demás (Matrix, recursos, búsqueda de Rooms, puertos del anfitrión) sigue en el
// núcleo completo `core` (cmd/pqc-wasm), que solo descarga quien lo usa
// (docs/decisions.md#0086). El tamaño lo vigila TestElNucleoDeBovedaCabeEnSuPresupuesto.
package main

import (
	"git.sytue.eu/sytue/one/apps/nucleo/cmd/internal/kernelops"
	"git.sytue.eu/sytue/one/apps/nucleo/webport/rpc"
)

func main() {
	rpc.Serve(rpc.Module{ID: "vault", Version: 1, Execute: kernelops.Execute, RejectedCode: "crypto_rejected"})
}
