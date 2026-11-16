// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"strings"
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// El hilo de comentarios de un registro, en un panel a la derecha: el espejo de
// `PanelComentarios.tsx`. Lee quien lee el registro; escribe el comentarista hacia
// arriba; borra quien lo escribió o quien modera. Lo que se pinta sale del rol y de
// `CanDelete`, pero cada operación la vuelve a autorizar el servicio.
//
// El texto de un comentario es TEXTO: va en un párrafo, nunca se interpreta.

// maxComentario es `MAX_COMENTARIO` de la web (el servicio manda: `MaxLongComentario`).
const maxComentario = 2000

// hilo es un comentario raíz con sus respuestas.
type hilo struct {
	raiz       api.BasesComment
	respuestas []api.BasesComment
}

// hilosDe es `hilosDe` de lib/comentarios.ts: agrupa en hilos de un nivel conservando
// el orden. Una respuesta sin su raíz se enseña como raíz.
func hilosDe(cs []api.BasesComment) []*hilo {
	var hilos []*hilo
	porID := map[string]*hilo{}
	for _, c := range cs {
		if padre := porID[c.ParentID]; c.ParentID != "" && padre != nil {
			padre.respuestas = append(padre.respuestas, c)
			continue
		}
		h := &hilo{raiz: c}
		hilos = append(hilos, h)
		porID[c.ID] = h
	}
	return hilos
}

// puedeComentar es el rol comentarista o superior.
func puedeComentar(rol string) bool {
	return rol == api.BasesCommenter || puedeEscribir(rol)
}

// panelComentarios es el estado del panel abierto.
type panelComentarios struct {
	p        *Pantalla
	registro string
	puede    bool
	// lista es nil mientras se carga.
	lista        []api.BasesComment
	respondiendo *api.BasesComment
	fallo        string
	enviando     bool
	// texto es el MISMO entre repintados: lo escrito no se pierde porque llegue la lista.
	texto *ui.AreaTexto
	d     *ui.Dialogo
	// alContar avisa del número de comentarios (sin lápidas) cada vez que llega la lista.
	alContar func(n int)
}

// abrirComentarios abre el panel del registro `r`. `nombre` es cómo se llama (su primer
// campo con valor) y `alContar` recibe el número cada vez que se carga.
func (p *Pantalla) abrirComentarios(r api.BasesRecord, nombre string, alContar func(int)) *panelComentarios {
	b := p.abierta
	c := &panelComentarios{p: p, registro: r.ID, alContar: alContar}
	p.comentando = c
	if b != nil {
		c.puede = puedeComentar(b.rol())
	}
	c.texto = ui.NuevaAreaTexto("")
	c.texto.Rotulo, c.texto.Filas = p.t("comments.write"), 3
	c.texto.AlCambiar(func(s string) {
		if utf8.RuneCountInString(s) > maxComentario { // el `maxLength` del textarea
			c.texto.PonerTexto(string([]rune(s)[:maxComentario]))
		}
		c.ajustarPie()
	})
	c.d = ui.NuevoDialogo(p.t("comments.open", i18n.Vars{"name": nombre}), container.New(ui.Columna{})).ALaDerecha()
	c.d.Con(ui.DialogoMD, container.New(ui.Fila{Hueco: ui.HuecoPieDialogo}))
	c.d.RotuloCerrar = p.t("ui.close")
	if c.puede {
		c.d.Foco = c.texto
	}
	c.rehacer()
	c.d.Mostrar(p.ventana.Canvas())
	c.cargar()
	return c
}

// cargar pide la lista de nuevo.
func (c *panelComentarios) cargar() {
	p := c.p
	if p.svc == nil {
		return
	}
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		lista, err := p.svc.ListComments(ctx, c.registro)
		enHilo(func() {
			if err != nil {
				c.lista, c.fallo = []api.BasesComment{}, p.mensajeDeError(err)
			} else {
				c.lista, c.fallo = lista, ""
				if lista == nil {
					c.lista = []api.BasesComment{}
				}
			}
			if c.alContar != nil {
				c.alContar(contarComentarios(c.lista))
			}
			c.rehacer()
		})
	})
}

