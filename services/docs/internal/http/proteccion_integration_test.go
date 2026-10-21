// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	docsdb "git.sytue.eu/sytue/one/services/docs/db"
	"git.sytue.eu/sytue/one/services/docs/internal/collab"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
	"git.sytue.eu/sytue/one/services/docs/internal/store"
)

// La protección de hoja se cumple en el servicio por la vía REST: POST /ops, GET /ops,
// GET /state y la restauración de versiones, contra el servicio Go con PostgreSQL real
// (no contra un fake: la regla vive dentro de la transacción de escritura).
func TestProteccionPorRutasREST(t *testing.T) {
	h := pgtest.New(t, docsdb.Migrations, docsdb.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	hd := &Handler{
		svc: domain.NewService(st), store: st, hub: collab.NewHub(nil),
		presence: collab.NewPresenceRegistry(), inline: 256 * 1024,
	}
	doc, _, err := st.CreateDocument(ctx, domain.CreateDocumentParams{Kind: domain.KindSpreadsheet, Title: "Cuentas", OwnerSub: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	for _, quien := range []string{"editor", "lector"} {
		rol := domain.RoleEditor
		if quien == "lector" {
			rol = domain.RoleViewer
		}
		if err := st.UpsertPermission(ctx, doc.ID, domain.Permission{PrincipalKind: domain.PrincipalUser, PrincipalID: quien, Role: rol, GrantedBy: "owner"}); err != nil {
			t.Fatal(err)
		}
	}
	peticion := func(metodo, quien, cuerpo string, extra map[string]string) *http.Request {
		params := map[string]string{"id": doc.ID}
		for k, v := range extra {
			params[k] = v
		}
		r := req(t, metodo, "/v1/documents/"+doc.ID, &authn.Claims{Subject: quien, Tenant: "acme"}, params, cuerpo)
		return r.WithContext(tenancy.Into(r.Context(), tenancy.Tenant{Slug: "acme"}))
	}
	modelo := func(celdas, protect string) string {
		inner := fmt.Sprintf(`{"active":0,"sheets":[{"name":"H","cells":%s%s,"styles":{"B1":{"locked":false}}}]}`, celdas, protect)
		v, _ := json.Marshal(inner)
		return string(v)
	}
	lamport := 0
	ops := func(valor, extra string) string {
		lamport++
		return fmt.Sprintf(`{"ops":[{"kind":"set","dot":{"a":"c","c":%d},"key":"doc:sheet","value":%s,"lamport":%d}]%s}`, lamport, valor, lamport, extra)
	}
	enviar := func(quien, cuerpo string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		hd.postOps(rec, peticion("POST", quien, cuerpo, nil))
		return rec
	}
	sinProteger := modelo(`{"A1":"10","B1":"libre"}`, "")

	if rec := enviar("editor", ops(sinProteger, "")); rec.Code != http.StatusOK {
		t.Fatalf("escritura normal: %d %s", rec.Code, rec.Body)
	}
	protegida := modelo(`{"A1":"10","B1":"libre"}`, `,"protect":{}`)
	if rec := enviar("editor", ops(protegida, `,"proteccion":{"new":{"hoja:H":"clave-rest"}}`)); rec.Code != http.StatusOK {
		t.Fatalf("proteger: %d %s", rec.Code, rec.Body)
	}

	// POST directo a /ops con un set de doc:sheet: lo que antes colaba cualquier editor.
	rec := enviar("editor", ops(modelo(`{"A1":"hackeada","B1":"libre"}`, `,"protect":{"hasPassword":true}`), ""))
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), `"protected"`) {
		t.Fatalf("celda bloqueada por POST: %d %s", rec.Code, rec.Body)
	}
	rec = enviar("editor", ops(sinProteger, `,"proteccion":{"password":"mala"}`))
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "protection_password") {
		t.Fatalf("contraseña mala: %d %s", rec.Code, rec.Body)
	}
	if rec := enviar("lector", ops(sinProteger, "")); rec.Code != http.StatusForbidden {
		t.Fatalf("un lector escribió: %d", rec.Code)
	}

	// Ni GET /ops ni GET /state devuelven la contraseña ni nada con que recuperarla.
	rec = httptest.NewRecorder()
	hd.getOps(rec, peticion("GET", "lector", "", nil))
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "clave-rest") || strings.Contains(rec.Body.String(), "hashValue") {
		t.Fatalf("GET /ops: %d %s", rec.Code, rec.Body)
	}
	rec = httptest.NewRecorder()
	hd.getState(rec, peticion("GET", "lector", "", nil))
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "clave-rest") || !strings.Contains(rec.Body.String(), "hasPassword") {
		t.Fatalf("GET /state: %d %s", rec.Code, rec.Body)
	}

	// Restaurar una versión sin protección: denegado sin contraseña, permitido con ella.
	// (la versión se crea ANTES de proteger, por eso se vuelve a poner la hoja a mano)
	if rec := enviar("owner", ops(sinProteger, "")); rec.Code != http.StatusOK {
		t.Fatalf("el propietario quita la protección: %d %s", rec.Code, rec.Body)
	}
	rec = httptest.NewRecorder()
	hd.createVersion(rec, peticion("POST", "owner", `{}`, nil))
	if rec.Code != http.StatusCreated {
		t.Fatalf("crear versión: %d %s", rec.Code, rec.Body)
	}
	var creada struct {
		Version struct {
			ID string `json:"id"`
		} `json:"version"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &creada); err != nil {
		t.Fatal(err)
	}
	if rec := enviar("editor", ops(protegida, `,"proteccion":{"new":{"hoja:H":"clave-rest"}}`)); rec.Code != http.StatusOK {
		t.Fatalf("volver a proteger: %d %s", rec.Code, rec.Body)
	}
	vid := map[string]string{"versionId": creada.Version.ID}
	rec = httptest.NewRecorder()
	hd.restoreVersion(rec, peticion("POST", "editor", "", vid))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("restaurar sin contraseña: %d %s", rec.Code, rec.Body)
	}
	rec = httptest.NewRecorder()
	hd.restoreVersion(rec, peticion("POST", "editor", `{"proteccion":{"password":"clave-rest"}}`, vid))
	if rec.Code != http.StatusOK {
		t.Fatalf("restaurar con la contraseña: %d %s", rec.Code, rec.Body)
	}
}

// La misma frontera por el WebSocket: una conexión de editor que manda un set de
// doc:sheet que toca lo protegido recibe «protected», no un ack; con la contraseña
// mala, «protection_password»; con la buena, ack. Contra el servicio real, no un fake.
func TestProteccionPorWebSocket(t *testing.T) {
	h := pgtest.New(t, docsdb.Migrations, docsdb.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	hd := &Handler{
		svc: domain.NewService(st), store: st, hub: collab.NewHub(nil),
		presence: collab.NewPresenceRegistry(), inline: 256 * 1024,
	}
	doc, _, err := st.CreateDocument(ctx, domain.CreateDocumentParams{Kind: domain.KindSpreadsheet, Title: "Cuentas", OwnerSub: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertPermission(ctx, doc.ID, domain.Permission{PrincipalKind: domain.PrincipalUser, PrincipalID: "editor", Role: domain.RoleEditor, GrantedBy: "owner"}); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(authn.Into(r.Context(), authn.Claims{Subject: r.URL.Query().Get("sub"), Tenant: "acme"}))
		r = r.WithContext(tenancy.Into(r.Context(), tenancy.Tenant{Slug: "acme"}))
		rc := chi.NewRouteContext()
		rc.URLParams.Add("id", doc.ID)
		hd.collabWS(w, r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rc)))
	}))
	defer srv.Close()
	wctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(wctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/?sub=editor", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	siguiente := func() collab.ServerMessage {
		t.Helper()
		for {
			var m collab.ServerMessage
			if err := wsjson.Read(wctx, conn, &m); err != nil {
				t.Fatal(err)
			}
			if m.Type == collab.MsgAck || m.Type == collab.MsgError {
				return m
			}
		}
	}
	lamport := 0
	enviar := func(modelo string, p *domain.Proteccion) collab.ServerMessage {
		t.Helper()
		lamport++
		v, _ := json.Marshal(modelo)
		op := domain.Op{Kind: domain.OpSet, Key: "doc:sheet", Value: v, Lamport: uint64(lamport), Dot: domain.Dot{Actor: "c", Counter: uint64(lamport)}}
		if err := wsjson.Write(wctx, conn, collab.ClientMessage{Type: collab.MsgCData, Ops: []domain.Op{op}, Proteccion: p}); err != nil {
			t.Fatal(err)
		}
		return siguiente()
	}
	hoja := func(a1, protect string) string {
		return fmt.Sprintf(`{"active":0,"sheets":[{"name":"H","cells":{"A1":%q}%s}]}`, a1, protect)
	}
	if m := enviar(hoja("10", `,"protect":{}`), &domain.Proteccion{Nuevas: map[string]string{"hoja:H": "clave-ws"}}); m.Type != collab.MsgAck {
		t.Fatalf("proteger: %+v", m)
	}
	if m := enviar(hoja("hackeada", `,"protect":{"hasPassword":true}`), nil); m.Type != collab.MsgError || m.Code != "protected" {
		t.Fatalf("celda bloqueada por WS: %+v", m)
	}
	if m := enviar(hoja("hackeada", `,"protect":{"hasPassword":true}`), &domain.Proteccion{Clave: "mala"}); m.Type != collab.MsgError || m.Code != "protection_password" {
		t.Fatalf("contraseña mala por WS: %+v", m)
	}
	if m := enviar(hoja("11", `,"protect":{"hasPassword":true}`), &domain.Proteccion{Clave: "clave-ws"}); m.Type != collab.MsgAck {
		t.Fatalf("con la contraseña: %+v", m)
	}
}
