// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"errors"
	"net/http"
	"slices"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Denunciar y bloquear (ADR 0073). Contrapartida de Moderation.tsx y de lo que
// ChatApp.tsx cablea alrededor: el diálogo de denuncia, la lista de personas
// bloqueadas, la cola de quien modera y el plegado de los mensajes de una persona
// bloqueada, dentro de la pantalla de canal declarada en pantalla.go.
//
// El bloqueo lo APLICA el servicio (no entrega directos ni avisos); aquí la lista
// sirve para plegar y para enseñar «Desbloquear».

// ServicioDenuncias es lo que esto pide al servicio. Interfaz aparte y no un añadido a
// `Servicio`: esa la cumplen los dobles de todos los guardianes de la sala, y
// ampliarla obligaría a tocarlos para métodos que no usan. Sin ella —un doble— la
// sala funciona igual y no ofrece ni denunciar ni bloquear.
type ServicioDenuncias interface {
	ReportMessage(ctx context.Context, mensaje, motivo, nota string) error
	ReportMember(ctx context.Context, canal, persona, motivo, nota string) error
	Block(ctx context.Context, canal, persona string) (api.RoomsBlock, error)
	Blocks(ctx context.Context) ([]api.RoomsBlock, error)
	Unblock(ctx context.Context, persona string) error
	Reports(ctx context.Context) ([]api.RoomsReport, error)
	ResolveReport(ctx context.Context, id, accion string) error
}

var _ ServicioDenuncias = (*api.Rooms)(nil)

func (sa *Sala) denuncias() ServicioDenuncias {
	m, _ := sa.S.(ServicioDenuncias)
	return m
}

// claveMotivo es `REASON_KEY`.
var claveMotivo = map[string]string{
	"harassment": "report.reasonHarassment", "sexual": "report.reasonSexual",
	"violence": "report.reasonViolence", "spam": "report.reasonSpam", "other": "report.reasonOther",
}

func (sa *Sala) rotuloMotivo(motivo string) string {
	if k, hay := claveMotivo[motivo]; hay {
		return sa.P.T.T(k, nil)
	}
	return motivo
}

// cargarBloqueados lee a quién se tiene bloqueado. Si falla, nada se pliega: el
// servicio lo sigue aplicando.
func (sa *Sala) cargarBloqueados() {
	mod := sa.denuncias()
	if mod == nil {
		return
	}
	pedir(sa, mod.Blocks, func(bs []api.RoomsBlock, err error) {
		if err != nil {
			return
		}
		sa.bloqueados = bs
		sa.repintarPorBloqueo()
	})
}

func (sa *Sala) bloqueado(persona string) bool {
	return slices.ContainsFunc(sa.bloqueados, func(b api.RoomsBlock) bool { return b.Subject == persona })
}

// plegado dice si un mensaje se ve plegado: es de una persona bloqueada y no se ha
// pedido verlo.
func (sa *Sala) plegado(autor, mensaje string) bool {
	return sa.bloqueado(autor) && !sa.mostrados[mensaje]
}

func (sa *Sala) repintarPorBloqueo() {
	if sa.abierto == "" {
		return
	}
	sa.pintarMensajes()
	sa.P.Canal.Historial.Refresh()
}

// entradasModeracion son las del «…» de un mensaje AJENO y de una persona: denunciar
// el mensaje, bloquear o desbloquear a su autor y, si está plegado, mostrarlo.
//
// ponytail: la web enseña «Mostrar» dentro del propio mensaje plegado; aquí va en su
// menú. Pasa a la fila cuando el historial admita un control dentro del cuerpo.
func (sa *Sala) entradasModeracion(m Mensaje) []fyne.CanvasObject {
	if sa.denuncias() == nil || m.Autor.Bot || m.Autor.ID == sa.Yo || sa.Yo == "" {
		return nil
	}
	t := sa.P.T
	out := []fyne.CanvasObject{ui.NuevoSeparadorMenu()}
	if sa.plegado(m.Autor.ID, m.ID) {
		out = append(out, ui.NuevoElementoMenu(t.T("block.show", nil), iconos.Ninguno, func() {
			if sa.mostrados == nil {
				sa.mostrados = map[string]bool{}
			}
			sa.mostrados[m.ID] = true
			sa.repintarPorBloqueo()
		}))
	}
	denunciar := ui.NuevoElementoMenu(t.T("report.message", nil), iconos.Ninguno, func() {
		sa.AbrirDenuncia(m.ID, "", "", m.Autor.Nombre)
	})
	denunciar.Peligro = true
	return append(out, denunciar, sa.entradaBloqueo(sa.abierto, m.Autor.ID, m.Autor.Nombre))
}

