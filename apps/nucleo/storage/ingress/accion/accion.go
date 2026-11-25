// SPDX-License-Identifier: Apache-2.0

// Package accion es el formato de las acciones de entrada firmadas: la acción, la
// intención, la autoridad, la autorización y sus bytes canónicos. Es lo que la
// bóveda firma (vaultproof) sin verificar ninguna cadena de dispositivos de Matrix.
//
// Vivía en matrixproof, que verifica esas cadenas con los tipos de respuesta del SDK
// (el paquete raíz de mautrix, y con él zerolog, net/http y TLS). El núcleo de bóveda
// del navegador firma acciones pero nunca verifica dispositivos, y por importar el
// formato se llevaba el SDK entero: 2,7 MB de los 10 que pesaba (docs/decisions.md#0086).
// matrixproof reexporta todo lo de aquí con el mismo nombre; los tipos son los mismos.
package accion

import (
	"encoding/base64"
	"errors"
)

var (
	ErrUnverified     = errors.New("ingress: independently authenticated Matrix device is required")
	ErrRootEnrollment = errors.New("ingress: explicitly confirmed Matrix root enrollment is required")
)

// ValidKey admite una clave Ed25519 pública en base64 sin relleno y canónica.
func ValidKey(value string) bool {
	raw, err := base64.RawStdEncoding.DecodeString(value)
	return err == nil && len(raw) == 32 && base64.RawStdEncoding.EncodeToString(raw) == value
}
