// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"image/color"
	"slices"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// AnchoBuscador es el `sm:w-64` del buscador por nombre.
const AnchoBuscador = 64 * 0.25 * tema.Rem

// nuevoBuscador es el buscador de la galería: el `Input` de la casa en su talla `md`,
// con la lupa delante (`leadingIcon`). Era un campo `sm` sin icono, 28 px al lado de
// botones de 32.
func nuevoBuscador(v Vista, texto string) *ui.Entrada {
	e := ui.NuevaEntrada(v.Traduce("gallery.search"))
	e.Icono = iconos.Buscar
	e.PonerTexto(texto)
	return e
}

// ---- Datos de las pantallas ----

// GrupoMes es un mes de la línea de tiempo (`porMes()` de lib/agrupar.ts): su rótulo,
// sus fotos y cuántas van por fecha de SUBIDA y no por EXIF.
type GrupoMes struct {
	Titulo      string
	Fotos       []api.Photo
	Aproximadas int
}

// PorMes agrupa las fotos por mes, de la más reciente a la más antigua, como
// `porMes()`. El rótulo lo formatea el idioma.
func PorMes(fotos []api.Photo, t *i18n.Translator) []GrupoMes {
	var grupos []GrupoMes
	clave := ""
	for _, f := range fotos {
		k := f.TakenAt.Format("2006-01")
		if k != clave {
			clave = k
			grupos = append(grupos, GrupoMes{Titulo: i18n.FormatMonthYear(t.Locale(), f.TakenAt)})
		}
		g := &grupos[len(grupos)-1]
		g.Fotos = append(g.Fotos, f)
		if !f.TakenFromEXIF {
			g.Aproximadas++
		}
	}
	return grupos
}

// Lote son los mandos del modo selección (el `Toolbar` pegajoso de la galería).
type Lote struct {
	// Albumes son los del selector «Añadir a un álbum…»; nil, todavía sin pedir.
	Albumes []api.Album
	// Describiendo es el «Describiendo 3 de 12…» mientras corre el lote de Sytue AI;
	// vacío, no corre.
	Describiendo string

	AlNuevoAlbum, AlDescribir, AlParar, AlParecidas, AlDescargar, AlTirar func()
	AlAnyadirA                                                            func(api.Album)
}

// Galeria son los datos de la línea de tiempo y de «Favoritas», que en la web son LA
// MISMA vista con un filtro y no dos pantallas.
type Galeria struct {
	Grupos []GrupoMes
	// Arrastrando es que hay ficheros arrastrándose por encima de la ventana: el aviso
	// pasa a «Suelta para añadir» (`arrastrando` en la web).
	Arrastrando bool
	// Buscador es el campo de búsqueda. Vive FUERA de la pantalla porque la pantalla se
	// rehace al llegar cada página y lo escrito no se puede perder a media palabra.
	// nil: se crea uno con Busqueda dentro (lo que hacen los guardianes).
	Buscador *ui.Entrada
	// Busqueda es lo escrito en el buscador por nombre.
	Busqueda string
	// SoloFavoritas es el filtro: cambia la pestaña activa y los textos del vacío.
	SoloFavoritas bool
	// EnSeleccion es el modo selección; Seleccion, lo marcado.
	EnSeleccion bool
	Seleccion   map[string]bool
	Lote        Lote
	// Aviso es el `role="status"` de la subida y de las acciones en lote, y Error el
	// `role="alert"` que la web pone encima de la cuadrícula.
	Aviso, Error string
	// Recuerdos es la sección «Tal día como hoy», ya montada; nil, no hay.
	Recuerdos fyne.CanvasObject
	// Cargando pinta la rueda del centinela del final: se está pidiendo una página.
	Cargando bool
	// Subiendo inertiza «Añadir fotos» mientras dura una subida.
	Subiendo bool
	// Ampliar ofrece «Buscar también con otras palabras»: solo cuando la búsqueda literal
	// no encontró NADA, y Ampliando lo inertiza mientras Sytue AI contesta.
	Ampliar, Ampliando bool
	AlAmpliar          func()

	AlAbrir     func(api.Photo)
	AlAlternar  func(api.Photo)
	AlFavorita  func(api.Photo)
	AlIr        func(Seccion)
	AlAnyadir   func()
	AlComoEstas func()
	AlBuscar    func(string)
	// AlSeleccionar entra en el modo selección y AlSalir sale.
	AlSeleccionar, AlSalir func()
	AlAlternarMes          func([]api.Photo)
	// AlMes y AlBarra reciben la cabecera de cada mes (con su rejilla) y la barra de
	// selección tal y como quedan en la pantalla: la app las necesita para pegarlas
	// arriba al desplazarse (`sticky`).
	AlMes   func(cabecera, rejilla fyne.CanvasObject, g GrupoMes)
	AlBarra func(barra fyne.CanvasObject)
}

// AltoCentinela es el `h-16` del `<div ref={centinela}>` del final, donde gira la rueda
// mientras llega la página siguiente.
const AltoCentinela = 16 * 0.25 * tema.Rem

// PantallaGaleria monta la línea de tiempo (o «Favoritas»).
func PantallaGaleria(v Vista, d Galeria) *fyne.Container {
	activa := SecFotos
	if d.SoloFavoritas {
		activa = SecFavoritas
	}
	hayFotos := len(d.Grupos) > 0

	// La fila de arriba: la nav, el buscador empujado a la derecha y los botones.
	buscador := d.Buscador
	if buscador == nil {
		buscador = nuevoBuscador(v, d.Busqueda)
		buscador.AlCambiar(d.AlBuscar)
	}
	// `min-w-0 flex-1 sm:w-56 sm:flex-none`: en un teléfono el buscador llena su fila.
	campo := anchoFijo(buscador, AnchoBuscador)
	elastico := 0
	if v.Ventana < ui.CorteSM {
		campo, elastico = buscador, 2
	}
	// UNA acción rellena en la barra —«Añadir fotos»— y las demás de fantasma con su
	// icono: «Seleccionar» era otro botón relleno del mismo peso.
	derecha := []fyne.CanvasObject{campo}
	if d.Busqueda != "" && hayFotos && !d.EnSeleccion {
		derecha = append(derecha,
			ui.NuevoBoton(v.Traduce("gallery.likeTheseResults"), d.AlComoEstas).Con(ui.Fantasma, ui.TallaMD).ConIcono(iconos.Imagen, 0))
	}
	if hayFotos && !d.EnSeleccion {
		derecha = append(derecha,
			ui.NuevoBoton(v.Traduce("gallery.select"), d.AlSeleccionar).Con(ui.Fantasma, ui.TallaMD).ConIcono(iconos.Tic, 0))
	}
	derecha = append(derecha, botonAnyadir(v, d, false))
	arriba := append([]fyne.CanvasObject{Nav(v, activa, d.AlIr)}, derecha...)
	// `flex flex-wrap gap-2` con `mb-4` en cada hijo: entre fila y fila, 14 + 7.
	fila := container.New(ui.FilaAuto{
		Hueco: tema.Espacio(2), Derecha: 1, Partir: true,
		SaltoFila: SeparacionBloque + tema.Espacio(2), Elastico: elastico,
	}, arriba...)

	bloques := []ui.Bloque{{Obj: fila, Margen: SeparacionBloque}}
	if d.EnSeleccion {
		barra := barraSeleccion(v, d)
		if d.AlBarra != nil {
			d.AlBarra(barra)
		}
		bloques = append(bloques, ui.Bloque{Obj: barra, Margen: SeparacionBloque})
	}
	if e := avisoGaleria(v, d); e != nil {
		bloques = append(bloques, ui.Bloque{Obj: e, Margen: SeparacionBloque})
	}

	vacioTitulo, vacioTexto := "gallery.empty.title", "gallery.empty.description"
	if d.SoloFavoritas {
		vacioTitulo, vacioTexto = "favorites.empty.title", "favorites.empty.description"
	}
	vacio := EstadoVacio(v, vacioTitulo, vacioTexto, botonAnyadir(v, d, true))
	if v.Estado == Cargando {
		// La web no enseña un estado de carga aparte: pinta la cabecera y el centinela
		// del final, que es donde gira la rueda también con la galería vacía.
		bloques = append(bloques, ui.Bloque{Obj: centinela(v, true)})
		return Envolver(v, true, ui.PilaBloques(bloques...))
	}
	if cuerpo := CuerpoDeEstado(v, vacio); cuerpo != nil {
		bloques = append(bloques, ui.Bloque{Obj: cuerpo})
		return Envolver(v, true, ui.PilaBloques(bloques...))
	}
	if d.Error != "" {
		bloques = append(bloques, ui.Bloque{
			Obj: ParrafoEn(d.Error, tema.TextoSM, ui.Normal, "danger-text", v.Variante), Margen: SeparacionBloque,
		})
	}
	if d.Recuerdos != nil {
		bloques = append(bloques, ui.Bloque{Obj: d.Recuerdos, Margen: SeparacionSeccion})
	}
	if d.Ampliar {
		// `border-border mb-6 rounded-md border p-4`, la pista `mb-2 text-sm` y el botón.
		b := ui.NuevoBoton(v.Traduce("gallery.searchWider"), d.AlAmpliar).Con(ui.Secundario, ui.TallaSM).ConIcono(iconos.Buscar, 0)
		b.Inerte = d.Ampliando
		caja := recuadro(v, "", "border", tema.RadioMD,
			ui.Margen{Izq: tema.Espacio(4), Der: tema.Espacio(4), Arr: tema.Espacio(4), Abj: tema.Espacio(4)},
			ui.PilaBloques(bloqueLargo(ui.NuevoParrafo(v.Traduce("gallery.searchWiderHint"), tema.TextoSM, ui.Normal, "text-muted", v.Variante), tema.Espacio(2)),
				ui.Bloque{Obj: container.New(ui.FilaAuto{Hueco: 0, Derecha: 1}, b)}))
		bloques = append(bloques, ui.Bloque{Obj: caja, Margen: SeparacionSeccion})
	}

	for _, g := range d.Grupos {
		cab, rej := cabeceraMes(v, g, d), rejillaDeFotos(v, g.Fotos, d)
		if d.AlMes != nil {
			d.AlMes(cab, rej, g)
		}
		bloques = append(bloques, ui.Bloque{Obj: cab}, ui.Bloque{Obj: rej, Margen: SeparacionMes})
	}
	bloques = append(bloques, ui.Bloque{Obj: centinela(v, d.Cargando)})
	return Envolver(v, true, ui.PilaBloques(bloques...))
}

