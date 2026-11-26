// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package traer

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/calendar"
	calendarical "git.sytue.eu/sytue/one/apps/nucleo/calendar/ical"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts/card"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
	"git.sytue.eu/sytue/one/apps/nucleo/local/libreta"
)

// # Traer otra vez sin repetir
//
// Calendar, Contacts, Docus, Calcus, Diapos y Mail no guardan de dónde vino cada cosa,
// así que lo que se trae a ellos se apunta en la memoria de lo traído (memoria.go) y se
// casa con lo de la copia:
//
//   - Calendar: cada `.ics` de la copia va SIEMPRE al mismo calendario de aquí (el que se
//     creó para él la primera vez), y cada evento se casa por su UID: el objeto entero,
//     con su maestro y sus excepciones (RECURRENCE-ID), como lo guarda Calendar;
//   - Contacts: todo va a la misma libreta «Sytue», y cada contacto se casa por el UID de
//     su vCard (sin él, por la huella de la tarjeta; en una copia en CSV, por su clave: el
//     correo o, sin él, el nombre);
//   - Docus, Calcus y Diapos: cada fichero de `docs/documents` por su ruta en la copia;
//   - Mail: cada correo por su Message-ID (sin él, por la huella del .eml) entre los de
//     «Importados» de la cuenta local.
//
// Con lo casado se hace lo prudente, que es no pisar nunca lo que la persona cambió:
//
//   - lo que ya estaba igual, o viene de una copia más vieja que la de la última vez, se
//     queda como está (Repetidos);
//   - lo que cambió en Sytue y aquí no se ha tocado desde que se trajo (lo de aquí sigue
//     con la huella que tenía al traerlo) se pone al día (Actualizados): un evento o un
//     contacto, sobre el mismo; un documento, como versión nueva del mismo, guardando
//     antes la de ahora en su historial;
//   - lo que cambió en Sytue Y aquí no se pisa (Conservados): un documento entra aparte,
//     como «<título> (de Sytue, <fecha de la copia>)», y es ese el que sigue a Sytue desde
//     entonces; un evento o un contacto se queda como lo dejó la persona, y el fichero de
//     la copia (el .ics o el .vcf) se deja tal cual en la carpeta de lo traído;
//   - lo que se trajo y la persona quitó de aquí (lo borró, o lo llevó a otro calendario,
//     otra libreta u otra carpeta) no vuelve (Borrados), como una nota tirada a la
//     papelera. Si quitó el calendario o la libreta enteros, se crean otra vez con todo,
//     como un cuaderno de Notepads.
//
// Sin memoria (la primera vez con esta versión, o si se perdió), un evento o un contacto
// cuyo UID ya está en su calendario o su libreta, un documento anclado a su ruta en la
// copia (su origen: `sytue:<ruta>`) y un correo que ya está en «Importados» se dan por
// traídos: no se repiten ni se pisan.

// Lo que cambia cada vez que se escribe un evento o una tarjeta sin que cambie nada: no
// cuenta para su huella.
var (
	sinEnEventos  = []string{"DTSTAMP", "PRODID"}
	sinEnTarjetas = []string{"REV", "PRODID"}
)

// huellaDeEvento es la huella de un objeto de Calendar (el VCALENDAR de un UID).
func huellaDeEvento(datos string) string { return huellaSin(datos, sinEnEventos...) }

// masVieja dice si la copia de fecha copia es anterior a la de fecha ultima (vacías, no
// se sabe: no lo es).
func masVieja(copia, ultima string) bool { return copia != "" && ultima != "" && copia < ultima }

// ----- Calendar y Contacts -----

// crudo es un evento o un contacto de aquí, entero: el VCALENDAR de su UID o su vCard.
type crudo struct{ id, uid, datos string }

// deLaCopia es un evento o un contacto de la copia: su UID, su texto entero y su huella
// (huellaSin).
type deLaCopia struct{ uid, datos, huella string }

// elementos es un calendario o una libreta de aquí, que se lee y se escribe entero por
// cada UID (como los sincroniza `local/agenda`).
type elementos interface {
	// leer son los vivos, por su UID.
	leer() (map[string]crudo, error)
	// poner guarda datos: con id "" lo crea; si no, lo cambia si sigue con esperado.
	poner(id, esperado, datos string) (crudo, error)
	// cambioLocal dice si err es que lo de aquí cambió entre leerlo y ponerlo.
	cambioLocal(err error) bool
}

