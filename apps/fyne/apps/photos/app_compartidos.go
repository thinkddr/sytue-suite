// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"context"
	"io"
	"net/url"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/compartir"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # Compartido conmigo
//
// `Compartidos.tsx`: los álbumes que otras personas me han compartido y, abierto, uno de
// ellos. Con «puede ver», solo mirar. Con «puede editar», renombrarlo y quitar cualquier
// foto aquí, y añadir fotos propias desde la galería («Añadir a un álbum…» ya los
// lista), que se copian al Drive de su dueña. Las imágenes van por
// `/api/v1/shared/albums/<álbum>/…`, cuya verja es que el álbum esté compartido conmigo;
// la foto abierta va sola sobre negro con la cabecera de `VisorCompartido`.
//
// Y al revés, «Compartir con personas» del álbum propio: el diálogo común de
// `apps/fyne/compartir`, para ver o para editar, como la web.

type estadoCompartidos struct {
	estadoCarga
	lista []api.AlbumCompartido
	// nombres es quién compartió cada álbum, por su `sub`: el servicio solo da eso.
	nombres map[string]string
	// portadas dice de qué álbum es cada portada: la verja de la imagen es el álbum.
	portadas map[string]string
	teca     *Fototeca
	// album es el abierto (RutaCompartido), y por quién lo compartió.
	album, por string
	// editable: me lo compartieron para editar. fallo es el `role="alert"` de un
	// renombrado o una foto que no se quitó.
	editable     bool
	falloEdicion string
}

// AlbumDeEnlace saca el álbum de un enlace a «Compartido conmigo» (el aviso
// `photos.shared`), o "" si no lo es.
func AlbumDeEnlace(u *url.URL) string {
	id, _ := api.LeerAlbumCompartido(u)
	return id
}

// AlbumPropioDeEnlace saca el álbum propio del aviso `photos.album.added` —alguien con
// edición le ha añadido fotos—, o "" si no lo es.
func AlbumPropioDeEnlace(u *url.URL) string {
	id, _ := api.LeerAlbum(u)
	return id
}

// Compartidos son los datos de «Compartido conmigo».
type Compartidos struct {
	Lista   []api.AlbumCompartido
	Nombres map[string]string
	AlAbrir func(api.AlbumCompartido)
	AlIr    func(Seccion)
}

// PantallaCompartidos es la lista: la nav, el título y las tarjetas.
func PantallaCompartidos(v Vista, d Compartidos) *fyne.Container {
	bloques := []ui.Bloque{
		{Obj: Nav(v, SecCompartidos, d.AlIr), Margen: SeparacionBloque},
		{Obj: ParrafoEn(v.Traduce("sharedWithMe.title"), tema.TextoLG, ui.SemiNegrita, "text", v.Variante), Margen: SeparacionBloque},
	}
	if cuerpo := CuerpoDeEstado(v, EstadoVacio(v, "sharedWithMe.empty", "", nil)); cuerpo != nil {
		bloques = append(bloques, ui.Bloque{Obj: cuerpo})
		return Envolver(v, false, ui.PilaBloques(bloques...))
	}
	tarjetas := make([]fyne.CanvasObject, len(d.Lista))
	for i, a := range d.Lista {
		tarjetas[i] = TarjetaCompartida(v, a, porDe(v, d.Nombres[a.Por]), d.AlAbrir)
	}
	r := ui.RejillaAlbumes()
	bloques = append(bloques, ui.Bloque{
		Obj:  container.New(r, tarjetas...),
		Alto: func(ancho float32) float32 { return r.MinSizeCon(tarjetas, ancho).Height },
	})
	return Envolver(v, false, ui.PilaBloques(bloques...))
}

// porDe es «Compartido por …», con «alguien de tu organización» si no se sabe quién.
func porDe(v Vista, nombre string) string {
	if nombre == "" {
		nombre = v.Traduce("sharePeople.someone")
	}
	return v.Traduce("sharedWithMe.by", i18n.Vars{"name": nombre})
}

// TarjetaCompartida es la de «Álbumes» con «N fotos · Compartido por …» debajo.
func TarjetaCompartida(v Vista, a api.AlbumCompartido, por string, alAbrir func(api.AlbumCompartido)) fyne.CanvasObject {
	portada := api.Photo{Name: a.Title}
	if a.CoverID != nil {
		portada.ID = *a.CoverID
	}
	m := NuevaMiniatura(portada, v.Teca)
	m.Rotulo, m.SinMarcas = a.Title, true
	if alAbrir != nil {
		m.AlPulsar = func() { alAbrir(a) }
	}
	titulo := ParrafoEn(a.Title, tema.TextoSM, ui.Medio, "text", v.Variante)
	pie := ParrafoEn(Recuento(v, "albums.count", a.Count)+" · "+por, tema.TextoXS, ui.Normal, "text-muted", v.Variante)
	return container.New(tarjetaAlbumLayout{}, m, titulo, pie)
}

