// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// paridad-web: apps/frontend/apps/cast/src/pages/watch/[id].astro

package cast

import (
	"slices"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Ver son los datos de la página de un vídeo.
type Ver struct {
	Video api.CastVideo
	// Visor es el reproductor, que vive en App (ver visor.go).
	Visor fyne.CanvasObject
	// Descargar: el servicio dio `download_url` (descarga activada, o es el dueño).
	Descargar bool

	Comentarios Comentarios
	// Chat va en un directo; nil en un vídeo.
	Chat *Chat

	// DescripcionAbierta es el «Mostrar más» de una descripción larga.
	DescripcionAbierta bool

	AlCompartir, AlDescargar, AlAlternarDescripcion func()
	AlGestionar, AlCanal, AlTerminar                func()
	// AlDenunciar y AlDenunciarAutor abren la denuncia del vídeo y de quien lo publica;
	// AlBloquearAutor le bloquea. nil: no se ofrece (lo propio, o bloquear sin sesión).
	AlDenunciar, AlDenunciarAutor, AlBloquearAutor func()
}

// AnchoChat es el panel del chat a la derecha del reproductor (`lg:w-80`).
const AnchoChat = 20 * tema.Rem

// PantallaVer monta la página de un vídeo: reproductor, ficha, y comentarios debajo o
// chat al lado.
func PantallaVer(v Vista, d Ver) *fyne.Container {
	if cuerpo := CuerpoDeEstado(v, nil); cuerpo != nil {
		return Pagina(v, ui.Bloque{Obj: cuerpo})
	}
	// En el teléfono de pie el reproductor va de borde a borde, fuera del margen de la
	// página: en 360 de ancho, los 21 de cada lado son un vídeo más pequeño para nada.
	bordeABorde := v.Movil && v.Ventana < ui.CorteSM && d.Visor != nil
	principal := []ui.Bloque{}
	if d.Visor != nil && !bordeABorde {
		principal = append(principal, ui.Bloque{Obj: d.Visor, Margen: tema.Espacio(4)})
	}
	principal = append(principal, fichaVideo(v, d)...)
	var pagina *fyne.Container
	switch {
	case d.Chat == nil:
		principal = append(principal, ui.Bloque{Obj: PanelComentarios(v, d.Video, d.Comentarios)})
		pagina = Pagina(v, principal...)
	case v.Ventana < ui.CorteLG:
		// En estrecho el chat va debajo, con su alto propio.
		chat := PanelChat(v, *d.Chat)
		principal = append(principal, ui.Bloque{Obj: container.New(&altoFijo{alto: 26 * tema.Rem}, chat)})
		pagina = Pagina(v, principal...)
	default:
		chat := PanelChat(v, *d.Chat)
		pagina = Pagina(v, ui.Bloque{Obj: container.New(&conLateral{ancho: AnchoChat}, ui.PilaBloques(principal...), chat)})
	}
	if bordeABorde {
		return container.New(&visorArriba{ancho: v.Ventana}, d.Visor, pagina)
	}
	return pagina
}

// visorArriba pone el reproductor a todo el ancho arriba y la página debajo. Su alto
// depende del ancho, que MinSize no recibe: recuerda el de la última vez.
type visorArriba struct{ ancho float32 }

func (l *visorArriba) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, AltoVisor(l.ancho)+objs[1].MinSize().Height)
}

// AltoCon es el del reproductor a ese ancho más el de la página.
func (l *visorArriba) AltoCon(objs []fyne.CanvasObject, ancho float32) float32 {
	return AltoVisor(ancho) + ui.AltoA(objs[1], ancho)
}

func (l *visorArriba) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	l.ancho = t.Width
	alto := AltoVisor(t.Width)
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(fyne.NewSize(t.Width, alto))
	objs[1].Move(fyne.NewPos(0, alto))
	objs[1].Resize(fyne.NewSize(t.Width, max(t.Height-alto, 0)))
}