// contarComentarios cuenta los que se leen: una lápida no es un comentario.
func contarComentarios(cs []api.BasesComment) int {
	n := 0
	for _, c := range cs {
		if !c.Deleted {
			n++
		}
	}
	return n
}

// enviar publica lo escrito, como respuesta si se está respondiendo.
func (c *panelComentarios) enviar() {
	p := c.p
	cuerpo := strings.TrimSpace(c.texto.Texto())
	if cuerpo == "" || c.enviando || p.svc == nil {
		return
	}
	padre := ""
	if c.respondiendo != nil {
		padre = c.respondiendo.ID
	}
	c.enviando, c.fallo = true, ""
	c.rehacer()
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		_, err := p.svc.CreateComment(ctx, c.registro, cuerpo, padre)
		enHilo(func() {
			c.enviando = false
			if err != nil {
				c.fallo = p.mensajeDeError(err)
				c.rehacer()
				return
			}
			c.texto.PonerTexto("")
			c.respondiendo = nil
			c.cargar()
		})
	})
}

// borrar pregunta y borra un comentario.
func (c *panelComentarios) borrar(x api.BasesComment) {
	p := c.p
	p.confirmar(p.t("comments.deleteConfirm"), p.t("common.delete"), func() {
		if p.svc == nil {
			return
		}
		p.fondo(func() {
			ctx, cancelar := p.ctx()
			defer cancelar()
			err := p.svc.DeleteComment(ctx, x.ID)
			enHilo(func() {
				if err != nil {
					c.fallo = p.mensajeDeError(err)
					c.rehacer()
					return
				}
				if c.respondiendo != nil && c.respondiendo.ID == x.ID {
					c.respondiendo = nil
				}
				c.cargar()
			})
		})
	})
}

func (c *panelComentarios) autor(x api.BasesComment) string {
	if x.AuthorName != "" {
		return x.AuthorName
	}
	return c.p.t("comments.unknownAuthor")
}

// comentario pinta uno: autor, fecha, el texto como texto y sus acciones.
func (c *panelComentarios) comentario(x api.BasesComment, esRaiz bool, v tema.Variante, ancho float32) fyne.CanvasObject {
	p := c.p
	if x.Deleted {
		return ui.NuevoParrafo(p.t("comments.deleted"), tema.TextoSM, ui.Cursiva, "text-muted", v).A(ancho)
	}
	cabecera := container.New(ui.Fila{Hueco: tema.Espacio(2)},
		ui.NuevoRotulo(c.autor(x), tema.TextoSM, ui.SemiNegrita, "text"),
		ui.NuevoRotulo(i18n.FormatDateTime(p.tr.Locale(), x.CreatedAt), tema.TextoXS, ui.Normal, "text-muted"),
	)
	hijos := []fyne.CanvasObject{cabecera, ui.NuevoParrafo(x.Body, tema.TextoSM, ui.Normal, "text", v).A(ancho)}
	var acciones []fyne.CanvasObject
	if esRaiz && c.puede {
		acciones = append(acciones, ui.NuevoBoton(p.t("comments.reply"), func() {
			r := x
			c.respondiendo = &r
			c.rehacer()
			p.ventana.Canvas().Focus(c.texto)
		}).Con(ui.Fantasma, ui.TallaSM))
	}
	if x.CanDelete {
		acciones = append(acciones, ui.NuevoBoton(p.t("comments.delete"), func() { c.borrar(x) }).Con(ui.Fantasma, ui.TallaSM))
	}
	if len(acciones) > 0 {
		hijos = append(hijos, container.New(ui.Fila{Hueco: tema.Espacio(1)}, acciones...))
	}
	return container.New(ui.Columna{Hueco: tema.Espacio(1)}, hijos...)
}