// botonAnyadir es «Añadir fotos», LA acción principal de la galería: `primary` (talla
// `md`, la del buscador de al lado) con el `plus` delante. En la barra, el rótulo solo
// desde `sm:` («en móvil solo el icono: el rótulo mandaba el botón a una tercera
// fila»); en la galería VACÍA va siempre, que ahí es lo único que se puede hacer.
func botonAnyadir(v Vista, d Galeria, siempreConRotulo bool) *ui.Boton {
	b := ui.NuevoBoton(v.Traduce("gallery.add"), d.AlAnyadir).Con(ui.Primario, ui.TallaMD).ConIcono(iconos.Mas, 0)
	b.Inerte = d.Subiendo
	if !siempreConRotulo && v.Ventana < ui.CorteSM {
		b.Rotulo, b.Texto = b.Texto, ""
	}
	return b
}

// estadoSubida es el `<p role="status" class="mb-4 rounded-md px-3 py-2 text-sm
// text-text-muted">` de la subida y de las acciones en lote. El texto es un `ui.Rotulo`,
// que ocupa su caja de línea de CSS: medido en la web, el `<p>` mide 29,8 de alto.
func estadoSubida(v Vista, texto string) fyne.CanvasObject {
	return recuadro(v, "", "", tema.RadioMD, margenEstado, ui.NuevoRotulo(texto, tema.TextoSM, ui.Normal, "text-muted"))
}

// avisoGaleria es el `{estadoSubida}` de la galería, o nil si no hay nada que decir:
// «Suelta para añadir» manda sobre el progreso y el aviso, como en la web.
func avisoGaleria(v Vista, d Galeria) fyne.CanvasObject {
	switch {
	case d.Arrastrando:
		return estadoSoltar(v)
	case d.Aviso != "":
		return estadoSubida(v, d.Aviso)
	}
	return nil
}

// margenEstado es el `px-3 py-2` del `<p>` del estado.
var margenEstado = ui.Margen{Izq: tema.Espacio(3), Der: tema.Espacio(3), Arr: tema.Espacio(2), Abj: tema.Espacio(2)}

// estadoSoltar es el mismo `<p>` mientras se arrastran ficheros por encima de la ventana:
// `border border-dashed border-primary text-primary-text text-center` con
// `gallery.dropHere`. El borde es de verdad (`box-sizing: border-box`), así que el `<p>`
// crece dos píxeles: medido en la web, 31,8 de alto y el texto centrado.
func estadoSoltar(v Vista) fyne.CanvasObject {
	m := margenEstado
	m.Izq, m.Der, m.Arr, m.Abj = m.Izq+tema.Borde, m.Der+tema.Borde, m.Arr+tema.Borde, m.Abj+tema.Borde
	texto := ui.NuevoRotulo(v.Traduce("gallery.dropHere"), tema.TextoSM, ui.Normal, "primary-text")
	return container.NewStack(ui.NuevoBordeDiscontinuo(tema.Borde, tema.RadioMD, "primary"),
		container.New(m, container.New(centradoH{}, texto)))
}

// centinela es el `<div class="flex h-16 items-center justify-center">` del final, con
// la rueda mientras se pide la página siguiente.
func centinela(v Vista, cargando bool) fyne.CanvasObject {
	if !cargando {
		return container.New(fijo{alto: AltoCentinela})
	}
	r := ui.NuevaRueda(ui.LadoRueda, "text", v.Variante)
	r.Rotulo = v.Traduce("ui.loading")
	return container.New(fijo{alto: AltoCentinela}, container.NewCenter(r))
}

// cabeceraMes es el `<h2>` pegajoso del mes: `bg-bg sytue-overline flex
// items-center gap-3 py-2`, el rótulo en versalitas, cuántas fotos van por fecha de
// subida si las hay y, seleccionando, «Seleccionar el mes».
//
// Sus hijos ENCOGEN (FilaEncoge): en un teléfono los tres se parten en dos líneas en
// vez de empujar el botón fuera de la pantalla, que es lo que hace la web.
//
// Es pegajosa (`sticky top-0`, bajo la barra de selección cuando está): la pega la app,
// que es quien sabe cuánto se ha desplazado (ver pegar en app_pegar.go).
func cabeceraMes(v Vista, g GrupoMes, d Galeria) fyne.CanvasObject {
	titulo := NuevoVersalRepartido(g.Titulo, tema.TextoXS, ui.SemiNegrita, tema.Color("text-muted", v.Variante), ui.TrackingOverline)
	hijos, piezas := []fyne.CanvasObject{titulo}, []PiezaFlex{titulo.Pieza()}
	if g.Aproximadas > 0 {
		p := ui.NuevoParrafo(v.T.T("gallery.approxCount", i18n.Vars{"count": g.Aproximadas}),
			tema.TextoXS, ui.Normal, "text-subtle", v.Variante)
		hijos, piezas = append(hijos, p), append(piezas, PiezaParrafo(p))
	}
	if d.EnSeleccion {
		todas := true
		for _, f := range g.Fotos {
			if !d.Seleccion[f.ID] {
				todas = false
				break
			}
		}
		clave := "gallery.selectMonth"
		if todas {
			clave = "gallery.unselectMonth"
		}
		fotos := g.Fotos
		b := NuevoBotonMes(v.Traduce(clave), todas, func() {
			if d.AlAlternarMes != nil {
				d.AlAlternarMes(fotos)
			}
		})
		hijos, piezas = append(hijos, b), append(piezas, b.Pieza())
	}
	fila := container.New(FilaEncoge{Hueco: tema.Espacio(3), RellenoY: tema.Espacio(2), Piezas: piezas}, hijos...)
	return container.New(conFondo{}, canvas.NewRectangle(tema.Color(fondoCabeceraMes, v.Variante)), fila)
}

// fondoCabeceraMes es el `bg-bg` del `<h2>`: el color de la página, opaco para tapar
// las fotos cuando va pegada. Era `bg-surface`, una banda blanca sobre el fondo.
const fondoCabeceraMes = "bg"

// conFondo pone el primer hijo (un rectángulo) de fondo a todo el ancho y el segundo
// encima: el `bg-surface` del encabezado del mes, que en la web ocupa la sección entera.
type conFondo struct{}

func (conFondo) MinSize(objs []fyne.CanvasObject) fyne.Size { return objs[1].MinSize() }

// AltoCon es el del contenido a ese ancho: la cabecera del mes crece al partirse.
func (conFondo) AltoCon(objs []fyne.CanvasObject, ancho float32) float32 {
	return ui.AltoA(objs[1], ancho)
}

func (conFondo) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	for _, o := range objs {
		o.Move(fyne.NewPos(0, 0))
		o.Resize(t)
	}
}

