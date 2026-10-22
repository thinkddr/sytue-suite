// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

// La portada de los tres editores; la pantalla de la web de cada producto la marca su
// `cmd/<producto>/main.go`, que es quien la monta con su tipo.

import (
	"context"
	"errors"
	"image/color"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
)

// La portada de documentos: `DocList.tsx` bajo un producto (`?kind=`), que es lo que
// abre cada app —Docus con los documentos de texto, Calcus con las hojas, Diapos con las
// presentaciones—. Vive en el cromo compartido y no en Docus porque en la web es UNA
// isla con el tipo como parámetro, y tres copias serían tres portadas que divergen.
//
// Lo que NO trae todavía, con su motivo, está en `ponytail:` en su sitio: abrir un
// fichero del ordenador (no hay selector de ficheros nativo en `apps/fyne`), descargar
// y compartir desde la tesela (el SDK no tiene las exportaciones ni los permisos), y el
// modo sin conexión con los documentos guardados en el dispositivo (la portada nativa no
// tiene caché local: sin red dice que no hay red).

// Las constantes de la web que gobiernan la carga.
const (
	// PaginaPortada es el `PAGE_SIZE`: el servicio acota a 1..200.
	PaginaPortada = 100
	// CercaDelFinal es el `LOAD_MORE_PX`: a esta distancia del final se pide la página
	// siguiente.
	CercaDelFinal float32 = 600
	// PausaFiltro es el `FILTER_DEBOUNCE_MS` del filtro por título.
	PausaFiltro = 250 * time.Millisecond
	// EsqueletosPortada son las ocho teselas de carga, de 164 px de alto.
	EsqueletosPortada               = 8
	AltoEsqueletoPortada    float32 = 164
	AltoMinimoTarjetaEstado float32 = 280 // `min-h-[280px]` del CardBody
)

// Medidas del marco de la portada: `max-w-[1280px] px-6 pt-8`, la cabecera con
// `mb-8`, «Crear nuevo» con `mb-10` y los encabezados de sección con `mb-3 gap-2`.
var (
	MargenPortada             = tema.Espacio(6)  // px-6 → 21
	ArribaPortada             = tema.Espacio(8)  // pt-8 → 28
	BajoCabecera              = tema.Espacio(8)  // mb-8 → 28
	BajoLema                  = tema.Espacio(1)  // mt-1 → 3,5
	HuecoPapelera             = tema.Espacio(3)  // gap-3 → 12, título ↔ «Papelera»
	AnchoLema         float32 = 42 * tema.Rem    // max-w-2xl → 672
	BajoNuevo                 = tema.Espacio(10) // mb-10 → 35
	BajoEncabezado            = tema.Espacio(3)  // mb-3 → 10,5
	HuecoEncabezado           = tema.Espacio(2)  // gap-2 → 7
	HuecoAccesos              = tema.Espacio(3)  // gap-3 → 12
	AnchoBuscador             = tema.Espacio(64) // w-64 → 224
	AltoFranjaPortada         = tema.Espacio(8)  // h-8 → 28
	RuedaFranja       float32 = 14
	// La banda sin conexión: `mb-5 rounded-lg border px-3 py-2 text-sm gap-2` y el punto
	// `h-2 w-2`.
	BajoBandaRed    = tema.Espacio(5)
	MargenBandaRed  = tema.Espacio(3)
	RellenoBandaRed = tema.Espacio(2)
	PuntoBandaRed   = tema.Espacio(2)
)

// Portada es la pantalla y su estado. Se construye con NuevaPortada y se arranca con
// Cargar.
type Portada struct {
	Tipo Tipo
	T    func(string, i18n.Vars) string
	Loc  i18n.Locale
	// Svc es el cliente del servicio. nil: la portada se pinta y no pide nada.
	Svc *api.Docs
	// Red es el monitor de conectividad. nil: se asume que hay red.
	Red *offline.Monitor
	// Plazo es cuánto se espera a una petición.
	Plazo time.Duration
	// Avisos es la pila de avisos de la ventana; sin ella los fallos no se anuncian.
	Avisos *ui.PilaAvisos
	// AlAbrir abre el documento en el editor. Es de la app, no de la portada.
	AlAbrir func(api.Document)
	// AlEntrar abre Sytue ID. Solo se ofrece cuando el servicio dice 401.
	AlEntrar func()

	// AlAbrirArchivo es «Abrir un archivo», el botón de la fila de «Crear nuevo»: el
	// mismo `AbrirArchivo.Abrir` del menú Archivo del editor. nil: no sale.
	AlAbrirArchivo func()

	// ConPlantillas pide y enseña la galería de plantillas de «Crear nuevo» y el selector
	// de tema (plantillas.go). Lo encienden las apps de Office; sin él, solo «En blanco».
	ConPlantillas bool
	// Lo que trae el servicio para esa galería; se rellena en CargarPlantillas.
	Plantillas, PlantillasOrg []api.DocTemplate
	Temas                     []OpcionTema
	Tema                      string // el elegido para crear; vacío = como viene
	Marca                     *api.DocBrand
	MarcaVersiones            []api.DocBrand
	PuedeMarca                bool
	CreandoClave              string // la plantilla que se está creando
	versionPlantilla          map[string]string
	plantillasPedidas         bool

	// Lanzar corre una petición fuera del hilo de la interfaz; nil es una rutina nueva.
	// Las pruebas la cambian por una llamada directa para no tener que esperar a nadie.
	Lanzar func(func())

	// raiz es lo que se enseña: la portada o, abierta, su papelera (papelera.go).
	raiz     *fyne.Container
	papelera *PapeleraDocs

	mu         sync.Mutex
	seq        int
	pausa      *time.Timer
	masOcupado bool

	// Lo que se pinta. Solo se toca en el hilo de la interfaz.
	Estado      Estado
	Docs        []api.Document
	Cursor      string
	Consulta    string // lo que hay escrito
	Aplicada    string // lo que se ha pedido, tras la pausa
	Destacados  bool
	Creando     bool
	CargandoMas bool

	vista *vistaPortada
}

