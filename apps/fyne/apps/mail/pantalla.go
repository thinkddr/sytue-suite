// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Pantalla es Sytue Mail entero: el menú, la bandeja y el lector dentro del armazón
// de la suite.
//
// Es un ESTADO, no una vista: la app lo cambia y llama a Refrescar. Todo lo que hay
// aquí son datos y punteros a funciones; el armazón y las tres columnas son widgets
// que ya saben pintarse.
//
// La TOPBAR no está, y no es un olvido: es una de las dos exclusiones de paridad
// escritas en `docs/CONVENTIONS.md` §13.6 —existe solo en la web y en nativo la
// sustituye el cromo de la ventana—. Por eso el armazón empieza en y = 0 y no en 56.
type Pantalla struct {
	T      *i18n.Translator
	Locale i18n.Locale
	// AvisosSinReloj: los avisos no se van solos (ver Notificar). Solo pruebas.
	AvisosSinReloj bool

	Menu    *Menu
	Bandeja *Bandeja
	Lector  *Lector
	// Franja es la de solo lectura del buzón compartido abierto, o nil.
	Franja fyne.CanvasObject
	// Acciones son las del triaje, compartidas por la lista y el lector: las dos
	// columnas ofrecen lo mismo sobre la misma conversación.
	Acciones *Acciones
	// Avisos es el `ToastProvider`: HERMANO del armazón, no capa (ver `ui.PilaAvisos`).
	Avisos *ui.PilaAvisos
	// Movil es la barra de AppShell por debajo de 768: la hamburguesa del cajón o,
	// con un hilo abierto, «volver a la lista» y el nombre de la vista. AlVolver es
	// su `onBack`.
	Movil    *ui.BarraMovil
	AlVolver func()

	armazon *ui.Armazon
	raiz    *fyne.Container
	bandeja fyne.CanvasObject
}

// NuevaPantalla monta las tres columnas con su traductor.
func NuevaPantalla(t *i18n.Translator, l i18n.Locale) *Pantalla {
	p := &Pantalla{
		T:        t,
		Locale:   l,
		Menu:     &Menu{T: t, Locale: l},
		Bandeja:  NuevaBandeja(t, l),
		Lector:   NuevoLector(t, l),
		Acciones: &Acciones{},
		Avisos:   ui.NuevaPilaAvisos(),
	}
	p.Bandeja.Acciones, p.Lector.Acciones = p.Acciones, p.Acciones
	p.Bandeja.Refresco.Arriba = func() bool {
		return p.Bandeja.Estado != Listo || p.Bandeja.r == nil || p.Bandeja.r.lista.GetScrollOffset() <= 0.5
	}
	p.bandeja = ui.RefrescarAlTirar(p.Bandeja, &p.Bandeja.Refresco)
	p.armazon = ui.NuevoArmazon(p.Lector)
	p.armazon.Panel = p.bandeja
	p.armazon.ClavePreferenciaPanel = "sytue.mail.panel-width"
	p.armazon.Menu = p.Menu.Construir()
	p.Movil = ui.NuevaBarraMovil()
	p.Movil.RotuloAbrir, p.Movil.RotuloCerrar, p.Movil.RotuloVolver =
		t.T("ui.openNav", nil), t.T("ui.closeNav", nil), t.T("ui.backToList", nil)
	p.Movil.AlMenu = func() { p.ponerCajon(!p.armazon.CajonAbierto) }
	p.Movil.AlVolver = func() {
		if p.AlVolver != nil {
			p.AlVolver()
		}
	}
	p.armazon.BarraMovil = p.Movil
	// «Redactar al alcance del pulgar: en móvil el menú lateral es un cajón cerrado»
	// (`MobileFab` en MailApp.tsx).
	fab := ui.NuevoBotonFlotante(t.T("nav.compose", nil), iconos.Mas, func() {
		if p.Menu.AlRedactar != nil {
			p.Menu.AlRedactar()
		}
	})
	p.raiz = container.NewStack(p.armazon, container.New(ui.CapaFlotante{}, fab), p.Avisos)
	return p
}

// ponerCajon abre o cierra el menú en ventana estrecha.
func (p *Pantalla) ponerCajon(abierto bool) {
	p.armazon.CajonAbierto, p.Movil.CajonAbierto = abierto, abierto
	p.armazon.Refresh()
	p.Movil.Refresh()
}

