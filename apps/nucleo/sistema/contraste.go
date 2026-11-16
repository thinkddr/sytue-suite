// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sistema

import (
	"sync"
	"sync/atomic"
)

var (
	masContrasteUna sync.Once
	masContrasteSi  atomic.Bool
)

// MasContraste dice si la persona ha pedido al sistema más contraste: el
// `prefers-contrast: more` de la web, con el que la hoja de tokens sube los bordes
// decorativos al contorno de control (≥ 3:1), el texto sutil al atenuado y quita los
// velos del traslúcido (apps/nucleo/tokens lo aplica con ElegirMasContraste).
//
//   - macOS: Accesibilidad → Pantalla → Aumentar contraste (`increaseContrast`).
//   - Linux: el «Alto contraste» de GNOME (`high-contrast` de
//     `org.gnome.desktop.a11y.interface`) o el `contrast` del portal de ajustes, que es
//     lo que leen los navegadores; con que uno lo pida, basta.
//   - Windows: el modo de contraste alto (`SPI_GETHIGHCONTRAST`), que Chromium y Edge
//     traducen también a `prefers-contrast: more`.
//
// ponytail: en Android e iOS se responde que no; y se pregunta una vez por proceso, como
// MenosMovimiento, así que cambiar el ajuste con la app abierta no se nota hasta volver
// a abrirla.
func MasContraste() bool {
	masContrasteUna.Do(func() { masContrasteSi.Store(masContraste()) })
	return masContrasteSi.Load()
}

// FijarMasContraste pone la respuesta de MasContraste sin preguntar al sistema y devuelve
// con qué restaurar la anterior. Es para las pruebas, por lo mismo que
// FijarMenosMovimiento: lo que pinta un test no puede depender del ajuste de la máquina.
func FijarMasContraste(si bool) (restaurar func()) {
	// Gastar la consulta: si no, la primera llamada a MasContraste pisaría lo fijado.
	masContrasteUna.Do(func() {})
	antes := masContrasteSi.Swap(si)
	return func() { masContrasteSi.Store(antes) }
}