// fichaVideo es el título, la línea del autor, los mandos y la descripción.
func fichaVideo(v Vista, d Ver) []ui.Bloque {
	vid := d.Video
	titulo := Parrafo(v, vid.Title, ui.EscalonTitulo(3), ui.PesoTitulo(3), "text")

	// La línea del autor: avatar, nombre (que lleva al canal si el vídeo es público),
	// fecha y vistas o espectadores.
	var autor fyne.CanvasObject = ui.Texto(vid.Owner.Name, tema.TextoSM, ui.Medio, tema.Color("text", v.Variante))
	if d.AlCanal != nil {
		autor = ui.NuevoEnlace(vid.Owner.Name, tema.TextoSM, ui.Medio, "text", d.AlCanal)
	}
	datos := ui.Texto(Metadatos(v, vid), tema.TextoSM, ui.Normal, tema.Color("text-muted", v.Variante))
	izquierda := []fyne.CanvasObject{ui.NuevoAvatar(vid.Owner.Name).Con(ui.AvatarSM), autor, datos}

	var mandos []fyne.CanvasObject
	mandos = append(mandos, ui.NuevoBoton(v.Traduce("watch.share"), d.AlCompartir).Con(ui.Secundario, ui.TallaSM).ConIcono(iconos.Enlace, iconos.TallaSM))
	if d.Descargar {
		mandos = append(mandos, ui.NuevoBoton(v.Traduce("watch.download"), d.AlDescargar).Con(ui.Secundario, ui.TallaSM).ConIcono(iconos.Descargar, iconos.TallaSM))
	}
	if vid.Can.Edit {
		mandos = append(mandos, ui.NuevoBoton(v.Traduce("watch.manage"), d.AlGestionar).Con(ui.Secundario, ui.TallaSM).ConIcono(iconos.Lapiz, iconos.TallaSM))
		if vid.EnDirecto() {
			mandos = append(mandos, ui.NuevoBoton(v.Traduce("studio.browser.stop"), d.AlTerminar).Con(ui.Peligro, ui.TallaSM))
		}
	}
	if d.AlDenunciar != nil {
		mandos = append(mandos, botonDenunciar(v, d))
	}
	bloques := []ui.Bloque{{Obj: titulo, Margen: tema.Espacio(3)}}
	if v.Ventana < ui.CorteSM {
		// `flex-col sm:flex-row`: por debajo de sm, el autor en su fila y los mandos
		// debajo, a la izquierda.
		autorFila := container.New(ui.FilaAuto{Hueco: HuecoFila, Derecha: len(izquierda), Partir: true, SaltoFila: tema.Espacio(1)}, izquierda...)
		bloques = append(bloques, ui.Bloque{Obj: autorFila, Margen: tema.Espacio(3)}, ui.Bloque{Obj: Fila(mandos...), Margen: tema.Espacio(4)})
	} else {
		hijos := slices.Concat(izquierda, mandos)
		fila := container.New(ui.FilaAuto{Hueco: HuecoFila, Derecha: len(izquierda), Partir: true, SaltoFila: tema.Espacio(3)}, hijos...)
		bloques = append(bloques, ui.Bloque{Obj: fila, Margen: tema.Espacio(4)})
	}
	if vid.Can.Edit && vid.Status == api.CastStatusFailed && vid.Failure != "" {
		bloques = append(bloques, ui.Bloque{
			Obj:    Parrafo(v, v.Traduce("manage.failure", i18n.Vars{"reason": vid.Failure}), tema.TextoSM, ui.Normal, "danger-text"),
			Margen: tema.Espacio(3),
		})
	}
	if desc := strings.TrimSpace(vid.Description); desc != "" {
		caja := ui.NuevaCaja(tema.Color("bg-subtle", v.Variante), transparente(), tema.Forma("radius-card"))
		var cuerpo fyne.CanvasObject = Parrafo(v, desc, tema.TextoSM, ui.Normal, "text")
		// Larga (más de 280 caracteres o de cuatro líneas): recortada a cuatro con
		// «Mostrar más», como Descripcion de WatchApp.tsx.
		if len([]rune(desc)) > 280 || strings.Count(desc, "\n") >= 4 {
			p := Parrafo(v, desc, tema.TextoSM, ui.Normal, "text")
			clave := "watch.less"
			if !d.DescripcionAbierta {
				p.Lineas, clave = 4, "watch.more"
			}
			mas := ui.NuevoEnlace(v.Traduce(clave), tema.TextoSM, ui.SemiNegrita, "text", d.AlAlternarDescripcion)
			cuerpo = ui.PilaBloques(ui.Bloque{Obj: p, Margen: tema.Espacio(1)}, ui.Bloque{Obj: container.New(ui.FilaAuto{Derecha: 1}, mas)})
		}
		bloques = append(bloques, ui.Bloque{
			Obj:    container.New(&recuadroLayout{caja: caja}, append(caja.Objetos(), cuerpo)...),
			Margen: HuecoSeccion,
		})
	} else {
		bloques[len(bloques)-1].Margen = HuecoSeccion
	}
	return bloques
}

