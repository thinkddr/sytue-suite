// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"context"
	"image/color"
	"math"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// El panel «Comentarios» (`CommentsPanel.tsx`): los hilos de un documento de la ofimática
// —la raíz y sus respuestas, por fecha—, con responder, resolver o reabrir y borrar, y el
// formulario de un comentario nuevo al pie. Es el mismo en Docus y en Calcus, como en la
// web, donde los dos editores montan `CommentsPanel` en su carril: lo propio de cada uno
// (el ancla en el folio de Docus, las menciones con su directorio) llega por FuenteComentarios.

// AnchoCuerpoPanel es lo que queda para el contenido de un panel: el panel menos su
// borde y el `p-3` de cada lado.
var AnchoCuerpoPanel = AnchoPanel - tema.Borde - 2*tema.Espacio(3)

// FuenteComentarios es lo que el panel necesita de su editor.
type FuenteComentarios struct {
	T      func(string) string
	Loc    i18n.Locale
	Svc    *api.Docs
	ID     string // el documento
	Lanzar func(func())
	Plazo  time.Duration
	Avisos *ui.PilaAvisos
	// SinRed apaga los botones mientras no hay conexión.
	SinRed func() bool
	// Menciones, si no es nil, cuelga el selector de «@» de un área: devuelve la lista
	// que se pinta bajo ella y lo que hay que llamar con cada cambio del texto.
	Menciones func(*ui.AreaTexto) (fyne.CanvasObject, func(string))
	// Resaltar pinta (o, con "", apaga) el ancla de un hilo al pasar por encima;
	// QuitarAncla la retira al resolver o borrar el hilo. Nil en un editor sin anclas.
	Resaltar, QuitarAncla func(marca string)
	// En una hoja, el hilo de una celda dice dónde está HOY (`comments.cell`, «Celda B5» u
	// «Hoja2!B5») con un botón que lleva a ella: Celda da ese rótulo ("" si la celda ya no
	// existe, y entonces vale la referencia citada) e IrACelda la selecciona (false si ya no
	// existe). ResaltarCelda enmarca la celda al pasar por el hilo (nil la apaga). Nil en un
	// editor sin hojas.
	Celda         func(api.AnclaCelda) string
	IrACelda      func(api.AnclaCelda) bool
	ResaltarCelda func(*api.AnclaCelda)
	// AlCargar recibe cada lista recién pedida: la rejilla de Calcus marca con ella sus
	// celdas comentadas.
	AlCargar func([]api.DocComment)
}

// Comentarios es el estado del panel de UN documento abierto.
type Comentarios struct {
	f FuenteComentarios

	lista        []api.DocComment
	respondiendo string // el hilo con el formulario de respuesta abierto
	pendiente    bool   // una petición en marcha: los botones se apagan (anti doble clic)

	// Nuevo es el área del comentario nuevo y Enviar, su «Comentar».
	Nuevo     *ui.AreaTexto
	Enviar    *ui.Boton
	respuesta *ui.AreaTexto
	// menNuevo y menRespuesta son las listas de menciones de las dos áreas (o nil).
	menNuevo, menRespuesta fyne.CanvasObject
	hilos                  *fyne.Container
	objeto                 fyne.CanvasObject
	// anchoCuerpo es el de la columna de hilos, al que se reparten los textos: el del
	// panel de escritorio hasta que el panel diga otro (ver anchoDeComentarios).
	anchoCuerpo float32
}

// cuerpo es el ancho al que se reparten los hilos.
func (c *Comentarios) cuerpo() float32 {
	if c.anchoCuerpo > 0 {
		return c.anchoCuerpo
	}
	return AnchoCuerpoPanel
}

// anchoDeComentarios coloca el panel y, si su ancho cambia, vuelve a repartir los hilos.
//
// Los textos se repartían SIEMPRE al ancho del panel de escritorio (288), y en el teléfono
// el panel ocupa la pantalla entera: cada comentario partía en la columna de 245 con más de
// cien píxeles libres a la derecha (auditoría del 2026-11-24). El reparto de un `Parrafo`
// es su `Ancho`, así que se rehacen con el de verdad.
type anchoDeComentarios struct{ c *Comentarios }

func (a anchoDeComentarios) MinSize(objs []fyne.CanvasObject) fyne.Size { return objs[0].MinSize() }

func (a anchoDeComentarios) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	if cuerpo := t.Width - 2*MargenPanel; t.Width > 0 && math.Abs(float64(cuerpo-a.c.cuerpo())) > 0.5 {
		a.c.anchoCuerpo = cuerpo
		a.c.pintar()
	}
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(t)
}

