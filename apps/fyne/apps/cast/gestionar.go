// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// paridad-web: apps/frontend/apps/cast/src/pages/manage/[id].astro

package cast

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Gestionar son los datos de la pantalla de gestionar un vídeo.
type Gestionar struct {
	Video api.CastVideo
	// Titulo, Descripcion y Persona son los campos; viven en App.
	Titulo      *ui.Entrada
	Descripcion *ui.AreaTexto
	Persona     *ui.Entrada
	// Visor es el reproductor para elegir el fotograma del póster; nil si el vídeo aún
	// no se puede ver (entonces se enseña el póster).
	Visor fyne.CanvasObject

	Personas     []api.CastPerson
	ErrorPersona string
	// Guardando: se guardan título y descripción. Cambiado: difieren de lo guardado.
	Guardando, Cambiado bool
	// PosterOcupado: se sube una imagen o se pide un fotograma.
	PosterOcupado bool
	// Calidades es la tarjeta de calidades (ManageQualities.tsx); nil, no se enseña
	// (un directo programado o en curso).
	Calidades *TarjetaCalidades

	// AlVisibilidad y los tres interruptores guardan al momento, como la web.
	AlVisibilidad              func(string)
	AlComentarios, AlChat      func(bool)
	AlDescargas                func(bool)
	AlAnadirPersona            func()
	AlQuitarPersona            func(api.CastPerson)
	AlSubirPoster, AlFotograma func()
	AlGuardar, AlBorrar        func()
	AlVer                      func()
}

// PantallaGestionar monta «Gestionar» como ManageApp.tsx: a la izquierda los detalles
// (con Guardar) y quién puede verlo; a la derecha, de 24rem, el póster con su
// reproductor, las opciones y borrar.
func PantallaGestionar(v Vista, d Gestionar) *fyne.Container {
	vid := d.Video
	ver := ui.NuevoBoton(v.Traduce("manage.view"), d.AlVer).Con(ui.Secundario, ui.TallaMD).ConIcono(iconos.FlechaArribaDerecha, iconos.TallaSM)
	if cuerpo := CuerpoDeEstado(v, nil); cuerpo != nil {
		return Pagina(v, ui.Bloque{Obj: Cabecera(v.Traduce("manage.title"), ""), Margen: HuecoSeccion}, ui.Bloque{Obj: cuerpo})
	}
	hueco := tema.Espacio(6)
	bloques := []ui.Bloque{{Obj: Cabecera(v.Traduce("manage.title"), vid.Title, ver), Margen: hueco}}
	if vid.Status == api.CastStatusFailed && vid.Failure != "" {
		bloques = append(bloques, ui.Bloque{Obj: alertaPeligro(v, v.Traduce("manage.failure", i18n.Vars{"reason": vid.Failure})), Margen: hueco})
	}

	guardar := ui.NuevoBoton(v.Traduce("ui.save"), d.AlGuardar).Con(ui.Primario, ui.TallaMD)
	guardar.Cargando = d.Guardando
	guardar.Inerte = d.Guardando || !d.Cambiado || d.Titulo == nil || strings.TrimSpace(d.Titulo.Texto()) == ""
	detalles := Recuadro(v, v.Traduce("manage.details"), ui.PilaBloques(
		ui.Bloque{Obj: ui.GrupoCampo(v.Traduce("manage.titleField"), true, d.Titulo, "", ""), Margen: tema.Espacio(4)},
		ui.Bloque{Obj: ui.GrupoCampo(v.Traduce("manage.descriptionField"), false, d.Descripcion, "", ""), Margen: tema.Espacio(4)},
		ui.Bloque{Obj: container.New(ui.FilaAuto{}, guardar)},
	))
	visibilidad := []ui.Bloque{{Obj: SelectorVisibilidad(v, vid.Visibility, d.AlVisibilidad)}}
	if vid.Visibility == api.CastPeople {
		visibilidad[0].Margen = tema.Espacio(4)
		visibilidad = append(visibilidad, ui.Bloque{Obj: selectorPersonas(v, d)})
	}
	acceso := Recuadro(v, "", ui.PilaBloques(visibilidad...))
	opciones := []ui.Bloque{{Obj: conmutador(v, "manage.comments", vid.CommentsEnabled, d.AlComentarios), Margen: tema.Espacio(3)}}
	if vid.Kind == "live" {
		opciones = append(opciones, ui.Bloque{Obj: conmutador(v, "manage.chat", vid.ChatEnabled, d.AlChat), Margen: tema.Espacio(3)})
	}
	opciones = append(opciones, ui.Bloque{Obj: conmutador(v, "manage.download", vid.DownloadEnabled, d.AlDescargas)})
	interaccion := Recuadro(v, v.Traduce("manage.options"), ui.PilaBloques(opciones...))
	poster := Recuadro(v, v.Traduce("manage.poster"), bloquePoster(v, d))
	borrar := container.New(ui.FilaAuto{Derecha: 1},
		ui.NuevoBoton(v.Traduce("manage.delete"), d.AlBorrar).Con(ui.Peligro, ui.TallaMD).ConIcono(iconos.Papelera, iconos.TallaSM))

	izquierda := ui.PilaBloques(
		ui.Bloque{Obj: detalles, Margen: hueco},
		ui.Bloque{Obj: acceso},
	)
	derechaB := []ui.Bloque{{Obj: poster, Margen: hueco}}
	if d.Calidades != nil {
		derechaB = append(derechaB, ui.Bloque{Obj: tarjetaCalidades(v, *d.Calidades), Margen: hueco})
	}
	derecha := ui.PilaBloques(append(derechaB,
		ui.Bloque{Obj: interaccion, Margen: hueco},
		ui.Bloque{Obj: borrar},
	)...)
	if v.Ventana < ui.CorteLG {
		bloques = append(bloques, ui.Bloque{Obj: izquierda, Margen: hueco}, ui.Bloque{Obj: derecha})
	} else {
		bloques = append(bloques, ui.Bloque{Obj: container.New(&dosColumnas{derecha: 24 * tema.Rem}, izquierda, derecha)})
	}
	return Pagina(v, bloques...)
}