// botonDenunciar es «Denunciar» junto a Compartir: un menú con el vídeo, quien lo
// publica y —con sesión— bloquearle. Un menú y no tres botones: son acciones poco
// frecuentes.
func botonDenunciar(v Vista, d Ver) fyne.CanvasObject {
	nombre := i18n.Vars{"name": d.Video.Owner.Name}
	elementos := []fyne.CanvasObject{ui.NuevoElementoMenu(v.Traduce("report.title.video"), iconos.Ninguno, d.AlDenunciar)}
	if d.AlDenunciarAutor != nil {
		elementos = append(elementos, ui.NuevoElementoMenu(v.Traduce("report.title.person", nombre), iconos.Ninguno, d.AlDenunciarAutor))
	}
	if d.AlBloquearAutor != nil {
		bloquear := ui.NuevoElementoMenu(v.Traduce("block.action", nombre), iconos.Ninguno, d.AlBloquearAutor)
		bloquear.Peligro = true
		elementos = append(elementos, bloquear)
	}
	var b *ui.Boton
	b = ui.NuevoBoton(v.Traduce("report.action"), func() {
		m := ui.NuevoMenu(elementos...)
		m.Rotulo = v.Traduce("report.action")
		m.Mostrar(b)
	}).Con(ui.Secundario, ui.TallaSM).ConIcono(iconos.Peligro, iconos.TallaSM)
	return b
}

// Metadatos es la línea de debajo del título (Metadatos de WatchApp.tsx): en directo,
// desde cuándo; si no, las vistas y la fecha de publicación (o del directo grabado);
// y la visibilidad.
func Metadatos(v Vista, vid api.CastVideo) string {
	var partes []string
	loc := v.T.Locale()
	if vid.EnDirecto() && vid.Live != nil && vid.Live.StartedAt != nil {
		partes = append(partes, v.Traduce("watch.liveSince", i18n.Vars{"time": i18n.FormatTime(loc, *vid.Live.StartedAt)}))
	} else {
		partes = append(partes, v.Traduce("watch.views", i18n.Vars{"count": vid.Views}))
		fecha := vid.Fecha()
		if vid.Kind == "live" {
			if vid.Live != nil && vid.Live.StartedAt != nil {
				fecha = *vid.Live.StartedAt
			}
			partes = append(partes, v.Traduce("watch.recorded", i18n.Vars{"date": i18n.FormatDate(loc, fecha)}))
		} else {
			partes = append(partes, v.Traduce("watch.published", i18n.Vars{"date": i18n.FormatDate(loc, fecha)}))
		}
	}
	partes = append(partes, v.Traduce(claveVisibilidad(vid.Visibility)))
	return strings.Join(partes, " · ")
}

// ---- Compartir ----

// Compartir son los datos del diálogo de compartir (ShareDialog.tsx).
type Compartir struct {
	Video  api.CastVideo
	Enlace string
	// Insercion es el `<iframe>`; vacío si el vídeo no se puede insertar.
	Insercion string
	// Copiado es la línea de «Copiado» tras pulsar.
	Copiado string

	AlCopiarEnlace, AlCopiarInsercion func()
}

// enlaceCompartir es la fila del enlace de ShareDialog.tsx: el campo de solo lectura y
// «Copiar el enlace» (primario, con su icono); en el teléfono, el botón debajo y a todo
// el ancho (`max-md:flex-col`). Era la caja del enlace de OBS con «Copiar» `sm`.
func enlaceCompartir(v Vista, d Compartir) fyne.CanvasObject {
	campo := ui.NuevaEntrada("")
	campo.PonerTexto(d.Enlace)
	campo.SoloLectura(true)
	copiar := ui.NuevoBoton(v.Traduce("share.copyLink"), d.AlCopiarEnlace).Con(ui.Primario, ui.TallaMD).ConIcono(iconos.Enlace, iconos.TallaSM)
	var fila fyne.CanvasObject = container.New(ui.FilaAuto{Hueco: HuecoFila, Derecha: 2, Elastico: 1, Partir: true}, campo, copiar)
	if v.Ventana > 0 && v.Ventana < ui.Estrecho {
		fila = container.New(ui.Columna{Hueco: HuecoFila}, campo, copiar)
	}
	return ui.GrupoCampo(v.Traduce("share.link"), false, fila, "", "")
}