// entradaBloqueo es «Bloquear a X» o «Desbloquear a X».
func (sa *Sala) entradaBloqueo(canal, persona, nombre string) *ui.ElementoMenu {
	t := sa.P.T
	ya := sa.bloqueado(persona)
	rotulo := t.T("block.block", i18n.Vars{"name": nombre})
	if ya {
		rotulo = t.T("block.unblock", i18n.Vars{"name": nombre})
	}
	it := ui.NuevoElementoMenu(rotulo, iconos.Ninguno, func() { sa.AlternarBloqueo(canal, persona, nombre, ya) })
	it.Peligro = !ya
	return it
}

// AbrirDenuncia es `ReportModal`: el motivo, de la lista cerrada, y una nota
// opcional. Con `mensaje` se denuncia el mensaje; sin él, a `persona` en `canal`.
func (sa *Sala) AbrirDenuncia(mensaje, canal, persona, nombre string) {
	mod := sa.denuncias()
	lienzo := fyne.CurrentApp().Driver().CanvasForObject(sa.P.Objeto())
	if mod == nil || lienzo == nil {
		return
	}
	t := sa.P.T
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	titulo := t.T("report.message", nil)
	if mensaje == "" {
		titulo = t.T("report.person", i18n.Vars{"name": nombre})
	}

	var d *ui.Dialogo
	motivo := ""
	radios := make([]*ui.Eleccion, 0, len(api.MotivosDeDenuncia))
	filas := []fyne.CanvasObject{
		ui.NuevoParrafo(t.T("report.intro", nil), tema.TextoSM, ui.Normal, "text-muted", v),
		ui.NuevoParrafo(t.T("report.reason", nil), tema.TextoSM, ui.SemiNegrita, "text", v),
	}
	for _, clave := range api.MotivosDeDenuncia {
		r := ui.NuevoRadio(sa.rotuloMotivo(clave), nil)
		r.Escalon = tema.TextoSM
		r.AlCambiar = func(bool) {
			motivo = clave
			for i, otro := range radios {
				otro.Marcada = api.MotivosDeDenuncia[i] == clave
				otro.Refresh()
			}
			d.SinConfirmar(false)
		}
		radios = append(radios, r)
		filas = append(filas, r)
	}
	nota := ui.NuevaAreaTexto("")
	nota.Filas = 3
	filas = append(filas, ui.NuevoParrafo(t.T("report.note", nil), tema.TextoSM, ui.SemiNegrita, "text", v), nota)

	d = ui.Confirmacion(titulo, t.T("action.cancel", nil), t.T("report.submit", nil), false,
		container.New(ui.Columna{Hueco: tema.Espacio(2)}, filas...), nil, func() {
			texto := []rune(nota.Texto())
			texto = texto[:min(len(texto), 500)]
			d.Ocupado(true)
			pedir(sa, func(ctx context.Context) (struct{}, error) {
				if mensaje != "" {
					return struct{}{}, mod.ReportMessage(ctx, mensaje, motivo, string(texto))
				}
				return struct{}{}, mod.ReportMember(ctx, canal, persona, motivo, string(texto))
			}, func(_ struct{}, err error) {
				d.Ocupado(false)
				if err != nil {
					clave := "report.failed"
					if estadoHTTP(err) == http.StatusTooManyRequests {
						clave = "report.tooMany"
					}
					sa.P.Avisar(ui.AvisoPeligro, t.T(clave, nil))
					return
				}
				d.Cerrar()
				sa.P.Avisar(ui.AvisoExito, t.T("report.sent", nil))
			})
		})
	d.RotuloCerrar = t.T("action.close", nil)
	d.SinConfirmar(true)
	sa.pregunta = d
	d.Mostrar(lienzo)
}

// estadoHTTP es el código con que contestó el servicio, o 0.
func estadoHTTP(err error) int {
	var e *api.Error
	if errors.As(err, &e) {
		return e.Status
	}
	return 0
}