// CompartidoAbierto es un álbum compartido conmigo, abierto.
type CompartidoAbierto struct {
	Titulo   string
	Fotos    []api.Photo
	Por      string
	AlAbrir  func(api.Photo)
	AlVolver func()
	AlIr     func(Seccion)
	// Editable: me lo compartieron para editar. Duenya es su nombre, para la pista.
	Editable    bool
	Duenya      string
	Fallo       string
	AlRenombrar func()
}

// PantallaCompartido es la cabecera —la vuelta, el título, el recuento, quién y «Solo
// lectura»— y las mismas filas justificadas que la galería.
func PantallaCompartido(v Vista, d CompartidoAbierto) *fyne.Container {
	titulo := ParrafoEn(d.Titulo, tema.TextoLG, ui.SemiNegrita, "text", v.Variante)
	izquierda := []fyne.CanvasObject{botonVolver(v, "sharedWithMe.title", d.AlVolver), titulo}
	var derecha []fyne.CanvasObject
	if d.Fotos != nil {
		izquierda = append(izquierda,
			ParrafoEn(Recuento(v, "albums.count", len(d.Fotos)), tema.TextoSM, ui.Normal, "text-muted", v.Variante),
			ParrafoEn(d.Por, tema.TextoSM, ui.Normal, "text-muted", v.Variante))
		insignia := "sharedWithMe.readOnly"
		if d.Editable {
			insignia = "sharePeople.write"
		}
		derecha = append(derecha, ui.NuevaInsignia(v.Traduce(insignia), ui.InsigniaNeutra))
		if d.Editable {
			derecha = append(derecha, ui.NuevoBoton(v.Traduce("sharedAlbum.rename"), d.AlRenombrar).
				Con(ui.Fantasma, ui.TallaSM).ConIcono(iconos.Lapiz, 0))
		}
	}
	grupo := container.New(ui.FilaAuto{Hueco: tema.Espacio(2), Derecha: len(derecha), Partir: true, SaltoFila: tema.Espacio(2)}, derecha...)
	cabecera := container.New(&cabeceraAlbum{titulo: titulo, completo: d.Titulo, derecha: grupo}, append(izquierda, grupo)...)
	bloques := []ui.Bloque{
		{Obj: Nav(v, SecCompartidos, d.AlIr), Margen: SeparacionBloque},
		{Obj: cabecera, Margen: SeparacionBloque},
	}
	if d.Editable && d.Fotos != nil {
		pista := v.Traduce("sharedAlbum.addHint", i18n.Vars{"name": d.Duenya, "action": v.Traduce("albums.addTo")})
		bloques = append(bloques, ui.Bloque{Obj: ParrafoEn(pista, tema.TextoSM, ui.Normal, "text-muted", v.Variante), Margen: SeparacionBloque})
	}
	if d.Fallo != "" {
		bloques = append(bloques, ui.Bloque{Obj: ParrafoEn(d.Fallo, tema.TextoSM, ui.Normal, "danger-text", v.Variante), Margen: SeparacionBloque})
	}
	if cuerpo := CuerpoDeEstado(v, EstadoVacio(v, "sharedWithMe.empty", "", nil)); cuerpo != nil {
		bloques = append(bloques, ui.Bloque{Obj: cuerpo})
		return Envolver(v, false, ui.PilaBloques(bloques...))
	}
	hijos := make([]fyne.CanvasObject, len(d.Fotos))
	for i, f := range d.Fotos {
		m := NuevaMiniatura(f, v.Teca)
		m.Rotulo, m.SinMarcas, m.Filete = f.Name, true, true
		if d.AlAbrir != nil {
			m.AlPulsar = func() { d.AlAbrir(f) }
		}
		hijos[i] = m
	}
	bloques = append(bloques, ui.Bloque{Obj: container.New(filasDe(v, d.Fotos), hijos...)})
	return Envolver(v, false, ui.PilaBloques(bloques...))
}

// ---- En marcha ----

// pedirVarianteCompartida es la puerta de las imágenes de «Compartido conmigo».
func (a *App) pedirVarianteCompartida(album func(id string) string) func(context.Context, string, string) ([]byte, error) {
	cli := a.cli
	return func(ctx context.Context, id, variante string) ([]byte, error) {
		r, err := cli.VarianteCompartida(ctx, album(id), id, variante)
		if err != nil {
			return nil, err
		}
		defer func() { _ = r.Close() }()
		return io.ReadAll(r)
	}
}

