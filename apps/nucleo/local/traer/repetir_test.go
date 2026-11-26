// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package traer_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/calendar"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
	"git.sytue.eu/sytue/one/apps/nucleo/local/biblioteca"
	"git.sytue.eu/sytue/one/apps/nucleo/local/correo"
	"git.sytue.eu/sytue/one/apps/nucleo/local/libreta"
	"git.sytue.eu/sytue/one/apps/nucleo/local/traer"
)

// agenda abre los calendarios de Sytue Office de la carpeta de datos, como Calendar.
func agenda(t *testing.T, datos string) (*calendar.Repository, func()) {
	t.Helper()
	rec, err := almacen.Abrir(context.Background(), filepath.Join(datos, "Datos", "calendar"), "calendar", "instalacion")
	if err != nil {
		t.Fatal(err)
	}
	k, err := calendar.New(rec.Config)
	if err != nil {
		rec.Cerrar()
		t.Fatal(err)
	}
	if err := almacen.Preparar(t.Context(), k); err != nil {
		t.Fatal(err)
	}
	return k, func() { k.Close(); rec.Cerrar() }
}

// agendaDe abre los contactos de Sytue Office de la carpeta de datos, como Contacts: su
// libreta SQLite (`local/libreta`).
func agendaDe(t *testing.T, datos string) (*libreta.Libreta, func()) {
	t.Helper()
	c, err := libreta.Abrir(t.Context(), filepath.Join(datos, "Datos", "contacts"), "instalacion")
	if err != nil {
		t.Fatal(err)
	}
	return c, c.Close
}

