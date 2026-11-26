// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package agenda

import (
	"bytes"
	"cmp"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"maps"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-ical"

	calical "git.sytue.eu/sytue/one/apps/nucleo/calendar/ical"
)

// Invitaciones por correo (iMIP, RFC 6047; A-11). Un CalDAV sin planificación del
// servidor (sin `calendar-auto-schedule`) guarda el evento y no avisa a nadie: quien
// organiza tiene que mandar él la invitación, sus cambios y su cancelación (iTIP,
// RFC 5546) por correo. Aquí sale por la cuenta de Sytue Mail de este equipo cuya
// dirección es la del organizador, por su SMTP y sin abrir el buzón de Mail
// (`correo.Remitente`, el mismo camino que la factura del ERP); si no hay ninguna, se
// avisa a la persona ([SinRemitente]). Cuando la persona no organiza pero contesta (su
// PARTSTAT cambió aquí), el REPLY va al organizador por el mismo camino. Lo que no sale
// (sin cuenta de Mail, o el SMTP que falla) espera en la cola ([ColaIMIP]) y se
// reintenta solo.
//
// El sincronizador decide CUÁNDO: solo tras subir de verdad un evento con invitados a
// una colección que no planifica, y solo si cambió lo que ven los invitados (la
// [firma]: fechas, repetición, título, lugar, descripción, estado y la lista de
// invitados con sus respuestas; un recordatorio no cuenta). El [Cartero] decide QUÉ.

// Buzon es una cuenta de correo con la que se pueden mandar las invitaciones.
type Buzon struct {
	ID, Direccion, Nombre string
}

// Correo son las cuentas de Sytue Mail de este equipo y su envío (en Sytue Office,
// `correo.Remitente`).
type Correo interface {
	Buzones() ([]Buzon, error)
	Enviar(ctx context.Context, buzon string, para []string, crudo []byte) error
}

// Envio es lo que el sincronizador pasa a Invitar tras subir o borrar un evento.
type Envio struct {
	// Datos es el VCALENDAR tal como quedó en el servidor o, si se borró, como estaba.
	Datos string
	// Antes son los invitados (correo → PARTSTAT) de la última versión que se vio; nil
	// si no se vio con invitados.
	Antes map[string]string
	// Nuevo: se acaba de crear en el servidor. Borrado: se acaba de borrar.
	Nuevo, Borrado bool
	// Contenido: cambió lo que ven los invitados (fechas, título…), no solo respuestas.
	Contenido bool
}

// SinRemitente: hay algo que mandar por correo y ninguna cuenta de Mail de este equipo
// tiene la dirección con que hay que mandarlo.
type SinRemitente struct {
	Direccion, Resumen string
}

func (e SinRemitente) Error() string {
	return fmt.Sprintf("agenda: no hay ninguna cuenta de Mail con la dirección %s para mandar las invitaciones de %q", e.Direccion, e.Resumen)
}

// FalloEnvio: el correo de las invitaciones no salió.
type FalloEnvio struct {
	Direccion, Resumen string
	Err                error
}

func (e FalloEnvio) Error() string {
	return fmt.Sprintf("agenda: no salieron por %s las invitaciones de %q: %v", e.Direccion, e.Resumen, e.Err)
}

func (e FalloEnvio) Unwrap() error { return e.Err }

// TextosIMIP son los textos del correo, en el idioma de la app: los asuntos llevan %s
// para el título del evento.
type TextosIMIP struct {
	Invitacion, Actualizada, Cancelada string
	Cuando, Donde, Organiza            string
}

func (t TextosIMIP) con(def TextosIMIP) TextosIMIP {
	return TextosIMIP{
		Invitacion: cmp.Or(t.Invitacion, def.Invitacion), Actualizada: cmp.Or(t.Actualizada, def.Actualizada),
		Cancelada: cmp.Or(t.Cancelada, def.Cancelada), Cuando: cmp.Or(t.Cuando, def.Cuando),
		Donde: cmp.Or(t.Donde, def.Donde), Organiza: cmp.Or(t.Organiza, def.Organiza),
	}
}

// textosIMIPIngles son los de respaldo (los que lee cualquier calendario de fuera).
var textosIMIPIngles = TextosIMIP{
	Invitacion: "Invitation: %s", Actualizada: "Updated invitation: %s", Cancelada: "Canceled: %s",
	Cuando: "When", Donde: "Where", Organiza: "Organizer",
}

