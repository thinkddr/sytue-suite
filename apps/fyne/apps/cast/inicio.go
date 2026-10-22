// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// paridad-web: apps/frontend/apps/cast/src/pages/index.astro
// paridad-web: apps/frontend/apps/cast/src/pages/explore.astro
// paridad-web: apps/frontend/apps/cast/src/pages/c/[org].astro

package cast

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// ---- Inicio ----

// Inicio son los datos de la portada con sesión: lo mío, lo que está en directo en la
// organización, lo compartido conmigo y lo último de la organización.
type Inicio struct {
	Mios         []api.CastVideo
	EnDirecto    []api.CastVideo
	Compartidos  []api.CastVideo
	Organizacion []api.CastVideo
	// Busqueda es el campo que busca en lo mío; Texto, lo escrito.
	Busqueda *ui.Entrada
	Texto    string
	// HayMas y CargandoMas son el «Mostrar más» de lo mío.
	HayMas, CargandoMas bool
	AlMas               func()

	AlAbrir  func(api.CastVideo)
	AlSubir  func()
	AlEmitir func()
}

// PantallaInicio monta «Inicio».
func PantallaInicio(v Vista, d Inicio) *fyne.Container {
	subir := ui.NuevoBoton(v.Traduce("nav.upload"), d.AlSubir).Con(ui.Primario, ui.TallaSM).ConIcono(iconos.Subir, iconos.TallaSM)
	emitir := ui.NuevoBoton(v.Traduce("nav.studio"), d.AlEmitir).Con(ui.Secundario, ui.TallaSM)
	acciones := []fyne.CanvasObject{emitir, subir}
	if v.Estado == SinSesion || v.Ventana < ui.CorteSM {
		// Sin sesión no hay nada que subir ni emitir: el botón es «Entrar». Y en un
		// teléfono (`hidden sm:flex`) subir es el botón flotante y emitir, el menú.
		acciones = nil
	}
	bloques := []ui.Bloque{{
		Obj:    Cabecera(v.Traduce("home.title"), "", acciones...),
		Margen: HuecoSeccion,
	}}
	vacio := Vacio(v, glifoVideo, v.Traduce("home.empty.title"), v.Traduce("home.empty.description"),
		ui.NuevoBoton(v.Traduce("nav.upload"), d.AlSubir).Con(ui.Primario, ui.TallaSM))
	if cuerpo := CuerpoDeEstado(v, vacio); cuerpo != nil {
		return Pagina(v, append(bloques, ui.Bloque{Obj: cuerpo})...)
	}
	bloques = append(bloques, seccionVideos(v, v.Traduce("home.liveNow"), d.EnDirecto, d.AlAbrir)...)
	// «Mis vídeos» con su buscador a la derecha del título.
	// Por debajo de sm (`w-full sm:w-64`), el buscador baja a su propia fila, a todo el
	// ancho.
	var buscador []fyne.CanvasObject
	estrecho := v.Ventana < ui.CorteSM
	if d.Busqueda != nil && !estrecho {
		buscador = append(buscador, container.New(&anchoFijo{ancho: 16 * tema.Rem}, d.Busqueda))
	}
	bloques = append(bloques, ui.Bloque{Obj: TituloSeccion(v, v.Traduce("home.mine"), buscador...), Margen: HuecoTitulo})
	if d.Busqueda != nil && estrecho {
		bloques = append(bloques, ui.Bloque{Obj: d.Busqueda, Margen: HuecoTitulo})
	}
	switch {
	case len(d.Mios) > 0:
		bloques = append(bloques, ui.Bloque{Obj: Tarjetas(v, d.Mios, true, d.AlAbrir), Margen: HuecoSeccion})
		if d.HayMas {
			bloques[len(bloques)-1].Margen = tema.Espacio(4)
			bloques = append(bloques, ui.Bloque{Obj: botonMasCon(v, "home.more", d.CargandoMas, d.AlMas), Margen: HuecoSeccion})
		}
	case d.Texto != "":
		bloques = append(bloques, ui.Bloque{Obj: Parrafo(v, v.Traduce("home.noMatch", i18n.Vars{"query": d.Texto}), tema.TextoSM, ui.Normal, "text-muted"), Margen: HuecoSeccion})
	default:
		bloques = append(bloques, ui.Bloque{Obj: Parrafo(v, v.Traduce("home.empty.description"), tema.TextoSM, ui.Normal, "text-muted"), Margen: HuecoSeccion})
	}
	bloques = append(bloques, seccionVideos(v, v.Traduce("home.shared"), d.Compartidos, d.AlAbrir)...)
	bloques = append(bloques, seccionVideos(v, v.Traduce("home.org"), d.Organizacion, d.AlAbrir)...)
	return Pagina(v, bloques...)
}

