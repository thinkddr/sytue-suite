// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package contactos es la interfaz de Sytue Contacts en Fyne.
//
// paridad-web: apps/frontend/apps/contacts/src/pages/index.astro
//
// Contacts tiene UNA sola página en la web y es una SPA: la interfaz vive en los
// componentes (`ContactsApp.tsx`, `ContactDetail.tsx` y sus ocho modales), no en el
// `.astro`. Aquí pasa lo mismo: esta pantalla es el armazón de tres columnas y lo
// demás cuelga de ella.
//
// # De dónde salen los números
//
// De `test/e2e/medidas-web.mjs contacts` contra producción (corporate.sytue.eu,
// 1280×800, rem 14) y de las clases de `apps/frontend/apps/contacts/src/**`.
//
//	menú lateral   260, con 231 útiles                       medido
//	panel de lista 280, y 279 dentro (el `border-r` se come 1) medido
//	ficha          740, lo que sobra                          medido
//	fila           49 × 279 (19,25 + 15,75 + 2 × py-2)        medido
//	fila del menú  33,25, separadas 1,75                      medido
//
// El armazón de esos tres anchos es `ui.Armazon`, la misma pieza que monta Mail: los
// 260/280/740 están medidos allí y no se vuelven a declarar aquí.
package contactos

