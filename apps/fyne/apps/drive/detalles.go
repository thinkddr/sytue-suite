// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"context"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # Detalles
//
// `DetailsDialog.tsx` y `lib/detalles.ts`: el «Detalles» de Google Drive o el «Obtener
// información» del Finder. Qué es, dónde está, de quién es, cuándo y quién lo cambió, en
// qué versión va, cómo está clasificado y cuánta gente más entra. Las MISMAS filas y en
// el MISMO orden que la web, en el mismo panel lateral `md`.
//
// La web lo reúne en su BFF (`/api/details`); aquí no hay BFF y se piden las mismas
// cosas al servicio: el metadato (`GET /files/{id}` o `/folders/{id}`), las
// comparticiones (`/{id}/shares`) y las personas al directorio. Las reglas son las del
// BFF: un enlace caducado no cuenta, sin directorio se dice el correo o «Alguien» (nunca
// el `sub`) y si las comparticiones no se pueden leer se dice «No disponible», no un
// «0» que mentiría.
//
// La ubicación sale de la ruta si el servicio la da; si no, de subir de padre en padre
// (`RutaHasta`, como «Mostrar en su carpeta»), y si un peldaño falla es «No disponible».

// Acceso es quién más entra: enlaces vivos y personas y grupos distintos.
type Acceso struct{ Enlaces, Personas, Grupos int }

// Detalles es lo que el panel enseña de un elemento, ya reunido.
type Detalles struct {
	// Ubicacion son los nombres de las carpetas desde «Mi unidad» hasta la que lo
	// contiene (vacía: está en la raíz). Nil: no se pudo saber.
	Ubicacion []string
	Tipo      string // `content_type` (ficheros)
	Tamano    int64  // -1 en una carpeta
	// Propietario y ModificadoPor: nil es que el servicio no dijo quién; una Persona
	// vacía, que hay alguien pero el directorio no le puso cara.
	Propietario   *api.Persona
	Creado        time.Time
	Modificado    time.Time
	ModificadoPor *api.Persona
	Version       int // 0: no se sabe (o es una carpeta)
	Etiqueta      *api.DriveLabel
	// Acceso nil: el listado de comparticiones no se pudo leer.
	Acceso *Acceso
}

// FuentesDetalles son las peticiones que necesita ReunirDetalles. Son las de
// `api.Drive` y `api.Directorio`; se pasan sueltas para poder probarlas sin pantalla.
type FuentesDetalles struct {
	Fichero func(context.Context, string) (api.File, error)
	Carpeta func(context.Context, string) (api.Folder, error)
	Shares  func(context.Context, api.DriveKind, string) ([]api.DriveShare, error)
	// Persona resuelve un `sub`; nil = sin directorio.
	Persona func(context.Context, string) (api.Persona, error)
	// Raiz es el id de «Mi unidad», si ya se sabe.
	Raiz  string
	Ahora time.Time
}