func (c *Comentarios) t(clave string) string {
	if c.f.T == nil {
		return clave
	}
	return c.f.T(clave)
}

// Objeto es el cuerpo del panel; Lista, los comentarios que enseña.
func (c *Comentarios) Objeto() fyne.CanvasObject { return c.objeto }
func (c *Comentarios) Lista() []api.DocComment   { return c.lista }

func (c *Comentarios) avisar(clave string) {
	if c.f.Avisos != nil {
		c.f.Avisos.Notificar(ui.NuevoAviso(ui.AvisoPeligro, c.t(clave), ""))
	}
}

func (c *Comentarios) menciones(area *ui.AreaTexto) (fyne.CanvasObject, func(string)) {
	if c.f.Menciones == nil {
		return container.New(ui.Columna{}), func(string) {}
	}
	return c.f.Menciones(area)
}

// NuevosComentarios monta el panel y pide la lista.
func NuevosComentarios(f FuenteComentarios) *Comentarios {
	if f.Lanzar == nil {
		f.Lanzar = func(g func()) { go g() }
	}
	if f.Plazo == 0 {
		f.Plazo = 30 * time.Second
	}
	c := &Comentarios{f: f}
	c.Nuevo = ui.NuevaAreaTexto(c.t("comments.new.placeholder"))
	c.Nuevo.Filas = 2
	var cambioNuevo, cambioRespuesta func(string)
	c.menNuevo, cambioNuevo = c.menciones(c.Nuevo)
	c.Nuevo.AlCambiar(func(x string) { c.pintarEnviar(); cambioNuevo(x) })
	c.respuesta = ui.NuevaAreaTexto(c.t("comments.reply.placeholder"))
	c.respuesta.Filas = 2
	c.menRespuesta, cambioRespuesta = c.menciones(c.respuesta)
	c.respuesta.AlCambiar(cambioRespuesta)
	c.Enviar = ui.NuevoBoton(c.t("comments.new.send"), func() {
		cuerpo := strings.TrimSpace(c.Nuevo.Texto())
		c.mutar(func(ctx context.Context) error {
			_, err := c.f.Svc.CreateComment(ctx, c.f.ID, "", cuerpo)
			return err
		}, "comments.addError", func() { c.Nuevo.PonerTexto("") })
	}).Con(ui.Primario, ui.TallaSM)
	c.hilos = container.New(ui.Columna{Hueco: tema.Espacio(2)})
	// `flex min-h-full flex-col gap-2 p-3`, con el formulario al pie (`mt-auto`).
	pie := container.New(ui.Columna{Hueco: tema.Espacio(1.5)}, c.Nuevo, c.menNuevo, container.New(AlDerecha{}, c.Enviar))
	// Los hilos se componen con las tintas ya resueltas (las cajas, las citas, el 60 % de
	// los resueltos): al cambiar de tema en caliente se vuelven a componer, o las tarjetas
	// se quedaban en el tema anterior.
	c.objeto = ui.RehacerConElTema(container.New(anchoDeComentarios{c}, container.New(layout.NewCustomPaddedLayout(MargenPanel, MargenPanel, MargenPanel, MargenPanel),
		container.NewBorder(nil, container.New(layout.NewCustomPaddedLayout(tema.Espacio(1), 0, 0, 0), pie), nil, nil,
			container.NewVScroll(c.hilos)))), c.pintar)
	c.pintar()
	c.Recargar()
	return c
}

// Recargar pide la lista (con los resueltos, que la web también enseña, atenuados).
func (c *Comentarios) Recargar() {
	if c.f.Svc == nil {
		return
	}
	f := c.f
	f.Lanzar(func() {
		ctx, cancelar := context.WithTimeout(context.Background(), f.Plazo)
		defer cancelar()
		lista, err := f.Svc.Comments(ctx, f.ID, true)
		fyne.Do(func() {
			if err == nil {
				c.lista = lista
				if c.f.AlCargar != nil {
					c.f.AlCargar(lista)
				}
			}
			c.pintar()
		})
	})
}

