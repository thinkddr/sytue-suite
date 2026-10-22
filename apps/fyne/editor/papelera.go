// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

// La papelera de los tres editores; la pantalla de la web de cada producto la marca su
// `cmd/<producto>/main.go`, que es quien la monta con su tipo.

import (
	"context"
	"errors"
	"net/http"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// # La papelera de documentos
//
// `DocTrash.tsx` bajo el producto (`?kind=`): lo que se tira desde la portada se ve
// aquí, se restaura o se elimina para siempre, y la papelera se vacía entera; las dos
// cosas definitivas preguntan antes. El servicio hace lo mismo con el fichero de cada
// documento en la papelera de Sytue Drive (decidido por el dueño el 2026-09-28).
//
// Se abre desde la portada, con el botón «Papelera» junto a «Destacados», y «Volver
// a los documentos» devuelve a ella con la lista al día.

// Medidas de la papelera: el marco de la portada (`max-w-[1280px] px-6 pt-8`), la
// cabecera con `mb-6`, la vuelta con `mb-2` y las filas `px-3 py-2 border-b` con un
// glifo de 28.
var (
	BajoCabeceraPapelera          = tema.Espacio(6) // mb-6 → 21
	BajoVuelta                    = tema.Espacio(2) // mb-2 → 7
	GlifoFilaPapelera     float32 = 28
	MargenFilaPapelera            = tema.Espacio(3) // px-3
	RellenoFilaPapelera           = tema.Espacio(2) // py-2
	HuecoFilaPapelera             = tema.Espacio(3) // gap-3
	HuecoBotonesPapelera          = tema.Espacio(2) // gap-2
	EsqueletosPapelera            = 4
	AltoEsqueletoPapelera float32 = 56
)

// PapeleraDocs es la pantalla y su estado.
type PapeleraDocs struct {
	Tipo Tipo
	T    func(string, i18n.Vars) string
	Loc  i18n.Locale
	// Svc es el cliente del servicio. nil: se pinta y no pide nada.
	Svc    *api.Docs
	Red    *offline.Monitor
	Plazo  time.Duration
	Avisos *ui.PilaAvisos
	// Lanzar corre una petición fuera del hilo de la interfaz; nil es una rutina nueva.
	Lanzar func(func())
	// AlVolver vuelve a la portada.
	AlVolver func()

	Estado Estado
	Docs   []api.Document
	// Ocupado apaga los botones mientras hay una operación en curso, como el `busy`.
	Ocupado bool
	// cerrada: ya se volvió a la portada. Lo que llegue tarde no recarga nada.
	cerrada bool

	raiz *fyne.Container
}

// NuevaPapeleraDocs crea la papelera de un producto, en su estado inicial: cargando.
func NuevaPapeleraDocs(tipo Tipo, t func(string, i18n.Vars) string, loc i18n.Locale, svc *api.Docs) *PapeleraDocs {
	p := &PapeleraDocs{Tipo: tipo, T: t, Loc: loc, Svc: svc, Plazo: 30 * time.Second, Estado: Cargando}
	p.raiz = container.NewStack()
	p.Repintar()
	return p
}

// Objeto es la pantalla.
func (p *PapeleraDocs) Objeto() fyne.CanvasObject { return p.raiz }

func (p *PapeleraDocs) t(clave string, v ...i18n.Vars) string {
	if p.T == nil {
		return clave
	}
	if len(v) == 0 {
		return p.T(clave, nil)
	}
	return p.T(clave, v[0])
}

func (p *PapeleraDocs) lanzar(f func()) {
	if p.Lanzar != nil {
		p.Lanzar(f)
		return
	}
	go f()
}

func (p *PapeleraDocs) hayRed() bool { return p.Red == nil || p.Red.State() != offline.Offline }

func (p *PapeleraDocs) ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), p.Plazo)
}

func (p *PapeleraDocs) avisar(tono ui.TonoAviso, titulo string) {
	if p.Avisos != nil {
		p.Avisos.Notificar(ui.NuevoAviso(tono, titulo, ""))
	}
}

