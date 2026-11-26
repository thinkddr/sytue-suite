// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package correo

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"net/mail"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-ical"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/eml"
	"git.sytue.eu/sytue/one/apps/nucleo/local/biblioteca"
)

// Las RESPUESTAS DE LOS INVITADOS que llegan por correo (iMIP, RFC 6047) a lo que organiza
// la persona. Con un CalDAV que no planifica, la invitación sale por correo desde Calendar
// (`local/agenda`, el Cartero) y cada invitado contesta con un REPLY de iTIP (RFC 5546
// §3.2.3) que llega aquí, a Mail. Nadie más lo aplica al evento: ni el servidor (no
// planifica) ni Calendar, que no lee el correo.
//
// Mail no abre el almacén de Calendar —no admite dos escritores y Calendar puede estar
// abierta—: deja el REPLY tal cual llegó en una COLA COMPARTIDA, un `.ics` por respuesta en
// `<Datos>/Correo/respuestas-imip/` (escrito entero con un temporal y un renombrado), y
// Calendar lo recoge al abrirse y, abierta, en cada vuelta de su sincronización
// ([RespuestasPorAplicar]): pone el PARTSTAT del invitado en el evento y lo sube con él.
// Es la cola de «Enviados» (enviados.go) al revés.
//
// Solo entra lo que llega NUEVO a la entrada (no lo de la primera bajada de una cuenta),
// es un REPLY con su UID, el organizador es la dirección de una cuenta de Mail de este
// equipo, y quien contesta es quien manda el correo (From, Sender o Reply-To, o el
// SENT-BY del asistente): un correo cualquiera no cambia la respuesta de otra persona.
// El mensaje queda marcado como procesado en el índice (no se vuelve a mirar, tampoco si
// otro cliente lo mueve y vuelve) y el lector no lo enseña como invitación.

// carpetaColaRespuestas es la cola, dentro de la carpeta del correo.
const carpetaColaRespuestas = "respuestas-imip"

// extensionICS es la de los ficheros de la cola. Partida para que el guardián de los
// catálogos no la tome por una clave de texto.
const extensionICS = "." + "ics"

// CaducidadRespuestas es cuánto espera una respuesta en la cola: si en un mes nadie la ha
// recogido (Calendar no está instalada), se tira.
const CaducidadRespuestas = 30 * 24 * time.Hour

// Estados de un mensaje con un REPLY en el índice.
const (
	respuestaEncolada = "encolada" // dejada en la cola para Calendar
	respuestaAjena    = "ajena"    // no es de algo que organice la persona, o no la manda el invitado
)

// metodoRespuesta es el METHOD de iTIP de una respuesta.
const metodoRespuesta = "REPLY"

// EsRespuesta dice si el calendario del mensaje es la respuesta de un invitado (un REPLY),
// no una invitación: el lector no la enseña como tal.
func (i Invitacion) EsRespuesta() bool { return i.Metodo == metodoRespuesta }

// respuestaITIP es lo que se mira de un REPLY antes de dejarlo en la cola.
type respuestaITIP struct {
	uid, organizador string
	// asistentes son los que contestan (en minúsculas), y enviadoPor, su SENT-BY.
	asistentes, enviadoPor []string
}

// leerRespuestaITIP lee el primer calendario METHOD:REPLY de un mensaje. Un iMIP lleva el
// mismo calendario dos veces (la parte `text/calendar` y el `invite.ics`): vale uno.
func leerRespuestaITIP(m eml.Mensaje) ([]byte, respuestaITIP, bool) {
	for _, a := range m.Adjuntos {
		cal, ok := calendarioRespuesta(a)
		if !ok {
			continue
		}
		r := respuestaDe(cal)
		if r.uid == "" || r.organizador == "" || len(r.asistentes) == 0 {
			continue
		}
		return a.Datos, r, true
	}
	return nil, respuestaITIP{}, false
}

// calendarioRespuesta lee un adjunto como calendario, si es uno METHOD:REPLY.
func calendarioRespuesta(a eml.Adjunto) (*ical.Calendar, bool) {
	if !esCalendario(a) || len(a.Datos) == 0 {
		return nil, false
	}
	cal, err := ical.NewDecoder(bytes.NewReader(a.Datos)).Decode()
	if err != nil {
		return nil, false
	}
	p := cal.Props.Get(ical.PropMethod)
	return cal, p != nil && strings.EqualFold(strings.TrimSpace(p.Value), metodoRespuesta)
}

