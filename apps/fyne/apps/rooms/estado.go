// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Contrapartida de StatusMenu.tsx y lib/status.ts dentro de la pantalla principal
// declarada en pantalla.go.
//
// El menú de «tu estado»: la presencia (En línea, Ausente, No molestar), el estado
// personalizado con su caducidad, y «Marcar todo como leído».
//
// «Activar/desactivar avisos de mensajes directos» es una preferencia del Hub, que es
// quien reparte los avisos a todas las apps y dispositivos (ver `avisosdirectos.go`).
// La presencia elegida se marca con el FONDO (`aria-current` con `bg-bg-subtle`), sin tic,
// como en StatusMenu.tsx.

// Caducidad es `StatusExpiry`.
type Caducidad int

// Las cinco, en el orden de `EXPIRY_OPTIONS`.
const (
	NoCaduca Caducidad = iota
	Caduca30m
	Caduca1h
	CaducaHoy
	CaducaSemana
)

var clavesCaducidad = []string{
	"status.expiry.never", "status.expiry.30m", "status.expiry.1h", "status.expiry.today", "status.expiry.week",
}

// CaducaEn es `expiryAt`: cuándo se borra el estado, o el cero si no caduca. «Hoy» es la
// próxima medianoche local y «esta semana», la medianoche del lunes que viene.
func CaducaEn(c Caducidad, ahora time.Time) time.Time {
	switch c {
	case Caduca30m:
		return ahora.Add(30 * time.Minute)
	case Caduca1h:
		return ahora.Add(time.Hour)
	case CaducaHoy, CaducaSemana:
		fin := time.Date(ahora.Year(), ahora.Month(), ahora.Day()+1, 0, 0, 0, 0, ahora.Location())
		for c == CaducaSemana && fin.Weekday() != time.Monday {
			fin = fin.AddDate(0, 0, 1)
		}
		return fin
	case NoCaduca:
	}
	return time.Time{}
}

// estadosElegibles son `SELECTABLE_STATES`, con su punto.
var estadosElegibles = []struct {
	estado api.EstadoPresencia
	punto  ui.Presencia
}{
	{api.EnLinea, ui.EnLinea}, {api.Ausente, ui.Ausente}, {api.NoMolestar, ui.NoMolestar},
}

// BloqueEstado es la parte del menú con el estado personalizado: `px-2 py-1.5`, la
// etiqueta (`.sytue-label font-medium`), la caja, «Quitar dentro de» con su selector y
// la fila de «Quitar» y «Guardar».
type BloqueEstado struct {
	widget.BaseWidget

	T       *i18n.Translator
	Actual  string
	caja    *ui.Entrada
	plazo   *ui.Selector
	columna *fyne.Container

	// AlGuardar recibe el texto y la caducidad; «Quitar» manda el texto vacío.
	AlGuardar func(texto string, c Caducidad)
}

// NuevoBloqueEstado crea el bloque con el estado vigente en la caja.
func NuevoBloqueEstado(t *i18n.Translator, actual string, alGuardar func(string, Caducidad)) *BloqueEstado {
	b := &BloqueEstado{T: t, Actual: actual, AlGuardar: alGuardar}
	b.ExtendBaseWidget(b)
	b.caja = ui.NuevaEntrada(t.T("status.customPlaceholder", nil))
	b.caja.PonerTexto(actual)
	b.caja.AlCambiar(func(s string) {
		if len([]rune(s)) > 100 {
			b.caja.PonerTexto(string([]rune(s)[:100])) // `maxLength={100}`
		}
	})
	b.caja.AlTeclear = func(k *fyne.KeyEvent) bool {
		if k.Name == fyne.KeyReturn || k.Name == fyne.KeyEnter {
			b.guardar()
			return true
		}
		return false
	}
	opciones := make([]string, len(clavesCaducidad))
	for i, k := range clavesCaducidad {
		opciones[i] = t.T(k, nil)
	}
	b.plazo = ui.NuevoSelector(opciones, nil)
	b.construir()
	return b
}