// Cartero manda las invitaciones de una cuenta de calendario.
type Cartero struct {
	Correo Correo
	Textos TextosIMIP
	// Propia es la dirección de la cuenta de calendario (el usuario de iCloud, de
	// Fastmail…): si organiza ella o contesta ella y no hay cuenta de Mail con esa
	// dirección, se avisa.
	Propia string
	// Direccion, si no es nil, es la dirección de la persona que dio el servidor (el
	// `calendar-user-address-set`), que vale como Propia.
	Direccion func() string
	// Cola, si no es nil, guarda lo que no sale para reintentarlo ([ColaIMIP]).
	Cola *ColaIMIP
	// Ahora es el reloj (las pruebas lo cambian).
	Ahora func() time.Time
}

func (c *Cartero) ahora() time.Time {
	if c.Ahora != nil {
		return c.Ahora()
	}
	return time.Now()
}

// Invitar es el gancho del sincronizador (Sincronizador.Invitar).
func (c *Cartero) Invitar(ctx context.Context, e Envio) error {
	r, err := leerReunion(e.Datos)
	if err != nil || r.organizador == "" {
		return nil //nolint:nilerr // sin organizador (o sin poder leerlo) no es una reunión: nadie espera nada.
	}
	r.visto = !e.Nuevo && e.Antes != nil
	buzones, err := c.Correo.Buzones()
	if err != nil {
		return err
	}
	if b, ok := buzonDe(buzones, r.organizador); ok {
		return c.organiza(ctx, b, r, e)
	}
	// No organiza ninguna cuenta de Mail: ¿contesta una?
	if contesta, err := c.contestar(ctx, buzones, r, e); contesta {
		return err
	}
	if c.propia(r.organizador) && (e.Nuevo || e.Contenido || e.Borrado || r.cancelado || cambiaronInvitados(e.Antes, r)) {
		if c.Cola == nil {
			return SinRemitente{Direccion: r.organizador, Resumen: r.resumen}
		}
		return c.organiza(ctx, Buzon{Direccion: r.organizador, Nombre: r.nombreOrganizador}, r, e)
	}
	return nil
}

// contestar manda el REPLY de la primera invitada cuya respuesta cambió y que es una
// cuenta de Mail (o la persona, por la cola). contesta=false si no hay ninguna.
func (c *Cartero) contestar(ctx context.Context, buzones []Buzon, r reunion, e Envio) (contesta bool, err error) {
	if e.Antes == nil || e.Borrado {
		return false, nil
	}
	for _, a := range r.asistentes {
		antes, visto := e.Antes[a.correo]
		if !visto || antes == a.estado || !respuestaValida(a.estado) {
			continue
		}
		b, ok := buzonDe(buzones, a.correo)
		if !ok {
			if c.propia(a.correo) {
				if c.Cola == nil {
					return true, SinRemitente{Direccion: a.correo, Resumen: r.resumen}
				}
				// A la cola: sale cuando haya cuenta de Mail con esa dirección.
				return true, c.responder(ctx, Buzon{Direccion: a.correo, Nombre: a.nombre}, r, a)
			}
			continue
		}
		return true, c.responder(ctx, b, r, a)
	}
	return false, nil
}

// propia dice si una dirección es la de la persona en la cuenta de calendario.
func (c *Cartero) propia(d string) bool {
	if d == "" {
		return false
	}
	if strings.EqualFold(d, c.Propia) {
		return true
	}
	return c.Direccion != nil && strings.EqualFold(d, c.Direccion())
}

func respuestaValida(p string) bool {
	return p == "ACCEPTED" || p == "TENTATIVE" || p == "DECLINED"
}

func buzonDe(bs []Buzon, direccion string) (Buzon, bool) {
	for _, b := range bs {
		if strings.EqualFold(strings.TrimSpace(b.Direccion), direccion) {
			return b, true
		}
	}
	return Buzon{}, false
}

func cambiaronInvitados(antes map[string]string, r reunion) bool {
	if antes == nil {
		return true
	}
	otros := r.invitados()
	if len(otros) != len(antes) {
		return true
	}
	for _, x := range otros {
		if _, ok := antes[x]; !ok {
			return true
		}
	}
	return false
}

