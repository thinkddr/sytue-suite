// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sistema

import (
	"sync"
	"sync/atomic"
)

var (
	menosMovimientoUna sync.Once
	menosMovimientoSi  atomic.Bool
)

// MenosMovimiento dice si la persona ha pedido al sistema menos animaciones: el
// `prefers-reduced-motion: reduce` de la web, que la suite consulta antes de animar
// (quien lo activa, por vértigo o migraña, no quiere una rueda girando ni un barrido
// entero cada vez que salta de página).
//
//   - macOS: Accesibilidad → Pantalla → Reducir movimiento (`reduceMotion`).
//   - Linux: la preferencia de GNOME `enable-animations`, la velocidad de las
//     animaciones de KDE Plasma (`AnimationDurationFactor` de kdeglobals) o el
//     `reduced-motion` del portal de ajustes; con que uno lo pida, basta.
//   - Windows: «Mostrar animaciones en Windows» apagado (`SPI_GETCLIENTAREAANIMATION`).
//
// ponytail: en Android e iOS se responde que no; y se pregunta una vez por proceso, así
// que cambiar el ajuste con la app abierta no se nota hasta volver a abrirla.
func MenosMovimiento() bool {
	menosMovimientoUna.Do(func() { menosMovimientoSi.Store(menosMovimiento()) })
	return menosMovimientoSi.Load()
}

// FijarMenosMovimiento pone la respuesta de MenosMovimiento sin preguntar al sistema, y
// devuelve con qué restaurar la anterior. Es para las pruebas: lo que pinta un test no
// puede depender de cómo tenga el ajuste la máquina que lo corre, así que el `TestMain`
// de cada paquete fija «no» y el test que mira el «sí» lo fija él.
func FijarMenosMovimiento(si bool) (restaurar func()) {
	// Gastar la consulta: si no, la primera llamada a MenosMovimiento pisaría lo fijado.
	menosMovimientoUna.Do(func() {})
	antes := menosMovimientoSi.Swap(si)
	return func() { menosMovimientoSi.Store(antes) }
}
