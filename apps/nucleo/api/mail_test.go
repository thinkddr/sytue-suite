// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

func TestCorreo(t *testing.T) {
	cuerpos := map[string]map[string]any{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clave := r.Method + " " + r.URL.Path
		if r.Body != nil && r.Method != http.MethodGet {
			datos, _ := io.ReadAll(r.Body)
			var c map[string]any
			_ = json.Unmarshal(datos, &c)
			cuerpos[clave] = c
		}
		switch clave {
		case "GET /api/v1/aliases":
			_, _ = io.WriteString(w, `{"aliases":[],"primary":"ada@example.org"}`)
		case "GET /api/v1/mailboxes":
			_, _ = io.WriteString(w, `{"mailboxes":[{"id":"b1","name":"INBOX","role":"inbox","total_emails":12,"unread_emails":3}]}`)
		case "GET /api/v1/mailboxes/b1/messages":
			if r.URL.Query().Get("cursor") != "c1" || r.URL.Query().Get("limit") != "50" {
				t.Errorf("paginación: %s", r.URL.RawQuery)
			}
			_, _ = io.WriteString(w, `{"messages":[{"id":"m1","subject":"Hola","from":"g@example.org","from_name":"Grace","keywords":["\\Seen"],"preview":"…"}],"next_cursor":"c2"}`)
		case "GET /api/v1/messages/m1/raw":
			_, _ = io.WriteString(w, "Subject: Hola\r\n\r\ncuerpo\r\n")
		case "POST /api/v1/messages/m1/flags", "POST /api/v1/messages/m1/move", "DELETE /api/v1/messages/m1":
			w.WriteHeader(http.StatusNoContent)
		case "POST /api/v1/aliases":
			w.WriteHeader(http.StatusConflict)
			_, _ = io.WriteString(w, `{"error":{"code":"alias_limit","message":"Your account can have one alias."}}`)
		case "DELETE /api/v1/aliases/a1":
			w.WriteHeader(http.StatusNoContent)
		case "POST /api/v1/send":
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, `{"status":"queued"}`)
		case "POST /api/v1/drafts", "PUT /api/v1/drafts/d1":
			_, _ = io.WriteString(w, `{"id":"d2","mailbox_id":"b9","subject":"A medias","bcc":["o@example.org"]}`)
		case "GET /api/v1/threads/h1/messages":
			_, _ = io.WriteString(w, `{"messages":[{"id":"m2","thread_id":"h1"},{"id":"m1","thread_id":"h1","thread_count":2}]}`)
		case "POST /api/v1/messages/batch/move", "POST /api/v1/messages/batch/flags":
			_, _ = io.WriteString(w, `{"moved":2}`)
		case "GET /api/v1/labels":
			_, _ = io.WriteString(w, `{"labels":[{"id":"e1","name":"Obra","color":"teal","unread_emails":1}]}`)
		case "POST /api/v1/labels/e1/apply", "PUT /api/v1/signature", "PUT /api/v1/vacation":
			w.WriteHeader(http.StatusNoContent)
		case "GET /api/v1/labels/e1/messages":
			_, _ = io.WriteString(w, `{"messages":[{"id":"m1","keywords":["e1"]}],"next_cursor":""}`)
		case "GET /api/v1/signature":
			_, _ = io.WriteString(w, `{"signature":"Ada\nSytue"}`)
		case "GET /api/v1/vacation":
			_, _ = io.WriteString(w, `{"enabled":true,"subject":"Fuera","body":"Vuelvo el lunes","ends_at":"2026-10-05T00:00:00Z","updated_at":"2026-09-21T10:00:00Z"}`)
		default:
			t.Errorf("petición inesperada: %s", clave)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	c, ctx := NewMail(srv.URL, nil, nil), context.Background()

	if de, err := c.Address(ctx); err != nil || de != "ada@example.org" {
		t.Fatalf("Address: %q, %v", de, err)
	}
	carpetas, err := c.Mailboxes(ctx)
	if err != nil || len(carpetas) != 1 || carpetas[0].UnreadEmails != 3 || carpetas[0].Role != "inbox" {
		t.Fatalf("carpetas: %+v, %v", carpetas, err)
	}
	mensajes, siguiente, err := c.Messages(ctx, "b1", "c1", 50)
	if err != nil || siguiente != "c2" || len(mensajes) != 1 {
		t.Fatalf("mensajes: %+v, %q, %v", mensajes, siguiente, err)
	}
	if m := mensajes[0]; !m.Has(MailFlagSeen) || m.Has(MailFlagFlagged) || m.Sender() != "Grace" {
		t.Errorf("mensaje: %+v", m)
	}
	crudo, err := c.Raw(ctx, "m1")
	if err != nil {
		t.Fatal(err)
	}
	datos, _ := io.ReadAll(crudo)
	_ = crudo.Close()
	if !strings.Contains(string(datos), "cuerpo") {
		t.Errorf("Raw: %q", datos)
	}
	for nombre, err := range map[string]error{
		"SetFlag": c.SetFlag(ctx, "m1", MailFlagSeen, true), "Move": c.Move(ctx, "m1", "b2"),
		"Delete": c.Delete(ctx, "m1"), "Send": soloError(c.Send(ctx, "", []string{"g@example.org"}, []byte("Subject: x\r\n\r\ny"), 0)),
	} {
		if err != nil {
			t.Errorf("%s: %v", nombre, err)
		}
	}
	if f := cuerpos["POST /api/v1/messages/m1/flags"]; f["flag"] != `\Seen` || f["add"] != true {
		t.Errorf("cuerpo de flags: %v", f)
	}
	if e := cuerpos["POST /api/v1/send"]; e["raw"] != "Subject: x\r\n\r\ny" || e["recipients"] == nil {
		t.Errorf("cuerpo de send: %v", e)
	}

	// Borradores: crear es POST, sustituir es PUT, y el id que vale es el DEVUELTO.
	for _, id := range []string{"", "d1"} {
		if b, err := c.SaveDraft(ctx, id, "ada@example.org", []byte("Subject: A medias\r\n\r\n")); err != nil || b.ID != "d2" || len(b.Bcc) != 1 {
			t.Fatalf("SaveDraft(%q): %+v, %v", id, b, err)
		}
	}
	if b := cuerpos["PUT /api/v1/drafts/d1"]; b["raw"] != "Subject: A medias\r\n\r\n" || b["from"] != "ada@example.org" || len(b) != 2 {
		t.Errorf("cuerpo de drafts (el servicio no admite campos de más): %v", b)
	}
	if hilo, err := c.Thread(ctx, "h1"); err != nil || len(hilo) != 2 || hilo[1].ThreadCount != 2 {
		t.Fatalf("Thread: %+v, %v", hilo, err)
	}
	etiquetas, err := c.Labels(ctx)
	if err != nil || len(etiquetas) != 1 || etiquetas[0].Name != "Obra" || etiquetas[0].UnreadEmails != 1 {
		t.Fatalf("Labels: %+v, %v", etiquetas, err)
	}
	if con, _, err := c.LabelMessages(ctx, "e1", "", 50); err != nil || len(con) != 1 || !con[0].Has("e1") {
		t.Fatalf("LabelMessages: %+v, %v", con, err)
	}
	if firma, err := c.Signature(ctx); err != nil || firma != "Ada\nSytue" {
		t.Fatalf("Signature: %q, %v", firma, err)
	}
	vac, err := c.Vacation(ctx)
	if err != nil || !vac.Enabled || vac.StartsAt != nil || vac.EndsAt == nil || vac.EndsAt.Day() != 5 {
		t.Fatalf("Vacation: %+v, %v", vac, err)
	}
	for nombre, err := range map[string]error{
		"MoveMany": c.MoveMany(ctx, []string{"m1", "m2"}, "b2"), "SetFlagMany": c.SetFlagMany(ctx, []string{"m1"}, MailFlagSeen, false),
		"ApplyLabel": c.ApplyLabel(ctx, "e1", []string{"m1"}, true), "SetSignature": c.SetSignature(ctx, "Ada"), "SetVacation": c.SetVacation(ctx, vac),
	} {
		if err != nil {
			t.Errorf("%s: %v", nombre, err)
		}
	}
	if b := cuerpos["POST /api/v1/messages/batch/move"]; b["mailbox_id"] != "b2" || len(b["ids"].([]any)) != 2 {
		t.Errorf("cuerpo de batch/move: %v", b)
	}
	if b := cuerpos["POST /api/v1/labels/e1/apply"]; b["add"] != true || len(b["message_ids"].([]any)) != 1 {
		t.Errorf("cuerpo de labels/apply: %v", b)
	}
	// El PUT de vacaciones quiere TEXTO en las fechas y ni un campo de más.
	if b := cuerpos["PUT /api/v1/vacation"]; b["starts_at"] != "" || b["ends_at"] != "2026-10-05T00:00:00Z" || len(b) != 5 {
		t.Errorf("cuerpo de vacation: %v", b)
	}
}

func TestLosCamposDeMailSonLosDelServicio(t *testing.T) {
	dominio := webref.Read(t, "services/mail/internal/domain/domain.go")
	for _, campo := range []string{
		`json:"total_emails"`, `json:"unread_emails"`, `json:"role,omitempty"`, `json:"from_name,omitempty"`, `json:"preview,omitempty"`,
		`json:"keywords,omitempty"`, `json:"has_attachment"`, `json:"received_at"`, `json:"message_id"`, `json:"in_reply_to,omitempty"`,
		`FlagSeen     = "\\Seen"`, `FlagFlagged  = "\\Flagged"`,
	} {
		if !strings.Contains(dominio, campo) {
			t.Errorf("services/mail ya no declara %s: revisar internal/api/mail.go", campo)
		}
	}
	manejadores := webref.Read(t, "services/mail/internal/jmap/handlers.go")
	for _, literal := range []string{`json:"flag"`, `json:"add"`, `json:"mailbox_id"`, `json:"recipients"`, `json:"raw"`, `json:"raw_blob_id,omitempty"`, `"position"`, `"next_cursor": next`, `"mailboxes": boxes`} {
		if !strings.Contains(manejadores, literal) {
			t.Errorf("services/mail/internal/jmap/handlers.go ya no dice %s", literal)
		}
	}
	rutas := webref.Read(t, "services/mail/internal/jmap/jmap.go")
	for _, ruta := range []string{
		`ar.Get("/mailboxes", a.listMailboxes)`, `"/mailboxes/{mailboxID}/messages"`, `"/messages/search"`, `"/messages/{messageID}/raw"`,
		`"/messages/{messageID}/flags"`, `"/messages/{messageID}/move"`, `ar.Post("/send", a.sendMessage)`,
		`pr.Post("/jmap/upload/{accountId}", a.handleUpload)`, `pr.Get("/jmap/eventos", a.handleStream)`,
	} {
		if !strings.Contains(rutas, ruta) {
			t.Errorf("services/mail ya no monta %s", ruta)
		}
	}
	for _, ruta := range []string{
		`ar.Post("/drafts", a.createDraft)`, `ar.Put("/drafts/{messageID}", a.updateDraft)`, `"/threads/{threadID}/messages"`,
		`"/messages/batch/move"`, `"/messages/batch/flags"`, `ar.Get("/labels", a.listLabels)`, `"/labels/{labelID}/apply"`,
		`"/labels/{labelID}/messages"`, `ar.Put("/signature", a.setSignature)`, `ar.Put("/vacation", a.setVacation)`,
		`ar.Get("/quota", a.getQuota)`,
	} {
		if !strings.Contains(rutas, ruta) {
			t.Errorf("services/mail ya no monta %s", ruta)
		}
	}
	for fichero, literales := range map[string][]string{
		"services/mail/internal/jmap/batch.go":    {`json:"ids"`, `json:"mailbox_id"`, `json:"flag"`},
		"services/mail/internal/jmap/labels.go":   {`json:"message_ids"`, `"labels": labels`},
		"services/mail/internal/jmap/vacation.go": {`json:"starts_at"`, `json:"ends_at"`, `json:"signature"`, `json:"enabled"`},
		"services/mail/internal/jmap/quota.go":    {`"used_bytes":  acc.UsedBytes`, `"quota_bytes": acc.QuotaBytes`},
		"services/mail/internal/domain/domain.go": {`json:"thread_count,omitempty"`, `json:"bcc,omitempty"`},
		// El tope de MaxMailBody es ESTE: el día que /send admita más, que suba aquí también.
		"platform/pkg/httpx/respond.go": {`const maxBodyBytes = 1 << 20`},
	} {
		fuente := webref.Read(t, fichero)
		for _, literal := range literales {
			if !strings.Contains(fuente, literal) {
				t.Errorf("%s ya no dice %s: revisar internal/api/mail.go", fichero, literal)
			}
		}
	}
	if !strings.Contains(manejadores, "var body sendReq // {from, recipients, raw | raw_blob_id}: mismo contrato que /send.") {
		t.Error("POST /drafts ya no decodifica un sendReq: revisar Mail.SaveDraft")
	}
	if alias := webref.Read(t, "services/mail/internal/jmap/aliases.go"); !strings.Contains(alias, `"primary": acc.Address`) {
		t.Error("GET /aliases ya no devuelve `primary`: Mail.Address se queda sin remitente")
	}
}

// soloError se queda con el error de una llamada que devuelve dos cosas, para las tablas
// de arriba.
func soloError[T any](_ T, err error) error { return err }

// Deshacer el envío y darse de baja: las dos cosas que el servicio ya hacía y no se
// alcanzaban desde Gio.
func TestDeshacerYDarseDeBaja(t *testing.T) {
	var enviado map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		datos, _ := io.ReadAll(r.Body)
		switch r.Method + " " + r.URL.Path {
		case "POST /api/v1/aliases":
			w.WriteHeader(http.StatusConflict)
			_, _ = io.WriteString(w, `{"error":{"code":"alias_limit","message":"Your account can have one alias."}}`)
		case "DELETE /api/v1/aliases/a1":
			w.WriteHeader(http.StatusNoContent)
		case "POST /api/v1/send":
			_ = json.Unmarshal(datos, &enviado)
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, `{"status":"undo","id":"o1","release_at":"2026-09-21T10:00:10Z"}`)
		case "POST /api/v1/outbound/o1/cancel":
			_, _ = io.WriteString(w, `{"canceled":true,"kind":"undo"}`)
		case "POST /api/v1/outbound/o2/cancel": // ya había salido
			w.WriteHeader(http.StatusConflict)
			_, _ = io.WriteString(w, `{"code":"conflict"}`)
		case "GET /api/v1/messages/m1/unsubscribe":
			_, _ = io.WriteString(w, `{"one_click":true,"mailto":false}`)
		case "POST /api/v1/messages/m1/unsubscribe":
			_, _ = io.WriteString(w, `{"status":"ok"}`)
		case "GET /api/v1/messages/m2/unsubscribe":
			_, _ = io.WriteString(w, `{"one_click":false,"mailto":false}`)
		default:
			t.Errorf("petición inesperada: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	c, ctx := NewMail(srv.URL, nil, nil), context.Background()

	envio, err := c.Send(ctx, "", []string{"g@example.org"}, []byte("Subject: x\r\n\r\ny"), 10)
	if err != nil || envio.ID != "o1" || envio.ReleaseAt.IsZero() || envio.Status != "undo" {
		t.Fatalf("envío retenido: %+v, %v", envio, err)
	}
	if enviado["undo_seconds"] != float64(10) {
		t.Fatalf("el cuerpo de /send no lleva undo_seconds: %v", enviado)
	}
	// Sin ventana NO viaja el campo: el servicio lo trata como envío inmediato.
	enviado = nil
	if _, err := c.Send(ctx, "", []string{"g@example.org"}, []byte("x"), 0); err != nil {
		t.Fatal(err)
	}
	if _, hay := enviado["undo_seconds"]; hay {
		t.Errorf("un envío sin ventana manda undo_seconds: %v", enviado)
	}

	if err := c.CancelOutbound(ctx, "o1"); err != nil {
		t.Fatalf("cancelar: %v", err)
	}
	// El 409 del servicio es «ya ha salido», y hay que poder distinguirlo de una caída.
	if err := c.CancelOutbound(ctx, "o2"); !errors.Is(err, ErrDemasiadoTarde) {
		t.Fatalf("cancelar tarde: %v", err)
	}

	baja, err := c.Unsubscribe(ctx, "m1")
	if err != nil || !baja.Hay() || !baja.OneClick {
		t.Fatalf("métodos de baja: %+v, %v", baja, err)
	}
	if sin, err := c.Unsubscribe(ctx, "m2"); err != nil || sin.Hay() {
		t.Fatalf("un correo sin baja: %+v, %v", sin, err)
	}
	if estado, err := c.DoUnsubscribe(ctx, "m1"); err != nil || estado != BajaHecha {
		t.Fatalf("darse de baja: %q, %v", estado, err)
	}
}

func TestLoQueMailSirveYGioNoPedia(t *testing.T) {
	rutas := webref.Read(t, "services/mail/internal/jmap/jmap.go")
	for _, ruta := range []string{
		`ar.Post("/outbound/{id}/cancel"`, `ar.Get("/messages/{messageID}/unsubscribe"`, `ar.Post("/messages/{messageID}/unsubscribe"`,
	} {
		if !strings.Contains(rutas, ruta) {
			t.Errorf("services/mail ya no monta %s", ruta)
		}
	}
	manejadores := webref.Read(t, "services/mail/internal/jmap/handlers.go")
	for _, literal := range []string{
		"UndoSeconds int    `json:\"undo_seconds,omitempty\"`", `"id":         id`, `"release_at": releaseAt.UTC().Format(time.RFC3339)`,
		"const maxUndoSeconds = 30",
	} {
		if !strings.Contains(manejadores, literal) {
			t.Errorf("services/mail/internal/jmap/handlers.go ya no dice %s", literal)
		}
	}
	// El «ya ha salido» es un 409, que es lo que distingue ErrDemasiadoTarde de una caída.
	if !strings.Contains(manejadores, "case errors.Is(err, domain.ErrScheduleGone):\n\t\treturn httpx.ErrConflict") {
		t.Error("services/mail ya no contesta 409 a un envío que se ha liberado: revisar api.ErrDemasiadoTarde")
	}
	baja := webref.Read(t, "services/mail/internal/jmap/unsubscribe.go")
	for _, literal := range []string{`"one_click": u.OneClick`, `"mailto":    u.Mailto != ""`, `"status": "ok"`, `"status": "sent"`, `"status": "failed"`} {
		if !strings.Contains(baja, literal) {
			t.Errorf("services/mail/internal/jmap/unsubscribe.go ya no dice %s", literal)
		}
	}
}

// El delta: el arranque va SIN `since` —si se mandara vacío, el servicio lo leería como
// cursor ilegible y contestaría 400— y las vueltas siguientes con el estado recibido.
func TestCorreoCambios(t *testing.T) {
	var pedidos []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/changes" {
			t.Errorf("ruta inesperada: %s", r.URL.Path)
		}
		pedidos = append(pedidos, r.URL.RawQuery)
		if r.URL.Query().Has("since") {
			_, _ = io.WriteString(w, `{"changed":[{"id":"m3","mailbox_id":"b1","keywords":["\\Seen"]}],"destroyed":["m1"],"state":"s2","has_more":true}`)
			return
		}
		_, _ = io.WriteString(w, `{"changed":[],"destroyed":[],"state":"s1","has_more":false}`)
	}))
	defer srv.Close()
	c, ctx := NewMail(srv.URL, nil, nil), context.Background()

	primero, err := c.Changes(ctx, "")
	if err != nil || primero.State != "s1" || len(primero.Changed) != 0 {
		t.Fatalf("arranque: %+v, %v", primero, err)
	}
	delta, err := c.Changes(ctx, primero.State)
	if err != nil || delta.State != "s2" || !delta.HasMore || len(delta.Destroyed) != 1 || delta.Destroyed[0] != "m1" {
		t.Fatalf("delta: %+v, %v", delta, err)
	}
	if len(delta.Changed) != 1 || !delta.Changed[0].Has(MailFlagSeen) {
		t.Errorf("el sobre cambiado no llega entero: %+v", delta.Changed)
	}
	if len(pedidos) != 2 || pedidos[0] != "" || pedidos[1] != "since=s1" {
		t.Errorf("consultas: %q", pedidos)
	}
}