// ReunirDetalles pide lo que el panel enseña de `el`. Solo falla si no se puede leer el
// propio elemento: lo demás (comparticiones, personas, ubicación) se queda en «No
// disponible» y el resto del panel sigue valiendo.
func ReunirDetalles(ctx context.Context, f FuentesDetalles, el Elemento) (Detalles, error) {
	d := Detalles{Tamano: -1}
	var propietario, modificador, ruta, padre string
	// Las comparticiones van a la vez que el metadato: son independientes, y el panel
	// espera a las dos.
	type resShares struct {
		lista []api.DriveShare
		err   error
	}
	canal := make(chan resShares, 1)
	go func() {
		l, err := f.Shares(ctx, claseDe(el), el.ID)
		canal <- resShares{l, err}
	}()
	if el.Carpeta {
		c, err := f.Carpeta(ctx, el.ID)
		if err != nil {
			<-canal
			return Detalles{}, err
		}
		propietario, ruta = c.OwnerSub, c.Path
		d.Creado, d.Modificado = c.CreatedAt, c.UpdatedAt
		if c.ParentID != nil {
			padre = *c.ParentID
		}
	} else {
		x, err := f.Fichero(ctx, el.ID)
		if err != nil {
			<-canal
			return Detalles{}, err
		}
		propietario, modificador, ruta, padre = x.OwnerSub, x.ModifiedBy, x.Path, x.FolderID
		d.Tipo, d.Tamano, d.Version, d.Etiqueta = x.ContentType, x.SizeBytes, x.Version, x.Label
		d.Creado, d.Modificado = x.CreatedAt, x.UpdatedAt
	}
	if r := <-canal; r.err == nil {
		a := ContarAcceso(r.lista, f.Ahora)
		d.Acceso = &a
	}

	// Cada persona se pregunta una vez: casi siempre quien lo creó es quien lo cambió.
	vistas := map[string]*api.Persona{}
	persona := func(sub string) *api.Persona {
		if sub == "" {
			return nil
		}
		if p, ok := vistas[sub]; ok {
			return p
		}
		p := &api.Persona{}
		if f.Persona != nil {
			if per, err := f.Persona(ctx, sub); err == nil {
				// Solo la cara: el `sub` no le dice nada a nadie.
				p = &api.Persona{Nombre: per.Nombre, Email: per.Email}
			}
		}
		vistas[sub] = p
		return p
	}
	d.Propietario, d.ModificadoPor = persona(propietario), persona(modificador)

	switch {
	case ruta != "":
		d.Ubicacion = CarpetasDeRuta(ruta)
	case padre != "" && f.Carpeta != nil:
		d.Ubicacion = carpetasHasta(ctx, f.Carpeta, padre, f.Raiz)
	}
	return d, nil
}

// carpetasHasta son los nombres de las carpetas desde debajo de «Mi unidad» hasta
// `padre`, incluida. Es `MigasDe`, pero sin su salida de emergencia: las migas se
// conforman con la última carpeta si no pueden subir, y aquí eso pintaría «Mi unidad ›
// 2026» de algo que vive en «Mi unidad › Contabilidad › 2026». Si cualquier peldaño
// falla, nil («No disponible»): mejor eso que una ruta a medias que parece entera.
func carpetasHasta(ctx context.Context, pedir func(context.Context, string) (api.Folder, error), padre, raiz string) []string {
	if padre == raiz {
		return []string{}
	}
	c, err := pedir(ctx, padre)
	if err != nil {
		return nil
	}
	if c.IsRoot || c.ParentID == nil {
		return []string{}
	}
	ruta, err := RutaHasta(ctx, pedir, DeCarpeta(c))
	if err != nil {
		return nil
	}
	nombres := make([]string, 0, len(ruta))
	for _, m := range ruta {
		nombres = append(nombres, m.Nombre)
	}
	return nombres
}

// ContarAcceso es `contarAcceso` de la web: los enlaces que siguen abriendo algo y las
// personas y los grupos distintos. Un enlace caducado no expone nada; contarlo haría
// creer que el fichero sigue expuesto.
func ContarAcceso(shares []api.DriveShare, ahora time.Time) Acceso {
	var a Acceso
	personas, grupos := map[string]bool{}, map[string]bool{}
	for _, s := range shares {
		switch {
		case s.Kind == "link":
			if s.ExpiresAt == nil || s.ExpiresAt.After(ahora) {
				a.Enlaces++
			}
		case s.Kind == "user" && s.Subject != "":
			personas[s.Subject] = true
		case s.Kind == "group" && s.Subject != "":
			grupos[s.Subject] = true
		}
	}
	a.Personas, a.Grupos = len(personas), len(grupos)
	return a
}