// casar trae los elementos de la copia a el, cuya memoria es col, sin repetir ni pisar
// lo que la persona cambió (ver arriba), y lo cuenta en p. normal es la huella de uno de
// aquí comparable con la de la copia, para reconocerlo sin memoria (nil, no se sabe).
// dejar dice si algo de la copia no ha entrado y hay que dejar su fichero tal cual.
func (r *repartidor) casar(p *Parte, col *coleccion, el elementos, copia []deLaCopia, normal func(string) string) (dejar bool, err error) {
	aqui, err := el.leer()
	if err != nil {
		return true, err
	}
	apuntar := func(uid, huella string, quedo crudo) {
		col.Elementos[uid] = traido{S: huella, A: huellaCorta([]byte(quedo.datos)), C: r.copia}
	}
	for _, c := range copia {
		if err := r.ctx.Err(); err != nil {
			return true, err
		}
		m, conMemoria := col.Elementos[c.uid]
		l, esta := aqui[c.uid]
		switch {
		case !esta && conMemoria: // la persona lo quitó: no vuelve
			p.Borrados++
		case !esta:
			nuevo, err := el.poner("", "", c.datos)
			if err != nil {
				p.Fallidos++
				dejar = true
				continue
			}
			apuntar(c.uid, c.huella, nuevo)
			aqui[c.uid] = nuevo
			p.Importados++
		case !conMemoria: // ya estaba, de antes de apuntarlo
			p.Repetidos++
			if normal != nil && normal(l.datos) == c.huella {
				apuntar(c.uid, c.huella, l)
			}
		case c.huella == m.S || masVieja(r.copia, m.C):
			p.Repetidos++
		case huellaCorta([]byte(l.datos)) == m.A: // cambió en Sytue y aquí no se ha tocado
			nuevo, err := el.poner(l.id, l.datos, c.datos)
			switch {
			case el.cambioLocal(err):
				p.Conservados++
				dejar = true
			case err != nil:
				p.Fallidos++
				dejar = true
			default:
				apuntar(c.uid, c.huella, nuevo)
				p.Actualizados++
			}
		default: // cambió en Sytue y aquí: se queda lo de aquí
			p.Conservados++
			dejar = true
			m.S, m.C = c.huella, cmp.Or(r.copia, m.C)
			col.Elementos[c.uid] = m
		}
	}
	return dejar, nil
}

// calendarioDeAqui es un calendario de Calendar como [elementos].
type calendarioDeAqui struct {
	ctx context.Context
	k   *calendar.Repository
	id  string
}

func (c calendarioDeAqui) leer() (map[string]crudo, error) {
	objetos, err := c.k.ObjetosCrudos(c.ctx, c.id)
	if err != nil {
		return nil, err
	}
	out := make(map[string]crudo, len(objetos))
	for _, o := range objetos {
		out[o.UID] = crudo{id: o.ID, uid: o.UID, datos: o.Datos}
	}
	return out, nil
}

func (c calendarioDeAqui) poner(id, esperado, datos string) (crudo, error) {
	o, err := c.k.PonerObjetoCrudo(c.ctx, c.id, id, esperado, datos)
	return crudo{id: o.ID, uid: o.UID, datos: o.Datos}, err
}

func (calendarioDeAqui) cambioLocal(err error) bool { return errors.Is(err, calendar.ErrCambioLocal) }