// TestAliasYProgramado: el selector «De» del redactor sale de /aliases (la principal
// primero) y «Programar envío» es el mismo /send con `send_at` en RFC 3339 y en UTC.
func TestAliasYProgramado(t *testing.T) {
	var programado map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/aliases":
			_, _ = io.WriteString(w, `{"aliases":[{"id":"a1","address":"ventas@example.org","created_at":"2026-09-01T10:00:00Z"}],"primary":"ada@example.org"}`)
		case "POST /api/v1/aliases":
			w.WriteHeader(http.StatusConflict)
			_, _ = io.WriteString(w, `{"error":{"code":"alias_limit","message":"Your account can have one alias."}}`)
		case "DELETE /api/v1/aliases/a1":
			w.WriteHeader(http.StatusNoContent)
		case "POST /api/v1/send":
			datos, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(datos, &programado)
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, `{"status":"scheduled","id":"o1","release_at":"2026-09-24T08:00:00Z"}`)
		default:
			t.Errorf("petición inesperada: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	c, ctx := NewMail(srv.URL, nil, nil), context.Background()
	principal, alias, err := c.Aliases(ctx)
	if err != nil || principal != "ada@example.org" || len(alias) != 1 || alias[0] != (MailAlias{ID: "a1", Address: "ventas@example.org"}) {
		t.Fatalf("alias: %q %v %v", principal, alias, err)
	}
	var e *Error
	if _, err := c.CreateAlias(ctx, "otra@example.org"); !errors.As(err, &e) || e.Code() != "alias_limit" {
		t.Errorf("el alta rechazada no trae su código: %v", err)
	}
	if err := c.DeleteAlias(ctx, "a1"); err != nil {
		t.Errorf("borrar alias: %v", err)
	}
	cuando := time.Date(2026, 9, 24, 10, 0, 0, 0, time.FixedZone("CEST", 2*3600))
	env, err := c.Schedule(ctx, "ada@example.org", []string{"g@example.org"}, []byte("raw"), cuando)
	if err != nil || env.Status != "scheduled" || env.ID != "o1" {
		t.Fatalf("programado: %+v, %v", env, err)
	}
	if programado["send_at"] != "2026-09-24T08:00:00Z" || programado["raw"] != "raw" {
		t.Errorf("cuerpo del programado: %v", programado)
	}
	manejadores := webref.Read(t, "services/mail/internal/jmap/handlers.go")
	if !strings.Contains(manejadores, "SendAt      string `json:\"send_at,omitempty\"`") {
		t.Error("POST /send ya no acepta `send_at`: revisar Mail.Schedule")
	}
	if dominio := webref.Read(t, "services/mail/internal/domain/alias.go"); !strings.Contains(dominio, "Address   string    `json:\"address\"`") {
		t.Error("el alias ya no se llama `address`: revisar Mail.Aliases")
	}
}