// Caja es la del texto.
func (b *BloqueEstado) Caja() *ui.Entrada { return b.caja }

// Plazo es el selector de la caducidad.
func (b *BloqueEstado) Plazo() *ui.Selector { return b.plazo }

func (b *BloqueEstado) guardar() {
	if b.AlGuardar != nil {
		b.AlGuardar(strings.TrimSpace(b.caja.Texto()), Caducidad(max(b.plazo.Elegida, 0)))
	}
}

func (b *BloqueEstado) construir() {
	t := b.T
	etiqueta := func(clave string) fyne.CanvasObject {
		return ui.NuevoRotulo(t.T(clave, nil), tema.TextoXS, ui.Medio, "text-muted")
	}
	guardar := ui.NuevoBoton(t.T("action.save", nil), b.guardar).Con(ui.Primario, ui.TallaSM)
	izq := sangria(0)
	if b.Actual != "" {
		q := ui.NuevoEnlace(t.T("action.clear", nil), escalonMeta, ui.Normal, "text-subtle", func() {
			if b.AlGuardar != nil {
				b.AlGuardar("", NoCaduca)
			}
		})
		q.Boton, q.Subrayado = true, true
		izq = q
	}
	pie := container.NewBorder(nil, nil, izq, guardar)
	b.columna = container.New(ui.Columna{},
		etiqueta("status.custom"), sangriaAlto(tema.Espacio(1)), b.caja,
		sangriaAlto(tema.Espacio(2)), etiqueta("status.clearAfter"), sangriaAlto(tema.Espacio(1)), b.plazo,
		sangriaAlto(tema.Espacio(2)), pie)
}

// CreateRenderer monta la columna con su `px-2 py-1.5`.
func (b *BloqueEstado) CreateRenderer() fyne.WidgetRenderer {
	return &bloqueEstadoRenderer{b: b}
}

type bloqueEstadoRenderer struct{ b *BloqueEstado }

func (r *bloqueEstadoRenderer) MinSize() fyne.Size {
	m := r.b.columna.MinSize()
	return fyne.NewSize(m.Width+2*tema.Espacio(2), m.Height+2*tema.Espacio(1.5))
}

func (r *bloqueEstadoRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.b.columna} }
func (r *bloqueEstadoRenderer) Destroy()                     {}

func (r *bloqueEstadoRenderer) Layout(t fyne.Size) {
	r.b.columna.Move(fyne.NewPos(tema.Espacio(2), tema.Espacio(1.5)))
	r.b.columna.Resize(fyne.NewSize(max(t.Width-2*tema.Espacio(2), 0), max(t.Height-2*tema.Espacio(1.5), 0)))
}

func (r *bloqueEstadoRenderer) Refresh() { r.Layout(r.b.Size()) }

// sangriaAlto es un hueco de ese alto, para una `ui.Columna`.
func sangriaAlto(alto float32) fyne.CanvasObject { return hueco(alto) }

// ---- En la Sala ----

// AbrirEstado despliega el menú de «tu estado» colgando de su disparador.
func (sa *Sala) AbrirEstado(disparador fyne.CanvasObject) {
	t := sa.P.T
	l := sa.P.Lateral
	var m *ui.Menu
	elementos := make([]fyne.CanvasObject, 0, 8)
	for _, e := range estadosElegibles {
		it := ui.NuevoElementoMenu(t.T(clavePresencia(e.punto), nil), iconos.Ninguno, func() {
			sa.CambiarPresencia(e.estado, l.EstadoPersonal, false, time.Time{})
		})
		it.Punto = e.punto.Token()
		it.Actual = l.Presencia == e.punto || (l.Presencia == ui.SinPresencia && e.punto == ui.EnLinea)
		elementos = append(elementos, it)
	}
	bloque := NuevoBloqueEstado(t, l.EstadoPersonal, func(texto string, c Caducidad) {
		m.Cerrar()
		sa.CambiarPresencia(presenciaAPI(l.Presencia), texto, true, CaducaEn(c, sa.ahora()))
	})
	elementos = append(elementos, ui.NuevoSeparadorMenu(), bloque, ui.NuevoSeparadorMenu(),
		ui.NuevoElementoMenu(t.T("status.markAllRead", nil), iconos.Tic, sa.MarcarTodoLeido))
	if sa.PonerPrefAviso != nil {
		// La acción va en el TEXTO (activar/desactivar): el nombre accesible dice el efecto.
		clave := "status.dmAlertsOff"
		if sa.directosCallados {
			clave = "status.dmAlertsOn"
		}
		elementos = append(elementos, ui.NuevoElementoMenu(t.T(clave, nil), iconos.Campana, func() {
			m.Cerrar()
			sa.AlternarAvisosDirectos()
		}))
	}
	elementos = append(elementos,
		ui.NuevoElementoMenu(t.T("notify.openSettings", nil), iconos.Ajustes, func() {
			m.Cerrar()
			sa.AbrirAjustesAviso()
		}))
	m = ui.NuevoMenu(elementos...)
	m.Rotulo = t.T("status.yours", nil)
	sa.bloqueEstado = bloque
	m.Mostrar(disparador)
}

