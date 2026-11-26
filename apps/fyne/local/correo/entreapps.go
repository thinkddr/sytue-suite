// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package correo

import (
	"context"
	"errors"
	"net/mail"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"fyne.io/fyne/v2/storage"

	pantalla "git.sytue.eu/sytue/one/apps/fyne/apps/mail"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	almacen "git.sytue.eu/sytue/one/apps/nucleo/local/correo"
	"git.sytue.eu/sytue/one/apps/nucleo/local/espacio"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema/escritorio"
)

// Mail con las demás apps de Sytue Office (§5.11): aquí no hay Hub que componga los
// enlaces ni web de Sytue a la que caer. «Crear evento» y la respuesta a una invitación
// abren Sytue Calendar de este equipo por su esquema (`sytueoffice-calendar:`), y Mail
// atiende `mailto:` y `sytueoffice-mail:/redactar?…`.

// Abridor es el `api.Abridor` de Mail en la edición local. La pantalla de Mail, que es la
// de la suite, abre otras apps con `sytue-<producto>:`; aquí eso se manda a
// `sytueoffice-<producto>:` (api.AbrirEnOffice). Las direcciones web van al navegador como
// siempre (api.AbrirFuera), y cualquier otra cosa no se abre.
type Abridor struct{ A api.Abridor }

// OpenURL cumple api.Abridor. No abre nada por sí mismo: pasa por las puertas de
// `nucleo/api/abrir.go`.
func (x Abridor) OpenURL(u *url.URL) error {
	switch {
	case u == nil:
		return api.ErrNoSeAbre
	case strings.HasPrefix(u.Scheme, "sytue-"):
		ruta := u.EscapedPath()
		if u.Opaque != "" {
			ruta = u.Opaque
		}
		if u.RawQuery != "" {
			ruta += "?" + u.RawQuery
		}
		if !strings.HasPrefix(ruta, "/") {
			ruta = "/" + ruta
		}
		return api.AbrirEnOffice(x.A, api.EsquemaOffice+strings.TrimPrefix(u.Scheme, "sytue-")+":"+ruta)
	case u.Scheme == "https" || u.Scheme == "http":
		return api.AbrirFuera(x.A, u.String())
	}
	return api.ErrNoSeAbre
}

// Intents hace en el equipo lo que el Hub hace en la suite: componer el enlace de un
// intent. Solo conoce «crear un evento desde un correo», que abre el compositor de
// Calendar con el asunto, el extracto y los participantes (M-14), con los mismos
// parámetros que el Hub (`services/hub/internal/intents`).
type Intents struct{}

// ResolveIntent cumple mail.Intents.
func (Intents) ResolveIntent(_ context.Context, tipo string, params map[string]string) (api.HubIntent, error) {
	if tipo != "create-event-from-mail" {
		return api.HubIntent{}, api.ErrNoSeAbre
	}
	q := url.Values{"action": {"create"}}
	for _, k := range [][2]string{{"message_id", "from_mail"}, {"subject", "title"}, {"body", "description"}, {"start", "start"}, {"attendees", "attendees"}} {
		if v := strings.TrimSpace(params[k[0]]); v != "" {
			q.Set(k[1], recortarRunas(v, 2000))
		}
	}
	return api.HubIntent{Type: tipo, Target: "calendar", URL: "/events/new?" + q.Encode()}, nil
}

func recortarRunas(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// AbrirEnCalendar le da a Sytue Calendar de este equipo un evento en `.ics` (la
// invitación con la respuesta puesta), como si se abriera el fichero con ella: lo
// importa en el calendario de la persona. El fichero se queda en la carpeta temporal de
// Sytue Office hasta que el sistema la limpie.
func AbrirEnCalendar(a api.Abridor, dir, nombre string, ics []byte) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	ruta := filepath.Join(dir, nombre)
	if err := os.WriteFile(ruta, ics, 0o600); err != nil {
		return err
	}
	return api.AbrirEnOffice(a, espacio.Esquema("calendar")+":/abrir?"+url.Values{"local": {storage.NewFileURI(ruta).String()}}.Encode())
}

// LeerEnlace desarma un enlace que pide redactar: `mailto:` (RFC 6068: `to`, `subject`,
// `body`; las direcciones del propio enlace y de `to`, juntas) y el de Sytue Office,
// `sytueoffice-mail:/redactar?para=…&asunto=…&cuerpo=…&adjunto=…`, que también entiende
// las claves de la suite (`?action=compose&to=…&subject=…&body=…`). Devuelve false si no
// es uno.
//
// Los adjuntos solo llegan por el de Sytue Office y nunca por su ruta: cada `adjunto` es
// el identificador de lo que otra app de este equipo dejó en salida
// (`almacen.ParaEnviar`, «para-enviar»), que se recoge aquí y se BORRA de allí (de un
// solo uso). Lo que no se puede recoger —un identificador inventado o ya usado, una
// ruta, un enlace simbólico— no se adjunta y se cuenta en SinAdjuntar. `mailto:` no
// adjunta nada, aunque lo pida.
func LeerEnlace(u *url.URL, salida almacen.ParaEnviar) (api.Redactar, bool) {
	if u == nil {
		return api.Redactar{}, false
	}
	q := u.Query()
	var r api.Redactar
	var para []string
	switch {
	case strings.EqualFold(u.Scheme, "mailto"):
		if a, err := url.PathUnescape(u.Opaque); err == nil && a != "" {
			para = append(para, a)
		}
		para = append(para, q["to"]...)
		r.Asunto, r.Cuerpo = q.Get("subject"), q.Get("body")
	case u.Scheme == espacio.Esquema("mail"):
		ruta := strings.Trim(cmpOr(u.Opaque, u.Path), "/")
		if ruta != "redactar" && ruta != "compose" && q.Get("action") != "compose" {
			return api.Redactar{}, false
		}
		para = append(para, q["para"]...)
		para = append(para, q["to"]...)
		r.Asunto, r.Cuerpo = cmpOr(q.Get("asunto"), q.Get("subject")), cmpOr(q.Get("cuerpo"), q.Get("body"))
		r.Adjuntos, r.SinAdjuntar = recogerAdjuntos(salida, q[almacen.ParamAdjunto])
	default:
		return api.Redactar{}, false
	}
	var limpias []string
	for _, p := range para {
		for _, d := range strings.Split(p, ",") {
			if a, err := mail.ParseAddress(strings.TrimSpace(d)); err == nil {
				limpias = append(limpias, a.Address)
			}
		}
	}
	r.Para = strings.Join(limpias, ", ")
	return r, true
}