// organiza manda, como organizador, lo que toca: CANCEL si se borró o se canceló;
// si no, REQUEST a todos si es nuevo o cambió, o solo a los que se añadieron, y CANCEL
// a los que se quitaron.
func (c *Cartero) organiza(ctx context.Context, b Buzon, r reunion, e Envio) error {
	otros := r.invitados()
	var quitados, nuevos []string
	for x := range e.Antes {
		if x != r.organizador && !slices.Contains(otros, x) {
			quitados = append(quitados, x)
		}
	}
	sort.Strings(quitados)
	for _, x := range otros {
		if _, ok := e.Antes[x]; !ok {
			nuevos = append(nuevos, x)
		}
	}
	var errs []error
	switch {
	case e.Borrado || r.cancelado:
		if para := slices.Compact(slices.Sorted(slices.Values(append(slices.Clone(otros), quitados...)))); len(para) > 0 {
			errs = append(errs, c.mandar(ctx, b, r, "CANCEL", para, nil))
		}
		return errors.Join(errs...)
	case e.Nuevo || e.Contenido || e.Antes == nil:
		if len(otros) > 0 {
			errs = append(errs, c.mandar(ctx, b, r, "REQUEST", otros, nil))
		}
	case len(nuevos) > 0:
		// Para los que se añaden es una invitación, no un cambio.
		rn := r
		rn.visto = false
		errs = append(errs, c.mandar(ctx, b, rn, "REQUEST", nuevos, nil))
	}
	if len(quitados) > 0 {
		errs = append(errs, c.mandar(ctx, b, r, "CANCEL", quitados, quitados))
	}
	return errors.Join(errs...)
}

func (c *Cartero) responder(ctx context.Context, b Buzon, r reunion, a asistenteReunion) error {
	return c.mandar(ctx, b, r, "REPLY", []string{r.organizador}, []string{a.correo})
}

// mandar compone y envía un mensaje iTIP. solo, si no es nil, deja en el calendario solo
// esos asistentes (el CANCEL a los quitados, el REPLY de quien contesta).
func (c *Cartero) mandar(ctx context.Context, b Buzon, r reunion, metodo string, para, solo []string) error {
	ahora := c.ahora()
	cal, err := calendarioITIP(r, metodo, solo, ahora)
	if err != nil {
		return err
	}
	t := c.Textos.con(textosIMIPIngles)
	var asunto string
	switch metodo {
	case "CANCEL":
		asunto = fmt.Sprintf(t.Cancelada, r.resumen)
	case "REPLY":
		var estado string
		for _, a := range r.asistentes {
			if len(solo) > 0 && a.correo == solo[0] {
				estado = a.estado
			}
		}
		// Como el REPLY de Mail: en inglés, que es lo que leen todos los calendarios.
		asunto = map[string]string{"ACCEPTED": "Accepted", "TENTATIVE": "Tentative", "DECLINED": "Declined"}[estado] + ": " + r.resumen
	default:
		asunto = fmt.Sprintf(t.Invitacion, r.resumen)
		if r.secuencia > 0 || r.visto {
			asunto = fmt.Sprintf(t.Actualizada, r.resumen)
		}
	}
	crudo, err := componerIMIP(b, para, metodo, asunto, textoIMIP(r, t, metodo), cal, ahora)
	if err != nil {
		return err
	}
	pend := PendienteIMIP{Direccion: b.Direccion, Para: para, UID: r.uid, Metodo: metodo, Resumen: r.resumen}
	if b.ID == "" {
		// Sin cuenta de Mail con esa dirección: espera en la cola a que la haya.
		if c.Cola != nil {
			if err := c.Cola.Guardar(pend, crudo); err != nil {
				return err
			}
		}
		return SinRemitente{Direccion: b.Direccion, Resumen: r.resumen}
	}
	if err := c.Correo.Enviar(ctx, b.ID, para, crudo); err != nil {
		if c.Cola != nil && !errors.Is(err, context.Canceled) {
			pend.Intentos, pend.Error = 1, err.Error()
			if qerr := c.Cola.Guardar(pend, crudo); qerr != nil {
				err = errors.Join(err, qerr)
			}
		}
		return FalloEnvio{Direccion: b.Direccion, Resumen: r.resumen, Err: err}
	}
	if c.Cola != nil {
		// Lo que esperaba en la cola para estas personas ya no hace falta.
		c.Cola.Superar(r.uid, para)
	}
	return nil
}

