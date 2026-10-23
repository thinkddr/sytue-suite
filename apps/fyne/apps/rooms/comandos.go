// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"strings"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// En la web: apps/frontend/apps/rooms/src/components/ChatApp.tsx (`runCommand` y
// `runRemoteCommand`) y el freno de @channel de Composer.tsx (`sendBody`).

// UmbralAvisoMasivo es `UMBRAL_AVISO_MASIVO`: desde cuántos miembros se pide confirmar
// @channel, @here y @all. Por debajo estorba más de lo que protege.
const UmbralAvisoMasivo = 8

// reATodos es `CHANNEL_MENTION`, IGUAL que `channelMentionRE` del servicio: si se
// separan, se confirma un aviso que no llega a nadie o se avisa a todos sin preguntar.
var reATodos = regexp.MustCompile(`(^|[^\w@])@(channel|here|all)($|\W)`)

// MencionaATodos es `mentionsWholeChannel`.
func MencionaATodos(texto string) bool { return reATodos.MatchString(texto) }

// confirmarATodos pregunta antes de despertar a un canal entero; si no, el texto se
// queda en la caja.
func (sa *Sala) confirmarATodos(seguir func()) {
	t := sa.P.T
	sa.confirmar(t.T("mention.confirmAll", i18n.Vars{"count": len(sa.roster), "channel": sa.P.Canal.Cabecera.Canal.Nombre}),
		t.T("mention.confirmAllAction", nil), false, seguir)
}