// TestRenombrarYBorrarCarpetaVanPorJMAP: el servicio solo los ofrece como `Mailbox/set`
// en `POST /jmap`, y el rechazo llega como motivo, no como error.
func TestRenombrarYBorrarCarpetaVanPorJMAP(t *testing.T) {
	var pedido struct {
		Using       []string `json:"using"`
		MethodCalls [][]any  `json:"methodCalls"` //nolint:tagliatelle // JMAP (RFC 8620) es camelCase
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method+" "+r.URL.Path != "POST /jmap" {
			t.Errorf("petición inesperada: %s %s", r.Method, r.URL.Path)
			return
		}
		datos, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(datos, &pedido)
		args, _ := pedido.MethodCalls[0][1].(map[string]any)
		if _, borra := args["destroy"]; borra {
			_, _ = io.WriteString(w, `{"methodResponses":[["Mailbox/set",{"destroyed":[],"notDestroyed":{"m2":{"type":"mailboxHasEmail"}}},"0"]]}`)
			return
		}
		_, _ = io.WriteString(w, `{"methodResponses":[["Mailbox/set",{"updated":{"m1":null},"notUpdated":{}},"0"]]}`)
	}))
	defer srv.Close()
	c, ctx := NewMail(srv.URL, nil, nil), context.Background()
	if motivo, err := c.RenameMailbox(ctx, "m1", "Facturas"); motivo != "" || err != nil {
		t.Fatalf("renombrar: %q %v", motivo, err)
	}
	if pedido.MethodCalls[0][0] != "Mailbox/set" || len(pedido.Using) != 2 {
		t.Errorf("pedido: %+v", pedido)
	}
	if u, _ := pedido.MethodCalls[0][1].(map[string]any)["update"].(map[string]any); u["m1"].(map[string]any)["name"] != "Facturas" {
		t.Errorf("el renombrado no lleva el nombre: %+v", pedido.MethodCalls)
	}
	if motivo, err := c.DeleteMailbox(ctx, "m2"); motivo != "mailboxHasEmail" || err != nil {
		t.Errorf("borrar con correo dentro: %q %v", motivo, err)
	}
	if !strings.Contains(webref.Read(t, "services/mail/internal/jmap/jmap.go"), `pr.Post("/jmap", a.handleJMAP)`) {
		t.Error("el servicio ya no atiende JMAP en POST /jmap: revisar Mail.mailboxSet")
	}
}

