// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package migrate_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/portabilidad"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/migrate"
)

// fakeSalida es un Google/Microsoft de destino: ficheros y contactos de fakeSytue,
// más correo y agenda.
type fakeSalida struct {
	*fakeSytue
	buzones    map[string]string      // ruta|papel → id
	correos    []string               // carpeta/contenido
	excluidas  map[string][]time.Time // asunto → repeticiones fuera de la serie
	sinPermiso bool
}

func (f *fakeSalida) MailFolder(_ context.Context, ruta, rol string) (string, error) {
	id, err := f.next("buz")
	f.buzones[ruta+"|"+rol] = id
	return id, err
}

func (f *fakeSalida) ImportMessage(_ context.Context, folderID string, _ int64, raw io.Reader) (string, error) {
	if f.sinPermiso {
		f.writes++
		return "", migrate.ErrSinPermiso
	}
	b, _ := io.ReadAll(raw)
	f.correos = append(f.correos, folderID+"/"+string(b))
	return f.next("msg")
}

func (f *fakeSalida) CreateCalendar(context.Context, string) (string, error) { return f.next("cal") }

func (f *fakeSalida) ImportEvent(_ context.Context, _ string, ev migrate.EventoSalida) (string, error) {
	f.excluidas[ev.Summary] = ev.Excluidas()
	return f.next("evt")
}

// entrega arma la respuesta de GET /api/v1/export de un producto.
func entrega(t *testing.T, escribir func(*portabilidad.Paquete) error) *http.Response {
	t.Helper()
	rec := httptest.NewRecorder()
	p, err := portabilidad.Nuevo(rec, httptest.NewRequest(http.MethodGet, "/api/v1/export?format=json", http.NoBody))
	if err == nil {
		err = escribir(p)
	}
	if err == nil {
		err = p.Cerrar()
	}
	if err != nil {
		t.Fatal(err)
	}
	return rec.Result()
}

func TestRunEntregas(t *testing.T) {
	t0 := time.Date(2026, 9, 7, 9, 0, 0, 0, time.UTC)
	t1, t2 := t0.AddDate(0, 0, 7), t0.AddDate(0, 0, 14)
	productos := map[domain.MigrationDomain]func(*portabilidad.Paquete) error{
		domain.DomainDrive: func(p *portabilidad.Paquete) error {
			if err := p.FicheroDeTamano("files/leeme.txt", t0, 4, strings.NewReader("hola")); err != nil {
				return err
			}
			if err := portabilidad.Tabla(p, "files", "file", portabilidad.Todas([]map[string]string{{"name": "leeme.txt"}})); err != nil {
				return err
			}
			return p.Fichero("files/Proyectos/2026/plan.pdf", t0, strings.NewReader("pdf"))
		},
		domain.DomainMail: func(p *portabilidad.Paquete) error {
			buzones := []map[string]string{{"path": "INBOX", "role": "inbox"}, {"path": "Clientes/Acme", "role": ""}}
			if err := portabilidad.Tabla(p, "mailboxes", "mailbox", portabilidad.Todas(buzones)); err != nil {
				return err
			}
			if err := p.Fichero("messages/INBOX/1.eml", t0, strings.NewReader("uno")); err != nil {
				return err
			}
			return p.Fichero("messages/Clientes/Acme/2.eml", t0, strings.NewReader("dos"))
		},
		domain.DomainCalendar: func(p *portabilidad.Paquete) error {
			cals := []map[string]string{{"id": "c1", "name": "personal", "display_name": "Personal"}}
			if err := portabilidad.Tabla(p, "calendars", "calendar", portabilidad.Todas(cals)); err != nil {
				return err
			}
			// El cambio de una repetición llega ANTES que su serie: tiene que salir igual de ella.
			evs := []migrate.EventoSalida{
				{UID: "s", CalendarID: "c1", Summary: "Movida", DTStart: t2.Add(time.Hour), DTEnd: t2.Add(2 * time.Hour), RecurrenceID: &t2},
				{UID: "s", CalendarID: "c1", Summary: "Semanal", DTStart: t0, DTEnd: t0.Add(time.Hour), RRule: "RRULE:FREQ=WEEKLY", ExDate: []time.Time{t1}},
			}
			return portabilidad.Tabla(p, "events", "event", portabilidad.Todas(evs))
		},
	}

	st := newMemStore("org-1")
	dst := &fakeSalida{fakeSytue: newFakeSytue(), buzones: map[string]string{}, excluidas: map[string][]time.Time{}}
	d := migrate.Deps{Dst: dst, Store: st}
	for dom, escribir := range productos {
		j := job("out-"+string(dom), domain.SourceGoogle, dom)
		j.Direction, j.SourceUser = domain.DirectionOut, ""
		st.put(j)
		if err := migrate.RunEntregas(context.Background(), d, j, entrega(t, escribir)); err != nil {
			t.Fatalf("%s: %v", dom, err)
		}
		if got, _ := st.get(j.ID); got.Status != domain.MigrationDone || got.Counts.Errors != 0 {
			t.Fatalf("%s: estado %s, %d errores (%s)", dom, got.Status, got.Counts.Errors, got.LastError)
		}
	}

	if !hasPath(dst.fakeSytue, "Proyectos", "2026", "plan.pdf") || len(dst.files) != 2 {
		t.Fatalf("ficheros: %v · carpetas: %v", dst.files, dst.folders)
	}
	if dst.buzones["INBOX|inbox"] == "" || dst.buzones["Clientes/Acme|"] == "" {
		t.Fatalf("buzones con su papel: %v", dst.buzones)
	}
	if want := []time.Time{t1, t2}; !slices.EqualFunc(dst.excluidas["Semanal"], want, time.Time.Equal) {
		t.Fatalf("la serie debe excluir la repetición quitada y la cambiada: %v", dst.excluidas["Semanal"])
	}
	if _, ok := dst.excluidas["Movida"]; !ok {
		t.Fatal("la repetición cambiada debe importarse como evento suelto")
	}

	// Volver a lanzarlo no escribe nada: todo está en el mapa.
	escrituras := dst.writes
	for dom, escribir := range productos {
		j, _ := st.get("out-" + string(dom))
		if err := migrate.RunEntregas(context.Background(), d, *j, entrega(t, escribir)); err != nil {
			t.Fatalf("relanzar %s: %v", dom, err)
		}
	}
	if dst.writes != escrituras {
		t.Fatalf("relanzar duplicó %d escrituras", dst.writes-escrituras)
	}

	// Un destino que no deja escribir para el trabajo en el primer correo.
	otro := &fakeSalida{fakeSytue: newFakeSytue(), buzones: map[string]string{}, sinPermiso: true}
	j := job("sin-permiso", domain.SourceGoogle, domain.DomainMail)
	st.put(j)
	err := migrate.RunEntregas(context.Background(), migrate.Deps{Dst: otro, Store: st}, j, entrega(t, productos[domain.DomainMail]))
	if !errors.Is(err, migrate.ErrSinPermiso) || otro.writes != 2 { // el buzón y un solo correo
		t.Fatalf("debe parar al primer rechazo: err=%v, escrituras=%d", err, otro.writes)
	}
	if got, _ := st.get(j.ID); got.Status != domain.MigrationError {
		t.Fatalf("estado %s", got.Status)
	}
}
