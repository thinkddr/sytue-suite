// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package migrate_test

// Tests OFFLINE del buzón: un Gmail simulado y un Sytue Mail simulado que habla
// JMAP. Lo que se prueba es lo que hace que una mudanza de correo sea creíble:
// que cada mensaje aterriza en SU carpeta (lo enviado en Enviados, no en la
// bandeja), que conserva la fecha y el estado de leído, y que una segunda pasada
// no duplica el buzón.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/migrate"
	"git.sytue.eu/sytue/one/services/integrations/internal/policy"
)

// googleConsentInstance es una conexión hecha por CONSENTIMIENTO: sin cuenta de
// servicio, con el token que inyecta la pasarela. Es el camino del asistente de
// un solo inicio de sesión.
func googleConsentInstance(base string) connector.Instance {
	return connector.Instance{
		ID: "inst-1", OrgID: "org-1", TemplateKey: "google-workspace",
		Config: map[string]string{
			migrate.ConfigGmailBase:       base,
			migrate.ConfigGoogleDriveBase: base,
		},
		Secrets:   map[string]string{"bearer": "goog-token"},
		HasBearer: true,
	}
}

// gmailRuntime inyecta el token del consentimiento, como en producción.
func gmailRuntime() connector.Runtime {
	return policy.BuildRuntime(policy.RuntimeParams{
		OrgID: "org-1", InstanceID: "inst-1", Bearer: "goog-token", Log: slog.Default(),
	})
}

// --- Gmail simulado ----------------------------------------------------------

func gmailServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	raw := func(subject string) string {
		msg := "From: ana@old.example\r\nTo: grace@acme.test\r\nSubject: " + subject +
			"\r\nDate: Fri, 01 Mar 2024 09:12:00 +0000\r\n\r\ncuerpo\r\n"
		return base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString([]byte(msg))
	}

	mux.HandleFunc("/gmail/v1/users/me/labels", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"labels": []map[string]any{
			{"id": "INBOX", "name": "INBOX", "type": "system"},
			{"id": "SENT", "name": "SENT", "type": "system"},
			{"id": "UNREAD", "name": "UNREAD", "type": "system"},
			{"id": "CATEGORY_PROMOTIONS", "name": "CATEGORY_PROMOTIONS", "type": "system"},
			{"id": "Label_7", "name": "Trabajo/Clientes", "type": "user"},
		}})
	})
	mux.HandleFunc("/gmail/v1/users/me/messages", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"messages": []map[string]any{
			{"id": "m1"}, {"id": "m2"}, {"id": "m3"},
		}})
	})
	mux.HandleFunc("/gmail/v1/users/me/messages/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/gmail/v1/users/me/messages/")
		if r.URL.Query().Get("format") == "raw" {
			writeJSON(w, map[string]any{"id": id, "raw": raw("asunto de " + id)})
			return
		}
		meta := map[string]any{
			"id": id, "sizeEstimate": 1024, "internalDate": "1709283120000",
			"payload": map[string]any{"headers": []map[string]any{{"name": "Subject", "value": "asunto de " + id}}},
		}
		switch id {
		case "m1": // en la bandeja y SIN leer
			meta["labelIds"] = []string{"INBOX", "UNREAD", "CATEGORY_PROMOTIONS"}
		case "m2": // enviado (y por tanto leído)
			meta["labelIds"] = []string{"SENT"}
		case "m3": // etiqueta propia, leído y destacado
			meta["labelIds"] = []string{"Label_7", "STARRED"}
		}
		writeJSON(w, meta)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// --- Sytue Mail simulado (JMAP) ----------------------------------------------

type importedMail struct {
	folder     string
	keywords   map[string]bool
	receivedAt string
	body       string
}

type fakeMail struct {
	srv      *httptest.Server
	folders  map[string]string // id → nombre
	byName   map[string]string // nombre → id
	blobs    map[string]string // blobId → contenido
	imported map[string]importedMail
	nextID   int
}