// CuerpoCompartir es lo de dentro del diálogo: el enlace con su botón, la nota de
// quién podrá abrirlo si no es abierto, y el código de inserción si lo es.
func CuerpoCompartir(v Vista, d Compartir) fyne.CanvasObject {
	abierto := d.Video.Visibility == api.CastPublic || d.Video.Visibility == api.CastUnlisted
	enlace := enlaceCompartir(v, d)
	bloques := []ui.Bloque{{Obj: enlace, Margen: tema.Espacio(3)}}
	if !abierto {
		bloques = append(bloques, ui.Bloque{Obj: Parrafo(v, v.Traduce("share.restricted"), tema.TextoXS, ui.Normal, "text-muted"), Margen: tema.Espacio(4)})
	}
	rotulo := ui.Texto(v.Traduce("share.embed"), tema.TextoSM, ui.SemiNegrita, tema.Color("text", v.Variante))
	bloques = append(bloques, ui.Bloque{Obj: container.New(ui.FilaAuto{Derecha: 1, AlturaFila: tema.TextoSM.Linea}, rotulo), Margen: ui.HuecoGrupoCampo})
	if abierto {
		codigo := Parrafo(v, d.Insercion, tema.TextoXS, ui.Mono, "text")
		caja := ui.NuevaCaja(tema.Color("bg-subtle", v.Variante), tema.Color("border", v.Variante), tema.RadioMD)
		bloques = append(bloques,
			ui.Bloque{Obj: container.New(&recuadroFino{caja: caja}, append(caja.Objetos(), codigo)...), Margen: tema.Espacio(2)},
			ui.Bloque{Obj: container.New(ui.FilaAuto{Derecha: 1},
				ui.NuevoBoton(v.Traduce("share.copyEmbed"), d.AlCopiarInsercion).Con(ui.Secundario, ui.TallaSM).ConIcono(iconos.Copias, iconos.TallaSM))},
		)
	} else {
		bloques = append(bloques, ui.Bloque{Obj: Parrafo(v, v.Traduce("share.embedOnlyOpen"), tema.TextoXS, ui.Normal, "text-muted")})
	}
	if d.Copiado != "" {
		bloques[len(bloques)-1].Margen = tema.Espacio(3)
		bloques = append(bloques, ui.Bloque{Obj: Parrafo(v, d.Copiado, tema.TextoSM, ui.Normal, "success-text")})
	}
	return ui.PilaBloques(bloques...)
}

// ---- Comentarios ----

// Comentarios son los datos del bloque de comentarios.
type Comentarios struct {
	Lista    []api.CastComment
	Cargando bool
	Error    string
	// Campo es donde se escribe; vive en App.
	Campo *ui.Entrada
	// RespondiendoA es el comentario al que se contesta; nil, uno nuevo.
	RespondiendoA *api.CastComment
	ConSesion     bool
	Enviando      bool
	// ErrorCarga: no se pudieron leer (con reintentar). HayMas: queda otra página.
	ErrorCarga, HayMas bool

	AlComentar          func()
	AlReintentar        func()
	AlMas               func()
	AlResponder         func(api.CastComment)
	AlCancelarRespuesta func()
	AlBorrar            func(api.CastComment)
	AlEntrar            func()
	// MiSub es quien mira: lo suyo no se denuncia ni se bloquea. AlDenunciar vale
	// también sin sesión; AlBloquear, solo con ella.
	MiSub       string
	AlDenunciar func(api.CastComment)
	AlBloquear  func(api.CastComment)
}