// ----- La reunión -----

type asistenteReunion struct {
	correo, nombre, estado string
}

type reunion struct {
	cal                *ical.Calendar
	uid, resumen       string
	lugar, descripcion string
	organizador        string // en minúsculas
	nombreOrganizador  string
	asistentes         []asistenteReunion // del maestro (o del primer evento)
	cancelado          bool
	secuencia          int
	visto              bool // se usa en el asunto: una actualización
	inicio             time.Time
	todoElDia          bool
	zona               string
}

// invitados son los asistentes que no son el organizador.
func (r reunion) invitados() []string {
	var out []string
	for _, a := range r.asistentes {
		if a.correo != r.organizador && !slices.Contains(out, a.correo) {
			out = append(out, a.correo)
		}
	}
	return out
}

func direccionICS(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 7 && strings.EqualFold(v[:7], "mailto:") {
		v = v[7:]
	}
	return strings.ToLower(strings.TrimSpace(v))
}

func principal(cal *ical.Calendar) *ical.Component {
	var primero *ical.Component
	for _, c := range cal.Children {
		if c.Name != ical.CompEvent {
			continue
		}
		if primero == nil {
			primero = c
		}
		if c.Props.Get(ical.PropRecurrenceID) == nil {
			return c
		}
	}
	return primero
}

func leerReunion(datos string) (reunion, error) {
	cal, err := ical.NewDecoder(strings.NewReader(datos)).Decode()
	if err != nil {
		return reunion{}, err
	}
	ev := principal(cal)
	if ev == nil {
		return reunion{}, errors.New("agenda: el calendario no tiene eventos")
	}
	texto := func(n string) string { t, _ := ev.Props.Text(n); return strings.TrimSpace(t) }
	r := reunion{cal: cal, uid: texto(ical.PropUID), resumen: texto(ical.PropSummary), lugar: texto(ical.PropLocation), descripcion: texto(ical.PropDescription)}
	if p := ev.Props.Get(ical.PropOrganizer); p != nil {
		r.organizador = direccionICS(p.Value)
		r.nombreOrganizador = p.Params.Get(ical.ParamCommonName)
	}
	for _, p := range ev.Props.Values(ical.PropAttendee) {
		if d := direccionICS(p.Value); d != "" {
			r.asistentes = append(r.asistentes, asistenteReunion{
				correo: d, nombre: p.Params.Get(ical.ParamCommonName),
				estado: strings.ToUpper(cmp.Or(p.Params.Get(ical.ParamParticipationStatus), "NEEDS-ACTION")),
			})
		}
	}
	r.cancelado = strings.EqualFold(texto(ical.PropStatus), "CANCELLED")
	// SEQUENCE es un entero: `Props.Text` no lo lee (daba siempre 0, y lo que se mandaba
	// tras un cambio llevaba SEQUENCE:1 aunque el evento fuera por la 3).
	r.secuencia, _ = strconv.Atoi(strings.TrimSpace(valorDe(ev, ical.PropSequence)))
	if p := ev.Props.Get(ical.PropDateTimeStart); p != nil {
		r.todoElDia = p.ValueType() == ical.ValueDate
		r.zona = p.Params.Get(ical.ParamTimezoneID)
		loc := time.UTC
		if r.zona != "" {
			if l, err := time.LoadLocation(r.zona); err == nil {
				loc = l
			} else {
				r.zona = ""
			}
		}
		r.inicio, _ = p.DateTime(loc)
	}
	return r, nil
}

// ----- La firma -----

// firma es lo que se recuerda de un evento con invitados para saber, al subirlo, qué
// cambió: el resumen de lo que ven los invitados y sus respuestas. Se guarda en el
// estado como «contenido|correo=PARTSTAT,…».
//
// El contenido es el de [calical.ContenidoDeLaInvitacion]: lo que SIGNIFICA cada
// propiedad, no cómo está escrita (la EXDATE en su zona o en UTC, el STATUS que falta o
// CONFIRMED…), así que lo que vino de la cuenta escrito de otra forma y se guarda aquí
// sin cambiar nada no manda la invitación. Va con el prefijo [versionFirma]; las firmas
// de antes (el texto de cada propiedad) se siguen entendiendo: una firma recién hecha
// lleva también la de antes (`textual`) para compararse con ellas.
type firma struct {
	contenido string
	invitados map[string]string
	// textual es el contenido como se firmaba antes, solo en una firma recién hecha.
	textual string
}

