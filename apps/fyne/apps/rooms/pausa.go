// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// «Pausar avisos» (Slack «Pause notifications»), gemelo de lib/pausa.ts: hasta cuándo
// no avisa Rooms de nada. El instante vive en el servicio (`paused_until`, mig. 0061),
// así que vale en todos los dispositivos, y cada cliente lo compara con su reloj, como
// la ventana «No molestar». Aquí lo aplica la barra de la suite: mientras dura, los
// avisos de Rooms no salen al sistema (`ui.BarraSuite.CallarAvisosSi`). Se lee al
// arrancar y se RELEE al volver a la app y al reabrir el tiempo real tras un corte:
// una pausa puesta en otro dispositivo llega aquí entonces, no al instante.

// DuracionPausa es una de las cinco opciones del menú.
type DuracionPausa string

// Las cinco duraciones, con su sufijo de catálogo (`pause.<d>`).
const (
	Pausa30m    DuracionPausa = "30m"
	Pausa1h     DuracionPausa = "1h"
	Pausa2h     DuracionPausa = "2h"
	PausaManana DuracionPausa = "tomorrow"
	PausaLunes  DuracionPausa = "monday"
)

// DuracionesPausa es `DURACIONES_PAUSA`, en el orden del menú.
var DuracionesPausa = []DuracionPausa{Pausa30m, Pausa1h, Pausa2h, PausaManana, PausaLunes}

// FinDePausa es `finDePausa`: las tres primeras cuentan desde ahora; mañana y el lunes
// son a las 9:00 locales, los mismos instantes que «Enviar más tarde».
func FinDePausa(d DuracionPausa, ahora time.Time) time.Time {
	switch d {
	case Pausa30m:
		return ahora.Add(30 * time.Minute)
	case Pausa1h:
		return ahora.Add(time.Hour)
	case Pausa2h:
		return ahora.Add(2 * time.Hour)
	case PausaManana, PausaLunes:
	}
	manana, lunes := PresetsProgramar(ahora)
	if d == PausaManana {
		return manana
	}
	return lunes
}

// PausaActiva es `pausaActiva`: hay pausa mientras su hora no ha pasado.
func PausaActiva(hasta, ahora time.Time) bool {
	return !hasta.IsZero() && hasta.After(ahora)
}

// ---- En la Sala ----

// pausaHasta es el fin de la pausa vigente; el cero, ninguna. Atómico porque lo lee
// la barra de avisos desde la rutina del flujo del Hub.
func (sa *Sala) pausaHasta() time.Time {
	n := sa.pausa.Load()
	if n == 0 {
		return time.Time{}
	}
	return time.Unix(0, n)
}

func (sa *Sala) ponerPausa(p api.RoomsNotifyPrefs) {
	if p.PausedUntil == nil {
		sa.pausa.Store(0)
		return
	}
	sa.pausa.Store(p.PausedUntil.UnixNano())
}

// AvisosEnPausa dice si ahora mismo los avisos están en pausa. Es lo que la barra de la
// suite pregunta antes de sacar un aviso de Rooms al sistema.
func (sa *Sala) AvisosEnPausa() bool { return PausaActiva(sa.pausaHasta(), sa.ahora()) }

// cargarPausa lee la pausa; si falla, se queda la que había (al arrancar, ninguna: el
// lado que avisa). Si entretanto se pausó o se reanudó desde aquí, la respuesta es de
// antes y no pisa lo que el servicio acaba de confirmar.
func (sa *Sala) cargarPausa() {
	turno := sa.pausaTurno.Load()
	pedir(sa, sa.S.NotifyPrefs, func(p api.RoomsNotifyPrefs, err error) {
		if err == nil && sa.pausaTurno.Load() == turno {
			sa.ponerPausa(p)
		}
	})
}

// EnPrimerPlano relee la pausa al volver a la app, por si se puso o se quitó en otro
// dispositivo mientras tanto, y quita del icono lo que llegó al canal abierto con la
// app en segundo plano: ya está a la vista. Antes de arrancar no hay nada que releer.
func (sa *Sala) EnPrimerPlano() {
	sa.enFondo = false
	if len(sa.noVistos) > 0 {
		clear(sa.noVistos)
		sa.decirNoLeidos()
	}
	if sa.ctx == nil {
		return
	}
	sa.cargarPausa()
}

// PausarAvisos pausa con esa duración, o reanuda con "". La pausa se apunta cuando el
// servicio la confirma; si dice que no, se avisa y queda como estaba. Mientras una
// escritura está en vuelo no sale otra (`pauseSaving` de la web): las dos respuestas
// podían llegar en cualquier orden y ganaba la última en llegar, no la última pedida.
func (sa *Sala) PausarAvisos(d DuracionPausa) {
	if sa.pausaGuardando {
		return
	}
	var hasta time.Time
	if d != "" {
		hasta = FinDePausa(d, sa.ahora())
	}
	sa.pausaGuardando = true
	sa.pausaTurno.Add(1)
	pedir(sa, func(ctx context.Context) (api.RoomsNotifyPrefs, error) {
		return sa.S.SetNotifyPause(ctx, hasta)
	}, func(p api.RoomsNotifyPrefs, err error) {
		sa.pausaGuardando = false
		sa.pausaTurno.Add(1)
		if err != nil {
			sa.P.Avisar(ui.AvisoPeligro, sa.P.T.T("pause.failed", nil))
			return
		}
		sa.ponerPausa(p)
	})
}

// entradasPausa son las del menú de «tu estado»: en pausa, hasta cuándo y «Reanudar
// avisos»; si no, el título y las cinco duraciones. Con una escritura en vuelo van
// inertes, como el `disabled={pauseSaving}` de StatusMenu.
func (sa *Sala) entradasPausa(t *i18n.Translator, l i18n.Locale, cerrar func()) []fyne.CanvasObject {
	if hasta := sa.pausaHasta(); PausaActiva(hasta, sa.ahora()) {
		reanudar := ui.NuevoElementoMenu(t.T("pause.resume", nil), iconos.Campana, func() {
			cerrar()
			sa.PausarAvisos("")
		})
		reanudar.Inerte = sa.pausaGuardando
		return []fyne.CanvasObject{
			textoMenu(t.T("pause.until", i18n.Vars{"when": RotuloProgramado(l, hasta.Local())}), ui.Normal),
			reanudar,
		}
	}
	out := []fyne.CanvasObject{textoMenu(t.T("pause.title", nil), ui.Medio)}
	for _, d := range DuracionesPausa {
		texto := t.T("pause."+string(d), nil)
		e := ui.NuevoElementoMenu(texto, iconos.Ninguno, func() {
			cerrar()
			sa.PausarAvisos(d)
		})
		// El título va en el nombre accesible: quien recorre el menú por sus ítems no
		// pasa por el rótulo de encima, y oía «Durante 1 hora» sin saber de qué.
		e.Rotulo = t.T("pause.optionAria", i18n.Vars{"option": texto})
		e.Inerte = sa.pausaGuardando
		out = append(out, e)
	}
	return out
}

// textoMenu es un `<p class="text-text-muted px-3 py-1 text-xs">` dentro del menú.
func textoMenu(texto string, peso ui.Peso) fyne.CanvasObject {
	return container.New(layout.NewCustomPaddedLayout(tema.Espacio(1), tema.Espacio(1), tema.Espacio(3), tema.Espacio(3)),
		ui.NuevoRotulo(texto, tema.TextoXS, peso, "text-muted"))
}