// CerrarCajon es lo que hace AppShell con lo que se activa DENTRO del cajón: cerrarlo.
// En el teléfono el cajón tapa la pantalla, y elegir una carpeta la cargaba detrás de
// un menú que seguía ahí.
func (p *Pantalla) CerrarCajon() {
	if p.armazon.CajonAbierto {
		p.ponerCajon(false)
	}
}

// Objeto es lo que se le da a la ventana.
func (p *Pantalla) Objeto() fyne.CanvasObject { return p.raiz }

// Lienzo es donde se abren los diálogos y los menús: el de la ventana que la muestra.
func (p *Pantalla) Lienzo() fyne.Canvas {
	if c := fyne.CurrentApp().Driver().CanvasForObject(p.armazon); c != nil {
		return c
	}
	if p.raiz != nil {
		if c := fyne.CurrentApp().Driver().CanvasForObject(p.raiz); c != nil {
			return c
		}
	}
	for _, win := range fyne.CurrentApp().Driver().AllWindows() {
		if win != nil && win.Canvas() != nil {
			return win.Canvas()
		}
	}
	return nil
}

// Avisar pone un aviso en la esquina, ya traducido. Los de peligro se quedan hasta que
// alguien los cierra: un error que se va solo puede no leerse nunca.
func (p *Pantalla) Avisar(tono ui.TonoAviso, titulo, descripcion string) {
	a := ui.NuevoAviso(tono, titulo, descripcion)
	a.RotuloCerrar = p.T.T("ui.dismissNotification", nil)
	p.Notificar(a)
}

// Notificar pone un aviso ya montado (el de «Deshacer», con su acción).
//
// Con AvisosSinReloj el aviso no se va solo: es para las pruebas, donde el driver de
// Fyne ejecuta el `fyne.Do` del reloj EN la rutina del reloj, y con `-race` un test
// dura más que los 5 s de un aviso de éxito —su `Refresh` medía texto a la vez que el
// test, sobre el mismo búfer de HarfBuzz—.
func (p *Pantalla) Notificar(a *ui.Aviso) {
	if p.AvisosSinReloj {
		a.Duracion = -1
	}
	p.Avisos.Notificar(a)
}

// DetalleAbierto es el turno de lista y detalle en ventana estrecha: con un hilo
// abierto se ve el lector, y sin él la lista. Sin esto, en un teléfono el correo que
// pulsas no aparece por ninguna parte.
func (p *Pantalla) DetalleAbierto(si bool) {
	p.armazon.DetalleAbierto = si
	p.Movil.EnDetalle = si
	p.Movil.Titulo = p.Bandeja.nombre()
	p.armazon.Refresh()
	p.Movil.Refresh()
}

// Refrescar vuelve a montar el menú —que se construye a partir de datos— y repinta
// las tres columnas.
func (p *Pantalla) Refrescar() {
	p.Menu.CargandoLista = p.Bandeja.Estado == Cargando
	// La franja del buzón compartido va sobre la lista, como en `MessageList.tsx`.
	if p.Franja != nil {
		p.armazon.Panel = container.New(ui.Pila{Elastico: 1}, p.Franja, p.bandeja)
	} else {
		p.armazon.Panel = p.bandeja
	}
	p.armazon.Menu = p.Menu.Construir()
	p.armazon.Refresh()
	p.Bandeja.Refresh()
	p.Lector.Refresh()
}

// Elegir abre una conversación: la marca en la lista y la pinta en el lector. Es la
// única costura entre las dos columnas, y por eso está aquí y no dentro de ninguna.
func (p *Pantalla) Elegir(c Conversacion) {
	p.Bandeja.AbiertaID = c.HiloID
	p.Lector.Conversacion = c
	p.Lector.Estado = Listo
	p.DetalleAbierto(true)
	p.Bandeja.Refresh()
	p.Lector.Refresh()
}

// Reloj fija el reloj con el que se escriben las fechas relativas, en la lista y en
// el lector a la vez. El guardián lo congela para no depender de la hora.
func (p *Pantalla) Reloj(f func() time.Time) {
	p.Bandeja.Ahora, p.Lector.Ahora = f, f
}