// respuestaDe junta lo que dicen los VEVENT de un REPLY: el primer UID y ORGANIZER, y
// todos los que contestan.
func respuestaDe(cal *ical.Calendar) respuestaITIP {
	var r respuestaITIP
	for _, ev := range cal.Children {
		if ev.Name != ical.CompEvent {
			continue
		}
		if uid, _ := ev.Props.Text(ical.PropUID); r.uid == "" {
			r.uid = strings.TrimSpace(uid)
		}
		if p := ev.Props.Get(ical.PropOrganizer); p != nil && r.organizador == "" {
			r.organizador = direccionCal(p.Value)
		}
		for _, p := range ev.Props.Values(ical.PropAttendee) {
			if d := direccionCal(p.Value); d != "" && !slices.Contains(r.asistentes, d) {
				r.asistentes = append(r.asistentes, d)
			}
			if s := direccionCal(p.Params.Get("SENT-BY")); s != "" {
				r.enviadoPor = append(r.enviadoPor, s)
			}
		}
	}
	return r
}

// procesarRespuestas mira lo que acaba de llegar a la entrada y deja en la cola de
// Calendar los REPLY a lo que organiza la persona. Lo que aún no tiene el cuerpo en el
// equipo (uno grande) no se mira: una respuesta es pequeña y se baja en la misma vuelta.
func (b *Buzon) procesarRespuestas(ctx context.Context, ids []int64) {
	var propias []string
	for _, c := range b.reg.lista() {
		if d := strings.ToLower(strings.TrimSpace(c.Direccion)); d != "" {
			propias = append(propias, d)
		}
	}
	for _, n := range ids {
		s, err := b.x.sobre(ctx, n)
		if err != nil || !s.eml || s.rol != "inbox" || b.respuestaVista(ctx, s) {
			continue
		}
		crudo, err := os.ReadFile(b.rutaEML(s.cuenta, s.id))
		if err != nil {
			continue
		}
		m, err := eml.LeerConLimite(ctx, bytes.NewReader(crudo), int64(len(crudo))+1)
		if err != nil && len(m.Adjuntos) == 0 {
			continue
		}
		ics, r, ok := leerRespuestaITIP(m)
		if !ok {
			continue
		}
		estado := respuestaAjena
		switch {
		case !slices.Contains(propias, r.organizador):
			slog.Info("correo: una respuesta a una invitación que no organiza ninguna cuenta de Mail", "uid", r.uid)
		case !contestaQuienEscribe(crudo, m, r):
			slog.Warn("correo: una respuesta a una invitación que no manda el invitado; no se aplica", "uid", r.uid)
		default:
			if err := dejarRespuesta(b.raiz, ics); err != nil {
				// Sin la cola no se marca: la próxima vez que se mire, otra oportunidad.
				slog.Warn("correo: no se pudo dejar la respuesta para Calendar", "error", err)
				continue
			}
			estado = respuestaEncolada
		}
		if _, err := b.x.db.ExecContext(ctx, "INSERT OR REPLACE INTO respuestas_imip (mensaje, message_id, estado, cuando) VALUES (?, ?, ?, ?)",
			s.id, s.messageID, estado, time.Now().Unix()); err != nil {
			slog.Warn("correo: no se pudo marcar la respuesta como procesada", "error", err)
		}
	}
}

// respuestaVista dice si el mensaje (o uno con su Message-ID: el mismo que otro cliente
// movió y volvió) ya se procesó.
func (b *Buzon) respuestaVista(ctx context.Context, s sobre) bool {
	var ya bool
	err := b.x.db.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM respuestas_imip WHERE mensaje = ? OR (message_id != '' AND message_id = ?))",
		s.id, s.messageID).Scan(&ya)
	return err == nil && ya
}

// RespuestaProcesada dice si el mensaje id es una respuesta a una invitación que Mail ya
// procesó (dejada para Calendar, o descartada por no ser de algo que organice la persona).
func (b *Buzon) RespuestaProcesada(ctx context.Context, id string) (bool, error) {
	n, err := leerID(id)
	if err != nil {
		return false, err
	}
	var ya bool
	err = b.x.db.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM respuestas_imip WHERE mensaje = ?)", n).Scan(&ya)
	return ya, err
}