// calendario trae cada `.ics` de `calendar/calendars/` a su calendario de aquí (uno por
// fichero, con el nombre del fichero: calendarioDe), sin repetir lo que ya se trajo. Lo
// que no es un .ics, o no entra entero, se copia.
func (r *repartidor) calendario(es []entrada) ([]entrada, error) {
	var ics, resto []entrada
	for _, e := range es {
		if strings.HasPrefix(e.ruta, "calendar/calendars/") && strings.EqualFold(path.Ext(e.ruta), ".ics") {
			ics = append(ics, e)
		} else {
			resto = append(resto, e)
		}
	}
	if len(ics) == 0 {
		return resto, nil
	}
	rec, err := r.abrirAlmacen("calendar")
	if err != nil {
		return es, err
	}
	defer rec.Cerrar()
	k, err := calendar.New(rec.Config)
	if err != nil {
		return es, err
	}
	defer k.Close()
	if err := almacen.Preparar(r.ctx, k); err != nil {
		return es, err
	}
	p := r.parte("calendar")
	var errs []error
	for _, e := range ics {
		datos, err := e.leer()
		if err != nil {
			resto = append(resto, e)
			continue
		}
		objetos, err := calendarical.DecodeICSFeed(string(datos), calendar.MaxImportarICS)
		if err != nil {
			resto = append(resto, e)
			continue
		}
		col, err := r.calendarioDe(k, e)
		if err != nil {
			errs = append(errs, err)
			resto = append(resto, e)
			continue
		}
		copia := make([]deLaCopia, 0, len(objetos))
		for _, po := range objetos {
			copia = append(copia, deLaCopia{uid: po.UID, datos: po.Master.ICalData, huella: huellaDeEvento(po.Master.ICalData)})
		}
		dejar, err := r.casar(p, col, calendarioDeAqui{ctx: r.ctx, k: k, id: col.ID}, copia, huellaDeEvento)
		if err != nil {
			errs = append(errs, err)
		}
		if dejar {
			resto = append(resto, e) // el fichero entero, para no perder lo que no entró
		}
	}
	return resto, errors.Join(errs...)
}

// calendarioDe es el calendario de aquí al que va el .ics e de la copia, con lo que ya
// se trajo a él: el de la memoria, si sigue; si no, el que se creó para él (su nombre
// interno sale de su ruta en la copia) o, de cuando no se apuntaba, uno «sytue-<uuid>»
// con su mismo nombre; y si no hay ninguno, uno nuevo con el nombre del fichero.
func (r *repartidor) calendarioDe(k *calendar.Repository, e entrada) (*coleccion, error) {
	if col := r.mem.Calendarios[e.ruta]; col != nil {
		if sigue, err := k.ExisteCalendario(r.ctx, col.ID); err != nil || sigue {
			return col, err
		}
	}
	nombre := strings.TrimSuffix(path.Base(e.ruta), path.Ext(e.ruta))
	interno := "sytue-" + huellaCorta([]byte(e.ruta))[:16]
	cals, err := k.List(r.ctx)
	if err != nil {
		return nil, err
	}
	id := ""
	for _, c := range cals {
		if c.Name == interno && !c.ReadOnly {
			id = c.ID
			break
		}
	}
	for _, c := range cals {
		if u, ok := strings.CutPrefix(c.Name, "sytue-"); id == "" && ok && !c.ReadOnly && c.DisplayName == nombre {
			if _, err := uuid.Parse(u); err == nil {
				id = c.ID
			}
		}
	}
	if id == "" {
		cal, err := k.CreateCalendar(r.ctx, api.CalendarInput{Name: interno, DisplayName: nombre})
		if err != nil {
			return nil, err
		}
		id = cal.ID
	}
	col := &coleccion{ID: id, Elementos: map[string]traido{}}
	r.mem.Calendarios[e.ruta] = col
	return col, nil
}

// libretaDeAqui es una libreta de Contacts como [elementos].
type libretaDeAqui struct {
	ctx context.Context
	c   contacts.LibretasExternas
	id  string
}

func (l libretaDeAqui) leer() (map[string]crudo, error) {
	ts, err := l.c.TarjetasCrudas(l.ctx, l.id)
	if err != nil {
		return nil, err
	}
	out := make(map[string]crudo, len(ts))
	for _, t := range ts {
		out[card.CleanUID(t.UID)] = crudo{id: t.ID, uid: t.UID, datos: t.Datos}
	}
	return out, nil
}

func (l libretaDeAqui) poner(id, esperado, datos string) (crudo, error) {
	t, err := l.c.PonerTarjetaCruda(l.ctx, l.id, id, esperado, datos)
	return crudo{id: t.ID, uid: t.UID, datos: t.Datos}, err
}

func (libretaDeAqui) cambioLocal(err error) bool { return errors.Is(err, contacts.ErrCambioLocal) }

