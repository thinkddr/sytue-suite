// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// En la web: apps/frontend/apps/rooms/src/components/NotifySettingsModal.tsx
//
// «Ajustes de notificación»: las palabras clave que avisan como una mención y la franja
// de «No molestar». Se abre desde el menú del canal y desde el de «tu estado».
//
// ponytail: sin el bloque de «Notificaciones push». En la web es Web Push, que solo
// existe en un navegador; en nativo los avisos con la app cerrada son del sistema (APNs,
// el SSE del Hub en Android) y los lleva el agente de las funciones del sistema.

// TopePalabra es el `maxLength={64}` de la caja de palabras clave.
const TopePalabra = 64

// Las franjas por defecto al encender «No molestar»: de 22:00 a 07:00.
const (
	InicioNoMolestar = 22 * 60
	FinNoMolestar    = 7 * 60
)

// HoraDeMinutos es `minutesToTime`: «07:05».
func HoraDeMinutos(m int) string { return fmt.Sprintf("%02d:%02d", (m/60)%24, m%60) }

// AjustesAviso es el estado del diálogo.
type AjustesAviso struct {
	T *i18n.Translator
	// Palabras, NoMolestar y Desde/Hasta (minutos desde medianoche) son lo que se edita.
	Palabras    []string
	NoMolestar  bool
	Desde       int
	Hasta       int
	Cargando    bool
	Guardando   bool
	Fallo       bool
	AlGuardar   func(api.RoomsNotifyPrefs)
	AlCancelar  func()
	alRecolocar func()

	caja        *ui.Entrada
	desde, hast *ui.Entrada
	cuerpo      *fyne.Container
	guardar     *ui.Boton
}

// NuevoAjustesAviso crea el diálogo cargando.
func NuevoAjustesAviso(t *i18n.Translator) *AjustesAviso {
	a := &AjustesAviso{T: t, Cargando: true, Desde: InicioNoMolestar, Hasta: FinNoMolestar}
	a.caja = ui.NuevaEntrada(t.T("notify.keywordPlaceholder", nil))
	a.caja.AlTeclear = func(k *fyne.KeyEvent) bool {
		if k.Name == fyne.KeyReturn || k.Name == fyne.KeyEnter {
			a.Anadir(a.caja.Texto())
			return true
		}
		return false
	}
	// La coma también añade (`e.key === ','`), y la caja no pasa de 64.
	a.caja.AlCambiar(func(s string) {
		if i := strings.IndexRune(s, ','); i >= 0 {
			a.Anadir(s[:i])
			return
		}
		if r := []rune(s); len(r) > TopePalabra {
			a.caja.PonerTexto(string(r[:TopePalabra]))
		}
	})
	a.desde, a.hast = ui.CampoHora(ui.CampoMD), ui.CampoHora(ui.CampoMD)
	// `w-32`: «De [hora] a [hora]» en una línea, como la web.
	a.desde.Ancho, a.hast.Ancho = tema.Espacio(32), tema.Espacio(32)
	a.cuerpo = container.New(ui.Columna{Hueco: tema.Espacio(5)})
	a.guardar = ui.NuevoBoton(t.T("action.save", nil), a.pedirGuardar).Con(ui.Primario, ui.TallaMD)
	return a
}

// Poner llena el diálogo con lo que dijo el servicio: sin franja (inicio igual a fin),
// «No molestar» apagado y las horas por defecto.
func (a *AjustesAviso) Poner(p api.RoomsNotifyPrefs) {
	a.Palabras = slices.Clone(p.Keywords)
	a.NoMolestar = p.DNDStart != p.DNDEnd
	if a.NoMolestar {
		a.Desde, a.Hasta = p.DNDStart, p.DNDEnd
	}
	a.Cargando = false
	a.pintar()
}

// Anadir es `addKeyword`: en minúsculas, sin espacios a los lados y sin repetir.
func (a *AjustesAviso) Anadir(s string) {
	k := strings.ToLower(strings.TrimSpace(s))
	a.caja.PonerTexto("")
	if k != "" && !slices.Contains(a.Palabras, k) {
		a.Palabras = append(a.Palabras, k)
	}
	a.pintar()
}

// Prefs son las del diálogo, listas para guardar: sin franja, 0 y 0.
func (a *AjustesAviso) Prefs() api.RoomsNotifyPrefs {
	p := api.RoomsNotifyPrefs{Keywords: slices.Clone(a.Palabras)}
	if a.NoMolestar {
		p.DNDStart, p.DNDEnd = a.minutos(a.desde, a.Desde), a.minutos(a.hast, a.Hasta)
	}
	return p
}

// minutos es `timeToMinutes` de lo tecleado; si no es una hora, la que había.
func (a *AjustesAviso) minutos(e *ui.Entrada, antes int) int {
	h, m, vacia, ok := ui.LeerHora(e.Texto())
	if vacia || !ok {
		return antes
	}
	return h*60 + m
}

func (a *AjustesAviso) pedirGuardar() {
	if a.Guardando || a.Cargando || a.AlGuardar == nil {
		return
	}
	a.Guardando, a.Fallo = true, false
	a.pintar()
	a.AlGuardar(a.Prefs())
}

// Pie son «Cancelar» y «Guardar».
func (a *AjustesAviso) Pie() fyne.CanvasObject {
	cancelar := ui.NuevoBoton(a.T.T("action.cancel", nil), func() {
		if a.AlCancelar != nil {
			a.AlCancelar()
		}
	}).Con(ui.Fantasma, ui.TallaMD)
	return container.New(ui.Fila{Hueco: tema.Espacio(2)}, cancelar, a.guardar)
}

