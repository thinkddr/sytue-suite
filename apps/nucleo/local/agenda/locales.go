// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package agenda

import (
	"bytes"
	"context"
	"errors"
	"strings"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-vcard"
	"github.com/google/uuid"

	privateCalendar "git.sytue.eu/sytue/one/apps/nucleo/calendar"
	privateContacts "git.sytue.eu/sytue/one/apps/nucleo/contacts"
)

// Los almacenes de aquí: el repositorio de Calendar y el de Contacts, los MISMOS que
// usan sus pantallas (`apps/nucleo/calendar`, `apps/nucleo/contacts`), por los métodos
// que tienen para esto (`externos.go`).

// LocalCalendario es [Local] sobre el repositorio de Calendar.
type LocalCalendario struct{ Repo *privateCalendar.Repository }

// Crear da de alta un calendario de aquí para uno de la cuenta.
func (l LocalCalendario) Crear(ctx context.Context, nombre, color string, soloLectura bool) (string, error) {
	c, err := l.Repo.CrearCalendarioExterno(ctx, nombre, color, soloLectura)
	return c.ID, err
}

// Actualizar pone el nombre, el color y la solo lectura de la cuenta.
func (l LocalCalendario) Actualizar(ctx context.Context, id, nombre, color string, soloLectura bool) error {
	return l.Repo.ActualizarCalendarioExterno(ctx, id, nombre, color, soloLectura)
}

// Existe dice si el calendario sigue aquí.
func (l LocalCalendario) Existe(ctx context.Context, id string) (bool, error) {
	return l.Repo.ExisteCalendario(ctx, id)
}

// Objetos son los eventos del calendario.
func (l LocalCalendario) Objetos(ctx context.Context, col string) ([]ObjetoLocal, error) {
	os, err := l.Repo.ObjetosCrudos(ctx, col)
	out := make([]ObjetoLocal, 0, len(os))
	for _, o := range os {
		out = append(out, ObjetoLocal(o))
	}
	return out, err
}

// Poner guarda un evento.
func (l LocalCalendario) Poner(ctx context.Context, col, id, esperado, datos string) (ObjetoLocal, error) {
	o, err := l.Repo.PonerObjetoCrudo(ctx, col, id, esperado, datos)
	if errors.Is(err, privateCalendar.ErrCambioLocal) {
		err = ErrCambioLocal
	}
	return ObjetoLocal(o), err
}

// Borrar quita un evento.
func (l LocalCalendario) Borrar(ctx context.Context, id, esperado string) error {
	err := l.Repo.BorrarObjetoCrudo(ctx, id, esperado)
	if errors.Is(err, privateCalendar.ErrCambioLocal) {
		err = ErrCambioLocal
	}
	return err
}

// Copia es el evento con un UID nuevo y la marca delante del título.
func (LocalCalendario) Copia(datos, marca string) (string, error) { return CopiaICS(datos, marca) }

// CopiaICS hace de un VCALENDAR uno nuevo: UID nuevo en todos sus eventos (el maestro y
// sus excepciones) y la marca delante del título.
func CopiaICS(datos, marca string) (string, error) {
	cal, err := ical.NewDecoder(strings.NewReader(datos)).Decode()
	if err != nil {
		return "", err
	}
	uid := uuid.NewString()
	for _, c := range cal.Children {
		if c.Name != ical.CompEvent {
			continue
		}
		c.Props.SetText(ical.PropUID, uid)
		if marca != "" {
			resumen, _ := c.Props.Text(ical.PropSummary)
			c.Props.SetText(ical.PropSummary, strings.TrimSpace(marca+" "+resumen))
		}
	}
	var b bytes.Buffer
	if err := ical.NewEncoder(&b).Encode(cal); err != nil {
		return "", err
	}
	return b.String(), nil
}

// LocalContactos es [Local] sobre la libreta de Contacts: la SQLite de Sytue Office
// (`local/libreta`) o el repositorio de Contacts sobre el motor. Los grupos de una
// libreta (KIND:group) son sus etiquetas: van con su id de aquí tras «grupo:».
type LocalContactos struct {
	Repo privateContacts.LibretasExternas
}

const prefijoGrupo = "grupo:"

// Crear da de alta una libreta de aquí para una de la cuenta. Contacts no tiene
// libretas de solo lectura: lo que se escriba en una así se queda en este equipo.
func (l LocalContactos) Crear(ctx context.Context, nombre, _ string, _ bool) (string, error) {
	b, err := l.Repo.CrearLibretaExterna(ctx, nombre)
	return b.ID, err
}

// Actualizar cambia el nombre de la libreta.
func (l LocalContactos) Actualizar(ctx context.Context, id, nombre, _ string, _ bool) error {
	if strings.TrimSpace(nombre) == "" {
		return nil
	}
	_, err := l.Repo.RenameAddressBook(ctx, id, nombre)
	return err
}

// Existe dice si la libreta sigue aquí.
func (l LocalContactos) Existe(ctx context.Context, id string) (bool, error) {
	return l.Repo.ExisteLibreta(ctx, id)
}