// recogerAdjuntos recoge las entregas ids (como mucho almacen.MaxAdjuntosEnlace, y
// almacen.TopeParaEnviar entre todas) y cuenta las que no.
func recogerAdjuntos(salida almacen.ParaEnviar, ids []string) ([]api.AdjuntoRedactar, int) {
	var out []api.AdjuntoRedactar
	fuera, total := 0, 0
	for i, id := range ids {
		if i >= almacen.MaxAdjuntosEnlace {
			return out, fuera + len(ids) - i
		}
		a, err := salida.Recoger(id)
		if err != nil || total+len(a.Datos) > almacen.TopeParaEnviar {
			fuera++
			continue
		}
		total += len(a.Datos)
		out = append(out, a)
	}
	return out, fuera
}

// Invitaciones es `mail.Invitaciones` sobre el buzón local (M-17): contestar manda el
// REPLY por la bandeja de salida y abre el evento, con la respuesta puesta, en Sytue
// Calendar de este equipo. Dir es la carpeta donde se deja ese `.ics`.
type Invitaciones struct {
	B   *almacen.Buzon
	A   api.Abridor
	Dir string
}

// LeerInvitacion cumple mail.Invitaciones.
func (x Invitaciones) LeerInvitacion(ctx context.Context, id string) (pantalla.ResumenInvitacion, bool, error) {
	inv, err := x.B.Invitacion(ctx, id)
	if errors.Is(err, almacen.ErrSinInvitacion) {
		return pantalla.ResumenInvitacion{}, false, nil
	}
	if err != nil {
		return pantalla.ResumenInvitacion{}, false, err
	}
	if inv.EsRespuesta() {
		// La respuesta de un invitado a lo que organiza la persona no es una invitación:
		// Mail la deja para Calendar, que la aplica al evento (`almacen.RespuestasPorAplicar`).
		return pantalla.ResumenInvitacion{}, false, nil
	}
	return pantalla.ResumenInvitacion{
		Resumen: inv.Resumen, Lugar: inv.Lugar, Organizador: inv.Organizador,
		NombreOrganizador: inv.NombreOrganizador, Inicio: inv.Inicio, Fin: inv.Fin, TodoElDia: inv.TodoElDia,
		Contestable: inv.Contestable(),
	}, true, nil
}

// ContestarInvitacion cumple mail.Invitaciones. Que Calendar no esté instalada no hace
// fallar la respuesta: el REPLY ya ha salido.
func (x Invitaciones) ContestarInvitacion(ctx context.Context, id, respuesta string) (bool, error) {
	copia, enviada, err := x.B.ResponderInvitacion(ctx, id, respuesta)
	if err != nil {
		return false, err
	}
	if x.A != nil && x.Dir != "" {
		_ = AbrirEnCalendar(x.A, x.Dir, "invitacion-"+filepath.Base(id)+".ics", copia)
	}
	return enviada, nil
}

// AvisarLlegadas saca al sistema el aviso del correo nuevo (M-07): uno por mensaje con
// el remitente y el asunto, o uno solo con el total si llegan muchos de golpe. Pulsarlo
// abre la conversación (alAbrir, en su rutina). Donde el sistema no tiene avisos
// propios no sale nada: la lista ya lo enseña.
func (g *Gestor) AvisarLlegadas(ls []almacen.Llegada, alAbrir func(hilo string)) {
	m := g.L.A.Metadata()
	esquema, _ := m.Custom["esquema"]
	avisar := func(titulo, cuerpo, hilo string) {
		_ = escritorio.Avisar(escritorio.Aviso{
			ID: m.ID, Nombre: m.Name, Esquema: esquema, Titulo: titulo, Cuerpo: cuerpo,
			Destino: escritorio.Destino{Origen: "mail"}, AlPulsar: func() {
				if alAbrir != nil && hilo != "" {
					alAbrir(hilo)
				}
			},
		})
	}
	if len(ls) > 3 {
		avisar(strings.ReplaceAll(g.t("llegadas"), "{n}", strconv.Itoa(len(ls))), ls[0].Cuenta.Titulo(), "")
		return
	}
	for _, x := range ls {
		de := cmpOr(x.Mensaje.FromName, x.Mensaje.From)
		avisar(de, cmpOr(x.Mensaje.Subject, g.t("sinAsunto")), x.Mensaje.ThreadID)
	}
}