// confirmar es el `confirmar` de la web: el mensaje, «Cancelar» y la acción, que es la
// que sigue; `peligro` es su `tone: 'danger'`.
func (sa *Sala) confirmar(mensaje, accion string, peligro bool, seguir func()) {
	t := sa.P.T
	lienzo := fyne.CurrentApp().Driver().CanvasForObject(sa.P.Objeto())
	if lienzo == nil {
		return
	}
	var d *ui.Dialogo
	cuerpo := ui.NuevoParrafo(mensaje, tema.TextoSM, ui.Normal, "text", tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	d = ui.Confirmacion(accion, t.T("action.cancel", nil), accion, peligro,
		cuerpo, func() { d.Cerrar() }, func() {
			d.Cerrar()
			seguir()
		})
	d.RotuloCerrar = t.T("action.close", nil)
	sa.pregunta = d
	d.Mostrar(lienzo)
}

// cargarComandos trae los comandos de servidor para el autocompletado. Si falla, se
// queda con los integrados, como la web.
func (sa *Sala) cargarComandos() {
	pedir(sa, sa.S.Commands, func(cs []api.RoomsCommand, err error) {
		if err != nil {
			return
		}
		sa.P.Canal.Compositor.Remotos = cs
		if sa.hilo != nil {
			sa.hilo.Compositor.Remotos = cs
		}
	})
}

// ComandoRemoto es `runRemoteCommand`: lo ejecuta el servicio, que llama al tercero. Lo
// que se publicó en el canal ya llega por el tiempo real; aquí solo se enseña lo que
// solo ve quien lo ejecutó, y el fallo.
func (sa *Sala) ComandoRemoto(nombre, arg string) {
	canal, t := sa.abierto, sa.P.T
	if canal == "" {
		return
	}
	linea := "/" + nombre
	if arg != "" {
		linea += " " + arg
	}
	pedir(sa, func(ctx context.Context) (api.RoomsCommandResult, error) {
		return sa.S.ExecuteCommand(ctx, canal, linea)
	},
		func(r api.RoomsCommandResult, err error) {
			switch {
			case err != nil:
				sa.P.Avisar(ui.AvisoPeligro, t.T("command.failed", i18n.Vars{"name": nombre}))
			case r.ResponseType != "in_channel" && strings.TrimSpace(r.Text) != "":
				sa.P.Avisos.Notificar(ui.NuevoAviso(ui.AvisoInfo, "/"+nombre, r.Text))
			}
		})
}

// Comando es `runCommand`: los integrados CON EFECTO. El nombre llega canónico
// (/dm → msg, /header → topic).
func (sa *Sala) Comando(nombre, arg string) {
	l := sa.P.Lateral
	switch nombre {
	case "away":
		sa.CambiarPresencia(api.Ausente, l.EstadoPersonal, false, time.Time{})
	case "online":
		sa.CambiarPresencia(api.EnLinea, l.EstadoPersonal, false, time.Time{})
	case "dnd":
		sa.CambiarPresencia(api.NoMolestar, l.EstadoPersonal, false, time.Time{})
	case "status":
		sa.CambiarPresencia(presenciaAPI(l.Presencia), arg, false, time.Time{})
	case "topic":
		sa.CambiarTema(arg, false)
	case "purpose":
		sa.CambiarTema(arg, true)
	case "search":
		sa.cerrarDerecho()
		sa.AbrirPanel(PanelBusqueda)
		if sa.lista != nil {
			sa.lista.PonerConsulta(arg)
		}
	case "leave":
		sa.Salir()
	case "msg", "invite", "remove":
		sa.conPersona(nombre, strings.TrimSpace(strings.TrimPrefix(arg, "@")))
	}
}

// CambiarTema es /topic (o /purpose con `proposito`): se ve ya y vuelve si el servicio
// dice que no, con su aviso.
func (sa *Sala) CambiarTema(texto string, proposito bool) {
	fallo := "command.topicFailed"
	if proposito {
		fallo = "command.purposeFailed"
	}
	sa.cambiarTema(texto, proposito, fallo)
}

// cambiarTema es el cambio optimista del tema o del propósito; `fallo` es la clave del
// aviso si el servicio dice que no, o "" para volver en silencio (`commitTopic`).
func (sa *Sala) cambiarTema(texto string, proposito bool, fallo string) {
	id := sa.abierto
	i := slices.IndexFunc(sa.canales, func(c api.RoomsChannel) bool { return c.ID == id })
	if i < 0 {
		return
	}
	antes := sa.canales[i]
	cambio := api.RoomsChannelPatch{Topic: &texto}
	if proposito {
		cambio = api.RoomsChannelPatch{Purpose: &texto}
		sa.canales[i].Purpose = texto
	} else {
		sa.canales[i].Topic = texto
	}
	sa.repintarCanalAbierto()
	pedir(sa, func(ctx context.Context) (api.RoomsChannel, error) { return sa.S.UpdateChannel(ctx, id, cambio) },
		func(_ api.RoomsChannel, err error) {
			if err == nil {
				return
			}
			if j := slices.IndexFunc(sa.canales, func(c api.RoomsChannel) bool { return c.ID == id }); j >= 0 {
				sa.canales[j].Topic, sa.canales[j].Purpose = antes.Topic, antes.Purpose
			}
			sa.repintarCanalAbierto()
			if fallo != "" {
				sa.P.Avisar(ui.AvisoPeligro, sa.P.T.T(fallo, nil))
			}
		})
}

// repintarCanalAbierto lleva al carril y a la cabecera lo que cambió del canal abierto.
func (sa *Sala) repintarCanalAbierto() {
	sa.pintarCanales()
	if i := slices.IndexFunc(sa.canales, func(c api.RoomsChannel) bool { return c.ID == sa.abierto }); i >= 0 {
		sa.P.Canal.Cabecera.Canal = sa.canalDe(sa.canales[i])
		sa.P.Canal.Cabecera.Refresh()
	}
	sa.P.Refrescar()
}

// Salir es /leave: sale del canal abierto (ver SalirDe).
func (sa *Sala) Salir() { sa.SalirDe(sa.abierto) }

// SalirDe sale de un canal: lo comparten /leave y «Salir del canal» de los menús. Sale
// del carril ya —si era el abierto, se abre el primero que quede— y vuelve si el
// servicio dice que no, con su aviso: un 409 es que eres el último admin de un canal
// privado, que no es lo mismo que un fallo.
func (sa *Sala) SalirDe(id string) {
	if id == "" {
		return
	}
	deshacer := sa.quitarCanal(id)
	pedir(sa, func(ctx context.Context) (struct{}, error) { return struct{}{}, sa.S.LeaveChannel(ctx, id) },
		func(_ struct{}, err error) {
			if err == nil {
				sa.olvidarCanal(id)
				return
			}
			deshacer()
			clave := "command.leaveFailed"
			if e := (*api.Error)(nil); errors.As(err, &e) && e.Status == 409 {
				clave = "channel.lastAdmin"
			}
			sa.P.Avisar(ui.AvisoPeligro, sa.P.T.T(clave, nil))
		})
}

// quitarCanal retira una fila y devuelve un deshacer por ID, sin sustituir listas
// que hayan cambiado entre tanto. Una baja posterior invalida ese deshacer.
func (sa *Sala) quitarCanal(id string) func() {
	if sa.retiradas == nil {
		sa.retiradas = map[string]uint64{}
	}
	sa.retiradas[id]++
	revision := sa.retiradas[id]
	i := slices.IndexFunc(sa.canales, func(c api.RoomsChannel) bool { return c.ID == id })
	if i < 0 {
		return func() {}
	}
	antes, eraAbierto := sa.canales[i], sa.abierto == id
	sa.canales = slices.Delete(sa.canales, i, i+1)
	sa.pintarCanales()
	if eraAbierto {
		sa.vista++ // una lectura en curso del canal retirado ya no puede repintarlo
		sa.mensajes, sa.roster = nil, nil
		sa.P.Canal.Historial.Mensajes = nil
		sa.cancelarAdjuntos()
		sa.restaurando = true
		sa.P.Canal.Compositor.Vaciar()
		sa.restaurando = false
		sa.cerrarDerecho()
		sa.abierto = ""
		if len(sa.P.Lateral.Canales) > 0 {
			sa.Elegir(sa.P.Lateral.Canales[0])
		} else {
			sa.P.SinCanal()
		}
	}
	vista := sa.vista
	sa.P.Refrescar()
	return func() {
		if sa.retiradas[id] != revision {
			return
		}
		if !slices.ContainsFunc(sa.canales, func(c api.RoomsChannel) bool { return c.ID == id }) {
			sa.canales = slices.Insert(sa.canales, min(i, len(sa.canales)), antes)
		}
		sa.pintarCanales()
		if eraAbierto && sa.vista == vista {
			sa.Elegir(sa.canalDe(antes))
		}
		sa.P.Refrescar()
	}
}

// olvidarCanal elimina las copias descifradas y borradores de una conversación
// confirmada como retirada. No revoca claves ya recibidas ni purga Matrix.
func (sa *Sala) olvidarCanal(id string) {
	if cache, ok := sa.S.(interface{ ForgetChannel(string) }); ok {
		cache.ForgetChannel(id)
	}
	for key, stop := range sa.subidas {
		if key.canal == id {
			stop()
			delete(sa.subidas, key)
		}
	}
	for key := range sa.borradores {
		if key.canal == id {
			delete(sa.borradores, key)
		}
	}
	if sa.canalHilo == id && sa.hilo != nil {
		sa.hilo.Raiz, sa.hilo.Respuestas = Mensaje{}, nil
		sa.restaurando = true
		sa.hilo.Compositor.Vaciar()
		sa.restaurando = false
		sa.hilo = nil
		sa.canalHilo = ""
		sa.CerrarHilo()
	}
	// Las listas globales pueden contener mensajes de este canal. Una respuesta
	// tardía de una acción anterior no debe cerrar el hilo de otro canal.
	if sa.panel == PanelGuardados || sa.panel == PanelHilos || sa.panel == PanelBusqueda || sa.panel == PanelBorradores {
		sa.cerrarDerecho()
	}
	sa.seguidos = slices.DeleteFunc(sa.seguidos, func(h api.RoomsThreadSummary) bool { return h.Root.ChannelID == id })
	sa.pintarSeguidos()
	delete(sa.pend, id)
	delete(sa.escribiendo, id)
}

// errNadie es que no hay nadie en el equipo con ese nombre.
var errNadie = errors.New("rooms: no hay nadie con ese nombre")

// conPersona es /msg, /invite y /remove: buscan a la persona POR NOMBRE en el equipo
// (`membersByName`) y hacen lo suyo; uno que no está, lo dice.
func (sa *Sala) conPersona(nombre, quien string) {
	t, equipo := sa.P.T, sa.equipo
	if quien == "" {
		quien = "…"
	}
	type hecho struct {
		m     api.RoomsMember
		canal api.RoomsChannel
	}
	pedir(sa, func(ctx context.Context) (hecho, error) {
		ms, err := sa.S.TeamMembers(ctx, equipo)
		if err != nil {
			return hecho{}, err
		}
		i := slices.IndexFunc(ms, func(m api.RoomsMember) bool { return m.Name == quien })
		if i < 0 {
			return hecho{}, errNadie
		}
		h := hecho{m: ms[i]}
		switch nombre {
		case "msg":
			h.canal, err = sa.S.OpenDirect(ctx, equipo, []string{h.m.Subject})
		case "invite":
			err = sa.S.AddTeamMember(ctx, equipo, h.m.Subject, h.m.Name)
		case "remove":
			err = sa.S.RemoveTeamMember(ctx, equipo, h.m.Subject)
		}
		return h, err
	}, func(h hecho, err error) {
		fallos := map[string][2]string{
			"msg": {"command.notFound", "command.dmFailed"}, "invite": {"command.noSuchMember", "command.inviteFailed"},
			"remove": {"command.noSuchMember", "command.removeFailed"},
		}[nombre]
		switch {
		case errors.Is(err, errNadie):
			sa.P.Avisar(ui.AvisoPeligro, t.T(fallos[0], i18n.Vars{"name": quien}))
		case err != nil:
			sa.P.Avisar(ui.AvisoPeligro, t.T(fallos[1], nil))
		case nombre == "msg":
			h.canal.Name, h.canal.Peers = h.m.Name, []string{h.m.Name}
			sa.entrarEn(h.canal)
		case nombre == "invite":
			sa.P.Avisar(ui.AvisoExito, t.T("command.invited", i18n.Vars{"name": h.m.Name}))
		default:
			sa.P.Avisar(ui.AvisoExito, t.T("command.removed", i18n.Vars{"name": h.m.Name}))
		}
	})
}