// Objetos son los contactos de la libreta y sus etiquetas.
func (l LocalContactos) Objetos(ctx context.Context, col string) ([]ObjetoLocal, error) {
	ts, err := l.Repo.TarjetasCrudas(ctx, col)
	if err != nil {
		return nil, err
	}
	gs, err := l.Repo.GruposCrudos(ctx, col)
	if err != nil {
		return nil, err
	}
	out := make([]ObjetoLocal, 0, len(ts)+len(gs))
	for _, t := range ts {
		out = append(out, ObjetoLocal(t))
	}
	for _, g := range gs {
		out = append(out, ObjetoLocal{ID: prefijoGrupo + g.ID, UID: g.UID, Datos: g.Datos})
	}
	return out, nil
}

// Poner guarda un contacto o, si la vCard es un grupo, una etiqueta.
func (l LocalContactos) Poner(ctx context.Context, col, id, esperado, datos string) (ObjetoLocal, error) {
	grupo, esGrupo := strings.CutPrefix(id, prefijoGrupo)
	if esGrupoVCard(datos) {
		if id != "" && !esGrupo {
			return ObjetoLocal{}, errors.New("agenda: un contacto no puede pasar a ser un grupo")
		}
		t, err := l.Repo.PonerGrupoCrudo(ctx, col, grupo, esperado, grupoV4(datos))
		if errors.Is(err, privateContacts.ErrCambioLocal) {
			err = ErrCambioLocal
		}
		return ObjetoLocal{ID: prefijoGrupo + t.ID, UID: t.UID, Datos: t.Datos}, err
	}
	if esGrupo {
		return ObjetoLocal{}, errors.New("agenda: un grupo no puede pasar a ser un contacto")
	}
	t, err := l.Repo.PonerTarjetaCruda(ctx, col, id, esperado, datos)
	if errors.Is(err, privateContacts.ErrCambioLocal) {
		err = ErrCambioLocal
	}
	return ObjetoLocal(t), err
}

// Borrar manda un contacto a la papelera, o borra una etiqueta.
func (l LocalContactos) Borrar(ctx context.Context, id, esperado string) error {
	var err error
	if grupo, ok := strings.CutPrefix(id, prefijoGrupo); ok {
		err = l.Repo.BorrarGrupoCrudo(ctx, grupo, esperado)
	} else {
		err = l.Repo.BorrarTarjetaCruda(ctx, id, esperado)
	}
	if errors.Is(err, privateContacts.ErrCambioLocal) {
		err = ErrCambioLocal
	}
	return err
}

// Copia es el contacto con un UID nuevo y la marca detrás del nombre.
func (LocalContactos) Copia(datos, marca string) (string, error) { return CopiaVCard(datos, marca) }

// CopiaVCard hace de una vCard una nueva: UID nuevo y la marca detrás del nombre.
func CopiaVCard(datos, marca string) (string, error) {
	card, err := vcard.NewDecoder(strings.NewReader(datos)).Decode()
	if err != nil {
		return "", err
	}
	card.SetValue(vcard.FieldUID, "urn:uuid:"+uuid.NewString())
	if marca != "" {
		card.SetValue(vcard.FieldFormattedName, strings.TrimSpace(card.Value(vcard.FieldFormattedName)+" "+marca))
	}
	var b bytes.Buffer
	if err := vcard.NewEncoder(&b).Encode(card); err != nil {
		return "", err
	}
	return b.String(), nil
}

// conUID pone uid a un VCALENDAR (en todos sus eventos) o a una vCard.
func conUID(datos, uid string) (string, error) {
	var b bytes.Buffer
	if strings.Contains(datos[:min(len(datos), 64)], "VCALENDAR") {
		cal, err := ical.NewDecoder(strings.NewReader(datos)).Decode()
		if err != nil {
			return "", err
		}
		for _, c := range cal.Children {
			if c.Name == ical.CompEvent {
				c.Props.SetText(ical.PropUID, uid)
			}
		}
		err = ical.NewEncoder(&b).Encode(cal)
		return b.String(), err
	}
	card, err := vcard.NewDecoder(strings.NewReader(datos)).Decode()
	if err != nil {
		return "", err
	}
	card.SetValue(vcard.FieldUID, uid)
	err = vcard.NewEncoder(&b).Encode(card)
	return b.String(), err
}

// uidDe es el UID de un VCALENDAR o de una vCard (sin «urn:uuid:»).
func uidDe(datos string) string {
	if strings.Contains(datos[:min(len(datos), 64)], "VCALENDAR") {
		cal, err := ical.NewDecoder(strings.NewReader(datos)).Decode()
		if err != nil {
			return ""
		}
		for _, ev := range cal.Events() {
			if uid, err := ev.Props.Text(ical.PropUID); err == nil && uid != "" {
				return uid
			}
		}
		return ""
	}
	card, err := vcard.NewDecoder(strings.NewReader(datos)).Decode()
	if err != nil {
		return ""
	}
	uid := strings.TrimSpace(card.Value(vcard.FieldUID))
	if strings.HasPrefix(strings.ToLower(uid), "urn:uuid:") {
		uid = uid[len("urn:uuid:"):]
	}
	return uid
}