// barraSeleccion es el `Toolbar` del modo selección: `bg-surface-raised border rounded-md
// px-2 py-1.5 shadow-sm gap-1`. Una barra de CONTEXTO y no una fila de botones: cómo se
// sale y cuántas van, a la izquierda; lo que se puede hacer con ellas, de fantasma y con
// su icono; y, tras un filete y en el extremo, lo que destruye.
//
// `surface-raised` y no `surface`: va pegada encima de las fotos, y `surface` es
// translúcida en el tema traslúcido.
//
// También es pegajosa (`sticky top-0 z-20`): ver pegar en app_pegar.go.
func barraSeleccion(v Vista, d Galeria) fyne.CanvasObject {
	n := len(d.Seleccion)
	l := d.Lote
	sinNada := n == 0
	// `max-xl:sr-only`: por debajo de `xl` los cuatro rótulos no caben en una fila en los
	// idiomas largos y queda el icono, con el rótulo de nombre accesible.
	conRotulo := v.Ventana >= ui.CorteXL
	boton := func(clave string, ic iconos.Icono, al func()) *ui.Boton {
		b := ui.NuevoBoton(v.Traduce(clave), al).Con(ui.Fantasma, ui.TallaSM).ConIcono(ic, 0)
		if !conRotulo {
			b.Rotulo, b.Texto = b.Texto, ""
		}
		b.Inerte = sinNada
		return b
	}
	salir := ui.NuevoBotonIcono(iconos.Cerrar, v.Traduce("gallery.cancelSelection"), d.AlSalir)
	salir.Lado, salir.TamIcono = ui.LadoIconoLG, IconoMando

	// «Álbum nuevo» y «Añadir a un álbum…» eran dos mandos —el segundo, un selector—
	// para la misma pregunta: ahora es UN menú con el nuevo arriba.
	var album *ui.Boton
	album = boton("albums.addTo", iconos.Album, func() {
		elems := []fyne.CanvasObject{ui.NuevoElementoMenu(v.Traduce("gallery.newAlbum"), iconos.Mas, l.AlNuevoAlbum)}
		if len(l.Albumes) > 0 {
			elems = append(elems, ui.NuevoSeparadorMenu())
		}
		for _, a := range l.Albumes {
			elems = append(elems, ui.NuevoElementoMenu(a.Rotulo(), iconos.Ninguno, func() {
				if l.AlAnyadirA != nil {
					l.AlAnyadirA(a)
				}
			}))
		}
		mostrarMenu(v.Traduce("albums.addTo"), album, elems)
	})

	describir := boton("gallery.describeSelected", iconos.Texto, l.AlDescribir)
	hijos := []fyne.CanvasObject{
		salir,
		ParrafoEn(v.T.T("gallery.selectedCount", i18n.Vars{"count": n}), tema.TextoSM, ui.Medio, "text", v.Variante),
		album,
		describir,
	}
	if l.Describiendo != "" {
		// El progreso se lee siempre, también donde los rótulos se esconden.
		describir.Texto, describir.Rotulo, describir.Inerte = l.Describiendo, "", true
		// Parar tiene que estar a mano MIENTRAS corre: cada foto es una llamada que se
		// cobra, y quien se arrepiente a la décima no debería tener que cerrar la app.
		hijos = append(hijos, ui.NuevoBoton(v.Traduce("gallery.describeStop"), l.AlParar).Con(ui.Fantasma, ui.TallaSM))
	}
	tirar := ui.NuevoBotonIcono(iconos.Papelera, v.Traduce("viewer.delete"), l.AlTirar)
	tirar.Lado, tirar.TamIcono, tirar.Inerte = ui.LadoIconoLG, IconoMando, sinNada
	tirar.FondoEncima, tirar.TrazoEncima = "bg-subtle", "danger-text" // `tone="danger"`
	hijos = append(hijos,
		// El mismo icono que «Fotos parecidas» en el visor: es la misma pregunta.
		boton("gallery.moreLikeThese", iconos.Imagen, l.AlParecidas),
		boton("gallery.downloadSelected", iconos.Descargar, l.AlDescargar),
		filete(v),
		tirar,
	)
	// El recuento lleva `mr-auto`: lo que le sigue va empujado a la derecha.
	cuerpo := container.New(ui.FilaAuto{Hueco: tema.Espacio(1), Derecha: 2, Partir: true, SaltoFila: tema.Espacio(1)}, hijos...)
	return conSombra(tema.SombraSM, tema.RadioMD, recuadro(v, "surface-raised", "border", tema.RadioMD,
		ui.Margen{Izq: tema.Espacio(2), Der: tema.Espacio(2), Arr: tema.Espacio(1.5), Abj: tema.Espacio(1.5)}, cuerpo))
}

// filete es el `<Filete />` de la web: `bg-border mx-1 h-5 w-px`, lo que separa dos
// grupos de mandos en una barra.
func filete(v Vista) fyne.CanvasObject {
	raya := canvas.NewRectangle(tema.Color("border", v.Variante))
	return container.New(fileteLayout{}, raya)
}

type fileteLayout struct{}

func (fileteLayout) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(tema.Borde+2*tema.Espacio(1), tema.Espacio(5))
}

func (fileteLayout) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(tema.Espacio(1), (t.Height-tema.Espacio(5))/2))
	objs[0].Resize(fyne.NewSize(tema.Borde, tema.Espacio(5)))
}

// conSombra pone la sombra de ese nivel DETRÁS del objeto, como el `box-shadow` de la
// web: no ocupa sitio, así que el objeto mide lo mismo con ella que sin ella.
func conSombra(n tema.NivelSombra, radio float32, o fyne.CanvasObject) fyne.CanvasObject {
	s := ui.NuevaSombra(n, radio)
	return container.New(&sombraLayout{s: s}, s.Objeto(), o)
}

type sombraLayout struct{ s *ui.Sombra }

func (l *sombraLayout) MinSize(objs []fyne.CanvasObject) fyne.Size { return objs[1].MinSize() }

// AltoCon: el del objeto, que es lo que ocupa.
func (l *sombraLayout) AltoCon(objs []fyne.CanvasObject, ancho float32) float32 {
	return ui.AltoA(objs[1], ancho)
}

func (l *sombraLayout) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[1].Move(fyne.NewPos(0, 0))
	objs[1].Resize(t)
	l.s.Ajustar(fyne.NewPos(0, 0), t, tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
}

// rejillaDeFotos es el `<ul class="filas">` de un mes, de un álbum o de «parecidas»: las
// mismas filas justificadas en todas las pantallas (ver filas.go).
func rejillaDeFotos(v Vista, fotos []api.Photo, d Galeria) *fyne.Container {
	hijos := make([]fyne.CanvasObject, len(fotos))
	for i, f := range fotos {
		hijos[i] = teselaDe(v, f, d)
	}
	return container.New(filasDe(v, fotos), hijos...)
}

func teselaDe(v Vista, f api.Photo, d Galeria) *Miniatura {
	m := NuevaMiniatura(f, v.Teca)
	m.Filete = true
	if d.EnSeleccion {
		m.Seleccionando, m.Seleccionada = true, d.Seleccion[f.ID]
		m.Rotulo = v.T.T("gallery.selectTile", i18n.Vars{"name": f.Name})
		m.AlPulsar = func() {
			if d.AlAlternar != nil {
				d.AlAlternar(f)
			}
		}
		return m
	}
	m.Rotulo = v.T.T("gallery.open", i18n.Vars{"name": f.Name})
	m.AlPulsar = func() {
		if d.AlAbrir != nil {
			d.AlAbrir(f)
		}
	}
	if d.AlFavorita != nil {
		clave := "gallery.favorite"
		if f.Favorite {
			clave = "gallery.unfavorite"
		}
		m.RotuloFavorita = v.Traduce(clave)
		m.AlFavorita = func() { d.AlFavorita(f) }
	}
	return m
}

// versalEn es el `.sytue-overline` de los encabezados de sección: versalitas con
// `tracking-wide`, que es lo que pinta `ui.Versal`.
func versalEn(s, token string, v tema.Variante) fyne.CanvasObject {
	c := &cajaVersal{vs: ui.NuevoVersal(s, tema.TextoXS, ui.SemiNegrita, tema.Color(token, v))}
	return c.contenedor()
}

// cajaVersal coloca un `ui.Versal` —que no es un CanvasObject, sino un juego de
// letras— dentro de un contenedor que sí lo es, con un relleno arriba si lo lleva.
type cajaVersal struct {
	vs     *ui.Versal
	arriba float32
}

var _ fyne.Layout = (*cajaVersal)(nil)

func (c *cajaVersal) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(c.vs.Ancho(), c.arriba+tema.TextoXS.Linea)
}

func (c *cajaVersal) Layout([]fyne.CanvasObject, fyne.Size) {}

// contenedor arma el contenedor con las letras ya colocadas.
func (c *cajaVersal) contenedor() *fyne.Container {
	c.vs.Colocar(0, c.arriba)
	return container.New(c, c.vs.Objetos()...)
}

// ---- Álbumes ----