// NuevaPortada crea la portada de ese tipo.
func NuevaPortada(tipo Tipo, t func(string, i18n.Vars) string, loc i18n.Locale, svc *api.Docs) *Portada {
	p := &Portada{Tipo: tipo, T: t, Loc: loc, Svc: svc, Plazo: 30 * time.Second, Estado: Cargando}
	p.vista = nuevaVistaPortada(p)
	p.raiz = container.NewStack(p.vista)
	return p
}

// Objeto es la pantalla: la portada o su papelera, la que esté abierta.
func (p *Portada) Objeto() fyne.CanvasObject { return p.raiz }

// Papelera es la papelera abierta, o nil.
func (p *Portada) Papelera() *PapeleraDocs { return p.papelera }

// AbrirPapelera enseña la papelera del producto en lugar de la portada, con el mismo
// servicio, red y avisos.
func (p *Portada) AbrirPapelera() {
	x := NuevaPapeleraDocs(p.Tipo, p.T, p.Loc, p.Svc)
	x.Red, x.Plazo, x.Avisos, x.Lanzar = p.Red, p.Plazo, p.Avisos, p.Lanzar
	x.AlVolver = p.CerrarPapelera
	p.papelera = x
	p.raiz.Objects = []fyne.CanvasObject{x.Objeto()}
	p.raiz.Refresh()
	x.Cargar()
}

// CerrarPapelera vuelve a la portada con la lista al día: lo restaurado reaparece.
func (p *Portada) CerrarPapelera() {
	if p.papelera != nil {
		p.papelera.Cerrar()
	}
	p.papelera = nil
	p.raiz.Objects = []fyne.CanvasObject{p.vista}
	p.raiz.Refresh()
	p.Cargar()
}

func (p *Portada) t(clave string, v ...i18n.Vars) string {
	if p.T == nil {
		return clave
	}
	if len(v) == 0 {
		return p.T(clave, nil)
	}
	return p.T(clave, v[0])
}

func (p *Portada) lanzar(f func()) {
	if p.Lanzar != nil {
		p.Lanzar(f)
		return
	}
	go f()
}

func (p *Portada) hayRed() bool { return p.Red == nil || p.Red.State() != offline.Offline }

func (p *Portada) ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), p.Plazo)
}

// EstadoDe traduce el resultado de una carga al estado que toca pintar, en el orden de
// la web: sin red no se enseña un error de red, un 401 es la sesión y un 403 es
// permiso. Solo lo que sobrevive a los tres es «no se pudo cargar».
func EstadoDe(err error, hayRed bool, elementos int) Estado {
	if !hayRed {
		return SinConexion
	}
	var e *api.Error
	if errors.As(err, &e) {
		switch e.Status {
		case http.StatusUnauthorized:
			return SinSesion
		case http.StatusForbidden:
			return SinPermiso
		}
	}
	switch {
	case err != nil:
		return Error
	case elementos == 0:
		return Vacio
	}
	return Listo
}

// Repintar rehace la vista con el estado de ahora. Se llama en el hilo de la interfaz.
func (p *Portada) Repintar() { p.vista.rehacer() }

// avisar anuncia un fallo en la pila de avisos, si la hay.
func (p *Portada) avisar(clave string) {
	if p.Avisos == nil {
		return
	}
	p.Avisos.Notificar(ui.NuevoAviso(ui.AvisoPeligro, p.t(clave), ""))
}

// filtro es lo que se le pide al servicio con el estado de ahora.
func (p *Portada) filtro(cursor string) api.DocsFilter {
	return api.DocsFilter{
		Kind: p.Tipo.Kind(), Query: p.Aplicada, Starred: p.Destacados,
		Cursor: cursor, Limit: PaginaPortada,
	}
}

// Cargar pide la primera página con el filtro de ahora. Se llama en el hilo de la
// interfaz; la petición va en su propia rutina.
func (p *Portada) Cargar() {
	p.mu.Lock()
	p.seq++
	seq := p.seq
	p.mu.Unlock()
	p.Estado, p.Docs, p.Cursor = Cargando, nil, ""
	p.Repintar()
	if p.Svc == nil {
		return
	}
	if !p.hayRed() {
		p.Estado = SinConexion
		p.Repintar()
		return
	}
	if p.ConPlantillas && !p.plantillasPedidas {
		p.plantillasPedidas = true // la galería no cambia con la búsqueda: una vez
		p.CargarPlantillas()
	}
	f := p.filtro("")
	p.lanzar(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		docs, cursor, err := p.Svc.List(ctx, f)
		fyne.Do(func() {
			if !p.vigente(seq) {
				return // llegó tarde: ya se pidió otro listado
			}
			p.Docs, p.Cursor = docs, cursor
			p.Estado = EstadoDe(err, p.hayRed(), len(docs))
			p.Repintar()
		})
	})
}

