// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"context"
	"image"
	"image/color"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/certificados"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

/*
La PANTALLA de CertiPDF (`PdfApp.tsx`): la cinta arriba y, debajo, el documento o el
estado que toque.

Los estados van en un solo sitio ([Pantalla.cuerpo]) y no repartidos por cada trozo.
Se reutiliza [editor.Estado] —los mismos seis, exportados y sin dependencias— para el
vocabulario, pero NO se pintan como el `EmptyState` del editor: la web de este producto
tiene los suyos, escritos a mano en `PdfApp.tsx`:

  - cargando: la rueda de 16 y `pdf.loading` en `text-sm text-text-muted`, centrados;
  - sin documento (error de apertura, sin conexión, sesión caducada): la frase en
    `text-danger-text`, «Reintentar» a un clic y la salida al lado;
  - contraseña: el formulario de `PideContrasena`.

Lo PESADO —las hojas, su desplazamiento, los dibujos que ya llegaron— se construye UNA
vez por documento y se conserva entre recomposiciones: elegir una herramienta rehace la
cinta, no el documento, y el desplazamiento no vuelve arriba.
*/

// TraductorVars es el traductor con variables (`{size}`, `{n}`): la misma firma que
// `(*i18n.Translator).T`, así que `cmd/` pasa el método tal cual.
type TraductorVars = func(clave string, vars i18n.Vars) string

