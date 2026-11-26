// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"

	"git.sytue.eu/sytue/one/apps/fyne/apps/hub"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/local/indice"
)

// La portada del Hub local. No es `hub.Lanzador`: esa portada es la del portal web,
// medida contra él, y la mitad de lo que lleva —«Hoy» con el correo del servidor,
// accesos rápidos a dispositivos, descargas y «Tus datos», el aviso de privacidad de la
// cuenta— no existe sin servicio (HU-08). Lo que SÍ es del Hub conectado se reutiliza
// tal cual: las teselas de producto (`hub.TeselaApp`) y las filas de lo reciente
// (`hub.FilaReciente`).

const (
	// AnchoTesela es el de cada tesela de la rejilla: caben cinco en 1280.
	AnchoTesela float32 = 232
	// MaxRecientes y MaxResultados son las filas de la lista.
	MaxRecientes  = 12
	MaxResultados = 40
)

// Portada es la pantalla del Hub.
type Portada struct {
	T     *i18n.Translator
	Datos string // la carpeta de datos de la edición
	Apps  []Instalada
	// Movil dice que este sistema no lanza otras apps (cada una va por separado).
	Movil bool
	// Lanzar abre una app con un enlace; Fichero abre un fichero con el visor del
	// sistema; Aviso enseña un mensaje. Se inyectan para las pruebas.
	Lanzar  func(Instalada, string) error
	Fichero func(ruta string) error
	Aviso   func(titulo, texto string)
	// Ahora es el reloj de las antigüedades («hace 5 min»).
	Ahora func() time.Time

	// AlCambiarHoy se llama, en el hilo de la interfaz, cada vez que se vuelve a leer lo
	// de hoy: la bandeja lo enseña también.
	AlCambiarHoy func(indice.Hoy)

	mu       sync.Mutex
	snap     indice.Instantanea
	hoy      indice.Hoy
	consulta string

	rotuloHoy *canvas.Text
	cajaHoy   *fyne.Container
	buscador  *ui.Entrada
	rotulo    *canvas.Text
	lista     *fyne.Container
	aviso     *canvas.Text
	raiz      fyne.CanvasObject
}

// NuevaPortada monta la portada; los datos llegan con Recargar.
func NuevaPortada(t *i18n.Translator, datos string, apps []Instalada) *Portada {
	p := &Portada{
		T: t, Datos: datos, Apps: apps, Ahora: time.Now,
		Fichero: ficheros.Abrir, Aviso: func(string, string) {},
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	p.buscador = ui.NuevaEntrada(p.tx("buscar"))
	p.buscador.AlCambiar(p.Buscar)
	p.rotulo = ui.Texto("", tema.TextoXS, ui.SemiNegrita, tema.Color("text-muted", v))
	p.aviso = ui.Texto("", tema.TextoXS, ui.Normal, tema.Color("text-muted", v))
	p.lista = container.NewVBox()
	p.rotuloHoy = ui.Texto("", tema.TextoXS, ui.SemiNegrita, tema.Color("text-muted", v))
	p.cajaHoy = container.NewVBox()

	titulo := ui.Texto(p.tx("titulo"), tema.Texto2XL, ui.SemiNegrita, tema.Color("text", v))
	cuerpo := container.NewVBox(titulo, ui.Espaciador(tema.Espacio(4)), p.rejilla(),
		ui.Espaciador(tema.Espacio(8)), p.rotuloHoy, ui.Espaciador(tema.Espacio(2)), p.cajaHoy,
		ui.Espaciador(tema.Espacio(8)), p.rotulo, ui.Espaciador(tema.Espacio(2)), p.lista, p.aviso)
	p.raiz = container.NewVScroll(container.New(layout.NewCustomPaddedLayout(tema.Espacio(7), tema.Espacio(8), tema.Espacio(6), tema.Espacio(6)), cuerpo))
	p.pintar()
	p.pintarHoy()
	return p
}

func (p *Portada) tx(clave string, vars ...string) string { return texto(p.T.Locale(), clave, vars...) }

// Objeto es la portada para poner en la ventana.
func (p *Portada) Objeto() fyne.CanvasObject { return p.raiz }

// Buscador es la caja de búsqueda, para la barra.
func (p *Portada) Buscador() *ui.Entrada { return p.buscador }

// rejilla son las teselas de las apps instaladas o, sin ninguna, el porqué.
func (p *Portada) rejilla() fyne.CanvasObject {
	if len(p.Apps) == 0 {
		cuerpo := p.tx("sinApps.texto")
		if p.Movil {
			cuerpo = p.tx("sinApps.movil")
		}
		v := ui.NuevoVacio(p.tx("sinApps"), cuerpo)
		v.Compacto = true
		return v
	}
	var teselas []fyne.CanvasObject
	for _, a := range p.Apps {
		prod := a.Tesela()
		desc := p.T.T(prod.ClaveDescripcion(), nil)
		t := hub.NuevaTeselaApp(prod, desc, prod.Nombre+" — "+desc, func() { p.abrirApp(a, "") })
		teselas = append(teselas, t)
	}
	return container.NewGridWrap(fyne.NewSize(AnchoTesela, hub.AltoTesela(hub.LineasDescripcion)), teselas...)
}

// Recargar vuelve a leer los almacenes de todas las apps, fuera del hilo de la interfaz.
func (p *Portada) Recargar() {
	go func() {
		p.cargar()
		fyne.Do(p.repintar)
	}()
}

// repintar pinta lo leído y se lo cuenta a la bandeja.
func (p *Portada) repintar() {
	p.pintar()
	p.pintarHoy()
	if p.AlCambiarHoy != nil {
		p.AlCambiarHoy(p.Hoy())
	}
}

// cargar lee los almacenes; no toca la interfaz.
func (p *Portada) cargar() {
	s := indice.Leer(p.Datos)
	for fuente, err := range s.Fallos {
		slog.Warn("hub: no se ha podido leer una fuente", "fuente", fuente, "error", err)
	}
	h := indice.LeerHoy(p.Datos, p.Ahora())
	for fuente, err := range h.Fallos {
		slog.Warn("hub: no se ha podido leer lo de hoy", "fuente", fuente, "error", err)
	}
	p.mu.Lock()
	p.snap, p.hoy = s, h
	p.mu.Unlock()
}

// Buscar cambia la lista por los resultados de consulta; vacía, vuelve a lo reciente.
func (p *Portada) Buscar(consulta string) {
	p.mu.Lock()
	p.consulta = strings.TrimSpace(consulta)
	p.mu.Unlock()
	p.pintar()
}

// Elementos es lo que la lista enseña ahora, y su rótulo.
func (p *Portada) Elementos() (string, []indice.Elemento) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.consulta != "" {
		return p.tx("resultados"), p.snap.Buscar(p.consulta, MaxResultados)
	}
	return p.tx("recientes"), p.snap.Recientes(MaxRecientes)
}

