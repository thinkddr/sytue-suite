// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package agenda_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	privateCalendar "git.sytue.eu/sytue/one/apps/nucleo/calendar"
	"git.sytue.eu/sytue/one/apps/nucleo/cuentas/dav"
	"git.sytue.eu/sytue/one/apps/nucleo/cuentas/dav/davprueba"
	"git.sytue.eu/sytue/one/apps/nucleo/local/agenda"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
	"git.sytue.eu/sytue/one/apps/nucleo/local/libreta"
)

func abrirCalendario(t *testing.T, dir string) *privateCalendar.Repository {
	t.Helper()
	rec, err := almacen.Abrir(context.Background(), dir, "calendar", "instalacion-1")
	if err != nil {
		t.Fatal(err)
	}
	repo, err := privateCalendar.New(rec.Config)
	if err != nil {
		t.Fatal(err)
	}
	if err := almacen.Preparar(t.Context(), repo); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { repo.Close(); rec.Cerrar() })
	return repo
}

// abrirContactos abre la libreta de Contacts de Sytue Office en dir: la SQLite de
// `local/libreta`, la que usa `cmd-local/contacts`.
func abrirContactos(t *testing.T, dir string) *libreta.Libreta {
	t.Helper()
	repo, err := libreta.Abrir(t.Context(), dir, "instalacion-1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(repo.Close)
	return repo
}

func ics(uid, resumen string, inicio time.Time, extra string) string {
	return "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//prueba//ES\r\nBEGIN:VEVENT\r\nUID:" + uid +
		"\r\nDTSTAMP:20260101T000000Z\r\nDTSTART:" + inicio.UTC().Format("20060102T150405Z") +
		"\r\nDTEND:" + inicio.Add(time.Hour).UTC().Format("20060102T150405Z") + "\r\nSUMMARY:" + resumen + "\r\n" + extra +
		"END:VEVENT\r\nEND:VCALENDAR\r\n"
}

func vcf(uid, nombre string) string {
	return "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:" + uid + "\r\nFN:" + nombre + "\r\nN:" + nombre + ";;;;\r\nEMAIL:" + strings.ToLower(nombre) + "@example.org\r\nEND:VCARD\r\n"
}

type montaje struct {
	srv  *davprueba.Servidor
	repo *privateCalendar.Repository
	s    *agenda.Sincronizador
	col  string // ruta de la colección en el servidor
	cal  string // id del calendario de aquí
	t    *testing.T
}

func montarCalendario(t *testing.T, sinSync bool) *montaje {
	t.Helper()
	srv := davprueba.Nuevo()
	srv.SinSync = sinSync
	t.Cleanup(srv.Close)
	col := davprueba.CasaCal + "trabajo/"
	srv.Crear(col, "Trabajo", false)
	srv.Crear(davprueba.CasaCal+"festivos/", "Festivos", true)
	dir := t.TempDir()
	repo := abrirCalendario(t, dir)
	cli, err := dav.Nuevo(dav.Cuenta{Direccion: srv.URL + "/cal/", Usuario: srv.Usuario, Clave: srv.Clave}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	s := &agenda.Sincronizador{
		Remoto:     &agenda.RemotoDAV{Cliente: cli, Clase: dav.Calendario},
		Local:      agenda.LocalCalendario{Repo: repo},
		Estado:     filepath.Join(dir, "agenda", "cuenta.json"),
		Ventana:    agenda.VentanaCalendario,
		MarcaCopia: "(copia en conflicto)",
	}
	return &montaje{srv: srv, repo: repo, s: s, col: col, t: t}
}

func (m *montaje) sincronizar() agenda.Resultado {
	m.t.Helper()
	res, err := m.s.Sincronizar(m.t.Context())
	if err != nil {
		m.t.Fatal(err)
	}
	if len(res.Errores) > 0 {
		m.t.Fatalf("errores: %v", res.Errores)
	}
	if m.cal == "" {
		for _, c := range m.s.Colecciones() {
			if c.Nombre == "Trabajo" {
				m.cal = c.ID
			}
		}
	}
	return res
}

func (m *montaje) locales() map[string]privateCalendar.ObjetoCrudo {
	m.t.Helper()
	os, err := m.repo.ObjetosCrudos(m.t.Context(), m.cal)
	if err != nil {
		m.t.Fatal(err)
	}
	out := map[string]privateCalendar.ObjetoCrudo{}
	for _, o := range os {
		out[o.UID] = o
	}
	return out
}

func probarCalendario(t *testing.T, sinSync bool) {
	m := montarCalendario(t, sinSync)
	ahora := time.Now()
	m.srv.Poner(m.col+"a.ics", []byte(ics("a", "Reunión", ahora.AddDate(0, 0, 2), "")))
	m.srv.Poner(m.col+"viejo.ics", []byte(ics("viejo", "Hace dos años", ahora.AddDate(-2, 0, 0), "")))
	m.srv.Poner(davprueba.CasaCal+"festivos/navidad.ics", []byte(ics("navidad", "Navidad", ahora.AddDate(0, 1, 0), "")))

	// Primera vez: entran los dos calendarios, el de festivos de solo lectura, y del de
	// trabajo solo lo que está dentro de la ventana (con sync-collection).
	res := m.sincronizar()
	cols := m.s.Colecciones()
	if len(cols) != 2 || cols[0].Nombre != "Festivos" || !cols[0].SoloLectura || cols[1].SoloLectura {
		t.Fatalf("colecciones: %+v", cols)
	}
	l := m.locales()
	if _, ok := l["a"]; !ok {
		t.Fatalf("no bajó el evento: %+v (%+v)", l, res)
	}
	if _, ok := l["viejo"]; ok && !sinSync {
		t.Fatal("bajó un evento de fuera de la ventana")
	}
	cals, _ := m.repo.List(t.Context())
	for _, c := range cals {
		if c.DisplayName == "Festivos" && !c.ReadOnly {
			t.Fatal("el de solo lectura se puede editar aquí")
		}
	}

	// Se crea uno aquí: sube con su UID.
	if _, err := m.repo.PonerObjetoCrudo(t.Context(), m.cal, "", "", ics("local-1", "Dentista", ahora.AddDate(0, 0, 3), "")); err != nil {
		t.Fatal(err)
	}
	if res := m.sincronizar(); res.Subidos != 1 {
		t.Fatalf("subida: %+v", res)
	}
	if d := m.srv.Datos(m.col + "local-1.ics"); !strings.Contains(string(d), "Dentista") {
		t.Fatalf("no está en el servidor: %q", d)
	}
	// Una segunda vuelta sin cambios no sube ni baja nada.
	if res := m.sincronizar(); res.Subidos+res.Bajados+res.Borrados != 0 {
		t.Fatalf("vuelta en vacío: %+v", res)
	}

	// Cambia en el servidor lo que aquí no se tocó: el servidor manda.
	m.srv.Poner(m.col+"a.ics", []byte(ics("a", "Reunión movida", ahora.AddDate(0, 0, 4), "")))
	m.sincronizar()
	if l := m.locales(); !strings.Contains(l["a"].Datos, "Reunión movida") {
		t.Fatalf("no bajó el cambio: %q", l["a"].Datos)
	}

	// Conflicto: cambia en los dos lados. Lo del servidor queda en su sitio y lo de aquí
	// en una copia que sube como evento nuevo.
	l = m.locales()
	if _, err := m.repo.PonerObjetoCrudo(t.Context(), m.cal, l["a"].ID, "", ics("a", "Reunión editada aquí", ahora.AddDate(0, 0, 4), "")); err != nil {
		t.Fatal(err)
	}
	m.srv.Poner(m.col+"a.ics", []byte(ics("a", "Reunión editada allí", ahora.AddDate(0, 0, 4), "")))
	res = m.sincronizar()
	if len(res.Conflictos) != 1 {
		t.Fatalf("conflicto: %+v", res)
	}
	l = m.locales()
	if !strings.Contains(l["a"].Datos, "editada allí") {
		t.Fatalf("el del servidor no quedó: %q", l["a"].Datos)
	}
	copia := false
	for uid, o := range l {
		if uid != "a" && strings.Contains(o.Datos, "(copia en conflicto) Reunión editada aquí") {
			copia = true
			if d := m.srv.Datos(m.col + uid + ".ics"); d == nil {
				t.Fatal("la copia no subió")
			}
		}
	}
	if !copia {
		t.Fatalf("se perdió lo escrito aquí: %+v", l)
	}

	// Borrado aquí: se borra en el servidor. Borrado allí: se borra aquí.
	if err := m.repo.BorrarObjetoCrudo(t.Context(), l["local-1"].ID, ""); err != nil {
		t.Fatal(err)
	}
	m.srv.Quitar(m.col + "a.ics")
	m.sincronizar()
	if m.srv.Datos(m.col+"local-1.ics") != nil {
		t.Fatal("no se borró en el servidor")
	}
	if _, ok := m.locales()["a"]; ok {
		t.Fatal("no se borró aquí")
	}

	// Lo de solo lectura no sube aunque cambie aquí (la pantalla no deja, pero el
	// repositorio sí al sincronizador).
	var fest string
	for _, c := range m.s.Colecciones() {
		if c.Nombre == "Festivos" {
			fest = c.ID
		}
	}
	os, _ := m.repo.ObjetosCrudos(t.Context(), fest)
	if len(os) != 1 {
		t.Fatalf("festivos: %+v", os)
	}
	if _, err := m.repo.CreateEvent(t.Context(), fest, api.EventInput{Summary: "x", Start: ahora, End: ahora.Add(time.Hour)}); err == nil {
		t.Fatal("la pantalla pudo escribir en un calendario de solo lectura")
	}
}

func TestSincronizaCalendarioConSyncCollection(t *testing.T) { probarCalendario(t, false) }
func TestSincronizaCalendarioPorETag(t *testing.T)           { probarCalendario(t, true) }

func TestPendientesConLaRedCortada(t *testing.T) {
	m := montarCalendario(t, false)
	reloj := time.Now()
	m.s.Ahora = func() time.Time { return reloj }
	m.sincronizar()
	if _, err := m.repo.PonerObjetoCrudo(t.Context(), m.cal, "", "", ics("sin-red", "Sin red", reloj, "")); err != nil {
		t.Fatal(err)
	}
	// Con el servidor caído, la cuenta entera falla y nada se pierde aquí.
	m.srv.Caido = true
	if _, err := m.s.Sincronizar(t.Context()); err == nil {
		t.Fatal("con el servidor caído no falló")
	}
	if _, ok := m.locales()["sin-red"]; !ok {
		t.Fatal("se perdió lo escrito sin red")
	}
	// Vuelve la red: sube.
	m.srv.Caido = false
	if res := m.sincronizar(); res.Subidos != 1 {
		t.Fatalf("al volver la red: %+v", res)
	}

	// Una subida que falla en el servidor queda pendiente con espera creciente.
	if _, err := m.repo.PonerObjetoCrudo(t.Context(), m.cal, "", "", ics("roto", "Roto", reloj, "")); err != nil {
		t.Fatal(err)
	}
	cli, _ := dav.Nuevo(dav.Cuenta{Direccion: m.srv.URL + "/cal/", Usuario: m.srv.Usuario, Clave: "mala"}, m.srv.Client())
	bueno := m.s.Remoto
	m.s.Remoto = &soloSubidaMala{Remoto: bueno, malo: &agenda.RemotoDAV{Cliente: cli, Clase: dav.Calendario}}
	res, err := m.s.Sincronizar(t.Context())
	if err != nil || res.Pendientes != 1 {
		t.Fatalf("pendiente: %+v %v", res, err)
	}
	// Antes de la espera no se reintenta; después, sí.
	m.s.Remoto = bueno
	if res := m.sincronizar(); res.Pendientes != 1 || res.Subidos != 0 {
		t.Fatalf("antes de la espera: %+v", res)
	}
	reloj = reloj.Add(agenda.EsperaMinima + time.Second)
	if res := m.sincronizar(); res.Subidos != 1 || res.Pendientes != 0 {
		t.Fatalf("tras la espera: %+v", res)
	}
}

// soloSubidaMala lee del bueno y sube con el malo (una contraseña que el servidor
// rechaza al escribir).
type soloSubidaMala struct {
	agenda.Remoto
	malo agenda.Remoto
}

func (s *soloSubidaMala) Crear(ctx context.Context, col agenda.Coleccion, uid string, datos []byte) (agenda.Objeto, error) {
	return s.malo.Crear(ctx, col, uid, datos)
}

func TestRecurrenciaConExcepcionesIdaYVuelta(t *testing.T) {
	m := montarCalendario(t, false)
	inicio := time.Now().Add(24 * time.Hour).UTC().Truncate(time.Hour)
	excepcion := inicio.AddDate(0, 0, 7)
	datos := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//prueba//ES\r\n" +
		"BEGIN:VEVENT\r\nUID:serie\r\nDTSTAMP:20260101T000000Z\r\nDTSTART:" + inicio.Format("20060102T150405Z") + "\r\nDTEND:" + inicio.Add(time.Hour).Format("20060102T150405Z") +
		"\r\nSUMMARY:Semanal\r\nRRULE:FREQ=WEEKLY;COUNT=5\r\nEXDATE:" + inicio.AddDate(0, 0, 14).Format("20060102T150405Z") + "\r\nX-PROPIA:se conserva\r\nEND:VEVENT\r\n" +
		"BEGIN:VEVENT\r\nUID:serie\r\nDTSTAMP:20260101T000000Z\r\nRECURRENCE-ID:" + excepcion.Format("20060102T150405Z") + "\r\nDTSTART:" + excepcion.Add(2*time.Hour).Format("20060102T150405Z") +
		"\r\nDTEND:" + excepcion.Add(3*time.Hour).Format("20060102T150405Z") + "\r\nSUMMARY:Semanal (movida)\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	m.srv.Poner(m.col+"serie.ics", []byte(datos))
	m.sincronizar()
	inst, err := m.repo.Instances(t.Context(), m.cal, inicio.Add(-time.Hour), inicio.AddDate(0, 0, 40))
	if err != nil {
		t.Fatal(err)
	}
	if len(inst) != 4 {
		t.Fatalf("ocurrencias (5 menos la quitada): %d", len(inst))
	}
	movida := false
	for _, i := range inst {
		if i.Start.Equal(excepcion.Add(2 * time.Hour)) {
			movida = true
		}
	}
	if !movida {
		t.Fatal("la excepción no se expandió en su hora")
	}
	// Se cambia aquí el título de la serie y vuelve al servidor con su excepción, su
	// EXDATE y la propiedad que la app no conoce.
	l := m.locales()
	nuevo := strings.Replace(l["serie"].Datos, "SUMMARY:Semanal\r\n", "SUMMARY:Semanal de equipo\r\n", 1)
	if nuevo == l["serie"].Datos {
		t.Fatalf("no se encontró el título en %q", l["serie"].Datos)
	}
	if _, err := m.repo.PonerObjetoCrudo(t.Context(), m.cal, l["serie"].ID, "", nuevo); err != nil {
		t.Fatal(err)
	}
	m.sincronizar()
	d := string(m.srv.Datos(m.col + "serie.ics"))
	for _, debe := range []string{"Semanal de equipo", "RECURRENCE-ID", "Semanal (movida)", "EXDATE", "X-PROPIA"} {
		if !strings.Contains(d, debe) {
			t.Fatalf("falta %q en lo subido:\n%s", debe, d)
		}
	}
}

func TestSincronizaContactos(t *testing.T) {
	srv := davprueba.Nuevo()
	t.Cleanup(srv.Close)
	col := davprueba.CasaCard + "libreta/"
	srv.Crear(col, "Mis contactos", false)
	srv.Poner(col+"ana.vcf", []byte(vcf("ana", "Ana")))
	dir := t.TempDir()
	repo := abrirContactos(t, dir)
	cli, _ := dav.Nuevo(dav.Cuenta{Direccion: srv.URL + "/card/", Usuario: srv.Usuario, Clave: srv.Clave}, srv.Client())
	s := &agenda.Sincronizador{
		Remoto: &agenda.RemotoDAV{Cliente: cli, Clase: dav.Libreta}, Local: agenda.LocalContactos{Repo: repo},
		Estado: filepath.Join(dir, "agenda", "c.json"), MarcaCopia: "(copia en conflicto)",
	}
	sinc := func() agenda.Resultado {
		t.Helper()
		res, err := s.Sincronizar(t.Context())
		if err != nil || len(res.Errores) > 0 {
			t.Fatalf("%v %v", err, res.Errores)
		}
		return res
	}
	sinc()
	cols := s.Colecciones()
	if len(cols) != 1 {
		t.Fatalf("libretas: %+v", cols)
	}
	libro := cols[0].ID
	ts, _ := repo.TarjetasCrudas(t.Context(), libro)
	if len(ts) != 1 || ts[0].UID != "ana" {
		t.Fatalf("bajada: %+v", ts)
	}
	// Uno nuevo aquí sube; Ana cambiada aquí sube con If-Match.
	if _, err := repo.Create(t.Context(), libro, api.ContactInput{FormattedName: "Bea", GivenName: "Bea"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Update(t.Context(), ts[0].ID, api.ContactInput{FormattedName: "Ana María", GivenName: "Ana María"}, ""); err != nil {
		t.Fatal(err)
	}
	if res := sinc(); res.Subidos != 2 {
		t.Fatalf("subida: %+v", res)
	}
	if !strings.Contains(string(srv.Datos(col+"ana.vcf")), "Ana María") || len(srv.Rutas(col)) != 2 {
		t.Fatalf("servidor: %v %q", srv.Rutas(col), srv.Datos(col+"ana.vcf"))
	}
	// Borrado en el servidor: aquí a la papelera.
	srv.Quitar(col + "ana.vcf")
	sinc()
	ts, _ = repo.TarjetasCrudas(t.Context(), libro)
	if len(ts) != 1 {
		t.Fatalf("tras borrar allí: %+v", ts)
	}
}