// AlternarBloqueo es `toggleBlock`: desbloquea sin preguntar; para bloquear confirma
// antes, diciendo qué pasa y que no se borra nada.
func (sa *Sala) AlternarBloqueo(canal, persona, nombre string, bloqueado bool) {
	mod := sa.denuncias()
	lienzo := fyne.CurrentApp().Driver().CanvasForObject(sa.P.Objeto())
	if mod == nil || lienzo == nil {
		return
	}
	t := sa.P.T
	n := i18n.Vars{"name": nombre}
	if bloqueado {
		pedir(sa, func(ctx context.Context) (struct{}, error) { return struct{}{}, mod.Unblock(ctx, persona) },
			func(_ struct{}, err error) {
				if err != nil {
					sa.P.Avisar(ui.AvisoPeligro, t.T("block.failed", nil))
					return
				}
				sa.bloqueados = slices.DeleteFunc(sa.bloqueados, func(b api.RoomsBlock) bool { return b.Subject == persona })
				sa.repintarPorBloqueo()
				sa.P.Avisar(ui.AvisoExito, t.T("block.unblocked", n))
			})
		return
	}
	var d *ui.Dialogo
	cuerpo := ui.NuevoParrafo(t.T("block.explain", nil), tema.TextoSM, ui.Normal, "text",
		tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	// El nombre solo en el título; el botón, «Bloquear» a secas (como la web).
	d = ui.Confirmacion(t.T("block.confirm", n), t.T("action.cancel", nil), t.T("block.blockShort", nil), true,
		cuerpo, nil, func() {
			d.Cerrar()
			pedir(sa, func(ctx context.Context) (api.RoomsBlock, error) { return mod.Block(ctx, canal, persona) },
				func(b api.RoomsBlock, err error) {
					if err != nil {
						sa.P.Avisar(ui.AvisoPeligro, t.T("block.failed", nil))
						return
					}
					sa.bloqueados = append([]api.RoomsBlock{b},
						slices.DeleteFunc(sa.bloqueados, func(x api.RoomsBlock) bool { return x.Subject == persona })...)
					sa.repintarPorBloqueo()
					sa.P.Avisar(ui.AvisoExito, t.T("block.blocked", n))
				})
		})
	d.RotuloCerrar = t.T("action.close", nil)
	sa.pregunta = d
	d.Mostrar(lienzo)
}

// filaConBoton es una fila con el texto a la izquierda y sus botones a la derecha.
func filaConBoton(texto fyne.CanvasObject, botones ...fyne.CanvasObject) fyne.CanvasObject {
	return container.New(ui.FilaAuto{Hueco: tema.Espacio(2), Derecha: 1, Partir: true, SaltoFila: tema.Espacio(2), Elastico: 1},
		append([]fyne.CanvasObject{texto}, botones...)...)
}

// AbrirBloqueados es `BlockedModal`: a quién se tiene bloqueado, con «Desbloquear».
func (sa *Sala) AbrirBloqueados() {
	lienzo := fyne.CurrentApp().Driver().CanvasForObject(sa.P.Objeto())
	if sa.denuncias() == nil || lienzo == nil {
		return
	}
	t := sa.P.T
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	filas := []fyne.CanvasObject{ui.NuevoParrafo(t.T("block.explain", nil), tema.TextoSM, ui.Normal, "text-muted", v)}
	if len(sa.bloqueados) == 0 {
		filas = append(filas, ui.NuevoParrafo(t.T("block.listEmpty", nil), tema.TextoSM, ui.Normal, "text-subtle", v))
	}
	var d *ui.Dialogo
	for _, b := range sa.bloqueados {
		nombre := b.Name
		if nombre == "" {
			nombre = b.Subject
		}
		quitar := ui.NuevoBoton(t.T("block.unblockShort", nil), func() {
			d.Cerrar()
			sa.AlternarBloqueo("", b.Subject, nombre, true)
		}).Con(ui.Secundario, ui.TallaSM)
		// Como aria-label en la web: el nombre completo se anuncia, pero no
		// ensancha el botón hasta dejar el nombre de la persona sin sitio.
		quitar.Rotulo = t.T("block.unblock", i18n.Vars{"name": nombre})
		filas = append(filas, filaConBoton(ui.NuevoParrafo(nombre, tema.TextoSM, ui.SemiNegrita, "text", v), quitar))
	}
	d = ui.NuevoDialogo(t.T("block.list", nil), container.New(ui.Columna{Hueco: tema.Espacio(2)}, filas...))
	d.RotuloCerrar = t.T("action.close", nil)
	sa.pregunta = d
	d.Mostrar(lienzo)
}

// AbrirDenunciasPorEnlace abre la cola desde el aviso del Hub (`?moderation=1`). Con
// la app recién abierta el enlace llega antes que la sesión: se apunta y se abre al
// arrancar.
func (sa *Sala) AbrirDenunciasPorEnlace() {
	if !sa.arrancada {
		sa.denunciasPendientes = true
		return
	}
	sa.AbrirDenuncias()
}

// AbrirDenuncias es `ReportsModal`: la cola de quien modera. Cada denuncia dice de
// quién es, por qué y dónde, enseña el mensaje si sigue existiendo, y ofrece
// descartarla, borrar el mensaje o expulsar del canal.
func (sa *Sala) AbrirDenuncias() {
	mod := sa.denuncias()
	lienzo := fyne.CurrentApp().Driver().CanvasForObject(sa.P.Objeto())
	if mod == nil || lienzo == nil {
		return
	}
	t := sa.P.T
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	parrafo := func(texto, token string) fyne.CanvasObject {
		return ui.NuevoParrafo(texto, tema.TextoSM, ui.Normal, token, v)
	}
	d := ui.NuevoDialogo(t.T("moderation.title", nil), ui.NuevoEsqueleto(0, tema.Espacio(16))).Con(ui.DialogoLG, nil)
	d.RotuloCerrar = t.T("action.close", nil)

	var pintar func(ds []api.RoomsReport)
	resolver := func(ds []api.RoomsReport, r api.RoomsReport, accion string) {
		pedir(sa, func(ctx context.Context) (struct{}, error) { return struct{}{}, mod.ResolveReport(ctx, r.ID, accion) },
			func(_ struct{}, err error) {
				if err != nil {
					sa.P.Avisar(ui.AvisoPeligro, t.T("block.failed", nil))
					return
				}
				sa.P.Avisar(ui.AvisoExito, t.T("moderation.resolved", nil))
				pintar(slices.DeleteFunc(slices.Clone(ds), func(x api.RoomsReport) bool { return x.ID == r.ID }))
			})
	}
	pintar = func(ds []api.RoomsReport) {
		if len(ds) == 0 {
			d.CambiarCuerpo(parrafo(t.T("moderation.empty", nil), "text-subtle"))
			return
		}
		filas := make([]fyne.CanvasObject, 0, len(ds)*6)
		for _, r := range ds {
			nombre := r.TargetName
			if nombre == "" {
				nombre = r.TargetSub
			}
			donde := t.T("moderation.inChannel", i18n.Vars{"channel": r.ChannelName})
			if r.ChannelKind == "direct" {
				donde = t.T("moderation.inDirect", nil)
			}
			filas = append(filas,
				ui.NuevoParrafo(nombre, tema.TextoSM, ui.SemiNegrita, "text", v),
				parrafo(sa.rotuloMotivo(r.Reason)+" · "+donde, "text-muted"))
			switch {
			case r.MessageBody != "":
				filas = append(filas, parrafo(r.MessageBody, "text"))
			case r.MessageID != "":
				filas = append(filas, parrafo(t.T("moderation.messageGone", nil), "text-subtle"))
			default:
				filas = append(filas, parrafo(t.T("moderation.personOnly", nil), "text-subtle"))
			}
			if r.Note != "" {
				filas = append(filas, parrafo(t.T("moderation.note", i18n.Vars{"note": r.Note}), "text-muted"))
			}
			botones := []fyne.CanvasObject{
				ui.NuevoBoton(t.T("moderation.dismiss", nil), func() { resolver(ds, r, api.DenunciaDescartada) }).
					Con(ui.Secundario, ui.TallaSM),
			}
			if r.MessageBody != "" {
				botones = append(botones, ui.NuevoBoton(t.T("moderation.deleteMessage", nil),
					func() { resolver(ds, r, api.DenunciaMensajeBorrado) }).Con(ui.Peligro, ui.TallaSM))
			}
			if r.ChannelKind != "direct" {
				botones = append(botones, ui.NuevoBoton(t.T("moderation.removeMember", nil),
					func() { resolver(ds, r, api.DenunciaMiembroExpulsado) }).Con(ui.Peligro, ui.TallaSM))
			}
			filas = append(filas, container.New(ui.FilaAuto{Hueco: tema.Espacio(2), Partir: true, SaltoFila: tema.Espacio(2)}, botones...))
		}
		d.CambiarCuerpo(container.New(ui.Columna{Hueco: tema.Espacio(2)}, filas...))
	}

	sa.pregunta = d
	d.Mostrar(lienzo)
	pedir(sa, mod.Reports, func(ds []api.RoomsReport, err error) {
		if err != nil {
			d.CambiarCuerpo(parrafo(t.T("moderation.loadFailed", nil), "danger-text"))
			return
		}
		pintar(ds)
	})
}
