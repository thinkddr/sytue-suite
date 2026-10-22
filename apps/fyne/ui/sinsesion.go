// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"context"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// NuevaPaginaSinSesion es lo que enseña una app de la suite a quien aún no ha iniciado
// sesión: un estado vacío con «Iniciar sesión». La web no la tiene —su middleware manda
// al inicio de sesión antes de pintar nada—; en nativo la ventana existe antes que la
// sesión, y sin esto cada app pintaba su propio malentendido: Mail y Rooms «Cargando…»
// para siempre, Contacts una libreta vacía, el ERP un resumen a cero (visto en una
// ventana real el 2026-09-27).
//
// `entrar` es lo que hace el botón; si ya hay un inicio de sesión esperando al navegador,
// `sesion.Entrar` lo vuelve a abrir donde estaba.
func NuevaPaginaSinSesion(t *i18n.Translator, entrar func()) fyne.CanvasObject {
	v := NuevoVacio(t.T("account.signedOutTitle", nil), t.T("account.signedOutDescription", nil))
	v.Glifo = iconos.Trazo(iconos.Usuario, 32, "text-subtle", tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	v.Accion = NuevoBoton(t.T("account.signIn", nil), entrar).Con(Primario, TallaMD)
	return v // sobre el fondo de la ventana, que es el `bg` del tema
}

// entrarDesde es el botón de la página: Entrar bloquea hasta volver del navegador, así
// que va en su goroutine. Su error ya lo deja escrito `sesion.Entrar`.
func entrarDesde(s *sesion.Suite) func() {
	return func() { go func() { _ = s.Entrar(context.Background()) }() }
}