// TestLasReglasSonLasDelServicio: la lista, crear sin carpeta (move_to null) y la
// actualización entera por PUT.
func TestLasReglasSonLasDelServicio(t *testing.T) {
	var cuerpos []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		datos, _ := io.ReadAll(r.Body)
		var c map[string]any
		_ = json.Unmarshal(datos, &c)
		cuerpos = append(cuerpos, c)
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/filters":
			_, _ = io.WriteString(w, `{"filters":[{"id":"r1","name":"","enabled":true,"priority":0,"match_from":"banco","move_to":"m9","mark_read":true,"mark_flagged":false}]}`)
		case "POST /api/v1/filters":
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"id":"r2","name":"x","enabled":true,"match_subject":"factura"}`)
		case "PUT /api/v1/filters/r1", "DELETE /api/v1/filters/r1":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("petición inesperada: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	c, ctx := NewMail(srv.URL, nil, nil), context.Background()
	rs, err := c.Filters(ctx)
	if err != nil || len(rs) != 1 || rs[0].MatchFrom != "banco" || rs[0].MoveTo != "m9" || !rs[0].MarkRead {
		t.Fatalf("reglas: %+v %v", rs, err)
	}
	if r, err := c.CreateFilter(ctx, MailFilter{Name: "x", Enabled: true, MatchSubject: "factura"}); err != nil || r.ID != "r2" {
		t.Fatalf("crear: %+v %v", r, err)
	}
	if v, hay := cuerpos[1]["move_to"]; !hay || v != nil {
		t.Errorf("sin carpeta, move_to tiene que ir null: %v", cuerpos[1])
	}
	rs[0].Enabled = false
	if err := c.UpdateFilter(ctx, rs[0]); err != nil || cuerpos[2]["enabled"] != false || cuerpos[2]["move_to"] != "m9" {
		t.Errorf("actualizar: %v %v", cuerpos[2], err)
	}
	if err := c.DeleteFilter(ctx, "r1"); err != nil {
		t.Errorf("borrar: %v", err)
	}
	if !strings.Contains(webref.Read(t, "services/mail/internal/jmap/filters.go"), "MoveTo       *string `json:\"move_to\"`") {
		t.Error("el servicio ya no llama `move_to` al destino: revisar MailFilter")
	}
}