// alertaPeligro es el `<p role="alert">` de peligro: `bg-danger-bg text-danger-text
// rounded-md px-4 py-3 text-sm` con el glifo delante.
func alertaPeligro(v Vista, texto string) fyne.CanvasObject {
	caja := ui.NuevaCaja(tema.Color("danger-bg", v.Variante), transparente(), tema.RadioMD)
	glifo := iconos.Trazo(iconos.Peligro, 16, "danger-text", v.Variante).Objeto()
	p := Parrafo(v, texto, tema.TextoSM, ui.Normal, "danger-text")
	fila := container.New(&glifoYTexto{}, glifo, p)
	return container.New(&recuadroFino{caja: caja}, append(caja.Objetos(), fila)...)
}

// glifoYTexto: el glifo arriba a la izquierda (`items-start gap-2`) y el párrafo al
// lado, a lo que quede.
type glifoYTexto struct{}

func (glifoYTexto) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(16+tema.Espacio(2), max(16, objs[1].MinSize().Height))
}

// AltoCon es el del párrafo al ancho que le queda.
func (glifoYTexto) AltoCon(objs []fyne.CanvasObject, ancho float32) float32 {
	return max(16, ui.AltoA(objs[1], ancho-16-tema.Espacio(2)))
}

func (g glifoYTexto) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, (tema.TextoSM.Linea-16)/2))
	objs[0].Resize(fyne.NewSize(16, 16))
	w := t.Width - 16 - tema.Espacio(2)
	objs[1].Move(fyne.NewPos(16+tema.Espacio(2), 0))
	objs[1].Resize(fyne.NewSize(w, ui.AltoA(objs[1], w)))
}