func (p *Portada) vigente(seq int) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return seq == p.seq
}

// Mas pide la página siguiente, si la hay. Tiene candado propio: el desplazamiento
// dispara en ráfaga y el estado llega tarde, igual que en la web.
func (p *Portada) Mas() {
	p.mu.Lock()
	if p.masOcupado || p.Cursor == "" || p.Svc == nil || p.Estado != Listo {
		p.mu.Unlock()
		return
	}
	p.masOcupado = true
	seq := p.seq
	p.mu.Unlock()
	p.CargandoMas = true
	p.vista.franja()
	f := p.filtro(p.Cursor)
	p.lanzar(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		docs, cursor, err := p.Svc.List(ctx, f)
		fyne.Do(func() {
			p.mu.Lock()
			p.masOcupado = false
			p.mu.Unlock()
			p.CargandoMas = false
			if !p.vigente(seq) {
				p.vista.franja()
				return
			}
			if err != nil {
				p.avisar("list.error.more")
				p.vista.franja()
				return
			}
			// Sin duplicados: un documento tocado entre dos páginas puede repetirse
			// con la paginación por clave.
			visto := make(map[string]bool, len(p.Docs))
			for _, d := range p.Docs {
				visto[d.ID] = true
			}
			for _, d := range docs {
				if !visto[d.ID] {
					p.Docs = append(p.Docs, d)
				}
			}
			p.Cursor = cursor
			p.Repintar()
		})
	})
}

// Buscar recibe lo que se teclea y lo aplica tras la pausa de la web.
func (p *Portada) Buscar(texto string) {
	p.Consulta = texto
	p.mu.Lock()
	if p.pausa != nil {
		p.pausa.Stop()
	}
	p.pausa = time.AfterFunc(PausaFiltro, func() { fyne.Do(p.aplicarConsulta) })
	p.mu.Unlock()
}

// aplicarConsulta pide la lista con lo que haya escrito, si ha cambiado.
func (p *Portada) aplicarConsulta() {
	if q := strings.TrimSpace(p.Consulta); q != p.Aplicada {
		p.Aplicada = q
		p.Cargar()
	}
}

// AlternarDestacados enciende o apaga el filtro, que resuelve el SERVICIO.
func (p *Portada) AlternarDestacados() {
	p.Destacados = !p.Destacados
	p.Cargar()
}

// Crear da de alta un documento en blanco del tipo del producto y lo abre. Sin id en
// la respuesta NO se navega: se avisa.
func (p *Portada) Crear() {
	if p.Svc == nil || p.Creando || !p.hayRed() {
		return
	}
	p.Creando = true
	p.Repintar()
	p.lanzar(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		d, err := p.Svc.Create(ctx, p.Tipo.Kind(), "")
		fyne.Do(func() {
			p.Creando = false
			if err != nil || d.ID == "" {
				if p.Avisos != nil {
					p.Avisos.Notificar(ui.NuevoAviso(ui.AvisoPeligro,
						p.t("create.error.title"), p.t("create.error.description")))
				}
				p.Repintar()
				return
			}
			p.Repintar()
			if p.AlAbrir != nil {
				p.AlAbrir(d)
			}
		})
	})
}

// Destacar marca o desmarca, OPTIMISTA y reversible: la estrella cambia al instante y
// vuelve si el servicio dice que no. Con el filtro puesto, quitarla saca la tesela.
func (p *Portada) Destacar(d api.Document) {
	if p.Svc == nil {
		return
	}
	siguiente := !d.Starred
	p.ponerEstrella(d.ID, siguiente)
	p.lanzar(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		err := p.Svc.Star(ctx, d.ID, siguiente)
		fyne.Do(func() {
			if err != nil {
				p.ponerEstrella(d.ID, d.Starred)
				p.avisar("tile.star.failed")
				return
			}
			if p.Destacados && !siguiente {
				p.quitar(d.ID)
			}
		})
	})
}

func (p *Portada) ponerEstrella(id string, si bool) {
	for i := range p.Docs {
		if p.Docs[i].ID == id {
			p.Docs[i].Starred = si
		}
	}
	p.Repintar()
}

func (p *Portada) quitar(id string) {
	out := p.Docs[:0]
	for _, d := range p.Docs {
		if d.ID != id {
			out = append(out, d)
		}
	}
	p.Docs = out
	if len(p.Docs) == 0 {
		p.Estado = Vacio
	}
	p.Repintar()
}

func (p *Portada) lienzo() fyne.Canvas {
	if app := fyne.CurrentApp(); app != nil {
		return app.Driver().CanvasForObject(p.vista)
	}
	return nil
}