// presenciaAPI es la vuelta de PresenciaDe: sin presencia cuenta como «en línea».
func presenciaAPI(p ui.Presencia) api.EstadoPresencia {
	for _, e := range estadosElegibles {
		if e.punto == p {
			return e.estado
		}
	}
	if p == ui.Desconectado {
		return api.Desconectado
	}
	return api.EnLinea
}

// CambiarPresencia es `changeMyPresence`: se pinta ya y se publica en el Hub, que es
// quien la reparte. Con `conPlazo`, `caduca` es cuándo se borra el estado (el cero, que
// no caduca); sin él se conserva la caducidad vigente, que es lo que pasa al cambiar
// solo de presencia.
//
// La caducidad se guarda en el almacén de la persona, como la web en `localStorage`:
// al volver a abrir la app, un estado vencido se borra y uno vigente vuelve a tener su
// temporizador.
func (sa *Sala) CambiarPresencia(e api.EstadoPresencia, texto string, conPlazo bool, caduca time.Time) {
	l := sa.P.Lateral
	l.Presencia, l.EstadoPersonal = PresenciaDe(e), texto
	sa.P.Refrescar()
	if conPlazo || texto == "" {
		sa.guardarEstado(texto, caduca)
	} else {
		_, vigente := sa.caducidadGuardada()
		sa.guardarEstado(texto, vigente)
	}
	if conPlazo || texto == "" {
		if sa.pararCaducidad != nil {
			sa.pararCaducidad()
			sa.pararCaducidad = nil
		}
		if texto != "" && !caduca.IsZero() {
			sa.pararCaducidad = sa.Esperar(caduca.Sub(sa.ahora()), func() {
				sa.pararCaducidad = nil
				sa.CambiarPresencia(presenciaAPI(sa.P.Lateral.Presencia), "", true, time.Time{})
			})
		}
	}
	if sa.AlPonerPresencia != nil {
		sa.Lanzar(func() { sa.AlPonerPresencia(sa.ctx, e, texto) })
	}
}

// MarcarTodoLeido es `marcarTodoLeido`: todos los contadores a cero de golpe, cada
// canal pendiente dado por leído, y el aviso con cuántos.
func (sa *Sala) MarcarTodoLeido() {
	var pendientes []string
	for id, p := range sa.pend {
		if p.Unread > 0 || p.Mentions > 0 {
			pendientes = append(pendientes, id)
		}
	}
	clear(sa.pend)
	sa.noLeidoAbierto = false
	sa.pintarCanales()
	sa.P.Refrescar()
	t := sa.P.T
	pedir(sa, func(ctx context.Context) (struct{}, error) {
		for _, id := range pendientes {
			_ = sa.S.MarkRead(ctx, id) // `allSettled`: uno que falle no para a los demás
		}
		return struct{}{}, nil
	}, func(struct{}, error) {
		sa.P.Avisar(ui.AvisoExito, t.T("status.markedAllRead", i18n.Vars{"count": len(pendientes)}))
	})
}