// conmutador es un interruptor con su rótulo y su ayuda debajo.
func conmutador(v Vista, clave string, encendido bool, al func(bool)) fyne.CanvasObject {
	c := ui.NuevoConmutador(v.Traduce(clave), al)
	c.Encendido = encendido
	return container.New(ui.FilaAuto{Derecha: 1}, c)
}

// selectorPersonas es el PeoplePicker: el campo para «@usuario», nombre o correo, y
// las personas que ya tienen acceso como fichas con su aspa.
func selectorPersonas(v Vista, d Gestionar) fyne.CanvasObject {
	anadir := ui.NuevoBoton(v.Traduce("manage.people.add"), d.AlAnadirPersona).Con(ui.Secundario, ui.TallaSM)
	campo := container.New(ui.FilaAuto{Hueco: HuecoFila, Derecha: 2, Elastico: 1, Partir: true}, d.Persona, anadir)
	// La ayuda va aparte, como párrafo: la de GrupoCampo es una línea que no se parte, y
	// a 360 de ancho se salía de la pantalla (TestLasPantallasEnElMovil).
	bloques := []ui.Bloque{{Obj: ui.GrupoCampo(v.Traduce("manage.people.label"), false, campo, "", d.ErrorPersona), Margen: tema.Espacio(3)}}
	if d.ErrorPersona == "" {
		bloques[0].Margen = ui.HuecoGrupoCampo
		bloques = append(bloques, ui.Bloque{Obj: Parrafo(v, v.Traduce("visibility.people.description"), tema.TextoXS, ui.Normal, "text-muted"), Margen: tema.Espacio(3)})
	}
	if len(d.Personas) == 0 {
		bloques = append(bloques, ui.Bloque{Obj: Parrafo(v, v.Traduce("manage.people.empty"), tema.TextoXS, ui.Normal, "text-muted")})
		return ui.PilaBloques(bloques...)
	}
	fichas := make([]fyne.CanvasObject, 0, len(d.Personas))
	for _, p := range d.Personas {
		fichas = append(fichas, fichaPersona(v, p, d.AlQuitarPersona))
	}
	bloques = append(bloques, ui.Bloque{Obj: container.New(ui.FilaAuto{Hueco: HuecoFila, Derecha: len(fichas), Partir: true, SaltoFila: HuecoFila}, fichas...)})
	return ui.PilaBloques(bloques...)
}

// fichaPersona es un chip: avatar, nombre, @usuario y el aspa de quitar.
func fichaPersona(v Vista, p api.CastPerson, alQuitar func(api.CastPerson)) fyne.CanvasObject {
	nombre := p.Name
	if p.Username != "" {
		nombre += " · @" + p.Username
	}
	quitar := ui.NuevoBotonIcono(iconos.Cerrar, v.Traduce("manage.people.remove", i18n.Vars{"name": p.Name}), func() {
		if alQuitar != nil {
			alQuitar(p)
		}
	})
	quitar.Lado = ui.LadoIconoSM
	caja := ui.NuevaCaja(tema.Color("bg", v.Variante), tema.Color("border", v.Variante), tema.RadioPill)
	fila := container.New(ui.Fila{Hueco: tema.Espacio(1.5)},
		ui.NuevoAvatar(p.Name).Con(ui.AvatarXS),
		container.New(&centradoV{alto: ui.LadoIconoSM}, ui.Texto(nombre, tema.TextoXS, ui.Medio, tema.Color("text", v.Variante))),
		quitar,
	)
	return container.New(&ficha{caja: caja}, append(caja.Objetos(), fila)...)
}

// ficha es la píldora de una persona: `pl-1 pr-1 py-0.5`.
type ficha struct{ caja *ui.Caja }

func (f *ficha) MinSize(objs []fyne.CanvasObject) fyne.Size {
	m := objs[len(objs)-1].MinSize()
	return fyne.NewSize(m.Width+2*tema.Espacio(1), m.Height+2*tema.Espacio(0.5))
}

