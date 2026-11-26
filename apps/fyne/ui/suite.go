// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !edicionlocal

package ui

import (
	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/sesion"
)

// # La sesión de la suite, en UN sitio
//
// La barra de la suite (cuenta, campana, buscador) es lo único de `ui` que habla con la
// sesión de Sytue ID. Lo que usa de `sesion` se nombra aquí, y solo aquí, para que la
// edición local (Sytue Office, etiqueta `edicionlocal`) compile `ui` SIN `sesion` —y sin
// el login OIDC que este arrastra—: allí `suite_local.go` pone en su lugar una sesión
// que nunca entra (PLAN-EDICION-LOCAL §5.4, C-04).
//
// En la edición conectada son alias: los mismos tipos y las mismas llamadas de siempre,
// y `NuevaBarraSuite` recibe exactamente un `*sesion.Suite`.

type (
	suiteSesion        = sesion.Suite
	estadoSesion       = sesion.Estado
	autorizacionSalida = sesion.AutorizacionSalida
)

// avisoDeRespaldo es [sesion.AvisoDeRespaldo]: el aviso de Fyne cuando el del sistema
// no pudo.
func avisoDeRespaldo(a fyne.App, titulo, cuerpo string) { sesion.AvisoDeRespaldo(a, titulo, cuerpo) }

// appAbierta es [sesion.Abierta]: si la app de ese identificador contesta en su enchufe.
func appAbierta(id string) bool { return sesion.Abierta(id) }
