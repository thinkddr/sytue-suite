// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sistema

import "sync"

var (
	menosMovimientoUna sync.Once
	menosMovimientoSi  bool
)

// MenosMovimiento dice si la persona ha pedido al sistema menos animaciones: el
// `prefers-reduced-motion: reduce` de la web, que la suite consulta antes de animar un
// desplazamiento (quien lo activa, por vértigo o migraña, no quiere un barrido entero
// cada vez que salta de página).
//
//   - macOS: Accesibilidad → Pantalla → Reducir movimiento (`reduceMotion`).
//   - Linux: la preferencia de GNOME `enable-animations`, la velocidad de las
//     animaciones de KDE Plasma (`AnimationDurationFactor` de kdeglobals) o el
//     `reduced-motion` del portal de ajustes; con que uno lo pida, basta.
//
// ponytail: en Windows (`SPI_GETCLIENTAREAANIMATION`), Android e iOS se responde que no;
// y se pregunta una vez por proceso, así que cambiar el ajuste con la app abierta no se
// nota hasta volver a abrirla.
func MenosMovimiento() bool {
	menosMovimientoUna.Do(func() { menosMovimientoSi = menosMovimiento() })
	return menosMovimientoSi
}
