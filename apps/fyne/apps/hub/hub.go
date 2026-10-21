// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package hub es Sytue Hub en Fyne: la puerta de entrada de la suite.
//
// La referencia es la web, `apps/frontend/apps/portal/src`, medida contra producción
// (`corporate.sytue.eu`) con `test/e2e/medidas-web.mjs portal`. Cada número de este
// paquete sale de ahí o de la hoja; ninguno está estimado.
//
// # Lo que el Hub tiene de propio
//
//   - **La rejilla de productos.** Es la única pantalla de la suite que enseña las
//     diecisiete marcas a la vez, y la que decide cómo se salta de una aplicación a
//     otra: cada producto es su propia aplicación también en escritorio y en móvil
//     (`docs/CONVENTIONS.md` §13.6), así que pulsar una tesela NO abre una pantalla de
//     este binario — le pide al SISTEMA que abra la otra aplicación por su esquema de
//     URL. Eso es `abrir.go`.
//   - **El abanico federado del Hub**: `/recent` y `/search` preguntan a la vez a Mail,
//     Drive, Docs, Calendar, Contacts y Rooms, y un origen caído no tumba a los demás
//     —pero hay que DECIRLO, o quien busca da por perdido un correo que sí existe—.
//   - **El flujo SSE de avisos**, que es de `apps/nucleo/api` y no se reescribe aquí.
//
// # La topbar no está, y es a propósito
//
// Es una de las dos excepciones de paridad escritas en §13.6: la `TopBar` existe solo
// en la web y en nativo la sustituye el cromo del sistema (la barra de título y el
// menú de la ventana, que monta `cmd/hub`). Por eso las pantallas empiezan en y = 0 y
// no en 56, y por eso el buscador global vive en la pantalla de búsqueda y no arriba.
package hub

import (
	"errors"
	"net/url"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// ---- Los seis estados ----

// Estado es por cuál de los seis estados pasa una pantalla. Los seis son requisito de
// calidad, no una lista de casos bonitos: cinco de ellos son lo que la persona ve
// cuando algo va mal, que es cuando más importa que la pantalla diga qué pasa.
type Estado uint8

// Los seis, en el orden en que ocurren.
const (
	// Cargando: la petición está en vuelo. La web pinta esqueletos con la geometría
	// real de la lista para que no haya salto al llegar los datos.
	Cargando Estado = iota
	// Listo: hay datos y se pintan.
	Listo
	// SinNada: la respuesta llegó y está vacía. NO es un error.
	SinNada
	// Fallo: el servicio contestó mal (5xx, 4xx que no sea 403).
	Fallo
	// SinConexion: la petición no llegó a tener respuesta. Se distingue de Fallo
	// porque lo que hay que hacer es distinto: mirar la red, no reintentar a ciegas.
	SinConexion
	// Prohibido: hay sesión, pero esta cuenta no tiene permiso (403).
	Prohibido
)

// EstadoDe traduce el error de una llamada al estado que hay que pintar.
//
// El criterio es el de `apps/nucleo/api`: lo que vuelve como `*api.Error` es una
// respuesta HTTP —el servidor habló—, y cualquier otra cosa es que la petición no
// llegó a tener respuesta, o sea que no hay conexión. Distinguirlo importa porque el
// texto que se enseña es distinto: «vuelve a intentarlo» no ayuda en un tren.
func EstadoDe(err error) Estado {
	if err == nil {
		return Listo
	}
	var e *api.Error
	if errors.As(err, &e) {
		if e.Status == 403 {
			return Prohibido
		}
		return Fallo
	}
	var u *url.Error
	if errors.As(err, &u) {
		return SinConexion
	}
	return Fallo
}

// ---- El contenedor de página ----

// La caja de contenido de todas las pantallas del Hub:
// `mx-auto w-full max-w-[var(--container-content,1280px)] px-6 py-8`.
//
// Medido en producción a 1280 de ventana: la caja arranca en x = 21 y mide 1238, y el
// primer bloque cae en y = 84 — o sea 56 de topbar (que aquí no existe) más los 28 del
// `py-8`. En nativo el 56 se va con la topbar y el contenido empieza en 28.
const (
	// MargenX es el `px-6`.
	MargenX float32 = 6 * 0.25 * tema.Rem
	// MargenY es el `py-8`.
	MargenY float32 = 8 * 0.25 * tema.Rem
)

// CajaContenido es dónde empieza y cuánto mide la caja de contenido para un ancho de
// ventana dado. `mx-auto` la centra y `max-w` la acota a 1280.
func CajaContenido(ventana float32) (x, ancho float32) {
	caja := min(ventana, tema.AnchoContenido)
	return (ventana-caja)/2 + MargenX, max(caja-2*MargenX, 0)
}

// pagina es la cáscara que comparten las pantallas del Hub. Guarda el ancho de
// VENTANA, que es el que deciden los cortes de Tailwind (`sm:`, `lg:`), no el de la
// caja: comparar el corte con el ancho disponible pinta una columna de más.
//
// Existe porque `fyne.Widget.MinSize()` no recibe el ancho y el alto de una pantalla
// depende de él: cuántas filas tiene la rejilla, en cuántas líneas se parte un
// párrafo. El ancho se recuerda al redimensionar y `MinSize` contesta con el alto que
// corresponde a ese ancho, que es lo que necesita el `container.Scroll` de arriba.
type pagina struct {
	widget.BaseWidget

	ancho float32
}

// Ancho es el de la ventana que se le ha dado a la pantalla. Antes del primer Resize
// vale el canónico, que es con el que se midió la web.
func (p *pagina) Ancho() float32 {
	if p.ancho <= 0 {
		return tema.AnchoContenido
	}
	return p.ancho
}

// recordar guarda el ancho y dice si ha cambiado.
func (p *pagina) recordar(s fyne.Size) bool {
	if s.Width == p.ancho {
		return false
	}
	p.ancho = s.Width
	return true
}