// ---- Explorar ----

// Explorar son los datos de la portada pública.
type Explorar struct {
	Directos, Videos []api.CastVideo
	// Buscador es el campo; vive en App para no perder el foco al repintar.
	Buscador *ui.Entrada
	// Texto filtra lo cargado por título y autor.
	Texto string
	// HayMas y CargandoMas son el «Cargar más» del final.
	HayMas, CargandoMas bool

	AlMas   func()
	AlAbrir func(api.CastVideo)
}

// Filtrar deja los vídeos cuyo título o autor contienen el texto, sin mayúsculas.
//
// ponytail: filtra lo ya cargado; `/public/explore` no busca en el servicio. El relevo
// es un `q=` en la portada pública cuando haya catálogo para que merezca la pena.
func Filtrar(vs []api.CastVideo, texto string) []api.CastVideo {
	texto = buscar.Plegar(strings.TrimSpace(texto))
	if texto == "" {
		return vs
	}
	var out []api.CastVideo
	for _, x := range vs {
		if strings.Contains(buscar.Plegar(x.Title), texto) || strings.Contains(buscar.Plegar(x.Owner.Name), texto) {
			out = append(out, x)
		}
	}
	return out
}

// PantallaExplorar monta «Explorar».
func PantallaExplorar(v Vista, d Explorar) *fyne.Container {
	cabecera := ui.PilaBloques(
		ui.Bloque{Obj: Parrafo(v, v.Traduce("explore.title"), ui.EscalonTitulo(2), ui.PesoTitulo(2), "text"), Margen: tema.Espacio(2)},
		ui.Bloque{Obj: Parrafo(v, v.Traduce("explore.subtitle"), tema.TextoSM, ui.Normal, "text-muted")},
	)
	bloques := []ui.Bloque{{
		Obj:    cabecera,
		Margen: HuecoSeccion,
	}}
	if d.Buscador != nil {
		if v.Ventana < ui.CorteLG {
			bloques[0].Margen = tema.Espacio(4)
			bloques = append(bloques, ui.Bloque{Obj: d.Buscador, Margen: HuecoSeccion})
		} else {
			buscador := container.New(&anchoFijo{ancho: 18 * tema.Rem}, d.Buscador)
			bloques[0].Obj = container.New(ui.FilaAuto{Hueco: tema.Espacio(4), Derecha: 1, Elastico: 1, Partir: true, SaltoFila: tema.Espacio(4)}, cabecera, buscador)
		}
	}
	vacio := Vacio(v, glifoVideo, v.Traduce("explore.empty.title"), v.Traduce("explore.empty.description"), nil)
	if cuerpo := CuerpoDeEstado(v, vacio); cuerpo != nil {
		return Pagina(v, append(bloques, ui.Bloque{Obj: cuerpo})...)
	}
	directos, videos := Filtrar(d.Directos, d.Texto), Filtrar(d.Videos, d.Texto)
	if len(directos)+len(videos) == 0 {
		return Pagina(v, append(bloques, ui.Bloque{Obj: Vacio(v, glifoVideo,
			v.Traduce("explore.noMatch", i18n.Vars{"query": d.Texto}), "", nil)})...)
	}
	bloques = append(bloques, seccionVideos(v, v.Traduce("explore.live"), directos, d.AlAbrir)...)
	bloques = append(bloques, seccionVideos(v, v.Traduce("explore.videos"), videos, d.AlAbrir)...)
	if d.HayMas {
		bloques = append(bloques, ui.Bloque{Obj: botonMas(v, d.CargandoMas, d.AlMas)})
	}
	return Pagina(v, bloques...)
}

// botonMas es el «Cargar más» centrado del final de un listado.
func botonMas(v Vista, cargando bool, alMas func()) fyne.CanvasObject {
	return botonMasCon(v, "explore.more", cargando, alMas)
}

// botonMasCon es botonMas con su rótulo («Mostrar más», «Cargar más»).
func botonMasCon(v Vista, clave string, cargando bool, alMas func()) fyne.CanvasObject {
	if cargando {
		clave = "ui.loading"
	}
	b := ui.NuevoBoton(v.Traduce(clave), alMas).Con(ui.Secundario, ui.TallaSM)
	b.Inerte = cargando
	return container.New(&centrado{}, b)
}

// centrado pone su hijo a su tamaño en el centro de la fila.
type centrado struct{}

func (centrado) MinSize(objs []fyne.CanvasObject) fyne.Size { return objs[0].MinSize() }

func (centrado) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	m := objs[0].MinSize()
	objs[0].Move(fyne.NewPos((t.Width-m.Width)/2, (t.Height-m.Height)/2))
	objs[0].Resize(m)
}

// anchoFijo da a su hijo un ancho fijo y su alto mínimo (un buscador de `w-64`).
type anchoFijo struct{ ancho float32 }

