// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"context"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Sytue AI en Photos: describir y preguntar desde el visor, describir en lote desde la
// selección y ampliar una búsqueda que no encontró nada. La web lo hace por su BFF; aquí
// va en directo al servicio de IA con el token de la sesión (`api.IA`), y las
// instrucciones y los lectores de respuesta son los mismos (`api/photos_ia.go`).

// TopeDescribir es el tope por pasada del lote: cada foto es una llamada de visión, y se
// cobra.
const TopeDescribir = 50

// ctxIA es el de una llamada de visión: los 70 s de la web más la miniatura.
func ctxIA(vida context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(vida, 75*time.Second)
}

// idioma es la lengua de la sesión, en la que Sytue AI contesta.
func (a *App) idioma() string { return string(a.v.T.Locale()) }

// falloIA es el texto de un fallo de Sytue AI: «no está configurada» es otra cosa que
// «ha fallado», y la web los distingue.
func (a *App) falloIA(err error, clave string) string {
	if a.IA == nil || api.IANoConfigurada(err) {
		return a.v.Traduce("viewer.describeUnavailable")
	}
	return a.v.Traduce(clave)
}

// ponerDescripcion deja la descripción nueva en la foto, en todas las listas donde
// esté: es el `setFotos(l => l.map(...))` que la web hace con `onDescribe`.
func (a *App) ponerDescripcion(id, descripcion string, etiquetas []string) {
	for _, l := range [][]api.Photo{a.g.fotos, a.uno.fotos, a.enc.fotos, a.vistaSuelta()} {
		for i := range l {
			if l[i].ID == id {
				l[i].Description, l[i].Tags = descripcion, etiquetas
			}
		}
	}
}

// vistaSuelta es la lista de la foto que llegó sola, si el visor la enseña.
func (a *App) vistaSuelta() []api.Photo {
	if a.vis == nil {
		return nil
	}
	return a.vis.suelta
}

// describirFoto es «Describir con Sytue AI» del visor. Se GUARDA en cuanto vuelve, sin
// pedir confirmación: es el texto alternativo de la foto, y una descripción que espera
// aprobación no la lee ningún lector de pantalla.
func (a *App) describirFoto() {
	e := a.vis
	if e == nil || e.describiendo || a.cli == nil {
		return
	}
	f, ok := e.foto()
	if !ok {
		return
	}
	if a.IA == nil {
		a.avisarVisor(a.falloIA(nil, ""))
		return
	}
	e.describiendo, e.aviso = true, a.v.Traduce("viewer.describing")
	a.pintarVisor()
	gen := a.gen
	a.enSesion(func(vida context.Context) {
		d, et, err := a.describirYGuardar(vida, f.ID)
		//nolint:contextcheck // Sólo reconstruye UI; los eventos futuros capturan la vida de cuenta en enSesion.
		a.hilo(gen, func() {
			if a.vis != e {
				return
			}
			e.describiendo = false
			if err != nil {
				e.aviso = a.falloIA(err, "viewer.describeFailed")
			} else {
				a.ponerDescripcion(f.ID, d, et)
				e.conFicha, e.aviso = true, a.v.Traduce("viewer.describedInInfo")
				a.pintar()
			}
			a.pintarVisor()
		})
	})
}

// describirYGuardar le pide a Sytue AI lo que ve en la foto y lo guarda. Corre FUERA del
// hilo de la interfaz.
func (a *App) describirYGuardar(vida context.Context, id string) (string, []string, error) {
	if a.IA == nil {
		return "", nil, api.ErrIAIlegible // falloIA lo dice como «no configurada»
	}
	c, fin := ctxIA(vida)
	defer fin()
	d, et, err := a.cli.Describir(c, a.IA, id, a.idioma())
	if err == nil {
		err = a.cli.SetDescription(c, id, d, et)
	}
	return d, et, err
}

// preguntarFoto es el `<form>` de «Pregunta sobre esta foto». La respuesta NO se guarda.
func (a *App) preguntarFoto(pregunta string) {
	e := a.vis
	pregunta = strings.TrimSpace(pregunta)
	if e == nil || pregunta == "" || e.pensando || a.cli == nil {
		return
	}
	f, ok := e.foto()
	if !ok {
		return
	}
	if a.IA == nil {
		e.respuesta = a.falloIA(nil, "")
		a.pintarVisor()
		return
	}
	e.pensando, e.respuesta = true, ""
	a.pintarVisor()
	gen := a.gen
	a.enSesion(func(vida context.Context) {
		c, fin := ctxIA(vida)
		defer fin()
		r, err := a.cli.Preguntar(c, a.IA, f.ID, pregunta, a.idioma())
		//nolint:contextcheck // Sólo reconstruye UI; los eventos futuros capturan la vida de cuenta en enSesion.
		a.hilo(gen, func() {
			if a.vis != e {
				return
			}
			e.pensando, e.respuesta = false, r
			if err != nil {
				e.respuesta = a.falloIA(err, "viewer.askFailed")
			}
			a.pintarVisor()
		})
	})
}

// editarDescripcion abre el área con la descripción de ahora dentro.
func (a *App) editarDescripcion() {
	e := a.vis
	f, ok := e.foto()
	if !ok {
		return
	}
	if e.campoDesc == nil {
		// `rows={3} rounded-md border px-2.5 py-1.5 text-xs sm:text-sm bg-bg`.
		e.campoDesc = NuevaAreaFicha(a.v.Traduce("viewer.infoDescriptionEmpty"), textoArea(a.v), 3,
			tema.Espacio(2.5), tema.Espacio(1.5), tema.RadioMD, "bg")
	}
	e.campoDesc.PonerTexto(f.Description)
	e.editandoDesc = true
	a.pintarVisor()
}