// Pantalla es la vista completa de un PDF abierto.
type Pantalla struct {
	t     Traductor
	tv    TraductorVars
	barra *Barra
	// panel es la columna lateral; se rehace cuando cambia el documento.
	panel *PanelLateral

	// Estado es cuál de los seis se pinta.
	Estado editor.Estado
	// PideClave enseña el formulario de contraseña, con ErrorClave debajo si la última
	// no valía.
	PideClave  bool
	ErrorClave string
	// Contenido es lo que devolvió `op: contenido`. Nil mientras no haya llegado.
	Contenido *Contenido
	// Zoom es el factor de dibujo; 1 es el tamaño natural de la página.
	Zoom float64
	// Ajuste es cómo se calcula el zoom; con [AjusteLibre] manda Zoom.
	Ajuste Ajuste
	// DosPaginas pone dos hojas por fila.
	DosPaginas bool
	// PaginaActual es la que se está mirando (1-based).
	PaginaActual int
	// Aviso es la franja verde de «hecho»; Error, la roja. Los dos YA traducidos.
	Aviso, Error string
	// Copia es la ruta de la copia que dejó la última operación, para abrirla.
	Copia string
	// Trabajando es que hay una operación en vuelo.
	Trabajando bool
	// PanelAbierto es el cajón, que solo cuenta por debajo de [Estrecho].
	PanelAbierto bool

	// AlVolver es la salida de la cinta; AlAbrirRuta abre otro PDF de Drive (la copia);
	// AlReintentar vuelve a traer el fichero cuando ni siquiera llegó.
	AlVolver     func()
	AlAbrirRuta  func(ruta string)
	AlReintentar func()
	// AlEntrar abre el inicio de sesión de Sytue ID (sin sesión). Nil: no se ofrece.
	AlEntrar func()
	// AlCrear crea un PDF en blanco, AlElegir abre uno del disco y AlEscanear convierte
	// fotos en un PDF: la portada y el grupo «Archivo» de la cinta. Nil: no se ofrecen.
	AlCrear, AlElegir, AlEscanear func()
	// AlAbrirLocal abre un PDF de la máquina como «Abrir un archivo»: por aquí vuelve el
	// firmado de la aplicación de firma. Nil: no vuelve solo.
	AlAbrirLocal func(ruta string)

	// Portada es lo que se ve sin documento; la monta PonerPortada.
	Portada *Portada
	// dialogo es el diálogo de la cinta que se ve, si hay uno.
	dialogo *dialogoAbierto
	// peticion es la de firma que alguien me ha hecho sobre ESTE PDF y sigue
	// pendiente; sinEntregar, el firmado que no se llegó a entregar (pedirfirma.go).
	peticion    *api.PDFPeticionFirma
	sinEntregar string
	// bloqueando es la franja pidiendo confirmar «Bloquear» (ADR 0059).
	bloqueando bool
	// porEntregar es la petición del documento que se firmó FUERA (la aplicación
	// oficial): el firmado vuelve abriéndose como otro documento, y al abrirlo se entrega.
	porEntregar *api.PDFPeticionFirma
	// IA es el cliente del servicio de IA, el del Asistente. Nil: sus consultas dicen
	// que no se ha podido.
	IA *api.IA
	// Almacenes son los certificados del sistema con que se firma en dos pasos
	// (certificados.DelSistema). Nil: la firma con certificado es solo la de fichero.
	Almacenes func() []certificados.Almacen

	// Autor es el nombre de quien anota, que firma lo que pone (`autor` de la web).
	Autor string
	// Idioma es el de la interfaz, con el que se lee en voz alta.
	Idioma string
	// callar para la lectura en voz alta en curso; lectura cuenta las lecturas, para
	// que el final de una callada no dé por acabada la siguiente.
	callar  func()
	lectura int

	doc        *Documento
	claveUsada string
	textos     [][]Fragmento

	// Lo marcado sobre las hojas y todavía sin aplicar, y el tipo del próximo campo.
	zonas        []ZonaCensura
	camposNuevos []CampoMarcado
	tipoCampo    string
	// tramos son los `<text>` de cada página ya pedida, para seleccionar (`seleccion.go`).
	tramos         map[int][]TramoTexto
	pidiendoTramos map[int]bool
	// versionTramos cambia cada vez que se tiran: lo que llegue de antes no vale.
	versionTramos int
	// copiable es el texto seleccionado con la mano, lo que copia Ctrl+C.
	copiable string

	// Lo que se conserva entre recomposiciones.
	hojas     []*Hoja
	rejilla   *fyne.Container
	desplazar *container.Scroll
	// sombraCajon es el `shadow-xl` del cajón en estrecho.
	sombraCajon *ui.Sombra
	// animDesplazar es el salto de página animado en curso, si lo hay.
	animDesplazar *fyne.Animation
	imagenes      map[int]image.Image
	fallidas      map[int]bool
	pidiendo      map[int]bool
	escala        float64 // la escala a la que están pedidas las imágenes
	visor         *fyne.Container
	raiz          *fyne.Container
	// portal es donde se pintan las pistas de la cinta, encima de todo (ver ui.Pista).
	portal *fyne.Container
	// ancho es el de la última colocación: decide si una franja parte en dos líneas.
	ancho float32
	// ctx es el de las peticiones al motor. Es de la PANTALLA y no de quien abrió el
	// documento: la apertura acaba y el documento sigue pidiendo páginas.
	ctx context.Context
	// atajos son los Ctrl+tecla registrados, por su tecla (los prueba el guardián sin
	// pasar por el lienzo, que en el de pruebas no despacha atajos).
	atajos map[fyne.KeyName]func()
	// trasRed lo llama la pantalla al terminar de atender una respuesta del motor, ya en
	// el hilo de la interfaz. Es para las pruebas: esperar a él ordena la memoria.
	trasRed func()
}

// NuevaPantalla monta CertiPDF. `tv` traduce; no se guarda ningún texto a pelo.
func NuevaPantalla(tv TraductorVars) *Pantalla {
	t := func(clave string) string { return tv(clave, nil) }
	p := &Pantalla{t: t, tv: tv, Estado: editor.Cargando, Zoom: 1, PaginaActual: 1, ctx: context.Background(), tipoCampo: "texto"}
	p.barra = NuevaBarra(t)
	p.portal = container.NewWithoutLayout()
	p.barra.Portal = p.portal
	p.barra.AlCambiar = p.Recomponer
	p.barra.AlIrA = p.IrA
	p.barra.AlZoom = p.CambiarZoom
	p.barra.AlAjuste = p.PonerAjuste
	p.barra.AlDosPaginas = func(v bool) { p.DosPaginas = v; p.relayout() }
	p.barra.AlLeer = p.leer
	p.barra.AlVolver = func() {
		if p.AlVolver != nil {
			p.AlVolver()
		}
	}
	p.barra.AlPanel = func(v bool) { p.PanelAbierto = v; p.Recomponer() }
	p.barra.AlPulsarDialogo = p.abrirDialogo
	return p
}

