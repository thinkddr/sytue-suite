// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package intents

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/hub/internal/domain"
)

// TestCallChannel cubre el intent call-channel: compone el deep-link esperado a
// Sytue Rooms (host de Chat) y exige un channel_id (o primer ID) para resolverse.
func TestCallChannel(t *testing.T) {
	// La base con barra final se normaliza en NewResolver (no debe duplicarse).
	r := NewResolver(Targets{Chat: "https://chat.sytue.localhost/"})

	const want = "https://chat.sytue.localhost/?call=c-42"

	// channel_id explícito -> deep-link a Rooms con el canal en `call`.
	res, err := r.Resolve(domain.IntentRequest{
		Type:   domain.IntentCallChannel,
		Params: map[string]string{"channel_id": "c-42"},
	})
	if err != nil {
		t.Fatalf("call-channel válido: %v", err)
	}
	if res.URL != want {
		t.Errorf("URL = %q, quiero %q", res.URL, want)
	}
	if res.Target != domain.SourceChat {
		t.Errorf("Target = %q, quiero %q", res.Target, domain.SourceChat)
	}
	if res.Label == "" {
		t.Errorf("Label vacío")
	}

	// El id también puede llegar como primer elemento de IDs.
	if res, err := r.Resolve(domain.IntentRequest{
		Type: domain.IntentCallChannel, IDs: []string{"c-42"},
	}); err != nil || res.URL != want {
		t.Errorf("call-channel por IDs: url=%q err=%v", res.URL, err)
	}

	// Sin channel_id -> ErrInvalidInput.
	if _, err := r.Resolve(domain.IntentRequest{Type: domain.IntentCallChannel}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("sin channel_id: err = %v, quiero ErrInvalidInput", err)
	}
}

// TestCreateEventFromMail cubre el intent create-event-from-mail: compone el
// deep-link al compositor de Calendar prerellenado desde un correo (asunto →
// título, cuerpo → descripción, participantes → invitados) y exige un message_id.
func TestCreateEventFromMail(t *testing.T) {
	r := NewResolver(Targets{
		Calendar: "https://calendar.sytue.localhost",
		Mail:     "https://mail.sytue.localhost",
	})

	res, err := r.Resolve(domain.IntentRequest{
		Type: domain.IntentCreateEventFromMail,
		Params: map[string]string{
			"message_id": "m-7",
			"subject":    "Sync semanal",
			"body":       "Puntos a tratar",
			"attendees":  "ana@sytue.eu,beto@sytue.eu",
		},
	})
	if err != nil {
		t.Fatalf("create-event válido: %v", err)
	}
	if res.Target != domain.SourceCalendar {
		t.Errorf("Target = %q, quiero %q", res.Target, domain.SourceCalendar)
	}
	// El deep-link va al compositor de Calendar con los campos prerellenados,
	// incluidos los invitados (comprobamos que cada parámetro viaja).
	for _, want := range []string{
		"https://calendar.sytue.localhost/events/new?",
		"action=create",
		"from_mail=m-7",
		"title=Sync+semanal",
		"description=Puntos+a+tratar",
		"attendees=ana%40sytue.eu%2Cbeto%40sytue.eu",
	} {
		if !strings.Contains(res.URL, want) {
			t.Errorf("URL %q no contiene %q", res.URL, want)
		}
	}
	// El enlace de origen (correo) acompaña el resultado para volver al mensaje.
	if len(res.Links) != 1 || res.Links[0].Source != domain.SourceMail {
		t.Errorf("Links = %+v, quiero un enlace al correo de origen", res.Links)
	}

	// Sin message_id (ni IDs) -> ErrInvalidInput.
	if _, err := r.Resolve(domain.IntentRequest{Type: domain.IntentCreateEventFromMail}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("sin message_id: err = %v, quiero ErrInvalidInput", err)
	}
}

// TestAttachDriveToMailLlevaAUnaPaginaQueExiste: el enlace va a una página que Mail
// TIENE. Iba a `/compose`, que no existe (404 en producción): se comprueba contra las
// páginas de la web, no contra una cadena escrita aquí.
func TestAttachDriveToMailLlevaAUnaPaginaQueExiste(t *testing.T) {
	r := NewResolver(Targets{Mail: "https://mail.sytue.localhost/mail", Drive: "https://drive.sytue.localhost/drive"})
	res, err := r.Resolve(domain.IntentRequest{
		Type: domain.IntentAttachDriveToMail, IDs: []string{"f1", " "}, Params: map[string]string{"subject": "Acta"},
	})
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(res.URL)
	if err != nil {
		t.Fatal(err)
	}
	if q := u.Query(); q.Get("action") != "compose" || strings.Join(q["attach_drive"], ",") != "f1" || q.Get("subject") != "Acta" {
		t.Errorf("el redactor recibe %v", q)
	}
	pagina := strings.TrimPrefix(strings.TrimPrefix(u.Path, "/mail"), "/")
	if pagina == "" {
		pagina = "index"
	}
	if _, err := os.Stat(filepath.Join("..", "..", "..", "..", "apps", "frontend", "apps", "mail", "src", "pages", pagina+".astro")); err != nil {
		t.Errorf("el enlace lleva a %s y Mail no tiene esa página: %v", u.Path, err)
	}
}
