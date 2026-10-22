// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package migrate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/portabilidad"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
)

// Salida SIN delegación: se lleva a un tercero lo que Sytue entrega para «Tus
// datos» (platform/pkg/portabilidad). La entrega la abrió quien lanzó el trabajo,
// con su propio token, así que aquí no hace falta ninguna credencial de Sytue: vale
// igual para una cuenta personal, para quien administra y para una organización sin
// client delegable.

// MailExporter es el destino de SALIDA del correo.
type MailExporter interface {
	// MailFolder devuelve (o crea) dónde van los correos de un buzón de Sytue: su
	// ruta («Clientes/Acme») y su papel (inbox, sent, drafts, trash, junk, archive o
	// vacío, que es una carpeta de la persona).
	MailFolder(ctx context.Context, ruta, rol string) (string, error)
	// ImportMessage importa un .eml tal cual. size es -1 si no se sabe.
	ImportMessage(ctx context.Context, folderID string, size int64, raw io.Reader) (string, error)
}

// CalendarExporter es el destino de SALIDA de la agenda.
type CalendarExporter interface {
	CreateCalendar(ctx context.Context, nombre string) (string, error)
	ImportEvent(ctx context.Context, calendarID string, ev EventoSalida) (string, error)
}

// EventoSalida es un evento tal como lo entrega Sytue Calendar (events.json).
type EventoSalida struct {
	UID          string       `json:"uid"`
	CalendarID   string       `json:"calendar_id"`
	Summary      string       `json:"summary"`
	Description  string       `json:"description"`
	Location     string       `json:"location"`
	DTStart      time.Time    `json:"dtstart"`
	DTEnd        time.Time    `json:"dtend"`
	TZID         string       `json:"tzid"`
	AllDay       bool         `json:"all_day"`
	Status       string       `json:"status"`
	RRule        string       `json:"rrule"`
	RDate        []time.Time  `json:"rdate"`
	ExDate       []time.Time  `json:"exdate"`
	RecurrenceID *time.Time   `json:"recurrence_id"`
	Attendees    []asistente  `json:"attendees"`
	extra        []*time.Time // cambios de una sola repetición: van como EXDATE de la serie.
}

type asistente struct {
	Email      string `json:"email"`
	CommonName string `json:"common_name"`
}

// RunEntregas ejecuta (o continúa) un trabajo de salida leyendo la entrega del
// producto que le toca. Mismas garantías que Run: lo que ya está en el mapa no se
// vuelve a escribir, lo que no entra se omite con su motivo y un fallo puntual se
// cuenta y se reintenta al volver a lanzarlo.
func RunEntregas(ctx context.Context, d Deps, job domain.MigrationJob, entrega *http.Response) error {
	defer entrega.Body.Close()
	if err := d.Store.StartMigrationJob(ctx, job.ID); err != nil {
		return err
	}
	counts := job.Counts
	counts.Errors = 0
	r := &run{d: d, job: job, counts: counts, done: map[string]struct{}{}}
	entries, err := d.Store.LoadMigrationMap(ctx, job.ID)
	if err != nil {
		return d.finish(ctx, job.ID, domain.MigrationError, err)
	}
	s := &salida{run: r, ids: make(map[string]string, len(entries)), roles: map[string]string{}, calendarios: map[string]string{}}
	for _, e := range entries {
		r.done[e.SourceID] = struct{}{}
		s.ids[e.SourceID] = e.SytueID
	}

	for parte, err := range portabilidad.Leer(entrega) {
		if err == nil {
			err = s.parte(ctx, parte)
		}
		if err == nil && len(r.batch) >= batchItems {
			err = r.checkpoint(ctx, "")
		}
		if err != nil {
			return r.cerrar(ctx, err)
		}
	}
	err = s.eventos(ctx)
	if err == nil {
		err = r.checkpoint(ctx, "")
	}
	return r.cerrar(ctx, err)
}

// salida es el estado de una ejecución de RunEntregas.
type salida struct {
	*run
	// ids traduce una clave del mapa (carpeta, buzón, calendario) al id en el destino.
	ids         map[string]string
	roles       map[string]string // ruta del buzón → papel
	calendarios map[string]string // id del calendario en Sytue → nombre
	pendientes  []EventoSalida
}

func (s *salida) parte(ctx context.Context, p portabilidad.Parte) error {
	switch s.job.Domain {
	case domain.DomainDrive:
		if rel, ok := strings.CutPrefix(p.Ruta, "files/"); ok {
			return s.archivo(ctx, rel, p)
		}
	case domain.DomainContacts:
		if p.Ruta == "contacts.json" {
			return filas(p, func(c contactoSalida) error { return s.contacto(ctx, c) })
		}
	case domain.DomainMail:
		switch rel, ok := strings.CutPrefix(p.Ruta, "messages/"); {
		case p.Ruta == "mailboxes.json":
			return filas(p, func(b buzonSalida) error { s.roles[b.Path] = b.Role; return nil })
		case ok && strings.HasSuffix(rel, ".eml"):
			return s.correo(ctx, rel, p)
		}
	case domain.DomainCalendar:
		switch p.Ruta {
		case "calendars.json":
			return filas(p, func(c calendarioSalida) error {
				s.calendarios[c.ID] = firstNonEmpty(c.DisplayName, c.Name)
				return nil
			})
		case "events.json":
			return filas(p, func(ev EventoSalida) error {
				ev.RRule = strings.TrimPrefix(ev.RRule, "RRULE:") // Sytue la guarda con el prefijo; Google y Graph, sin él.
				s.pendientes = append(s.pendientes, ev)
				return nil
			})
		}
	default:
		return fmt.Errorf("migrate: la salida no lleva %q", s.job.Domain)
	}
	return nil
}