// TestElCertificadoSmimeVaEnPEM: el ancla se baja tal cual y el propio sube como PEM,
// sin JSON, con el motivo del rechazo legible.
func TestElCertificadoSmimeVaEnPEM(t *testing.T) {
	var subido, tipo string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/smime/anchor":
			_, _ = io.WriteString(w, "-----BEGIN CERTIFICATE-----\nAAA\n-----END CERTIFICATE-----\n")
		case "PUT /api/v1/smime/identity":
			datos, _ := io.ReadAll(r.Body)
			subido, tipo = string(datos), r.Header.Get("Content-Type")
			if strings.Contains(subido, "mala") {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = io.WriteString(w, `{"error":{"code":"bad_request","message":"The certificate does not match the key."}}`)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("petición inesperada: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	c, ctx := NewMail(srv.URL, nil, nil), context.Background()
	if pem, err := c.SmimeAnchor(ctx); err != nil || !strings.HasPrefix(string(pem), "-----BEGIN CERTIFICATE-----") {
		t.Fatalf("ancla: %q %v", pem, err)
	}
	if err := c.SetSmimeIdentity(ctx, "PEM"); err != nil || subido != "PEM" || tipo != "application/x-pem-file" {
		t.Fatalf("instalar: %v, %q como %q", err, subido, tipo)
	}
	var e *Error
	if err := c.SetSmimeIdentity(ctx, "mala"); !errors.As(err, &e) || e.Message() != "The certificate does not match the key." {
		t.Errorf("el rechazo no trae su motivo: %v", err)
	}
	if err := c.SetSmimeIdentity(ctx, strings.Repeat("x", MaxSmimeIdentity+1)); !errors.Is(err, ErrMailTooLarge) {
		t.Errorf("un PEM de más de 128 KiB se manda: %v", err)
	}
	manejadores := webref.Read(t, "services/mail/internal/jmap/jmap.go")
	if !strings.Contains(manejadores, `ar.Get("/smime/anchor"`) || !strings.Contains(manejadores, `ar.Put("/smime/identity"`) {
		t.Error("las rutas S/MIME del servicio ya no son las que usa el SDK")
	}
}

// TestLosBuzonesCompartidosSonLosDelServicio: la lista, sus carpetas y mensajes, el
// acceso a uno compartido y la delegación del propio, con los cuerpos que espera.
func TestLosBuzonesCompartidosSonLosDelServicio(t *testing.T) {
	var cuerpos []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		datos, _ := io.ReadAll(r.Body)
		if len(datos) > 0 {
			cuerpos = append(cuerpos, r.URL.Path+" "+string(datos))
		}
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/shared-mailboxes":
			_, _ = io.WriteString(w, `{"shared_mailboxes":[{"account_id":"s1","address":"soporte@example.org","kind":"shared","permission":"manage"}]}`)
		case "POST /api/v1/shared-mailboxes":
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"account_id":"s2","address":"ventas@example.org","display_name":"Ventas","kind":"shared","permission":"manage"}`)
		case "GET /api/v1/shared-mailboxes/s1/mailboxes":
			_, _ = io.WriteString(w, `{"mailboxes":[{"id":"i1","name":"INBOX","role":"inbox"}]}`)
		case "GET /api/v1/shared-mailboxes/s1/mailboxes/i1/messages":
			if r.URL.Query().Get("cursor") != "c1" {
				t.Errorf("sin cursor: %s", r.URL.RawQuery)
			}
			_, _ = io.WriteString(w, `{"messages":[{"id":"m1"}],"next_cursor":""}`)
		case "GET /api/v1/shared-mailboxes/s1/grants", "GET /api/v1/delegation":
			_, _ = io.WriteString(w, `{"grants":[{"grantee":"ana@example.org","permission":"send","created_at":"2026-09-01T10:00:00Z"}]}`)
		case "POST /api/v1/shared-mailboxes/s1/grants", "POST /api/v1/delegation":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("petición inesperada: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	c, ctx := NewMail(srv.URL, nil, nil), context.Background()
	if xs, err := c.SharedMailboxes(ctx); err != nil || len(xs) != 1 || xs[0].AccountID != "s1" || xs[0].Permission != "manage" {
		t.Fatalf("lista: %+v %v", xs, err)
	}
	if x, err := c.CreateSharedMailbox(ctx, "ventas@example.org", "Ventas"); err != nil || x.AccountID != "s2" {
		t.Fatalf("crear: %+v %v", x, err)
	}
	if cs, err := c.SharedFolders(ctx, "s1"); err != nil || len(cs) != 1 || cs[0].Role != "inbox" {
		t.Fatalf("carpetas: %+v %v", cs, err)
	}
	if ms, sig, err := c.SharedMessages(ctx, "s1", "i1", "c1", 50); err != nil || len(ms) != 1 || sig != "" {
		t.Fatalf("mensajes: %+v %q %v", ms, sig, err)
	}
	for _, f := range []func(context.Context) ([]MailGrant, error){func(ctx context.Context) ([]MailGrant, error) { return c.Grants(ctx, "s1") }, c.Delegations} {
		if gs, err := f(ctx); err != nil || len(gs) != 1 || gs[0] != (MailGrant{"ana@example.org", "send"}) {
			t.Errorf("accesos: %+v %v", gs, err)
		}
	}
	for _, err := range []error{
		c.Grant(ctx, "s1", "ana@example.org", "read"), c.Revoke(ctx, "s1", "ana@example.org"),
		c.Delegate(ctx, "luis@example.org", "send"), c.Undelegate(ctx, "luis@example.org"),
	} {
		if err != nil {
			t.Error(err)
		}
	}
	quiero := []string{
		`/api/v1/shared-mailboxes {"address":"ventas@example.org","display_name":"Ventas"}`,
		`/api/v1/shared-mailboxes/s1/grants {"grantee":"ana@example.org","permission":"read"}`,
		`/api/v1/shared-mailboxes/s1/grants {"delete":true,"grantee":"ana@example.org"}`,
		`/api/v1/delegation {"grantee":"luis@example.org","permission":"send"}`,
		`/api/v1/delegation {"delete":true,"grantee":"luis@example.org"}`,
	}
	if strings.Join(cuerpos, "\n") != strings.Join(quiero, "\n") {
		t.Errorf("cuerpos:\n%s", strings.Join(cuerpos, "\n"))
	}
	fuente := webref.Read(t, "services/mail/internal/jmap/shared.go")
	for _, campo := range []string{"`json:\"grantee\"`", "`json:\"permission\"`", "`json:\"delete\"`", "`json:\"display_name\"`", `"shared_mailboxes": boxes`} {
		if !strings.Contains(fuente, campo) {
			t.Errorf("el servicio ya no usa %s: revisar los buzones compartidos del SDK", campo)
		}
	}
}