// Albumes son los datos de la pantalla de álbumes.
type Albumes struct {
	Lista []api.Album
	// Titulo es el campo de «Título del álbum»; como el buscador, vive fuera.
	Titulo *ui.Entrada
	// Montones son las sugerencias de Sytue AI; nil es «todavía no se han pedido».
	Montones [][]api.Photo
	Buscando bool
	Error    string

	// ExcluidaLoteIA y PuedeGestionarLoteIA reflejan el ajuste de organización en
	// Sytue Photos: si la organización está excluida del lote automático de IA y si
	// quien mira (`owner`/`admin`) puede cambiarlo.
	ExcluidaLoteIA       bool
	PuedeGestionarLoteIA bool
	AlExcluirLoteIA      func(bool)

	AlAbrir   func(api.Album)
	AlIr      func(Seccion)
	AlCrear   func(titulo string)
	AlBuscar  func()
	AlGuardar func([]api.Photo)
	AlVer     func([]api.Photo)
}

// PantallaAlbumes monta «Álbumes»: el formulario de crear, las sugerencias y la
// rejilla de tarjetas.
func PantallaAlbumes(v Vista, d Albumes) *fyne.Container {
	campo := d.Titulo
	if campo == nil {
		campo = ui.NuevaEntrada(v.Traduce("albums.title"))
	}
	crear := container.New(filaConCampoElastico{Hueco: tema.Espacio(2), MaxCampo: tema.Espacio(64)},
		campo,
		ui.NuevoBoton(v.Traduce("albums.create"), func() {
			if d.AlCrear != nil {
				d.AlCrear(campo.Texto())
			}
		}).Con(ui.Primario, ui.TallaMD).ConIcono(iconos.Mas, 0),
	)
	claveBoton := "albums.suggestionsFind"
	if d.Buscando {
		claveBoton = "albums.suggestionsLoading"
	}
	// De fantasma: la acción principal de esta pantalla es crear un álbum.
	buscar := ui.NuevoBoton(v.Traduce(claveBoton), d.AlBuscar).Con(ui.Fantasma, ui.TallaSM).ConIcono(iconos.Buscar, 0)
	buscar.Inerte = d.Buscando
	hijosSugerencias := []fyne.CanvasObject{
		ParrafoEn(v.Traduce("albums.suggestions"), tema.TextoSM, ui.SemiNegrita, "text", v.Variante),
		buscar,
	}
	if d.PuedeGestionarLoteIA {
		casilla := ui.NuevaCasilla(v.Traduce("aiBatch.excludeOrg"), d.AlExcluirLoteIA)
		casilla.Marcada = d.ExcluidaLoteIA
		casilla.Escalon = tema.TextoXS
		hijosSugerencias = append(hijosSugerencias, casilla)
	}
	sugerencias := container.New(ui.FilaAuto{Hueco: tema.Espacio(2), Derecha: 2, Partir: true, SaltoFila: tema.Espacio(2)},
		hijosSugerencias...,
	)

	bloques := []ui.Bloque{
		{Obj: Nav(v, SecAlbumes, d.AlIr), Margen: SeparacionBloque},
		{Obj: crear, Margen: SeparacionSeccion},
	}
	if d.Error != "" {
		bloques = append(bloques, ui.Bloque{
			Obj: ParrafoEn(d.Error, tema.TextoSM, ui.Normal, "danger-text", v.Variante), Margen: SeparacionBloque,
		})
	}
	// La sección `mb-6`: su cabecera con `mb-2`, y debajo lo que haya.
	seccion := []ui.Bloque{{Obj: sugerencias, Margen: tema.Espacio(2)}}
	if d.ExcluidaLoteIA {
		seccion = append(seccion, ui.Bloque{
			Obj:    ParrafoEn(v.Traduce("aiBatch.excludedHint"), tema.TextoXS, ui.Normal, "text-muted", v.Variante),
			Margen: tema.Espacio(2),
		})
	}
	if d.Montones != nil && len(d.Montones) == 0 {
		seccion = append(seccion, ui.Bloque{
			Obj: ParrafoEn(v.Traduce("albums.suggestionsNone"), tema.TextoSM, ui.Normal, "text-muted", v.Variante),
		})
	}
	for _, m := range d.Montones {
		seccion = append(seccion, ui.Bloque{Obj: filaMonton(v, m, d), Margen: tema.Espacio(3)})
	}
	seccion[len(seccion)-1].Margen = 0
	bloques = append(bloques, ui.Bloque{Obj: ui.PilaBloques(seccion...), Margen: SeparacionSeccion})

	vacio := EstadoVacio(v, "albums.empty.title", "albums.empty.description", nil)
	if cuerpo := CuerpoDeEstado(v, vacio); cuerpo != nil {
		bloques = append(bloques, ui.Bloque{Obj: cuerpo})
		return Envolver(v, false, ui.PilaBloques(bloques...))
	}

	tarjetas := make([]fyne.CanvasObject, len(d.Lista))
	for i, a := range d.Lista {
		tarjetas[i] = TarjetaAlbum(v, a, d.AlAbrir)
	}
	r := ui.RejillaAlbumes()
	bloques = append(bloques, ui.Bloque{
		Obj:  container.New(r, tarjetas...),
		Alto: func(ancho float32) float32 { return r.MinSizeCon(tarjetas, ancho).Height },
	})
	return Envolver(v, false, ui.PilaBloques(bloques...))
}

// ladoMonton es el `h-14 w-14` de las fotos de un montón sugerido.
const ladoMonton = 14 * 0.25 * tema.Rem

// filaMonton es un `<li class="border rounded-md p-2 flex flex-wrap items-center
// gap-2">`: hasta ocho fotos, el recuento y, empujados a la derecha, «Ver todas» y
// «Guardar como álbum».
func filaMonton(v Vista, fotos []api.Photo, d Albumes) fyne.CanvasObject {
	hijos := make([]fyne.CanvasObject, 0, 12)
	for _, f := range fotos[:min(len(fotos), 8)] {
		m := NuevaMiniatura(f, v.Teca)
		m.SinMarcas, m.Rotulo = true, f.Name
		hijos = append(hijos, anchoFijo(m, ladoMonton))
	}
	hijos = append(hijos, ParrafoEn(Recuento(v, "albums.count", len(fotos)), tema.TextoSM, ui.Normal, "text-muted", v.Variante))
	derecha := len(hijos)
	// «Ver todas» y «Guardar como álbum»: dos botones de fantasma con su icono, en un
	// grupo con `gap-1`. Eran un enlace subrayado y un botón relleno.
	ver := ui.NuevoBoton(v.Traduce("albums.suggestionSee"), func() {
		if d.AlVer != nil {
			d.AlVer(fotos)
		}
	}).Con(ui.Fantasma, ui.TallaSM).ConIcono(iconos.Imagen, 0)
	guardar := ui.NuevoBoton(v.Traduce("albums.suggestionSave"), func() {
		if d.AlGuardar != nil {
			d.AlGuardar(fotos)
		}
	}).Con(ui.Fantasma, ui.TallaSM).ConIcono(iconos.Album, 0)
	hijos = append(hijos, container.New(ui.Fila{Hueco: tema.Espacio(1)}, ver, guardar))
	fila := container.New(ui.FilaAuto{Hueco: tema.Espacio(2), Derecha: derecha, Partir: true, SaltoFila: tema.Espacio(2)}, hijos...)
	return recuadro(v, "", "border", tema.RadioMD,
		ui.Margen{Izq: tema.Espacio(2), Der: tema.Espacio(2), Arr: tema.Espacio(2), Abj: tema.Espacio(2)}, fila)
}

// AlbumAbierto son los datos de un álbum (AlbumView.tsx).
type AlbumAbierto struct {
	Album api.Album
	// Fotos nil es «todavía llegando».
	Fotos        []api.Photo
	Aviso, Error string

	AlVolver, AlCompartir, AlDejarDeCompartir, AlBorrar func()
	// AlCompartirConPersonas abre «Compartir con personas»; nil: no se ofrece.
	AlCompartirConPersonas func()
	AlAbrir                func(api.Photo)
	AlIr                   func(Seccion)
}

// PantallaAlbum es un álbum: cabecera con la vuelta, el título, el recuento y los mandos
// de compartir y borrar, y debajo la cuadrícula.