// guardarDescripcion guarda lo escrito a mano, con las etiquetas que ya tenía.
func (a *App) guardarDescripcion() {
	e := a.vis
	f, ok := e.foto()
	if !ok || e.guardandoDesc || a.cli == nil || e.campoDesc == nil {
		return
	}
	nueva := strings.TrimSpace(e.campoDesc.Texto())
	e.guardandoDesc = true
	a.pintarVisor()
	gen := a.gen
	a.enSesion(func(vida context.Context) {
		c, fin := ctx(vida)
		defer fin()
		err := a.cli.SetDescription(c, f.ID, nueva, f.Tags)
		//nolint:contextcheck // Sólo reconstruye UI; los eventos futuros capturan la vida de cuenta en enSesion.
		a.hilo(gen, func() {
			if a.vis != e {
				return
			}
			e.guardandoDesc = false
			if err != nil {
				e.aviso = a.v.Traduce("viewer.describeFailed")
			} else {
				a.ponerDescripcion(f.ID, nueva, f.Tags)
				e.editandoDesc, e.aviso = false, a.v.Traduce("viewer.describedInInfo")
				a.pintar()
			}
			a.pintarVisor()
		})
	})
}

// describirSeleccion es «Describir con Sytue AI» en lote.
//
// DE UNA EN UNA Y EN SERIE: cincuenta a la vez se llevan el límite del servicio y el
// presupuesto de golpe. LAS QUE YA TIENEN DESCRIPCIÓN SE SALTAN: pagar dos veces por un
// texto parecido no es lo que se pidió. Y se puede parar a mitad: cada foto se guarda en
// cuanto vuelve.
func (a *App) describirSeleccion() {
	if a.g.seleccion == nil || a.g.lote != nil || a.cli == nil {
		return
	}
	var pendientes []api.Photo
	for _, f := range a.g.fotos {
		if a.g.seleccion[f.ID] && strings.TrimSpace(f.Description) == "" {
			pendientes = append(pendientes, f)
		}
	}
	if len(pendientes) == 0 {
		a.g.aviso = a.v.Traduce("gallery.describeNothing")
		a.pintar()
		return
	}
	lote := pendientes[:min(len(pendientes), TopeDescribir)]
	a.g.parar, a.g.lote = false, &[2]int{0, len(lote)}
	a.pintar()
	a.describirLote(a.gen, lote, 0, 0, len(pendientes) > TopeDescribir)
}

// describirLote describe la foto `i` y, al volver, la siguiente: así «Parar» se lee en
// el hilo de la interfaz, entre una llamada y otra.
func (a *App) describirLote(gen int, lote []api.Photo, i, hechas int, topado bool) {
	if i >= len(lote) || a.g.parar {
		a.g.lote = nil
		aviso := a.v.T.T("gallery.described", i18n.Vars{"count": hechas})
		if topado {
			aviso += " · " + a.v.T.T("gallery.describeCapped", i18n.Vars{"count": TopeDescribir})
		}
		a.g.aviso = aviso
		a.pintar()
		return
	}
	f := lote[i]
	a.enSesion(func(vida context.Context) {
		d, et, err := a.describirYGuardar(vida, f.ID)
		//nolint:contextcheck // Sólo reconstruye UI; los eventos futuros capturan la vida de cuenta en enSesion.
		a.hilo(gen, func() {
			if a.g.lote == nil {
				return
			}
			if err != nil {
				// Una foto que falla no para el lote: se pidió describir el álbum, no
				// describirlo entero o nada.
				a.g.error = a.falloIA(err, "viewer.describeFailed")
			} else {
				a.ponerDescripcion(f.ID, d, et)
				hechas++
			}
			a.g.lote = &[2]int{hechas, len(lote)}
			a.pintar()
			a.describirLote(gen, lote, i+1, hechas, topado)
		})
	})
}

// ampliarBusqueda le pide a Sytue AI otras formas de decir lo buscado y vuelve a buscar
// con ellas. SOLO A PETICIÓN, y solo se ofrece cuando la búsqueda no encontró nada.
func (a *App) ampliarBusqueda() {
	texto := strings.TrimSpace(a.g.texto)
	if a.g.ampliando || texto == "" {
		return
	}
	if a.IA == nil {
		a.g.aviso = a.v.Traduce("gallery.searchWiderNone")
		a.pintar()
		return
	}
	a.g.ampliando = true
	a.pintar()
	gen := a.gen
	a.enSesion(func(vida context.Context) {
		c, fin := ctx(vida)
		defer fin()
		terminos, err := api.TerminosRelacionados(c, a.IA, texto, a.idioma())
		//nolint:contextcheck // Sólo reconstruye UI; los eventos futuros capturan la vida de cuenta en enSesion.
		a.hilo(gen, func() {
			a.g.ampliando = false
			if err != nil || len(terminos) == 0 {
				a.g.aviso = a.v.Traduce("gallery.searchWiderNone")
				a.pintar()
				return
			}
			a.gen++
			a.g = estadoGaleria{
				texto: a.g.texto, tambien: terminos, seleccion: a.g.seleccion,
				estadoCarga: estadoCarga{aviso: a.v.T.T("gallery.searchWiderWith", i18n.Vars{"terms": strings.Join(terminos, ", ")})},
			}
			a.pintar()
			a.cargarMas()
		})
	})
}

// textoArea es el `text-xs sm:text-sm` del área de la descripción, con la línea de cada
// escalón (el `<textarea>` no lleva `leading-*`).
func textoArea(v Vista) tema.Escalon {
	if v.Ventana >= ui.CorteSM {
		return tema.TextoSM
	}
	return tema.TextoXS
}
