// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package auth

import (
	"context"
	"net/url"

	"git.sytue.eu/sytue/one/apps/nucleo/autorizacion"
)

// LoopbackCallback configures a single browser return, shared by Sytue OIDC and
// Matrix SSO. Vive en `nucleo/autorizacion`, donde la usa también el OAuth de las
// nubes de Sytue Office sin enlazar la sesión.
type LoopbackCallback = autorizacion.LoopbackCallback

// WaitLoopback listens only on loopback, opens the browser after binding, and
// closes both server and listener on return (autorizacion.WaitLoopback).
func WaitLoopback(ctx context.Context, options LoopbackCallback) (url.Values, error) {
	return autorizacion.WaitLoopback(ctx, options)
}