// pedirCompartidos trae la lista y quién compartió cada álbum.
func (a *App) pedirCompartidos() {
	a.cmp.portadas = map[string]string{}
	a.cmp.teca = NuevaFototeca()
	if a.cli != nil {
		portadas := a.cmp.portadas
		a.cmp.teca.Pedir = a.pedirVarianteCompartida(func(id string) string { return portadas[id] })
	}
	type par struct {
		lista   []api.AlbumCompartido
		nombres map[string]string
	}
	dir := a.Directorio
	pedir(a, func(x context.Context, c *api.Photos) (par, error) {
		lista, err := c.AlbumesCompartidos(x)
		if lista == nil {
			lista = []api.AlbumCompartido{}
		}
		return par{lista, nombresDe(x, dir, lista)}, err
	}, func(p par, err error) {
		a.cmp.cargando, a.cmp.fallo = false, err
		if err != nil {
			return
		}
		a.cmp.lista, a.cmp.nombres = p.lista, p.nombres
		for _, al := range p.lista {
			if al.CoverID != nil {
				a.cmp.portadas[*al.CoverID] = al.ID
			}
		}
	})
}

// nombresDe pone cara a quien compartió cada álbum, con el directorio.
func nombresDe(x context.Context, dir *api.Directorio, lista []api.AlbumCompartido) map[string]string {
	nombres := map[string]string{}
	if dir == nil {
		return nombres
	}
	for _, al := range lista {
		if _, ya := nombres[al.Por]; ya {
			continue
		}
		if p, err := dir.PorID(x, al.Por); err == nil {
			nombres[al.Por] = cmpOr(p.Nombre, p.Email)
		}
	}
	return nombres
}

func cmpOr(s ...string) string {
	for _, x := range s {
		if x != "" {
			return x
		}
	}
	return ""
}

func (a *App) datosCompartidos() Compartidos {
	return Compartidos{
		Lista: a.cmp.lista, Nombres: a.cmp.nombres,
		AlAbrir: func(al api.AlbumCompartido) { a.AbrirCompartido(al.ID) },
		AlIr:    a.irSeccion,
	}
}

// AbrirCompartido abre un álbum compartido conmigo (`/shared?album=`).
func (a *App) AbrirCompartido(id string) {
	a.cancelarFotos()
	teca, grande := NuevaFototeca(), NuevaFototeca()
	grande.Variante = VarianteGrande
	if a.cli != nil {
		pedirV := a.pedirVarianteCompartida(func(string) string { return id })
		teca.Pedir, grande.Pedir = pedirV, pedirV
	}
	// La foto abierta es la del enlace: reutiliza su estado y su capa.
	a.pub = estadoPublico{teca: teca, grande: grande}
	a.cmp.album, a.cmp.por, a.cmp.editable, a.cmp.falloEdicion = id, "", false, ""
	a.Ir(RutaCompartido)
}

func (a *App) pedirCompartido() {
	a.cerrarPublica()
	a.pub.estadoCarga = estadoCarga{cargando: true}
	a.pintar()
	id, dir := a.cmp.album, a.Directorio
	type par struct {
		al    api.AlbumCompartido
		fotos []api.Photo
		por   string
	}
	pedir(a, func(x context.Context, c *api.Photos) (par, error) {
		al, fotos, err := c.AlbumCompartidoConmigo(x, id)
		if fotos == nil {
			fotos = []api.Photo{}
		}
		return par{al, fotos, nombresDe(x, dir, []api.AlbumCompartido{al})[al.Por]}, err
	}, func(p par, err error) {
		a.pub.cargando, a.pub.fallo = false, err
		if err != nil {
			return
		}
		a.pub.album = api.PublicAlbum{Title: p.al.Title, Count: len(p.fotos), Items: p.fotos}
		a.cmp.por, a.cmp.editable = p.por, p.al.Permiso == api.NivelEditar
	})
}

func (a *App) pantallaCompartido() *fyne.Container {
	p := a.pub
	v := a.vistaDe(p.estadoCarga, p.album.Items == nil, len(p.album.Items) == 0)
	v.Teca, v.TecaGrande = p.teca, p.grande
	duenya := a.cmp.por
	if duenya == "" {
		duenya = a.v.Traduce("sharePeople.someone")
	}
	return PantallaCompartido(v, CompartidoAbierto{
		Titulo: p.album.Title, Fotos: p.album.Items, Por: porDe(a.v, a.cmp.por),
		AlAbrir: a.verPublica, AlVolver: func() { a.Ir(RutaCompartidos) }, AlIr: a.irSeccion,
		Editable: a.cmp.editable, Duenya: duenya, Fallo: a.cmp.falloEdicion, AlRenombrar: a.renombrarCompartido,
	})
}