func (a *anchoFijo) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(a.ancho, objs[0].MinSize().Height)
}

func (a *anchoFijo) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(t)
}

// ---- Canal ----

// Canal son los datos del canal público de una organización.
type Canal struct {
	Org, Nombre string
	Videos      []api.CastVideo
	HayMas      bool
	CargandoMas bool

	AlMas   func()
	AlAbrir func(api.CastVideo)
	// AlDenunciar abre la denuncia del canal. nil: no se ofrece.
	AlDenunciar func()
}

// PantallaCanal monta el canal: la cabecera con el nombre y el recuento, y la rejilla.
func PantallaCanal(v Vista, d Canal) *fyne.Container {
	nombre := d.Nombre
	if nombre == "" {
		nombre = d.Org
	}
	// El recuento solo cuando se tienen todos (ChannelHeader con `cuantos` null si no).
	ayuda := ""
	if !d.HayMas {
		ayuda = v.Traduce("channel.count", i18n.Vars{"count": len(d.Videos)})
	}
	cabecera := cabeceraCanal(v, nombre, ayuda)
	if d.AlDenunciar != nil {
		// «Denunciar» del canal: cualquiera puede, también sin sesión.
		denunciar := ui.NuevoBoton(v.Traduce("report.action"), d.AlDenunciar).Con(ui.Fantasma, ui.TallaSM).ConIcono(iconos.Peligro, iconos.TallaSM)
		if v.Ventana < ui.CorteSM {
			cabecera = ui.PilaBloques(ui.Bloque{Obj: cabecera, Margen: tema.Espacio(4)}, ui.Bloque{Obj: Fila(denunciar)})
		} else {
			cabecera = container.New(ui.FilaAuto{Hueco: HuecoFila, Derecha: 1, Elastico: 1, Partir: true, SaltoFila: tema.Espacio(4)}, cabecera, denunciar)
		}
	}
	bloques := []ui.Bloque{{Obj: cabecera, Margen: HuecoSeccion}}
	vacio := Vacio(v, glifoVideo, v.Traduce("channel.empty.title"), v.Traduce("channel.empty.description"), nil)
	if cuerpo := CuerpoDeEstado(v, vacio); cuerpo != nil {
		return Pagina(v, append(bloques, ui.Bloque{Obj: cuerpo})...)
	}
	bloques = append(bloques,
		ui.Bloque{Obj: Tarjetas(v, d.Videos, false, d.AlAbrir), Margen: HuecoSeccion},
	)
	if d.HayMas {
		bloques = append(bloques, ui.Bloque{Obj: botonMasCon(v, "channel.more", d.CargandoMas, d.AlMas)})
	}
	return Pagina(v, bloques...)
}

// cabeceraCanal es el ChannelHeader: el avatar de la organización, su nombre en
// `sytue-h2` —con el peso y la escala de la forma— y la línea de ayuda en `text-sm`.
func cabeceraCanal(v Vista, nombre, ayuda string) fyne.CanvasObject {
	avatar := ui.NuevoAvatar(nombre).Con(ui.AvatarLG)
	textos := []ui.Bloque{{Obj: ui.NuevoParrafo(nombre, ui.EscalonTitulo(2), ui.PesoTitulo(2), "text", v.Variante)}}
	if ayuda != "" {
		textos = append(textos, ui.Bloque{Obj: ui.NuevoParrafo(ayuda, tema.TextoSM, ui.Normal, "text-muted", v.Variante)})
	}
	return container.New(&conAvatar{}, avatar, ui.PilaBloques(textos...))
}

// conAvatar: el avatar a la izquierda, centrado con el bloque de texto.
type conAvatar struct{}

func (conAvatar) MinSize(objs []fyne.CanvasObject) fyne.Size {
	a, t := objs[0].MinSize(), objs[1].MinSize()
	return fyne.NewSize(a.Width+HuecoTitulo+t.Width, max(a.Height, t.Height))
}

// AltoCon: el del texto al ancho que queda, o el del avatar.
func (conAvatar) AltoCon(objs []fyne.CanvasObject, ancho float32) float32 {
	a := objs[0].MinSize()
	return max(a.Height, ui.AltoA(objs[1], ancho-a.Width-HuecoTitulo))
}

func (c conAvatar) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	a := objs[0].MinSize()
	objs[0].Move(fyne.NewPos(0, (t.Height-a.Height)/2))
	objs[0].Resize(a)
	w := t.Width - a.Width - HuecoTitulo
	h := ui.AltoA(objs[1], w)
	objs[1].Move(fyne.NewPos(a.Width+HuecoTitulo, (t.Height-h)/2))
	objs[1].Resize(fyne.NewSize(w, h))
}