// PanelComentarios es la sección de comentarios bajo un vídeo: el recuento, el campo y
// los hilos de un nivel.
func PanelComentarios(v Vista, vid api.CastVideo, d Comentarios) fyne.CanvasObject {
	vivos := 0
	for _, c := range d.Lista {
		if !c.Deleted {
			vivos++
		}
	}
	bloques := []ui.Bloque{{
		Obj:    TituloSeccion(v, v.Traduce("comments.title"), ui.Texto(v.Traduce("comments.count", i18n.Vars{"count": vivos}), tema.TextoSM, ui.Normal, tema.Color("text-muted", v.Variante))),
		Margen: tema.Espacio(4),
	}}
	switch {
	case !vid.CommentsEnabled:
		bloques = append(bloques, ui.Bloque{Obj: Parrafo(v, v.Traduce("comments.disabled"), tema.TextoSM, ui.Normal, "text-muted")})
		return ui.PilaBloques(bloques...)
	case !d.ConSesion:
		entrar := ui.NuevoEnlace(v.Traduce("comments.signIn"), tema.TextoSM, ui.Medio, "primary-text", d.AlEntrar)
		bloques = append(bloques, ui.Bloque{Obj: container.New(ui.FilaAuto{Derecha: 1}, entrar), Margen: tema.Espacio(5)})
	case d.Campo != nil:
		bloques = append(bloques, ui.Bloque{Obj: redactor(v, d), Margen: tema.Espacio(5)})
	}
	if d.Error != "" {
		bloques = append(bloques, ui.Bloque{Obj: Parrafo(v, d.Error, tema.TextoSM, ui.Normal, "danger-text"), Margen: tema.Espacio(3)})
	}
	if d.ErrorCarga && len(d.Lista) == 0 {
		bloques = append(bloques, ui.Bloque{Obj: Fila(
			Parrafo(v, v.Traduce("comments.loadError"), tema.TextoSM, ui.Normal, "danger-text"),
			ui.NuevoBoton(v.Traduce("errors.retry"), d.AlReintentar).Con(ui.Secundario, ui.TallaSM))})
		return ui.PilaBloques(bloques...)
	}
	if d.Cargando && len(d.Lista) == 0 {
		r := ui.NuevaRueda(ui.LadoRueda, "text-muted", v.Variante)
		r.Rotulo = v.Traduce("ui.loading")
		bloques = append(bloques, ui.Bloque{Obj: container.New(ui.FilaAuto{Derecha: 1}, r)})
		return ui.PilaBloques(bloques...)
	}
	if vivos == 0 {
		bloques = append(bloques, ui.Bloque{Obj: Parrafo(v, v.Traduce("comments.empty"), tema.TextoSM, ui.Normal, "text-muted")})
		return ui.PilaBloques(bloques...)
	}
	bloques = append(bloques, hilos(v, d)...)
	if d.HayMas {
		bloques = append(bloques, ui.Bloque{Obj: botonMasCon(v, "comments.more", d.Cargando, d.AlMas)})
	}
	return ui.PilaBloques(bloques...)
}

// hilos son los comentarios con sus respuestas debajo, sangradas. Un comentario
// borrado solo se deja si tiene respuestas vivas (para que no queden huérfanas).
func hilos(v Vista, d Comentarios) []ui.Bloque {
	var bloques []ui.Bloque
	respuestas := map[string][]api.CastComment{}
	for _, c := range d.Lista {
		if c.ReplyTo != "" {
			respuestas[c.ReplyTo] = append(respuestas[c.ReplyTo], c)
		}
	}
	for _, c := range d.Lista {
		if c.ReplyTo != "" {
			continue
		}
		if !c.Deleted || len(respuestas[c.ID]) > 0 {
			bloques = append(bloques, ui.Bloque{Obj: filaComentario(v, c, d, false), Margen: tema.Espacio(4)})
		}
		for _, r := range respuestas[c.ID] {
			if !r.Deleted {
				bloques = append(bloques, ui.Bloque{Obj: filaComentario(v, r, d, true), Margen: tema.Espacio(4)})
			}
		}
	}
	return bloques
}

// redactor es el campo de comentar con su botón, y el «Respondiendo a …» encima.
func redactor(v Vista, d Comentarios) fyne.CanvasObject {
	enviar := ui.NuevoBoton(v.Traduce("comments.send"), d.AlComentar).Con(ui.Primario, ui.TallaSM)
	enviar.Inerte = d.Enviando
	fila := container.New(ui.FilaAuto{Hueco: HuecoFila, Derecha: 2, Elastico: 1, Partir: true}, d.Campo, enviar)
	if d.RespondiendoA == nil {
		return fila
	}
	aviso := ui.Texto(v.Traduce("comments.replyPlaceholder", i18n.Vars{"name": d.RespondiendoA.Author.Name}),
		tema.TextoXS, ui.Normal, tema.Color("text-muted", v.Variante))
	cancelar := ui.NuevoEnlace(v.Traduce("ui.cancel"), tema.TextoXS, ui.Medio, "primary-text", d.AlCancelarRespuesta)
	return ui.PilaBloques(
		ui.Bloque{Obj: container.New(ui.FilaAuto{Hueco: HuecoFila, Derecha: 2}, aviso, cancelar), Margen: tema.Espacio(2)},
		ui.Bloque{Obj: fila},
	)
}

