// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"context"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// La libreta «Organización»: `POST /api/directorio` del BFF de la web, que aquí no hay.
// Se trae la agenda de Sytue ID y se crea en Contactos a quien falte, en una libreta
// propia y separada de los contactos de siempre.
//
// LO QUE YA HAY NO SE TOCA: el móvil o la nota que alguien añadió a la ficha de un
// compañero son suyos, y reescribir los campos en cada pasada los borraría. A uno mismo
// no se añade: la agenda es con quién trabajas, no un espejo.

// libretaOrganizacion es su ruta CardDAV: fija en cualquier idioma, es parte de la URL.
const libretaOrganizacion = "organizacion"

// colorOrganizacion es el color con que la estrena la web.
const colorOrganizacion = "#8A6A2F"

// nombreOrganizacion es `nombreVisible` del BFF: el rótulo con que se ESTRENA, en la
// lengua de quien la crea. Después es suyo: si la renombra, se respeta.
func nombreOrganizacion(idioma string) string {
	switch idioma {
	case "es":
		return "Organización"
	case "it":
		return "Organizzazione"
	default: // en, fr, de y cualquier otro
		return "Organisation"
	}
}

// correosDe son las direcciones de una persona, la principal delante y sin repetidas.
func correosDe(m api.Miembro) []string {
	var fuera []string
	vistos := map[string]bool{}
	for _, c := range append([]string{m.Email}, m.Emails...) {
		if l := strings.ToLower(c); l != "" && !vistos[l] {
			vistos[l] = true
			fuera = append(fuera, c)
		}
	}
	return fuera
}

// sincronizarOrganizacion pone al día la libreta y dice cuántas fichas ha creado. Una
// ficha que falla no deja fuera a las demás.
//
// ponytail: una sola página de la agenda (500, el tope de Sytue ID), como la web; una
// organización mayor pide paginar en el servicio.
func sincronizarOrganizacion(ctx context.Context, svc *api.Contacts, dir *api.Directorio, yo, idioma string) (int, error) {
	gente, err := dir.Agenda(ctx, 500)
	if err != nil {
		return 0, err
	}
	libretas, err := svc.AddressBooks(ctx)
	if err != nil {
		return 0, err
	}
	var libreta api.AddressBook
	for _, b := range libretas {
		if b.Name == libretaOrganizacion {
			libreta = b
		}
	}
	if libreta.ID == "" {
		if libreta, err = svc.CreateAddressBookAt(ctx, libretaOrganizacion, nombreOrganizacion(idioma), colorOrganizacion); err != nil {
			return 0, err
		}
	}
	// Quién está ya, por correo: es lo que identifica a una persona entre el directorio y
	// una ficha ya creada.
	fichas, err := svc.InBook(ctx, libreta.ID)
	if err != nil {
		return 0, err
	}
	ya := map[string]bool{}
	for _, f := range fichas {
		for _, e := range f.Emails {
			ya[strings.ToLower(e.Email)] = true
		}
	}
	creados := 0
	for _, m := range gente {
		correos := correosDe(m)
		if m.ID == yo || m.Email == "" || estaYa(correos, ya) {
			continue
		}
		in := api.ContactInput{
			FormattedName: strings.TrimSpace(m.Nombre), Organization: m.Departamento, Title: m.Puesto,
		}
		if in.FormattedName == "" {
			in.FormattedName = correos[0]
		}
		// TODAS sus direcciones: quien busca a un compañero por un alias tiene que
		// encontrarlo igual.
		for i, c := range correos {
			in.Emails = append(in.Emails, api.ContactEmail{Email: c, Type: "work", IsPrimary: i == 0})
		}
		if _, err := svc.Create(ctx, libreta.ID, in); err == nil {
			creados++
			for _, correo := range correos {
				ya[strings.ToLower(correo)] = true
			}
		}
	}
	return creados, nil
}

// estaYa es que YA está cualquiera de sus direcciones: la persona es la misma, y
// crearla otra vez la duplicaría con otro alias delante.
func estaYa(correos []string, ya map[string]bool) bool {
	for _, c := range correos {
		if ya[strings.ToLower(c)] {
			return true
		}
	}
	return false
}