func newFakeMail(t *testing.T) *fakeMail {
	t.Helper()
	f := &fakeMail{
		// El buzón de destino ya existe con sus carpetas de sistema.
		folders:  map[string]string{"mb-inbox": "Inbox", "mb-sent": "Enviados"},
		byName:   map[string]string{"Inbox": "mb-inbox", "Enviados": "mb-sent"},
		blobs:    map[string]string{},
		imported: map[string]importedMail{},
	}
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}

	mux.HandleFunc("/jmap/session", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"primaryAccounts": map[string]string{"urn:ietf:params:jmap:mail": "acc-1"},
			"accounts":        map[string]any{"acc-1": map[string]any{}},
		})
	})
	mux.HandleFunc("/jmap/upload/", func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		f.nextID++
		id := fmt.Sprintf("blobs/acc-1/%d", f.nextID)
		f.blobs[id] = string(body)
		w.WriteHeader(http.StatusCreated)
		writeJSON(w, map[string]any{"accountId": "acc-1", "blobId": id, "size": len(body)})
	})
	mux.HandleFunc("/jmap", func(w http.ResponseWriter, r *http.Request) {
		//nolint:tagliatelle // el doble habla JMAP: sus tags son el contrato del RFC.
		var req struct {
			MethodCalls [][]json.RawMessage `json:"methodCalls"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		var method string
		_ = json.Unmarshal(req.MethodCalls[0][0], &method)

		switch method {
		case "Mailbox/get":
			list := make([]map[string]any, 0, len(f.folders))
			for id, name := range f.folders {
				role := ""
				switch id {
				case "mb-inbox":
					role = "inbox"
				case "mb-sent":
					role = "sent"
				}
				list = append(list, map[string]any{"id": id, "name": name, "role": role})
			}
			writeJSON(w, map[string]any{"methodResponses": []any{
				[]any{"Mailbox/get", map[string]any{"list": list}, "c0"},
			}})
		case "Mailbox/set":
			var args struct {
				Create map[string]struct{ Name string } `json:"create"`
			}
			_ = json.Unmarshal(req.MethodCalls[0][1], &args)
			created := map[string]any{}
			for cid, c := range args.Create {
				f.nextID++
				id := fmt.Sprintf("mb-%d", f.nextID)
				f.folders[id], f.byName[c.Name] = c.Name, id
				created[cid] = map[string]any{"id": id}
			}
			writeJSON(w, map[string]any{"methodResponses": []any{
				[]any{"Mailbox/set", map[string]any{"created": created}, "c0"},
			}})
		case "Email/import":
			//nolint:tagliatelle // contrato de Email/import (RFC 8621 §2.7).
			var args struct {
				Emails map[string]struct {
					BlobID     string          `json:"blobId"`
					MailboxIDs map[string]bool `json:"mailboxIds"`
					Keywords   map[string]bool `json:"keywords"`
					ReceivedAt string          `json:"receivedAt"`
				} `json:"emails"`
			}
			_ = json.Unmarshal(req.MethodCalls[0][1], &args)
			created := map[string]any{}
			for cid, em := range args.Emails {
				folder := ""
				for id := range em.MailboxIDs {
					folder = id
				}
				f.nextID++
				id := fmt.Sprintf("msg-%d", f.nextID)
				f.imported[id] = importedMail{
					folder: f.folders[folder], keywords: em.Keywords,
					receivedAt: em.ReceivedAt, body: f.blobs[em.BlobID],
				}
				created[cid] = map[string]any{"id": id}
			}
			writeJSON(w, map[string]any{"methodResponses": []any{
				[]any{"Email/import", map[string]any{"created": created}, "c0"},
			}})
		default:
			writeJSON(w, map[string]any{"methodResponses": []any{
				[]any{"error", map[string]any{"type": "unknownMethod"}, "c0"},
			}})
		}
	})

	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

// --- Tests -------------------------------------------------------------------

// TestGmailLlegaACadaCarpetaConSuEstado: el buzón se muda entero y cada mensaje
// mantiene dónde estaba, cuándo llegó y cómo estaba.
func TestGmailLlegaACadaCarpetaConSuEstado(t *testing.T) {
	allowLoopback(t)
	gsrv := gmailServer(t)
	mail := newFakeMail(t)

	inst := googleConsentInstance(gsrv.URL)

	j := job("job-mail", domain.SourceGoogle, domain.DomainMail)
	j.SourceUser = "" // el asistente automático migra la cuenta que autorizó
	st := newMemStore("org-1")
	st.put(j)
	src, err := migrate.NewSource(context.Background(), gmailRuntime(), inst, j)
	if err != nil {
		t.Fatalf("NewSource: %v", err)
	}
	deps := migrate.Deps{
		Src:   src,
		Mail:  migrate.NewSytueMail(mail.srv.URL, migrate.StaticToken("tok"), nil),
		Store: st,
	}
	if err := migrate.Run(context.Background(), deps, j); err != nil {
		t.Fatalf("Run: %v", err)
	}

	got, _ := st.get("job-mail")
	if got.Status != domain.MigrationDone {
		t.Fatalf("estado final = %q (%s)", got.Status, got.LastError)
	}
	if len(mail.imported) != 3 {
		t.Fatalf("mensajes importados = %d, quiero 3", len(mail.imported))
	}

	// Cada mensaje, en su sitio.
	porAsunto := map[string]importedMail{}
	for _, im := range mail.imported {
		for _, id := range []string{"m1", "m2", "m3"} {
			if strings.Contains(im.body, "asunto de "+id) {
				porAsunto[id] = im
			}
		}
	}
	if porAsunto["m1"].folder != "Inbox" {
		t.Errorf("el correo de la bandeja acabó en %q", porAsunto["m1"].folder)
	}
	if porAsunto["m2"].folder != "Enviados" {
		t.Errorf("el correo ENVIADO acabó en %q; ese es el fallo que arruina una mudanza", porAsunto["m2"].folder)
	}
	if porAsunto["m3"].folder != "Clientes" {
		t.Errorf("la etiqueta propia no se convirtió en carpeta: %q", porAsunto["m3"].folder)
	}

	// Estado: m1 sin leer, m2 leído, m3 leído y destacado.
	if porAsunto["m1"].keywords["$seen"] {
		t.Error("un correo SIN leer llegó marcado como leído")
	}
	if !porAsunto["m2"].keywords["$seen"] {
		t.Error("un correo leído llegó como no leído")
	}
	if !porAsunto["m3"].keywords["$flagged"] {
		t.Error("se perdió el destacado")
	}
	// Fecha ORIGINAL (internalDate), no la de la migración.
	if !strings.HasPrefix(porAsunto["m1"].receivedAt, "2024-03-01") {
		t.Errorf("fecha de recepción = %q; debía ser la del origen", porAsunto["m1"].receivedAt)
	}

	// Segunda pasada: idempotente.
	before := len(mail.imported)
	j2, _ := st.get("job-mail")
	if err := migrate.Run(context.Background(), deps, *j2); err != nil {
		t.Fatalf("Run (2.ª pasada): %v", err)
	}
	if len(mail.imported) != before {
		t.Errorf("la segunda pasada duplicó el buzón (%d → %d)", before, len(mail.imported))
	}
}

// TestGmailSinDestinoLoDiceClaro: sin Sytue Mail configurado, el trabajo se
// cierra con un motivo legible en vez de fallar a mitad.
func TestGmailSinDestinoLoDiceClaro(t *testing.T) {
	allowLoopback(t)
	gsrv := gmailServer(t)
	inst := googleConsentInstance(gsrv.URL)

	j := job("job-sin-mail", domain.SourceGoogle, domain.DomainMail)
	j.SourceUser = ""
	st := newMemStore("org-1")
	st.put(j)
	src, err := migrate.NewSource(context.Background(), gmailRuntime(), inst, j)
	if err != nil {
		t.Fatalf("NewSource: %v", err)
	}
	err = migrate.Run(context.Background(), migrate.Deps{Src: src, Store: st}, j)
	if err == nil || !strings.Contains(err.Error(), "Sytue Mail") {
		t.Fatalf("error = %v; debía explicar que falta el destino", err)
	}
}
