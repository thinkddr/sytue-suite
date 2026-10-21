// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package photos es Sytue Photos en Fyne, con paridad 1:1 con
// `apps/frontend/apps/photos` (ADR 0048).
//
// # Cómo está organizado, y por qué así
//
// Cada pantalla es una FUNCIÓN que recibe sus datos ya resueltos y devuelve un
// `fyne.CanvasObject`. No hay máquina de estados, ni interfaz de «fuente de datos», ni
// contenedor de inyección: el binario (`cmd/photos`) rellena la estructura desde
// `apps/nucleo/api` y un guardián la rellena a mano con tres fotos. Eso deja que los
// tests de píxel pinten cualquiera de los seis estados de cualquier pantalla sin red,
// sin sesión y sin servidor gráfico, que es exactamente lo que hace `pantallas_test.go`.
//
// La TOPBAR no está: existe solo en la web y en nativo la sustituye el cromo de la
// ventana. Es una de las dos exclusiones de paridad escritas en `CONVENTIONS.md` §13.6.
//
// # Lo que esta app aporta al catálogo
//
// La galería es la primera pantalla de la suite con IMÁGENES DE VERDAD, y ahí el
// catálogo compartido no llegaba: `fototeca.go` y `miniatura.go` son la pieza, con sus
// mediciones en el comentario de cada una.
package photos

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Traductor es el de la app: el catálogo común más el de Photos, en ese orden.
func Traductor(l i18n.Locale) (*i18n.Translator, error) {
	return i18n.New(l, i18n.Common(), catalogos.Photos())
}

// Estado son los seis en los que puede estar una pantalla. El cero es el bueno, que es
// el que más se pinta.
type Estado uint8

// Los seis.
const (
	// Exito: hay datos y se enseñan.
	Exito Estado = iota
	// Cargando: se han pedido y no han llegado.
	Cargando
	// Vacia: han llegado y no hay ninguno. NO es un fallo, y por eso tiene su propio
	// texto en vez del de error.
	Vacia
	// Error: el servicio ha contestado que no.
	Error
	// SinConexion: no se ha podido ni preguntar.
	SinConexion
	// SinPermiso: la sesión no alcanza a estos datos (403).
	SinPermiso
	// SinSesion: nadie ha entrado. La web manda a Sytue ID sin preguntar; una app de
	// escritorio no abre el navegador al arrancar sin que nadie lo pida, así que
	// ofrece el botón (el mismo «Iniciar sesión» del menú de cuenta).
	SinSesion
)

// Vista es el contexto común de todas las pantallas: con qué se escribe, con qué se
// pintan las fotos y cuánto mide la ventana.
type Vista struct {
	T *i18n.Translator
	// Teca es la caché de MINIATURAS. Puede ser nil en una pantalla sin fotos.
	Teca *Fototeca
	// TecaGrande es la de la variante «grande», que solo usa el visor. Son dos y no
	// una porque son dos costes: una foto de 2048 px ocupa dieciséis veces más que
	// una de 512, y mezclarlas en el mismo tope echaría la galería entera al abrir
	// tres fotos. nil: el visor cae en Teca.
	TecaGrande *Fototeca
	// Ventana es el ancho de la VENTANA, que es lo que miden los cortes `sm:`/`xl:`
	// de la hoja — no el de la caja. Es la trampa que pinta siete teselas donde la
	// web pinta seis.
	Ventana  float32
	Variante tema.Variante
	// Estado es en cuál de los seis está la pantalla.
	Estado Estado
	// Detalle es el texto del error, cuando lo hay y el servicio lo da.
	Detalle string
	// AlEntrar es el botón del estado SinSesion.
	AlEntrar func()
}

// Traduce es el `t()` de la web.
func (v Vista) Traduce(clave string, vars ...i18n.Vars) string {
	if len(vars) > 0 {
		return v.T.T(clave, vars[0])
	}
	return v.T.T(clave, nil)
}

// Las medidas del marco de contenido de esta app, de sus diez páginas.
//
// La galería usa `max-w-screen-2xl px-4 py-6 lg:px-8` y las demás `max-w-7xl px-4
// py-6`. Medido en producción a 1280: la galería deja la caja en 1224 empezando en 28
// (el `lg:px-8`), y «Álbumes» en 1092 empezando en 94 (`max-w-7xl` son 1120 con el rem
// de 14, centrados, más el `px-4`).
const (
	AnchoMaximoGaleria = 1536 // `max-w-screen-2xl`, en px literales
	AnchoMaximoPagina  = 80 * tema.Rem
	MargenPagina       = 4 * 0.25 * tema.Rem // px-4
	MargenPaginaLG     = 8 * 0.25 * tema.Rem // lg:px-8
	MargenPaginaY      = 6 * 0.25 * tema.Rem // py-6
	SeparacionBloque   = 4 * 0.25 * tema.Rem // mb-4
	SeparacionSeccion  = 6 * 0.25 * tema.Rem // mb-6
	SeparacionMes      = 8 * 0.25 * tema.Rem // mb-8
	CorteLGPagina      = ui.CorteLG
)

