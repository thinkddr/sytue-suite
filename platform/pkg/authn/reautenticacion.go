// SPDX-License-Identifier: Apache-2.0
package authn

import (
	"context"
	"slices"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
)

// FrescuraReautenticacion es cuánto vale una identificación para una acción que
// no se deshace en un servicio de la suite: la misma que el step-up de Sytue ID
// (services/id/internal/http/stepup.go) y la del vaciado de una clave de bóveda.
const FrescuraReautenticacion = 5 * time.Minute

// segundoFactor: los métodos que Sytue ID anota cuando hubo segundo factor. Los
// mismos que acepta el vaciado de una clave de bóveda (claves_boveda.go).
func segundoFactor(amr []string) bool {
	return slices.Contains(amr, "mfa") || slices.Contains(amr, "otp")
}

// IdentificacionReciente dice si quien llama se identificó hace menos de
// FrescuraReautenticacion y, si su cuenta tiene segundo factor, con él.
//
// Un token sin `mfa_enabled` (anterior al claim) cuenta como cuenta CON segundo
// factor: ante la duda se pide más, nunca menos. Un `auth_time` del futuro (más
// de un minuto de desfase) no cuenta. Un token delegado (`act`) nunca vale: la
// persona no está delante.
func (c Claims) IdentificacionReciente(ahora time.Time) bool {
	if c.Delegado || c.AuthTime.IsZero() || c.AuthTime.After(ahora.Add(time.Minute)) || ahora.Sub(c.AuthTime) > FrescuraReautenticacion {
		return false
	}
	conMFA := c.MFAEnabled == nil || *c.MFAEnabled
	return !conMFA || segundoFactor(c.AMR)
}

// PrivateStorageReauthentication adapta los claims ya verificados a la prueba de
// reautenticación que exige «soltar la fuente» (httpx.PrivateStorageRelease).
// Devuelve false si no es reciente o falta el segundo factor de la cuenta.
func PrivateStorageReauthentication(ctx context.Context) (httpx.Reauthentication, bool) {
	claims, ok := FromContext(ctx)
	if !ok || !claims.IdentificacionReciente(time.Now()) {
		return httpx.Reauthentication{}, false
	}
	return httpx.Reauthentication{AuthTime: claims.AuthTime, AMR: slices.Clone(claims.AMR)}, true
}