// filaComentario es un comentario: avatar, autor y fecha, texto y sus acciones.
func filaComentario(v Vista, c api.CastComment, d Comentarios, respuesta bool) fyne.CanvasObject {
	if c.Deleted {
		return container.New(&sangria{izq: sangriaDe(respuesta)},
			Parrafo(v, v.Traduce("comments.deleted"), tema.TextoSM, ui.Cursiva, "text-subtle"))
	}
	cabecera := container.New(ui.FilaAuto{Hueco: HuecoFila, Derecha: 2},
		ui.Texto(c.Author.Name, tema.TextoSM, ui.SemiNegrita, tema.Color("text", v.Variante)),
		ui.Texto(Hace(v, c.CreatedAt), tema.TextoXS, ui.Normal, tema.Color("text-muted", v.Variante)),
	)
	derecha := []ui.Bloque{
		{Obj: cabecera, Margen: tema.Espacio(1)},
		{Obj: Parrafo(v, c.Body, tema.TextoSM, ui.Normal, "text")},
	}
	var acciones []fyne.CanvasObject
	if d.ConSesion && !respuesta && d.AlResponder != nil {
		acciones = append(acciones, ui.NuevoEnlace(v.Traduce("comments.reply"), tema.TextoXS, ui.Medio, "text-muted", func() { d.AlResponder(c) }))
	}
	if c.Can.Delete && d.AlBorrar != nil {
		acciones = append(acciones, ui.NuevoEnlace(v.Traduce("comments.delete"), tema.TextoXS, ui.Medio, "text-muted", func() { d.AlBorrar(c) }))
	}
	if c.Author.Sub != d.MiSub && d.AlDenunciar != nil {
		acciones = append(acciones, ui.NuevoEnlace(v.Traduce("report.action"), tema.TextoXS, ui.Medio, "text-muted", func() { d.AlDenunciar(c) }))
	}
	if c.Author.Sub != d.MiSub && d.ConSesion && d.AlBloquear != nil {
		acciones = append(acciones, ui.NuevoEnlace(v.Traduce("block.action", i18n.Vars{"name": c.Author.Name}), tema.TextoXS, ui.Medio, "text-muted", func() { d.AlBloquear(c) }))
	}
	if len(acciones) > 0 {
		derecha[1].Margen = tema.Espacio(1.5)
		derecha = append(derecha, ui.Bloque{Obj: container.New(ui.FilaAuto{Hueco: tema.Espacio(3), Derecha: len(acciones), Partir: true, SaltoFila: tema.Espacio(1)}, acciones...)})
	}
	return container.New(&sangria{izq: sangriaDe(respuesta)},
		container.New(&filaAvatar{}, ui.NuevoAvatar(c.Author.Name).Con(ui.AvatarSM), ui.PilaBloques(derecha...)))
}

func sangriaDe(respuesta bool) float32 {
	if respuesta {
		return tema.Espacio(10)
	}
	return 0
}

// ---- Chat del directo ----

// Chat son los datos del panel de chat de un directo.
type Chat struct {
	Mensajes []api.CastChatMessage
	Campo    *ui.Entrada
	// Activo es `chat_enabled`.
	Activo    bool
	ConSesion bool
	Moderar   bool
	// Conectado: el WebSocket está abierto. Sin él no se puede escribir.
	Conectado bool
	Error     string
	Viendo    int
	// Lista es el desplazamiento de los mensajes, que vive en App para seguir abajo.
	Lista *container.Scroll

	AlEnviar func()
	AlBorrar func(api.CastChatMessage)
	AlEntrar func()
	// MiSub, AlDenunciar y AlBloquear: como en los comentarios, para los mensajes
	// ajenos, desde el «…» de cada uno.
	MiSub       string
	AlDenunciar func(api.CastChatMessage)
	AlBloquear  func(api.CastChatMessage)
}