// PedirNombre abre el diálogo de renombrar: el campo con el título, Intro envía, y el
// botón no se deja pulsar con el título vacío.
func (p *Portada) PedirNombre(d api.Document) {
	lienzo := p.lienzo()
	if lienzo == nil {
		return
	}
	campo := ui.NuevaEntrada("")
	campo.PonerTexto(d.Title)
	var dlg *ui.Dialogo
	var enviar *ui.Boton
	mandar := func() {
		titulo := strings.TrimSpace(campo.Texto())
		if titulo == "" || enviar.Cargando {
			return
		}
		enviar.Cargando = true
		enviar.Refresh()
		p.lanzar(func() {
			p.renombrar(d.ID, titulo, func(ok bool) {
				enviar.Cargando = false
				enviar.Refresh()
				if ok {
					dlg.Cerrar()
				}
			})
		})
	}
	cancelar := ui.NuevoBoton(p.t("action.cancel"), func() { dlg.Cerrar() }).Con(ui.Fantasma, ui.TallaMD)
	enviar = ui.NuevoBoton(p.t("rename.submit"), mandar)
	campo.AlCambiar(func(s string) { enviar.Inerte = strings.TrimSpace(s) == ""; enviar.Refresh() })
	campo.AlTeclear = func(e *fyne.KeyEvent) bool {
		if e.Name == fyne.KeyReturn || e.Name == fyne.KeyEnter {
			mandar()
			return true
		}
		return false
	}
	rotulo := ui.NuevoRotulo(p.t("rename.field"), tema.TextoSM, ui.SemiNegrita, "text")
	cuerpo := container.New(ui.Columna{Hueco: tema.Espacio(1.5)}, rotulo, campo)
	pie := container.New(ui.Fila{Hueco: ui.HuecoPieDialogo}, cancelar, enviar)
	dlg = ui.NuevoDialogo(p.t("rename.title"), cuerpo).Con(ui.DialogoMD, pie)
	dlg.RotuloCerrar = p.t("ui.close")
	dlg.Mostrar(lienzo)
}

func (p *Portada) renombrar(id, titulo string, listo func(bool)) {
	ctx, cancelar := p.ctx()
	defer cancelar()
	err := p.Svc.Rename(ctx, id, titulo)
	fyne.Do(func() {
		if err != nil {
			p.avisar("rename.error")
			listo(false)
			return
		}
		for i := range p.Docs {
			if p.Docs[i].ID == id {
				p.Docs[i].Title = titulo
			}
		}
		p.Repintar()
		listo(true)
	})
}

