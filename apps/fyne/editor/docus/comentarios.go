// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"image/color"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/editor/docus/rico"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// El panel «Comentarios» (`CommentsPanel.tsx`): los hilos del documento —la raíz y sus
// respuestas, por fecha—, con responder, resolver o reabrir y borrar, y el formulario de
// un comentario nuevo al pie.
//
// ponytail: el ancla no se resalta en el folio al pasar por encima del hilo, como hace
// la web con `::highlight`: un comentario nuevo desde aquí no lleva ancla, y los que la
// traen de la web van a un `<mark data-comment-id>` que el folio todavía no entiende
// (un documento con marcas se abre en solo lectura). Relevar cuando `rico` entienda
// `<mark>`.

// anchoCuerpoPanel es lo que queda para el contenido de un panel: el panel menos su
// borde y el `p-3` de cada lado.
var anchoCuerpoPanel = editor.AnchoPanel - tema.Borde - 2*tema.Espacio(3)

// comentarios es el estado del panel de UN documento abierto.
type comentarios struct {
	a  *App
	ab *abierto

	lista        []api.DocComment
	respondiendo string // el hilo con el formulario de respuesta abierto
	pendiente    bool   // una petición en marcha: los botones se apagan (anti doble clic)

	nuevo     *ui.AreaTexto
	respuesta *ui.AreaTexto
	// menNuevo y menRespuesta son los selectores de menciones de las dos áreas.
	menNuevo, menRespuesta *menciones
	enviar                 *ui.Boton
	hilos                  *fyne.Container
	objeto                 fyne.CanvasObject
}

func (a *App) panelComentarios(ab *abierto) fyne.CanvasObject {
	if ab.comentarios == nil {
		ab.comentarios = nuevosComentarios(a, ab)
		ab.comentarios.recargar()
	}
	return ab.comentarios.objeto
}

func nuevosComentarios(a *App, ab *abierto) *comentarios {
	c := &comentarios{a: a, ab: ab}
	c.nuevo = ui.NuevaAreaTexto(a.t("comments.new.placeholder"))
	c.nuevo.Filas = 2
	c.menNuevo = nuevasMenciones(a, c.nuevo)
	c.nuevo.AlCambiar(func(x string) { c.pintarEnviar(); c.menNuevo.cambio(x) })
	c.respuesta = ui.NuevaAreaTexto(a.t("comments.reply.placeholder"))
	c.respuesta.Filas = 2
	c.menRespuesta = nuevasMenciones(a, c.respuesta)
	c.respuesta.AlCambiar(c.menRespuesta.cambio)
	c.enviar = ui.NuevoBoton(a.t("comments.new.send"), func() {
		cuerpo := strings.TrimSpace(c.nuevo.Texto())
		c.mutar(func(ctx context.Context) error {
			_, err := a.Svc.CreateComment(ctx, ab.id, "", cuerpo)
			return err
		}, "comments.addError", func() { c.nuevo.PonerTexto("") })
	}).Con(ui.Primario, ui.TallaSM)
	c.hilos = container.New(ui.Columna{Hueco: tema.Espacio(2)})
	// `flex min-h-full flex-col gap-2 p-3`, con el formulario al pie (`mt-auto`).
	pie := container.New(ui.Columna{Hueco: tema.Espacio(1.5)}, c.nuevo, c.menNuevo.caja, container.New(alDerecha{}, c.enviar))
	c.objeto = container.New(layout.NewCustomPaddedLayout(MargenCuerpo, MargenCuerpo, MargenCuerpo, MargenCuerpo),
		container.NewBorder(nil, container.New(layout.NewCustomPaddedLayout(tema.Espacio(1), 0, 0, 0), pie), nil, nil,
			container.NewVScroll(c.hilos)))
	c.pintar()
	return c
}

// recargar pide la lista (con los resueltos, que la web también enseña, atenuados).
func (c *comentarios) recargar() {
	if c.a.Svc == nil {
		return
	}
	a, ab := c.a, c.ab
	a.lanzar(func() {
		ctx, cancelar := context.WithTimeout(context.Background(), a.Plazo)
		defer cancelar()
		lista, err := a.Svc.Comments(ctx, ab.id, true)
		fyne.Do(func() {
			if err == nil {
				c.lista = lista
			}
			c.pintar()
		})
	})
}