// filas recorre una tabla JSON de la entrega fila a fila, sin cargarla entera.
func filas[T any](r io.Reader, cada func(T) error) error {
	dec := json.NewDecoder(r)
	if _, err := dec.Token(); err != nil {
		return fmt.Errorf("migrate: tabla ilegible: %w", err)
	}
	for dec.More() {
		var fila T
		if err := dec.Decode(&fila); err != nil {
			return fmt.Errorf("migrate: fila ilegible: %w", err)
		}
		if err := cada(fila); err != nil {
			return err
		}
	}
	return nil
}

// buzonSalida y calendarioSalida son lo que hace falta de mailboxes.json y
// calendars.json para saber adónde va cada correo y cada evento.
type (
	buzonSalida struct {
		Path string `json:"path"`
		Role string `json:"role"`
	}
	calendarioSalida struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		DisplayName string `json:"display_name"`
	}
)

// fallo cuenta un error puntual: no se apunta en el mapa, así que volver a lanzar
// el trabajo lo reintenta. Un destino que rechaza la escritura NO es puntual: se
// devuelve y el trabajo para, en vez de leer gigas de Sytue para fallar en cada uno.
func (s *salida) fallo(err error) error {
	if errors.Is(err, ErrSinPermiso) {
		return err
	}
	s.counts.Errors++
	s.job.LastError = err.Error()
	return nil
}

func (s *salida) archivo(ctx context.Context, rel string, p portabilidad.Parte) error {
	clave := "archivo:" + rel
	if _, ok := s.done[clave]; ok {
		return nil
	}
	carpeta, err := s.carpeta(ctx, path.Dir(rel))
	if err != nil {
		return s.fallo(err)
	}
	nombre := path.Base(rel)
	tipo := mime.TypeByExtension(path.Ext(nombre))
	var id string
	if su, ok := s.d.Dst.(SizedUploader); ok && p.Tamano >= 0 {
		id, err = su.UploadFileSized(ctx, carpeta, nombre, tipo, p.Tamano, p)
	} else {
		id, err = s.d.Dst.UploadFile(ctx, carpeta, nombre, tipo, p)
	}
	switch {
	case errors.Is(err, ErrUnimportable):
		s.skip(clave, domain.KindFile, rel, err.Error())
	case err != nil:
		return s.fallo(fmt.Errorf("migrate: subir %q: %w", rel, err))
	default:
		s.record(domain.MigrationItem{SourceID: clave, Kind: domain.KindFile, SytueID: id, Path: rel})
		s.counts.Bytes += max(p.Tamano, 0)
	}
	return nil
}

// carpeta crea (una vez) la cadena de carpetas de una ruta bajo la raíz del destino.
func (s *salida) carpeta(ctx context.Context, ruta string) (string, error) {
	if ruta == "." || ruta == "" {
		return s.id("carpeta:", domain.KindFolder, func() (string, error) { return s.d.Dst.Root(ctx) })
	}
	padre, err := s.carpeta(ctx, path.Dir(ruta))
	if err != nil {
		return "", err
	}
	return s.id("carpeta:"+ruta, domain.KindFolder, func() (string, error) {
		return s.d.Dst.CreateFolder(ctx, padre, path.Base(ruta))
	})
}

// id devuelve el id en el destino de algo que se crea una sola vez (carpeta, buzón,
// calendario) y lo apunta en el mapa: volver a lanzar el trabajo no lo duplica. No
// cuenta como copiado: una carpeta no es contenido.
func (s *salida) id(clave string, kind domain.MigrationKind, crear func() (string, error)) (string, error) {
	if id := s.ids[clave]; id != "" {
		return id, nil
	}
	id, err := crear()
	if err != nil {
		return "", err
	}
	s.ids[clave] = id
	s.done[clave] = struct{}{}
	s.batch = append(s.batch, domain.MigrationItem{SourceID: clave, Kind: kind, SytueID: id, Path: clave})
	return id, nil
}

// contactoSalida es un contacto tal como lo entrega Sytue Contacts (contacts.json).
type contactoSalida struct {
	UID           string `json:"uid"`
	FormattedName string `json:"formatted_name"`
	GivenName     string `json:"given_name"`
	FamilyName    string `json:"family_name"`
	Organization  string `json:"organization"`
	Title         string `json:"title"`
	Note          string `json:"note"`
	Emails        []struct {
		Email string `json:"email"`
	} `json:"emails"`
	Phones []struct {
		Phone string `json:"phone"`
	} `json:"phones"`
}

