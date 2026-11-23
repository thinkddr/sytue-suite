// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"context"
	"image"
	"maps"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/miniatura"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/exif"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/galeriacifrada"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/render"
)

// # El editor de una foto cifrada (ADR 0086)
//
// El mismo editor que el de las fotos del servidor (app_editor.go), con lo que cambia en
// la web (`EditorCifrado` de FotosCifradas.tsx):
//
//   - La previa es la foto descifrada en memoria y ya orientada, reducida al lado de la
//     previa del servidor; nunca sale del dispositivo.
//   - Sin la pestaña de Sytue AI, que no ve estas fotos.
//   - Guardar aplica la receta a la foto ENTERA en el dispositivo, con el motor de
//     Photos (`galeriacifrada.Galeria.Editar`), y sube el resultado como una versión
//     nueva cifrada del mismo fichero: la anterior sigue en el historial de versiones de
//     Drive, y la entrada del índice conserva fecha de captura, favorita y álbumes.

// LadoPreviaCifrada es el lado de la previa de una foto cifrada en el editor: el de la
// variante `preview` del servidor.
const LadoPreviaCifrada = 2048

// abrirEditorCifrado abre el editor con una foto de la galería cifrada.
func (a *App) abrirEditorCifrado(f galeriacifrada.Foto) {
	if a.cif.galeria == nil {
		return
	}
	a.abrirEditorCon(FotoDeLinea(f), "", &f)
}

// cargarEditorCifrado descifra el original en memoria y prepara la previa orientada.
func (a *App) cargarEditorCifrado(e *estadoEditor) {
	g, f := a.cif.galeria, *e.cifrada
	if g == nil {
		return
	}
	gen := a.gen
	a.enSesion(func(vida context.Context) {
		c, fin := context.WithTimeout(vida, PlazoSubida(f.Fichero.SizeBytes))
		defer fin()
		previa, err := previaCifrada(c, g, f.Fichero)
		//nolint:contextcheck // Sólo reconstruye UI; los eventos futuros capturan la vida de cuenta en enSesion.
		a.hilo(gen, func() {
			if a.ed != e {
				return
			}
			e.cargando = false
			if err != nil {
				e.aviso = a.v.Traduce("gallery.encrypted.failed")
				a.pintarEditor()
				return
			}
			e.original = previa
			a.pintarEditor()
		})
	})
}

// previaCifrada es la foto descifrada, orientada por su EXIF y reducida a
// [LadoPreviaCifrada].
func previaCifrada(ctx context.Context, g *galeriacifrada.Galeria, f api.File) (image.Image, error) {
	datos, err := g.LeerOriginal(ctx, f, render.MaxOriginal)
	if err != nil {
		return nil, err
	}
	defer clear(datos)
	release, err := miniatura.TurnoConContexto(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	meta, _ := exif.Leer(datos) // sin EXIF no es un fallo
	img, err := render.Imagen(ctx, datos, model.OrientacionDe(int(meta.Orientacion)), model.RecetaVacia())
	if err != nil {
		return nil, err
	}
	return miniatura.Reducir(img, LadoPreviaCifrada), nil
}

// guardarEdicionCifrada guarda la receta como versión nueva cifrada. Al volver, el
// editor se cierra y la galería enseña la foto editada.
func (a *App) guardarEdicionCifrada(e *estadoEditor) {
	g := a.cif.galeria
	if g == nil {
		return
	}
	e.guardando = true
	a.pintarEditor()
	receta, f, gen := recetaDeModelo(e.receta), *e.cifrada, a.gen
	a.enSesion(func(vida context.Context) {
		c, fin := context.WithTimeout(vida, PlazoSubida(f.Fichero.SizeBytes))
		defer fin()
		hecha, err := g.Editar(c, f, receta)
		//nolint:contextcheck // Sólo reconstruye UI; los eventos futuros capturan la vida de cuenta en enSesion.
		a.hilo(gen, func() {
			if a.ed != e {
				return
			}
			e.guardando = false
			if err != nil {
				e.aviso = a.v.Traduce("editor.saveFailed")
				a.pintarEditor()
				return
			}
			if a.cif.galeria == g {
				a.cambiarCifrada(hecha)
			}
			for _, t := range []*Fototeca{a.v.Teca, a.v.TecaGrande} {
				if t != nil {
					t.Tirar(idDeLinea(f.Fichero.ID))
				}
			}
			a.cerrarEditor()
			a.avisar(ui.AvisoExito, a.v.Traduce("gallery.encrypted.saved"), "")
			a.Ir(RutaFotos)
		})
	})
}

// recetaDeModelo pasa la receta del editor al modelo del motor de Photos: los mismos
// pasos, en el mismo orden.
func recetaDeModelo(r api.Receta) model.Receta {
	m := model.Receta{Version: r.Version, Pasos: make([]model.Paso, 0, len(r.Pasos))}
	for _, p := range r.Pasos {
		m.Pasos = append(m.Pasos, model.Paso{Tipo: model.TipoPaso(p.Tipo), Params: maps.Clone(p.Params), Eje: p.Eje})
	}
	return m
}