// PanelChat es el ChatPanel: cabecera con los espectadores, los mensajes con su
// desplazamiento y el campo abajo.
func PanelChat(v Vista, d Chat) fyne.CanvasObject {
	cabecera := container.New(ui.FilaAuto{Hueco: HuecoFila, Derecha: 1, AlturaFila: tema.Espacio(6)},
		ui.Texto(v.Traduce("chat.title"), tema.TextoSM, ui.SemiNegrita, tema.Color("text", v.Variante)),
		ui.Texto(v.Traduce("live.viewers", i18n.Vars{"count": d.Viendo}), tema.TextoXS, ui.Normal, tema.Color("text-muted", v.Variante)),
	)
	var mensajes []ui.Bloque
	switch {
	case !d.Activo:
		mensajes = append(mensajes, ui.Bloque{Obj: Parrafo(v, v.Traduce("chat.disabled"), tema.TextoSM, ui.Normal, "text-muted")})
	case len(d.Mensajes) == 0:
		mensajes = append(mensajes, ui.Bloque{Obj: Parrafo(v, v.Traduce("chat.empty"), tema.TextoSM, ui.Normal, "text-muted")})
	}
	for _, m := range d.Mensajes {
		mensajes = append(mensajes, ui.Bloque{Obj: filaChat(v, m, d), Margen: tema.Espacio(2.5)})
	}
	cuerpo := ui.PilaBloques(mensajes...)
	var lista fyne.CanvasObject
	if d.Lista != nil {
		d.Lista.Content = container.New(&alAnchoDe{s: d.Lista}, cuerpo)
		lista = d.Lista
	} else {
		lista = cuerpo
	}
	var pie fyne.CanvasObject
	switch {
	case !d.Activo:
	case !d.ConSesion:
		pie = container.New(ui.FilaAuto{Derecha: 1}, ui.NuevoEnlace(v.Traduce("chat.signIn"), tema.TextoSM, ui.Medio, "primary-text", d.AlEntrar))
	case d.Campo != nil:
		enviar := ui.NuevoBoton(v.Traduce("chat.send"), d.AlEnviar).Con(ui.Primario, ui.TallaSM)
		enviar.Inerte = !d.Conectado
		pie = container.New(ui.FilaAuto{Hueco: HuecoFila, Derecha: 2, Elastico: 1, Partir: true}, d.Campo, enviar)
	}
	hijos := []fyne.CanvasObject{cabecera, lista}
	if d.Error != "" {
		hijos = append(hijos, Parrafo(v, d.Error, tema.TextoXS, ui.Normal, "danger-text"))
	}
	if pie != nil {
		hijos = append(hijos, pie)
	}
	caja := ui.NuevaCaja(tema.Color("surface", v.Variante), tema.Color("border", v.Variante), tema.RadioLG)
	return container.New(&panelLayout{caja: caja, n: len(hijos)}, append(caja.Objetos(), hijos...)...)
}

// filaChat es un mensaje: el nombre en negrita y el texto seguido, y la papelera para
// quien modera.
func filaChat(v Vista, m api.CastChatMessage, d Chat) fyne.CanvasObject {
	texto := ui.PilaBloques(
		ui.Bloque{Obj: container.New(ui.FilaAuto{Derecha: 1, AlturaFila: tema.TextoXS.Linea},
			ui.Texto(m.Author.Name, tema.TextoXS, ui.SemiNegrita, tema.Color("text-muted", v.Variante)))},
		ui.Bloque{Obj: Parrafo(v, m.Body, tema.TextoSM, ui.Normal, "text")},
	)
	hijos := []fyne.CanvasObject{texto}
	if d.Moderar && d.AlBorrar != nil {
		borrar := ui.NuevoBotonIcono(iconos.Papelera, v.Traduce("chat.delete"), func() { d.AlBorrar(m) })
		borrar.Lado = ui.LadoIconoSM
		hijos = append(hijos, borrar)
	}
	if m.Author.Sub != d.MiSub && d.AlDenunciar != nil {
		hijos = append(hijos, masDelMensaje(v, m, d))
	}
	if len(hijos) == 1 {
		return texto
	}
	return container.New(ui.FilaAuto{Hueco: tema.Espacio(1), Derecha: len(hijos) - 1, Elastico: 1, Partir: true}, hijos...)
}

// masDelMensaje es el «…» de un mensaje ajeno: denunciarlo y bloquear a quien lo escribió.
func masDelMensaje(v Vista, m api.CastChatMessage, d Chat) fyne.CanvasObject {
	nombre := i18n.Vars{"name": m.Author.Name}
	var b *ui.BotonIcono
	b = ui.NuevoBotonIcono(iconos.Puntos, v.Traduce("chat.more", nombre), func() {
		elementos := []fyne.CanvasObject{ui.NuevoElementoMenu(v.Traduce("report.action"), iconos.Ninguno, func() { d.AlDenunciar(m) })}
		if d.AlBloquear != nil {
			bloquear := ui.NuevoElementoMenu(v.Traduce("block.action", nombre), iconos.Ninguno, func() { d.AlBloquear(m) })
			bloquear.Peligro = true
			elementos = append(elementos, bloquear)
		}
		menu := ui.NuevoMenu(elementos...)
		menu.Rotulo = v.Traduce("chat.more", nombre)
		menu.Mostrar(b)
	})
	b.Lado = ui.LadoIconoSM
	return b
}

// ---- Layouts de la página ----

// conLateral: el primer hijo se lleva el ancho que deje el segundo, que va a la derecha
// con `ancho` y el alto del primero (el panel del chat se estira a la altura de la
// columna principal, y como poco a la del visor).
type conLateral struct{ ancho float32 }

func (c *conLateral) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(c.ancho*2, objs[0].MinSize().Height)
}

