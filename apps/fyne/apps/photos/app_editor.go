// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// paridad-web: apps/frontend/apps/photos/src/pages/edit.astro

package photos

import (
	"bytes"
	"context"
	"image"
	"io"
	"net/url"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// El editor vivo: el estado que en la web vive en los `useState` de Editor.tsx. Se abre a
// pantalla completa sobre la app, como la página `/edit` de la web, y al salir vuelve a
// la galería (o a la carpeta de Drive de la que se vino).

// maxPrevia es lo más que se acepta de la previa (2048 px de lado, unos 2 MB en JPEG).
const maxPrevia = 32 << 20

type estadoEditor struct {
	foto             api.Photo
	volver           string // la carpeta de Drive de la que se vino; vacía, la galería
	receta, guardada api.Receta
	original         image.Image
	claveGeo         string
	fuente           *image.NRGBA

	cargando, guardando bool
	comparar            bool
	aviso               string
	pestanaIA           bool
	recortando          bool
	proporcion          float64

	iaCargando  bool
	iaError     string
	iaResultado *api.EdicionIA

	lienzo     *LienzoEditor
	mandos     map[string]*ui.Deslizador
	hayMarco   bool
	peticion   *AreaFicha
	capa       *capaVisor
	deshacerAt *desktop.CustomShortcut
	guardarAt  *desktop.CustomShortcut
}

// AbrirEditor abre el editor con esa foto. `volver` es la carpeta de Drive de la que se
// vino, si se vino de allí.
func (a *App) AbrirEditor(f api.Photo, volver string) {
	a.cerrarVisor()
	a.cerrarEditor()
	e := &estadoEditor{
		foto: f, volver: volver, receta: api.RecetaVacia(), guardada: api.RecetaVacia(),
		cargando: true, lienzo: NuevoLienzoEditor(),
	}
	e.peticion = NuevaAreaPeticion(a.v)
	// «Editar con IA» se enciende al escribir: se repinta solo cuando cambia si hay
	// algo o no, no a cada letra.
	vacia := true
	e.peticion.AlCambiar = func(s string) {
		if nueva := strings.TrimSpace(s) == ""; nueva != vacia {
			vacia = nueva
			a.pintarEditor()
		}
	}
	e.mandos = map[string]*ui.Deslizador{}
	for _, tipo := range Deslizadores {
		lo, hi := rangoAjuste(tipo)
		m := ui.NuevoDeslizador(lo, hi, 0, func(x float64) {
			e.receta = conValor(e.receta, tipo, x)
			a.pintarEditor()
		}).Con(a.v.Traduce(claveAjuste(tipo)))
		// `onDblClick`: el doble clic devuelve el ajuste a 0 (y quita su paso).
		m.AlDobleClic = func() {
			m.Valor = 0
			e.receta = conValor(e.receta, tipo, 0)
			a.pintarEditor()
		}
		e.mandos[tipo] = m
	}
	// Arrastrar el marco no rehace la pantalla a cada píxel: solo cuando «Aplicar» pasa
	// de apagado a encendido (o al revés).
	e.lienzo.AlMarco = func() {
		if m := e.lienzo.Marco; (m != nil && m.W > 0.01) != e.hayMarco {
			a.pintarEditor()
		}
	}
	e.capa = nuevaCapaVisor(a.salirDelEditor)
	a.ed = e
	a.pintarEditor()
	if a.w != nil {
		c := a.w.Canvas()
		e.capa.mostrar(c)
		c.Unfocus()
		// Los atajos con modificador: deshacer y guardar (Ctrl o Cmd, según el sistema).
		e.deshacerAt = &desktop.CustomShortcut{KeyName: fyne.KeyZ, Modifier: fyne.KeyModifierShortcutDefault}
		e.guardarAt = &desktop.CustomShortcut{KeyName: fyne.KeyS, Modifier: fyne.KeyModifierShortcutDefault}
		c.AddShortcut(e.deshacerAt, func(fyne.Shortcut) { a.deshacerEditor() })
		c.AddShortcut(e.guardarAt, func(fyne.Shortcut) {
			if a.ed != nil && a.ed.sucia() && !a.ed.guardando && !a.ed.cargando {
				a.guardarEdicion()
			}
		})
	}
	a.cargarEditor(e)
}

func (e *estadoEditor) sucia() bool { return !e.receta.Igual(e.guardada) }

// cargarEditor trae, a la vez, la receta guardada y la previa del ORIGINAL. La derivada
// vigente lleva la receta horneada, y volver a aplicársela enseñaría cada ajuste dos
// veces; el original se pide por la huella de la receta vacía.
func (a *App) cargarEditor(e *estadoEditor) {
	if a.cli == nil {
		return
	}
	gen := a.gen
	a.enSesion(func(vida context.Context) {
		c, fin := ctx(vida)
		defer fin()
		var receta api.Receta
		var original image.Image
		errReceta := make(chan error, 1)
		go func() {
			var err error
			receta, err = a.cli.LeerReceta(c, e.foto.ID)
			errReceta <- err
		}()
		cuerpo, err := a.cli.VariantDe(c, e.foto.ID, api.PhotoPreview, api.RecetaVacia().Huella())
		if err == nil {
			var datos []byte
			datos, err = io.ReadAll(io.LimitReader(cuerpo, maxPrevia))
			_ = cuerpo.Close()
			if err == nil {
				original, _, err = image.Decode(bytes.NewReader(datos))
			}
		}
		if e2 := <-errReceta; err == nil {
			err = e2
		}
		//nolint:contextcheck // Sólo reconstruye UI; los eventos futuros capturan la vida de cuenta en enSesion.
		a.hilo(gen, func() {
			if a.ed != e {
				return
			}
			e.cargando = false
			if err != nil {
				e.aviso = a.v.Traduce("editor.saveFailed")
				a.pintarEditor()
				return
			}
			e.original, e.receta, e.guardada = original, receta, receta.Clonar()
			a.pintarEditor()
		})
	})
}

// cerrarEditor retira la capa y los atajos.
func (a *App) cerrarEditor() {
	e := a.ed
	if e == nil {
		return
	}
	a.ed = nil
	if a.w != nil {
		c := a.w.Canvas()
		c.Overlays().Remove(e.capa)
		c.RemoveShortcut(e.deshacerAt)
		c.RemoveShortcut(e.guardarAt)
	}
}

// salirDelEditor es «← Volver a la galería» (o a Drive). Escape también, salvo que se
// esté recortando: ahí cancela el recorte.
func (a *App) salirDelEditor() {
	e := a.ed
	if e == nil {
		return
	}
	if e.recortando {
		a.cancelarRecorte()
		return
	}
	a.cerrarEditor()
	if e.volver != "" && a.AbrirApp != nil {
		a.AbrirApp("drive", "?path="+url.QueryEscape(e.volver))
		return
	}
	// La web vuelve a `./`, que es la galería, y la pide otra vez: lo editado se ve con
	// su derivada nueva.
	a.Ir(RutaFotos)
}

// pintarEditor rehace el editor con su estado. La geometría solo se rehace si cambió un
// paso de geometría; el color lo pone el lienzo al pintar.
func (a *App) pintarEditor() {
	e := a.ed
	if e == nil {
		return
	}
	recetaVista := e.receta
	if e.comparar {
		recetaVista = sinColor(e.receta)
	}
	if e.original != nil {
		if k := claveGeometria(recetaVista); e.fuente == nil || k != e.claveGeo {
			e.fuente, e.claveGeo = aplicarGeometria(e.original, recetaVista), k
		}
		e.lienzo.Poner(e.fuente, recetaVista, a.fondoLienzo())
	}
	e.lienzo.Recortando, e.lienzo.Proporcion = e.recortando, e.proporcion
	e.hayMarco = e.lienzo.Marco != nil && e.lienzo.Marco.W > 0.01
	d := Editor{
		Foto: e.foto, Receta: e.receta, Sucia: e.sucia(), Lienzo: e.lienzo,
		Cargando: e.cargando, Guardando: e.guardando, Aviso: e.aviso,
		VolverDrive: e.volver != "", PestanaIA: e.pestanaIA,
		Comparar:   e.comparar,
		Recortando: e.recortando, HayMarco: e.hayMarco,
		Proporcion: e.proporcion, Fondo: a.fondoLienzo(),
		Peticion: e.peticion, Mandos: e.mandos, IACargando: e.iaCargando, IAError: e.iaError, IAResultado: e.iaResultado,

		AlVolver:   a.salirDelEditor,
		AlDeshacer: a.deshacerEditor,
		AlGuardar:  a.guardarEdicion,
		AlComparar: a.alternarComparar,
		AlAjuste: func(tipo string, v float64) {
			e.receta = conValor(e.receta, tipo, v)
			a.pintarEditor()
		},
		AlPestana:     func(ia bool) { e.pestanaIA = ia; a.pintarEditor() },
		AlRecortar:    a.alternarRecorte,
		AlGirar:       func() { a.anyadirPaso(api.PasoReceta{Tipo: api.PasoGiro, Params: map[string]float64{"grados": 90}}) },
		AlEspejoH:     func() { a.anyadirPaso(api.PasoReceta{Tipo: api.PasoEspejo, Eje: "h"}) },
		AlEspejoV:     func() { a.anyadirPaso(api.PasoReceta{Tipo: api.PasoEspejo, Eje: "v"}) },
		AlRestablecer: func() { e.receta = api.RecetaVacia(); a.pintarEditor() },
		AlProporcion: func(p float64) {
			e.proporcion = p
			e.lienzo.AjustarProporcion(p)
			a.pintarEditor()
		},
		AlAplicarRecorte:  a.aplicarRecorte,
		AlCancelarRecorte: a.cancelarRecorte,
		AlFondo: func(f FondoLienzo) {
			a.ponerFondo(f)
			a.pintarEditor()
		},
		AlIA: a.editarConIA,
	}
	e.capa.poner(PantallaEditor(a.v, d))
}

// anyadirPaso añade un paso de geometría (girar, espejar).
func (a *App) anyadirPaso(p api.PasoReceta) {
	if e := a.ed; e != nil && !e.cargando {
		e.receta = conPaso(e.receta, p)
		a.pintarEditor()
	}
}

// deshacerEditor quita el último paso.
func (a *App) deshacerEditor() {
	e := a.ed
	if e == nil || len(e.receta.Pasos) == 0 {
		return
	}
	c := e.receta.Clonar()
	c.Pasos = c.Pasos[:len(c.Pasos)-1]
	e.receta = c
	if !conAjusteDeColor(e.receta) {
		e.comparar = false
	}
	a.pintarEditor()
}

// alternarComparar enseña la foto sin ajustes de color («Comparar con el original»).
// La tecla `\` y el botón con el icono de ojo también la alternan.
func (a *App) alternarComparar() {
	e := a.ed
	if e == nil || e.cargando || !conAjusteDeColor(e.receta) {
		return
	}
	e.comparar = !e.comparar
	a.pintarEditor()
}

// alternarRecorte es la tecla «c» y el mando del recorte: entra o sale, y el marco se
// olvida.
func (a *App) alternarRecorte() {
	e := a.ed
	if e == nil || e.cargando {
		return
	}
	e.recortando = !e.recortando
	e.lienzo.Marco = nil
	a.pintarEditor()
}

// aplicarRecorte añade el recorte si el marco tiene tamaño (más del 1 % en cada lado).
func (a *App) aplicarRecorte() {
	e := a.ed
	if e == nil {
		return
	}
	if m := e.lienzo.Marco; m != nil && m.W > 0.01 && m.H > 0.01 {
		e.receta = conPaso(e.receta, api.PasoReceta{Tipo: api.PasoRecorte, Params: map[string]float64{
			"x": m.X, "y": m.Y, "ancho": m.W, "alto": m.H,
		}})
	}
	e.lienzo.Marco, e.recortando = nil, false
	a.pintarEditor()
}

func (a *App) cancelarRecorte() {
	if e := a.ed; e != nil {
		e.lienzo.Marco, e.recortando = nil, false
		a.pintarEditor()
	}
}

// guardarEdicion manda la receta ENTERA. La galería verá lo editado al volver a pedirla.
func (a *App) guardarEdicion() {
	e := a.ed
	if e == nil || e.guardando || a.cli == nil || !e.sucia() {
		return
	}
	e.guardando = true
	a.pintarEditor()
	receta, gen := e.receta.Clonar(), a.gen
	a.enSesion(func(vida context.Context) {
		c, fin := ctx(vida)
		defer fin()
		huella, err := a.cli.GuardarReceta(c, e.foto.ID, receta)
		//nolint:contextcheck // Sólo reconstruye UI; los eventos futuros capturan la vida de cuenta en enSesion.
		a.hilo(gen, func() {
			if a.ed != e {
				return
			}
			e.guardando = false
			if err != nil {
				e.aviso = a.v.Traduce("editor.saveFailed")
			} else {
				e.guardada, e.aviso = receta, a.v.Traduce("editor.saved")
				// Lo que la galería y el visor tenían de esta foto ya no es lo que el
				// servicio sirve.
				a.huellas.Store(e.foto.ID, huella)
				for _, t := range []*Fototeca{a.v.Teca, a.v.TecaGrande} {
					if t != nil {
						t.Tirar(e.foto.ID)
					}
				}
			}
			a.pintarEditor()
		})
	})
}

// editarConIA pide a Sytue AI un revelado (un preajuste o lo escrito) y aplica lo que
// propone a la receta: el recorte como un paso más y cada ajuste en su deslizador.
func (a *App) editarConIA(accion string) {
	e := a.ed
	if e == nil || e.iaCargando || a.cli == nil {
		return
	}
	peticion := strings.TrimSpace(e.peticion.Texto())
	if accion == "" && peticion == "" {
		return
	}
	if a.IA == nil {
		e.iaError = a.v.Traduce("editor.aiFailed")
		a.pintarEditor()
		return
	}
	e.iaCargando, e.iaError = true, ""
	a.pintarEditor()
	gen := a.gen
	a.enSesion(func(vida context.Context) {
		c, fin := ctxIA(vida)
		defer fin()
		r, err := a.cli.EditarConIA(c, a.IA, e.foto.ID, accion, peticion, a.idioma())
		//nolint:contextcheck // Sólo reconstruye UI; los eventos futuros capturan la vida de cuenta en enSesion.
		a.hilo(gen, func() {
			if a.ed != e {
				return
			}
			e.iaCargando = false
			if err != nil {
				e.iaError = a.v.Traduce("editor.aiFailed")
				a.pintarEditor()
				return
			}
			e.iaResultado = &r
			if rc := r.Recorte; rc != nil {
				e.receta = conPaso(e.receta, api.PasoReceta{Tipo: api.PasoRecorte, Params: map[string]float64{
					"x": rc.X, "y": rc.Y, "ancho": rc.Ancho, "alto": rc.Alto,
				}})
			}
			// En el orden de DESLIZADORES: el orden de los pasos de color cambia la foto.
			for i, v := range [...]*float64{r.Ajustes.Exposicion, r.Ajustes.Contraste, r.Ajustes.Saturacion, r.Ajustes.Temperatura} {
				if v != nil {
					e.receta = conValor(e.receta, Deslizadores[i], *v)
				}
			}
			e.aviso = a.v.Traduce("editor.aiResultTitle")
			a.pintarEditor()
		})
	})
}

// teclaEditor: Escape cancela el recorte o sale.
func (a *App) teclaEditor(k *fyne.KeyEvent) {
	if k.Name == fyne.KeyEscape {
		a.salirDelEditor()
	}
}

// runaEditor: «r» gira y «c» entra o sale del recorte, como en la web; sin modificador,
// y nunca escribiendo en un campo (a un campo enfocado no le llega la runa suelta).
func (a *App) runaEditor(r rune) {
	switch r {
	case 'r', 'R':
		a.anyadirPaso(api.PasoReceta{Tipo: api.PasoGiro, Params: map[string]float64{"grados": 90}})
	case 'c', 'C':
		a.alternarRecorte()
	case '\\':
		a.alternarComparar()
	}
}