// mutar corre una petición con el candado de la web: mientras dura, todo se apaga; al
// acabar se recarga la lista, y si falla se dice con un aviso.
func (c *comentarios) mutar(hacer func(context.Context) error, claveError string, alAcabar func()) {
	if c.pendiente || c.a.Svc == nil {
		return
	}
	c.pendiente = true
	c.pintar()
	a := c.a
	a.lanzar(func() {
		ctx, cancelar := context.WithTimeout(context.Background(), a.Plazo)
		defer cancelar()
		err := hacer(ctx)
		fyne.Do(func() {
			c.pendiente = false
			if err != nil {
				a.avisos.Notificar(ui.NuevoAviso(ui.AvisoPeligro, a.t(claveError), ""))
				c.pintar()
				return
			}
			if alAcabar != nil {
				alAcabar()
			}
			c.recargar()
		})
	})
}

func (c *comentarios) sinRed() bool { return c.ab.ctx.SinRed }

func (c *comentarios) pintarEnviar() {
	c.enviar.Inerte = c.pendiente || c.sinRed() || strings.TrimSpace(c.nuevo.Texto()) == ""
	c.enviar.Refresh()
}

// hilos agrupa: las raíces por fecha y, de cada una, sus respuestas por fecha. Una
// respuesta cuyo padre ya no está se enseña como raíz, como en la web.
func hilos(lista []api.DocComment) ([]api.DocComment, map[string][]api.DocComment) {
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
func (c *comentarios) pintar() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	raices, resp := hilos(c.lista)
	c.hilos.Objects = nil
	if len(raices) == 0 {
		c.hilos.Add(ui.NuevoParrafo(c.a.t("comments.empty"), tema.TextoSM, ui.Normal, "text-muted", v).A(anchoCuerpoPanel))
	}
	for _, r := range raices {
		c.hilos.Add(c.hilo(r, resp[r.ID], v))
	}
	c.hilos.Refresh()
	c.pintarEnviar()
}

