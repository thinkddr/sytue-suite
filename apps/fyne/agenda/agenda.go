// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package agenda es lo que comparten Sytue Calendar y Sytue Contacts.
//
// Son DOS aplicaciones independientes —un binario por producto, como manda
// `docs/CONVENTIONS.md` §13.6— y este paquete no porta ninguna pantalla: no lleva
// marcador `paridad-web:` y nunca debe llevarlo. Es plomería, y está aquí por lo que
// dice §13.2: lo que las dos necesitan se importa, no se copia en cada una.
//
// Lo que hay dentro y por qué NO está en `apps/fyne/ui`:
//
//   - Los SEIS ESTADOS de una pantalla. La composición (esqueleto, `ui.Vacio` con su
//     ilustración, su copia y su acción) es la misma en las dos apps, pero los TEXTOS
//     son de cada catálogo. Es un patrón, no un widget.
//
// ponytail: en cuanto una TERCERA app repita esta composición, `Estados` sube a
// `apps/fyne/ui` —es genérico de la suite, no de la agenda— y este paquete se queda
// con lo que de verdad es de personas y fechas. Hoy son dos y mudarlo antes de tiempo
// deja una pieza compartida que nadie ha probado en un tercer sitio.
//
// Lo que NO está aquí porque el catálogo YA lo tiene: la fila de contacto es
// `ui.FilaLista` (medida 49 con subtítulo, que es lo que mide la de producción), el
// avatar con iniciales es `ui.NuevoAvatar` con el mismo hash que la web, y la cabecera
// de vista es `ui.CabeceraApp`.
package agenda

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// transparente es el relleno de un hueco que solo reserva sitio.
var transparente = color.NRGBA{}

// Estado es en cuál de los seis estados está una pantalla. El cero es Exito, que es el
// caso normal: una pantalla sin estado declarado enseña sus datos.
type Estado uint8

// Los seis estados que toda pantalla de la suite tiene que saber pintar.
const (
	// Exito son los datos ya cargados: este paquete no pinta nada y manda la pantalla.
	Exito Estado = iota
	// Cargando es el esqueleto con la GEOMETRÍA de lo que va a venir, no un
	// indicador giratorio: la web evita el salto de layout en cada navegación.
	Cargando
	// Vacia es «no hay nada todavía», con la acción que lo llena.
	Vacia
	// Error es «no se pudo cargar», con «Reintentar».
	Error
	// SinConexion es la pérdida de red: la app pasa a consulta y las escrituras se
	// apagan. No es un error: los datos que ya están siguen sirviendo.
	SinConexion
	// SinPermiso es el 403: la libreta o el calendario existen pero no son tuyos.
	// No lleva «Reintentar», porque reintentar no lo va a arreglar.
	SinPermiso
)

// Textos es la copia de un estado, en claves YA traducidas por la app. Este paquete no
// inventa ni traduce: recibe cadenas hechas.
type Textos struct {
	Titulo      string
	Descripcion string
	// RotuloAccion es el botón del estado. Vacío: sin botón.
	RotuloAccion string
	// Accion es lo que hace ese botón. nil con RotuloAccion puesto pinta el botón
	// inerte, que es lo que hace la web cuando no hay red.
	Accion func()
	// Primaria pinta el botón `primary` con su «+» delante: el de CREAR. Los de
	// limpiar un filtro o reintentar son `secondary`, que es el defecto.
	Primaria bool
	// Ilustracion es el spot o el icono de encima del título. nil: sin ilustración.
	Ilustracion fyne.CanvasObject
}

// Panel es lo que se pinta en el hueco de contenido para este estado.
//
// Devuelve nil en Exito: ahí manda la pantalla y este paquete no estorba. Los otros
// cinco son el mismo `EmptyState` de la web con otra copia, salvo Cargando, que es el
// esqueleto que le pase la app —cada pantalla tiene la suya y la geometría es lo único
// que importa de un esqueleto.
func Panel(e Estado, t Textos, esqueleto fyne.CanvasObject) fyne.CanvasObject {
	switch e {
	case Exito:
		return nil
	case Cargando:
		return esqueleto
	case Vacia, Error, SinConexion, SinPermiso:
	}
	v := ui.NuevoVacio(t.Titulo, t.Descripcion)
	v.Ilustracion = t.Ilustracion
	if t.RotuloAccion != "" {
		b := ui.NuevoBoton(t.RotuloAccion, t.Accion).Con(ui.Secundario, ui.TallaSM)
		if t.Primaria {
			b = b.Con(ui.Primario, ui.TallaSM).ConIcono(iconos.Mas, 0)
		}
		b.Inerte = t.Accion == nil
		v.Accion = b
	}
	return v
}

// Bloque es un `<Skeleton>` de la agenda: `ui.Esqueleto` con el tinte que toca.
//
// Sobre el cromo oscuro del menú la web lo tiñe de `sidebar-hover-bg` —un gris claro
// ahí sería un destello—, que es lo que hace `sobreCromo`. Lo demás lo pone el widget
// del catálogo, con su pulso; hasta el 2026-09-22 esto pintaba su propio rectángulo y
// lo redondeaba con `RadioSM` donde la hoja pide `rounded-md`.
func Bloque(ancho, alto float32, sobreCromo bool) fyne.CanvasObject {
	e := ui.NuevoEsqueleto(ancho, alto)
	if sobreCromo {
		e.SobreCromo()
	}
	return e
}

// Espaciador reserva alto sin pintar nada. Sirve para los huecos que en la web son
// `mt-*` sobre un hijo y no un `gap` del contenedor.
func Espaciador(alto float32) fyne.CanvasObject {
	r := canvas.NewRectangle(transparente)
	r.SetMinSize(fyne.NewSize(0, alto))
	return r
}

// EnColumna apila con un hueco constante. Es azúcar sobre `ui.Columna` para que las
// pantallas no repitan el `container.New` en cada bloque.
func EnColumna(hueco float32, hijos ...fyne.CanvasObject) *fyne.Container {
	return container.New(ui.Columna{Hueco: hueco}, hijos...)
}

// EnFila pone en horizontal con un hueco constante.
func EnFila(hueco float32, hijos ...fyne.CanvasObject) *fyne.Container {
	return container.New(ui.Fila{Hueco: hueco}, hijos...)
}