// CajaGaleria es la caja de contenido de la galería a esa anchura de ventana: dónde
// empieza y cuánto mide. Devuelve los mismos 28 y 1224 que mide producción a 1280.
func CajaGaleria(ventana float32) (x, ancho float32) {
	margen := float32(MargenPagina)
	if ventana >= CorteLGPagina {
		margen = MargenPaginaLG
	}
	util := min(ventana, AnchoMaximoGaleria)
	return (ventana-util)/2 + margen, max(util-2*margen, 0)
}

// CajaPagina es la de las otras nueve, con `max-w-7xl`: a 1280 devuelve 94 y 1092, que
// es lo que mide «Álbumes» en producción.
func CajaPagina(ventana float32) (x, ancho float32) {
	util := min(ventana, AnchoMaximoPagina)
	return (ventana-util)/2 + MargenPagina, max(util-2*MargenPagina, 0)
}

// ---- Los seis estados ----

// EstadoVacio es el `EmptyState` de la web con la ilustración `emptyPhotos` a su
// `size = 160` por defecto.
func EstadoVacio(v Vista, titulo, descripcion string, accion fyne.CanvasObject) *ui.Vacio {
	e := ui.NuevoVacio(v.Traduce(titulo), v.Traduce(descripcion))
	e.Ilustracion = iconos.Pintar(iconos.VacioFotos, v.Variante).A(AnchoIlustracion).Objeto()
	e.Accion = accion
	return e
}

// AnchoIlustracion es el `size = 160` por defecto de Illustration.tsx: px literales,
// el rem de 14 no lo toca.
const AnchoIlustracion = 160

// CuerpoDeEstado es lo que se pinta en cada uno de los cinco estados que NO son éxito.
// Devuelve nil en éxito, que es la señal de «pinta tus datos».
//
// Los textos salen del catálogo COMÚN, que ya tiene los cinco: `ui.loading`,
// `error.connection`, `offline.status`/`offline.description` y
// `forbidden.title`/`forbidden.description`. No se inventa ninguna clave.
func CuerpoDeEstado(v Vista, vacio *ui.Vacio) fyne.CanvasObject {
	switch v.Estado {
	case Exito:
		return nil
	case Cargando:
		// El `<Spinner />` suelto de la web: 18 px, a la izquierda y en el color del
		// texto, con `ui.loading` como nombre accesible.
		r := ui.NuevaRueda(ui.LadoRueda, "text", v.Variante)
		r.Rotulo = v.Traduce("ui.loading")
		return container.New(fijo{ancho: ui.LadoRueda, alto: ui.LadoRueda}, r)
	case Vacia:
		return vacio
	case Error:
		return Parrafo(textoODefecto(v.Detalle, v.Traduce("error.connection")),
			tema.TextoSM, ui.Normal, "danger-text", v.Variante)
	case SinConexion:
		return EstadoVacio(v, "offline.status", "offline.description", nil)
	case SinPermiso:
		return EstadoVacio(v, "forbidden.title", "forbidden.description", nil)
	case SinSesion:
		entrar := ui.NuevoBoton(v.Traduce("account.signIn"), v.AlEntrar).Con(ui.Primario, ui.TallaSM)
		e := ui.NuevoVacio(v.Traduce("app.title"), "")
		e.Ilustracion = iconos.Pintar(iconos.VacioFotos, v.Variante).A(AnchoIlustracion).Objeto()
		e.Accion = entrar
		return e
	}
	return nil
}

func textoODefecto(s, porDefecto string) string {
	if s != "" {
		return s
	}
	return porDefecto
}

// Parrafo es un `<p>` de una línea con su token de color: el patrón que la web repite
// en las diez páginas para el error, el aviso y los recuentos. En la variante de la
// vista: con `tema.Claro` fijo, el error salía en oscuro con el rojo del claro (#a3342a
// sobre #0d171c, 2,4:1) en vez de #f08f84.
func Parrafo(texto string, e tema.Escalon, p ui.Peso, token string, v tema.Variante) *canvas.Text {
	return ui.Texto(texto, e, p, tema.Color(token, v))
}

// ParrafoEn es Parrafo con la variante de tema explícita.
func ParrafoEn(texto string, e tema.Escalon, p ui.Peso, token string, v tema.Variante) *canvas.Text {
	return ui.Texto(texto, e, p, tema.Color(token, v))
}

// ColocarParrafo pone un párrafo en (x, y) y devuelve el alto que ha ocupado, que es lo
// que necesita quien apila bloques uno debajo de otro.
func ColocarParrafo(t *canvas.Text, e tema.Escalon, x, y float32) float32 {
	ui.ColocarTexto(t, e, x, y)
	return e.Linea
}

// Recuento formatea «N fotos» y parecidos con el plural del idioma, que es lo que hace
// `t('albums.count', { count })`.
func Recuento(v Vista, clave string, n int) string {
	return v.T.T(clave, i18n.Vars{"count": n})
}

// fmtLados es el «1920 × 1080» de la ficha del visor.
func fmtLados(w, h int) string { return fmt.Sprintf("%d × %d", w, h) }