// Cerrar marca la papelera como cerrada: una respuesta que llegue después ya no
// recarga una pantalla que nadie ve.
func (p *PapeleraDocs) Cerrar() { p.cerrada = true }

// Cargar pide la papelera del producto.
func (p *PapeleraDocs) Cargar() {
	if p.cerrada {
		return
	}
	p.Estado, p.Docs = Cargando, nil
	p.Repintar()
	if p.Svc == nil {
		return
	}
	if !p.hayRed() {
		p.Estado = SinConexion
		p.Repintar()
		return
	}
	p.lanzar(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		docs, err := p.Svc.Papelera(ctx, p.Tipo.Kind())
		fyne.Do(func() {
			p.Docs = docs
			p.Estado = EstadoDe(err, p.hayRed(), len(docs))
			p.Repintar()
		})
	})
}

// codigoDe es el código del error del servicio, o "".
func codigoDe(err error) (int, string) {
	var e *api.Error
	if errors.As(err, &e) {
		return e.Status, e.Code()
	}
	return 0, ""
}

// Restaurar saca el documento de la papelera, y su fichero de la de Drive.
func (p *PapeleraDocs) Restaurar(d api.Document) {
	if p.Ocupado || !p.hayRed() || p.Svc == nil {
		return
	}
	p.Ocupado = true
	p.Repintar()
	p.lanzar(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		err := p.Svc.Restaurar(ctx, d.ID)
		fyne.Do(func() {
			p.Ocupado = false
			if err != nil {
				clave := "trash.restoreFailed"
				switch _, codigo := codigoDe(err); codigo {
				case "file_name_taken":
					clave = "trash.restoreConflict"
				case "already_restored":
					clave = "trash.restoreDuplicate"
				}
				p.avisar(ui.AvisoPeligro, p.t(clave, i18n.Vars{"title": d.Title}))
				p.Repintar()
				return
			}
			p.avisar(ui.AvisoExito, p.t("trash.restored", i18n.Vars{"title": d.Title}))
			p.Cargar()
		})
	})
}

func (p *PapeleraDocs) lienzo() fyne.Canvas {
	if app := fyne.CurrentApp(); app != nil {
		return app.Driver().CanvasForObject(p.raiz)
	}
	return nil
}

// confirmar abre un diálogo `sm` de acción irreversible.
func (p *PapeleraDocs) confirmar(titulo, cuerpo, verbo string, hacer func()) {
	lienzo := p.lienzo()
	if lienzo == nil {
		return
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	texto := ui.NuevoParrafo(cuerpo, tema.TextoBase, ui.Normal, "text-muted", v)
	var dlg *ui.Dialogo
	dlg = ui.Confirmacion(titulo, p.t("action.cancel"), verbo, true, texto,
		func() { dlg.Cerrar() },
		func() {
			dlg.Cerrar()
			hacer()
		})
	dlg.RotuloCerrar = p.t("ui.close")
	dlg.Mostrar(lienzo)
}

// PedirPurga confirma antes de eliminar para siempre: nunca va directo.
func (p *PapeleraDocs) PedirPurga(d api.Document) {
	p.confirmar(p.t("trash.purgeTitle"), p.t("trash.purgeBody", i18n.Vars{"title": d.Title}),
		p.t("trash.purge"), func() { p.Purgar(d) })
}

// Purgar elimina el documento para siempre, sin preguntar: la pregunta es PedirPurga.
func (p *PapeleraDocs) Purgar(d api.Document) {
	if p.Ocupado || !p.hayRed() || p.Svc == nil {
		return
	}
	p.Ocupado = true
	p.Repintar()
	p.lanzar(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		err := p.Svc.Purgar(ctx, d.ID)
		fyne.Do(func() {
			p.Ocupado = false
			if err != nil {
				clave := "trash.purgeFailed"
				if estado, _ := codigoDe(err); estado == http.StatusLocked {
					clave = "trash.purgeHold"
				}
				p.avisar(ui.AvisoPeligro, p.t(clave, i18n.Vars{"title": d.Title}))
				p.Repintar()
				return
			}
			p.avisar(ui.AvisoExito, p.t("trash.purged", i18n.Vars{"title": d.Title}))
			p.Cargar()
		})
	})
}