// Barra es la cinta, para que quien monta la ventana pueda cablear sus diálogos.
func (p *Pantalla) Barra() *Barra { return p.barra }

// Objeto es lo que se mete en la ventana.
func (p *Pantalla) Objeto() fyne.CanvasObject {
	if p.raiz == nil {
		p.raiz = container.NewStack()
		p.recomponer()
	}
	return p.raiz
}

// Recomponer rehace la pantalla. Lo llama cualquier cosa que cambie el estado.
func (p *Pantalla) Recomponer() {
	if p.raiz == nil {
		return
	}
	p.recomponer()
	p.raiz.Refresh()
}

func (p *Pantalla) recomponer() {
	b := p.barra
	if c := p.Contenido; c != nil {
		b.Total = c.Total
		// `puedeAnotar` de `Barra.tsx`, literal.
		b.PuedeAnotar = c.Permisos.Anotar || !c.Permisos.SoloConClaveDeDueno
	}
	b.Pagina, b.Zoom, b.Ajuste, b.DosPaginas = p.PaginaActual, p.Zoom, p.Ajuste, p.DosPaginas
	b.Trabajando, b.PanelAbierto, b.Ancho = p.Trabajando, p.PanelAbierto, p.ancho
	b.AlNuevo, b.AlAbrir = p.AlCrear, p.AlElegir
	cuerpo := p.cuerpo()
	if p.enDocumento() {
		p.sincronizarHojas()
		objs := []fyne.CanvasObject{b.Objeto(), cuerpo}
		if p.enCajon() {
			objs = append(objs, p.cajon()...)
		}
		// La cinta se acaba de rehacer: una pista que estuviera abierta es de un botón
		// que ya no está y no recibirá nunca su MouseOut.
		p.portal.Objects = nil
		p.raiz.Objects = []fyne.CanvasObject{container.New(&marcoPantalla{p: p}, objs...), p.portal}
		return
	}
	// Sin documento no hay cinta: `PdfApp` devuelve el estado a pantalla completa.
	p.raiz.Objects = []fyne.CanvasObject{cuerpo}
}

// enDocumento es si hay un documento que enseñar, con su cinta.
func (p *Pantalla) enDocumento() bool {
	return !p.PideClave && p.Contenido != nil
}

// ClavesDelEstado son las claves de catálogo que pinta un estado: el título y la
// descripción, esta vacía si no la hay. Es la MISMA tabla que se pinta, así que el
// guardián no puede quedarse mirando una copia.
func ClavesDelEstado(e editor.Estado) (titulo, descripcion string) {
	switch e {
	case editor.Cargando:
		return "pdf.loading", ""
	case editor.Vacio:
		return "pdf.list.heading", "pdf.list.empty"
	case editor.Error:
		// `pdf.retry` es la salida, y la tiene que haber: casi todo lo que trae aquí
		// es pasajero —la red, el servicio—.
		return "pdf.error.open", "pdf.retry"
	case editor.SinConexion:
		// «Sin conexión» y «la sesión ha caducado» piden cosas OPUESTAS: esperar, o
		// volver a entrar. Por eso no comparten frase.
		return "pdf.error.offline", "pdf.retry"
	case editor.SinPermiso:
		// El 403: hay sesión, pero no para ESTE fichero. Volver a entrar no lo arregla.
		return "forbidden.title", "pdf.retry"
	case editor.SinSesion:
		// El 401: la web redirige al inicio de sesión; aquí se ofrece el botón.
		return "pdf.error.session", "account.signIn"
	case editor.Listo:
	}
	return "", ""
}