// renombrarCompartido es «Renombrar» de un álbum compartido conmigo para editar: el
// `usePrompt` de la web —el mensaje de título, «Guardar»— con el título de ahora escrito.
func (a *App) renombrarCompartido() {
	id, antes := a.cmp.album, a.pub.album.Title
	pregunta := a.v.Traduce("sharedAlbum.renamePrompt")
	a.preguntar(pregunta, pregunta, a.v.Traduce("ui.save"), antes, func(titulo string) {
		if titulo = strings.TrimSpace(titulo); titulo != "" && titulo != antes {
			a.renombrarCompartidoA(id, titulo)
		}
	})
}

// renombrarCompartidoA le pone ese título.
func (a *App) renombrarCompartidoA(id, titulo string) {
	pedir(a, func(x context.Context, c *api.Photos) (struct{}, error) {
		return struct{}{}, c.RenameAlbum(x, id, titulo)
	},
		func(_ struct{}, err error) {
			if err != nil {
				a.cmp.falloEdicion = a.textoDeFallo(err)
				return
			}
			a.pub.album.Title = titulo
		})
}

// quitarCompartida es «Quitar del álbum» del visor de un álbum compartido para editar.
func (a *App) quitarCompartida(f api.Photo) {
	id := a.cmp.album
	pedir(a, func(x context.Context, c *api.Photos) (struct{}, error) {
		return struct{}{}, c.RemoveFromAlbum(x, id, f.ID)
	},
		func(_ struct{}, err error) {
			if err != nil {
				a.cmp.falloEdicion = a.textoDeFallo(err)
				return
			}
			quedan := a.pub.album.Items[:0:0]
			for _, x := range a.pub.album.Items {
				if x.ID != f.ID {
					quedan = append(quedan, x)
				}
			}
			a.pub.album.Items, a.pub.album.Count = quedan, len(quedan)
			a.cerrarPublica()
		})
}

// Medidas de la cabecera de `VisorCompartido`: `border-b px-3 py-2 gap-2`.
var (
	pxCabeceraVisor = tema.Espacio(3)
	pyCabeceraVisor = tema.Espacio(2)
)

// VisorCompartido es la foto de un álbum compartido conmigo: la cabecera de
// `VisorCompartido` de la web —el nombre, «Quitar del álbum» si puedo editar y cerrar—
// sobre la foto sola del enlace, con las flechas del visor a los lados.
func VisorCompartido(v Vista, f api.Photo, aTamanyoReal bool, alZoom, alAnterior, alSiguiente, alCerrar func(), alQuitar func(api.Photo)) fyne.CanvasObject {
	nombre := ParrafoEn(f.Name, tema.TextoSM, ui.Medio, "text", v.Variante)
	var mandos []fyne.CanvasObject
	if alQuitar != nil {
		mandos = append(mandos, botonQuitarDelAlbum(v, func() { alQuitar(f) }))
	}
	icono := func(i iconos.Icono, clave string, fn func()) *ui.BotonIcono {
		b := ui.NuevoBotonIcono(i, v.Traduce(clave), fn)
		b.Lado, b.TamIcono, b.Inerte = ui.LadoIconoLG, IconoMando, fn == nil // `size="lg"`, como el visor
		return b
	}
	// Anterior y siguiente van sobre la foto, las del visor de la galería (visorSuelto).
	mandos = append(mandos, icono(iconos.Cerrar, "viewer.close", alCerrar))
	fila := container.NewBorder(nil, nil, nil, container.New(ui.Fila{Hueco: tema.Espacio(2)}, mandos...), nombre)
	fondo := canvas.NewRectangle(tema.Color("bg", v.Variante))
	filete := canvas.NewRectangle(tema.Color("border", v.Variante))
	filete.SetMinSize(fyne.NewSize(0, tema.Borde))
	cabecera := container.NewStack(fondo, container.NewBorder(nil, filete, nil, nil,
		container.New(ui.Margen{Izq: pxCabeceraVisor, Der: pxCabeceraVisor, Arr: pyCabeceraVisor, Abj: pyCabeceraVisor}, fila)))
	return container.NewBorder(cabecera, nil, nil, nil, visorSuelto(v, f, aTamanyoReal, alZoom, alAnterior, alSiguiente))
}

// compartirConPersonas abre el diálogo común sobre el álbum abierto: ver o editar.
func (a *App) compartirConPersonas() {
	if a.cli == nil || a.w == nil || a.Directorio == nil {
		return
	}
	compartir.Abrir(a.w.Canvas(), compartir.Opciones{
		T: a.v.T, V: a.v.Variante, Nombre: a.uno.album.Title,
		Niveles:    []string{api.NivelVer, api.NivelEditar},
		Personas:   a.cli.PersonasDelAlbum(a.uno.id),
		Directorio: a.Directorio,
		Avisar: func(_ bool, texto string) {
			a.uno.aviso = texto
			a.pintar()
		},
	})
}

// iconoCompartirConPersonas es el `users` del botón, como la web.
var iconoCompartirConPersonas = iconos.Usuarios