// PedirBorrado confirma antes de borrar: es una acción destructiva y nunca va directa.
func (p *Portada) PedirBorrado(d api.Document) {
	lienzo := p.lienzo()
	if lienzo == nil {
		return
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	cuerpo := ui.NuevoParrafo(p.t("delete.body", i18n.Vars{"title": d.Title}), tema.TextoBase, ui.Normal, "text-muted", v)
	var dlg *ui.Dialogo
	dlg = ui.Confirmacion(p.t("delete.title"), p.t("action.cancel"), p.t("delete.submit"), true, cuerpo,
		func() { dlg.Cerrar() },
		func() {
			dlg.Cerrar()
			p.lanzar(func() { p.borrar(d.ID) })
		})
	dlg.RotuloCerrar = p.t("ui.close")
	dlg.Mostrar(lienzo)
}

func (p *Portada) borrar(id string) {
	ctx, cancelar := p.ctx()
	defer cancelar()
	err := p.Svc.Delete(ctx, id)
	fyne.Do(func() {
		if err != nil {
			p.avisar("delete.error")
			return
		}
		p.quitar(id)
	})
}

// ---- la vista ----

// vistaPortada es el widget de la portada: cabecera, «Crear nuevo», «Recientes» y el
// cuerpo que toque según el estado.
type vistaPortada struct {
	widget.BaseWidget
	p *Portada

	acceso   *AccesoNuevo
	galeria  fyne.CanvasObject
	abrir    *ui.Boton
	filtro   *FiltroDestacados
	papelera *FiltroDestacados
	buscador *ui.Entrada
	insignia *ui.Insignia
	cuerpo   fyne.CanvasObject
	scroll   *container.Scroll
	rejilla  *rejillaPortada
	r        *vistaRenderer
}

func nuevaVistaPortada(p *Portada) *vistaPortada {
	w := &vistaPortada{p: p}
	w.ExtendBaseWidget(w)
	w.acceso = NuevoAccesoNuevo(p.Tipo, p.t2(), p.Crear)
	// `variant="secondary" size="sm" ml-auto`, con el glifo `upload` de 14 delante.
	w.abrir = ui.NuevoBoton(p.t("list.new.open"), func() {
		if p.AlAbrirArchivo != nil {
			p.AlAbrirArchivo()
		}
	}).Con(ui.Secundario, ui.TallaSM)
	w.abrir.Glifo = iconos.TrazoDeRuta(GlifoSubir, 14, tema.Color("text", tema.De(fyne.CurrentApp().Settings().ThemeVariant())))
	w.abrir.TamIcono = 14
	w.filtro = NuevoFiltroDestacados(p.t("list.filter.starred"), p.AlternarDestacados)
	w.papelera = NuevoAccesoPapelera(p.t("list.trash"), p.AbrirPapelera)
	w.buscador = ui.NuevaEntrada(p.t("list.filter.placeholder"))
	w.buscador.Icono = iconos.Buscar
	w.buscador.Ancho = AnchoBuscador
	w.buscador.AlCambiar(p.Buscar)
	w.insignia = ui.NuevaInsignia("", ui.InsigniaNeutra)
	w.rehacerCuerpo()
	return w
}

// t2 es el traductor con la firma de las piezas, con el hueco tapado.
func (p *Portada) t2() func(string, i18n.Vars) string { return conClaves(p.T) }

// rehacer recompone lo que depende del estado y repinta.
func (w *vistaPortada) rehacer() {
	p := w.p
	w.acceso.Creando = p.Creando
	w.acceso.Inerte = p.Creando || !p.hayRed() || p.Svc == nil
	// `disabled={offline || creating !== null || importing}`.
	w.abrir.Inerte = p.Creando || !p.hayRed()
	w.filtro.Activo = p.Destacados
	w.insignia.Texto = strconv.Itoa(len(p.Docs))
	if p.Cursor != "" {
		w.insignia.Texto += "+"
	}
	w.galeria = p.galeria()
	w.rehacerCuerpo()
	w.Refresh()
}

// franja repinta solo la franja de «Cargando más…», que no cambia el cuerpo.
func (w *vistaPortada) franja() { w.Refresh() }

func (w *vistaPortada) rehacerCuerpo() {
	p := w.p
	w.rejilla = nil
	switch p.Estado {
	case Cargando:
		esqueletos := make([]fyne.CanvasObject, EsqueletosPortada)
		for i := range esqueletos {
			e := ui.NuevoEsqueleto(0, AltoEsqueletoPortada)
			e.Radio = tema.RadioLG
			esqueletos[i] = e
		}
		w.rejilla = &rejillaPortada{auto: true, alto: AltoEsqueletoPortada}
		w.cuerpo = container.New(w.rejilla, esqueletos...)
	case Listo:
		teselas := make([]fyne.CanvasObject, len(p.Docs))
		acc := AccionesTesela{Abrir: p.AlAbrir, Destacar: p.Destacar, Renombrar: p.PedirNombre, Eliminar: p.PedirBorrado}
		for i, d := range p.Docs {
			s := NuevaTeselaDoc(d, p.t2(), p.Loc, acc)
			s.SinRed = !p.hayRed()
			teselas[i] = s
		}
		// ponytail: sin virtualizar. La web pinta solo las filas visibles; aquí se
		// montan todas las teselas cargadas (100 por página). Techo: unos pocos miles
		// de documentos antes de notarlo al desplazar; el relevo es trocear en filas
		// sobre `widget.List`, que ya recicla.
		w.rejilla = &rejillaPortada{alto: MedidaTesela(), porFila: HuecoRejillaTesela}
		w.cuerpo = container.New(w.rejilla, teselas...)
	default:
		w.cuerpo = ui.NuevaTarjeta("", container.New(minimo{alto: AltoMinimoTarjetaEstado - 2*tema.Espacio(4) - 2*tema.Borde}, w.vacio()))
	}
}

// vacio es el EmptyState de cada estado que no es «hay documentos».
func (w *vistaPortada) vacio() *ui.Vacio {
	p := w.p
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	aviso := func() fyne.CanvasObject { return iconos.Trazo(iconos.Aviso, 40, "danger-text", v).Objeto() }
	reintentar := ui.NuevoBoton(p.t("list.retry"), p.Cargar).Con(ui.Secundario, ui.TallaSM)
	var e *ui.Vacio
	switch p.Estado {
	case SinConexion:
		e = ui.NuevoVacio(p.t("list.offline.emptyTitle"), p.t("list.offline.emptyDescription"))
		e.Ilustracion, e.Accion = aviso(), reintentar
	case SinSesion:
		// La web nunca llega aquí: sin sesión redirige al inicio de sesión. En nativo
		// lo que se hace es lo mismo, pero lo pide quien mira: abrir el navegador por su
		// cuenta al arrancar sería peor que un botón.
		e = ui.NuevoVacio(p.t("list.error.title"), p.t("pdf.error.session"))
		e.Ilustracion = aviso()
		if p.AlEntrar != nil {
			e.Accion = ui.NuevoBoton(p.t("account.signIn"), p.AlEntrar).Con(ui.Primario, ui.TallaSM)
		}
	case SinPermiso:
		e = ui.NuevoVacio(p.t("forbidden.title"), p.t("forbidden.description"))
		e.Ilustracion = aviso()
	case Vacio:
		if p.Aplicada != "" {
			e = ui.NuevoVacio(p.t("list.empty.search.title", i18n.Vars{"query": p.Aplicada}), p.t("list.empty.search.description"))
			e.Ilustracion = iconos.Pintar(iconos.VacioBusqueda, v).Objeto()
			break
		}
		e = ui.NuevoVacio(p.t("list.empty.title"), p.t("list.empty.description"))
		e.Ilustracion = tiposVacio(v)
		crear := ui.NuevoBoton(p.t("list.empty.action"), p.Crear).Con(ui.Primario, ui.TallaSM)
		crear.Cargando = p.Creando
		e.Accion = crear
	default: // Error
		e = ui.NuevoVacio(p.t("list.error.title"), p.t("list.error.description"))
		e.Ilustracion, e.Accion = aviso(), reintentar
	}
	return e
}

// tiposVacio son las tres teselas de `bg-bg-subtle` con el glifo de cada tipo que la
// portada vacía enseña en vez de una ilustración: `flex gap-2`, `h-11 w-11 rounded-lg`,
// glifo de 22 en `text-subtle`.
func tiposVacio(v tema.Variante) fyne.CanvasObject {
	hijos := make([]fyne.CanvasObject, 0, 3)
	for _, t := range []Tipo{Documento, Hoja, Presentacion} {
		fondo := canvas.NewRectangle(tema.Color("bg-subtle", v))
		fondo.CornerRadius = tema.RadioLG
		g := iconos.TrazoDeRuta(t.Glifo(), GlifoAcceso, tema.Color("text-subtle", v))
		hijos = append(hijos, container.New(teselaGlifo{}, fondo, g.Objeto()))
	}
	c := container.New(ui.Fila{Hueco: tema.Espacio(2)}, hijos...)
	c.Resize(c.MinSize())
	return c
}

// teselaGlifo coloca un fondo de 38,5 y un glifo de 22 centrado.
type teselaGlifo struct{}

func (teselaGlifo) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(LadoTeselaAcceso, LadoTeselaAcceso)
}

