// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"context"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// La bandeja de invitaciones: InvitationsModal.tsx, en un `Modal size="md"`. Carga al
// abrir; vacía, el EmptyState con su ilustración; con invitaciones, la ayuda y una
// tarjeta por invitación con quién la manda, cuándo es y las tres respuestas.
//
//	tarjeta   border-border bg-surface rounded-md border p-3
//	título    text-text font-medium · detalle `text-text-muted text-sm`
//	botones   mt-2.5 flex flex-wrap gap-2, `sm`: aceptar (primary), quizá (secondary),
//	          rechazar (danger)

// respuestas son ACTIONS: cada respuesta dice lo que HACE, con su aviso.
var respuestas = []struct {
	estado, clave, aviso string
	variante             ui.Variante
}{
	{api.InvitacionAceptada, "invitations.accept", "toast.invitationAccepted", ui.Primario},
	{api.InvitacionQuiza, "invitations.tentative", "toast.invitationTentative", ui.Secundario},
	{api.InvitacionRechazada, "invitations.decline", "toast.invitationDeclined", ui.Peligro},
}

// abrirInvitaciones abre la bandeja y la carga.
func (a *App) abrirInvitaciones() {
	lienzo := a.lienzo()
	if lienzo == nil || a.P.SinConexion {
		return
	}
	p := a.P
	cuerpo := container.New(ui.Columna{})
	d := ui.NuevoDialogo(p.t("invitations.title", nil), cuerpo).Con(ui.DialogoMD, nil)
	d.RotuloCerrar = p.t("common.close", nil)
	var lista []api.Invitation
	ocupada := ""
	var pintar func()
	responder := func(inv api.Invitation, estado, aviso string) {
		if ocupada != "" {
			return
		}
		ocupada = inv.ID
		pintar()
		a.escribir(func(ctx context.Context) error {
			_, err := a.Cal.RespondInvitation(ctx, inv.ID, estado)
			return err
		}, func(err error) {
			ocupada = ""
			if err != nil {
				a.avisar(ui.AvisoPeligro, "toast.invitationFailed")
				pintar()
				return
			}
			resto := lista[:0:0]
			for _, x := range lista {
				if x.ID != inv.ID {
					resto = append(resto, x)
				}
			}
			lista = resto
			a.P.Invitaciones = len(lista)
			a.avisar(ui.AvisoExito, aviso)
			pintar()
			a.pintar()
		})
	}
	pintar = func() {
		cuerpo.Objects = []fyne.CanvasObject{a.P.cuerpoInvitaciones(anchoUtilFormulario(lienzo.Size()), lista, ocupada, responder)}
		cuerpo.Refresh()
		if arriba := lienzo.Overlays().Top(); arriba != nil {
			arriba.Refresh()
		}
	}
	// Mientras carga: la rueda centrada con su `py-10`.
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	cuerpo.Objects = []fyne.CanvasObject{container.New(&centrado{Y: tema.Espacio(10)}, ui.NuevaRueda(ui.LadoRueda, "text-muted", v))}
	d.Mostrar(lienzo)
	if a.Cal == nil {
		return
	}
	a.lanzar(func() {
		ctx, parar := context.WithTimeout(context.Background(), plazo)
		defer parar()
		invs, err := a.Cal.Invitations(ctx, true)
		a.enUI(func() {
			if err != nil {
				a.avisar(ui.AvisoPeligro, "toast.invitationsLoadFailed")
			}
			lista = invs
			a.P.Invitaciones = len(invs)
			pintar()
			a.pintar()
		})
	})
}