func PantallaAlbum(v Vista, d AlbumAbierto) *fyne.Container {
	titulo := ParrafoEn(d.Album.Title, tema.TextoLG, ui.SemiNegrita, "text", v.Variante)
	izquierda := []fyne.CanvasObject{botonVolver(v, "albums.back", d.AlVolver), titulo}
	if d.Fotos != nil {
		izquierda = append(izquierda, ParrafoEn(Recuento(v, "albums.count", len(d.Fotos)), tema.TextoSM, ui.Normal, "text-muted", v.Variante))
	}
	var mandos []fyne.CanvasObject
	if d.Aviso != "" {
		mandos = append(mandos, ParrafoEn(d.Aviso, tema.TextoXS, ui.Normal, "text-muted", v.Variante))
	}
	// UNA acción principal —compartir con personas—, las demás de fantasma y borrar
	// aparte, tras un filete y sin gritar (`IconButton tone="danger"`): eran cuatro
	// botones seguidos, dos de ellos rellenos y del mismo peso.
	if d.AlCompartirConPersonas != nil {
		mandos = append(mandos, ui.NuevoBoton(v.Traduce("sharePeople.open"), d.AlCompartirConPersonas).
			Con(ui.Primario, ui.TallaSM).ConIcono(iconoCompartirConPersonas, 0))
	}
	mandos = append(mandos,
		ui.NuevoBoton(v.Traduce(compartirO(d.Album)), d.AlCompartir).Con(ui.Fantasma, ui.TallaSM).ConIcono(iconos.Enlace, 0))
	if d.Album.Shared {
		mandos = append(mandos, ui.NuevoBoton(v.Traduce("album.unshare"), d.AlDejarDeCompartir).Con(ui.Fantasma, ui.TallaSM))
	}
	borrar := ui.NuevoBotonIcono(iconos.Papelera, v.Traduce("albums.delete"), d.AlBorrar)
	borrar.Lado, borrar.TamIcono = ui.LadoIconoLG, IconoMando
	borrar.FondoEncima, borrar.TrazoEncima = "bg-subtle", "danger-text"
	mandos = append(mandos, filete(v), borrar)
	// El grupo de la derecha es `ml-auto flex flex-wrap gap-1`: se parte en su hueco.
	derecha := container.New(ui.FilaAuto{Hueco: huecoMandosAlbum, Derecha: len(mandos), Partir: true, SaltoFila: huecoMandosAlbum}, mandos...)
	cabecera := container.New(&cabeceraAlbum{titulo: titulo, completo: d.Album.Title, derecha: derecha}, append(izquierda, derecha)...)
	bloques := []ui.Bloque{
		{Obj: Nav(v, SecAlbumes, d.AlIr), Margen: SeparacionBloque},
		{Obj: cabecera, Margen: SeparacionBloque},
	}
	if d.Error != "" {
		bloques = append(bloques, ui.Bloque{Obj: ParrafoEn(d.Error, tema.TextoSM, ui.Normal, "danger-text", v.Variante), Margen: SeparacionBloque})
	}
	if cuerpo := CuerpoDeEstado(v, EstadoVacio(v, "albums.empty.title", "albums.empty.description", nil)); cuerpo != nil {
		bloques = append(bloques, ui.Bloque{Obj: cuerpo})
		return Envolver(v, false, ui.PilaBloques(bloques...))
	}
	bloques = append(bloques, ui.Bloque{Obj: rejillaDeFotos(v, d.Fotos, Galeria{AlAbrir: d.AlAbrir})})
	return Envolver(v, false, ui.PilaBloques(bloques...))
}

// cabeceraAlbum es el `<header class="flex items-center gap-2">` del álbum: la vuelta,
// el título en `truncate`, el recuento y el grupo de mandos empujado a la derecha. Si no
// cabe todo, encoge primero el título (su mínimo es cero) y después el grupo de la
// derecha, que se parte en filas hasta el ancho de su mando más ancho.
type cabeceraAlbum struct {
	titulo   *canvas.Text
	completo string // el título sin truncar: Truncar escribe en titulo.Text
	derecha  *fyne.Container
}

// reparto dice cuánto se lleva el título y cuánto el grupo de la derecha a ese ancho.
func (c *cabeceraAlbum) reparto(objs []fyne.CanvasObject, ancho float32) (titulo, derecha float32) {
	hueco := tema.Espacio(2)
	fijos := hueco * float32(len(objs)-1)
	for _, o := range objs[:len(objs)-1] {
		if o != c.titulo {
			fijos += o.MinSize().Width
		}
	}
	natural := ui.Medir(c.completo, tema.TextoLG, ui.SemiNegrita).Width
	grupo := float32(0)
	minimo := float32(0)
	huecoGrupo := float32(hueco)
	if f, ok := c.derecha.Layout.(ui.FilaAuto); ok {
		huecoGrupo = f.Hueco // el del grupo, que no siempre es el de la cabecera
	}
	for i, o := range c.derecha.Objects {
		w := o.MinSize().Width
		minimo = max(minimo, w)
		if i > 0 {
			grupo += huecoGrupo
		}
		grupo += w
	}
	libre := ancho - fijos
	derecha = min(grupo, max(libre-natural, minimo))
	return max(min(natural, libre-derecha), 0), derecha
}

func (c *cabeceraAlbum) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, max(tema.TextoLG.Linea, c.derecha.MinSize().Height))
}

// AltoCon: el de la fila, o el del grupo de la derecha si se parte.
func (c *cabeceraAlbum) AltoCon(objs []fyne.CanvasObject, ancho float32) float32 {
	_, derecha := c.reparto(objs, ancho)
	return max(tema.TextoLG.Linea, ui.AltoA(c.derecha, derecha))
}

func (c *cabeceraAlbum) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	anchoTitulo, anchoDerecha := c.reparto(objs, t.Width)
	ui.Truncar(c.titulo, c.completo, tema.TextoLG, ui.SemiNegrita, anchoTitulo)
	x := float32(0)
	for _, o := range objs[:len(objs)-1] {
		m := o.MinSize()
		if o == c.titulo {
			ui.ColocarTexto(c.titulo, tema.TextoLG, x, (t.Height-tema.TextoLG.Linea)/2)
			x += anchoTitulo + tema.Espacio(2)
			continue
		}
		o.Resize(m)
		o.Move(fyne.NewPos(x, (t.Height-m.Height)/2))
		x += m.Width + tema.Espacio(2)
	}
	h := ui.AltoA(c.derecha, anchoDerecha)
	c.derecha.Move(fyne.NewPos(t.Width-anchoDerecha, (t.Height-h)/2))
	c.derecha.Resize(fyne.NewSize(anchoDerecha, h))
}

// huecoMandosAlbum es el `gap-1` del grupo de mandos de la cabecera de un álbum.
const huecoMandosAlbum = 1 * 0.25 * tema.Rem

// botonVolver es la vuelta de una cabecera: `Button variant="ghost" size="sm"` con el
// galón delante. Era un enlace de texto con una flecha escrita («← Volver»).
func botonVolver(v Vista, clave string, al func()) *ui.Boton {
	return ui.NuevoBoton(v.Traduce(clave), al).Con(ui.Fantasma, ui.TallaSM).ConIcono(iconos.ChevronIzquierda, 0)
}

// botonQuitarDelAlbum es «Quitar del álbum» en un visor: de fantasma, con el `minus`
// delante —la foto se queda en la biblioteca; la papelera es otra cosa— y el rótulo
// solo desde `md:` (`max-md:sr-only`).
func botonQuitarDelAlbum(v Vista, al func()) *ui.Boton {
	b := ui.NuevoBoton(v.Traduce("albums.remove"), al).Con(ui.Fantasma, ui.TallaSM).ConIcono(iconos.Menos, 0)
	if v.Ventana < ui.CorteMD {
		b.Rotulo, b.Texto = b.Texto, ""
	}
	return b
}

func compartirO(a api.Album) string {
	if a.Shared {
		return "album.newLink"
	}
	return "album.share"
}

// TarjetaAlbum es una tarjeta de «Álbumes»: portada cuadrada, título y recuento.
func TarjetaAlbum(v Vista, a api.Album, alAbrir func(api.Album)) fyne.CanvasObject {
	portada := NuevaMiniatura(api.Photo{ID: a.CoverAssetID, Name: a.Title}, v.Teca)
	portada.Rotulo, portada.SinMarcas = a.Title, true
	if alAbrir != nil {
		portada.AlPulsar = func() { alAbrir(a) }
	}
	titulo := ParrafoEn(a.Title, tema.TextoSM, ui.Medio, "text", v.Variante)
	cuenta := ParrafoEn(Recuento(v, "albums.count", a.Count), tema.TextoXS, ui.Normal, "text-muted", v.Variante)
	return container.New(tarjetaAlbumLayout{}, portada, titulo, cuenta)
}

// tarjetaAlbumLayout: portada cuadrada a todo el ancho, `mt-1` y las dos líneas.
type tarjetaAlbumLayout struct{}

var _ fyne.Layout = tarjetaAlbumLayout{}

const margenPieAlbum = 1 * 0.25 * tema.Rem // `mt-1`

func (tarjetaAlbumLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	lado := float32(11 * tema.Rem) // `minmax(11rem,1fr)`
	return fyne.NewSize(lado, lado+margenPieAlbum+tema.TextoSM.Linea+tema.TextoXS.Linea)
}