func (s *salida) contacto(ctx context.Context, c contactoSalida) error {
	clave := "contacto:" + c.UID
	if _, ok := s.done[clave]; ok {
		return nil
	}
	libro, err := s.id("agenda:", domain.KindContact, func() (string, error) { return s.d.Dst.AddressBook(ctx) })
	if err != nil {
		return s.fallo(err)
	}
	mc := Contact{
		FormattedName: c.FormattedName, GivenName: c.GivenName, FamilyName: c.FamilyName,
		Organization: c.Organization, Title: c.Title, Note: c.Note,
	}
	for _, e := range c.Emails {
		mc.Emails = append(mc.Emails, e.Email)
	}
	for _, t := range c.Phones {
		mc.Phones = append(mc.Phones, t.Phone)
	}
	id, err := s.d.Dst.CreateContact(ctx, libro, mc)
	switch {
	case errors.Is(err, ErrUnimportable):
		s.skip(clave, domain.KindContact, c.FormattedName, err.Error())
	case err != nil:
		return s.fallo(fmt.Errorf("migrate: contacto %q: %w", c.FormattedName, err))
	default:
		s.record(domain.MigrationItem{SourceID: clave, Kind: domain.KindContact, SytueID: id, Path: c.FormattedName})
	}
	return nil
}

func (s *salida) correo(ctx context.Context, rel string, p portabilidad.Parte) error {
	dst, ok := s.d.Dst.(MailExporter)
	if !ok {
		return errors.New("migrate: este destino no recibe correo")
	}
	clave := "correo:" + rel
	if _, ok := s.done[clave]; ok {
		return nil
	}
	buzon := path.Dir(rel)
	carpeta, err := s.id("buzon:"+buzon, domain.KindMailbox, func() (string, error) {
		return dst.MailFolder(ctx, buzon, s.roles[buzon])
	})
	if err != nil {
		return s.fallo(err)
	}
	id, err := dst.ImportMessage(ctx, carpeta, p.Tamano, p)
	switch {
	case errors.Is(err, ErrUnimportable):
		s.skip(clave, domain.KindMessage, rel, err.Error())
	case err != nil:
		return s.fallo(fmt.Errorf("migrate: correo %q: %w", rel, err))
	default:
		s.record(domain.MigrationItem{SourceID: clave, Kind: domain.KindMessage, SytueID: id, Path: rel})
		s.counts.Bytes += max(p.Tamano, 0)
	}
	return nil
}

// eventos importa la agenda al final de la entrega: un cambio en una sola
// repetición llega como evento aparte y la serie tiene que saberlo antes de crearse
// (esa repetición sale de la serie y queda como evento suelto).
func (s *salida) eventos(ctx context.Context) error {
	if len(s.pendientes) == 0 {
		return nil
	}
	dst, ok := s.d.Dst.(CalendarExporter)
	if !ok {
		return errors.New("migrate: este destino no recibe calendarios")
	}
	cambios := map[string][]*time.Time{}
	for _, ev := range s.pendientes {
		if ev.RecurrenceID != nil {
			cambios[ev.UID] = append(cambios[ev.UID], ev.RecurrenceID)
		}
	}
	for _, ev := range s.pendientes {
		clave := "evento:" + ev.UID
		if ev.RecurrenceID != nil {
			clave += "@" + ev.RecurrenceID.UTC().Format(time.RFC3339)
		} else {
			ev.extra = cambios[ev.UID]
		}
		if _, ok := s.done[clave]; ok {
			continue
		}
		nombre := firstNonEmpty(s.calendarios[ev.CalendarID], "Sytue")
		cal, err := s.id("calendario:"+ev.CalendarID, domain.KindCalendar, func() (string, error) {
			return dst.CreateCalendar(ctx, nombre)
		})
		if err != nil {
			if err := s.fallo(err); err != nil {
				return err
			}
			continue
		}
		id, err := dst.ImportEvent(ctx, cal, ev)
		switch {
		case errors.Is(err, ErrUnimportable):
			s.skip(clave, domain.KindEvent, ev.Summary, err.Error())
		case err != nil:
			if err := s.fallo(fmt.Errorf("migrate: evento %q: %w", ev.Summary, err)); err != nil {
				return err
			}
		default:
			s.record(domain.MigrationItem{SourceID: clave, Kind: domain.KindEvent, SytueID: id, Path: ev.Summary})
		}
		if len(s.batch) >= batchItems {
			if err := s.checkpoint(ctx, ""); err != nil {
				return err
			}
		}
	}
	return nil
}

// Excluidas son las repeticiones que NO hay que crear: las quitadas en Sytue y las
// que se cambiaron una a una (esas van como evento suelto).
func (ev EventoSalida) Excluidas() []time.Time {
	out := append([]time.Time(nil), ev.ExDate...)
	for _, t := range ev.extra {
		out = append(out, *t)
	}
	return out
}