// TestLaBusquedaPagina: la primera página no manda `position`; la siguiente, lo ya
// recibido. Es el `searchEmails(q, {limit, position})` de la web.
func TestLaBusquedaPagina(t *testing.T) {
	var pedidas []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/messages/search" {
			t.Errorf("ruta %s", r.URL.Path)
		}
		pedidas = append(pedidas, r.URL.Query().Encode())
		_, _ = io.WriteString(w, `{"messages":[{"id":"m1"}]}`)
	}))
	defer srv.Close()
	c, ctx := NewMail(srv.URL, nil, nil), context.Background()
	if ms, err := c.Search(ctx, "from:ana", 0, 50); err != nil || len(ms) != 1 {
		t.Fatalf("Search: %v %v", ms, err)
	}
	if _, err := c.Search(ctx, "from:ana", 50, 50); err != nil {
		t.Fatal(err)
	}
	if want := []string{"limit=50&q=from%3Aana", "limit=50&position=50&q=from%3Aana"}; !slices.Equal(pedidas, want) {
		t.Errorf("peticiones %q, quiero %q", pedidas, want)
	}
}

// TestLasLlegadasSonUnAviso: del flujo de `/jmap/eventos` solo cuentan las `llegada`;
// el `listo` del saludo y los latidos no avisan de nada.
func TestLasLlegadasSonUnAviso(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/jmap/eventos" || r.Header.Get("Accept") != "text/event-stream" {
			t.Errorf("petición %s (Accept %q)", r.URL.Path, r.Header.Get("Accept"))
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: listo\ndata: {\"account_id\":\"a\"}\n\n: latido\n\n"+
			"event: llegada\ndata: {\"mailbox_id\":\"in\"}\n\nevent: llegada\ndata: {}\n\n")
	}))
	defer srv.Close()
	n := 0
	if err := NewMail(srv.URL, nil, nil).Llegadas(context.Background(), func() { n++ }); err != nil || n != 2 {
		t.Errorf("llegadas %d (%v), quiero 2", n, err)
	}
}

