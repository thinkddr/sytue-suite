// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
)

// Los SEIS ESTADOS de una pantalla y su composición: el esqueleto con la geometría de
// lo que va a venir, o `Vacio` con su ilustración, su copia y su acción.
//
// Vivía en `apps/fyne/agenda`, que lo compartían Calendar y Contacts, con una nota que
// pedía subirlo aquí en cuanto una TERCERA app repitiera la composición. Notes es esa
// tercera (los esqueletos de carga de sus cuadernos), así que está aquí: es genérico de
// la suite, no de la agenda. Los TEXTOS siguen siendo de cada app: esto recibe cadenas
// ya traducidas y no inventa ninguna.

// Estado es en cuál de los seis estados está una pantalla. El cero es EstadoExito, que
// es el caso normal: una pantalla sin estado declarado enseña sus datos.
type Estado uint8

// Los seis estados que toda pantalla de la suite tiene que saber pintar.
const (
	// EstadoExito son los datos ya cargados: aquí no se pinta nada y manda la pantalla.
	EstadoExito Estado = iota
	// EstadoCargando es el esqueleto con la GEOMETRÍA de lo que va a venir, no un
	// indicador giratorio: la web evita el salto de layout en cada navegación.
	EstadoCargando
	// EstadoVacia es «no hay nada todavía», con la acción que lo llena.
	EstadoVacia
	// EstadoError es «no se pudo cargar», con «Reintentar».
	EstadoError
	// EstadoSinConexion es la pérdida de red: la app pasa a consulta y las escrituras
	// se apagan. No es un error: los datos que ya están siguen sirviendo.
	EstadoSinConexion
	// EstadoSinPermiso es el 403: la libreta o el calendario existen pero no son tuyos.
	// No lleva «Reintentar», porque reintentar no lo va a arreglar.
	EstadoSinPermiso
)

// TextosEstado es la copia de un estado, en claves YA traducidas por la app.
type TextosEstado struct {
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

// PanelEstado es lo que se pinta en el hueco de contenido para este estado.
//
// Devuelve nil en EstadoExito: ahí manda la pantalla. Los otros cinco son el mismo
// `EmptyState` de la web con otra copia, salvo EstadoCargando, que es el esqueleto que
// le pase la app —cada pantalla tiene el suyo y la geometría es lo único que importa de
// un esqueleto—.
func PanelEstado(e Estado, t TextosEstado, esqueleto fyne.CanvasObject) fyne.CanvasObject {
	switch e {
	case EstadoExito:
		return nil
	case EstadoCargando:
		return esqueleto
	case EstadoVacia, EstadoError, EstadoSinConexion, EstadoSinPermiso:
	}
	v := NuevoVacio(t.Titulo, t.Descripcion)
	v.Ilustracion = t.Ilustracion
	if e == EstadoError {
		// DS-06: «no se pudo cargar» es un error y lo dice su marca (`tone="error"`), no
		// el gris de «aún no hay nada». Hasta el 2026-11-24 solo lo usaba el editor.
		v.Tono = VacioError
	}
	if t.RotuloAccion != "" {
		b := NuevoBoton(t.RotuloAccion, t.Accion).Con(Secundario, TallaSM)
		if t.Primaria {
			b = b.Con(Primario, TallaSM).ConIcono(iconos.Mas, 0)
		}
		b.Inerte = t.Accion == nil
		v.Accion = b
	}
	return v
}

// Espaciador reserva alto sin pintar nada. Sirve para los huecos que en la web son
// `mt-*` sobre un hijo y no un `gap` del contenedor.
func Espaciador(alto float32) fyne.CanvasObject {
	r := canvas.NewRectangle(color.NRGBA{})
	r.SetMinSize(fyne.NewSize(0, alto))
	return r
}

// EnColumna apila con un hueco constante. Es azúcar sobre `Columna` para que los
// esqueletos y los estados no repitan el `container.New` en cada bloque.
func EnColumna(hueco float32, hijos ...fyne.CanvasObject) *fyne.Container {
	return container.New(Columna{Hueco: hueco}, hijos...)
}

// EnFila pone en horizontal con un hueco constante.
func EnFila(hueco float32, hijos ...fyne.CanvasObject) *fyne.Container {
	return container.New(Fila{Hueco: hueco}, hijos...)
}