func (tarjetaAlbumLayout) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	if len(objs) < 3 {
		return
	}
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(fyne.NewSize(t.Width, t.Width))
	y := t.Width + margenPieAlbum
	objs[1].Move(fyne.NewPos(0, y))
	objs[1].Resize(fyne.NewSize(t.Width, tema.TextoSM.Linea))
	objs[2].Move(fyne.NewPos(0, y+tema.TextoSM.Linea))
	objs[2].Resize(fyne.NewSize(t.Width, tema.TextoXS.Linea))
}

// ---- Papelera ----

// Papelera son los datos de la papelera.
type Papelera struct {
	Fotos        []api.Photo
	Aviso, Error string

	AlIr                  func(Seccion)
	AlVaciar              func()
	AlRestaurar, AlPurgar func(api.Photo)
}

// PantallaPapelera monta la papelera. Su rejilla es la ÚNICA de la suite que no es de
// cuadrados: ahí cada miniatura conserva su proporción.
func PantallaPapelera(v Vista, d Papelera) *fyne.Container {
	arriba := []fyne.CanvasObject{Nav(v, SecPapelera, d.AlIr)}
	if len(d.Fotos) > 0 {
		arriba = append(arriba,
			ui.NuevoBoton(v.Traduce("trash.empty"), d.AlVaciar).Con(ui.Peligro, ui.TallaSM).ConIcono(iconos.Papelera, 0))
	}
	bloques := []ui.Bloque{
		{Obj: container.New(ui.FilaAuto{Hueco: tema.Espacio(2), Derecha: 1, Partir: true, SaltoFila: SeparacionBloque + tema.Espacio(2)}, arriba...), Margen: SeparacionBloque},
		// `min-h-5`: el aviso reserva su línea aunque esté vacío.
		{Obj: ParrafoEn(d.Aviso, tema.TextoSM, ui.Normal, "text-muted", v.Variante), Alto: func(float32) float32 { return max(5*0.25*tema.Rem, tema.TextoSM.Linea) }},
	}
	if d.Error != "" {
		bloques = append(bloques, ui.Bloque{Obj: ParrafoEn(d.Error, tema.TextoSM, ui.Normal, "danger-text", v.Variante), Margen: SeparacionBloque})
	}
	vacio := EstadoVacio(v, "trash.empty.title", "trash.empty.description", nil)
	if cuerpo := CuerpoDeEstado(v, vacio); cuerpo != nil {
		bloques = append(bloques, ui.Bloque{Obj: cuerpo})
		return Envolver(v, false, ui.PilaBloques(bloques...))
	}
	hijos := make([]fyne.CanvasObject, len(d.Fotos))
	for i, f := range d.Fotos {
		hijos[i] = teselaPapelera(v, f, d)
	}
	bloques = append(bloques, ui.Bloque{Obj: container.New(filasDe(v, d.Fotos), hijos...)})
	return Envolver(v, false, ui.PilaBloques(bloques...))
}

// teselaPapelera es la foto al 70 % con UN mando abajo a la derecha: el «⋯» sobre foto
// (`IconButton tone="media" size="sm"`), que abre el menú con «Restaurar» y «Borrar para
// siempre». Eran dos botones de icono en cada tesela, que con el dedo no caben en una
// foto vertical. Su nombre es el de la foto: es el menú DE esa foto.
func teselaPapelera(v Vista, f api.Photo, d Papelera) fyne.CanvasObject {
	m := NuevaMiniatura(f, v.Teca)
	m.Atenuada, m.SinMarcas, m.Filete, m.Rotulo = true, true, true, f.Name
	var mando *MandoSobreFoto
	mando = NuevoMandoSobreFoto(iconos.Puntos, f.Name, ui.LadoIconoSM, iconos.TallaSM, func() {
		restaurar := ui.NuevoElementoMenu(v.Traduce("trash.restore"), iconos.Deshacer, func() {
			if d.AlRestaurar != nil {
				d.AlRestaurar(f)
			}
		})
		purgar := ui.NuevoElementoMenu(v.Traduce("trash.purge"), iconos.Papelera, func() {
			if d.AlPurgar != nil {
				d.AlPurgar(f)
			}
		})
		purgar.Peligro = true
		mostrarMenu(f.Name, mando, []fyne.CanvasObject{restaurar, purgar})
	})
	return container.New(esquinaAbajo{}, m, mando)
}

// esquinaAbajo pone el primer hijo a toda la caja y el segundo a `right-1 bottom-1`.
type esquinaAbajo struct{}

func (esquinaAbajo) MinSize(objs []fyne.CanvasObject) fyne.Size { return objs[0].MinSize() }

func (esquinaAbajo) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(t)
	m := objs[1].MinSize()
	objs[1].Move(fyne.NewPos(t.Width-tema.Espacio(1)-m.Width, t.Height-tema.Espacio(1)-m.Height))
	objs[1].Resize(m)
}

// ---- Repetidas ----

// Repetidas son los grupos de fotos que se repiten.
type Repetidas struct {
	Grupos       [][]api.Photo
	Aviso, Error string
	// Trabajando inertiza los «Quedarme con esta» mientras se mandan las otras a la
	// papelera.
	Trabajando bool

	ExcluidaLoteIA       bool
	PuedeGestionarLoteIA bool
	AlExcluirLoteIA      func(bool)

	AlIr       func(Seccion)
	AlQuedarse func(grupo int, f api.Photo)
}

// PantallaRepetidas monta «Fotos que se repiten»: se PROPONE y no se ejecuta, así que
// cada grupo enseña sus fotos y un botón por foto para quedarse con esa.
func PantallaRepetidas(v Vista, d Repetidas) *fyne.Container {
	var titulo fyne.CanvasObject = ParrafoEn(v.Traduce("dup.title"), tema.TextoLG, ui.SemiNegrita, "text", v.Variante)
	if d.PuedeGestionarLoteIA {
		casilla := ui.NuevaCasilla(v.Traduce("aiBatch.excludeOrg"), d.AlExcluirLoteIA)
		casilla.Marcada = d.ExcluidaLoteIA
		casilla.Escalon = tema.TextoXS
		titulo = container.New(ui.FilaAuto{Hueco: tema.Espacio(2), Derecha: 1, Partir: true, SaltoFila: tema.Espacio(2)},
			titulo, casilla)
	}
	bloques := []ui.Bloque{
		{Obj: Nav(v, SecRepetidas, d.AlIr), Margen: SeparacionBloque},
		{Obj: titulo},
		// `max-w-2xl`: el párrafo no pasa de 42rem aunque haya sitio.
		{Obj: container.New(anchoMaximo(42*tema.Rem), ui.NuevoParrafo(v.Traduce("dup.explain"),
			tema.TextoSM, ui.Normal, "text-muted", v.Variante)), Margen: SeparacionBloque},
	}
	if d.ExcluidaLoteIA {
		bloques = append(bloques, ui.Bloque{
			Obj:    ParrafoEn(v.Traduce("aiBatch.excludedHint"), tema.TextoXS, ui.Normal, "text-muted", v.Variante),
			Margen: SeparacionBloque,
		})
	}
	bloques = append(bloques, ui.Bloque{
		Obj:  ParrafoEn(d.Aviso, tema.TextoSM, ui.Normal, "text-muted", v.Variante),
		Alto: func(float32) float32 { return max(5*0.25*tema.Rem, tema.TextoSM.Linea) },
	})
	if d.Error != "" {
		bloques = append(bloques, ui.Bloque{Obj: ParrafoEn(d.Error, tema.TextoSM, ui.Normal, "danger-text", v.Variante), Margen: SeparacionBloque})
	}
	vacio := EstadoVacio(v, "dup.empty.title", "dup.empty.description", nil)
	if cuerpo := CuerpoDeEstado(v, vacio); cuerpo != nil {
		bloques = append(bloques, ui.Bloque{Obj: cuerpo})
		return Envolver(v, false, ui.PilaBloques(bloques...))
	}
	bloques = append(bloques, ui.Bloque{
		Obj:    ParrafoEn(Recuento(v, "dup.count", len(d.Grupos)), tema.TextoSM, ui.Normal, "text-muted", v.Variante),
		Margen: tema.Espacio(3),
	})
	for i, g := range d.Grupos {
		bloques = append(bloques, ui.Bloque{Obj: grupoRepetido(v, i, g, d), Margen: tema.Espacio(6)})
	}
	return Envolver(v, false, ui.PilaBloques(bloques...))
}

// anchoMaximo es un `max-w-*`: el hijo ocupa todo el ancho hasta ese tope.
type anchoMaximo float32

func (a anchoMaximo) MinSize(objs []fyne.CanvasObject) fyne.Size { return objs[0].MinSize() }

