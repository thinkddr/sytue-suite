// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import "git.sytue.eu/sytue/one/apps/nucleo/sistema"

// # prefers-reduced-motion
//
// El navegador consulta el ajuste del sistema y la hoja de la suite apaga con él todas
// las animaciones; Fyne v2.8.1 no expone nada equivalente, así que lo pregunta la suite
// (sistema.MenosMovimiento) y cada animación de `ui` —la rueda, el pulso del esqueleto,
// el cajón, el velo y los fundidos de diálogo y pista, las transiciones de color, el desplazamiento
// suave, los GIF— y del calendario mira MenosMovimiento antes de arrancar. Así lo adoptan
// todas las apps sin tocarlas.

// SinMovimiento apaga las animaciones de la suite aunque el sistema no lo pida. Lo pone
// quien monta la app o un test que quiere los widgets ya en su sitio; para LEER si hay
// que animar está MenosMovimiento, que también atiende al ajuste del sistema (lo vigila
// TestNadieLeeSinMovimientoAPelo).
var SinMovimiento bool

// MenosMovimiento dice si hay que ahorrarse las animaciones: lo ha pedido la app
// (SinMovimiento) o la persona en el sistema (`prefers-reduced-motion` de la web). Con
// él, lo que se repite sin fin se queda quieto y lo que entra sale ya puesto.
func MenosMovimiento() bool { return SinMovimiento || sistema.MenosMovimiento() }