// TestEscucharReconectaYSale: un flujo que se corta se vuelve a abrir (tras 1 s la
// primera vez) y, en cuanto muere el contexto, Escuchar vuelve.
func TestEscucharReconectaYSale(t *testing.T) {
	ctx, cancelar := context.WithCancel(context.Background())
	defer cancelar()
	veces := 0
	hecho := make(chan struct{})
	go func() {
		Escuchar(ctx, time.Minute, func(context.Context) error {
			veces++
			if veces == 2 {
				cancelar()
			}
			return errors.New("cortado")
		})
		close(hecho)
	}()
	select {
	case <-hecho:
	case <-time.After(5 * time.Second):
		t.Fatal("Escuchar no ha vuelto al morir el contexto")
	}
	if veces != 2 {
		t.Errorf("se ha conectado %d veces, quiero 2", veces)
	}
}

// TestElMensajeGrandeVaSubido: un mensaje que no cabe en el JSON de 1 MiB del servicio
// —un adjunto de 800 KB en base64 ya no cabía, en Fyne ni en la web— se sube antes por
// `/jmap/upload` y el envío lo referencia (`raw_blob_id`), sin `raw`. Uno pequeño sigue
// yendo dentro, sin subir nada.
func TestElMensajeGrandeVaSubido(t *testing.T) {
	var subido []byte
	cuerpos := map[string]map[string]any{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		datos, _ := io.ReadAll(r.Body)
		switch r.Method + " " + r.URL.Path {
		case "POST /jmap/upload/me":
			if r.Header.Get("Content-Type") != "message/rfc822" {
				t.Errorf("subida con Content-Type %q", r.Header.Get("Content-Type"))
			}
			subido = datos
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"accountId":"a","blobId":"blobs/a/b1","type":"message/rfc822","size":1}`)
		case "POST /api/v1/send", "POST /api/v1/drafts":
			var c map[string]any
			_ = json.Unmarshal(datos, &c)
			cuerpos[r.URL.Path] = c
			if r.URL.Path == "/api/v1/drafts" {
				_, _ = io.WriteString(w, `{"id":"d1"}`)
				return
			}
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, `{"status":"queued"}`)
		default:
			t.Errorf("petición inesperada: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	c, ctx := NewMail(srv.URL, nil, nil), context.Background()

	grande := []byte("Subject: grande\r\n\r\n" + strings.Repeat("QUFB", MaxMailBody/4))
	if _, err := c.Send(ctx, "", []string{"g@example.org"}, grande, 0); err != nil {
		t.Fatalf("Send grande: %v", err)
	}
	b := cuerpos["/api/v1/send"]
	if !slices.Equal(subido, grande) || b["raw_blob_id"] != "blobs/a/b1" || b["raw"] != nil {
		t.Fatalf("grande: subidos %d bytes, cuerpo %v", len(subido), b)
	}
	subido = nil
	if _, err := c.SaveDraft(ctx, "", "", []byte("Subject: corto\r\n\r\n")); err != nil || subido != nil {
		t.Fatalf("un borrador pequeño se sube: %v", err)
	}
	if cuerpos["/api/v1/drafts"]["raw"] != "Subject: corto\r\n\r\n" {
		t.Errorf("borrador pequeño: %v", cuerpos["/api/v1/drafts"])
	}
}