// cuerpo son los estados. Un estado que se pinta igual que otro es un estado que no
// existe para quien lo mira.
func (p *Pantalla) cuerpo() fyne.CanvasObject {
	if p.PideClave {
		return nuevoFormularioClave(p.t, p.ErrorClave, p.enviarClave)
	}
	if p.Contenido != nil {
		return p.documento()
	}
	titulo, _ := ClavesDelEstado(p.Estado)
	switch p.Estado {
	case editor.Cargando:
		return p.cargando()
	case editor.Vacio:
		if p.Portada != nil {
			return p.Portada.Objeto()
		}
		return ui.NuevoVacio(p.t("pdf.list.heading"), p.t("pdf.list.empty"))
	case editor.Error, editor.SinConexion, editor.SinPermiso, editor.SinSesion:
		frase := p.Error
		if frase == "" {
			frase = p.t(titulo)
		}
		return p.sinDocumento(frase)
	case editor.Listo:
	}
	return ui.NuevoVacio(p.t("pdf.list.heading"), p.t("pdf.list.empty"))
}

// cargando es `flex h-full items-center justify-center gap-3 text-sm text-text-muted`
// con la rueda de 16 delante del rótulo.
func (p *Pantalla) cargando() fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	return container.New(centradoEnTodo{},
		container.New(filaCentrada{hueco: tema.Espacio(3)},
			ui.NuevaRueda(16, "text-muted", v),
			ui.NuevoRotulo(p.t("pdf.loading"), tema.TextoSM, ui.Normal, "text-muted")))
}

// sinDocumento es el `!contenido` de `PdfApp.tsx`: la frase en `text-danger-text
// max-w-sm text-sm`, y debajo «Reintentar» (`bg-primary text-on-primary rounded-md px-3
// py-1.5 text-sm font-medium`) y la salida (`text-accent-text text-sm underline`), con
// `gap-3` entre todo. Era `bg-accent text-accent-text`: los dos son la tinta oscura de
// la casa (800 y 700), y el rótulo no se leía (1,3:1).
func (p *Pantalla) sinDocumento(frase string) fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	reintentar := NuevaAccion(p.t("pdf.retry"), p.Reintentar)
	if p.Estado == editor.SinSesion && p.AlEntrar != nil {
		// Sin sesión, reintentar da el mismo 401: lo que sirve es entrar, y al volver
		// se reintenta solo.
		reintentar = NuevaAccion(p.t("account.signIn"), p.AlEntrar)
	}
	reintentar.Escalon, reintentar.PX, reintentar.PY = tema.TextoSM, tema.Espacio(3), tema.Espacio(1.5)
	reintentar.Fondo, reintentar.FondoEncima, reintentar.Tinta = "primary", "primary-hover", "on-primary"
	reintentar.Inerte = p.Estado == editor.Cargando
	volver := "pdf.back"
	if p.barra.DesdeDrive {
		volver = "pdf.backToDrive"
	}
	salida := ui.NuevoEnlace(p.t(volver), tema.TextoSM, ui.Normal, "accent-text", p.barra.AlVolver)
	// `max-w-sm`: 24rem con raíz16, 384 px.
	texto := ui.NuevoParrafo(frase, tema.TextoSM, ui.Normal, "danger-text", v).A(24 * tema.Rem)
	return container.New(centradoEnTodo{relleno: tema.Espacio(6)},
		container.New(ui.Columna{Hueco: tema.Espacio(3), Encaja: true},
			texto, container.New(filaCentrada{hueco: tema.Espacio(3)}, reintentar, salida)))
}

// centradoEnTodo pone su único hijo centrado en el hueco, a su tamaño mínimo, con un
// relleno alrededor (`p-6`).
type centradoEnTodo struct{ relleno float32 }

func (c centradoEnTodo) MinSize(objs []fyne.CanvasObject) fyne.Size {
	m := objs[0].MinSize()
	return fyne.NewSize(m.Width+2*c.relleno, m.Height+2*c.relleno)
}

