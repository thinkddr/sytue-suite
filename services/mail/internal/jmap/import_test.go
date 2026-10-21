// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package jmap

// Tests de Email/import: la puerta por la que entra un buzón que se muda desde
// otro proveedor. Lo que se prueba es lo que hace creíble una migración de
// correo: que la FECHA y el ESTADO son los del original —no los del día de la
// importación— y que un identificador de buzón ajeno no cuela un mensaje.

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

// mensaje de ejemplo, con su fecha real en la cabecera.
const rawImported = "From: Ana <ana@old.example>\r\n" +
	"To: grace@acme.test\r\n" +
	"Subject: Presupuesto 2024\r\n" +
	"Message-ID: <abc@old.example>\r\n" +
	"Date: Fri, 01 Mar 2024 09:12:00 +0000\r\n" +
	"\r\n" +
	"Te paso el presupuesto.\r\n"

func TestImportConservaFechaYEstadoOriginales(t *testing.T) {
	t.Parallel()
	acc := domain.Account{ID: uuid.New()}
	mailbox := uuid.New()
	st := &fakeStore{byRole: domain.Mailbox{ID: mailbox, AccountID: acc.ID, Name: "Inbox"}}
	blobs := &fakeBlobs{getBody: rawImported}
	api := newAPI(st, blobs, &fakeOutbound{})

	recibido := "2024-03-01T09:12:00Z"
	out, err := api.emailImport(req(http.MethodPost, "/jmap", "", true, nil), acc, toRawArgs(map[string]any{
		"emails": map[string]any{
			"c1": map[string]any{
				"blobId":     "blobs/" + acc.ID.String() + "/uno",
				"mailboxIds": map[string]any{mailbox.String(): true},
				"keywords":   map[string]any{"$seen": true},
				"receivedAt": recibido,
			},
		},
	}))
	if err != nil {
		t.Fatalf("emailImport: %v", err)
	}
	res, _ := out.(map[string]any)
	created, _ := res["created"].(map[string]any)
	if len(created) != 1 {
		t.Fatalf("creados = %v, notCreated = %v", created, res["notCreated"])
	}

	got := st.lastInsert
	if got == nil {
		t.Fatal("el mensaje no se persistió")
	}
	if !got.ReceivedAt.Equal(time.Date(2024, 3, 1, 9, 12, 0, 0, time.UTC)) {
		t.Errorf("fecha de recepción = %s; la migración debe conservar la ORIGINAL", got.ReceivedAt)
	}
	if !got.HasFlag(domain.FlagSeen) {
		t.Error("el mensaje venía leído en el origen y llegó como no leído")
	}
	if got.Subject != "Presupuesto 2024" || got.FromAddr != "ana@old.example" {
		t.Errorf("el sobre no se parseó del .eml: %+v", got)
	}
	if got.MailboxID != mailbox {
		t.Errorf("aterrizó en el buzón %s en vez de en el pedido", got.MailboxID)
	}
	// La clave definitiva es la COMPARTIDA por contenido: el mismo correo migrado
	// por dos organizaciones ocupa un objeto.
	if !strings.HasPrefix(got.BlobKey, "shared/mensajes/") || !strings.HasSuffix(got.BlobKey, ".eml") {
		t.Errorf("el .eml no se copió a su clave definitiva: %q", got.BlobKey)
	}
	if blobs.deletedKey == "" {
		t.Error("el blob temporal de subida se quedó ocupando espacio")
	}
}

// TestImportSinFechaUsaLaDelMensaje: si quien importa no dice la fecha, se toma
// la del propio correo. Un buzón migrado tiene que leerse en su orden real.
func TestImportSinFechaUsaLaDelMensaje(t *testing.T) {
	t.Parallel()
	acc := domain.Account{ID: uuid.New()}
	mailbox := uuid.New()
	st := &fakeStore{byRole: domain.Mailbox{ID: mailbox, AccountID: acc.ID}}
	api := newAPI(st, &fakeBlobs{getBody: rawImported}, &fakeOutbound{})

	if _, err := api.emailImport(req(http.MethodPost, "/jmap", "", true, nil), acc, toRawArgs(map[string]any{
		"emails": map[string]any{"c1": map[string]any{
			"blobId":     "blobs/" + acc.ID.String() + "/uno",
			"mailboxIds": map[string]any{mailbox.String(): true},
		}},
	})); err != nil {
		t.Fatalf("emailImport: %v", err)
	}
	if st.lastInsert == nil || st.lastInsert.ReceivedAt.Year() != 2024 {
		t.Fatalf("no se usó la fecha del mensaje: %+v", st.lastInsert)
	}
	// Sin keywords, el correo entra SIN leer: marcarlo leído escondería trabajo.
	if st.lastInsert.HasFlag(domain.FlagSeen) {
		t.Error("un mensaje importado sin keywords no puede llegar marcado como leído")
	}
}

// TestImportRechazaArgumentosImposibles: sin buzón destino y con blobId fuera del
// prefijo de blobs, no se importa nada. El segundo caso es el que evita que un
// identificador manipulado lea una clave arbitraria del almacenamiento.
func TestImportRechazaArgumentosImposibles(t *testing.T) {
	t.Parallel()
	acc := domain.Account{ID: uuid.New()}
	st := &fakeStore{byRole: domain.Mailbox{ID: uuid.New(), AccountID: acc.ID}}
	api := newAPI(st, &fakeBlobs{getBody: rawImported}, &fakeOutbound{})

	cases := map[string]map[string]any{
		"sin buzón":       {"blobId": "blobs/x/uno"},
		"blob con escape": {"blobId": "../../etc/passwd", "mailboxIds": map[string]any{uuid.NewString(): true}},
	}
	for name, email := range cases {
		t.Run(name, func(t *testing.T) {
			out, err := api.emailImport(req(http.MethodPost, "/jmap", "", true, nil), acc, toRawArgs(map[string]any{
				"emails": map[string]any{"c1": email},
			}))
			if err != nil {
				t.Fatalf("la llamada entera falló en vez de rechazar ese mensaje: %v", err)
			}
			res, _ := out.(map[string]any)
			if created, _ := res["created"].(map[string]any); len(created) != 0 {
				t.Fatalf("se importó algo que no debía: %v", created)
			}
			if notCreated, _ := res["notCreated"].(map[string]any); len(notCreated) != 1 {
				t.Fatalf("el rechazo no se reportó: %v", res)
			}
		})
	}
	if st.lastInsert != nil {
		t.Fatal("no debía persistirse ningún mensaje")
	}
}

// TestImportAcotaElLote: una sola llamada no puede pedir un número arbitrario de
// mensajes; quien migra trocea.
func TestImportAcotaElLote(t *testing.T) {
	t.Parallel()
	acc := domain.Account{ID: uuid.New()}
	api := newAPI(&fakeStore{}, &fakeBlobs{}, &fakeOutbound{})

	emails := map[string]any{}
	for i := 0; i <= maxImportPerCall; i++ {
		emails[uuid.NewString()] = map[string]any{"blobId": "blobs/x/y"}
	}
	if _, err := api.emailImport(req(http.MethodPost, "/jmap", "", true, nil), acc, toRawArgs(map[string]any{"emails": emails})); err == nil {
		t.Fatal("un lote por encima del tope debía rechazarse entero")
	}
}