// recuadro es `border-border rounded-md border p-3`.
func recuadro(v tema.Variante, dentro fyne.CanvasObject) fyne.CanvasObject {
	caja := canvas.NewRectangle(tema.Color("surface", v))
	caja.StrokeColor, caja.StrokeWidth, caja.CornerRadius = tema.Color("border", v), tema.Borde, tema.RadioMD
	m := tema.Espacio(3)
	return container.NewStack(caja, container.New(ui.Margen{Izq: m, Arr: m, Der: m, Abj: m}, dentro))
}

// rehacer vuelve a armar el cuerpo y el pie con el estado de ahora.
func (c *panelComentarios) rehacer() {
	p := c.p
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	ancho := p.anchoDialogo(560)
	var hijos []fyne.CanvasObject
	switch {
	case c.lista == nil:
		hijos = append(hijos, ui.NuevoRotulo(p.t("ui.loading"), tema.TextoSM, ui.Normal, "text-muted"))
	case len(c.lista) == 0:
		hijos = append(hijos, ui.NuevoRotulo(p.t("comments.empty"), tema.TextoSM, ui.Normal, "text-muted"))
	default:
		dentro := ancho - 2*tema.Espacio(3)
		for _, h := range hilosDe(c.lista) {
			partes := []fyne.CanvasObject{c.comentario(h.raiz, true, v, dentro)}
			for _, r := range h.respuestas {
				// `border-l-2 pl-3`: las respuestas, sangradas tras una raya.
				raya := canvas.NewRectangle(tema.Color("border", v))
				raya.SetMinSize(fyne.NewSize(2, 1))
				fila := container.NewBorder(nil, nil,
					container.New(ui.Margen{Der: tema.Espacio(3)}, raya),
					nil, c.comentario(r, false, v, dentro-tema.Espacio(3)-2))
				partes = append(partes, fila)
			}
			hijos = append(hijos, recuadro(v, container.New(ui.Columna{Hueco: tema.Espacio(2)}, partes...)))
		}
	}
	if c.fallo != "" {
		hijos = append(hijos, ui.NuevoParrafo(c.fallo, tema.TextoSM, ui.Normal, "danger-text", v).A(ancho))
	}
	if c.puede {
		if c.respondiendo != nil {
			hijos = append(hijos, container.New(ui.Fila{Hueco: tema.Espacio(2)},
				ui.NuevoRotulo(p.t("comments.replyingTo", i18n.Vars{"name": c.autor(*c.respondiendo)}), tema.TextoXS, ui.Normal, "text-muted"),
				ui.NuevoBoton(p.t("comments.cancelReply"), func() { c.respondiendo = nil; c.rehacer() }).Con(ui.Fantasma, ui.TallaSM),
			))
		}
		c.texto.Ancho = ancho
		// La ayuda va como párrafo y no como `help` del grupo: es larga y tiene que partir.
		hijos = append(hijos, container.New(ui.Columna{Hueco: tema.Espacio(1)},
			ui.GrupoCampo(p.t("comments.write"), false, c.texto, "", ""),
			ui.NuevoParrafo(p.t("comments.mentionHelp"), tema.TextoXS, ui.Normal, "text-muted", v).A(ancho)))
	} else {
		hijos = append(hijos, ui.NuevoParrafo(p.t("comments.readOnly"), tema.TextoSM, ui.Normal, "text-muted", v).A(ancho))
	}
	c.d.CambiarCuerpo(container.New(ui.Columna{Hueco: tema.Espacio(4)}, hijos...))
	c.ajustarPie()
}

// ajustarPie pone «Comentar» (inerte sin texto o enviando) o «Cerrar» a quien solo lee.
func (c *panelComentarios) ajustarPie() {
	p := c.p
	if !c.puede {
		c.d.CambiarPie(container.New(ui.Fila{Hueco: ui.HuecoPieDialogo}, ui.NuevoBoton(p.t("ui.close"), func() { c.d.Cerrar() })))
		return
	}
	enviar := ui.NuevoBoton(p.t("comments.send"), c.enviar)
	enviar.Inerte = c.enviando || strings.TrimSpace(c.texto.Texto()) == ""
	c.d.CambiarPie(container.New(ui.Fila{Hueco: ui.HuecoPieDialogo}, enviar))
}
