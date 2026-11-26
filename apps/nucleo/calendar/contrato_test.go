// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendar_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/calendar"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/storagetest"
)

// # El contrato de Calendar (PLAN-EDICION-LOCAL §8.2)
//
// El MISMO juego de pruebas de comportamiento del repositorio de Calendar —lo que la
// pantalla usa por `api.Calendars`, y lo que Sytue Office le pide aparte (importar,
// los objetos crudos de las cuentas)— sobre los dos puertos del motor de registros: el
// de las pruebas en memoria (`storagetest.Memoria`, el publicador de la conectada sin
// red) y el almacén en claro de Sytue Office en disco (`local/almacen`, como lo abre
// `cmd-local/calendar`, con sus tres opciones). Cada prueba corre en los dos
// (porCadaPuerto); TestContratoDeCalendarVenLoMismo hace lo mismo en los dos y compara
// lo que enseña la pantalla.
//
// Diferencias aceptadas entre los dos puertos: solo el disco puede dejar a medias la
// última línea de su diario (puertoDeCalendar.lineaCortada); en memoria el corte es la
// respuesta perdida de una publicación, que prueban los dos.
//
// Lo que el contrato destapó del almacén en disco, y ya no pasa (los dos en
// `local/almacen`): dos procesos sobre el mismo almacén no veían lo que escribía el
// otro hasta reabrirlo, así que el segundo que cambiaba algo que el primero acababa de
// cambiar lo hacía sobre lo de antes, el objeto quedaba con dos cabezas y Calendar ya
// no abría («private revisions require resolution»): ahora el puerto lee lo que otro
// añadió a su diario antes de contestar y de escribir (`Puerto.seguir`), como la sala de
// la conectada, y gana el último. Y una escritura cortada a mitad (la última línea sin
// acabar) se pegaba a la siguiente y el diario no volvía a abrir («está dañado»): ahora
// esa línea se lee sin el trozo cortado (`leerLinea`).
//
// VISTO EN ROJO (2026-11-25): esperando que la excepción del 25 se quedara con el
// título viejo, y abriendo sin ConservarForma, TestContratoDeCalendarSeries falló en
// los dos puertos con el mismo mensaje; y, sin los dos arreglos del almacén,
// TestContratoDeCalendarDosProcesos y TestContratoDeCalendarCorteAMitad fallaron solo
// en disco.

const (
	sujetoDeCalendar  = "instalacion-1"
	correoDeLaPersona = "yo@ejemplo.org"
)

var madridDelContrato = func() *time.Location {
	l, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		panic(err)
	}
	return l
}()

// almacenDeCalendar es donde viven los calendarios en uno de los dos puertos.
type almacenDeCalendar struct {
	// dir es la carpeta del almacén en disco; "" en memoria.
	dir string
	// abrir es un proceso sobre el almacén: el repositorio de Calendar con su diario
	// transitorio. Abrir otra vez el mismo proceso es ese proceso al volver a arrancar.
	abrir func(t *testing.T, proceso string, o opcionesDeApertura) (*calendar.Repository, func())
}

// opcionesDeApertura son lo que cambia al abrir un repositorio en una prueba.
type opcionesDeApertura struct {
	// comoLaConectada: sin las opciones de Sytue Office (ConservarRespuestas,
	// ConservarForma, SerieALasExcepciones), como lo usa la edición conectada.
	comoLaConectada bool
	// corte, si no es nil, pierde la respuesta de la próxima publicación de un índice
	// cuando se active: la escritura queda hecha pero quien la pidió ve un fallo.
	corte *atomic.Bool
}

type puertoDeCalendar struct {
	nombre string
	nuevo  func(t *testing.T) *almacenDeCalendar
	// lineaCortada deja a medias la última escritura del almacén, como un corte de luz a
	// mitad de escribir; nil si el puerto no lo puede simular.
	lineaCortada func(t *testing.T, a *almacenDeCalendar)
}

var errCorteDeLuz = errors.New("prueba: la escritura se hizo pero la respuesta se perdió")

// discoConCorte es el almacén en disco con el corte de opcionesDeApertura.
type discoConCorte struct {
	*almacen.Puerto
	corte *atomic.Bool
}

func (d *discoConCorte) PutStorageObjectAt(ctx context.Context, t storage.PublicationTarget, txn, objeto, revision string, commit storage.ObjectCommit) (storage.ObjectVersion, error) {
	v, err := d.Puerto.PutStorageObjectAt(ctx, t, txn, objeto, revision, commit)
	if err == nil && d.corte.CompareAndSwap(true, false) {
		return storage.ObjectVersion{}, errCorteDeLuz
	}
	return v, err
}

// memoriaConCorte es la memoria con el corte de opcionesDeApertura.
type memoriaConCorte struct {
	*storagetest.Memoria
	corte *atomic.Bool
}

func (m *memoriaConCorte) PutStorageObjectAt(ctx context.Context, t storage.PublicationTarget, txn, objeto, revision string, commit storage.ObjectCommit) (storage.ObjectVersion, error) {
	v, err := m.Memoria.PutStorageObjectAt(ctx, t, txn, objeto, revision, commit)
	if err == nil && m.corte.CompareAndSwap(true, false) {
		return storage.ObjectVersion{}, errCorteDeLuz
	}
	return v, err
}

// repositorioDeCalendar es el repositorio sobre cfg, como lo abre cmd-local/calendar (o
// como la conectada), preparado.
func repositorioDeCalendar(t *testing.T, cfg records.Config, o opcionesDeApertura) *calendar.Repository {
	t.Helper()
	repo, err := calendar.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !o.comoLaConectada {
		repo.ConservarRespuestas, repo.ConservarForma, repo.SerieALasExcepciones = true, true, true
	}
	repo.Correo = correoDeLaPersona
	if err := almacen.Preparar(t.Context(), repo); err != nil {
		repo.Close()
		t.Fatal("preparar:", err)
	}
	return repo
}