// AltoCon: el del hijo al ancho que le queda con el tope.
func (a anchoMaximo) AltoCon(objs []fyne.CanvasObject, ancho float32) float32 {
	return ui.AltoA(objs[0], min(ancho, float32(a)))
}

func (a anchoMaximo) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(fyne.NewSize(min(t.Width, float32(a)), t.Height))
}

// ladoRepetida es el `h-40 w-40` de la miniatura de un grupo repetido.
const ladoRepetida = 40 * 0.25 * tema.Rem

// grupoRepetido es el `<li class="border rounded-md p-3">`: las fotos con su botón en
// `flex-wrap gap-3` y, con `mt-2`, cuántas irían a la papelera.
func grupoRepetido(v Vista, i int, g []api.Photo, d Repetidas) fyne.CanvasObject {
	hijos := make([]fyne.CanvasObject, 0, len(g))
	for _, f := range g {
		m := NuevaMiniatura(f, v.Teca)
		m.Rotulo, m.SinMarcas = f.Name, true
		quedarse := ui.NuevoBoton(v.Traduce("dup.keep"), func() {
			if d.AlQuedarse != nil {
				d.AlQuedarse(i, f)
			}
		}).Con(ui.Secundario, ui.TallaSM).ConIcono(iconos.Tic, iconos.TallaSM)
		quedarse.Ancho, quedarse.Inerte = ladoRepetida, d.Trabajando
		hijos = append(hijos, container.New(columnaRepetida{}, m, quedarse))
	}
	fila := container.New(ui.Fila{Hueco: tema.Espacio(3)}, hijos...)
	pie := ParrafoEn(Recuento(v, "dup.trashRest", len(g)-1), tema.TextoXS, ui.Normal, "text-subtle", v.Variante)
	return recuadro(v, "", "border", tema.RadioMD,
		ui.Margen{Izq: tema.Espacio(3), Der: tema.Espacio(3), Arr: tema.Espacio(3), Abj: tema.Espacio(3)},
		ui.PilaBloques(ui.Bloque{Obj: fila, Margen: tema.Espacio(2)}, ui.Bloque{Obj: pie}))
}

// columnaRepetida es la columna `w-40` de una foto repetida: la miniatura cuadrada y,
// con `mt-2`, el botón a todo el ancho.
type columnaRepetida struct{}

var _ fyne.Layout = columnaRepetida{}

func (columnaRepetida) MinSize(objs []fyne.CanvasObject) fyne.Size {
	alto := ladoRepetida + tema.Espacio(2)
	if len(objs) > 1 {
		alto += objs[1].MinSize().Height
	}
	return fyne.NewSize(ladoRepetida, alto)
}

func (columnaRepetida) Layout(objs []fyne.CanvasObject, _ fyne.Size) {
	if len(objs) < 2 {
		return
	}
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(fyne.NewSize(ladoRepetida, ladoRepetida))
	objs[1].Move(fyne.NewPos(0, ladoRepetida+tema.Espacio(2)))
	objs[1].Resize(fyne.NewSize(ladoRepetida, objs[1].MinSize().Height))
}

// ---- Parecidas y «por aquí» ----

// Encontradas son los datos de «parecidas a esta» y de «fotos de por aquí», que son la
// misma pantalla con otro título y otra pregunta detrás.
type Encontradas struct {
	// Referencias son las fotos de partida, que se enseñan pequeñas en la cabecera.
	Referencias []api.Photo
	// Fotos nil es «todavía llegando».
	Fotos []api.Photo
	// Cerca cambia el juego de textos al de «por aquí».
	Cerca bool
	// Varias es que se partió de más de una foto («Más como estas»).
	Varias bool
	// Coordenadas es el «41,3851, 2,1734» de la foto de partida; solo en «por aquí».
	Coordenadas  string
	Aviso, Error string
	AlIr         func(Seccion)
	AlAbrir      func(api.Photo)
	AlVolver     func()
	AlGuardar    func()
}

// ladoReferencia es el `h-12 w-12` de las miniaturas de la cabecera.
const ladoReferencia = 12 * 0.25 * tema.Rem

// PantallaEncontradas monta «parecidas» o «por aquí».
func PantallaEncontradas(v Vista, d Encontradas) *fyne.Container {
	// Las claves van LITERALES y no compuestas: una clave armada en tiempo de ejecución
	// no la ve el guardián de i18n.
	titulo, claveCuenta := "similar.title", "similar.count"
	vacioTitulo, vacioTexto := "similar.empty.title", "similar.empty.description"
	switch {
	case d.Cerca:
		titulo, claveCuenta = "nearby.title", "nearby.count"
		vacioTitulo, vacioTexto = "nearby.empty.title", "nearby.empty.description"
	case d.Varias || len(d.Referencias) > 1:
		titulo = "similar.titleMany"
	}
	hijos := []fyne.CanvasObject{botonVolver(v, "similar.back", d.AlVolver)}
	if !d.Cerca {
		for _, r := range d.Referencias {
			m := NuevaMiniatura(r, v.Teca)
			m.Rotulo, m.SinMarcas = r.Name, true
			hijos = append(hijos, anchoFijo(m, ladoReferencia))
		}
	}
	hijos = append(hijos, ParrafoEn(v.Traduce(titulo), tema.TextoLG, ui.SemiNegrita, "text", v.Variante))
	if d.Coordenadas != "" {
		hijos = append(hijos, ParrafoEn(d.Coordenadas, tema.TextoSM, ui.Normal, "text-muted", v.Variante))
	}
	if d.Fotos != nil {
		hijos = append(hijos, ParrafoEn(Recuento(v, claveCuenta, len(d.Fotos)),
			tema.TextoSM, ui.Normal, "text-muted", v.Variante))
	}
	derecha := len(hijos)
	if len(d.Fotos) > 0 && !d.Cerca {
		// La acción principal de esta pantalla: quedarse con lo encontrado.
		hijos = append(hijos, ui.NuevoBoton(v.Traduce("similar.saveAlbum"), d.AlGuardar).
			Con(ui.Primario, ui.TallaSM).ConIcono(iconos.Album, 0))
	}

	bloques := []ui.Bloque{
		{Obj: Nav(v, SecFotos, d.AlIr), Margen: SeparacionBloque},
		{Obj: container.New(ui.FilaAuto{Hueco: tema.Espacio(3), Derecha: derecha}, hijos...), Margen: SeparacionBloque},
	}
	for _, p := range [...]struct{ texto, token string }{{d.Aviso, "text-muted"}, {d.Error, "danger-text"}} {
		if p.texto != "" {
			bloques = append(bloques, ui.Bloque{Obj: ParrafoEn(p.texto, tema.TextoSM, ui.Normal, p.token, v.Variante), Margen: SeparacionBloque})
		}
	}
	vacio := EstadoVacio(v, vacioTitulo, vacioTexto, nil)
	if cuerpo := CuerpoDeEstado(v, vacio); cuerpo != nil {
		bloques = append(bloques, ui.Bloque{Obj: cuerpo})
		return Envolver(v, false, ui.PilaBloques(bloques...))
	}
	bloques = append(bloques, ui.Bloque{Obj: rejillaDeFotos(v, d.Fotos, Galeria{AlAbrir: d.AlAbrir})})
	return Envolver(v, false, ui.PilaBloques(bloques...))
}

// ---- El álbum público, /p/{token} ----

// PantallaPublica es el álbum compartido por enlace: sin sesión y sin nav. La abre el
// binario al recibir `sytue-photos:/p/{token}` o la dirección web del enlace (ver
// App.AbrirPublico). `alAbrir` es el `<a href={img(f.id, 'grande')}>` de cada tesela.
func PantallaPublica(v Vista, titulo string, cuantas int, fotos []api.Photo, alAbrir func(api.Photo)) *fyne.Container {
	bloques := []ui.Bloque{
		{Obj: ParrafoEn(titulo, tema.Texto2XL, ui.SemiNegrita, "text", v.Variante)},
		{
			Obj:    ParrafoEn(Recuento(v, "public.count", cuantas), tema.TextoSM, ui.Normal, "text-muted", v.Variante),
			Margen: SeparacionSeccion,
		},
	}
	if cuerpo := CuerpoDeEstado(v, EstadoVacio(v, "gallery.empty.title", "gallery.empty.description", nil)); cuerpo != nil {
		bloques = append(bloques, ui.Bloque{Obj: cuerpo})
		return Envolver(v, false, ui.PilaBloques(bloques...))
	}
	hijos := make([]fyne.CanvasObject, len(fotos))
	for i, f := range fotos {
		m := NuevaMiniatura(f, v.Teca)
		m.Rotulo, m.SinMarcas, m.Filete = f.Name, true, true
		if alAbrir != nil {
			m.AlPulsar = func() { alAbrir(f) }
		}
		hijos[i] = m
	}
	bloques = append(bloques,
		ui.Bloque{Obj: container.New(filasDe(v, fotos), hijos...), Margen: tema.Espacio(8)},
		ui.Bloque{Obj: container.New(centradoH{}, ParrafoEn(v.Traduce("public.by"), tema.TextoXS, ui.Normal, "text-subtle", v.Variante))},
	)
	return Envolver(v, false, ui.PilaBloques(bloques...))
}