// mutar corre una petición con el candado de la web: mientras dura, todo se apaga; al
// acabar se recarga la lista, y si falla se dice con un aviso.
func (c *Comentarios) mutar(hacer func(context.Context) error, claveError string, alAcabar func()) {
	if c.pendiente || c.f.Svc == nil {
		return
	}
	c.pendiente = true
	c.pintar()
	f := c.f
	f.Lanzar(func() {
		ctx, cancelar := context.WithTimeout(context.Background(), f.Plazo)
		defer cancelar()
		err := hacer(ctx)
		fyne.Do(func() {
			c.pendiente = false
			if err != nil {
				c.avisar(claveError)
				c.pintar()
				return
			}
			if alAcabar != nil {
				alAcabar()
			}
			c.Recargar()
		})
	})
}

func (c *Comentarios) sinRed() bool { return c.f.SinRed != nil && c.f.SinRed() }

func (c *Comentarios) pintarEnviar() {
	c.Enviar.Inerte = c.pendiente || c.sinRed() || strings.TrimSpace(c.Nuevo.Texto()) == ""
	c.Enviar.Refresh()
}

// Hilos agrupa: las raíces por fecha y, de cada una, sus respuestas por fecha. Una
// respuesta cuyo padre ya no está se enseña como raíz, como en la web.
func Hilos(lista []api.DocComment) ([]api.DocComment, map[string][]api.DocComment) {
	ids := map[string]bool{}
	for _, x := range lista {
		ids[x.ID] = true
	}
	var raices []api.DocComment
	resp := map[string][]api.DocComment{}
	for _, x := range lista {
		if x.ParentID != "" && ids[x.ParentID] {
			resp[x.ParentID] = append(resp[x.ParentID], x)
		} else {
			raices = append(raices, x)
		}
	}
	porFecha := func(s []api.DocComment) {
		sort.SliceStable(s, func(i, j int) bool { return s[i].CreatedAt.Before(s[j].CreatedAt) })
	}
	porFecha(raices)
	for k := range resp {
		porFecha(resp[k])
	}
	return raices, resp
}

// pintar rehace la lista de hilos con el estado de ahora.
func (c *Comentarios) pintar() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	raices, resp := Hilos(c.lista)
	c.hilos.Objects = nil
	if len(raices) == 0 {
		c.hilos.Add(ui.NuevoParrafo(c.t("comments.empty"), tema.TextoSM, ui.Normal, "text-muted", v).A(c.cuerpo()))
	}
	for _, r := range raices {
		c.hilos.Add(c.hilo(r, resp[r.ID], v))
	}
	c.hilos.Refresh()
	c.pintarEnviar()
}