func (c centradoEnTodo) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	m := objs[0].MinSize()
	objs[0].Move(fyne.NewPos((t.Width-m.Width)/2, (t.Height-m.Height)/2))
	objs[0].Resize(m)
}

// documento es el visor: las franjas de aviso, y debajo el panel y las hojas.
func (p *Pantalla) documento() fyne.CanvasObject {
	if p.visor == nil {
		p.montarVisor()
	}
	partes := []fyne.CanvasObject{}
	for _, f := range []fyne.CanvasObject{p.franjaAviso(), p.franjaPeticion(), p.franjaCampos(), p.franjaCensura()} {
		if f != nil {
			partes = append(partes, f)
		}
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo := canvas.NewRectangle(tema.Color("bg-subtle", v))
	p.panel.PaginaActual = p.PaginaActual
	// Con el cajón abierto en estrecho, el panel está en el cajón: un objeto no puede
	// estar en dos sitios, así que en la columna queda un hueco (que ahí no se ve).
	var columna fyne.CanvasObject = canvas.NewRectangle(color.Transparent)
	if !p.enCajon() {
		columna = p.panel.Objeto()
	}
	// El desplazador va sin la sombra que Fyne pinta en el borde cuando queda contenido
	// por ver: la web no la tiene (medido en la lámina: una franja gris al pie).
	principal := container.New(&conPanel{p: p}, columna,
		container.NewStack(fondo, container.NewThemeOverride(p.desplazar, temaVisor{})))
	partes = append(partes, principal)
	return container.New(ui.Pila{Elastico: len(partes) - 1}, partes...)
}

// montarVisor construye lo que se conserva: las hojas, su rejilla y el panel.
func (p *Pantalla) montarVisor() {
	c := p.Contenido
	p.hojas = make([]*Hoja, 0, len(c.Paginas))
	for _, pag := range c.Paginas {
		h := NuevaHoja(pag, p.Zoom, HojaCargando, p.t("pdf.page.loading"), p.marcasDe(pag))
		h.PonerEnlaces(p.enlacesDe(pag))
		p.hojas = append(p.hojas, h)
	}
	objs := make([]fyne.CanvasObject, len(p.hojas))
	for i, h := range p.hojas {
		objs[i] = h
	}
	p.rejilla = container.New(&rejillaHojas{p: p}, objs...)
	p.desplazar = container.NewScroll(p.rejilla)
	p.desplazar.OnScrolled = func(fyne.Position) { p.alDesplazar() }
	p.visor = container.NewStack()
	// El panel se CONSERVA al volver a leer el documento tras un cambio: la pestaña
	// abierta, lo buscado y el filtro siguen donde estaban, como el componente montado
	// de la web.
	if p.panel != nil && p.panel.pant == p {
		p.panel.PonerContenido(c)
		p.panel.Imagen = p.imagenDe
		for n, img := range p.imagenes {
			p.ponerImagen(n, img)
		}
		return
	}
	p.panel = NuevoPanelLateral(p.tv, c)
	p.panel.pant = p
	p.panel.AlIrA = func(n int) {
		p.IrA(n)
		// En un móvil el panel tapa el documento: llevar a una página y dejarlo
		// abierto encima es enseñar la página que no se ve.
		if p.PanelAbierto {
			p.PanelAbierto = false
			p.Recomponer()
		}
	}
	p.panel.Imagen = p.imagenDe
	for n, img := range p.imagenes {
		p.ponerImagen(n, img)
	}
}

// imagenDe es el dibujo de la página `n` si ya llegó; es lo que pintan las miniaturas.
func (p *Pantalla) imagenDe(n int) image.Image { return p.imagenes[n] }

// PonerContenido deja listo el documento: el panel, las hojas y el primer lote. Si ya
// había uno —se acaba de aplicar un cambio—, el desplazamiento se conserva: en la web los
// nodos de las hojas siguen donde estaban y la vista no salta al principio.
func (p *Pantalla) PonerContenido(c *Contenido) {
	var antes fyne.Position
	habia := p.desplazar != nil && p.Contenido != nil
	if habia {
		antes = p.desplazar.Offset
	}
	p.Contenido = c
	p.Estado = editor.Listo
	p.visor, p.hojas = nil, nil
	// El texto de las páginas puede haber cambiado con el documento.
	p.olvidarTramos()
	p.imagenes, p.fallidas, p.pidiendo = map[int]image.Image{}, map[int]bool{}, map[int]bool{}
	p.escala = 0
	if p.PaginaActual < 1 || p.PaginaActual > c.Total {
		p.PaginaActual = 1
	}
	p.Recomponer()
	if habia {
		p.desplazar.ScrollToOffset(antes)
	}
	p.pedirLote(1)
	if p.PaginaActual > 1 {
		p.pedirLote(max(1, p.PaginaActual-1))
	}
}

// olvidarTramos tira el texto leído de las páginas: el documento ha cambiado.
func (p *Pantalla) olvidarTramos() {
	p.tramos, p.pidiendoTramos, p.copiable = nil, nil, ""
	p.versionTramos++
}

// ponerImagen entrega a la hoja `n` su dibujo, o dice que no se pudo.
func (p *Pantalla) ponerImagen(n int, img image.Image) {
	if p.imagenes == nil {
		p.imagenes = map[int]image.Image{}
	}
	if img != nil {
		p.imagenes[n] = img
	}
	for _, h := range p.hojas {
		if h.Numero() != n {
			continue
		}
		switch {
		case img != nil:
			h.PonerImagen(img, "", "")
		case p.fallidas[n]:
			h.PonerImagen(nil, p.t("pdf.page.failed"), "")
		}
	}
	if p.panel != nil {
		p.panel.RefrescarMiniatura(n)
	}
}

// marcasDe son las marcas que caen en esa página, en el orden de la capa de la web: lo
// marcado para censurar, los campos sin crear y las anotaciones. El motor numera las
// páginas de las anotaciones desde 0 y las páginas desde 1.
func (p *Pantalla) marcasDe(pag Pagina) []Marca {
	var out []Marca
	for _, z := range p.zonas {
		if z.Pagina == pag.Numero-1 {
			out = append(out, Marca{Rect: z.Rect, Clase: MarcaCensura})
		}
	}
	for _, c := range p.camposNuevos {
		if c.Pagina == pag.Numero-1 {
			out = append(out, Marca{Rect: c.Rect, Clase: MarcaCampo, Titulo: c.Nombre})
		}
	}
	for _, a := range p.Contenido.Anotaciones {
		if a.Pagina == pag.Numero-1 {
			titulo := a.Texto
			if titulo == "" {
				titulo = a.Tipo
			}
			out = append(out, Marca{Rect: a.Rect, Clase: MarcaAnotacion, Titulo: titulo})
		}
	}
	return out
}

// enlacesDe son las zonas pulsables de esa página: un salto interno lleva a su página y
// uno externo se abre fuera. El servidor YA filtra las direcciones (http, https, mailto).
func (p *Pantalla) enlacesDe(pag Pagina) []fyne.CanvasObject {
	var out []fyne.CanvasObject
	for _, e := range p.Contenido.Enlaces {
		if e.Pagina != pag.Numero-1 {
			continue
		}
		if e.URL != "" {
			dir := e.URL
			out = append(out, nuevoEnlaceHoja(e, p.tv("pdf.link.external", i18n.Vars{"url": dir}), func() {
				abrirFuera(dir)
			}))
			continue
		}
		destino := e.Destino + 1
		out = append(out, nuevoEnlaceHoja(e, p.tv("pdf.link.internal", i18n.Vars{"n": strconv.Itoa(destino)}), func() {
			p.IrA(destino)
		}))
	}
	return out
}

// conPanel coloca [panel, documento]: el panel a su ancho fijo a la izquierda y el
// documento en el resto. Por debajo de [Estrecho] el panel sale del flujo y, si está
// abierto, lo pinta el marco como cajón por encima de todo.
type conPanel struct{ p *Pantalla }

func (c *conPanel) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return objs[1].MinSize()
}