// VisorPublico es esa foto abierta. En la web la tesela es un enlace a la variante
// «grande», y el navegador la enseña a ventana entera, encajada y centrada, y un clic la
// pasa a su tamaño; aquí es lo mismo con el lienzo y la zona del visor, sin cabecera, ni
// mandos, ni ficha, que son de quien tiene sesión. Escape la cierra (App.teclaSuelta).
//
// El tamaño real es el de la variante (2048 de lado mayor como mucho), no el de la foto:
// el navegador enseña a su tamaño la imagen que abre, y lo que abre es la «grande».
func VisorPublico(v Vista, f api.Photo, aTamanyoReal bool, alZoom func()) fyne.CanvasObject {
	return visorSuelto(v, f, aTamanyoReal, alZoom, nil, nil)
}

// visorSuelto es la foto sola sobre el fondo del navegador, con las flechas y el
// deslizamiento del visor si hay foto a los lados (el de un álbum compartido conmigo).
func visorSuelto(v Vista, f api.Photo, aTamanyoReal bool, alZoom, anterior, siguiente func()) fyne.CanvasObject {
	if w, h := float32(f.Width), float32(f.Height); w > 0 && h > 0 {
		k := min(1, 2048/max(w, h))
		f.Width, f.Height = int(w*k+0.5), int(h*k+0.5)
	}
	l := NuevoLienzoVisor(v, Visor{Foto: f, Fondo: fondoSuelta, TamanyoReal: aTamanyoReal, AlZoom: alZoom})
	objs := []fyne.CanvasObject{l}
	if !aTamanyoReal {
		// ponytail: la foto suelta abre el tamaño real por la esquina, como el navegador
		// con una imagen abierta sola; la del visor de la galería, en el punto pulsado.
		objs = append(objs, NuevoDeslizaFoto(l, anterior, siguiente, func(float32, float32, fyne.Position) {
			if alZoom != nil {
				alZoom()
			}
		}))
	}
	cuerpo := escenario(v, container.New(zonaVisor{lienzo: l, real: aTamanyoReal, ventana: v.Ventana}, objs...), aTamanyoReal, anterior, siguiente, fyne.Position{}, nil)
	return container.NewStack(canvas.NewRectangle(fondoSuelta.colorFondo()), cuerpo)
}

// ---- Utilidades ----

// anchoFijo envuelve un objeto para que mida exactamente eso de ancho, que es lo que
// hacen `w-64` y `h-12 w-12` en la web.
func anchoFijo(o fyne.CanvasObject, ancho float32) fyne.CanvasObject {
	return container.New(fijo{ancho: ancho}, o)
}

type fijo struct{ ancho, alto float32 }

var _ fyne.Layout = fijo{}

func (f fijo) MinSize(objs []fyne.CanvasObject) fyne.Size {
	alto := f.alto
	if alto == 0 && len(objs) > 0 {
		alto = objs[0].MinSize().Height
	}
	if f.alto == 0 && f.ancho > 0 && len(objs) > 0 {
		if _, es := objs[0].(*Miniatura); es {
			alto = f.ancho // las miniaturas de la cabecera son cuadradas
		}
	}
	return fyne.NewSize(f.ancho, alto)
}

func (f fijo) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	for _, o := range objs {
		o.Move(fyne.NewPos(0, 0))
		o.Resize(t)
	}
}

// IconoDe devuelve el trazo de un icono del catálogo con el token pedido. Existe para
// que las pantallas no repitan la variante en cada llamada.
func (v Vista) IconoDe(i iconos.Icono, lado float32, token string) fyne.CanvasObject {
	return iconos.Trazo(i, lado, token, v.Variante).Objeto()
}

// ---- El botón del mes ----

// BotonMes es el «Seleccionar el mes»/«Quitar el mes» de la cabecera de cada mes:
// `text-primary-text hover:bg-selected rounded-md px-2 py-0.5 text-xs font-medium`, con
// `aria-pressed`. No es un `Button` de la web —es un `<button>` a mano— y por eso no es
// un `ui.Boton`.
//
// En un teléfono se parte en dos líneas, centradas como en todo `<button>`: ver Pieza.
type BotonMes struct {
	ui.Pulsable
	Texto  string
	Todas  bool
	fondo  *ui.Caja
	lineas []*canvas.Text
	hechas []string
	aro    *ui.Aro
	objets []fyne.CanvasObject
}

var _ fyne.Widget = (*BotonMes)(nil)

// NuevoBotonMes crea el botón.
func NuevoBotonMes(texto string, todas bool, al func()) *BotonMes {
	b := &BotonMes{Texto: texto, Todas: todas}
	b.Nombre, b.AlPulsar = texto, al
	b.ExtendBaseWidget(b)
	return b
}

// MedidaBotonMes es el rótulo con `px-2 py-0.5`.
func MedidaBotonMes(texto string) fyne.Size {
	m := ui.Medir(texto, tema.TextoXS, ui.Medio)
	return fyne.NewSize(m.Width+2*tema.Espacio(2), tema.TextoXS.Linea+2*tema.Espacio(0.5))
}

// lineasBotonMes es el rótulo partido a lo que queda dentro del `px-2`.
func lineasBotonMes(texto string, ancho float32) []string {
	return ui.Repartir(texto, tema.TextoXS, ui.Medio, ancho-2*tema.Espacio(2))
}

// Pieza es lo que el botón pone en la FilaEncoge de la cabecera del mes: el rótulo
// entero, su palabra más larga, y a cada ancho tantas líneas como salgan más el `py-0.5`.
func (b *BotonMes) Pieza() PiezaFlex {
	medir := func(s string) float32 { return ui.Medir(s, tema.TextoXS, ui.Medio).Width }
	return PiezaFlex{
		Max:      MedidaBotonMes(b.Texto).Width,
		RellenoX: 2 * tema.Espacio(2),
		Min:      palabraMasLarga(b.Texto, medir) + 2*tema.Espacio(2),
		Alto: func(ancho float32) float32 {
			return float32(max(len(lineasBotonMes(b.Texto, ancho)), 1))*tema.TextoXS.Linea + 2*tema.Espacio(0.5)
		},
	}
}

// CreateRenderer: la caja del `hover`, el rótulo y el aro.
func (b *BotonMes) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	b.fondo = ui.NuevaCaja(color.NRGBA{}, color.NRGBA{}, tema.Forma("radius-control"))
	b.aro = ui.NuevoAro(tema.Forma("radius-control"), ui.SinHueco, v)
	r := &botonMesRenderer{b: b}
	r.Refresh()
	return r
}

type botonMesRenderer struct{ b *BotonMes }

func (r *botonMesRenderer) MinSize() fyne.Size           { return MedidaBotonMes(r.b.Texto) }
func (r *botonMesRenderer) Objects() []fyne.CanvasObject { return r.b.objets }
func (r *botonMesRenderer) Destroy()                     {}
func (r *botonMesRenderer) Layout(t fyne.Size) {
	b := r.b
	b.aro.Colocar(t)
	b.fondo.Redimensionar(t)
	if l := lineasBotonMes(b.Texto, t.Width); !slices.Equal(l, b.hechas) {
		v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
		b.hechas, b.lineas = l, nil
		b.objets = append(b.aro.Objetos(), b.fondo.Objetos()...)
		for _, s := range l {
			c := ui.Texto(s, tema.TextoXS, ui.Medio, tema.Color("primary-text", v))
			b.lineas, b.objets = append(b.lineas, c), append(b.objets, c)
		}
	}
	y := (t.Height - float32(len(b.lineas))*tema.TextoXS.Linea) / 2
	for i, c := range b.lineas {
		x := (t.Width - ui.Medir(b.hechas[i], tema.TextoXS, ui.Medio).Width) / 2 // `text-align: center`
		ui.ColocarTexto(c, tema.TextoXS, x, y+float32(i)*tema.TextoXS.Linea)
	}
}

func (r *botonMesRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	var fondo color.NRGBA
	if r.b.Encima() {
		fondo = tema.Color("selected", v)
	}
	r.b.fondo.Pintar(fondo, color.NRGBA{})
	r.b.hechas = nil // el rótulo puede haber cambiado
	r.Layout(r.b.Size())
	for _, c := range r.b.lineas {
		c.Color = tema.Color("primary-text", v)
		c.Refresh()
	}
	r.b.aro.Pintar(v)
	r.b.aro.Ver(r.b.Enfocado())
}