func (teselaGlifo) Layout(objs []fyne.CanvasObject, _ fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(fyne.NewSize(LadoTeselaAcceso, LadoTeselaAcceso))
	d := (LadoTeselaAcceso - GlifoAcceso) / 2
	objs[1].Move(fyne.NewPos(d, d))
	objs[1].Resize(fyne.NewSize(GlifoAcceso, GlifoAcceso))
}

// minimo es un `min-h-*` para un hijo que se centra solo en el hueco que le den.
type minimo struct{ alto float32 }

func (m minimo) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, max(m.alto, objs[0].MinSize().Height))
}

func (m minimo) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(t)
}

// rejillaPortada es la rejilla de la lista: tantas columnas como dice la cuenta de la
// web a ese ancho, el `gap-4` entre teselas y el `pb-4` debajo de CADA fila. Con `auto`
// es la de los esqueletos, que es un `auto-fill` de `minmax(180px, 1fr)` sin `pb`.
//
// Guarda el último ancho porque `fyne.Layout.MinSize` no lo recibe y el alto depende de
// él; quien la coloca se lo pone antes de redimensionar el desplazador.
type rejillaPortada struct {
	ancho, alto, porFila float32
	auto                 bool
}

func (r *rejillaPortada) rejilla() ui.Rejilla {
	if r.auto {
		return ui.Rejilla{Min: MinTesela, Hueco: HuecoRejillaTesela}
	}
	return ui.Rejilla{Fijas: ColumnasPortada(r.ancho), Hueco: HuecoRejillaTesela}
}

func (r *rejillaPortada) MinSize(objs []fyne.CanvasObject) fyne.Size {
	cols := r.rejilla().Columnas(r.ancho)
	filas := (len(objs) + cols - 1) / cols
	if filas == 0 {
		return fyne.NewSize(r.ancho, 0)
	}
	return fyne.NewSize(r.ancho, float32(filas)*(r.alto+r.porFila)+float32(filas-1)*(HuecoRejillaTesela-r.porFila))
}

func (r *rejillaPortada) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	r.ancho = t.Width
	rj := r.rejilla()
	ancho, cols := rj.AnchoColumna(t.Width), rj.Columnas(t.Width)
	for i, o := range objs {
		o.Move(fyne.NewPos(rj.X(t.Width, i%cols), float32(i/cols)*(r.alto+HuecoRejillaTesela)))
		o.Resize(fyne.NewSize(ancho, r.alto))
	}
}

// AccessibilityRole es la región principal de la app.
func (w *vistaPortada) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

// MinSize es lo mínimo con que la portada se deja ver.
func (w *vistaPortada) MinSize() fyne.Size {
	w.ExtendBaseWidget(w)
	return fyne.NewSize(MinTesela+2*MargenPortada, 0)
}

func (w *vistaPortada) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	p := w.p
	r := &vistaRenderer{
		w:        w,
		fondo:    canvas.NewRectangle(color.NRGBA{}),
		titulo:   ui.Texto(p.t(claveCabeceraPortada(p.Tipo)), ui.EscalonTitulo(2), ui.PesoTitulo(2), color.NRGBA{}),
		nuevo:    ui.NuevoVersal(p.t("list.new.heading"), tema.TextoXS, ui.SemiNegrita, tema.Color("text-subtle", v)),
		reciente: ui.NuevoVersal(p.t("list.recent.heading"), tema.TextoXS, ui.SemiNegrita, tema.Color("text-subtle", v)),
		banda:    ui.NuevaCaja(tema.Color("surface", v), tema.Color("border", v), tema.RadioLG),
		punto:    canvas.NewCircle(color.NRGBA{}),
		redT:     ui.Texto(p.t("list.offline.title"), tema.TextoSM, ui.Negrita, color.NRGBA{}),
		redD:     ui.Texto(p.t("list.offline.description"), tema.TextoSM, ui.Normal, color.NRGBA{}),
		rueda:    ui.NuevaRueda(RuedaFranja, "text-muted", v),
		mas:      ui.Texto(p.t("list.loadingMore"), tema.TextoSM, ui.Normal, color.NRGBA{}),
	}
	w.r = r
	// Un solo desplazamiento para la cabecera, las plantillas y los recientes.
	// AlAnchoFluido evita que el ancho de la galería fije el mínimo de la ventana;
	// el fondo declara el alto total medido por Layout y la página conserva sus
	// posiciones propias dentro del desplazador.
	r.pagina = container.NewWithoutLayout()
	w.scroll = container.NewVScroll(ui.AlAnchoFluido(0, r.pagina))
	w.scroll.OnScrolled = func(pos fyne.Position) {
		if p.Estado == Listo && pos.Y+w.scroll.Size().Height >= w.scroll.Content.MinSize().Height-CercaDelFinal {
			p.Mas()
		}
	}
	r.Refresh()
	return r
}