func (c *conPanel) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	if t.Width < Estrecho {
		objs[0].Hide()
		objs[1].Move(fyne.NewPos(0, 0))
		objs[1].Resize(t)
		c.p.alMedir(t)
		return
	}
	objs[0].Show()
	ancho := AnchoPanel
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(fyne.NewSize(ancho, t.Height))
	objs[1].Move(fyne.NewPos(ancho, 0))
	resto := fyne.NewSize(max(t.Width-ancho, 0), t.Height)
	objs[1].Resize(resto)
	c.p.alMedir(resto)
}

// marcoPantalla apila la cinta arriba, con su alto fijo, y le da el resto al cuerpo.
// Con el cajón abierto en estrecho, pinta encima el velo y el panel: `fixed inset-y-0
// left-0`, a todo el alto.
type marcoPantalla struct{ p *Pantalla }

var _ fyne.Layout = (*marcoPantalla)(nil)

func (m *marcoPantalla) MinSize(objs []fyne.CanvasObject) fyne.Size {
	c := objs[1].MinSize()
	return fyne.NewSize(max(objs[0].MinSize().Width, c.Width), m.p.barra.Alto()+c.Height)
}

func (m *marcoPantalla) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	p := m.p
	if p.ancho != t.Width {
		// El ancho decide si una franja parte en dos líneas —y eso cambia su alto— y si
		// el panel va en columna o en cajón: se recompone con el ancho nuevo, fuera de
		// esta pasada de colocación.
		cruza := (p.ancho < Estrecho) != (t.Width < Estrecho)
		franja := p.Error != "" || p.Aviso != "" || len(p.zonas) > 0 || len(p.camposNuevos) > 0
		// Por debajo de `md` la banda de la cinta encoge y envuelve sus grupos, y su alto
		// depende del ancho: también se recompone, incluida la PRIMERA vez que se sabe.
		estrecho := t.Width < md || (p.ancho > 0 && p.ancho < md)
		cambia := (p.ancho > 0 && (franja || (cruza && p.PanelAbierto))) || estrecho
		p.ancho = t.Width
		if cambia {
			fyne.Do(p.Recomponer)
		}
	}
	alto := p.barra.Alto()
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(fyne.NewSize(t.Width, alto))
	objs[1].Move(fyne.NewPos(0, alto))
	objs[1].Resize(fyne.NewSize(t.Width, max(t.Height-alto, 0)))
	if len(objs) < 5 {
		return
	}
	// El cajón: el velo a pantalla completa y el panel a todo el alto, a la izquierda.
	// Solo existe en estrecho; en ancho el panel es la columna de siempre.
	estrecho := t.Width < Estrecho
	cajon := fyne.NewSize(min(AnchoPanel, t.Width), t.Height)
	objs[2].Move(fyne.NewPos(0, 0))
	objs[2].Resize(t)
	if p.sombraCajon != nil {
		p.sombraCajon.Ajustar(fyne.NewPos(0, 0), cajon, tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	}
	objs[4].Move(fyne.NewPos(0, 0))
	objs[4].Resize(cajon)
	for _, o := range objs[2:] {
		esconder(o, !estrecho)
	}
}