// CarpetasDeRuta son las carpetas que contienen `ruta` («/Proyectos/2026/acta.odt» →
// Proyectos, 2026): el `carpetaPadre` de la web, partido.
func CarpetasDeRuta(ruta string) []string {
	partes := strings.Split(strings.Trim(ruta, "/"), "/")
	out := []string{}
	for _, p := range partes[:max(len(partes)-1, 0)] {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// FilasDetalles son las filas del panel —rótulo y valor—, en el orden de
// `DetailsDialog.tsx`: tipo, tamaño, ubicación, propietario, creado, modificado,
// versión, clasificación y acceso. Una carpeta no tiene tamaño, versión ni etiqueta.
func FilasDetalles(tr *i18n.Translator, el Elemento, d Detalles) [][2]string {
	f := filasDetalles{tr: tr, vacio: tr.T("details.unavailable", nil)}
	f.fila("details.type", f.tipo(el, d))
	if !el.Carpeta {
		tam := d.Tamano
		if tam < 0 {
			tam = el.Tamano
		}
		f.filaSi("details.size", tam >= 0, func() string { return Tamano(tr.Locale(), tam) })
	}
	f.filaSi("details.location", d.Ubicacion != nil, func() string {
		return strings.Join(append([]string{f.t("nav.myDrive", nil)}, d.Ubicacion...), " › ")
	})
	f.fila("details.owner", f.nombre(d.Propietario))
	f.filaSi("details.created", !d.Creado.IsZero(), func() string { return f.fecha(d.Creado) })
	f.fila("details.modified", f.modificado(el, d))
	if !el.Carpeta {
		f.filaSi("details.version", d.Version > 0, func() string { return strconv.Itoa(d.Version) })
		f.fila("details.label", f.etiqueta(d.Etiqueta))
	}
	a := d.Acceso
	f.filaSi("details.links", a != nil, func() string { return strconv.Itoa(a.Enlaces) })
	f.filaSi("details.people", a != nil, func() string { return strconv.Itoa(a.Personas) })
	f.filaSi("details.groups", a != nil, func() string { return strconv.Itoa(a.Grupos) })
	return f.filas
}

// filasDetalles va juntando las filas de FilasDetalles.
type filasDetalles struct {
	tr    *i18n.Translator
	vacio string
	filas [][2]string
}

func (f *filasDetalles) t(k string, v i18n.Vars) string { return f.tr.T(k, v) }

func (f *filasDetalles) fila(k, v string) { f.filas = append(f.filas, [2]string{f.t(k, nil), v}) }

// filaSi pone el valor si se sabe y «No disponible» si no.
func (f *filasDetalles) filaSi(k string, sabido bool, valor func() string) {
	if sabido {
		f.fila(k, valor())
		return
	}
	f.fila(k, f.vacio)
}

func (f *filasDetalles) tipo(el Elemento, d Detalles) string {
	switch ext := Extension(el.Nombre); {
	case el.Carpeta:
		return f.t("details.folder", nil)
	case ext != "":
		return f.t("details.fileOfType", i18n.Vars{"ext": strings.ToUpper(ext)})
	case d.Tipo != "":
		return d.Tipo
	}
	return f.t("details.file", nil)
}

// nombre es el de la persona, si no su correo, si no «Alguien»: nunca el `sub`.
func (f *filasDetalles) nombre(p *api.Persona) string {
	switch {
	case p == nil:
		return f.vacio
	case p.Nombre != "":
		return p.Nombre
	case p.Email != "":
		return p.Email
	}
	return f.t("details.someone", nil)
}

func (f *filasDetalles) fecha(x time.Time) string { return fechaHora(f.tr.Locale(), x.Local()) }

// modificado es la fecha y, si se sabe, quién.
func (f *filasDetalles) modificado(el Elemento, d Detalles) string {
	mod := d.Modificado
	if mod.IsZero() {
		mod = el.Modificado
	}
	switch {
	case mod.IsZero():
		return f.vacio
	case d.ModificadoPor != nil:
		return f.t("details.modifiedBy", i18n.Vars{"date": f.fecha(mod), "name": f.nombre(d.ModificadoPor)})
	}
	return f.fecha(mod)
}

func (f *filasDetalles) etiqueta(e *api.DriveLabel) string {
	switch {
	case e == nil || e.Name == "":
		return f.t("details.noLabel", nil)
	case e.BlockExternalShare:
		return f.t("details.labelBlocks", i18n.Vars{"name": e.Name})
	}
	return e.Name
}

// ---- El panel ----

// dialogoDetalles es el estado del panel.
type dialogoDetalles struct {
	p  *Pantalla
	el Elemento
	d  *ui.Dialogo

	detalles *Detalles
	fallo    bool
	// intento numera cada carga: un «Reintentar» pulsado dos veces no debe pintar la
	// respuesta vieja encima de la nueva.
	intento int

	cuerpo *fyne.Container
	ancho  float32
}

// Medidas del panel (`DetailsDialog.tsx`).
const (
	// ClaveDetalles es el `minmax(8rem, auto)` de la columna de los rótulos.
	ClaveDetalles float32 = 8 * tema.Rem
	// FilasEsqueletoDetalles son las líneas del esqueleto, y AltoEsqueletoDetalles su
	// `height="0.875rem"`.
	FilasEsqueletoDetalles         = 8
	AltoEsqueletoDetalles  float32 = 0.875 * tema.Rem
)

// pedirDetalles abre el panel de `el` y lo devuelve (para el guardián).
func (p *Pantalla) pedirDetalles(el Elemento) *dialogoDetalles {
	if p.win == nil || p.svc == nil || el.ID == "" {
		return nil
	}
	dd := &dialogoDetalles{p: p, el: el, cuerpo: container.NewStack()}
	dd.d = ui.NuevoDialogo(p.tr.T("details.title", i18n.Vars{"name": el.Nombre}), dd.cuerpo).ALaDerecha()
	dd.d.Con(ui.DialogoMD, dd.pie())
	dd.d.RotuloCerrar = p.tr.T("ui.close", nil)
	dd.ancho = ui.AnchoCuerpoDialogo(dd.d, p.win.Canvas().Size())
	dd.pintar()
	dd.d.Mostrar(p.win.Canvas())
	dd.cargar()
	return dd
}

// cargar pide los detalles aparte y los pinta al llegar, si siguen siendo los últimos
// pedidos. Se llama en la rutina de la interfaz.
func (dd *dialogoDetalles) cargar() {
	dd.intento++
	n, raiz := dd.intento, dd.p.raizID
	p := dd.p
	f := FuentesDetalles{Fichero: p.svc.File, Carpeta: p.svc.Folder, Shares: p.svc.Shares, Raiz: raiz, Ahora: time.Now()}
	if p.Directorio != nil {
		f.Persona = p.Directorio.PorID
	}
	p.enSegundoPlano(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		d, err := ReunirDetalles(ctx, f, dd.el)
		enHilo(func() {
			if n != dd.intento {
				return
			}
			if err != nil {
				dd.fallo = true
			} else {
				dd.detalles = &d
			}
			dd.pintar()
		})
	})
}

func (dd *dialogoDetalles) reintentar() {
	dd.fallo, dd.detalles = false, nil
	dd.pintar()
	dd.cargar()
}

// pie es el de la web: los atajos a «Historial de versiones» (solo ficheros) y a
// «Compartir», que cierran el panel y abren el diálogo de siempre, y «Cerrar».
func (dd *dialogoDetalles) pie() fyne.CanvasObject {
	p, v := dd.p, dd.p.variante()
	t := func(k string) string { return p.tr.T(k, nil) }
	atajo := func(texto, glifo string, abrir func()) *ui.Boton {
		b := ui.NuevoBoton(texto, func() { dd.d.Cerrar(); abrir() }).Con(ui.Secundario, ui.TallaMD)
		// El secundario va en el rol primario (Button.tsx): el glifo, en su `currentColor`.
		b.Glifo, b.TamIcono = glifoCon(glifo, 16, tema.Color("on-primary", v)), 16
		return b
	}
	var botones []fyne.CanvasObject
	if !dd.el.Carpeta {
		botones = append(botones, atajo(t("details.versions"), glifoReloj, func() { p.pedirVersiones(dd.el) }))
	}
	botones = append(botones,
		atajo(t("details.share"), glifoEnlace, func() { p.pedirCompartir(dd.el) }),
		ui.NuevoBoton(t("details.close"), func() { dd.d.Cerrar() }).Con(ui.Primario, ui.TallaMD))
	return container.New(ui.PieAcciones{}, botones...)
}

// pintar rehace el cuerpo con el estado de ahora.
func (dd *dialogoDetalles) pintar() {
	v := dd.p.variante()
	var dentro fyne.CanvasObject
	switch {
	case dd.fallo:
		// `grid min-h-40 place-items-center`, el texto en `text-muted` y «Reintentar».
		texto := lineaTexto(ui.Texto(dd.p.tr.T("details.loadError", nil), tema.TextoBase, ui.Normal, tema.Color("text-muted", v)), tema.TextoBase)
		reintentar := ui.NuevoBoton(dd.p.tr.T("details.retry", nil), dd.reintentar).Con(ui.Secundario, ui.TallaSM)
		dentro = container.New(&altoFijo{alto: AltoCargaVersiones},
			container.NewCenter(container.New(ui.Columna{Hueco: tema.Espacio(2), Encaja: true}, texto, reintentar)))
	case dd.detalles == nil:
		dentro = esqueletoDetalles(dd.ancho)
	default:
		dentro = listaDetalles(FilasDetalles(dd.p.tr, dd.el, *dd.detalles), dd.ancho, v)
	}
	dd.cuerpo.Objects = []fyne.CanvasObject{dentro}
	dd.cuerpo.Refresh()
	dd.d.Recolocar()
}

// esqueletoDetalles son las ocho líneas de la espera: rótulo al 35 % y valor al 45 %,
// a `gap-4`, y las filas a `gap-3`.
//
// Las dieciséis rayas son UN grupo de carga: laten a una y dicen «Cargando» una vez.
func esqueletoDetalles(ancho float32) fyne.CanvasObject {
	b := ui.NuevosBloquesEsqueleto()
	filas := make([]fyne.CanvasObject, 0, FilasEsqueletoDetalles)
	for range FilasEsqueletoDetalles {
		filas = append(filas, container.New(ui.Fila{Hueco: tema.Espacio(4)},
			b.Bloque(0.35*ancho, AltoEsqueletoDetalles), b.Bloque(0.45*ancho, AltoEsqueletoDetalles)))
	}
	return b.Con(container.New(ui.Columna{Hueco: tema.Espacio(3)}, filas...))
}

// listaDetalles es la `dl`: `grid-cols-[minmax(8rem,auto)_1fr] gap-x-4`, filas en
// `py-2` con `divide-y divide-border`, todo en `text-sm`. La columna de los rótulos mide
// lo que el más largo y como poco 8rem; el valor parte en líneas lo que no le cabe.
func listaDetalles(filas [][2]string, ancho float32, v tema.Variante) fyne.CanvasObject {
	col := ClaveDetalles
	for _, f := range filas {
		col = max(col, ui.Medir(f[0], tema.TextoSM, ui.Normal).Width)
	}
	col = min(col, ancho/2)
	resto := max(ancho-col-tema.Espacio(4), 0)
	objs := make([]fyne.CanvasObject, 0, 2*len(filas))
	for i, f := range filas {
		if i > 0 {
			r := canvas.NewRectangle(tema.Color("border", v))
			r.SetMinSize(fyne.NewSize(0, tema.Borde))
			objs = append(objs, r)
		}
		rotulo := ui.NuevoParrafo(f[0], tema.TextoSM, ui.Normal, "text-muted", v).A(col)
		valor := ui.NuevoParrafo(f[1], tema.TextoSM, ui.Normal, "text", v).A(resto)
		objs = append(objs, container.New(ui.Margen{Arr: tema.Espacio(2), Abj: tema.Espacio(2)},
			container.New(filaDetalle{col: col}, rotulo, valor)))
	}
	return container.New(ui.Columna{}, objs...)
}

// filaDetalle coloca el rótulo en su columna y el valor detrás, a `gap-x-4`.
type filaDetalle struct{ col float32 }

func (f filaDetalle) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(f.col+tema.Espacio(4)+objs[1].MinSize().Width,
		max(objs[0].MinSize().Height, objs[1].MinSize().Height, tema.TextoSM.Linea))
}

func (f filaDetalle) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(fyne.NewSize(f.col, objs[0].MinSize().Height))
	x := f.col + tema.Espacio(4)
	objs[1].Move(fyne.NewPos(x, 0))
	objs[1].Resize(fyne.NewSize(max(t.Width-x, 0), objs[1].MinSize().Height))
}