// cuerpoInvitaciones es lo de dentro del modal ya cargado.
func (p *Pantalla) cuerpoInvitaciones(util float32, lista []api.Invitation, ocupada string, responder func(api.Invitation, string, string)) fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	if len(lista) == 0 {
		vacio := ui.NuevoVacio(p.t("invitations.emptyTitle", nil), p.t("invitations.emptyDescription", nil))
		vacio.Ilustracion = iconos.Pintar(iconos.VacioCalendario, v).Objeto()
		return vacio
	}
	tarjetas := []fyne.CanvasObject{}
	for _, inv := range lista {
		quien := p.t("invitations.generic", nil)
		if inv.Organizer != "" {
			quien = p.t("invitations.invitedBy", i18n.Vars{"name": inv.Organizer})
			// Una invitación de otra organización dice cuál (COM-15).
			if inv.Organization != "" {
				quien = p.t("invitations.invitedByFrom", i18n.Vars{"name": inv.Organizer, "org": api.OrgVisible(inv.Organization, inv.OrganizationName)})
			}
		}
		detalle := quien
		if !inv.Start.IsZero() {
			detalle = p.t("invitations.whoWhen", i18n.Vars{"who": quien, "when": i18n.FormatDateTime(p.locale(), inv.Start.In(p.zona()))})
		}
		titulo := inv.Summary
		if titulo == "" {
			titulo = p.t("invitations.untitled", nil)
		}
		var botones []fyne.CanvasObject
		for _, r := range respuestas {
			b := ui.NuevoBoton(p.t(r.clave, nil), func() { responder(inv, r.estado, r.aviso) }).Con(r.variante, ui.TallaSM)
			b.Cargando = ocupada == inv.ID
			b.Inerte = ocupada != ""
			botones = append(botones, b)
		}
		contenido := container.New(ui.Columna{},
			ui.NuevoParrafo(titulo, tema.TextoBase, ui.Medio, "text", v),
			container.New(&margenArriba{Arr: tema.Espacio(0.5)}, ui.NuevoParrafo(detalle, tema.TextoSM, ui.Normal, "text-muted", v)),
			container.New(&margenArriba{Arr: tema.Espacio(2.5)}, container.New(&envolver{Hueco: espacio2, Ancho: util - 2*(espacio3+tema.Borde)}, botones...)),
		)
		caja := ui.NuevaCaja(tema.Color("surface", v), tema.Color("border", v), tema.RadioMD)
		tarjetas = append(tarjetas, container.New(&conCajaArriba{caja: caja, M: espacio3}, append(caja.Objetos(), contenido)...))
	}
	ayuda := ui.NuevaAyudaEnLinea(p.t("invitations.help", nil)).ConTono(ui.AyudaInfo)
	return container.New(ui.Columna{Hueco: tema.Espacio(4)}, ayuda, container.New(ui.Columna{Hueco: espacio3}, tarjetas...))
}

// margenArriba es un `mt-*` sobre un único hijo.
type margenArriba struct{ Arr float32 }

func (m *margenArriba) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return objs[0].MinSize().AddWidthHeight(0, m.Arr)
}

func (m *margenArriba) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, m.Arr))
	objs[0].Resize(fyne.NewSize(t.Width, max(t.Height-m.Arr, 0)))
}

// conCajaArriba es una caja con `p-*` alrededor de un contenido que se apoya ARRIBA:
// los primeros objetos son la caja y el último, el contenido.
type conCajaArriba struct {
	caja *ui.Caja
	M    float32
}

func (c *conCajaArriba) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return objs[len(objs)-1].MinSize().AddWidthHeight(2*(c.M+tema.Borde), 2*(c.M+tema.Borde))
}

func (c *conCajaArriba) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	c.caja.Redimensionar(t)
	m := c.M + tema.Borde
	o := objs[len(objs)-1]
	o.Move(fyne.NewPos(m, m))
	o.Resize(fyne.NewSize(max(t.Width-2*m, 0), max(t.Height-2*m, 0)))
}

// centrado es `flex items-center justify-center py-*`: el hijo a su medida, centrado.
type centrado struct{ Y float32 }

func (c *centrado) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return objs[0].MinSize().AddWidthHeight(0, 2*c.Y)
}

func (c *centrado) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	m := objs[0].MinSize()
	objs[0].Move(fyne.NewPos((t.Width-m.Width)/2, (t.Height-m.Height)/2))
	objs[0].Resize(m)
}