// Cuerpo es lo de dentro del diálogo.
func (a *AjustesAviso) Cuerpo() fyne.CanvasObject {
	a.pintar()
	return a.cuerpo
}

// pintar rehace el cuerpo con el estado: cambian las fichas y la franja aparece y
// desaparece.
func (a *AjustesAviso) pintar() {
	t := a.T
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	a.guardar.Cargando, a.guardar.Inerte = a.Guardando, a.Cargando
	a.guardar.Refresh()
	a.caja.Inerte(a.Cargando)
	var bloques []fyne.CanvasObject
	if a.Fallo {
		bloques = append(bloques, ui.NuevoParrafo(t.T("notify.failed", nil), tema.TextoSM, ui.Normal, "danger-text", v))
	}
	// Palabras clave.
	palabras := []fyne.CanvasObject{
		container.New(ui.Margen{Abj: tema.Espacio(1)}, ui.NuevoRotulo(t.T("notify.keywords", nil), tema.TextoSM, ui.SemiNegrita, "text")),
		container.New(ui.Margen{Abj: tema.Espacio(2)}, ui.NuevoParrafo(t.T("notify.keywordsHelp", nil), tema.TextoXS, ui.Normal, "text-muted", v)),
	}
	if len(a.Palabras) > 0 {
		fichas := make([]fyne.CanvasObject, 0, len(a.Palabras))
		for _, k := range a.Palabras {
			fichas = append(fichas, NuevoChipQuitable(k, t.T("action.removeItem", i18n.Vars{"name": k}), nil, func() {
				a.Palabras = slices.DeleteFunc(a.Palabras, func(x string) bool { return x == k })
				a.pintar()
			}))
		}
		palabras = append(palabras, container.New(ui.Margen{Abj: tema.Espacio(2)}, container.New(&envuelta{hueco: tema.Espacio(1.5)}, fichas...)))
	}
	palabras = append(palabras, a.caja)
	bloques = append(bloques, container.New(ui.Columna{}, palabras...))
	// No molestar.
	interruptor := ui.NuevoConmutador(t.T("presence.dnd", nil), func(si bool) {
		a.NoMolestar = si
		a.pintar()
	})
	interruptor.Encendido = a.NoMolestar
	if a.Cargando {
		interruptor.Disable()
	}
	fila := container.NewBorder(nil, nil, nil, interruptor,
		ui.NuevoParrafo(t.T("notify.dndHelp", nil), tema.TextoXS, ui.Normal, "text-muted", v))
	nm := []fyne.CanvasObject{fila}
	if a.NoMolestar {
		if a.desde.Texto() == "" {
			a.desde.PonerTexto(HoraDeMinutos(a.Desde))
			a.hast.PonerTexto(HoraDeMinutos(a.Hasta))
		}
		franja := container.New(&envuelta{hueco: tema.Espacio(2)},
			ui.NuevoRotulo(t.T("notify.from", nil), tema.TextoSM, ui.Normal, "text-muted"), a.desde,
			ui.NuevoRotulo(t.T("notify.to", nil), tema.TextoSM, ui.Normal, "text-muted"), a.hast)
		nm = append(nm, container.New(ui.Margen{Arr: tema.Espacio(3)}, franja))
	}
	bloques = append(bloques, container.New(ui.Columna{}, nm...))
	a.cuerpo.Objects = bloques
	a.cuerpo.Refresh()
	if a.alRecolocar != nil {
		a.alRecolocar()
	}
}

// envuelta es `flex flex-wrap items-center gap-*` como fyne.Layout, sobre
// colocarEnvuelto.
type envuelta struct {
	hueco float32
	ancho float32
}

func (e *envuelta) MinSize(objs []fyne.CanvasObject) fyne.Size {
	ancho := e.ancho
	if ancho <= 0 {
		for _, o := range objs {
			ancho += o.MinSize().Width + e.hueco
		}
	}
	return fyne.NewSize(0, colocarEnvuelto(objs, 0, 0, ancho, e.hueco, false))
}

func (e *envuelta) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	e.ancho = t.Width
	colocarEnvuelto(objs, 0, 0, t.Width, e.hueco, true)
}

// ---- En la Sala ----

// AbrirAjustesAviso es `setNotifySettingsOpen(true)`: pide las preferencias, deja
// editarlas y las guarda; un fallo se queda en el diálogo con su aviso.
func (sa *Sala) AbrirAjustesAviso() {
	lienzo := fyne.CurrentApp().Driver().CanvasForObject(sa.P.Objeto())
	if lienzo == nil {
		return
	}
	t := sa.P.T
	a := NuevoAjustesAviso(t)
	d := ui.NuevoDialogo(t.T("notify.title", nil), a.Cuerpo()).Con(ui.DialogoSM, a.Pie())
	d.RotuloCerrar, d.Foco = t.T("action.close", nil), a.caja
	a.alRecolocar = d.Recolocar
	a.AlCancelar = d.Cerrar
	a.AlGuardar = func(p api.RoomsNotifyPrefs) {
		pedir(sa, func(ctx context.Context) (api.RoomsNotifyPrefs, error) { return sa.S.PutNotifyPrefs(ctx, p) },
			func(guardadas api.RoomsNotifyPrefs, err error) {
				a.Guardando = false
				if err != nil {
					a.Fallo = true
					a.pintar()
					return
				}
				sa.prefsAviso = guardadas
				d.Cerrar()
			})
	}
	sa.ajustesAviso = a
	sa.pregunta = d
	d.Mostrar(lienzo)
	pedir(sa, sa.S.NotifyPrefs, func(p api.RoomsNotifyPrefs, err error) {
		if err != nil {
			a.Cargando, a.Fallo = false, true
			a.pintar()
			return
		}
		a.Poner(p)
	})
}