// libretaInterna es el nombre interno de la libreta «Sytue» de lo traído.
const libretaInterna = "sytue-traido"

// tarjetas parte un .vcf en sus VCARD.
func tarjetas(vcf string) []string {
	vcf = strings.ReplaceAll(vcf, "\r\n", "\n")
	var out []string
	var actual []string
	dentro := false
	for linea := range strings.SplitSeq(vcf, "\n") {
		l := strings.TrimSpace(linea)
		switch {
		case strings.EqualFold(l, "BEGIN:VCARD"):
			dentro, actual = true, []string{linea} //nolint:prealloc // la tarjeta en curso: una nueva en cada BEGIN:VCARD
		case dentro && strings.EqualFold(l, "END:VCARD"):
			actual = append(actual, linea)
			out = append(out, strings.Join(actual, "\r\n")+"\r\n")
			dentro = false
		case dentro:
			actual = append(actual, linea)
		}
	}
	return out
}

// uidDeTarjeta es el UID de una vCard, como lo guarda Contacts (sin `urn:uuid:`); vacío
// si no lo lleva.
func uidDeTarjeta(t string) string {
	t = strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(t, "\r\n", "\n"), "\n ", ""), "\n\t", "")
	for linea := range strings.SplitSeq(t, "\n") {
		if propiedad(linea) != "UID" {
			continue
		}
		if _, v, ok := strings.Cut(linea, ":"); ok {
			return card.CleanUID(v)
		}
	}
	return ""
}

// conUID es la vCard t con uid por UID (sin el que tuviera).
func conUID(t, uid string) string {
	t = strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(t, "\r\n", "\n"), "\n ", ""), "\n\t", "")
	var out []string
	for linea := range strings.SplitSeq(strings.TrimRight(t, "\n"), "\n") {
		switch {
		case propiedad(linea) == "UID":
		case strings.EqualFold(strings.TrimSpace(linea), "BEGIN:VCARD"):
			out = append(out, linea, "UID:urn:uuid:"+uid)
		default:
			out = append(out, linea)
		}
	}
	return strings.Join(out, "\r\n") + "\r\n"
}

// uidDerivado es un UID que sale siempre igual de la misma clave: el de un contacto de
// la copia que no trae uno (o trae el de otro).
func uidDerivado(clave string) string {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("sytue-traer\x00"+clave)).String()
}

// contactosDeVCF son los contactos de un .vcf de la copia. Uno sin UID se casa por la
// huella de su tarjeta (le da un UID que sale de ella); uno con el UID de otro anterior
// (el mismo contacto en dos libretas de Sytue), por el suyo y el orden en que viene.
func contactosDeVCF(vcf string) []deLaCopia {
	vistos := map[string]int{}
	ts := tarjetas(vcf)
	out := make([]deLaCopia, 0, len(ts))
	for _, t := range ts {
		huella := huellaSin(t, sinEnTarjetas...)
		suyo := uidDeTarjeta(t)
		uid := cmp.Or(suyo, uidDerivado("vcf\x00"+huella))
		if n := vistos[uid]; n > 0 {
			vistos[uid]++
			uid = uidDerivado("vcf\x00" + uid + "\x00" + strconv.Itoa(n))
		} else {
			vistos[uid] = 1
		}
		if uid != suyo {
			t = conUID(t, uid)
		}
		out = append(out, deLaCopia{uid: uid, datos: t, huella: huella})
	}
	return out
}

// contactosDeCSV son los contactos de un `contacts.csv` de la copia (el de Google
// Contacts), cada uno con un UID que sale de su clave: su correo o, sin él, su nombre (y,
// si dos la comparten, el orden en que vienen). saltadas son las filas que no se leen.
func contactosDeCSV(datos []byte) (out []deLaCopia, saltadas int, err error) {
	if len(datos) > 5<<20 {
		return nil, 0, errors.New("traer: el CSV de contactos es demasiado grande")
	}
	gente, saltadas, err := card.ParseContactsCSV(bytes.NewReader(datos))
	if err != nil {
		return nil, 0, err
	}
	vistos := map[string]int{}
	for _, c := range gente {
		clave := strings.ToLower(strings.TrimSpace(c.PrimaryEmail()))
		if clave == "" {
			clave = "nombre:" + strings.ToLower(strings.TrimSpace(c.DisplayName()))
		}
		if n := vistos[clave]; n > 0 {
			vistos[clave]++
			clave += "\x00" + strconv.Itoa(n)
		} else {
			vistos[clave] = 1
		}
		c.UID = uidDerivado("csv\x00" + clave)
		t, err := card.EncodeContact(c, "")
		if err != nil {
			saltadas++
			continue
		}
		out = append(out, deLaCopia{uid: c.UID, datos: t, huella: huellaSin(t, sinEnTarjetas...)})
	}
	return out, saltadas, nil
}