// claveCabeceraPortada es la clave de la cabecera de cada producto. Se enumeran, en
// lugar de fabricarlas, para que el guardián de catálogos compruebe que todas existen.
func claveCabeceraPortada(t Tipo) string {
	switch t {
	case Hoja:
		return "list.heading.spreadsheet"
	case Presentacion:
		return "list.heading.presentation"
	case Documento:
	}
	return "list.heading.document"
}

// claveLemaPortada es la del lema de debajo de la cabecera, enumerada por lo mismo.
func claveLemaPortada(t Tipo) string {
	switch t {
	case Hoja:
		return "list.tagline.spreadsheet"
	case Presentacion:
		return "list.tagline.presentation"
	case Documento:
	}
	return "list.tagline.document"
}

type vistaRenderer struct {
	w               *vistaPortada
	pagina          *fyne.Container
	fondo           *canvas.Rectangle
	titulo          *canvas.Text
	lema            []*canvas.Text
	nuevo, reciente *ui.Versal
	banda           *ui.Caja
	punto           *canvas.Circle
	redT, redD      *canvas.Text
	rueda           *ui.Rueda
	mas             *canvas.Text
	ultimoAnchoLema float32
}

func (r *vistaRenderer) MinSize() fyne.Size { return r.w.MinSize() }
func (r *vistaRenderer) Destroy()           {}

func (r *vistaRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.w.scroll}
}

func (r *vistaRenderer) piezas() []fyne.CanvasObject {
	w := r.w
	objs := []fyne.CanvasObject{r.fondo, r.titulo}
	for _, l := range r.lema {
		objs = append(objs, l)
	}
	if !w.p.hayRed() {
		objs = append(objs, r.banda.Objetos()...)
		objs = append(objs, r.punto, r.redT, r.redD)
	}
	objs = append(objs, r.nuevo.Objetos()...)
	if w.p.AlAbrirArchivo != nil {
		objs = append(objs, w.abrir)
	}
	objs = append(objs, w.acceso)
	if w.galeria != nil {
		objs = append(objs, w.galeria)
	}
	objs = append(objs, r.reciente.Objetos()...)
	if len(w.p.Docs) > 0 && w.p.Estado == Listo {
		objs = append(objs, w.insignia)
	}
	objs = append(objs, w.papelera, w.filtro, w.buscador)
	if w.cuerpo != nil {
		objs = append(objs, w.cuerpo)
	}
	if w.p.CargandoMas {
		objs = append(objs, r.rueda, r.mas)
	}
	return objs
}

// lemaEn reparte la frase de bajo el título en el ancho que le toca (`max-w-2xl`).
func (r *vistaRenderer) lemaEn(ancho float32, v tema.Variante) {
	if ancho == r.ultimoAnchoLema && len(r.lema) > 0 {
		return
	}
	r.ultimoAnchoLema = ancho
	r.lema = nil
	p := r.w.p
	for _, l := range ui.Repartir(p.t(claveLemaPortada(p.Tipo)), tema.TextoBase, ui.Normal, ancho) {
		r.lema = append(r.lema, ui.Texto(l, tema.TextoBase, ui.Normal, tema.Color("text-muted", v)))
	}
}