// docus abre la biblioteca de Docus de la carpeta de datos.
func docus(t *testing.T, datos string) *biblioteca.Biblioteca {
	t.Helper()
	b, err := biblioteca.Abrir(filepath.Join(datos, "Documentos"), "docs", biblioteca.Persona{ID: "instalacion", Nombre: "Yo"})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// calendariosLlamados son los calendarios de aquí con ese nombre.
func calendariosLlamados(t *testing.T, k *calendar.Repository, nombre string) []api.Calendar {
	t.Helper()
	cals, err := k.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return slices.DeleteFunc(cals, func(c api.Calendar) bool { return c.DisplayName != nombre })
}

// eventos son los objetos del único calendario «Casa», por su UID.
func eventos(t *testing.T, datos string) map[string]calendar.ObjetoCrudo {
	t.Helper()
	k, cerrar := agenda(t, datos)
	defer cerrar()
	cals := calendariosLlamados(t, k, "Casa")
	if len(cals) != 1 {
		t.Fatalf("calendarios «Casa»: %+v", cals)
	}
	return objetosDe(t, k, cals[0].ID)
}

// objetosDe son los objetos de un calendario, por su UID.
func objetosDe(t *testing.T, k *calendar.Repository, id string) map[string]calendar.ObjetoCrudo {
	t.Helper()
	objs, err := k.ObjetosCrudos(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]calendar.ObjetoCrudo{}
	for _, o := range objs {
		out[o.UID] = o
	}
	return out
}

// contactosSytue son los contactos de la única libreta «Sytue», por su UID.
func contactosSytue(t *testing.T, datos string) map[string]contacts.TarjetaCruda {
	t.Helper()
	c, cerrar := agendaDe(t, datos)
	defer cerrar()
	_, gente := libretaSytue(t, c)
	return gente
}

// libretaSytue es la única libreta «Sytue» y sus contactos, por su UID.
func libretaSytue(t *testing.T, c contacts.LibretasExternas) (string, map[string]contacts.TarjetaCruda) {
	t.Helper()
	libretas, err := c.AddressBooks(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	libretas = slices.DeleteFunc(libretas, func(l api.AddressBook) bool { return l.DisplayName != "Sytue" })
	if len(libretas) != 1 {
		t.Fatalf("libretas «Sytue»: %+v", libretas)
	}
	ts, err := c.TarjetasCrudas(t.Context(), libretas[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]contacts.TarjetaCruda{}
	for _, x := range ts {
		out[x.UID] = x
	}
	return libretas[0].ID, out
}

// importados son los asuntos de los correos de «Importados».
func importados(t *testing.T, datos string) []string {
	t.Helper()
	b, err := correo.Abrir(filepath.Join(datos, "Correo"), secretos{}, correo.Red{})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Cerrar()
	ms, _, err := b.Messages(t.Context(), correo.CarpetaImportados, "", 100)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.Subject)
	}
	slices.Sort(out)
	return out
}

// docxCon es un .docx con ese HTML, sacado de la biblioteca como lo sacaría el servicio.
func docxCon(t *testing.T, html string) []byte {
	t.Helper()
	b, err := biblioteca.Abrir(t.TempDir(), "docs", biblioteca.Persona{ID: "otro"})
	if err != nil {
		t.Fatal(err)
	}
	d, _, err := b.CreateDocumentFromSource(t.Context(), api.DocText, "x", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.SaveDocumentContent(t.Context(), d.ID, "doc:html", html, api.DocDot{}, 1, false, 0); err != nil {
		t.Fatal(err)
	}
	raw, err := b.ExportDocument(t.Context(), d.ID, "ooxml", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// vevento es un VEVENT de la copia; sello es su DTSTAMP, que Sytue pone al exportar.
func vevento(uid, sello, inicio, resumen string, mas ...string) string {
	return "BEGIN:VEVENT\r\nUID:" + uid + "\r\nDTSTAMP:" + sello + "\r\nDTSTART:" + inicio + "\r\nDURATION:PT1H\r\n" +
		strings.Join(mas, "") + "SUMMARY:" + resumen + "\r\nEND:VEVENT\r\n"
}

func vcalendar(eventos ...string) []byte {
	return []byte("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Sytue//Sytue Calendar//ES\r\n" + strings.Join(eventos, "") + "END:VCALENDAR\r\n")
}

// vcard es una vCard de la copia; uid vacío, sin UID; rev es su REV, que cambia aunque
// no cambie nada.
func vcard(uid, rev, fn string, mas ...string) string {
	t := "BEGIN:VCARD\r\nVERSION:4.0\r\n"
	if uid != "" {
		t += "UID:" + uid + "\r\n"
	}
	return t + "FN:" + fn + "\r\n" + strings.Join(mas, "") + "REV:" + rev + "\r\nEND:VCARD\r\n"
}

const (
	idInigo = "urn:uuid:11111111-1111-4111-8111-111111111111"
	idAna   = "ana@sytue.invalid"
	idDiego = "diego@sytue.invalid"
)

// copiaA es la copia de «Tus datos» del 20 de noviembre: un calendario con un evento, una
// serie con una excepción (RECURRENCE-ID) y otros tres; cuatro contactos, uno sin UID;
// dos documentos; y dos correos, uno sin Message-ID.
func copiaA(t *testing.T) map[string][]byte {
	t.Helper()
	const r, sello = "sytue-one-acme-2026-11-20/", "20261120T100000Z"
	return map[string][]byte{
		r + "calendar/calendars/Casa.ics": vcalendar(
			vevento("visita-1@sytue", sello, "20261201T090000Z", "Visita del arquitecto"),
			vevento("obra-1@sytue", sello, "20261202T080000Z", "Obra", "RRULE:FREQ=WEEKLY;COUNT=4\r\n"),
			vevento("obra-1@sytue", sello, "20261209T100000Z", "Obra (por la tarde)", "RECURRENCE-ID:20261209T080000Z\r\n"),
			vevento("reunion-1@sytue", sello, "20261203T170000Z", "Reunión de vecinos"),
			vevento("pintura-1@sytue", sello, "20261204T090000Z", "Pintar la cocina"),
			vevento("fontanero-1@sytue", sello, "20261205T090000Z", "Fontanero"),
		),
		r + "contacts/contacts.vcf": []byte(
			vcard(idInigo, "20261101T000000Z", "Íñigo Fontanero", "ORG:Reformas Íñigo\r\n") +
				vcard(idAna, "20261101T000000Z", "Ana Pintora", "NOTE:Llamar por la tarde\r\n") +
				vcard("", "20261101T000000Z", "Carmen Sinuid", "EMAIL:carmen@ejemplo.invalid\r\n") +
				vcard(idDiego, "20261101T000000Z", "Diego Electricista")),
		r + "docs/documents/Carta.docx": docxCon(t, "<p>Querida comunidad: la derrama del tejado</p>"),
		r + "docs/documents/Notas.docx": docxCon(t, "<p>Medidas de la cocina</p>"),
		r + "mail/messages/INBOX/1.eml": []byte(eml),
		r + "mail/messages/INBOX/3.eml": []byte("From: Luis <luis@ejemplo.invalid>\r\nTo: yo@ejemplo.invalid\r\nSubject: Sin identificador\r\n" +
			"Date: Fri, 20 Nov 2026 11:00:00 +0000\r\n\r\nUn correo sin Message-ID.\r\n"),
	}
}

// copiaB es la del 27, una semana después: en Sytue han cambiado la visita (aplazada)
// y la excepción de la serie, la pintura (que también cambia aquí), Íñigo, Ana (que
// también cambia aquí) y los dos documentos; hay un evento, un contacto y un correo
// nuevos; lo demás es igual, con otro DTSTAMP y otra REV.
func copiaB(t *testing.T) map[string][]byte {
	t.Helper()
	const r, sello = "sytue-one-acme-2026-11-27/", "20261127T100000Z"
	a := copiaA(t)
	return map[string][]byte{
		r + "calendar/calendars/Casa.ics": vcalendar(
			vevento("visita-1@sytue", sello, "20261211T090000Z", "Visita del arquitecto (aplazada)"),
			vevento("obra-1@sytue", sello, "20261202T080000Z", "Obra", "RRULE:FREQ=WEEKLY;COUNT=4\r\n"),
			vevento("obra-1@sytue", sello, "20261209T120000Z", "Obra (a mediodía)", "RECURRENCE-ID:20261209T080000Z\r\n"),
			vevento("reunion-1@sytue", sello, "20261203T170000Z", "Reunión de vecinos"),
			vevento("pintura-1@sytue", sello, "20261204T090000Z", "Pintar la cocina y el baño"),
			vevento("fontanero-1@sytue", sello, "20261205T090000Z", "Fontanero"),
			vevento("nuevo-1@sytue", sello, "20261206T090000Z", "Revisar la caldera"),
		),
		r + "contacts/contacts.vcf": []byte(
			vcard(idInigo, "20261125T000000Z", "Íñigo Fontanero", "ORG:Reformas Íñigo e Hijos\r\n") +
				vcard(idAna, "20261125T000000Z", "Ana Pintora", "NOTE:Llamar por la tarde\r\n", "TEL:+34600000000\r\n") +
				vcard("", "20261101T000000Z", "Carmen Sinuid", "EMAIL:carmen@ejemplo.invalid\r\n") +
				vcard(idDiego, "20261126T000000Z", "Diego Electricista") +
				vcard("berta@sytue.invalid", "20261126T000000Z", "Berta Cristalera")),
		r + "docs/documents/Carta.docx": docxCon(t, "<p>Querida comunidad: la derrama del tejado sube a 2.600 €</p>"),
		r + "docs/documents/Notas.docx": docxCon(t, "<p>Medidas de la cocina y el baño</p>"),
		r + "mail/messages/INBOX/1.eml": a["sytue-one-acme-2026-11-20/mail/messages/INBOX/1.eml"],
		r + "mail/messages/INBOX/3.eml": a["sytue-one-acme-2026-11-20/mail/messages/INBOX/3.eml"],
		r + "mail/messages/INBOX/2.eml": []byte("From: Ana <ana@ejemplo.invalid>\r\nTo: yo@ejemplo.invalid\r\nSubject: El color de la cocina\r\n" +
			"Date: Wed, 25 Nov 2026 10:00:00 +0000\r\nMessage-ID: <2@ejemplo.invalid>\r\n\r\nAzul.\r\n"),
	}
}

// cuenta comprueba lo que dice el informe de cada producto: importados, repetidos,
// actualizados, conservados y borrados.
func cuenta(t *testing.T, vuelta string, inf traer.Informe, quiero map[string][5]int) {
	t.Helper()
	for producto, q := range quiero {
		p := parte(inf, producto)
		if got := [5]int{p.Importados, p.Repetidos, p.Actualizados, p.Conservados, p.Borrados}; got != q || p.Fallidos != 0 {
			t.Errorf("%s, %s: %+v; quiero importados, repetidos, actualizados, conservados y borrados %v", vuelta, producto, p, q)
		}
	}
}

// HU-07: traer dos veces la misma copia no añade nada a Calendar, Contacts, Docus ni
// Mail; una copia posterior trae lo nuevo, pone al día lo que cambió en Sytue y aquí no
// se tocó, no pisa lo que se cambió aquí (lo de Sytue queda aparte) y no devuelve lo que
// la persona quitó; y una copia más vieja no deshace nada.
func TestTraerOtraVezNoRepiteNiPisa(t *testing.T) {
	ctx := t.Context()
	datos := t.TempDir()
	zipA, zipB := tusDatos(t, copiaA(t)), tusDatos(t, copiaB(t))

	inf, err := traer.Traer(ctx, zipA, destino(datos))
	if err != nil {
		t.Fatal(err)
	}
	cuenta(t, "la primera", inf, map[string][5]int{"calendar": {5, 0, 0, 0, 0}, "contacts": {4, 0, 0, 0, 0}, "docs": {2, 0, 0, 0, 0}, "mail": {2, 0, 0, 0, 0}})
	inf, err = traer.Traer(ctx, zipA, destino(datos))
	if err != nil {
		t.Fatal(err)
	}
	cuenta(t, "la misma otra vez", inf, map[string][5]int{"calendar": {0, 5, 0, 0, 0}, "contacts": {0, 4, 0, 0, 0}, "docs": {0, 2, 0, 0, 0}, "mail": {0, 2, 0, 0, 0}})
	if _, copiados, _ := inf.Total(); copiados != 0 || inf.Carpeta != "" {
		t.Errorf("la misma otra vez deja ficheros: %d en %q", copiados, inf.Carpeta)
	}
	if ev := eventos(t, datos); len(ev) != 5 {
		t.Fatalf("eventos tras traer dos veces: %d", len(ev))
	}

	// Aquí la persona cambia la pintura y a Ana, escribe en las notas y borra al fontanero.
	k, cerrar := agenda(t, datos)
	casa := calendariosLlamados(t, k, "Casa")[0].ID
	ev := objetosDe(t, k, casa)
	pintura := ev["pintura-1@sytue"]
	if _, err := k.PonerObjetoCrudo(ctx, casa, pintura.ID, pintura.Datos, strings.Replace(pintura.Datos, "Pintar la cocina", "Pintar la cocina de azul", 1)); err != nil {
		t.Fatal(err)
	}
	if err := k.BorrarObjetoCrudo(ctx, ev["fontanero-1@sytue"].ID, ""); err != nil {
		t.Fatal(err)
	}
	cerrar()
	c, cerrar := agendaDe(t, datos)
	sytue, gente := libretaSytue(t, c)
	ana := gente[idAna]
	if !strings.Contains(ana.Datos, "Llamar por la tarde") {
		t.Fatalf("Ana: %q", ana.Datos)
	}
	if _, err := c.PonerTarjetaCruda(ctx, sytue, ana.ID, ana.Datos, strings.Replace(ana.Datos, "Llamar por la tarde", "Llamar por la mañana", 1)); err != nil {
		t.Fatal(err)
	}
	cerrar()
	b := docus(t, datos)
	docs, _, _ := b.ListDocuments(ctx, api.DocsFilter{})
	notas := docs[slices.IndexFunc(docs, func(d api.Document) bool { return d.Title == "Notas" })]
	if _, err := b.SaveDocumentContent(ctx, notas.ID, "doc:html", "<p>Medidas de la cocina: 3 × 4</p>", api.DocDot{Counter: 9}, 9, true, notas.Seq); err != nil {
		t.Fatal(err)
	}

	// La copia posterior.
	inf, err = traer.Traer(ctx, zipB, destino(datos))
	if err != nil {
		t.Fatal(err)
	}
	cuenta(t, "la posterior", inf, map[string][5]int{
		"calendar": {1, 1, 2, 1, 1}, // nuevo; reunión; visita y serie; pintura; fontanero
		"contacts": {1, 2, 1, 1, 0}, // Berta; Carmen y Diego; Íñigo; Ana
		"docs":     {0, 0, 1, 1, 0}, // Carta; Notas
		"mail":     {1, 2, 0, 0, 0},
	})
	ev = eventos(t, datos)
	if len(ev) != 5 {
		t.Errorf("eventos: %d", len(ev))
	}
	for uid, quiero := range map[string]string{
		"visita-1@sytue": "aplazada", "obra-1@sytue": "Obra (a mediodía)", "pintura-1@sytue": "Pintar la cocina de azul",
		"nuevo-1@sytue": "Revisar la caldera",
	} {
		if !strings.Contains(ev[uid].Datos, quiero) {
			t.Errorf("%s no dice %q:\n%s", uid, quiero, ev[uid].Datos)
		}
	}
	if _, ok := ev["fontanero-1@sytue"]; ok || strings.Contains(ev["pintura-1@sytue"].Datos, "baño") {
		t.Error("vuelve lo borrado aquí o se pisa lo cambiado aquí")
	}
	gente = contactosSytue(t, datos)
	if len(gente) != 5 || !strings.Contains(gente[idAna].Datos, "Llamar por la mañana") || strings.Contains(gente[idAna].Datos, "600000000") ||
		!strings.Contains(gente[sinURN(idInigo)].Datos, "Hijos") || gente["berta@sytue.invalid"].ID == "" {
		t.Errorf("contactos: %+v", gente)
	}
	b = docus(t, datos)
	docs, _, _ = b.ListDocuments(ctx, api.DocsFilter{})
	titulos := make([]string, 0, len(docs))
	for _, d := range docs {
		titulos = append(titulos, d.Title)
	}
	slices.Sort(titulos)
	if !slices.Equal(titulos, []string{"Carta", "Notas", "Notas (de Sytue, 2026-11-27)"}) {
		t.Fatalf("documentos: %v", titulos)
	}
	for titulo, quiero := range map[string]string{"Carta": "2.600", "Notas": "3 × 4", "Notas (de Sytue, 2026-11-27)": "y el baño"} {
		d := docs[slices.IndexFunc(docs, func(d api.Document) bool { return d.Title == titulo })]
		cont, err := b.DocumentContent(ctx, d.ID, "doc:html")
		if err != nil || !strings.Contains(cont.Value, quiero) {
			t.Errorf("%s no dice %q: %q %v", titulo, quiero, cont.Value, err)
		}
		if titulo == "Carta" {
			vs, _ := b.DocumentVersions(ctx, d.ID)
			if len(vs) != 2 || !slices.ContainsFunc(vs, func(v api.DocVersion) bool { return v.Label == "De Sytue (2026-11-27)" }) {
				t.Errorf("la carta no ha entrado como versión nueva: %+v", vs)
			}
		}
	}
	if got := importados(t, datos); !slices.Equal(got, []string{"El color de la cocina", "Presupuesto del tejado", "Sin identificador"}) {
		t.Errorf("Importados: %v", got)
	}
	// Lo de Sytue que no ha entrado por estar cambiado aquí, tal cual en la carpeta.
	carpeta := filepath.Join(datos, "Drive", "Traído de Sytue 2026-11-24")
	for _, f := range []string{"calendar/calendars/Casa.ics", "contacts/contacts.vcf"} {
		if raw, err := os.ReadFile(filepath.Join(carpeta, filepath.FromSlash(f))); err != nil || !strings.Contains(string(raw), "baño") && !strings.Contains(string(raw), "600000000") {
			t.Errorf("no se ha dejado %s con lo de Sytue: %v", f, err)
		}
	}

	// La posterior otra vez: ya está todo.
	inf, err = traer.Traer(ctx, zipB, destino(datos))
	if err != nil {
		t.Fatal(err)
	}
	cuenta(t, "la posterior otra vez", inf, map[string][5]int{"calendar": {0, 5, 0, 0, 1}, "contacts": {0, 5, 0, 0, 0}, "docs": {0, 2, 0, 0, 0}, "mail": {0, 3, 0, 0, 0}})

	// La vieja otra vez: no deshace lo nuevo.
	inf, err = traer.Traer(ctx, zipA, destino(datos))
	if err != nil {
		t.Fatal(err)
	}
	cuenta(t, "la vieja después", inf, map[string][5]int{"calendar": {0, 4, 0, 0, 1}, "contacts": {0, 4, 0, 0, 0}, "docs": {0, 2, 0, 0, 0}, "mail": {0, 2, 0, 0, 0}})
	if ev := eventos(t, datos); !strings.Contains(ev["visita-1@sytue"].Datos, "aplazada") {
		t.Error("la copia vieja deshace la visita aplazada")
	}
}

// sinURN es el UID de una vCard como lo guarda Contacts (sin `urn:uuid:`).
func sinURN(uid string) string { return strings.TrimPrefix(uid, "urn:uuid:") }

// HU-07: sin la memoria de lo traído (se borró, o se trajo con una versión que no la
// llevaba), lo que ya está tampoco se repite: el calendario por su nombre interno o el
// de antes («sytue-<uuid>» con el nombre del fichero) y sus eventos por UID, la libreta
// por su nombre y sus contactos por UID, el documento por su origen y el correo por su
// Message-ID.
func TestSinMemoriaTampocoSeRepite(t *testing.T) {
	ctx := t.Context()
	datos := t.TempDir()
	zipA := tusDatos(t, copiaA(t))
	if _, err := traer.Traer(ctx, zipA, destino(datos)); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(datos, "Drive", ".traido-de-sytue.json")); err != nil {
		t.Fatal(err)
	}
	inf, err := traer.Traer(ctx, zipA, destino(datos))
	if err != nil {
		t.Fatal(err)
	}
	cuenta(t, "sin memoria", inf, map[string][5]int{"calendar": {0, 5, 0, 0, 0}, "contacts": {0, 4, 0, 0, 0}, "docs": {0, 2, 0, 0, 0}, "mail": {0, 2, 0, 0, 0}})
	if len(eventos(t, datos)) != 5 || len(contactosSytue(t, datos)) != 4 {
		t.Error("sin memoria se repite")
	}

	// Un calendario traído antes de apuntarlo: «sytue-<uuid>», con el nombre del fichero.
	otros := t.TempDir()
	k, cerrar := agenda(t, otros)
	cal, err := k.CreateCalendar(ctx, api.CalendarInput{Name: "sytue-" + uuid.NewString(), DisplayName: "Casa"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := k.ImportICS(ctx, cal.ID, copiaA(t)["sytue-one-acme-2026-11-20/calendar/calendars/Casa.ics"]); err != nil {
		t.Fatal(err)
	}
	cerrar()
	inf, err = traer.Traer(ctx, zipA, destino(otros))
	if err != nil {
		t.Fatal(err)
	}
	cuenta(t, "el calendario de antes", inf, map[string][5]int{"calendar": {0, 5, 0, 0, 0}})
	if len(eventos(t, otros)) != 5 {
		t.Error("el calendario de antes")
	}
	// Y lo que se reconoce sin memoria se apunta: si luego cambia en Sytue, se pone al día.
	inf, err = traer.Traer(ctx, tusDatos(t, copiaB(t)), destino(otros))
	if err != nil {
		t.Fatal(err)
	}
	cuenta(t, "el calendario de antes, con la posterior", inf, map[string][5]int{"calendar": {1, 2, 3, 0, 0}})
}

// HU-07: si la persona borró el calendario o la libreta enteros, traer otra vez los crea
// de nuevo con todo, como un cuaderno de Notepads.
func TestUnCalendarioBorradoSeCreaOtraVez(t *testing.T) {
	ctx := t.Context()
	datos := t.TempDir()
	zipA := tusDatos(t, copiaA(t))
	// La libreta de la persona (la primera es la de siempre, que no se puede borrar).
	c, cerrar := agendaDe(t, datos)
	if _, err := c.CreateAddressBook(ctx, "Personal"); err != nil {
		t.Fatal(err)
	}
	cerrar()
	if _, err := traer.Traer(ctx, zipA, destino(datos)); err != nil {
		t.Fatal(err)
	}
	k, cerrar := agenda(t, datos)
	if err := k.DeleteCalendar(ctx, calendariosLlamados(t, k, "Casa")[0].ID); err != nil {
		t.Fatal(err)
	}
	cerrar()
	c, cerrar = agendaDe(t, datos)
	libretas, _ := c.AddressBooks(ctx)
	if err := c.DeleteAddressBook(ctx, libretas[slices.IndexFunc(libretas, func(l api.AddressBook) bool { return l.DisplayName == "Sytue" })].ID); err != nil {
		t.Fatal(err)
	}
	cerrar()
	inf, err := traer.Traer(ctx, zipA, destino(datos))
	if err != nil {
		t.Fatal(err)
	}
	cuenta(t, "con el calendario y la libreta borrados", inf, map[string][5]int{"calendar": {5, 0, 0, 0, 0}, "contacts": {4, 0, 0, 0, 0}})
	if len(eventos(t, datos)) != 5 || len(contactosSytue(t, datos)) != 4 {
		t.Error("no se han creado otra vez")
	}
}

// HU-07: una copia con los contactos en CSV (la tabla de Google Contacts) tampoco se
// repite: cada fila se casa por su clave, el correo o, sin él, el nombre.
func TestContactosEnCSVNoSeRepiten(t *testing.T) {
	ctx := t.Context()
	datos := t.TempDir()
	const cabecera = "\xef\xbb\xbfName,Given Name,Family Name,E-mail 1 - Type,E-mail 1 - Value,Phone 1 - Type,Phone 1 - Value\r\n"
	csv := func(raiz string, filas ...string) string {
		return tusDatos(t, map[string][]byte{raiz + "/contacts/contacts.csv": []byte(cabecera + strings.Join(filas, ""))})
	}
	primera := csv("sytue-one-2026-11-20",
		"Ana Pintora,Ana,Pintora,* work,ana@ejemplo.invalid,,\r\n",
		"Luis Sincorreo,Luis,Sincorreo,,,mobile,+34600000001\r\n")
	for vuelta, quiero := range [][5]int{{2, 0, 0, 0, 0}, {0, 2, 0, 0, 0}} {
		inf, err := traer.Traer(ctx, primera, destino(datos))
		if err != nil {
			t.Fatal(err)
		}
		cuenta(t, []string{"la primera", "otra vez"}[vuelta], inf, map[string][5]int{"contacts": quiero})
	}
	inf, err := traer.Traer(ctx, csv("sytue-one-2026-11-27",
		"Ana Pintora,Ana,Pintora,* work,ana@ejemplo.invalid,mobile,+34600000002\r\n",
		"Luis Sincorreo,Luis,Sincorreo,,,mobile,+34600000001\r\n",
		"Marta Nueva,Marta,Nueva,* home,marta@ejemplo.invalid,,\r\n"), destino(datos))
	if err != nil {
		t.Fatal(err)
	}
	cuenta(t, "la posterior", inf, map[string][5]int{"contacts": {1, 1, 1, 0, 0}})
	if gente := contactosSytue(t, datos); len(gente) != 3 {
		t.Errorf("contactos: %+v", gente)
	}
}