// esconder es el `hidden` de la web: lo oculto no se pinta ni recibe el puntero.
func esconder(o fyne.CanvasObject, oculto bool) {
	if oculto {
		o.Hide()
		return
	}
	o.Show()
}

// enCajon es si el panel va en el cajón: abierto y en ventana estrecha.
func (p *Pantalla) enCajon() bool {
	return p.PanelAbierto && p.ancho > 0 && p.ancho < Estrecho
}

// cajon es el panel por encima de todo en estrecho: el velo (`bg-[var(--sytue-overlay)]`,
// tocar fuera cierra) y la columna con su cabecera `lg:hidden` —`border-b pr-1 pl-3`, el
// título en `text-xs font-medium` y el aspa de `h-11 w-11`—.
// Lleva el `max-lg:shadow-xl` de la web detrás: la parte que cae a la izquierda del
// borde de la ventana no se ve, igual que en el navegador.
func (p *Pantalla) cajon() []fyne.CanvasObject {
	cerrar := func() {
		p.PanelAbierto = false
		p.Recomponer()
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	aspa := ui.NuevoBotonIcono(iconos.Cerrar, p.t("pdf.close"), cerrar)
	aspa.Lado, aspa.TamIcono, aspa.Radio = tema.Espacio(11), 18, 0.25*tema.Rem
	aspa.Trazo, aspa.TrazoEncima, aspa.FondoEncima = "text-muted", "text", ""
	titulo := ui.NuevoRotulo(p.t("pdf.panel.title"), tema.TextoXS, ui.Medio, "text")
	cabecera := container.New(cabeceraCajon{}, canvas.NewRectangle(tema.Color("border", v)), titulo, aspa)
	fondo := canvas.NewRectangle(tema.Color("menu-surface", v))
	if p.sombraCajon == nil {
		p.sombraCajon = ui.NuevaSombra(tema.SombraXL, 0)
	}
	return []fyne.CanvasObject{
		ui.NuevoVelo(true, cerrar),
		p.sombraCajon.Objeto(),
		container.NewStack(fondo, container.New(ui.Pila{Elastico: 1}, cabecera, p.panel.Objeto())),
	}
}

// cabeceraCajon coloca [línea, título, aspa]: el título a `pl-3` y el aspa pegada al
// `pr-1`, los dos centrados en el alto del aspa, y la línea al pie.
type cabeceraCajon struct{}

func (cabeceraCajon) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(AnchoPanel, objs[2].MinSize().Height+tema.Borde)
}