func (r *vistaRenderer) Layout(t fyne.Size) {
	w := r.w
	p := w.p
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())

	// `mx-auto max-w-[1280px] px-6`: la columna centrada con su margen.
	caja := min(t.Width, tema.AnchoContenido)
	x := (t.Width-caja)/2 + MargenPortada
	ancho := max(caja-2*MargenPortada, 0)

	// La cabecera es `flex items-start gap-3`: el título y el lema a la izquierda y
	// «Papelera» arriba a la derecha. En la fila de «Recientes» no cabía a 360.
	y := ArribaPortada
	pap := w.papelera.MinSize()
	w.papelera.Move(fyne.NewPos(x+ancho-pap.Width, y))
	w.papelera.Resize(pap)
	izq := max(ancho-pap.Width-HuecoPapelera, 0)
	ui.Truncar(r.titulo, p.t(claveCabeceraPortada(p.Tipo)), ui.EscalonTitulo(2), ui.PesoTitulo(2), izq)
	ui.ColocarTexto(r.titulo, ui.EscalonTitulo(2), x, y)
	y += ui.EscalonTitulo(2).Linea + BajoLema
	r.lemaEn(min(izq, AnchoLema), v)
	for _, l := range r.lema {
		ui.ColocarTexto(l, tema.TextoBase, x, y)
		y += tema.TextoBase.Linea
	}
	y += BajoCabecera

	if !p.hayRed() {
		alto := 2*tema.Borde + 2*RellenoBandaRed + tema.TextoSM.Linea
		ui.MoverCaja(r.banda, fyne.NewPos(x, y), fyne.NewSize(ancho, alto))
		px := x + tema.Borde + MargenBandaRed
		r.punto.Move(fyne.NewPos(px, y+(alto-PuntoBandaRed)/2))
		r.punto.Resize(fyne.NewSize(PuntoBandaRed, PuntoBandaRed))
		px += PuntoBandaRed + HuecoEncabezado
		ty := y + (alto-tema.TextoSM.Linea)/2
		ui.ColocarTexto(r.redT, tema.TextoSM, px, ty)
		// `<strong>` y un espacio: la descripción sigue en la misma línea.
		ui.ColocarTexto(r.redD, tema.TextoSM, px+ui.Medir(r.redT.Text+" ", tema.TextoSM, ui.Negrita).Width, ty)
		y += alto + BajoBandaRed
	}

	// «Crear nuevo»: el encabezado —con «Abrir un archivo» a la derecha si la app lo
	// ofrece— y la rejilla de accesos, que bajo un producto es UNO.
	fila := tema.TextoXS.Linea
	if p.AlAbrirArchivo != nil {
		m := w.abrir.MinSize()
		fila = max(fila, m.Height) // `flex items-center`: la fila mide lo que el botón
		w.abrir.Move(fyne.NewPos(x+ancho-m.Width, y))
		w.abrir.Resize(m)
	}
	r.nuevo.Colocar(x, y+(fila-tema.TextoXS.Linea)/2)
	y += fila + BajoEncabezado
	rj := ui.RejillaTarjetas(t.Width, HuecoAccesos, ui.Cortes{1, 2, 0, 4, 0, 0})
	w.acceso.Move(fyne.NewPos(x, y))
	w.acceso.Resize(fyne.NewSize(rj.AnchoColumna(ancho), MedidaAccesoNuevo()))
	y += MedidaAccesoNuevo()
	if w.galeria != nil {
		// Las filas de la galería, a todo el ancho, con su `mt-5` por delante.
		alto := MedidaGaleria(w.galeria)
		y += HuecoFilaGaleria
		w.galeria.Move(fyne.NewPos(x, y))
		w.galeria.Resize(fyne.NewSize(ancho, alto))
		y += alto
	}
	y += BajoNuevo

	// «Recientes»: encabezado, insignia y a la derecha el filtro y el buscador.
	busc := w.buscador.MinSize()
	fil := w.filtro.MinSize()
	conInsignia := len(p.Docs) > 0 && p.Estado == Listo
	// `w-64 max-w-[60%]` y, como es un hijo flex que encoge, nunca más de lo que dejan
	// el encabezado, la insignia y «Destacados»: a 360 medía el 60 % entero y empujaba
	// «Destacados» encima de «Recientes» (visto en una ventana real el 2026-09-27).
	izquierda := r.reciente.Ancho() + HuecoEncabezado + fil.Width + HuecoEncabezado
	if conInsignia {
		izquierda += ui.MedidaInsignia(w.insignia.Texto).Width + HuecoEncabezado
	}
	busc.Width = max(min(AnchoBuscador, 0.6*ancho, ancho-izquierda), 0)
	fila = max(busc.Height, fil.Height, tema.TextoXS.Linea)
	r.reciente.Colocar(x, y+(fila-tema.TextoXS.Linea)/2)
	if conInsignia {
		m := ui.MedidaInsignia(w.insignia.Texto)
		w.insignia.Move(fyne.NewPos(x+r.reciente.Ancho()+HuecoEncabezado, y+(fila-m.Height)/2))
		w.insignia.Resize(m)
	}
	der := x + ancho
	w.buscador.Move(fyne.NewPos(der-busc.Width, y+(fila-busc.Height)/2))
	w.buscador.Resize(busc)
	der -= busc.Width + HuecoEncabezado
	w.filtro.Move(fyne.NewPos(der-fil.Width, y+(fila-fil.Height)/2))
	w.filtro.Resize(fil)
	y += fila + BajoEncabezado

	// Los recientes conservan todas sus filas en el mismo flujo que las plantillas.
	if w.cuerpo != nil {
		if w.rejilla != nil {
			w.rejilla.ancho = ancho
		}
		alto := ui.AltoA(w.cuerpo, ancho)
		w.cuerpo.Move(fyne.NewPos(x, y))
		w.cuerpo.Resize(fyne.NewSize(ancho, alto))
		y += alto
	}
	fy := y
	mw := ui.Medir(r.mas.Text, tema.TextoSM, ui.Normal).Width
	fx := x + (ancho-RuedaFranja-HuecoEncabezado-mw)/2
	r.rueda.Move(fyne.NewPos(fx, fy+(AltoFranjaPortada-RuedaFranja)/2))
	r.rueda.Resize(ui.MedidaRueda(RuedaFranja))
	ui.ColocarTexto(r.mas, tema.TextoSM, fx+RuedaFranja+HuecoEncabezado, fy+(AltoFranjaPortada-tema.TextoSM.Linea)/2)
	alto := y + AltoFranjaPortada
	r.fondo.SetMinSize(fyne.NewSize(0, alto))
	r.fondo.Resize(fyne.NewSize(t.Width, max(alto, t.Height)))
	r.pagina.Objects = r.piezas()
	w.scroll.Resize(t)
	w.scroll.Refresh()
}

func (r *vistaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.fondo.FillColor = tema.Color("bg", v)
	r.titulo.Color = tema.Color("text", v)
	for _, l := range r.lema {
		l.Color = tema.Color("text-muted", v)
	}
	r.nuevo.Pintar(tema.Color("text-subtle", v))
	r.reciente.Pintar(tema.Color("text-subtle", v))
	r.banda.Pintar(tema.Color("warning-bg", v), tema.Color("warning", v))
	r.punto.FillColor = tema.Color("warning", v)
	r.redT.Color, r.redD.Color = tema.Color("warning-text", v), tema.Color("warning-text", v)
	r.mas.Color = tema.Color("text-muted", v)
	for _, o := range []fyne.CanvasObject{r.fondo, r.titulo, r.punto, r.redT, r.redD, r.mas} {
		o.Refresh()
	}
	r.w.insignia.Refresh()
	r.Layout(r.w.Size())
	canvas.Refresh(r.w)
}