// hilo es el `<article class="border-border rounded-md border p-2">` de un comentario raíz.
func (c *comentarios) hilo(r api.DocComment, respuestas []api.DocComment, v tema.Variante) fyne.CanvasObject {
	a := c.a
	ancho := anchoCuerpoPanel - 2*tema.Borde - 2*tema.Espacio(2)
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
	if cita := r.AnchorText(); cita != "" {
		filas = append(filas, conArriba(tema.Espacio(1), citaComentario("«"+cita+"»", ancho, tinta)))
	}
	filas = append(filas, conArriba(tema.Espacio(1), cuerpoComentario(r.Body, ancho, tinta("text"))))

	responder := ui.NuevoBoton(a.t("comments.reply"), func() {
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
		c.ab.folio.ResaltarAncla("")
		c.ab.folio.QuitarAncla(r.AnchorMark())
	}
	resolver := ui.NuevoBoton(a.t(claveResolver), func() {
		alAcabar := quitarAncla
		if r.Resolved {
			alAcabar = nil
		}
		c.mutar(func(ctx context.Context) error {
			return a.Svc.ResolveComment(ctx, c.ab.id, r.ID, !r.Resolved)
		}, "comments.resolveError", alAcabar)
	}).Con(ui.Fantasma, ui.TallaXS).ConIcono(iconos.Tic, 14)
	resolver.Atenuado = true
	borrar := botonBorrar(a.t("comments.delete"), func() {
		c.mutar(func(ctx context.Context) error { return a.Svc.DeleteComment(ctx, c.ab.id, r.ID) }, "comments.deleteError", quitarAncla)
	})
	responder.Inerte, resolver.Inerte, borrar.Inerte = inerte, inerte, inerte
	filas = append(filas, conArriba(tema.Espacio(1.5), container.New(filaAcciones{}, responder, resolver, borrar)))

	for _, x := range respuestas {
		filas = append(filas, conArriba(tema.Espacio(2), c.respuestaDe(x, ancho, tinta, inerte)))
	}
	if c.respondiendo == r.ID {
		cancelar := ui.NuevoBoton(a.t("action.cancel"), func() {
			c.respondiendo = ""
			c.respuesta.PonerTexto("")
			c.pintar()
		}).Con(ui.Fantasma, ui.TallaSM)
		enviar := ui.NuevoBoton(a.t("comments.reply.send"), func() {
			cuerpo := strings.TrimSpace(c.respuesta.Texto())
			if cuerpo == "" {
				return
			}
			c.mutar(func(ctx context.Context) error {
				_, err := a.Svc.CreateComment(ctx, c.ab.id, r.ID, cuerpo)
				return err
			}, "comments.reply.error", func() {
				c.respondiendo = ""
				c.respuesta.PonerTexto("")
			})
		}).Con(ui.Primario, ui.TallaSM)
		enviar.Inerte = inerte
		filas = append(filas, conArriba(tema.Espacio(2), container.New(ui.Columna{Hueco: tema.Espacio(1.5)},
			c.respuesta, c.menRespuesta.caja, container.New(alDerecha{}, container.New(ui.Fila{Hueco: tema.Espacio(1.5)}, cancelar, enviar)))))
	}
	contenido := container.New(ui.Columna{}, filas...)
	// Sin fondo propio se ve el del panel: un relleno transparente dejaría ver el borde,
	// que la caja pinta DEBAJO del relleno (ver `ui.Caja`), y la tarjeta salía gris.
	caja := ui.NuevaCaja(tema.Color("surface", v), tema.Color("border", v), tema.RadioMD)
	tarjeta := container.New(&tarjetaComentario{caja: caja}, append(caja.Objetos(), contenido)...)
	marca := r.AnchorMark()
	if marca == "" {
		return tarjeta
	}
	return nuevaZonaAncla(tarjeta, func() { c.ab.folio.ResaltarAncla(marca) }, func() { c.ab.folio.ResaltarAncla("") })
}

// cabeceraComentario es el autor en `text-xs font-semibold` y la fecha en
// `sytue-meta-dense`, alineados por la línea base (`items-baseline gap-2`).
func (c *comentarios) cabeceraComentario(x api.DocComment) fyne.CanvasObject {
	nombre := ui.NuevoRotulo(x.Autor(), tema.TextoXS, ui.SemiNegrita, "text")
	fecha := ui.NuevoRotulo(i18n.FormatDateTime(c.a.loc, x.CreatedAt), tema.Texto2XS, ui.Normal, "text-subtle")
	return container.New(ui.Fila{Hueco: tema.Espacio(2)}, nombre,
		conArriba(lineaBase(tema.TextoXS)-lineaBase(tema.Texto2XS), fecha))
}

// lineaBase es dónde cae la línea base dentro de la caja de línea de un escalón.
func lineaBase(e tema.Escalon) float32 { return (e.Linea-cajaFuente*e.Tam)/2 + ascFuente*e.Tam }

// respuestaDe es una respuesta: `border-border mt-2 border-l-2 pl-2`, con su cabecera y
// el botón de borrar a la derecha.
func (c *comentarios) respuestaDe(x api.DocComment, ancho float32, tinta func(string) color.NRGBA, inerte bool) fyne.CanvasObject {
	a := c.a
	borrar := botonBorrar(a.t("comments.deleteReply"), func() {
		c.mutar(func(ctx context.Context) error { return a.Svc.DeleteComment(ctx, c.ab.id, x.ID) }, "comments.deleteError", nil)
	})
	borrar.Inerte = inerte
	cab := container.New(filaAcciones{}, c.cabeceraComentario(x), borrar)
	cuerpo := conArriba(tema.Espacio(0.5), cuerpoComentario(x.Body, ancho-2-tema.Espacio(2), tinta("text")))
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

// conArriba es un margen de arriba (`mt-N`).
func conArriba(m float32, o fyne.CanvasObject) fyne.CanvasObject {
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

// filaAcciones es `flex items-center gap-1` con un `flex-1` delante del último: los
// primeros a la izquierda, el último pegado a la derecha.
type filaAcciones struct{}

func (filaAcciones) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var w, h float32
	for _, o := range objs {
		m := o.MinSize()
		w += m.Width + tema.Espacio(1)
		h = max(h, m.Height)
	}
	return fyne.NewSize(w, h)
}

func (filaAcciones) Layout(objs []fyne.CanvasObject, t fyne.Size) {
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

// tarjetaComentario pone la caja con borde y, dentro, el contenido con `p-2`.
type tarjetaComentario struct{ caja *ui.Caja }

func (t *tarjetaComentario) MinSize(objs []fyne.CanvasObject) fyne.Size {
	m := objs[len(objs)-1].MinSize()
	return fyne.NewSize(m.Width+2*(tema.Espacio(2)+tema.Borde), m.Height+2*(tema.Espacio(2)+tema.Borde))
}

func (t *tarjetaComentario) Layout(objs []fyne.CanvasObject, s fyne.Size) {
	t.caja.Redimensionar(s)
	c := objs[len(objs)-1]
	m := tema.Espacio(2) + tema.Borde
	c.Move(fyne.NewPos(m, m))
	c.Resize(fyne.NewSize(max(s.Width-2*m, 0), max(s.Height-2*m, 0)))
}

// ---- el ancla en el folio ----

// Anclar envuelve la selección en el `<mark data-comment-id>` del comentario, bloque a
// bloque, y devuelve lo citado con los blancos juntos (`range.toString()` normalizado),
// o "" si no hay selección de texto.
func (f *Folio) Anclar(id string) string {
	a, b := f.orden()
	cita := strings.Join(strings.Fields(strings.ReplaceAll(f.textoSeleccionado(), " ", " ")), " ")
	if !f.haySeleccion() || cita == "" || !f.editable() {
		return ""
	}
	f.porBloques(a, b, func(bl rico.Bloque, e *rico.Edicion) bool {
		i := rico.Indice(f.bloques, bl.Lugar)
		desde, hasta := 0, e.Largo()
		if i == a.b {
			desde = a.p
		}
		if i == b.b {
			hasta = b.p
		}
		if desde >= hasta {
			return false
		}
		e.Anclar(desde, hasta, id)
		return true
	})
	f.reestructurado()
	return cita
}

// QuitarAncla quita el resaltado de un comentario del folio (al resolverlo, al borrarlo o
// si no se pudo crear), como `removeCommentMark`.
func (f *Folio) QuitarAncla(id string) {
	if id == "" || !f.editable() {
		return
	}
	antes := f.doc.HTML()
	if f.doc.QuitarAncla(id) {
		f.hist.atras = append(f.hist.atras, paso{antes, f.cur, f.anc})
		f.reestructurado()
	}
}

// ResaltarAncla pinta lo anclado a un comentario con el azul de la web (`#bfdbfe`, el
// `::highlight(sytue-comment-anchor)` del panel) y lo lleva a la vista; "" lo quita.
func (f *Folio) ResaltarAncla(id string) {
	f.ancla = nil
	for i, b := range f.bloques {
		if desde, hasta, ok := b.Ancla(id); ok && id != "" {
			f.ancla = append(f.ancla, coincidencia{i, desde, hasta})
		}
	}
	f.pag.Refresh()
}

// comentarAnclado es «Nuevo comentario» con el cuerpo ya escrito: ancla la selección (si
// la hay) y crea el hilo con el ancla `{ mark, text }`; si el servicio falla, el
// resaltado se quita, que no quede huérfano. Es `nuevoComentario` de Colaboracion.tsx.
func (a *App) comentarAnclado(ab *abierto, cuerpo string) {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	marca := hex.EncodeToString(b)
	cita := ab.folio.Anclar(marca)
	a.lanzar(func() {
		c, cancelar := context.WithTimeout(context.Background(), a.Plazo)
		defer cancelar()
		var err error
		if cita != "" {
			_, err = a.Svc.CreateAnchoredComment(c, ab.id, cuerpo, marca, cita)
		} else {
			_, err = a.Svc.CreateComment(c, ab.id, "", cuerpo)
		}
		fyne.Do(func() {
			if err != nil {
				ab.folio.QuitarAncla(marca)
				a.avisos.Notificar(ui.NuevoAviso(ui.AvisoPeligro, a.t("comments.addError"), ""))
				return
			}
			a.avisos.Notificar(ui.NuevoAviso(ui.AvisoExito, a.t("comments.added"), ""))
			// Se abre el panel para ver el hilo recién creado.
			ab.ctx.Panel = editor.PanelComentarios
			ab.marco.Recomponer()
			if ab.comentarios != nil {
				ab.comentarios.recargar()
			}
		})
	})
}

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