// contestaQuienEscribe dice si el correo lo manda quien contesta: su dirección es el
// From, el Sender o un Reply-To del mensaje, o quien lo manda es el SENT-BY del asistente.
func contestaQuienEscribe(crudo []byte, m eml.Mensaje, r respuestaITIP) bool {
	remitentes := []string{strings.ToLower(strings.TrimSpace(m.De))}
	for _, d := range m.ResponderA {
		remitentes = append(remitentes, strings.ToLower(strings.TrimSpace(d)))
	}
	if msg, err := mail.ReadMessage(bytes.NewReader(crudo)); err == nil {
		if a, err := mail.ParseAddress(msg.Header.Get("Sender")); err == nil {
			remitentes = append(remitentes, strings.ToLower(a.Address))
		}
	}
	for _, d := range append(slices.Clone(r.asistentes), r.enviadoPor...) {
		if d != "" && slices.Contains(remitentes, d) {
			return true
		}
	}
	return false
}

func rutaColaRespuestas(raiz string) string { return filepath.Join(raiz, carpetaColaRespuestas) }

// dejarRespuesta deja un REPLY en la cola (con un nombre que ordena por llegada) y tira
// lo que lleva más de CaducidadRespuestas esperando.
func dejarRespuesta(raiz string, ics []byte) error {
	dir := rutaColaRespuestas(raiz)
	nombre := strconv.FormatInt(time.Now().UnixNano(), 10) + "-" + uuid.NewString()[:8] + extensionICS
	if err := biblioteca.EscribirAtomico(filepath.Join(dir, nombre), ics); err != nil {
		return err
	}
	if es, err := os.ReadDir(dir); err == nil {
		for _, e := range es {
			if fi, err := e.Info(); err == nil && !e.IsDir() && time.Since(fi.ModTime()) > CaducidadRespuestas {
				_ = os.Remove(filepath.Join(dir, e.Name()))
			}
		}
	}
	return nil
}

// RespuestaPorAplicar es un REPLY que Mail dejó para Calendar.
type RespuestaPorAplicar struct {
	// Ruta es su fichero en la cola.
	Ruta string
	// ICS es el VCALENDAR METHOD:REPLY tal como llegó.
	ICS []byte
	// Llegada es cuándo lo dejó Mail.
	Llegada time.Time
}

// Quitar la saca de la cola: aplicada, o que no aplica a nada.
func (r RespuestaPorAplicar) Quitar() error {
	if err := os.Remove(r.Ruta); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// RespuestasPorAplicar son las respuestas de los invitados que esperan en la cola de raiz
// (`<Datos>/Correo`, la de Mail), de la más vieja a la más nueva. Es lo que usa Calendar,
// sin abrir el buzón de Mail. Sin cola, ninguna.
func RespuestasPorAplicar(raiz string) ([]RespuestaPorAplicar, error) {
	dir := rutaColaRespuestas(raiz)
	es, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []RespuestaPorAplicar
	for _, e := range es {
		nombre := e.Name()
		if e.IsDir() || strings.HasPrefix(nombre, ".") || !strings.HasSuffix(nombre, extensionICS) {
			continue // un temporal a medio escribir
		}
		ruta := filepath.Join(dir, nombre)
		datos, err := os.ReadFile(ruta)
		if err != nil {
			continue
		}
		r := RespuestaPorAplicar{Ruta: ruta, ICS: datos}
		if fi, err := e.Info(); err == nil {
			r.Llegada = fi.ModTime()
		}
		out = append(out, r)
	}
	slices.SortStableFunc(out, func(a, b RespuestaPorAplicar) int {
		if c := a.Llegada.Compare(b.Llegada); c != 0 {
			return c
		}
		return strings.Compare(a.Ruta, b.Ruta)
	})
	return out, nil
}

// RespuestasEnCola son los nombres de los ficheros de la cola de raiz, sin leerlos: lo que
// mira Calendar abierta para no esperar a su próxima vuelta cuando llega una nueva.
func RespuestasEnCola(raiz string) []string {
	es, err := os.ReadDir(rutaColaRespuestas(raiz))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range es {
		if n := e.Name(); !e.IsDir() && !strings.HasPrefix(n, ".") && strings.HasSuffix(n, extensionICS) {
			out = append(out, n)
		}
	}
	return out
}