// versionFirma va delante del contenido de las firmas por lo que significa.
const versionFirma = "2."

func (f firma) vacia() bool { return f.contenido == "" }

// deAntes: es una firma guardada por una versión anterior (el texto de cada propiedad).
func (f firma) deAntes() bool {
	return f.contenido != "" && !strings.HasPrefix(f.contenido, versionFirma)
}

func (f firma) String() string {
	if f.vacia() {
		return ""
	}
	ks := slices.Sorted(func(yield func(string) bool) {
		for k := range f.invitados {
			if !yield(k) {
				return
			}
		}
	})
	partes := make([]string, 0, len(ks))
	for _, k := range ks {
		partes = append(partes, k+"="+f.invitados[k])
	}
	return f.contenido + "|" + strings.Join(partes, ",")
}

func leerFirma(s string) firma {
	contenido, resto, ok := strings.Cut(s, "|")
	if !ok || contenido == "" {
		return firma{}
	}
	f := firma{contenido: contenido, invitados: map[string]string{}}
	for _, p := range strings.Split(resto, ",") {
		if k, v, ok := strings.Cut(p, "="); ok && k != "" {
			f.invitados[k] = v
		}
	}
	return f
}

// mismoContenido dice si las dos firmas son de lo mismo para los invitados. Si una es de
// antes, se compara con la de antes de la otra (la recién hecha la lleva).
func (f firma) mismoContenido(g firma) bool {
	switch {
	case f.deAntes() == g.deAntes():
		return f.contenido == g.contenido
	case g.deAntes():
		return f.textual != "" && f.textual == g.contenido
	default:
		return g.textual != "" && g.textual == f.contenido
	}
}

// igual dice si las dos firmas son la misma: lo mismo para los invitados y las mismas
// respuestas.
func (f firma) igual(g firma) bool {
	if f.vacia() || g.vacia() {
		return f.vacia() && g.vacia()
	}
	return f.mismoContenido(g) && maps.Equal(f.invitados, g.invitados)
}

// propsFirma son las propiedades de cada VEVENT que, si cambian, cambian la invitación:
// las mismas con que Calendar decide si sube la SEQUENCE al guardar.
var propsFirma = calical.PropiedadesDeLaInvitacion

// firmaInvitacion es la firma de un VCALENDAR; vacía si no tiene invitados.
func firmaInvitacion(datos string) firma {
	if !strings.Contains(datos, "ATTENDEE") {
		return firma{}
	}
	cal, err := ical.NewDecoder(strings.NewReader(datos)).Decode()
	if err != nil {
		return firma{}
	}
	f := firma{invitados: map[string]string{}}
	var trozos []string
	for _, ev := range cal.Children {
		if ev.Name != ical.CompEvent {
			continue
		}
		var b strings.Builder
		for _, n := range propsFirma {
			for _, p := range ev.Props.Values(n) {
				b.WriteString(propLinea(p) + "\n")
			}
		}
		var as []string
		for _, p := range ev.Props.Values(ical.PropAttendee) {
			d := direccionICS(p.Value)
			if d == "" {
				continue
			}
			as = append(as, d)
			if ev.Props.Get(ical.PropRecurrenceID) == nil {
				f.invitados[d] = strings.ToUpper(cmp.Or(p.Params.Get(ical.ParamParticipationStatus), "NEEDS-ACTION"))
			}
		}
		sort.Strings(as)
		b.WriteString(strings.Join(as, ","))
		trozos = append(trozos, b.String())
	}
	if len(f.invitados) == 0 {
		return firma{}
	}
	sort.Strings(trozos)
	h := sha256.Sum256([]byte(strings.Join(trozos, "\x00")))
	f.textual = hex.EncodeToString(h[:12])
	h = sha256.Sum256([]byte(calical.ContenidoDeLaInvitacion(cal)))
	f.contenido = versionFirma + hex.EncodeToString(h[:12])
	return f
}

// ----- El mensaje -----

