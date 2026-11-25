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
)

// ErrRejected es el mismo valor que webport.ErrRejected.
var ErrRejected = errors.New("webport: scoped host operation rejected")

// ValidID admite solo el identificador opaco de un puerto: 64 hexadecimales.
func ValidID(id string) bool {
	if len(id) != 64 {
		return false
	}
	return strings.Trim(id, "0123456789abcdef") == ""
}