// hilo es el `<article class="border-border rounded-md border p-2">` de un comentario raíz.
func (c *Comentarios) hilo(r api.DocComment, respuestas []api.DocComment, v tema.Variante) fyne.CanvasObject {
	ancho := c.cuerpo() - 2*tema.Borde - 2*tema.Espacio(2)
	inerte := c.pendiente || c.sinRed()
	// `opacity-60` en un hilo resuelto: sin opacidad de grupo, las tintas se aplanan.
	tinta := func(token string) color.NRGBA {
		col := tema.Color(token, v)
		if r.Resolved {
			col = al(col, 0.6, v)
		}
		return col
	}
	var filas []fyne.CanvasObject
	filas = append(filas, c.cabeceraComentario(r))
	celda, conCelda := r.AnclaCelda()
	conCelda = conCelda && c.f.Celda != nil
	if conCelda {
		filas = append(filas, ConArriba(tema.Espacio(1), c.botonCelda(celda)))
	} else if cita := r.AnchorText(); cita != "" {
		filas = append(filas, ConArriba(tema.Espacio(1), citaComentario("«"+cita+"»", ancho, tinta)))
	}
	filas = append(filas, ConArriba(tema.Espacio(1), cuerpoComentario(r.Body, ancho, tinta("text"))))

	responder := ui.NuevoBoton(c.t("comments.reply"), func() {
		c.respuesta.PonerTexto("")
		if c.respondiendo == r.ID {
			c.respondiendo = ""
		} else {
			c.respondiendo = r.ID
		}
		c.pintar()
	}).Con(ui.Fantasma, ui.TallaXS)
	responder.Atenuado = true // `TEXT_BTN`
	claveResolver := "comments.resolve"
	if r.Resolved {
		claveResolver = "comments.reopen"
	}
	// Resolver y borrar un hilo retiran su resaltado del folio; reabrirlo no lo vuelve
	// a poner, como en la web.
	quitarAncla := func() {
		if c.f.Resaltar != nil {
			c.f.Resaltar("")
		}
		if c.f.QuitarAncla != nil {
			c.f.QuitarAncla(r.AnchorMark())
		}
	}
	resolver := ui.NuevoBoton(c.t(claveResolver), func() {
		alAcabar := quitarAncla
		if r.Resolved {
			alAcabar = nil
		}
		c.mutar(func(ctx context.Context) error {
			return c.f.Svc.ResolveComment(ctx, c.f.ID, r.ID, !r.Resolved)
		}, "comments.resolveError", alAcabar)
	}).Con(ui.Fantasma, ui.TallaXS).ConIcono(iconos.Tic, 14)
	resolver.Atenuado = true
	borrar := botonBorrar(c.t("comments.delete"), func() {
		c.mutar(func(ctx context.Context) error { return c.f.Svc.DeleteComment(ctx, c.f.ID, r.ID) }, "comments.deleteError", quitarAncla)
	})
	responder.Inerte, resolver.Inerte, borrar.Inerte = inerte, inerte, inerte
	filas = append(filas, ConArriba(tema.Espacio(1.5), container.New(FilaAcciones{}, responder, resolver, borrar)))

	for _, x := range respuestas {
		filas = append(filas, ConArriba(tema.Espacio(2), c.respuestaDe(x, ancho, tinta, inerte)))
	}
	if c.respondiendo == r.ID {
		cancelar := ui.NuevoBoton(c.t("action.cancel"), func() {
			c.respondiendo = ""
			c.respuesta.PonerTexto("")
			c.pintar()
		}).Con(ui.Fantasma, ui.TallaSM)
		enviar := ui.NuevoBoton(c.t("comments.reply.send"), func() {
			cuerpo := strings.TrimSpace(c.respuesta.Texto())
			if cuerpo == "" {
				return
			}
			c.mutar(func(ctx context.Context) error {
				_, err := c.f.Svc.CreateComment(ctx, c.f.ID, r.ID, cuerpo)
				return err
			}, "comments.reply.error", func() {
				c.respondiendo = ""
				c.respuesta.PonerTexto("")
			})
		}).Con(ui.Primario, ui.TallaSM)
		enviar.Inerte = inerte
		filas = append(filas, ConArriba(tema.Espacio(2), container.New(ui.Columna{Hueco: tema.Espacio(1.5)},
			c.respuesta, c.menRespuesta, container.New(AlDerecha{}, container.New(ui.Fila{Hueco: tema.Espacio(1.5)}, cancelar, enviar)))))
	}
	contenido := container.New(ui.Columna{}, filas...)
	// Sin fondo propio se ve el del panel: un relleno transparente dejaría ver el borde,
	// que la caja pinta DEBAJO del relleno (ver `ui.Caja`), y la tarjeta salía gris.
	caja := ui.NuevaCaja(tema.Color("surface", v), tema.Color("border", v), tema.RadioMD)
	tarjeta := container.New(&TarjetaComentario{Caja: caja}, append(caja.Objetos(), contenido)...)
	if conCelda && c.f.ResaltarCelda != nil {
		return nuevaZonaAncla(tarjeta, func() { c.f.ResaltarCelda(&celda) }, func() { c.f.ResaltarCelda(nil) })
	}
	marca := r.AnchorMark()
	if marca == "" || c.f.Resaltar == nil {
		return tarjeta
	}
	return nuevaZonaAncla(tarjeta, func() { c.f.Resaltar(marca) }, func() { c.f.Resaltar("") })
}

// botonCelda es el «Celda B5» de un hilo de una hoja (`TEXT_BTN_INICIO`): lleva a la celda,
// cambiando de hoja si hace falta, o avisa de que ya no existe.
func (c *Comentarios) botonCelda(a api.AnclaCelda) fyne.CanvasObject {
	rotulo := c.f.Celda(a)
	if rotulo == "" {
		rotulo = a.Text
	}
	b := ui.NuevoBoton(rotulo, func() {
		if c.f.IrACelda == nil || !c.f.IrACelda(a) {
			if c.f.Avisos != nil {
				c.f.Avisos.Notificar(ui.NuevoAviso(ui.AvisoAdvertencia, c.t("comments.cellGone"), ""))
			}
		}
	}).Con(ui.Fantasma, ui.TallaXS)
	b.Atenuado = true
	return container.New(sangradoIzq{tema.Espacio(2)}, b)
}

// sangradoIzq es el `-ml-2` de `TEXT_BTN_INICIO`: el botón de texto se mete en su propio
// relleno para que el rótulo quede alineado con el texto del hilo.
type sangradoIzq struct{ m float32 }