// PedirVaciar confirma antes de vaciar la papelera.
func (p *PapeleraDocs) PedirVaciar() {
	p.confirmar(p.t("trash.emptyConfirmTitle"), p.t("trash.emptyConfirmBody"), p.t("trash.empty"), p.Vaciar)
}

// Vaciar elimina para siempre todo lo de la papelera del producto, página a página y
// saltando lo que no se pudo (una retención legal), como la web.
func (p *PapeleraDocs) Vaciar() {
	if p.Ocupado || !p.hayRed() || p.Svc == nil {
		return
	}
	p.Ocupado = true
	p.Repintar()
	p.lanzar(func() {
		fallidos, despues := 0, ""
		var err error
		for range 200 {
			// Un plazo POR PÁGINA: el servicio se da hasta 15 s en cada una, y uno solo
			// para todas daba por fallido un vaciado que sí se hacía.
			ctx, cancelar := p.ctx()
			var f int
			f, despues, err = p.Svc.Vaciar(ctx, p.Tipo.Kind(), despues)
			cancelar()
			fallidos += f
			if err != nil || despues == "" {
				break
			}
		}
		fyne.Do(func() {
			p.Ocupado = false
			switch {
			case err != nil:
				p.avisar(ui.AvisoPeligro, p.t("trash.purgeFailed"))
			case fallidos > 0 || despues != "":
				// Con `despues` aún puesto se paró en el tope de vueltas: no está vacía.
				p.avisar(ui.AvisoPeligro, p.t("trash.emptyPartial"))
			default:
				p.avisar(ui.AvisoExito, p.t("trash.emptied"))
			}
			p.Cargar()
		})
	})
}

// ---- la vista ----

// Repintar rehace la pantalla con el estado de ahora, en el hilo de la interfaz.
func (p *PapeleraDocs) Repintar() {
	p.raiz.Objects = []fyne.CanvasObject{p.construir()}
	p.raiz.Refresh()
}

func (p *PapeleraDocs) construir() fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	m := &marcoPapelera{
		fondo:  canvas.NewRectangle(tema.Color("bg", v)),
		volver: ui.NuevoEnlace(p.t("trash.back"), tema.TextoSM, ui.Normal, "text-muted", p.volver),
		titulo: ui.Texto(p.t("trash.title"), ui.EscalonTitulo(2), ui.PesoTitulo(2), tema.Color("text", v)),
		lema:   ui.NuevoParrafo(p.t("trash.subtitle"), tema.TextoBase, ui.Normal, "text-muted", v),
	}
	objs := []fyne.CanvasObject{m.fondo, m.volver, m.titulo, m.lema}
	if p.Estado == Listo && len(p.Docs) > 0 {
		m.vaciar = ui.NuevoBoton(p.t("trash.empty"), p.PedirVaciar).Con(ui.Peligro, ui.TallaSM)
		m.vaciar.Inerte = p.Ocupado || !p.hayRed()
		objs = append(objs, m.vaciar)
	}
	m.cuerpo = p.cuerpo(v)
	objs = append(objs, m.cuerpo)
	return container.New(m, objs...)
}

func (p *PapeleraDocs) volver() {
	if p.AlVolver != nil {
		p.AlVolver()
	}
}

