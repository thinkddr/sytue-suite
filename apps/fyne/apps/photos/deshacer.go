// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

// Espejo de `apps/photos/src/lib/deshacer.ts` de la web.

import (
	"context"
	"math"
	"slices"
	"sync"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # La papelera no pregunta: se deshace (docs/design.md §5.4-5.5)
//
// Mandar a la papelera es reversible (`POST /assets/{id}/restore`), así que ni el visor
// ni la barra de selección preguntan antes: lo que lo deshace es el aviso de 10 s, con
// su «Deshacer». Para que ese botón valga tiene que devolver ESA tanda —va en su cierre,
// no es «la última que se tiró»— y a SU sitio en la galería, no al final. Y si en esos
// 10 s la galería ya es otra (otra búsqueda, otra sección), se restaura igual pero no se
// pinta: meter las fotos devueltas en una lista que no las pidió sería peor.

// tirada es lo que se tiró de una vez, con lo necesario para devolverlo: las fotos tal
// como se veían, el orden de la galería en ese momento y qué galería era.
type tirada struct {
	fotos []api.Photo
	// orden son los ids de la galería en el orden en que se veían al tirar.
	orden []string
	// gen y ruta son la galería que se veía: `gen` cambia con cada búsqueda nueva.
	gen  int
	ruta Ruta
	// identidad es la cuenta que la tiró: con otra, el «Deshacer» ya no es suyo.
	identidad string
}

// nuevaTirada guarda la tanda que acaba de ir a la papelera: solo las que el servicio
// aceptó.
func (a *App) nuevaTirada(lista []api.Photo, hechas []string, gen int, ruta Ruta) tirada {
	t := tirada{orden: idsDe(lista), gen: gen, ruta: ruta, identidad: a.identidad}
	for _, f := range lista {
		if slices.Contains(hechas, f.ID) {
			t.fotos = append(t.fotos, f)
		}
	}
	return t
}

// reponer pone las devueltas donde estaban: cada una detrás de la última foto que la
// precedía y sigue en la lista. Lo que se ha movido o cargado entretanto se queda donde
// está, y una que ya está (por una recarga) no se repite.
func reponer(actual, devueltas []api.Photo, orden []string) []api.Photo {
	pos := make(map[string]int, len(orden))
	for i, id := range orden {
		pos[id] = i
	}
	suya := func(id string) int {
		if p, ok := pos[id]; ok {
			return p
		}
		return math.MaxInt
	}
	lista := slices.Clone(actual)
	presentes := make(map[string]bool, len(lista))
	for _, f := range lista {
		presentes[f.ID] = true
	}
	enOrden := slices.Clone(devueltas)
	slices.SortStableFunc(enOrden, func(x, y api.Photo) int { return suya(x.ID) - suya(y.ID) })
	for _, f := range enOrden {
		if presentes[f.ID] {
			continue
		}
		p, detras := suya(f.ID), -1
		for i, x := range lista {
			if q, ok := pos[x.ID]; ok && q < p {
				detras = i
			}
		}
		lista = slices.Insert(lista, detras+1, f)
		presentes[f.ID] = true
	}
	return lista
}

// enGaleria dice si lo que se ve es la galería de la tirada: misma sección y misma
// búsqueda.
func (a *App) enGaleria(t tirada) bool {
	return a.gen == t.gen && a.ruta == t.ruta && (a.ruta == RutaFotos || a.ruta == RutaFavoritas)
}

// avisar apila un aviso de la casa. Va en el hilo de la interfaz.
func (a *App) avisar(tono ui.TonoAviso, titulo, descripcion string) {
	a.avisos.Notificar(ui.NuevoAviso(tono, titulo, descripcion))
}

// avisarTirada dice «N fotos movidas a la papelera · El fichero se queda en Sytue Drive»
// durante los 10 s de ui.DuracionDeshacer, con «Deshacer». Va en el hilo de la interfaz.
func (a *App) avisarTirada(t tirada) {
	n := len(t.fotos)
	if n == 0 {
		return
	}
	av := ui.NuevoAviso(ui.AvisoExito, a.v.T.T("gallery.trashed", i18n.Vars{"count": n}),
		a.v.Traduce("gallery.trashedStays", i18n.Vars{"count": n}))
	av.Duracion = ui.DuracionDeshacer
	av.RotuloAccion = a.v.Traduce("gallery.undo")
	av.AlPulsarAccion = func() { a.deshacerTirada(t) }
	a.avisos.Notificar(av)
}

// deshacerTirada restaura cada foto de la tanda (el servicio lo hace por id: en tandas,
// como Drive) y, si la galería que se ve AL VOLVER es la de cuando se tiró, las repone
// en su sitio. Si alguna no vuelve, se dice cuántas.
func (a *App) deshacerTirada(t tirada) {
	// Con otra cuenta, esos ids son de la anterior: restaurarlos con las credenciales
	// de esta solo daría un «no se pudieron restaurar» falso.
	if a.cli == nil || a.identidad != t.identidad {
		return
	}
	a.enSesion(func(vida context.Context) {
		hechas, fallidas := ui.EnTandas(t.fotos, func(f api.Photo) error {
			c, fin := ctx(vida)
			defer fin()
			return a.cli.Restaurar(c, f.ID)
		}, ui.EnParalelo)
		// No va por `hilo`: lo restaurado ha vuelto aunque la pantalla sea otra, y el
		// aviso lo tiene que decir igual. Solo el repintar la galería depende de ella.
		//nolint:contextcheck // Sólo reconstruye UI; los eventos futuros capturan la vida de cuenta en enSesion.
		fyne.Do(func() {
			if len(hechas) > 0 && a.enGaleria(t) {
				a.g.fotos = reponer(a.g.fotos, hechas, t.orden)
				a.pintar()
			}
			if len(fallidas) > 0 {
				a.avisar(ui.AvisoPeligro, a.v.T.T("gallery.restoreFailed", i18n.Vars{"count": len(fallidas)}), "")
			} else if len(hechas) > 0 {
				a.avisar(ui.AvisoExito, a.v.T.T("gallery.restored", i18n.Vars{"count": len(hechas)}), "")
			}
		})
	})
}

// tirarSinRepetir manda a la papelera las de `ids` que no estén ya de camino y llama a
// `listo` en el hilo de la interfaz con las que se mandaron y las que el servicio
// aceptó. Sin la pregunta de antes nada frena un segundo toque (o Supr mantenido, que
// repite la tecla) mientras la primera llamada sigue en vuelo: así cada foto va una
// sola vez. `listo` no corre si no había ninguna nueva o si cambió la cuenta.
//
// Sí corre aunque entretanto se haya buscado, cambiado de sección o refrescado la lista
// (`misma` lo dice): las fotos YA están en la papelera, y sin el aviso no habría con qué
// deshacerlo, que es justo lo que sustituye a la pregunta. Como en la web.
func (a *App) tirarSinRepetir(ids []string, listo func(nuevas, hechas []string, fallo error, misma bool)) {
	if a.cli == nil {
		return
	}
	if a.tirando == nil {
		a.tirando = map[string]bool{}
	}
	var nuevas []string
	for _, id := range ids {
		if !a.tirando[id] {
			a.tirando[id] = true
			nuevas = append(nuevas, id)
		}
	}
	if len(nuevas) == 0 {
		return
	}
	gen, identidad := a.gen, a.identidad
	a.enSesion(func(vida context.Context) {
		var mu sync.Mutex
		var fallo error
		hechas, _ := ui.EnTandas(nuevas, func(id string) error {
			c, fin := ctx(vida)
			defer fin()
			err := a.cli.Trash(c, id)
			if err != nil {
				// El primer motivo, para decirlo como lo dice el visor.
				mu.Lock()
				if fallo == nil {
					fallo = err
				}
				mu.Unlock()
			}
			return err
		}, ui.EnParalelo)
		//nolint:contextcheck // Sólo reconstruye UI; los eventos futuros capturan la vida de cuenta en enSesion.
		fyne.Do(func() {
			for _, id := range nuevas {
				delete(a.tirando, id)
			}
			if identidad == a.identidad {
				listo(nuevas, hechas, fallo, gen == a.gen)
			}
		})
	})
}