// MinSize es la del hijo, menos lo que se mete a la izquierda.
func (s sangradoIzq) MinSize(objs []fyne.CanvasObject) fyne.Size {
	m := objs[0].MinSize()
	return fyne.NewSize(max(m.Width-s.m, 0), m.Height)
}

// Layout lo corre a la izquierda, a su medida.
func (s sangradoIzq) Layout(objs []fyne.CanvasObject, _ fyne.Size) {
	objs[0].Move(fyne.NewPos(-s.m, 0))
	objs[0].Resize(objs[0].MinSize())
}

// cabeceraComentario es el autor en `text-xs font-semibold` y la fecha en
// `sytue-meta-dense`, alineados por la línea base (`items-baseline gap-2`).
func (c *Comentarios) cabeceraComentario(x api.DocComment) fyne.CanvasObject {
	nombre := ui.NuevoRotulo(x.Autor(), tema.TextoXS, ui.SemiNegrita, "text")
	fecha := ui.NuevoRotulo(i18n.FormatDateTime(c.f.Loc, x.CreatedAt), tema.Texto2XS, ui.Normal, "text-subtle")
	return container.New(ui.Fila{Hueco: tema.Espacio(2)}, nombre,
		ConArriba(LineaBase(tema.TextoXS)-LineaBase(tema.Texto2XS), fecha))
}

// LineaBase es dónde cae la línea base dentro de la caja de línea de un escalón.
func LineaBase(e tema.Escalon) float32 { return (e.Linea-CajaFuente*e.Tam)/2 + AscFuente*e.Tam }

// respuestaDe es una respuesta: `border-border mt-2 border-l-2 pl-2`, con su cabecera y
// el botón de borrar a la derecha.
func (c *Comentarios) respuestaDe(x api.DocComment, ancho float32, tinta func(string) color.NRGBA, inerte bool) fyne.CanvasObject {
	borrar := botonBorrar(c.t("comments.deleteReply"), func() {
		c.mutar(func(ctx context.Context) error { return c.f.Svc.DeleteComment(ctx, c.f.ID, x.ID) }, "comments.deleteError", nil)
	})
	borrar.Inerte = inerte
	cab := container.New(FilaAcciones{}, c.cabeceraComentario(x), borrar)
	cuerpo := ConArriba(tema.Espacio(0.5), cuerpoComentario(x.Body, ancho-2-tema.Espacio(2), tinta("text")))
	barra := canvas.NewRectangle(tinta("border"))
	return container.New(&conBarra{ancho: 2, aire: tema.Espacio(2)}, barra, container.New(ui.Columna{}, cab, cuerpo))
}

// botonBorrar es el `IconButton size="sm" tone="danger"` con la papelera `sm` (14): gris
// en reposo, rojo solo al pasar por encima.
func botonBorrar(rotulo string, alPulsar func()) *ui.BotonIcono {
	b := ui.NuevoBotonIcono(iconos.Papelera, rotulo, alPulsar)
	b.Lado, b.TamIcono = ui.LadoIconoSM, 14
	b.TrazoEncima = "danger-text"
	return b
}

// cuerpoComentario es `text-sm text-text break-words whitespace-pre-wrap`: los saltos de
// línea del comentario se respetan.
func cuerpoComentario(texto string, ancho float32, tinta color.NRGBA) fyne.CanvasObject {
	lineas := strings.Split(texto, "\n")
	ps := make([]fyne.CanvasObject, 0, len(lineas))
	for _, l := range lineas {
		ps = append(ps, ui.NuevoParrafoConTinta(l, tema.TextoSM, ui.Normal, tinta).A(ancho))
	}
	return container.New(ui.Columna{}, ps...)
}

// citaComentario es `border-l-2 border-primary/50 pl-2 text-xs italic text-text-muted`.
//
// ponytail: en una línea, recortada; la web la reparte en varias y en cursiva
// sintética. Las citas son selecciones cortas casi siempre.
func citaComentario(texto string, ancho float32, tinta func(string) color.NRGBA) fyne.CanvasObject {
	t := ui.Texto(texto, tema.TextoXS, ui.Normal, tinta("text-muted"))
	t.FontSource = tema.Cursiva
	ui.Truncar(t, texto, tema.TextoXS, ui.Normal, ancho-2-tema.Espacio(2))
	barra := tinta("primary")
	barra.A /= 2
	return container.New(&conBarra{ancho: 2, aire: tema.Espacio(2)}, canvas.NewRectangle(barra),
		container.New(textoFijo{e: tema.TextoXS}, t))
}