func (p *PapeleraDocs) cuerpo(v tema.Variante) fyne.CanvasObject {
	aviso := func() fyne.CanvasObject { return iconos.Trazo(iconos.Aviso, 40, "danger-text", v).Objeto() }
	var e *ui.Vacio
	switch p.Estado {
	case Cargando:
		esqueletos := make([]fyne.CanvasObject, EsqueletosPapelera)
		for i := range esqueletos {
			s := ui.NuevoEsqueleto(0, AltoEsqueletoPapelera)
			s.Radio = tema.RadioLG
			esqueletos[i] = s
		}
		return container.New(ui.Columna{Hueco: tema.Espacio(2)}, esqueletos...)
	case Listo:
		return p.listado(v)
	case Vacio:
		e = ui.NuevoVacio(p.t("trash.emptyTitle"), p.t("trash.emptyBody"))
		e.Ilustracion = tiposVacio(v)
	case SinConexion:
		e = ui.NuevoVacio(p.t("list.offline.emptyTitle"), p.t("list.offline.emptyDescription"))
		e.Ilustracion = aviso()
	case SinSesion:
		e = ui.NuevoVacio(p.t("trash.errorTitle"), p.t("pdf.error.session"))
		e.Ilustracion = aviso()
	case SinPermiso:
		e = ui.NuevoVacio(p.t("forbidden.title"), p.t("forbidden.description"))
		e.Ilustracion = aviso()
	case Error:
		e = ui.NuevoVacio(p.t("trash.errorTitle"), p.t("list.error.description"))
		e.Ilustracion = aviso()
		e.Accion = ui.NuevoBoton(p.t("list.retry"), p.Cargar).Con(ui.Secundario, ui.TallaSM)
	}
	return e
}

// listado es la ayuda en línea y una fila por documento, con desplazamiento.
func (p *PapeleraDocs) listado(v tema.Variante) fyne.CanvasObject {
	hijos := make([]fyne.CanvasObject, 0, 1+len(p.Docs))
	hijos = append(hijos, ui.NuevaAyudaEnLinea(p.t("trash.help")))
	for _, d := range p.Docs {
		hijos = append(hijos, p.fila(v, d))
	}
	return container.NewVScroll(container.New(ui.Columna{}, hijos...))
}

// fila es un documento: glifo del tipo, título, cuándo se tiró y los dos botones.
func (p *PapeleraDocs) fila(v tema.Variante, d api.Document) fyne.CanvasObject {
	doc := d
	f := &filaPapeleraDoc{
		glifo:  iconos.TrazoDeRuta(TipoDe(d.Kind).Glifo(), GlifoFilaPapelera, tema.Color("text", v)),
		titulo: ui.Texto(d.Title, tema.TextoBase, ui.Medio, tema.Color("text", v)),
		linea:  canvas.NewRectangle(tokens.ConAlfa(tema.Color("border", v), 0.6)),
	}
	if d.DeletedAt != nil {
		f.meta = ui.Texto(p.t("trash.deletedOn", i18n.Vars{
			"date": i18n.FormatDateShortMonth(p.Loc, d.DeletedAt.In(time.Local)),
		}), tema.TextoXS, ui.Normal, tema.Color("text-subtle", v))
	}
	f.restaurar = ui.NuevoBoton(p.t("trash.restore"), func() { p.Restaurar(doc) }).Con(ui.Secundario, ui.TallaSM)
	f.purgar = ui.NuevoBoton(p.t("trash.purge"), func() { p.PedirPurga(doc) }).Con(ui.Fantasma, ui.TallaSM)
	inerte := p.Ocupado || !p.hayRed()
	f.restaurar.Inerte, f.purgar.Inerte = inerte, inerte
	objs := []fyne.CanvasObject{f.glifo.Objeto(), f.titulo, f.restaurar, f.purgar, f.linea}
	if f.meta != nil {
		objs = append(objs, f.meta)
	}
	return container.New(f, objs...)
}

// MedidaFilaPapeleraDoc es el alto de una fila: `py-2` a cada lado, lo más alto de lo
// de dentro —el título y la fecha apilados (35) mandan sobre los botones (28) y el
// glifo (28)— y el `border-b`.
func MedidaFilaPapeleraDoc() float32 {
	return 2*RellenoFilaPapelera + max(tema.TextoBase.Linea+tema.TextoXS.Linea, GlifoFilaPapelera, ui.MedidaBoton("", ui.TallaSM, 1).Height) + tema.Borde
}

// filaPapeleraDoc coloca una fila de la papelera.
type filaPapeleraDoc struct {
	glifo             *iconos.Dibujo
	titulo, meta      *canvas.Text
	restaurar, purgar *ui.Boton
	linea             *canvas.Rectangle
}

