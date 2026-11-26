// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package autorizacion es lo que el cliente `api`, la caché `offline` y las pantallas
// compartidas saben de la sesión de Sytue ID sin enlazarla: los testigos de una
// autorización (AuthorizationScope, AuthorizationIntent) y la identidad que llevan, los
// reclamos del access token, los dos errores con que la sesión dice «hay que volver a
// entrar» o «esto empezó con otra cuenta», y la vuelta del navegador a 127.0.0.1
// (WaitLoopback).
//
// Es una hoja: solo la biblioteca estándar. El login OIDC, los almacenes de la sesión y
// el llavero (con su cgo en Apple) siguen en `nucleo/auth`, que da estos mismos nombres
// como alias (`auth.ErrLoginRequired` ES `autorizacion.ErrLoginRequired`, así que
// `errors.Is` vale entre los dos). Así Sytue Office, que no tiene cuenta, no enlaza
// `auth`: lo vigila `apps/fyne/local/guardian_test.go`.
package autorizacion

import "errors"

// ErrLoginRequired: no queda nada que renovar; hay que volver a pasar por Sytue ID.
var ErrLoginRequired = errors.New("auth: hace falta iniciar sesión")

// ErrAuthorizationChanged rejects an operation begun in a different session.
var ErrAuthorizationChanged = errors.New("auth: authorization identity changed")
