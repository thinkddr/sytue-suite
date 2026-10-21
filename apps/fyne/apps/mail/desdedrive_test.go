// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// driveResoluble es Drive que además sabe decir de un id su nombre, o que ya no está.
type driveResoluble struct {
	driveNavegable
	noEstan map[string]bool
}

func (d *driveResoluble) File(_ context.Context, id string) (api.File, error) {
	if d.noEstan[id] {
		return api.File{}, errors.New("404")
	}
	return api.File{ID: id, Name: "Fichero " + id}, nil
}

// «Adjuntar a un correo» de Drive llega por el esquema: el redactor se abre con el
// asunto y los ficheros que se han podido resolver, como fichas; lo que ya no está se
// deja fuera, y si no queda NINGUNO no se abre un redactor vacío: se dice.
func TestElEnlaceDeDriveAbreElRedactorConSusFicheros(t *testing.T) {
	b, _, _ := buzonConVentana(t, servicioConTodas(), fyne.NewSize(1280, 800))
	b.ConDrive(&driveResoluble{noEstan: map[string]bool{"x": true}})
	b.URLDrive = "https://drive.sytue.eu/drive"
	leer := func(r api.Redactar) api.Redactar {
		u, err := url.Parse(api.Esquema("mail") + ":" + api.RutaRedactar(r))
		if err != nil {
			t.Fatal(err)
		}
		leido, ok := api.LeerRedactar(u)
		if !ok {
			t.Fatalf("Mail no entiende %s", u)
		}
		return leido
	}

	b.RedactarDesde(leer(api.Redactar{Asunto: "Acta", Drive: []string{"x"}}))
	if b.redaccion != nil {
		t.Fatal("sin ningún fichero que se pueda ver, se ha abierto un redactor vacío")
	}
	if _, titulo := ultimoAviso(b); titulo != traductor(t).T("compose.driveIntentFailed", nil) {
		t.Errorf("no se dice que no se ha podido: %q", titulo)
	}

	b.RedactarDesde(leer(api.Redactar{Asunto: "Acta", Drive: []string{"f1", "x"}}))
	rd := b.redaccion
	if rd == nil {
		t.Fatal("no se ha abierto el redactor")
	}
	quiero := EnlaceDrive{Nombre: "Fichero f1", URL: "https://drive.sytue.eu/drive/files/f1"}
	if rd.r.B.Asunto != "Acta" || len(rd.r.B.Drive) != 1 || rd.r.B.Drive[0] != quiero {
		t.Fatalf("el redactor abre con asunto %q y Drive %+v", rd.r.B.Asunto, rd.r.B.Drive)
	}
	// Y el fichero viaja como ENLACE en el cuerpo, bajo su rótulo.
	_, s, _, err := b.componer(rd.r.B, true)
	if err != nil || !strings.Contains(s.Texto, traductor(t).T("compose.driveAttachmentsHeading", nil)+"\nFichero f1: https://drive.sytue.eu/drive/files/f1") {
		t.Errorf("el cuerpo que se guarda es %q (%v)", s.Texto, err)
	}
}