// AltoCon es el de la columna principal.
func (c *conLateral) AltoCon(objs []fyne.CanvasObject, ancho float32) float32 {
	return ui.AltoA(objs[0], ancho-c.ancho-HuecoRejilla*1.5)
}

func (c *conLateral) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	w := t.Width - c.ancho - HuecoRejilla*1.5
	h := ui.AltoA(objs[0], w)
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(fyne.NewSize(w, h))
	objs[1].Move(fyne.NewPos(w+HuecoRejilla*1.5, 0))
	objs[1].Resize(fyne.NewSize(c.ancho, max(AltoVisor(w), min(h, AltoVisor(w)+12*tema.Rem))))
}

// altoFijo da a su hijo un alto fijo a todo el ancho.
type altoFijo struct{ alto float32 }

func (a *altoFijo) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, a.alto) }

// AltoCon es el fijo.
func (a *altoFijo) AltoCon([]fyne.CanvasObject, float32) float32 { return a.alto }

func (a *altoFijo) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(t)
}

// panelLayout: la caja detrás; cabecera arriba, el segundo hijo (la lista) estirado y
// lo demás al pie, con `p-3`.
type panelLayout struct {
	caja *ui.Caja
	n    int
}

func (p *panelLayout) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(12*tema.Rem, 12*tema.Rem)
}

func (p *panelLayout) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	p.caja.Redimensionar(t)
	hijos := objs[len(objs)-p.n:]
	m := tema.Espacio(3)
	ancho := t.Width - 2*m
	y := m
	cab := hijos[0].MinSize().Height
	hijos[0].Move(fyne.NewPos(m, y))
	hijos[0].Resize(fyne.NewSize(ancho, cab))
	y += cab + tema.Espacio(2)
	abajo := float32(0)
	for _, o := range hijos[2:] {
		abajo += ui.AltoA(o, ancho) + tema.Espacio(2)
	}
	lista := max(t.Height-y-abajo-m, 0)
	hijos[1].Move(fyne.NewPos(m, y))
	hijos[1].Resize(fyne.NewSize(ancho, lista))
	y += lista
	for _, o := range hijos[2:] {
		y += tema.Espacio(2)
		h := ui.AltoA(o, ancho)
		o.Move(fyne.NewPos(m, y))
		o.Resize(fyne.NewSize(ancho, h))
		y += h
	}
}

// alAnchoDe mide el contenido de un desplazamiento al ancho del desplazamiento: sin
// esto los párrafos no saben en cuántas líneas se parten.
type alAnchoDe struct{ s *container.Scroll }

func (a *alAnchoDe) MinSize(objs []fyne.CanvasObject) fyne.Size {
	w := a.s.Size().Width
	return fyne.NewSize(0, ui.AltoA(objs[0], w))
}

func (a *alAnchoDe) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(fyne.NewSize(t.Width, ui.AltoA(objs[0], t.Width)))
}

// sangria mete a su hijo `izq` hacia dentro (las respuestas).
type sangria struct{ izq float32 }

func (s *sangria) MinSize(objs []fyne.CanvasObject) fyne.Size {
	m := objs[0].MinSize()
	return fyne.NewSize(m.Width+s.izq, m.Height)
}

// AltoCon es el del hijo al ancho que queda.
func (s *sangria) AltoCon(objs []fyne.CanvasObject, ancho float32) float32 {
	return ui.AltoA(objs[0], ancho-s.izq)
}

func (s *sangria) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(s.izq, 0))
	objs[0].Resize(fyne.NewSize(t.Width-s.izq, t.Height))
}

// filaAvatar: el avatar arriba a la izquierda y el bloque de texto a su derecha
// (`flex gap-3 items-start`).
type filaAvatar struct{}

func (filaAvatar) MinSize(objs []fyne.CanvasObject) fyne.Size {
	a, t := objs[0].MinSize(), objs[1].MinSize()
	return fyne.NewSize(a.Width+HuecoTitulo+t.Width, max(a.Height, t.Height))
}

// AltoCon: el del texto al ancho que queda, o el del avatar.
func (filaAvatar) AltoCon(objs []fyne.CanvasObject, ancho float32) float32 {
	a := objs[0].MinSize()
	return max(a.Height, ui.AltoA(objs[1], ancho-a.Width-HuecoTitulo))
}

func (filaAvatar) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	a := objs[0].MinSize()
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(a)
	w := t.Width - a.Width - HuecoTitulo
	objs[1].Move(fyne.NewPos(a.Width+HuecoTitulo, 0))
	objs[1].Resize(fyne.NewSize(w, ui.AltoA(objs[1], w)))
}
