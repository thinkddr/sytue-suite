// SPDX-License-Identifier: Apache-2.0

// Package rpc es el transporte de los módulos Go/Wasm del núcleo de cliente: el
// despacho de operaciones que llama el worker, los bytes que cruzan la frontera y el
// error de rechazo. Vive aparte de webport a propósito: webport arrastra el
// protocolo de puertos del anfitrión (autoridad de Matrix, entrada del correo,
// fuentes), y el núcleo de bóveda que toda app descarga al arrancar solo necesita
// este transporte. Importar webport para llamar a Serve metía mautrix, zerolog y
// net/http en ese módulo (docs/decisions.md#0086).
//
// webport reexporta todo lo de aquí con el mismo nombre: no hay dos transportes.
package rpc

import (
	"errors"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/fallo"
)

// ErrRejected es el mismo valor que webport.ErrRejected.
var ErrRejected = errors.New("webport: scoped host operation rejected")

func init() { fallo.Conocido(ErrRejected, "rechazado") }

// ValidID admite solo el identificador opaco de un puerto: 64 hexadecimales.
func ValidID(id string) bool {
	if len(id) != 64 {
		return false
	}
	return strings.Trim(id, "0123456789abcdef") == ""
}

// Rechazo es la respuesta de una operación que falló: el código del módulo y, si
// el error lo dice con palabras fijas (paquete fallo), el motivo. Así «Más
// información» dice qué paso del núcleo falló y no solo `crypto_rejected`.
func Rechazo(code string, err error) map[string]any {
	result := map[string]any{"ok": false, "code": code}
	if motivo := fallo.Motivo(err); motivo != "" {
		result["motivo"] = motivo
	}
	return result
}