func (p *Portada) pintar() {
	rotulo, es := p.Elementos()
	p.mu.Lock()
	consulta := p.consulta
	fallos := make([]string, 0, len(p.snap.Fallos))
	for f := range p.snap.Fallos {
		fallos = append(fallos, f)
	}
	p.mu.Unlock()
	// Buscando, «Hoy» se aparta: los resultados van arriba, donde se mira.
	if consulta != "" {
		p.rotuloHoy.Hide()
		p.cajaHoy.Hide()
	} else {
		p.rotuloHoy.Show()
		p.cajaHoy.Show()
	}
	p.rotulo.Text = strings.ToUpper(rotulo)
	p.rotulo.Refresh()
	slices.Sort(fallos)
	p.aviso.Text = ""
	if len(fallos) > 0 {
		p.aviso.Text = p.tx("fallos", "fuentes", strings.Join(fallos, ", "))
	}
	p.aviso.Refresh()

	filas := make([]fyne.CanvasObject, 0, len(es))
	for _, e := range es {
		filas = append(filas, p.fila(e))
	}
	if len(filas) == 0 {
		var v *ui.Vacio
		if consulta != "" {
			v = ui.NuevoVacio(p.tx("sinResultados", "q", consulta), p.tx("sinResultados.texto"))
		} else {
			v = ui.NuevoVacio(p.tx("sinRecientes"), p.tx("sinRecientes.texto"))
		}
		v.Compacto = true
		filas = []fyne.CanvasObject{v}
	}
	p.lista.Objects = filas
	p.lista.Refresh()
}

// fila es una entrada de la lista, con la marca de su app.
func (p *Portada) fila(e indice.Elemento) *hub.FilaReciente {
	resumen := e.Detalle
	f := hub.NuevaFilaReciente(cmpOr(e.Titulo, "—"), resumen, func() { p.Abrir(e) })
	if m, ok := iconos.ProductoDe(e.Producto); ok {
		f.Marca, f.Origen, f.Conocido = m, m.NombreCorto(), true
	} else {
		f.Origen = e.Producto
	}
	if !e.Fecha.IsZero() {
		if e.Clase == indice.Evento {
			f.Cuando = i18n.FormatWeekdayDayMonth(p.T.Locale(), e.Fecha.Local())
		} else {
			f.Cuando = i18n.FormatTimeAgo(p.T.Locale(), e.Fecha, p.Ahora())
		}
	}
	return f
}

func cmpOr(a, b string) string {
	if strings.TrimSpace(a) == "" {
		return b
	}
	return a
}

// instalada es la app instalada que abre ese producto.
func (p *Portada) instalada(producto string) (Instalada, bool) {
	i := slices.IndexFunc(p.Apps, func(a Instalada) bool { return a.Producto == producto })
	if i < 0 {
		return Instalada{}, false
	}
	return p.Apps[i], true
}

// Abrir lleva el elemento a la app que lo abre. Un fichero sin app de la edición que lo
// abra va al visor del sistema; lo demás sin su app instalada lo dice.
func (p *Portada) Abrir(e indice.Elemento) {
	if a, ok := p.instalada(e.Producto); ok {
		p.abrirApp(a, e.Enlace())
		return
	}
	if e.Ruta != "" && p.Fichero != nil {
		if err := p.Fichero(e.Ruta); err != nil {
			p.Aviso(p.tx("noSeAbre", "app", e.Titulo), err.Error())
		}
		return
	}
	nombre := e.Producto
	if m, ok := iconos.ProductoDe(e.Producto); ok {
		nombre = m.NombreCorto()
	}
	p.Aviso(p.tx("noInstalada", "app", nombre), p.tx("noInstalada.texto", "app", nombre))
}

func (p *Portada) abrirApp(a Instalada, enlace string) {
	if p.Lanzar == nil {
		return
	}
	if err := p.Lanzar(a, enlace); err != nil {
		slog.Warn("hub: no se ha podido abrir la app", "app", a.Carpeta, "error", err)
		p.Aviso(p.tx("noSeAbre", "app", a.Marca.NombreCorto()), err.Error())
	}
}