func (cabeceraCajon) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	a := objs[2].MinSize()
	objs[0].Move(fyne.NewPos(0, t.Height-tema.Borde))
	objs[0].Resize(fyne.NewSize(t.Width, tema.Borde))
	m := objs[1].MinSize()
	objs[1].Move(fyne.NewPos(tema.Espacio(3), (a.Height-m.Height)/2))
	objs[1].Resize(m)
	objs[2].Move(fyne.NewPos(t.Width-tema.Espacio(1)-a.Width, 0))
	objs[2].Resize(a)
}

// confirmar es `confirmarEnEditor` con `tone: 'danger'` —las cuatro confirmaciones de
// CertiPDF destruyen algo—: el diálogo de confirmación de la casa con el mensaje
// en `text-sm leading-relaxed`, el rótulo del botón también de título y «Cancelar».
func (p *Pantalla) confirmar(mensaje, confirmar string, si func()) {
	p.confirmarCon(confirmar, mensaje, confirmar, si)
}

func (p *Pantalla) confirmarO(mensaje, confirmar string, si, no func()) {
	p.confirmarConO(confirmar, mensaje, confirmar, si, no)
}

// confirmarCon es confirmar con un título propio (el `titulo` de la web): «Quitar» a
// secas no decía qué se quitaba.
func (p *Pantalla) confirmarCon(titulo, mensaje, confirmar string, si func()) {
	p.confirmarConO(titulo, mensaje, confirmar, si, nil)
}

func (p *Pantalla) confirmarConO(titulo, mensaje, confirmar string, si, no func()) {
	c := fyne.CurrentApp().Driver().CanvasForObject(p.raiz)
	if c == nil {
		if no != nil {
			no()
		}
		return
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	cuerpo := ui.NuevoParrafo(mensaje, tema.Escalon{Tam: tema.TextoSM.Tam, Linea: tema.TextoSM.Tam * 1.625}, ui.Normal, "text", v)
	var d *ui.Dialogo
	d = ui.Confirmacion(titulo, p.t("ui.cancel"), confirmar, true, cuerpo,
		func() {
			d.Cerrar()
			if no != nil {
				no()
			}
		}, func() { d.Cerrar(); si() })
	d.Mostrar(c)
}