var _ fyne.Layout = (*filaPapeleraDoc)(nil)

func (f *filaPapeleraDoc) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(2*MargenFilaPapelera+GlifoFilaPapelera, MedidaFilaPapeleraDoc())
}

func (f *filaPapeleraDoc) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	contenido := t.Height - tema.Borde
	f.linea.Move(fyne.NewPos(0, contenido))
	f.linea.Resize(fyne.NewSize(t.Width, tema.Borde))
	centro := func(alto float32) float32 { return (contenido - alto) / 2 }

	x := MargenFilaPapelera
	f.glifo.Colocar(x, centro(GlifoFilaPapelera), GlifoFilaPapelera)
	x += GlifoFilaPapelera + HuecoFilaPapelera

	derecha := t.Width - MargenFilaPapelera
	for _, b := range []*ui.Boton{f.purgar, f.restaurar} {
		m := b.MinSize()
		b.Move(fyne.NewPos(derecha-m.Width, centro(m.Height)))
		b.Resize(m)
		derecha -= m.Width + HuecoBotonesPapelera
	}
	cabe := max(derecha-HuecoFilaPapelera-x, 0)
	alto := tema.TextoBase.Linea
	if f.meta != nil {
		alto += tema.TextoXS.Linea
	}
	y := centro(alto)
	ui.Truncar(f.titulo, f.titulo.Text, tema.TextoBase, ui.Medio, cabe)
	ui.ColocarTexto(f.titulo, tema.TextoBase, x, y)
	if f.meta != nil {
		ui.Truncar(f.meta, f.meta.Text, tema.TextoXS, ui.Normal, cabe)
		ui.ColocarTexto(f.meta, tema.TextoXS, x, y+tema.TextoBase.Linea)
	}
}

// marcoPapelera coloca la pantalla: la columna centrada de la portada, la vuelta, el
// título con su lema y «Vaciar papelera» a la derecha, y el cuerpo con el resto.
type marcoPapelera struct {
	fondo  *canvas.Rectangle
	volver *ui.Enlace
	titulo *canvas.Text
	lema   *ui.Parrafo
	vaciar *ui.Boton
	cuerpo fyne.CanvasObject
}

var _ fyne.Layout = (*marcoPapelera)(nil)

func (m *marcoPapelera) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(2*MargenPortada, ArribaPortada+m.volver.MinSize().Height+ui.EscalonTitulo(2).Linea)
}

func (m *marcoPapelera) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	m.fondo.Resize(t)
	caja := min(t.Width, tema.AnchoContenido)
	x := (t.Width-caja)/2 + MargenPortada
	ancho := max(caja-2*MargenPortada, 0)
	y := ArribaPortada

	cabe := ancho
	if m.vaciar != nil {
		b := m.vaciar.MinSize()
		m.vaciar.Move(fyne.NewPos(x+ancho-b.Width, y))
		m.vaciar.Resize(b)
		cabe = max(ancho-b.Width-tema.Espacio(3), 0)
	}
	v := m.volver.MinSize()
	m.volver.Move(fyne.NewPos(x, y))
	m.volver.Resize(v)
	y += v.Height + BajoVuelta
	ui.ColocarTexto(m.titulo, ui.EscalonTitulo(2), x, y)
	y += ui.EscalonTitulo(2).Linea + BajoLema
	m.lema.Ancho = min(cabe, AnchoLema)
	l := m.lema.MinSize()
	m.lema.Move(fyne.NewPos(x, y))
	m.lema.Resize(fyne.NewSize(m.lema.Ancho, l.Height))
	y += l.Height + BajoCabeceraPapelera

	alto := max(t.Height-y, 0)
	if _, desplaza := m.cuerpo.(*container.Scroll); !desplaza {
		alto = min(m.cuerpo.MinSize().Height, alto)
	}
	m.cuerpo.Move(fyne.NewPos(x, y))
	m.cuerpo.Resize(fyne.NewSize(ancho, alto))
}