// calendarioITIP es el VCALENDAR con METHOD del mensaje: el objeto entero en REQUEST
// (sin los recordatorios de quien organiza), y en CANCEL y REPLY lo que pide RFC 5546
// (§3.2.5, §3.2.3) con los asistentes de solo, si se dan.
func calendarioITIP(r reunion, metodo string, solo []string, ahora time.Time) ([]byte, error) {
	out := ical.NewCalendar()
	out.Props.SetText(ical.PropVersion, "2.0")
	out.Props.SetText(ical.PropProductID, "-//Sytue//Sytue Office Calendar//ES")
	out.Props.SetText(ical.PropMethod, metodo)
	for _, c := range r.cal.Children {
		if c.Name == ical.CompTimezone && metodo == "REQUEST" {
			out.Children = append(out.Children, c)
		}
	}
	for _, ev := range r.cal.Children {
		if ev.Name != ical.CompEvent {
			continue
		}
		maestro := ev.Props.Get(ical.PropRecurrenceID) == nil
		if metodo != "REQUEST" && !maestro {
			continue // la cancelación o la respuesta son de la serie entera
		}
		out.Children = append(out.Children, eventoITIP(ev, r, metodo, solo, ahora))
	}
	var b bytes.Buffer
	if err := ical.NewEncoder(&b).Encode(out); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// eventoITIP es el VEVENT que viaja en el mensaje: entero en un REQUEST (sin los
// recordatorios); en un CANCEL o un REPLY, lo que identifica la cita y los asistentes
// de solo.
func eventoITIP(ev *ical.Component, r reunion, metodo string, solo []string, ahora time.Time) *ical.Component {
	nuevo := ical.NewComponent(ical.CompEvent)
	if metodo == "REQUEST" {
		for n, ps := range ev.Props {
			nuevo.Props[n] = slices.Clone(ps)
		}
	} else {
		identificarCita(nuevo, ev, solo)
	}
	if metodo != "REPLY" && !r.esNueva() {
		// Lo que manda quien organiza tras la primera vez lleva una secuencia mayor
		// que la que ya tienen los invitados (RFC 5546 §2.1.4).
		sec := ical.NewProp(ical.PropSequence)
		sec.Value = strconv.Itoa(r.secuencia + 1)
		nuevo.Props.Set(sec)
	}
	if metodo == "CANCEL" {
		nuevo.Props.SetText(ical.PropStatus, "CANCELLED")
	}
	nuevo.Props.SetDateTime(ical.PropDateTimeStamp, ahora.UTC())
	if metodo == "REQUEST" {
		// Los recordatorios son de quien organiza.
		for _, h := range ev.Children {
			if h.Name != ical.CompAlarm {
				nuevo.Children = append(nuevo.Children, h)
			}
		}
	}
	return nuevo
}

// identificarCita copia de ev a nuevo lo que identifica la cita y los asistentes de solo
// (todos, si es nil).
func identificarCita(nuevo, ev *ical.Component, solo []string) {
	for _, n := range []string{
		ical.PropUID, ical.PropSequence, ical.PropDateTimeStart, ical.PropDateTimeEnd, ical.PropDuration,
		ical.PropSummary, ical.PropOrganizer, ical.PropRecurrenceRule,
	} {
		for _, p := range ev.Props.Values(n) {
			nuevo.Props.Add(&p)
		}
	}
	puestos := map[string]bool{}
	for _, p := range ev.Props.Values(ical.PropAttendee) {
		if d := direccionICS(p.Value); solo == nil || slices.Contains(solo, d) {
			nuevo.Props.Add(&p)
			puestos[d] = true
		}
	}
	// A quien se quitó de la lista ya no está en ella: la cancelación lo nombra.
	for _, d := range solo {
		if !puestos[d] {
			a := ical.NewProp(ical.PropAttendee)
			a.Value = "mailto:" + d
			nuevo.Props.Add(a)
		}
	}
}

func (r reunion) esNueva() bool { return !r.visto }

// textoIMIP es la parte de texto del correo: qué, cuándo, dónde y quién organiza.
func textoIMIP(r reunion, t TextosIMIP, metodo string) string {
	var b strings.Builder
	b.WriteString(r.resumen + "\n\n")
	if !r.inicio.IsZero() {
		cuando := r.inicio.Format("2006-01-02 15:04")
		if r.todoElDia {
			cuando = r.inicio.Format("2006-01-02")
		}
		if r.zona != "" && !r.todoElDia {
			cuando += " (" + r.zona + ")"
		} else if !r.todoElDia {
			cuando += " UTC"
		}
		b.WriteString(t.Cuando + ": " + cuando + "\n")
	}
	if r.lugar != "" {
		b.WriteString(t.Donde + ": " + r.lugar + "\n")
	}
	if r.organizador != "" {
		b.WriteString(t.Organiza + ": " + (&mail.Address{Name: r.nombreOrganizador, Address: r.organizador}).String() + "\n")
	}
	if r.descripcion != "" && metodo == "REQUEST" {
		b.WriteString("\n" + r.descripcion + "\n")
	}
	return b.String()
}

// componerIMIP escribe el correo (RFC 6047 §2.4): `multipart/mixed` con el texto y el
// `text/calendar; method=…` en un `multipart/alternative` y, al lado, el mismo
// calendario como `invite.ics`, que es lo que leen los programas que no miran la parte
// de calendario.
func componerIMIP(de Buzon, para []string, metodo, asunto, texto string, cal []byte, ahora time.Time) ([]byte, error) {
	var b bytes.Buffer
	cab := func(n, v string) { b.WriteString(n + ": " + v + "\r\n") }
	unaLinea := func(s string) string {
		return strings.Join(strings.FieldsFunc(s, func(r rune) bool { return r == '\r' || r == '\n' }), " ")
	}
	var id [12]byte
	_, _ = rand.Read(id[:])
	dominio := de.Direccion[strings.LastIndex(de.Direccion, "@")+1:]
	destinos := make([]string, 0, len(para))
	for _, p := range para {
		destinos = append(destinos, (&mail.Address{Address: unaLinea(p)}).String())
	}
	mixto := multipart.NewWriter(&b)
	cab("From", (&mail.Address{Name: unaLinea(de.Nombre), Address: unaLinea(de.Direccion)}).String())
	cab("To", strings.Join(destinos, ", "))
	cab("Subject", mime.QEncoding.Encode("utf-8", unaLinea(asunto)))
	cab("Date", ahora.Format(time.RFC1123Z))
	cab("Message-ID", "<"+hex.EncodeToString(id[:])+"@"+unaLinea(dominio)+">")
	cab("MIME-Version", "1.0")
	cab("Content-Type", mime.FormatMediaType("multipart/mixed", map[string]string{"boundary": mixto.Boundary()}))
	b.WriteString("\r\n")

	var alt bytes.Buffer
	aw := multipart.NewWriter(&alt)
	p, err := aw.CreatePart(textproto.MIMEHeader{"Content-Type": {"text/plain; charset=utf-8"}, "Content-Transfer-Encoding": {"quoted-printable"}})
	if err != nil {
		return nil, err
	}
	qp := quotedprintable.NewWriter(p)
	_, _ = qp.Write([]byte(strings.ReplaceAll(strings.ReplaceAll(texto, "\r\n", "\n"), "\n", "\r\n")))
	_ = qp.Close()
	p, err = aw.CreatePart(textproto.MIMEHeader{
		"Content-Type":              {mime.FormatMediaType("text/calendar", map[string]string{"charset": "utf-8", "method": metodo, "component": "VEVENT"})},
		"Content-Transfer-Encoding": {"base64"},
	})
	if err != nil {
		return nil, err
	}
	escribirBase64(p, cal)
	if err := aw.Close(); err != nil {
		return nil, err
	}
	p, err = mixto.CreatePart(textproto.MIMEHeader{"Content-Type": {mime.FormatMediaType("multipart/alternative", map[string]string{"boundary": aw.Boundary()})}})
	if err != nil {
		return nil, err
	}
	_, _ = p.Write(alt.Bytes())
	p, err = mixto.CreatePart(textproto.MIMEHeader{
		"Content-Type":              {mime.FormatMediaType("application/ics", map[string]string{"name": "invite.ics"})},
		"Content-Disposition":       {mime.FormatMediaType("attachment", map[string]string{"filename": "invite.ics"})},
		"Content-Transfer-Encoding": {"base64"},
	})
	if err != nil {
		return nil, err
	}
	escribirBase64(p, cal)
	if err := mixto.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func escribirBase64(w io.Writer, datos []byte) {
	s := base64.StdEncoding.EncodeToString(datos)
	for len(s) > 76 {
		_, _ = io.WriteString(w, s[:76]+"\r\n")
		s = s[76:]
	}
	_, _ = io.WriteString(w, s+"\r\n")
}