func (f *ficha) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	f.caja.Redimensionar(t)
	c := objs[len(objs)-1]
	m := c.MinSize()
	c.Move(fyne.NewPos(tema.Espacio(1), (t.Height-m.Height)/2))
	c.Resize(m)
}

// centradoV centra un texto en una fila de ese alto.
type centradoV struct{ alto float32 }

func (c *centradoV) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(objs[0].MinSize().Width, c.alto)
}

func (c *centradoV) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	if tx, ok := objs[0].(interface{ MinSize() fyne.Size }); ok {
		m := tx.MinSize()
		objs[0].Move(fyne.NewPos(0, (t.Height-m.Height)/2))
		objs[0].Resize(m)
	}
}

// bloquePoster es la tarjeta del póster: el reproductor para ir al fotograma (o, si
// el vídeo aún no se puede ver, el póster actual), la ayuda y las dos maneras de
// cambiarlo: «Usar este fotograma» y subir una imagen.
func bloquePoster(v Vista, d Gestionar) fyne.CanvasObject {
	var cuadro fyne.CanvasObject
	if d.Visor != nil {
		cuadro = d.Visor
	} else {
		tarjeta := NuevaTarjeta(v, d.Video, true, nil)
		tarjeta.SinPie = true
		cuadro = container.New(&soloPoster{}, tarjeta)
	}
	var botones []fyne.CanvasObject
	if d.Visor != nil {
		usar := ui.NuevoBoton(v.Traduce("manage.posterFrame"), d.AlFotograma).Con(ui.Secundario, ui.TallaMD)
		usar.Cargando = d.PosterOcupado
		botones = append(botones, usar)
	}
	subir := ui.NuevoBoton(v.Traduce("manage.posterUpload"), d.AlSubirPoster).Con(ui.Fantasma, ui.TallaMD).ConIcono(iconos.Imagen, iconos.TallaSM)
	subir.Inerte = d.PosterOcupado
	botones = append(botones, subir)
	return ui.PilaBloques(
		ui.Bloque{Obj: cuadro, Margen: tema.Espacio(3)},
		ui.Bloque{Obj: Parrafo(v, v.Traduce("manage.posterHint"), tema.TextoXS, ui.Normal, "text-muted"), Margen: tema.Espacio(3)},
		ui.Bloque{Obj: Fila(botones...)},
	)
}

// soloPoster enseña de la tarjeta solo el póster (el título ya está en la página).
type soloPoster struct{}

func (soloPoster) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, 0) }

// AltoCon es el 16:9 del ancho.
func (soloPoster) AltoCon(_ []fyne.CanvasObject, ancho float32) float32 { return ancho * 9 / 16 }

func (soloPoster) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(t)
}

// dosColumnas: la izquierda se lleva lo que deja la derecha, de ancho fijo, con
// `gap-6`; el alto es el de la más alta.
type dosColumnas struct{ derecha float32 }

func (c *dosColumnas) anchos(total float32) (izq, der float32) {
	return total - c.derecha - tema.Espacio(6), c.derecha
}

func (c *dosColumnas) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(2*c.derecha, max(objs[0].MinSize().Height, objs[1].MinSize().Height))
}

// AltoCon es el de la columna más alta.
func (c *dosColumnas) AltoCon(objs []fyne.CanvasObject, ancho float32) float32 {
	i, d := c.anchos(ancho)
	return max(ui.AltoA(objs[0], i), ui.AltoA(objs[1], d))
}

func (c *dosColumnas) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	i, d := c.anchos(t.Width)
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(fyne.NewSize(i, ui.AltoA(objs[0], i)))
	objs[1].Move(fyne.NewPos(i+tema.Espacio(6), 0))
	objs[1].Resize(fyne.NewSize(d, ui.AltoA(objs[1], d)))
}