var puertosDeCalendar = []puertoDeCalendar{
	{nombre: "en memoria", nuevo: func(t *testing.T) *almacenDeCalendar {
		scope := storage.Scope{Tenant: "local", Subject: sujetoDeCalendar, Product: "calendar"}
		sala := storagetest.NuevaMemoria(scope)
		origen := storagetest.NuevoOrigenVacio(scope)
		type diario struct {
			dir      string
			secretos *authtest.Secrets
		}
		diarios := map[string]diario{}
		return &almacenDeCalendar{abrir: func(t *testing.T, proceso string, o opcionesDeApertura) (*calendar.Repository, func()) {
			t.Helper()
			d, ok := diarios[proceso]
			if !ok {
				d = diario{dir: t.TempDir(), secretos: &authtest.Secrets{}}
				diarios[proceso] = d
			}
			journal, err := offline.OpenWithSecrets(d.dir, d.secretos)
			if err != nil {
				t.Fatal(err)
			}
			var crypto records.Crypto = sala
			if o.corte != nil {
				crypto = &memoriaConCorte{Memoria: sala, corte: o.corte}
			}
			repo := repositorioDeCalendar(t, records.Config{
				Context: context.Background(), Scope: scope, Crypto: crypto,
				Gateway: api.NewDrive("https://sin-red.invalid", nil, nil), Journal: journal, Source: origen,
			}, o)
			return repo, repo.Close
		}}
	}},
	{nombre: "en disco", nuevo: func(t *testing.T) *almacenDeCalendar {
		dir := t.TempDir()
		return &almacenDeCalendar{dir: dir, abrir: func(t *testing.T, _ string, o opcionesDeApertura) (*calendar.Repository, func()) {
			t.Helper()
			// Cada proceso abre la carpeta entera, con su diario transitorio dentro: dos
			// procesos sobre el mismo almacén comparten ese diario, como en el equipo.
			rec, err := almacen.Abrir(context.Background(), dir, "calendar", sujetoDeCalendar)
			if err != nil {
				t.Fatal(err)
			}
			cfg := rec.Config
			if o.corte != nil {
				cfg.Crypto = &discoConCorte{Puerto: rec.Puerto, corte: o.corte}
			}
			repo := repositorioDeCalendar(t, cfg, o)
			return repo, func() { repo.Close(); rec.Cerrar() }
		}}
	}, lineaCortada: func(t *testing.T, a *almacenDeCalendar) {
		t.Helper()
		f, err := os.OpenFile(filepath.Join(a.dir, "registros.jsonl"), os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		_, err = f.WriteString(`{"t":"registro","ref":{"room_id":"!local:calendar","event_id":"$cortad`)
		if err = errors.Join(err, f.Close()); err != nil {
			t.Fatal(err)
		}
	}},
}

// porCadaPuerto corre prueba en cada puerto, con un almacén nuevo.
func porCadaPuerto(t *testing.T, prueba func(t *testing.T, p puertoDeCalendar, a *almacenDeCalendar)) {
	t.Helper()
	for _, p := range puertosDeCalendar {
		t.Run(p.nombre, func(t *testing.T) { prueba(t, p, p.nuevo(t)) })
	}
}

// fachada es lo que usan las pantallas delante del repositorio.
func fachada(t *testing.T, repo *calendar.Repository) *api.Calendars {
	t.Helper()
	c, err := api.NewCalendars("", nil, nil).WithPrivateData(repo)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func conEstado(t *testing.T, err error, quiero int, que string) {
	t.Helper()
	var ae *api.Error
	if !errors.As(err, &ae) || ae.Status != quiero {
		t.Fatalf("%s: quería %d y fue %v", que, quiero, err)
	}
}

func calendarioNuevo(t *testing.T, c *api.Calendars, nombre string) api.Calendar {
	t.Helper()
	cal, err := c.CreateCalendar(t.Context(), api.CalendarInput{Name: nombre, DisplayName: strings.ToUpper(nombre[:1]) + nombre[1:], Timezone: "Europe/Madrid"})
	if err != nil {
		t.Fatal(err)
	}
	return cal
}

// enMadrid es la hora de Madrid de ese día.
func enMadrid(a int, m time.Month, d, h, minuto int) time.Time {
	return time.Date(a, m, d, h, minuto, 0, 0, madridDelContrato)
}

// ventana es el tramo del juego: de diciembre de 2026 a marzo de 2027.
var desdeDelContrato, hastaDelContrato = enMadrid(2026, 12, 1, 0, 0), enMadrid(2027, 3, 31, 0, 0)

func instancias(t *testing.T, c *api.Calendars, cal string) []api.Instance {
	t.Helper()
	is, err := c.Instances(t.Context(), cal, desdeDelContrato, hastaDelContrato)
	if err != nil {
		t.Fatal(err)
	}
	return is
}

// ocurrencias son las instancias de cal como «día hora título [lugar]», en su orden.
func ocurrencias(t *testing.T, c *api.Calendars, cal string) []string {
	t.Helper()
	is := instancias(t, c, cal)
	out := make([]string, 0, len(is))
	for _, i := range is {
		s := i.Start.In(madridDelContrato).Format("01-02 15:04") + " " + i.Event.Summary
		if i.Event.Location != "" {
			s += " [" + i.Event.Location + "]"
		}
		out = append(out, s)
	}
	return out
}

// objeto es el iCalendar guardado del UID en cal.
func objeto(t *testing.T, repo *calendar.Repository, cal, uid string) calendar.ObjetoCrudo {
	t.Helper()
	lista, err := repo.ObjetosCrudos(t.Context(), cal)
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range lista {
		if o.UID == uid {
			return o
		}
	}
	t.Fatalf("no está el objeto %s en %v", uid, lista)
	return calendar.ObjetoCrudo{}
}

func resumenes(es []api.Event) []string {
	out := make([]string, len(es))
	for i, e := range es {
		out[i] = e.Summary
	}
	return out
}

// Calendarios: validar, valores por defecto, orden, editar y la papelera.
func TestContratoDeCalendarCalendarios(t *testing.T) {
	porCadaPuerto(t, func(t *testing.T, _ puertoDeCalendar, a *almacenDeCalendar) {
		ctx := t.Context()
		repo, cerrar := a.abrir(t, "app", opcionesDeApertura{})
		defer cerrar()
		c := fachada(t, repo)
		trabajo := calendarioNuevo(t, c, "trabajo")
		if trabajo.Color != "#6B3E9E" || trabajo.Timezone != "Europe/Madrid" || trabajo.ReadOnly || trabajo.IsDefault {
			t.Fatalf("valores por defecto: %+v", trabajo)
		}
		casa, err := c.CreateCalendar(ctx, api.CalendarInput{Name: "casa", DisplayName: "Casa", Color: "#0E7C86"})
		if err != nil || casa.Timezone != "UTC" || casa.Color != "#0E7C86" {
			t.Fatalf("sin zona va en UTC: %+v %v", casa, err)
		}
		if _, err := c.CreateCalendar(ctx, api.CalendarInput{Name: "mal", Timezone: "Marte/Olympus"}); err == nil {
			t.Error("una zona que no existe entra")
		}
		if _, err := c.CreateCalendar(ctx, api.CalendarInput{Name: "mal", Color: "#zz"}); err == nil {
			t.Error("un color que no es un color entra")
		}
		if _, err := c.CreateCalendar(ctx, api.CalendarInput{Name: ""}); err == nil {
			t.Error("un calendario sin nombre entra")
		}
		cals, err := c.List(ctx)
		if err != nil || len(cals) != 2 || cals[0].ID != casa.ID || cals[1].ID != trabajo.ID {
			t.Fatalf("por nombre: %+v %v", cals, err)
		}
		editado, err := c.UpdateCalendar(ctx, trabajo.ID, api.CalendarInput{DisplayName: "Oficina", Color: "#123456"})
		if err != nil || editado.DisplayName != "Oficina" || editado.Color != "#123456" || editado.Timezone != "Europe/Madrid" || editado.Name != "trabajo" {
			t.Fatalf("editar cambia solo lo que llega: %+v %v", editado, err)
		}
		_, err = c.UpdateCalendar(ctx, trabajo.ID, api.CalendarInput{Timezone: "Marte/Olympus"})
		conEstado(t, err, http.StatusBadRequest, "editar con una zona que no existe")
		_, err = c.UpdateCalendar(ctx, "6f1d2c3b-5a49-4e87-9c10-0d6b7a8e9f21", api.CalendarInput{DisplayName: "x"})
		conEstado(t, err, http.StatusNotFound, "editar uno que no existe")

		// A la papelera: deja de verse, y sus eventos con él.
		inicio := enMadrid(2027, 1, 5, 9, 0)
		if _, err := c.CreateEvent(ctx, casa.ID, api.EventInput{Summary: "Fontanero", Start: inicio, End: inicio.Add(time.Hour)}); err != nil {
			t.Fatal(err)
		}
		if err := c.DeleteCalendar(ctx, casa.ID); err != nil {
			t.Fatal(err)
		}
		if cals, _ := c.List(ctx); len(cals) != 1 || cals[0].ID != trabajo.ID {
			t.Fatalf("el borrado sigue en la lista: %+v", cals)
		}
		_, err = c.Instances(ctx, casa.ID, desdeDelContrato, hastaDelContrato)
		conEstado(t, err, http.StatusNotFound, "las instancias de un calendario borrado")
		if hits, _ := c.SearchEvents(ctx, "fontanero", 10); len(hits) != 0 {
			t.Fatalf("lo del calendario borrado se sigue encontrando: %+v", hits)
		}
		_, err = c.CreateEvent(ctx, casa.ID, api.EventInput{Summary: "x", Start: inicio, End: inicio.Add(time.Hour)})
		conEstado(t, err, http.StatusNotFound, "crear en un calendario borrado")
		// Compartir, recursos y salas son de un servidor: fallan cerrados.
		if err := c.Share(ctx, trabajo.ID, "otra@ejemplo.org", api.PermisoLeer); err == nil {
			t.Error("compartir sin servidor no falla")
		}
		if _, err := c.SharedWithMe(ctx); err == nil {
			t.Error("los compartidos conmigo sin servidor no fallan")
		}
	})
}

// Eventos: crear, leer con sus recordatorios, editar, buscar (con y sin filtro, por
// páginas), exportar, los avisos vencidos y la papelera.
func TestContratoDeCalendarEventos(t *testing.T) {
	porCadaPuerto(t, func(t *testing.T, _ puertoDeCalendar, a *almacenDeCalendar) {
		ctx := t.Context()
		repo, cerrar := a.abrir(t, "app", opcionesDeApertura{})
		defer cerrar()
		c := fachada(t, repo)
		cal := calendarioNuevo(t, c, "trabajo")
		inicio := enMadrid(2027, 1, 12, 10, 0)
		quince := []int{15}
		rev, err := c.CreateEvent(ctx, cal.ID, api.EventInput{
			Summary: "Revisión trimestral", Location: "Sala Ébano", Description: "Traer las cifras",
			Start: inicio, End: inicio.Add(90 * time.Minute), TZID: "Europe/Madrid", Reminders: &quince,
			Attendees: []api.Attendee{{Email: "luis@ejemplo.org", CommonName: "Luis"}},
		})
		if err != nil || rev.Status != "CONFIRMED" || rev.UID == "" || len(rev.Attendees) != 1 || rev.Attendees[0].PartStat != "NEEDS-ACTION" {
			t.Fatalf("crear: %+v %v", rev, err)
		}
		if _, err := c.CreateEvent(ctx, cal.ID, api.EventInput{Summary: "al revés", Start: inicio, End: inicio.Add(-time.Hour)}); err == nil {
			t.Error("un evento que acaba antes de empezar entra")
		}
		if _, err := c.CreateEvent(ctx, cal.ID, api.EventInput{Summary: "regla mala", Start: inicio, End: inicio, RRule: "FREQ=CADA-POCO"}); err == nil {
			t.Error("una regla que no es una regla entra")
		}
		if _, err := c.CreateEvent(ctx, cal.ID, api.EventInput{
			Summary: "dos veces el mismo", Start: inicio, End: inicio,
			Attendees: []api.Attendee{{Email: "a@ejemplo.org"}, {Email: "A@ejemplo.org"}},
		}); err == nil {
			t.Error("el mismo invitado dos veces entra")
		}
		if _, err := c.CreateEvent(ctx, cal.ID, api.EventInput{Summary: "mandar", Start: inicio, End: inicio, Send: true}); !errors.Is(err, calendar.ErrPrivateIngress) {
			t.Errorf("mandar invitaciones sin servidor: %v", err)
		}
		leido, err := c.GetEvent(ctx, rev.ID)
		if err != nil || leido.Reminders == nil || !slices.Equal(*leido.Reminders, quince) || leido.Description != "Traer las cifras" || !leido.DTStart.Equal(inicio) {
			t.Fatalf("leer: %+v %v", leido, err)
		}
		// Editar sustituye el evento entero.
		in := leido.Input()
		in.Summary, in.Location = "Revisión del trimestre", ""
		editado, err := c.UpdateEvent(ctx, rev.ID, in)
		if err != nil || editado.Summary != "Revisión del trimestre" || editado.Location != "" || editado.UID != rev.UID {
			t.Fatalf("editar: %+v %v", editado, err)
		}
		_, err = c.GetEvent(ctx, "6f1d2c3b-5a49-4e87-9c10-0d6b7a8e9f21")
		conEstado(t, err, http.StatusNotFound, "leer uno que no existe")

		comida := enMadrid(2027, 1, 14, 14, 0)
		if _, err := c.CreateEvent(ctx, cal.ID, api.EventInput{Summary: "Comida con el equipo", Location: "Revuelta", Start: comida, End: comida.Add(time.Hour)}); err != nil {
			t.Fatal(err)
		}
		if _, err := c.CreateEvent(ctx, cal.ID, api.EventInput{Summary: "Revisar el contrato", Start: enMadrid(2027, 2, 2, 9, 0), End: enMadrid(2027, 2, 2, 10, 0)}); err != nil {
			t.Fatal(err)
		}
		if got := ocurrencias(t, c, cal.ID); !slices.Equal(got, []string{"01-12 10:00 Revisión del trimestre", "01-14 14:00 Comida con el equipo [Revuelta]", "02-02 09:00 Revisar el contrato"}) {
			t.Fatalf("instancias: %q", got)
		}
		_, err = c.Instances(ctx, cal.ID, desdeDelContrato, desdeDelContrato.AddDate(2, 0, 0))
		if err == nil {
			t.Error("un tramo de dos años entra")
		}
		// Buscar sin acentos ni mayúsculas, en el título, el lugar y los invitados.
		for q, quiero := range map[string][]string{
			"REVISION": {"Revisión del trimestre"}, "revis": {"Revisión del trimestre", "Revisar el contrato"},
			"revuelta": {"Comida con el equipo"}, "luis@": {"Revisión del trimestre"}, "x": {}, "nadie": {},
		} {
			hits, err := c.SearchEvents(ctx, q, 10)
			if err != nil || !slices.Equal(resumenes(hits), quiero) {
				t.Fatalf("buscar %q = %q, %v", q, resumenes(hits), err)
			}
		}
		var todos []string
		cursor := ""
		for {
			p, err := c.SearchEventsPage(ctx, "re", 1, cursor)
			if err != nil {
				t.Fatal(err)
			}
			todos = append(todos, resumenes(p.Events)...)
			if cursor = p.Siguiente; cursor == "" {
				break
			}
		}
		if !slices.Equal(todos, []string{"Revisión del trimestre", "Comida con el equipo", "Revisar el contrato"}) {
			t.Fatalf("de uno en uno: %q", todos)
		}
		if _, err := c.SearchEventsPage(ctx, "otra cosa", 1, "no-es-un-cursor"); err == nil {
			t.Error("un cursor de otra búsqueda vale")
		}
		f, err := c.SearchEventsFiltrada(ctx, "re", api.FiltroBusqueda{Calendario: cal.ID, Desde: enMadrid(2027, 2, 1, 0, 0)}, 10, "")
		if err != nil || !slices.Equal(resumenes(f.Events), []string{"Revisar el contrato"}) {
			t.Fatalf("con filtro: %q %v", resumenes(f.Events), err)
		}
		ics, err := c.ExportICS(ctx, cal.ID)
		if err != nil || strings.Count(string(ics), "BEGIN:VEVENT") != 3 || !strings.Contains(string(ics), "SUMMARY:Revisión del trimestre") {
			t.Fatalf("exportar: %s %v", ics, err)
		}

		// Un aviso que vence: avisa al empezar, dentro de un par de segundos (se guardan
		// los que aún no han llegado), y se descarta.
		pronto := time.Now().Truncate(time.Second).Add(2 * time.Second)
		if _, err := c.CreateEvent(ctx, cal.ID, api.EventInput{Summary: "Llamar al banco", Start: pronto, End: pronto.Add(30 * time.Minute), Reminders: &[]int{0}}); err != nil {
			t.Fatal(err)
		}
		if avisos, err := c.Reminders(ctx); err != nil || len(avisos) != 0 {
			t.Fatalf("un aviso que no ha llegado: %+v %v", avisos, err)
		}
		time.Sleep(time.Until(pronto) + 100*time.Millisecond)
		avisos, err := c.Reminders(ctx)
		if err != nil || len(avisos) != 1 || avisos[0].Summary != "Llamar al banco" || !avisos[0].TriggerAt.Equal(pronto) {
			t.Fatalf("avisos: %+v %v", avisos, err)
		}
		if err := c.DismissReminder(ctx, avisos[0].ID); err != nil {
			t.Fatal(err)
		}
		if avisos, _ := c.Reminders(ctx); len(avisos) != 0 {
			t.Fatalf("el aviso descartado sigue: %+v", avisos)
		}

		// A la papelera: ni se ve, ni se busca, ni se exporta, ni se lee.
		if err := c.DeleteEvent(ctx, rev.ID); err != nil {
			t.Fatal(err)
		}
		if got := ocurrencias(t, c, cal.ID); slices.Contains(got, "01-12 10:00 Revisión del trimestre") {
			t.Fatalf("el borrado se sigue viendo: %q", got)
		}
		if hits, _ := c.SearchEvents(ctx, "trimestre", 10); len(hits) != 0 {
			t.Fatalf("el borrado se sigue encontrando: %+v", hits)
		}
		if ics, _ := c.ExportICS(ctx, cal.ID); strings.Contains(string(ics), "trimestre") {
			t.Fatal("el borrado se sigue exportando")
		}
		_, err = c.GetEvent(ctx, rev.ID)
		conEstado(t, err, http.StatusNotFound, "leer uno borrado")
		conEstado(t, c.DeleteEvent(ctx, rev.ID), http.StatusNotFound, "borrar otra vez")
	})
}

// serieDelContrato es una serie semanal de Ana que trae una cuenta: seis lunes desde el 4
// de enero de 2027 a las 10 en Madrid, sin el tercero (EXDATE en la hora de su zona), el
// segundo movido a las 12 con otro título y el cuarto en otra sala; Luis y la persona,
// invitados sin contestar.
const serieDelContrato = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//prueba//ES\r\n" +
	"BEGIN:VEVENT\r\nUID:serie@prueba\r\nDTSTAMP:20261201T080000Z\r\nSEQUENCE:1\r\n" +
	"DTSTART;TZID=Europe/Madrid:20270104T100000\r\nDTEND;TZID=Europe/Madrid:20270104T110000\r\n" +
	"RRULE:FREQ=WEEKLY;COUNT=6\r\nEXDATE;TZID=Europe/Madrid:20270118T100000\r\n" +
	"SUMMARY:Seguimiento\r\nLOCATION:Sala 2\r\nORGANIZER;CN=Ana:mailto:ana@ejemplo.org\r\n" +
	"ATTENDEE;CN=Luis;PARTSTAT=NEEDS-ACTION;RSVP=TRUE:mailto:luis@ejemplo.org\r\n" +
	"ATTENDEE;PARTSTAT=NEEDS-ACTION;RSVP=TRUE:mailto:yo@ejemplo.org\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:serie@prueba\r\nDTSTAMP:20261201T080000Z\r\nSEQUENCE:1\r\n" +
	"RECURRENCE-ID;TZID=Europe/Madrid:20270111T100000\r\n" +
	"DTSTART;TZID=Europe/Madrid:20270111T120000\r\nDTEND;TZID=Europe/Madrid:20270111T130000\r\n" +
	"SUMMARY:Seguimiento (movido)\r\nLOCATION:Sala 2\r\nORGANIZER;CN=Ana:mailto:ana@ejemplo.org\r\n" +
	"ATTENDEE;CN=Luis;PARTSTAT=NEEDS-ACTION;RSVP=TRUE:mailto:luis@ejemplo.org\r\n" +
	"ATTENDEE;PARTSTAT=NEEDS-ACTION;RSVP=TRUE:mailto:yo@ejemplo.org\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:serie@prueba\r\nDTSTAMP:20261201T080000Z\r\nSEQUENCE:1\r\n" +
	"RECURRENCE-ID;TZID=Europe/Madrid:20270125T100000\r\n" +
	"DTSTART;TZID=Europe/Madrid:20270125T100000\r\nDTEND;TZID=Europe/Madrid:20270125T110000\r\n" +
	"SUMMARY:Seguimiento\r\nLOCATION:Sala 3\r\nORGANIZER;CN=Ana:mailto:ana@ejemplo.org\r\n" +
	"ATTENDEE;CN=Luis;PARTSTAT=NEEDS-ACTION;RSVP=TRUE:mailto:luis@ejemplo.org\r\n" +
	"ATTENDEE;PARTSTAT=NEEDS-ACTION;RSVP=TRUE:mailto:yo@ejemplo.org\r\nEND:VEVENT\r\n" +
	"END:VCALENDAR\r\n"

// enLaExcepcion es datos con el VEVENT que lleva marca cambiado por cambiar.
func enLaExcepcion(datos, marca string, cambiar func(string) string) string {
	partes := strings.Split(datos, "BEGIN:VEVENT")
	for i, p := range partes {
		if strings.Contains(p, marca) {
			partes[i] = cambiar(p)
		}
	}
	return strings.Join(partes, "BEGIN:VEVENT")
}

// maestroDeLaSerie es el evento maestro de la serie, como lo abre el formulario.
func maestroDeLaSerie(t *testing.T, c *api.Calendars, cal string) api.Event {
	t.Helper()
	for _, i := range instancias(t, c, cal) {
		if i.Event.UID == "serie@prueba" && i.Event.RecurrenceID == nil {
			ev, err := c.GetEvent(t.Context(), i.Event.ID)
			if err != nil {
				t.Fatal(err)
			}
			return ev
		}
	}
	t.Fatal("no está el maestro de la serie")
	return api.Event{}
}

// Repeticiones con excepciones y EXDATE, con las opciones de Sytue Office: la serie
// como llega de una cuenta, lo cambiado en la serie que llega a las excepciones que lo
// tenían igual (SerieALasExcepciones), «solo este» (con la EXDATE en la hora de su zona,
// ConservarForma), contestar por UID y «este y los siguientes», que lleva lo contestado
// al evento nuevo y quita de la serie las excepciones de lo recortado
// (ConservarRespuestas).
func TestContratoDeCalendarSeries(t *testing.T) {
	porCadaPuerto(t, func(t *testing.T, _ puertoDeCalendar, a *almacenDeCalendar) {
		ctx := t.Context()
		repo, cerrar := a.abrir(t, "app", opcionesDeApertura{})
		defer cerrar()
		c := fachada(t, repo)
		cal := calendarioNuevo(t, c, "cuenta")
		res, err := repo.ImportICS(ctx, cal.ID, []byte(serieDelContrato))
		if err != nil || res.Importados != 1 {
			t.Fatal("importar la serie:", res, err)
		}
		if res, err := repo.ImportICS(ctx, cal.ID, []byte(serieDelContrato)); err != nil || res.Omitidos != 1 || res.Importados != 0 {
			t.Fatal("importarla otra vez no la duplica:", res, err)
		}
		if got := ocurrencias(t, c, cal.ID); !slices.Equal(got, []string{
			"01-04 10:00 Seguimiento [Sala 2]", "01-11 12:00 Seguimiento (movido) [Sala 2]", "01-25 10:00 Seguimiento [Sala 3]",
			"02-01 10:00 Seguimiento [Sala 2]", "02-08 10:00 Seguimiento [Sala 2]",
		}) {
			t.Fatalf("la serie con su EXDATE y sus excepciones: %q", got)
		}

		// Cambiar el título de la serie: llega a la del 25, que lo tenía como la serie;
		// la del 11, que lo tenía cambiado, lo conserva.
		maestro := maestroDeLaSerie(t, c, cal.ID)
		in := maestro.Input()
		in.Summary = "Seguimiento semanal"
		if _, err := c.UpdateEvent(ctx, maestro.ID, in); err != nil {
			t.Fatal(err)
		}
		if got := ocurrencias(t, c, cal.ID); !slices.Equal(got, []string{
			"01-04 10:00 Seguimiento semanal [Sala 2]", "01-11 12:00 Seguimiento (movido) [Sala 2]", "01-25 10:00 Seguimiento semanal [Sala 3]",
			"02-01 10:00 Seguimiento semanal [Sala 2]", "02-08 10:00 Seguimiento semanal [Sala 2]",
		}) {
			t.Fatalf("lo cambiado en la serie, en las excepciones que lo tenían igual: %q", got)
		}

		// «Solo este» borra el 1 de febrero: la EXDATE nueva va como el DTSTART.
		if err := c.AmendRecurrence(ctx, cal.ID, "serie@prueba", enMadrid(2027, 2, 1, 10, 0), api.RecorteExcluir); err != nil {
			t.Fatal(err)
		}
		datos := objeto(t, repo, cal.ID, "serie@prueba").Datos
		if !strings.Contains(datos, "EXDATE;TZID=Europe/Madrid:20270201T100000") || !strings.Contains(datos, "20270118T100000") {
			t.Fatalf("la EXDATE en la hora de Madrid, y la de antes: %s", datos)
		}
		if got := ocurrencias(t, c, cal.ID); len(got) != 4 || slices.Contains(got, "02-01 10:00 Seguimiento semanal [Sala 2]") {
			t.Fatalf("sin el 1 de febrero: %q", got)
		}
		conEstado(t, c.AmendRecurrence(ctx, cal.ID, "no-existe@prueba", enMadrid(2027, 2, 1, 10, 0), api.RecorteExcluir), http.StatusNotFound, "recortar una serie que no existe")
		if err := c.AmendRecurrence(ctx, cal.ID, "serie@prueba", time.Time{}, api.RecorteExcluir); err == nil {
			t.Error("recortar sin ocurrencia entra")
		}

		// La persona acepta la serie (A-02): su respuesta va al maestro y a las excepciones.
		if err := c.ResponderPorUID(ctx, "serie@prueba", api.AsistenciaSi); err != nil {
			t.Fatal(err)
		}
		datos = objeto(t, repo, cal.ID, "serie@prueba").Datos
		if strings.Contains(datos, "NEEDS-ACTION;RSVP=TRUE:mailto:yo@") || !strings.Contains(datos, "PARTSTAT=ACCEPTED") {
			t.Fatalf("la respuesta de la persona, en toda la serie: %s", datos)
		}
		conEstado(t, c.ResponderPorUID(ctx, "nadie@prueba", api.AsistenciaSi), http.StatusNotFound, "contestar a un UID que no está")

		// «Este y los siguientes» desde el 25: mientras el formulario está abierto, Luis
		// declina esa ocurrencia (su excepción). El evento nuevo se lleva esa excepción,
		// con lo contestado y el título nuevo; la serie acaba antes y la pierde.
		maestro = maestroDeLaSerie(t, c, cal.ID)
		nueva := maestro.Input()
		nueva.Summary, nueva.Start, nueva.End = "Seguimiento (segunda etapa)", enMadrid(2027, 1, 25, 10, 0), enMadrid(2027, 1, 25, 11, 0)
		nueva.RRule, nueva.ExDate = "FREQ=WEEKLY;COUNT=2", nil
		o := objeto(t, repo, cal.ID, "serie@prueba")
		declina := enLaExcepcion(o.Datos, "RECURRENCE-ID;TZID=Europe/Madrid:20270125T100000", func(b string) string {
			return strings.Replace(b, "PARTSTAT=NEEDS-ACTION;RSVP=TRUE:mailto:luis@", "PARTSTAT=DECLINED;RSVP=TRUE:mailto:luis@", 1)
		})
		if declina == o.Datos {
			t.Fatalf("la excepción del 25 no está como se esperaba: %s", o.Datos)
		}
		if _, err := repo.PonerObjetoCrudo(ctx, cal.ID, o.ID, o.Datos, declina); err != nil {
			t.Fatal(err)
		}
		if err := c.DividirSerie(ctx, cal.ID, "serie@prueba", enMadrid(2027, 1, 25, 10, 0), api.RecorteHasta, nueva); err != nil {
			t.Fatal(err)
		}
		if got := ocurrencias(t, c, cal.ID); !slices.Equal(got, []string{
			"01-04 10:00 Seguimiento semanal [Sala 2]", "01-11 12:00 Seguimiento (movido) [Sala 2]",
			"01-25 10:00 Seguimiento (segunda etapa) [Sala 3]", "02-01 10:00 Seguimiento (segunda etapa) [Sala 2]",
		}) {
			t.Fatalf("la serie recortada y la segunda etapa, con la excepción del 25 (en la Sala 3): %q", got)
		}
		serie := objeto(t, repo, cal.ID, "serie@prueba").Datos
		if strings.Contains(serie, "RECURRENCE-ID;TZID=Europe/Madrid:20270125") || !strings.Contains(serie, "RECURRENCE-ID;TZID=Europe/Madrid:20270111") {
			t.Fatalf("la serie sin la excepción de lo recortado, con la de antes: %s", serie)
		}
		var etapa string
		lista, _ := repo.ObjetosCrudos(ctx, cal.ID)
		for _, x := range lista {
			if x.UID != "serie@prueba" {
				etapa = x.Datos
			}
		}
		if len(lista) != 2 || !strings.Contains(etapa, "PARTSTAT=DECLINED") || !strings.Contains(etapa, "ORGANIZER;CN=Ana:mailto:ana@ejemplo.org") {
			t.Fatalf("la segunda etapa con lo que contestó Luis a esa ocurrencia: %s", etapa)
		}
	})
}

const reunionDelContrato = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//prueba//ES\r\n" +
	"BEGIN:VEVENT\r\nUID:reunion@prueba\r\nDTSTAMP:20261201T000000Z\r\nSEQUENCE:2\r\nDTSTART:20270310T090000Z\r\nDTEND:20270310T100000Z\r\n" +
	"SUMMARY:Revisión\r\nSTATUS:CONFIRMED\r\nORGANIZER:mailto:ana@ejemplo.org\r\n" +
	"ATTENDEE;PARTSTAT=NEEDS-ACTION;RSVP=TRUE:mailto:luis@ejemplo.org\r\n" +
	"ATTENDEE;PARTSTAT=NEEDS-ACTION;RSVP=TRUE:mailto:yo@ejemplo.org\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"

// secuencia es la SEQUENCE del VEVENT.
func secuencia(d string) string {
	_, v, _ := strings.Cut(d, "\r\nSEQUENCE")
	v, _, _ = strings.Cut(v, "\r\n")
	_, v, _ = strings.Cut(v, ":")
	return v
}

// Respuestas: contestar desde el evento, y guardar lo que se leyó al abrir el formulario
// con una respuesta que llegó entre medias. Con las opciones de Sytue Office no se pisa y
// un recordatorio no sube la SEQUENCE; como la conectada, manda lo que llega.
func TestContratoDeCalendarRespuestas(t *testing.T) {
	porCadaPuerto(t, func(t *testing.T, _ puertoDeCalendar, a *almacenDeCalendar) {
		for _, conectada := range []bool{false, true} {
			ctx := t.Context()
			repo, cerrar := a.abrir(t, fmt.Sprint("conectada-", conectada), opcionesDeApertura{comoLaConectada: conectada})
			c := fachada(t, repo)
			cal := calendarioNuevo(t, c, fmt.Sprint("respuestas", conectada))
			if _, err := repo.PonerObjetoCrudo(ctx, cal.ID, "", "", reunionDelContrato); err != nil {
				t.Fatal(err)
			}
			if _, err := repo.PonerObjetoCrudo(ctx, cal.ID, "", "", reunionDelContrato); err == nil {
				t.Fatal("el mismo UID dos veces en un calendario entra")
			}
			is := instancias(t, c, cal.ID)
			if len(is) != 1 {
				t.Fatal(is)
			}
			if err := c.SetPartStat(ctx, is[0].Event.ID, api.AsistenciaQuiza); err != nil {
				t.Fatal(err)
			}
			conEstado(t, c.SetPartStat(ctx, is[0].Event.ID, "QUIZÁ"), http.StatusBadRequest, "una respuesta que no existe")
			ev, err := c.GetEvent(ctx, is[0].Event.ID) // abrir el formulario
			if err != nil || len(ev.Attendees) != 2 {
				t.Fatal(ev, err)
			}
			for _, x := range ev.Attendees {
				if x.Email == correoDeLaPersona && x.PartStat != api.AsistenciaQuiza {
					t.Fatalf("la respuesta de la persona, en el evento: %+v", ev.Attendees)
				}
			}
			o := objeto(t, repo, cal.ID, "reunion@prueba")
			if _, err := repo.PonerObjetoCrudo(ctx, cal.ID, o.ID, o.Datos, strings.Replace(o.Datos, "PARTSTAT=NEEDS-ACTION", "PARTSTAT=ACCEPTED", 1)); err != nil {
				t.Fatal(err)
			}
			if _, err := repo.PonerObjetoCrudo(ctx, cal.ID, o.ID, o.Datos, o.Datos); !errors.Is(err, calendar.ErrCambioLocal) {
				t.Fatalf("pisar con lo que había antes: %v", err)
			}
			in := ev.Input() // guardar con un recordatorio
			in.Reminders = &[]int{15}
			if _, err := c.UpdateEvent(ctx, ev.ID, in); err != nil {
				t.Fatal(err)
			}
			d := objeto(t, repo, cal.ID, "reunion@prueba").Datos
			switch {
			case !strings.Contains(d, "TRIGGER:-PT15M"):
				t.Fatalf("el recordatorio: %s", d)
			case !conectada && (!strings.Contains(d, "PARTSTAT=ACCEPTED") || secuencia(d) != "2"):
				t.Fatalf("con las opciones de Sytue Office, Luis sigue aceptando y la SEQUENCE no sube: %s", d)
			case conectada && (strings.Contains(d, "PARTSTAT=ACCEPTED") || secuencia(d) != "3"):
				t.Fatalf("como la conectada, manda lo que llega: %s", d)
			}
			cerrar()
		}
	})
}

// Suscripciones: de solo lectura, se leen al suscribirse y al refrescar (entra, cambia y
// se va lo del feed), un feed que no llega deja la suscripción como estaba, y sin
// Descargar fallan cerradas.
func TestContratoDeCalendarSuscripciones(t *testing.T) {
	porCadaPuerto(t, func(t *testing.T, _ puertoDeCalendar, a *almacenDeCalendar) {
		ctx := t.Context()
		repo, cerrar := a.abrir(t, "app", opcionesDeApertura{})
		defer cerrar()
		c := fachada(t, repo)
		if _, err := c.Subscribe(ctx, api.SubscriptionInput{URL: "https://ejemplo.org/f.ics"}); !errors.Is(err, calendar.ErrPrivateSubscriptions) {
			t.Fatalf("sin Descargar: %v", err)
		}
		feed, caido := icsDePrueba, false
		repo.Descargar = func(context.Context, string) ([]byte, error) {
			if caido {
				return nil, errors.New("prueba: sin conexión")
			}
			return []byte(feed), nil
		}
		if _, err := c.Subscribe(ctx, api.SubscriptionInput{URL: "ftp://ejemplo.org/f.ics"}); err == nil {
			t.Error("una URL que no es http entra")
		}
		sub, err := c.Subscribe(ctx, api.SubscriptionInput{URL: "webcal://ejemplo.org/festivos.ics"})
		if err != nil || !sub.ReadOnly || sub.DisplayName != "ejemplo.org" || sub.Color != "#0E7C86" {
			t.Fatalf("suscribirse: %+v %v", sub, err)
		}
		desde, hasta := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
		if is, _ := c.Instances(ctx, sub.ID, desde, hasta); len(is) != 5 {
			t.Fatalf("instancias de la suscripción = %d", len(is))
		}
		if _, err := c.CreateEvent(ctx, sub.ID, api.EventInput{Summary: "x", Start: desde, End: desde.Add(time.Hour)}); err == nil {
			t.Fatal("una suscripción es de solo lectura")
		}
		cabecera, _, _ := strings.Cut(icsDePrueba, "BEGIN:VEVENT")
		_, suelto, _ := strings.Cut(icsDePrueba, "BEGIN:VEVENT\r\nUID:suelto")
		feed = strings.Replace(cabecera+"BEGIN:VEVENT\r\nUID:suelto"+suelto, "SUMMARY:Comida", "SUMMARY:Comida de equipo", 1)
		if err := c.RefreshSubscription(ctx, sub.ID); err != nil {
			t.Fatal(err)
		}
		is, _ := c.Instances(ctx, sub.ID, desde, hasta)
		if len(is) != 1 || is[0].Event.Summary != "Comida de equipo" {
			t.Fatalf("tras refrescar: %+v", is)
		}
		caido = true
		if err := c.RefreshSubscription(ctx, sub.ID); err == nil {
			t.Fatal("un feed que no llega no falla")
		}
		if is, _ := c.Instances(ctx, sub.ID, desde, hasta); len(is) != 1 {
			t.Fatalf("un feed que no llega se lleva lo que había: %+v", is)
		}
		cal := calendarioNuevo(t, c, "propio")
		conEstado(t, c.RefreshSubscription(ctx, cal.ID), http.StatusBadRequest, "refrescar un calendario que no es una suscripción")
	})
}

// vistaDeCalendar es lo que enseña la pantalla de un almacén: sus calendarios y lo que
// hay en ellos en el tramo del juego, sin los identificadores (que son al azar).
func vistaDeCalendar(t *testing.T, c *api.Calendars) []string {
	t.Helper()
	cals, err := c.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, cal := range cals {
		out = append(out, fmt.Sprintf("calendario %s %q %s %s solo-lectura=%v", cal.Name, cal.DisplayName, cal.Color, cal.Timezone, cal.ReadOnly))
		for _, i := range instancias(t, c, cal.ID) {
			var quienes []string
			for _, x := range i.Event.Attendees {
				quienes = append(quienes, x.Email+"="+x.PartStat)
			}
			slices.Sort(quienes)
			out = append(out, fmt.Sprintf("  %s–%s %q %q %s excepción=%v %v", i.Start.UTC().Format(time.RFC3339), i.End.UTC().Format(time.RFC3339),
				i.Event.Summary, i.Event.Location, i.Event.Status, i.Event.RecurrenceID != nil, quienes))
		}
	}
	return out
}

// juegoParaComparar es un poco de todo lo anterior, sobre un almacén nuevo.
func juegoParaComparar(t *testing.T, a *almacenDeCalendar) []string {
	t.Helper()
	ctx := t.Context()
	repo, cerrar := a.abrir(t, "app", opcionesDeApertura{})
	c := fachada(t, repo)
	cal := calendarioNuevo(t, c, "cuenta")
	if _, err := repo.ImportICS(ctx, cal.ID, []byte(serieDelContrato)); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ImportICS(ctx, cal.ID, []byte(reunionDelContrato)); err != nil {
		t.Fatal(err)
	}
	maestro := maestroDeLaSerie(t, c, cal.ID)
	in := maestro.Input()
	in.Summary, in.Location = "Seguimiento semanal", "Sala 4"
	if _, err := c.UpdateEvent(ctx, maestro.ID, in); err != nil {
		t.Fatal(err)
	}
	if err := c.AmendRecurrence(ctx, cal.ID, "serie@prueba", enMadrid(2027, 2, 1, 10, 0), api.RecorteExcluir); err != nil {
		t.Fatal(err)
	}
	if err := c.ResponderPorUID(ctx, "reunion@prueba", api.AsistenciaNo); err != nil {
		t.Fatal(err)
	}
	otro := calendarioNuevo(t, c, "casa")
	ev, err := c.CreateEvent(ctx, otro.ID, api.EventInput{Summary: "Dentista", Start: enMadrid(2027, 1, 20, 18, 0), End: enMadrid(2027, 1, 20, 19, 0), TZID: "Europe/Madrid"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.CreateEvent(ctx, otro.ID, api.EventInput{Summary: "Cumpleaños de Zoe", AllDay: true, Start: time.Date(2027, 2, 3, 0, 0, 0, 0, time.UTC), End: time.Date(2027, 2, 4, 0, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	if err := c.DeleteEvent(ctx, ev.ID); err != nil {
		t.Fatal(err)
	}
	antes := vistaDeCalendar(t, c)
	cerrar()
	// Y lo escrito sobrevive a cerrar.
	repo, cerrar = a.abrir(t, "app", opcionesDeApertura{})
	defer cerrar()
	if despues := vistaDeCalendar(t, fachada(t, repo)); !slices.Equal(antes, despues) {
		t.Fatalf("al reabrir cambia lo que se ve:\nantes:   %q\ndespués: %q", antes, despues)
	}
	return antes
}

// Lo que enseña la pantalla tras el mismo juego es lo mismo en los dos puertos, y lo
// escrito sobrevive a reabrir.
func TestContratoDeCalendarVenLoMismo(t *testing.T) {
	var vistas [][]string
	for _, p := range puertosDeCalendar {
		t.Run(p.nombre, func(t *testing.T) { vistas = append(vistas, juegoParaComparar(t, p.nuevo(t))) })
	}
	if len(vistas) == 2 && !slices.Equal(vistas[0], vistas[1]) {
		t.Fatalf("los dos puertos no enseñan lo mismo:\n%s: %q\n%s: %q", puertosDeCalendar[0].nombre, vistas[0], puertosDeCalendar[1].nombre, vistas[1])
	}
	if len(vistas) == 2 && len(vistas[0]) < 8 {
		t.Fatalf("el juego enseña demasiado poco para comparar: %q", vistas[0])
	}
}

// Dos procesos sobre el mismo almacén (Calendar y el «Traer» del Hub): cada uno ve lo
// que escribe el otro al volver a leer, el que cambia algo después de otro lo cambia
// sobre lo de ese otro (gana el último) y al reabrir está todo y abre.
func TestContratoDeCalendarDosProcesos(t *testing.T) {
	porCadaPuerto(t, func(t *testing.T, _ puertoDeCalendar, a *almacenDeCalendar) {
		ctx := t.Context()
		repoA, cerrarA := a.abrir(t, "calendar", opcionesDeApertura{})
		cA := fachada(t, repoA)
		cal := calendarioNuevo(t, cA, "trabajo")
		repoB, cerrarB := a.abrir(t, "hub", opcionesDeApertura{})
		cB := fachada(t, repoB)
		if _, err := cA.CreateEvent(ctx, cal.ID, api.EventInput{Summary: "Escrito por A", Start: enMadrid(2027, 1, 4, 9, 0), End: enMadrid(2027, 1, 4, 10, 0)}); err != nil {
			t.Fatal(err)
		}
		if _, err := cB.CreateEvent(ctx, cal.ID, api.EventInput{Summary: "Escrito por B", Start: enMadrid(2027, 1, 5, 9, 0), End: enMadrid(2027, 1, 5, 10, 0)}); err != nil {
			t.Fatal(err)
		}
		if err := repoA.RefreshPrivateData(ctx); err != nil {
			t.Fatal(err)
		}
		dos := []string{"01-04 09:00 Escrito por A", "01-05 09:00 Escrito por B"}
		if got := ocurrencias(t, cA, cal.ID); !slices.Equal(got, dos) {
			t.Fatalf("lo que ve A al volver a leer: %q", got)
		}
		// El mismo calendario, primero A y luego B, sin que B haya vuelto a leer antes.
		if _, err := cA.UpdateCalendar(ctx, cal.ID, api.CalendarInput{DisplayName: "De A"}); err != nil {
			t.Fatal(err)
		}
		if _, err := cB.UpdateCalendar(ctx, cal.ID, api.CalendarInput{Color: "#123456"}); err != nil {
			t.Fatal(err)
		}
		for quien, c := range map[string]*api.Calendars{"A": cA, "B": cB} {
			if quien == "A" {
				if err := repoA.RefreshPrivateData(ctx); err != nil {
					t.Fatal(err)
				}
			}
			cals, err := c.List(ctx)
			if err != nil || len(cals) != 1 || cals[0].DisplayName != "De A" || cals[0].Color != "#123456" {
				t.Fatalf("%s: lo de B, sobre lo de A: %+v %v", quien, cals, err)
			}
		}
		cerrarB()
		cerrarA()
		repo, cerrar := a.abrir(t, "calendar", opcionesDeApertura{})
		defer cerrar()
		c := fachada(t, repo)
		if got := ocurrencias(t, c, cal.ID); !slices.Equal(got, dos) {
			t.Fatalf("al reabrir, lo de los dos: %q", got)
		}
		if cals, err := c.List(ctx); err != nil || len(cals) != 1 || cals[0].DisplayName != "De A" || cals[0].Color != "#123456" {
			t.Fatalf("al reabrir, el calendario de los dos: %+v %v", cals, err)
		}
	})
}

// Un corte a mitad de escritura: la publicación se hizo pero su respuesta se perdió.
// Repetir lo mismo no lo duplica; cerrar ahí y volver a abrir (el mismo proceso, con su
// diario) lo deja hecho una vez; y, donde se puede simular, una línea a medias al final
// del almacén no lo tumba.
func TestContratoDeCalendarCorteAMitad(t *testing.T) {
	porCadaPuerto(t, func(t *testing.T, p puertoDeCalendar, a *almacenDeCalendar) {
		ctx := t.Context()
		corte := &atomic.Bool{}
		repo, cerrar := a.abrir(t, "app", opcionesDeApertura{corte: corte})
		c := fachada(t, repo)
		cal := calendarioNuevo(t, c, "trabajo")
		in := api.EventInput{Summary: "Cortado y repetido", Start: enMadrid(2027, 1, 4, 9, 0), End: enMadrid(2027, 1, 4, 10, 0)}
		corte.Store(true)
		if _, err := c.CreateEvent(ctx, cal.ID, in); err == nil {
			t.Fatal("el corte no llegó a quien escribía")
		}
		if _, err := c.CreateEvent(ctx, cal.ID, in); err != nil {
			t.Fatal("repetir tras el corte:", err)
		}
		if got := ocurrencias(t, c, cal.ID); !slices.Equal(got, []string{"01-04 09:00 Cortado y repetido"}) {
			t.Fatalf("repetir lo cortado lo duplica: %q", got)
		}
		// Ahora el corte y el cierre, sin repetir: al volver a abrir, está una vez.
		in.Summary, in.Start, in.End = "Cortado y cerrado", enMadrid(2027, 1, 5, 9, 0), enMadrid(2027, 1, 5, 10, 0)
		corte.Store(true)
		if _, err := c.CreateEvent(ctx, cal.ID, in); err == nil {
			t.Fatal("el corte no llegó a quien escribía")
		}
		cerrar()
		repo, cerrar = a.abrir(t, "app", opcionesDeApertura{})
		c = fachada(t, repo)
		if got := ocurrencias(t, c, cal.ID); !slices.Equal(got, []string{"01-04 09:00 Cortado y repetido", "01-05 09:00 Cortado y cerrado"}) {
			t.Fatalf("lo cortado al cerrar, al volver a abrir: %q", got)
		}
		// Quien vio el fallo lo repite al volver: es lo cortado, que acaba, no otro.
		if _, err := c.CreateEvent(ctx, cal.ID, in); err != nil {
			t.Fatal(err)
		}
		if got := ocurrencias(t, c, cal.ID); len(got) != 2 {
			t.Fatalf("repetir lo cortado tras volver a abrir lo duplica: %q", got)
		}
		// Acabado, lo mismo otra vez ya es otro evento.
		if _, err := c.CreateEvent(ctx, cal.ID, in); err != nil {
			t.Fatal(err)
		}
		if got := ocurrencias(t, c, cal.ID); len(got) != 3 {
			t.Fatalf("lo mismo otra vez, tras acabar lo cortado, es otro evento: %q", got)
		}
		cerrar()
		if p.lineaCortada == nil {
			return
		}
		// La luz se va a mitad de escribir una línea: el almacén abre sin ella, y lo que
		// se escribe después se lee al volver a abrir.
		p.lineaCortada(t, a)
		repo, cerrar = a.abrir(t, "app", opcionesDeApertura{})
		c = fachada(t, repo)
		if got := ocurrencias(t, c, cal.ID); len(got) != 3 {
			t.Fatalf("una línea a medias al final del almacén: %q", got)
		}
		in.Summary, in.Start, in.End = "Tras el corte de luz", enMadrid(2027, 1, 6, 9, 0), enMadrid(2027, 1, 6, 10, 0)
		if _, err := c.CreateEvent(ctx, cal.ID, in); err != nil {
			t.Fatal(err)
		}
		cerrar()
		repo, cerrar = a.abrir(t, "app", opcionesDeApertura{})
		defer cerrar()
		if got := ocurrencias(t, fachada(t, repo), cal.ID); len(got) != 4 || got[3] != "01-06 09:00 Tras el corte de luz" {
			t.Fatalf("lo escrito tras una línea a medias, al volver a abrir: %q", got)
		}
	})
}
