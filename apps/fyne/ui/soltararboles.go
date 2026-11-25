// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"reflect"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

// SoltarArboles suelta en el acto lo que la caché de Fyne guarda de lo que una pantalla
// acaba de descolgar (`viejos`): los renderers de sus widgets, el lienzo de cada objeto
// y sus texturas (`widget.ReleaseRenderers`, parche 0021 del fork). Sin esto se quedaban
// hasta caducar, un minuto después de su último pintado, y una pantalla que se rehace con
// cada dato retenía un minuto de pantallas viejas.
//
// Lo que siga colgado de alguno de los conservados se deja como está, con todo lo suyo:
// el armazón con las piezas nuevas y lo que la pantalla guarda para reutilizar aunque
// ahora no se vea (un mes, un desplazador, un menú conservado). Un widget soltado que se
// vuelve a enseñar recibe un renderer nuevo, como al caducar.
//
// Se llama en el hilo de la interfaz, DESPUÉS de colgar y colocar las piezas nuevas: la
// caché solo sabe qué hay dentro de un widget por su renderer, y uno recién hecho que aún
// no se ha colocado escondería las piezas reutilizadas que lleve dentro.
//
// Los nil, también los de un puntero con tipo (una pieza que la pantalla aún no ha
// hecho), se saltan.
func SoltarArboles(viejos []fyne.CanvasObject, conservados ...fyne.CanvasObject) {
	sueltos := sinNulos(viejos)
	if len(sueltos) == 0 {
		return
	}
	// Un contenedor sin layout que nadie pinta: solo para recorrerlos de una vez, que
	// los conservados se recorren una sola vez y no una por cada viejo.
	widget.ReleaseRenderers(&fyne.Container{Objects: sueltos}, sinNulos(conservados)...)
}

// sinNulos son los objetos que no son nil ni un puntero nil con tipo.
func sinNulos(objs []fyne.CanvasObject) []fyne.CanvasObject {
	out := make([]fyne.CanvasObject, 0, len(objs))
	for _, o := range objs {
		if o == nil {
			continue
		}
		if v := reflect.ValueOf(o); v.Kind() == reflect.Pointer && v.IsNil() {
			continue
		}
		out = append(out, o)
	}
	return out
}