import (
	"image"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"

	"git.sytue.eu/sytue/one/apps/fyne/agenda"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Estado son los seis estados de pantalla del encargo.
type Estado = agenda.Estado

// AccionLote es lo que hace un botón de la barra de lote.
type AccionLote uint8

// Las cinco de `BatchBar`.
const (
	LoteDestacar AccionLote = iota
	LoteQuitarDestacado
	LoteAnadirEtiqueta
	LoteQuitarEtiqueta
	LoteAPapelera
)

// Pantalla es Sytue Contacts entero: menú, lista y ficha.
//
// No habla con la red: recibe los datos ya cargados y avisa por callbacks, que es lo
// que permite pintarla en un test sin servidor y sin CGO.
type Pantalla struct {
	T *i18n.Translator

	// Libretas son las del usuario y Activa la que se está viendo.
	Libretas []api.AddressBook
	Activa   *api.AddressBook
	// Etiquetas son los grupos de la libreta activa; EtiquetaActiva filtra por una.
	Etiquetas      []api.ContactGroup
	EtiquetaActiva string
	// Contactos son los de la libreta activa, YA filtrados por búsqueda y etiqueta.
	Contactos []api.Contact
	// Total es cuántos hay sin filtrar, para el «X de Y» del recuento.
	Total int
	// Seleccionado es el de la ficha de la derecha. nil: el estado vacío de la ficha.
	Seleccionado *api.Contact
	// Foto es la del contacto de la ficha, si la tiene y ha llegado.
	Foto image.Image
	// FotoDe es la foto de un contacto de la LISTA, o nil mientras no llega: la `<img>`
	// del avatar de cada fila en la web, que el navegador solo pide al verse la fila.
	FotoDe func(api.Contact) image.Image
	// Marcados son los id de los contactos con la casilla puesta, y Lote esos mismos
	// contactos de la libreta entera (el filtro puede esconder alguno).
	Marcados map[string]bool
	Lote     []api.Contact
	// Destacados son los UID ya limpios (`api.CleanUID`) de los favoritos.
	Destacados map[string]bool
	// Busqueda es lo escrito en el buscador de la lista.
	Busqueda string
	// SoloDestacados es el filtro de la vista «Destacados».
	SoloDestacados bool
	// CumpleanosPronto es el contador de la fila «Cumpleaños». 0: sin contador.
	CumpleanosPronto int

	// Estado son los seis del encargo, aplicados a la LISTA (que es lo que carga).
	Estado   Estado
	Refresco ui.RefrescoLista
	// CargandoLibretas y ErrorLibretas son los de la sección «Libretas» del menú, que
	// carga por su cuenta; CargandoEtiquetas y ErrorEtiquetas, los de «Etiquetas».
	CargandoLibretas, ErrorLibretas   bool
	CargandoEtiquetas, ErrorEtiquetas bool
	// SinConexion apaga las escrituras.
	SinConexion bool
	// DetalleAbierto es para ventana estrecha: en <768 la lista y la ficha se turnan.
	DetalleAbierto bool
	// CajonAbierto es el menú sacado como cajón en ventana estrecha.
	CajonAbierto bool
	// Ventana es el ancho de la VENTANA en px de CSS, que es lo que miden `sm:` y `lg:`.
	// Con 0 se asume el escritorio ancho.
	Ventana float32

	AlCrearContacto func()
	// AlVolver cierra la ficha en el teléfono y vuelve a la lista (`onBack`).
	AlVolver func()
	// AlRepintar vuelve a pintar la pantalla: lo pide quien cambia algo que no pasa
	// por otro callback, como cerrar el cajón antes de abrir un diálogo.
	AlRepintar       func()
	AlElegirContacto func(api.Contact)
	AlBuscar         func(string)
	AlElegirLibreta  func(api.AddressBook)
	AlCrearLibreta   func()
	AlElegirEtiqueta func(id string)
	AlCrearEtiqueta  func()
	AlVerDestacados  func()
	AlVerCumpleanos  func()
	AlVerPapelera    func()
	AlEditar         func(api.Contact)
	// AlEnviarCorreo abre Sytue Mail con ese destinatario.
	AlEnviarCorreo func(correo string)
	// Lo del menú «⋯» de la ficha: crear una reunión en Sytue Calendar, copiar el
	// correo, exportar la vCard y combinar con otro contacto.
	AlReunion      func(c api.Contact, correo string)
	AlCopiarCorreo func(correo string)
	AlExportar     func(api.Contact)
	AlCombinar     func(api.Contact)
	// AlAbrirEnlace abre un `mailto:` o un `tel:` con el sistema.
	AlAbrirEnlace func(destino string)
	// AlEtiquetar pone o quita una etiqueta a un contacto.
	AlEtiquetar     func(g api.ContactGroup, c api.Contact)
	AlDestacar      func(api.Contact)
	AlMarcar        func(id string)
	AlMarcarTodo    func()
	AlReintentar    func()
	AlLimpiarFiltro func()
	// AlReintentarLibretas y AlReintentarEtiquetas son los «Reintentar» del menú.
	AlReintentarLibretas  func()
	AlReintentarEtiquetas func()
	// Lo de la cabecera de la lista: las acciones de la etiqueta activa y de la
	// libreta, «Revisar duplicados» y el menú de importar y exportar.
	AlRenombrarEtiqueta, AlBorrarEtiqueta func()
	AlRenombrarLibreta, AlBorrarLibreta   func()
	AlRevisarDuplicados                   func()
	AlImportarVcard, AlImportarCSV        func()
	AlExportarVcard, AlExportarCSV        func()
	// La foto de la ficha (AvatarPicker): un color predefinido, subir una o quitarla.
	AlFotoColor  func(c api.Contact, token string)
	AlSubirFoto  func(c api.Contact)
	AlQuitarFoto func(c api.Contact)
	// AlLote es una acción de la barra de lote sobre los marcados; `g`, la etiqueta de
	// «Añadir a» y «Quitar de». AlSoltarMarcados vacía la selección (el aspa).
	AlLote           func(que AccionLote, sel []api.Contact, g *api.ContactGroup)
	AlSoltarMarcados func()
	// LoteOcupado apaga los botones de la barra mientras su petición va.
	LoteOcupado bool
	// Duplicados son los grupos de posibles duplicados de la libreta: con alguno sale
	// «Revisar duplicados (n)».
	Duplicados int
	// ImportOcupado es que hay una importación en curso, e Importando su avance
	// («Importando 3 de 20»), que se escribe en el propio botón (vacío con un CSV, que
	// va en una sola petición).
	ImportOcupado bool
	// PuedeImportar es que hay selector de ficheros; sin él, «Importar» sale apagado.
	PuedeImportar bool
	Importando    string

	// Importar es «Trae tus contactos» (`ui.ImportarDatos`), en el pie del menú; nil, el
	// pie va vacío.
	Importar *ui.ImportarDatos
	// AlServiciosVinculados abre libretas vivas de las cuentas externas.
	AlServiciosVinculados func()

	// Lo que NO se rehace al repintar: ver buscadorVivo y listaViva.
	buscador *ui.Entrada
	callando bool // el buscador se está poniendo al día desde fuera
	virtual  *widget.List
	cuerpo   fyne.CanvasObject
}

func (p *Pantalla) t(clave string, vars i18n.Vars) string {
	if p.T == nil {
		return clave
	}
	return p.T.T(clave, vars)
}

func (p *Pantalla) locale() i18n.Locale {
	if p.T == nil {
		return i18n.Default
	}
	return p.T.Locale()
}

// Construir arma el armazón de tres columnas y, en el teléfono, la barra móvil y el
// botón flotante de «Nuevo contacto».
func (p *Pantalla) Construir() fyne.CanvasObject {
	a := ui.NuevoArmazon(p.ficha())
	a.Menu = p.menu()
	a.Panel = p.lista()
	a.ClavePreferenciaPanel = "sytue.contacts.panel-width"
	a.DetalleAbierto = p.DetalleAbierto
	// El velo del armazón cierra el cajón por su cuenta: se entera por AlCajon.
	a.CajonAbierto = p.CajonAbierto
	a.AlCajon = func(abierto bool) { p.CajonAbierto = abierto }
	a.BarraMovil = p.barraMovil(a)
	if p.SinConexion {
		return a
	}
	// «Nuevo contacto al alcance del pulgar: en móvil el menú lateral es un cajón
	// cerrado» (MobileFab en ContactsApp.tsx).
	fab := ui.NuevoBotonFlotante(p.t("nav.newContact", nil), iconos.Mas, p.AlCrearContacto)
	capa := []fyne.CanvasObject{fab}
	if p.CajonAbierto {
		// En la web el velo del cajón (z 1020) va POR ENCIMA del botón (z 20): se ve
		// atenuado. Aquí el botón va encima del armazón, así que se le pone su trozo de
		// velo, del mismo color, y deja de recibir el toque.
		fab.Inerte = true
		v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
		velo := canvas.NewRectangle(ui.ColorVelo(v, 1))
		velo.CornerRadius = tema.Forma("radius-control")
		velo.SetMinSize(fab.MinSize())
		capa = append(capa, velo)
	}
	return container.NewStack(a, container.New(ui.CapaFlotante{}, capa...))
}

// barraMovil es la de AppShell: el menú (o «volver», con la ficha abierta) y, en la
// ficha, el nombre de la libreta.
func (p *Pantalla) barraMovil(a *ui.Armazon) *ui.BarraMovil {
	b := ui.NuevaBarraMovil()
	b.Titulo = p.t("list.title", nil)
	if p.Activa != nil {
		b.Titulo = nombreLibreta(*p.Activa)
	}
	b.EnDetalle, b.CajonAbierto = p.DetalleAbierto, p.CajonAbierto
	b.RotuloAbrir, b.RotuloCerrar, b.RotuloVolver = p.t("ui.openNav", nil), p.t("ui.closeNav", nil), p.t("ui.backToList", nil)
	b.AlMenu = func() {
		p.CajonAbierto = !p.CajonAbierto
		if p.AlRepintar != nil {
			p.AlRepintar() // también el botón flotante, que queda bajo el velo
			return
		}
		a.CajonAbierto, b.CajonAbierto = p.CajonAbierto, p.CajonAbierto
		a.Refresh()
		b.Refresh()
	}
	b.AlVolver = p.AlVolver
	return b
}

// enMenu es lo que hace AppShell con lo que se activa DENTRO del cajón: cerrarlo. En el
// teléfono el cajón tapa la pantalla, y pulsar «Nuevo contacto» abría el diálogo detrás
// de un menú que seguía ahí.
func (p *Pantalla) enMenu(f func()) func() {
	return func() {
		if p.CajonAbierto {
			p.CajonAbierto = false
			if p.AlRepintar != nil {
				p.AlRepintar()
			}
		}
		if f != nil {
			f()
		}
	}
}

// ---- El menú ----

// menu es el `<Sidebar>`: «Nuevo contacto» arriba y tres secciones — Vistas, Libretas
// y Etiquetas (esta última solo con libreta activa, como en la web).
func (p *Pantalla) menu() fyne.CanvasObject {
	crear := ui.NuevoBoton(p.t("nav.newContact", nil), p.enMenu(p.AlCrearContacto)).ConIcono(iconos.Mas, 0)
	crear.Ancho = ui.AnchoUtilMenu
	crear.Inerte = p.Activa == nil || p.SinConexion

	bloques := []fyne.CanvasObject{p.seccionVistas(), p.seccionLibretas()}
	if p.AlServiciosVinculados != nil {
		bloques = append(bloques, ui.NuevaFilaLateral(p.t("linkedServices.connected", nil), iconos.Trazo(iconos.Globo, ui.GlifoMenu, "sidebar-fg", tema.De(fyne.CurrentApp().Settings().ThemeVariant())).Objeto(), p.enMenu(p.AlServiciosVinculados)))
	}
	if p.Activa != nil {
		bloques = append(bloques, p.seccionEtiquetas())
	}
	// ConCabecera y no `l.Cabecera = crear`: es quien lo marca «sobre cromo», y sin eso
	// el botón primario sale tinta sobre la tinta del menú, sin contorno.
	l := ui.NuevoLateral(p.t("nav.views", nil), agenda.EnColumna(0, bloques...)).ConCabecera(crear)
	// El pie es el de `footer={<ImportData domains={['contacts']} …/>}`, «Trae tus
	// contactos»: solo sale si la organización ha conectado Google o Microsoft 365. Sin
	// él no hay pie: ni franja ni filete (en la web, `empty:hidden` en Sidebar.tsx).
	if p.Importar != nil {
		if o := p.Importar.Objeto(ui.AnchoUtilMenu, tema.De(fyne.CurrentApp().Settings().ThemeVariant())); o != nil {
			l.Pie = o
		}
	}
	return l
}

// seccionVistas es «VISTAS»: Destacados, Cumpleaños y Papelera.
func (p *Pantalla) seccionVistas() *ui.SeccionLateral {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	destacados := ui.NuevaFilaLateral(p.t("nav.starred", nil),
		p.estrella(p.SoloDestacados, v), p.enMenu(p.AlVerDestacados))
	destacados.Activa = p.SoloDestacados
	if n := p.contarDestacados(); n > 0 {
		destacados.Contador = entero(n)
	}
	cumples := ui.NuevaFilaLateral(p.t("nav.birthdays", nil),
		Glifo(TartaD, ui.GlifoMenu, "sidebar-fg-muted", v).Objeto(), p.enMenu(p.AlVerCumpleanos))
	if p.CumpleanosPronto > 0 {
		cumples.Contador = entero(p.CumpleanosPronto)
	}
	filas := []fyne.CanvasObject{destacados, cumples}
	// La papelera necesita red: sin conexión la web ni la pinta.
	if !p.SinConexion {
		filas = append(filas, ui.NuevaFilaLateral(p.t("nav.trash", nil),
			Glifo(PapeleraD, ui.GlifoMenu, "sidebar-fg-muted", v).Objeto(), p.enMenu(p.AlVerPapelera)))
	}
	s := ui.NuevaSeccionLateral(p.t("nav.views", nil), filas...)
	return s
}

// estrella es el `StarGlyph` de la fila «Destacados»: relleno cuando el filtro está
// activo y solo trazo cuando no.
func (p *Pantalla) estrella(relleno bool, v tema.Variante) fyne.CanvasObject {
	if relleno {
		return GlifoRelleno(EstrellaD, ui.GlifoMenu, tema.Color("sidebar-fg", v)).Objeto()
	}
	return Glifo(EstrellaD, ui.GlifoMenu, "sidebar-fg-muted", v).Objeto()
}

func (p *Pantalla) contarDestacados() int {
	n := 0
	for _, c := range p.Contactos {
		if p.Destacados[api.CleanUID(c.UID)] {
			n++
		}
	}
	return n
}

// seccionLibretas es «LIBRETAS», con el «Nueva libreta» al final.
func (p *Pantalla) seccionLibretas() *ui.SeccionLateral {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	s := ui.NuevaSeccionLateral(p.t("nav.addressBooks", nil))
	switch {
	case p.CargandoLibretas:
		s.Filas = []fyne.CanvasObject{ruedaDeMenu(v)}
		return s
	case p.ErrorLibretas:
		s.Filas = []fyne.CanvasObject{p.errorDeMenu("nav.booksError", p.enMenu(p.AlReintentarLibretas))}
		return s
	case len(p.Libretas) == 0:
		s.Filas = []fyne.CanvasObject{textoDeMenu(p.t("nav.booksEmpty", nil))}
	}
	for _, b := range p.Libretas {
		libro := b
		f := ui.NuevaFilaLateral(nombreLibreta(libro),
			iconos.Trazo(iconos.Usuario, ui.GlifoMenu, "sidebar-fg-muted", v).Objeto(),
			p.enMenu(func() {
				if p.AlElegirLibreta != nil {
					p.AlElegirLibreta(libro)
				}
			}))
		f.Activa = p.Activa != nil && p.Activa.ID == libro.ID
		s.Filas = append(s.Filas, f)
	}
	if !p.SinConexion {
		s.Filas = append(s.Filas, ui.NuevaFilaLateral(p.t("nav.newAddressBook", nil),
			iconos.Trazo(iconos.Mas, ui.GlifoMenu, "sidebar-fg-muted", v).Objeto(), p.enMenu(p.AlCrearLibreta)))
	}
	return s
}

// nombreLibreta es el nombre que se enseña, con el mismo criterio que el servicio.
func nombreLibreta(b api.AddressBook) string {
	if b.DisplayName != "" {
		return b.DisplayName
	}
	return b.Name
}

// seccionEtiquetas es «ETIQUETAS», con su contador de miembros por etiqueta.
func (p *Pantalla) seccionEtiquetas() *ui.SeccionLateral {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	s := ui.NuevaSeccionLateral(p.t("nav.labels", nil))
	switch {
	case p.CargandoEtiquetas:
		s.Filas = []fyne.CanvasObject{ruedaDeMenu(v)}
		return s
	case p.ErrorEtiquetas:
		s.Filas = []fyne.CanvasObject{p.errorDeMenu("nav.labelsError", p.enMenu(p.AlReintentarEtiquetas))}
		return s
	case len(p.Etiquetas) == 0:
		s.Filas = []fyne.CanvasObject{textoDeMenu(p.t("nav.labelsEmpty", nil))}
	}
	for _, g := range p.Etiquetas {
		grupo := g
		f := ui.NuevaFilaLateral(grupo.Name,
			Glifo(EtiquetaD, ui.GlifoMenu, "sidebar-fg-muted", v).Objeto(),
			p.enMenu(func() {
				if p.AlElegirEtiqueta != nil {
					p.AlElegirEtiqueta(grupo.ID)
				}
			}))
		f.Activa = p.EtiquetaActiva == grupo.ID
		f.Contador = entero(len(grupo.Members))
		s.Filas = append(s.Filas, f)
	}
	if !p.SinConexion {
		s.Filas = append(s.Filas, ui.NuevaFilaLateral(p.t("nav.newLabel", nil),
			iconos.Trazo(iconos.Mas, ui.GlifoMenu, "sidebar-fg-muted", v).Objeto(), p.enMenu(p.AlCrearEtiqueta)))
	}
	return s
}

// Las piezas sueltas de las secciones del menú, medidas en ContactsApp.tsx:
//
//	cargando `flex justify-center py-4` con un `<Spinner>` (18) en `sidebar-fg-muted`
//	vacío    `text-sidebar-fg-muted px-2.5 py-1 text-sm`
//	error    lo mismo, y debajo un «Reintentar» `mt-1 font-semibold` en `sidebar-fg`
const (
	// LadoRuedaMenu es el `size = 18` por defecto de Spinner.tsx.
	LadoRuedaMenu float32 = 18
	// RellenoRuedaMenu es su `py-4`.
	RellenoRuedaMenu = 4 * 0.25 * tema.Rem
)

// margenTextoMenu es el `px-2.5 py-1` del texto suelto de una sección.
var margenTextoMenu = ui.Margen{
	Izq: 2.5 * 0.25 * tema.Rem, Der: 2.5 * 0.25 * tema.Rem,
	Arr: 0.25 * tema.Rem, Abj: 0.25 * tema.Rem,
}

// ruedaDeMenu es el `<Spinner>` centrado de una sección que carga.
func ruedaDeMenu(v tema.Variante) fyne.CanvasObject {
	r := ui.NuevaRueda(LadoRuedaMenu, "sidebar-fg-muted", v)
	return conRelleno(ui.Margen{Arr: RellenoRuedaMenu, Abj: RellenoRuedaMenu}, container.NewCenter(r))
}

// textoDeMenu es un `<p>` suelto de una sección: «Todavía no hay etiquetas».
func textoDeMenu(s string) fyne.CanvasObject {
	return conRelleno(margenTextoMenu,
		ui.NuevoRotulo(s, tema.TextoSM, ui.Normal, "sidebar-fg-muted"))
}

// errorDeMenu es el aviso de una sección que no ha cargado, con su «Reintentar»: un
// `<button>` con pinta de enlace sobre el cromo oscuro —`text-sidebar-fg font-semibold`
// y el anillo de `chrome-focus`—, escrito a mano en ContactsApp.tsx.
func (p *Pantalla) errorDeMenu(clave string, reintentar func()) fyne.CanvasObject {
	otra := ui.NuevoEnlace(p.t("common.retry", nil), tema.TextoSM, ui.SemiNegrita, "sidebar-fg", reintentar)
	otra.TokenAro, otra.Boton = "sidebar-focus", true
	return conRelleno(margenTextoMenu, agenda.EnColumna(0.25*tema.Rem,
		ui.NuevoRotulo(p.t(clave, nil), tema.TextoSM, ui.Normal, "sidebar-fg-muted"), otra))
}

// entero escribe un contador pequeño sin traer `strconv` a cada fila.
func entero(n int) string {
	if n <= 0 {
		return ""
	}
	d := make([]byte, 0, 4)
	for n > 0 {
		d = append([]byte{byte('0' + n%10)}, d...)
		n /= 10
	}
	return string(d)
}

// ---- Layouts de esta pantalla ----

// franjas apila una cabecera de alto fijo, un bloque intermedio y un cuerpo que se
// lleva lo que sobra: el `flex flex-col` con un `flex-1 min-h-0` al final.
type franjas struct{ Altos []float32 }

var _ fyne.Layout = (*franjas)(nil)

func (f *franjas) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var alto, ancho float32
	for i, o := range objs {
		m := o.MinSize()
		ancho = max(ancho, m.Width)
		if i < len(f.Altos) {
			alto += f.Altos[i]
			continue
		}
		alto += m.Height
	}
	return fyne.NewSize(ancho, alto)
}

func (f *franjas) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	var y float32
	fijo := float32(0)
	for i := range objs {
		if i < len(f.Altos) {
			fijo += f.Altos[i]
		}
	}
	for i, o := range objs {
		h := max(t.Height-fijo, 0)
		if i < len(f.Altos) {
			h = f.Altos[i]
		}
		o.Move(fyne.NewPos(0, y))
		o.Resize(fyne.NewSize(t.Width, h))
		y += h
	}
}

// conRelleno es un `p-*` alrededor de un hijo que se estira: `ui.Margen`.
func conRelleno(m ui.Margen, hijo fyne.CanvasObject) *fyne.Container {
	return container.New(m, hijo)
}