// contactos lleva `contacts/contacts.vcf` (o, si la copia se pidió en CSV,
// `contacts/contacts.csv`) a la libreta «Sytue» (libretaDe), sin repetir lo que ya se
// trajo.
func (r *repartidor) contactos(es []entrada) ([]entrada, error) {
	fuente, resto := fuenteDeContactos(es)
	if fuente == nil {
		return resto, nil
	}
	datos, err := fuente.leer()
	if err != nil {
		return es, err
	}
	p := r.parte("contacts")
	var copia []deLaCopia
	saltadas := 0
	if strings.HasSuffix(fuente.ruta, ".csv") {
		if copia, saltadas, err = contactosDeCSV(datos); err != nil {
			return append(resto, *fuente), err
		}
	} else {
		copia = contactosDeVCF(string(datos))
	}
	// La libreta de Contacts de Sytue Office (`local/libreta`); la primera vez pasa a ella
	// el almacén de registros de antes, como al abrir Contacts (las apps que escriben en
	// Contacts están cerradas: [Escriben]).
	c, err := libreta.Abrir(r.ctx, filepath.Join(r.d.Datos, "Datos", "contacts"), r.d.Sujeto)
	if err != nil {
		return es, err
	}
	defer c.Close()
	col, err := r.libretaDe(c)
	if err != nil {
		return es, err
	}
	p.Fallidos += saltadas
	dejar, err := r.casar(p, col, libretaDeAqui{ctx: r.ctx, c: c, id: col.ID}, copia, nil)
	if dejar || saltadas > 0 {
		resto = append(resto, *fuente)
	}
	return resto, err
}

// fuenteDeContactos es `contacts/contacts.vcf` o, si la copia se pidió en CSV,
// `contacts/contacts.csv` (nil si no hay ninguno), y las demás entradas.
func fuenteDeContactos(es []entrada) (*entrada, []entrada) {
	var fuente *entrada
	for i, e := range es {
		switch {
		case e.ruta == "contacts/contacts.vcf":
			fuente = &es[i]
		case e.ruta == "contacts/contacts.csv" && (fuente == nil || fuente.ruta != "contacts/contacts.vcf"):
			fuente = &es[i]
		}
	}
	var resto []entrada
	for _, e := range es {
		if fuente == nil || e.ruta != fuente.ruta {
			resto = append(resto, e)
		}
	}
	return fuente, resto
}

// libretaDe es la libreta «Sytue» de lo traído, con lo que ya se trajo a ella: la de la
// memoria, si sigue; si no, la de su nombre interno; y si no hay ninguna (o la persona
// la borró), una nueva.
func (r *repartidor) libretaDe(c contacts.LibretasExternas) (*coleccion, error) {
	const origen = "contacts"
	if col := r.mem.Libretas[origen]; col != nil {
		if sigue, err := c.ExisteLibreta(r.ctx, col.ID); err != nil || sigue {
			return col, err
		}
	}
	libretas, err := c.AddressBooks(r.ctx)
	if err != nil {
		return nil, err
	}
	id := ""
	for _, l := range libretas {
		if strings.EqualFold(l.Name, libretaInterna) {
			id = l.ID
			break
		}
	}
	if id == "" {
		l, err := c.CreateAddressBookAt(r.ctx, libretaInterna, "Sytue", "")
		if err != nil {
			return nil, err
		}
		id = l.ID
	}
	col := &coleccion{ID: id, Elementos: map[string]traido{}}
	r.mem.Libretas[origen] = col
	return col, nil
}