// al es un color a esa opacidad sobre la superficie del panel.
func al(c color.NRGBA, o float32, v tema.Variante) color.NRGBA {
	c.A = uint8(float32(c.A) * o)
	return tokens.SobreSRGB(c, tema.Color("surface", v))
}

// ---- layouts pequeños del panel ----

// ConArriba es un margen de arriba (`mt-N`).
func ConArriba(m float32, o fyne.CanvasObject) fyne.CanvasObject {
	return container.New(layout.NewCustomPaddedLayout(m, 0, 0, 0), o)
}

// textoFijo coloca un `canvas.Text` en su caja de línea de CSS.
type textoFijo struct{ e tema.Escalon }

func (t textoFijo) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(objs[0].MinSize().Width, t.e.Linea)
}

func (t textoFijo) Layout(objs []fyne.CanvasObject, _ fyne.Size) {
	if tx, ok := objs[0].(*canvas.Text); ok {
		ui.ColocarTexto(tx, t.e, 0, 0)
	}
}

// conBarra es un `border-l-N pl-M`: la barra a la izquierda, del alto del contenido.
type conBarra struct{ ancho, aire float32 }

func (b *conBarra) MinSize(objs []fyne.CanvasObject) fyne.Size {
	m := objs[1].MinSize()
	return fyne.NewSize(m.Width+b.ancho+b.aire, m.Height)
}

func (b *conBarra) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(fyne.NewSize(b.ancho, t.Height))
	objs[1].Move(fyne.NewPos(b.ancho+b.aire, 0))
	objs[1].Resize(fyne.NewSize(max(t.Width-b.ancho-b.aire, 0), t.Height))
}

// FilaAcciones es `flex items-center gap-1` con un `flex-1` delante del último: los
// primeros a la izquierda, el último pegado a la derecha.
type FilaAcciones struct{}

func (FilaAcciones) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var w, h float32
	for _, o := range objs {
		m := o.MinSize()
		w += m.Width + tema.Espacio(1)
		h = max(h, m.Height)
	}
	return fyne.NewSize(w, h)
}

func (FilaAcciones) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	x := float32(0)
	for i, o := range objs {
		m := o.MinSize()
		if i == len(objs)-1 {
			x = max(x, t.Width-m.Width)
		}
		o.Move(fyne.NewPos(x, (t.Height-m.Height)/2))
		o.Resize(m)
		x += m.Width + tema.Espacio(1)
	}
}

// TarjetaComentario pone la caja con borde y, dentro, el contenido con `p-2`.
type TarjetaComentario struct{ Caja *ui.Caja }

func (t *TarjetaComentario) MinSize(objs []fyne.CanvasObject) fyne.Size {
	m := objs[len(objs)-1].MinSize()
	return fyne.NewSize(m.Width+2*(tema.Espacio(2)+tema.Borde), m.Height+2*(tema.Espacio(2)+tema.Borde))
}

func (t *TarjetaComentario) Layout(objs []fyne.CanvasObject, s fyne.Size) {
	t.Caja.Redimensionar(s)
	c := objs[len(objs)-1]
	m := tema.Espacio(2) + tema.Borde
	c.Move(fyne.NewPos(m, m))
	c.Resize(fyne.NewSize(max(s.Width-2*m, 0), max(s.Height-2*m, 0)))
}

// AscFuente y CajaFuente son las métricas verticales de Figtree (hhea): ascendente 950
// y descendente 250 por mil, que es la caja de 1,2 em que `canvas.Text` declara de alto.
// La maqueta del folio de Docus mide con las mismas.
const (
	AscFuente  = 0.95
	CajaFuente = 1.2
)

// zonaAncla es la tarjeta de un hilo con el «al pasar el ratón, se resalta su ancla» de la
// web. Es un refuerzo visual, no la vía de acceso: el hilo se abre con sus botones.
type zonaAncla struct {
	widget.BaseWidget
	contenido   fyne.CanvasObject
	entra, sale func()
}

func nuevaZonaAncla(contenido fyne.CanvasObject, entra, sale func()) *zonaAncla {
	z := &zonaAncla{contenido: contenido, entra: entra, sale: sale}
	z.ExtendBaseWidget(z)
	return z
}

func (z *zonaAncla) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(z.contenido)
}

func (z *zonaAncla) MouseIn(*desktop.MouseEvent)    { z.entra() }
func (z *zonaAncla) MouseMoved(*desktop.MouseEvent) {}
func (z *zonaAncla) MouseOut()                      { z.sale() }
