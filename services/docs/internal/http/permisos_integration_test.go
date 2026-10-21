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
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	docsdb "git.sytue.eu/sytue/one/services/docs/db"
	"git.sytue.eu/sytue/one/services/docs/internal/collab"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
	"git.sytue.eu/sytue/one/services/docs/internal/store"
)

// Permisos de extremo a extremo (ROADMAP-OFFICE permisos-2): el servicio real con PostgreSQL
// real, por REST y por el WebSocket, con personas distintas. Lo que cada prueba defiende es
// una frontera que el navegador y el BFF no pueden cruzar:
//   - quien solo lee o comenta no escribe, por ninguna vía;
//   - un rango editable por persona lo escribe esa persona y nadie más, y no se puede
//     suplantar desde el cuerpo de la petición;
//   - un permiso revocado deja de escribir AUNQUE el canal en vivo no se haya enterado;
//   - un enlace caducado o revocado no sirve;
//   - la contraseña de protección no sale por ninguna superficie que se sirve.

type escenarioPermisos struct {
	h   *pgtest.Harness
	t   *testing.T
	hd  *Handler
	st  *store.Store
	ctx context.Context
	doc *domain.Document
	srv *httptest.Server
	n   int
}

func nuevoEscenarioPermisos(t *testing.T, kind domain.Kind) *escenarioPermisos {
	t.Helper()
	h := pgtest.New(t, docsdb.Migrations, docsdb.MigrationsDir)
	st := store.New(h.Pool)
	e := &escenarioPermisos{h: h, t: t, st: st, ctx: tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})}
	e.hd = &Handler{
		svc: domain.NewService(st), store: st, hub: collab.NewHub(nil),
		presence: collab.NewPresenceRegistry(), inline: 256 * 1024,
		claveEnlaces: st, limitesClave: nuevosLimitesClave(),
	}
	doc, _, err := st.CreateDocument(e.ctx, domain.CreateDocumentParams{Kind: kind, Title: "Cuentas", OwnerSub: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	e.doc = doc
	for sub, rol := range map[string]domain.Role{
		"ana": domain.RoleEditor, "luis": domain.RoleEditor, "comenta": domain.RoleCommenter, "lector": domain.RoleViewer,
	} {
		e.conceder(sub, rol)
	}
	e.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		var roles []string
		if g := q.Get("grupo"); g != "" {
			roles = []string{g}
		}
		r = r.WithContext(authn.Into(r.Context(), authn.Claims{Subject: q.Get("sub"), Tenant: "acme", Roles: roles}))
		r = r.WithContext(tenancy.Into(r.Context(), tenancy.Tenant{Slug: "acme"}))
		rc := chi.NewRouteContext()
		rc.URLParams.Add("id", doc.ID)
		e.hd.collabWS(w, r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rc)))
	}))
	t.Cleanup(e.srv.Close)
	return e
}

func (e *escenarioPermisos) conceder(sub string, rol domain.Role) {
	e.t.Helper()
	if err := e.st.UpsertPermission(e.ctx, e.doc.ID, domain.Permission{
		PrincipalKind: domain.PrincipalUser, PrincipalID: sub, Role: rol, GrantedBy: "owner",
	}); err != nil {
		e.t.Fatal(err)
	}
}

// peticion arma una petición REST de `sub` sobre el documento.
func (e *escenarioPermisos) peticion(metodo, sub, cuerpo string, extra map[string]string, objetivo string) *http.Request {
	params := map[string]string{"id": e.doc.ID}
	for k, v := range extra {
		params[k] = v
	}
	r := req(e.t, metodo, "/v1/documents/"+e.doc.ID+objetivo, &authn.Claims{Subject: sub, Tenant: "acme"}, params, cuerpo)
	return r.WithContext(tenancy.Into(r.Context(), tenancy.Tenant{Slug: "acme"}))
}

func (e *escenarioPermisos) seq() int64 {
	e.t.Helper()
	d, err := e.st.GetDocument(e.ctx, e.doc.ID)
	if err != nil {
		e.t.Fatal(err)
	}
	return d.Seq
}

// valorHoja es el set de doc:sheet con el modelo dado (un string JSON, como lo guarda el editor).
func (e *escenarioPermisos) setHoja(modelo string) (string, string) {
	e.n++
	v, _ := json.Marshal(modelo)
	op := fmt.Sprintf(`{"kind":"set","dot":{"a":"c","c":%d},"key":"doc:sheet","value":%s,"lamport":%d}`, e.n, v, e.n)
	return op, fmt.Sprintf(`{"ops":[%s]`, op)
}

// postOps envía un set de hoja por POST /ops y devuelve la respuesta.
func (e *escenarioPermisos) postOps(sub, modelo, extra string) *httptest.ResponseRecorder {
	_, cuerpo := e.setHoja(modelo)
	rec := httptest.NewRecorder()
	e.hd.postOps(rec, e.peticion("POST", sub, cuerpo+extra+"}", nil, ""))
	return rec
}

// ws abre el canal en vivo de `sub`.
type canalWS struct {
	t    *testing.T
	conn *websocket.Conn
	ctx  context.Context
}

func (e *escenarioPermisos) abrirWS(sub, grupo string) *canalWS {
	e.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	e.t.Cleanup(cancel)
	url := "ws" + strings.TrimPrefix(e.srv.URL, "http") + "/?sub=" + sub
	if grupo != "" {
		url += "&grupo=" + grupo
	}
	conn, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(func() { _ = conn.CloseNow() })
	return &canalWS{t: e.t, conn: conn, ctx: ctx}
}

// envia manda un set de hoja y espera el ack o el error; (nil, cerrado=true) si el canal se cerró.
func (e *escenarioPermisos) envia(c *canalWS, modelo string, p *domain.Proteccion) (m collab.ServerMessage, cerrado bool) {
	e.t.Helper()
	e.n++
	v, _ := json.Marshal(modelo)
	op := domain.Op{Kind: domain.OpSet, Key: "doc:sheet", Value: v, Lamport: uint64(e.n), Dot: domain.Dot{Actor: "c", Counter: uint64(e.n)}}
	if err := wsjson.Write(c.ctx, c.conn, collab.ClientMessage{Type: collab.MsgCData, Ops: []domain.Op{op}, Proteccion: p}); err != nil {
		return m, true
	}
	return c.siguiente()
}

func (c *canalWS) siguiente() (collab.ServerMessage, bool) {
	ctx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
	defer cancel()
	for {
		var m collab.ServerMessage
		if err := wsjson.Read(ctx, c.conn, &m); err != nil {
			return m, true
		}
		if m.Type == collab.MsgAck || m.Type == collab.MsgError {
			return m, false
		}
	}
}

func hojaConRangos(b1, b2, a1, protect string) string {
	return fmt.Sprintf(`{"active":0,"sheets":[{"name":"H","cells":{"A1":%q,"B2":%q,"B4":%q}%s}]}`, a1, b1, b2, protect)
}

// Quien solo lee o comenta no escribe: ni por POST /ops ni por el WebSocket, y el documento
// no cambia (ni seq ni contenido).
func TestPermisosLectoresYComentaristasNoEscribenPorNingunaVia(t *testing.T) {
	e := nuevoEscenarioPermisos(t, domain.KindSpreadsheet)
	base := hojaConRangos("1", "2", "3", "")
	if rec := e.postOps("ana", base, ""); rec.Code != http.StatusOK {
		t.Fatalf("un editor escribe: %d %s", rec.Code, rec.Body)
	}
	antes := e.seq()
	for _, sub := range []string{"lector", "comenta"} {
		rec := e.postOps(sub, hojaConRangos("X", "2", "3", ""), "")
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s escribió por REST: %d %s", sub, rec.Code, rec.Body)
		}
		c := e.abrirWS(sub, "")
		m, cerrado := e.envia(c, hojaConRangos("X", "2", "3", ""), nil)
		if cerrado || m.Type != collab.MsgError || m.Code != "forbidden" {
			t.Fatalf("%s escribió por WS: %+v cerrado=%v", sub, m, cerrado)
		}
	}
	// Quien no tiene ningún permiso, ni siquiera entra.
	if rec := e.postOps("intruso", base, ""); rec.Code != http.StatusForbidden && rec.Code != http.StatusNotFound {
		t.Fatalf("quien no tiene permiso escribió: %d", rec.Code)
	}
	if e.seq() != antes {
		t.Fatal("una escritura denegada movió el seq del documento")
	}
	// Comentar sí (el comentarista) y el lector no: la frontera es por rol, no por vía.
	rec := httptest.NewRecorder()
	e.hd.createComment(rec, e.peticion("POST", "comenta", `{"body":"hola"}`, nil, "/comments"))
	if rec.Code >= 300 {
		t.Fatalf("el comentarista no pudo comentar: %d %s", rec.Code, rec.Body)
	}
	rec = httptest.NewRecorder()
	e.hd.createComment(rec, e.peticion("POST", "lector", `{"body":"hola"}`, nil, "/comments"))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("el lector comentó: %d", rec.Code)
	}
}

// Rangos editables por persona de extremo a extremo: lo concede el propietario, lo escribe
// esa persona (por REST y por WS) y nadie más, ni diciendo ser ella en el cuerpo.
func TestPermisosRangosEditablesPorPersonaDeExtremoAExtremo(t *testing.T) {
	e := nuevoEscenarioPermisos(t, domain.KindSpreadsheet)
	sinProteger := hojaConRangos("2", "4", "1", "")
	if rec := e.postOps("owner", sinProteger, ""); rec.Code != http.StatusOK {
		t.Fatalf("siembra: %d %s", rec.Code, rec.Body)
	}
	const protect = `,"protect":{"ranges":[{"ref":"B2:B3","users":["ana"]},{"ref":"B4","groups":["contables"]}]}`
	// Quien protege es el propietario; con la hoja ya protegida, sus rangos no los cambia un editor.
	if rec := e.postOps("owner", hojaConRangos("2", "4", "1", protect), ""); rec.Code != http.StatusOK {
		t.Fatalf("proteger con rangos: %d %s", rec.Code, rec.Body)
	}
	// Ana escribe su rango (B2); no escribe fuera de él (A1) ni en el de otro (B4).
	if rec := e.postOps("ana", hojaConRangos("20", "4", "1", protect), ""); rec.Code != http.StatusOK {
		t.Fatalf("ana escribe su rango: %d %s", rec.Code, rec.Body)
	}
	for nombre, modelo := range map[string]string{
		"A1 (bloqueada)":     hojaConRangos("20", "4", "HACK", protect),
		"B4 (rango de otro)": hojaConRangos("20", "HACK", "1", protect),
	} {
		if rec := e.postOps("ana", modelo, ""); rec.Code != http.StatusForbidden {
			t.Fatalf("ana escribió %s: %d %s", nombre, rec.Code, rec.Body)
		}
	}
	// Luis no figura en ningún rango: nada. Y no puede hacerse pasar por ana desde el cuerpo.
	if rec := e.postOps("luis", hojaConRangos("HACK", "4", "1", protect), ""); rec.Code != http.StatusForbidden {
		t.Fatalf("luis escribió el rango de ana: %d %s", rec.Code, rec.Body)
	}
	for _, suplanta := range []string{
		`,"proteccion":{"Quien":{"Sub":"ana"}}`,
		`,"proteccion":{"quien":{"sub":"ana"}}`,
		`,"author_sub":"ana","sub":"ana","subject":{"sub":"ana"}`,
	} {
		if rec := e.postOps("luis", hojaConRangos("HACK", "4", "1", protect), suplanta); rec.Code == http.StatusOK {
			t.Fatalf("luis se hizo pasar por ana con %s", suplanta)
		}
	}
	// Por el canal en vivo, igual; y el grupo concede por la sesión, no por lo que diga el mensaje.
	ana := e.abrirWS("ana", "")
	if m, cerrado := e.envia(ana, hojaConRangos("21", "4", "1", protect), nil); cerrado || m.Type != collab.MsgAck {
		t.Fatalf("ana por WS en su rango: %+v", m)
	}
	if m, _ := e.envia(ana, hojaConRangos("21", "4", "HACK", protect), nil); m.Type != collab.MsgError || m.Code != "protected" {
		t.Fatalf("ana por WS fuera de su rango: %+v", m)
	}
	luis := e.abrirWS("luis", "")
	if m, _ := e.envia(luis, hojaConRangos("HACK", "4", "1", protect), nil); m.Type != collab.MsgError || m.Code != "protected" {
		t.Fatalf("luis por WS: %+v", m)
	}
	if m, _ := e.envia(luis, hojaConRangos("21", "HACK", "1", protect), nil); m.Type != collab.MsgError || m.Code != "protected" {
		t.Fatalf("luis sin el grupo escribió el rango del grupo: %+v", m)
	}
	contable := e.abrirWS("luis", "contables")
	if m, cerrado := e.envia(contable, hojaConRangos("21", "HOLA", "1", protect), nil); cerrado || m.Type != collab.MsgAck {
		t.Fatalf("luis con el grupo, en el rango del grupo: %+v", m)
	}
	// Cambiar los rangos es cambiar la protección: un editor no, ni con el rango propio.
	otros := `,"protect":{"ranges":[{"ref":"A1:B4","users":["ana"]}]}`
	if rec := e.postOps("ana", hojaConRangos("21", "HOLA", "1", otros), ""); rec.Code != http.StatusForbidden {
		t.Fatalf("ana se amplió el rango: %d %s", rec.Code, rec.Body)
	}
}

// Un permiso revocado deja de escribir, haya o no avisado al canal en vivo: la autorización
// se repite dentro de la transacción que escribe.
func TestPermisosRevocacionEnCaliente(t *testing.T) {
	e := nuevoEscenarioPermisos(t, domain.KindSpreadsheet)
	if rec := e.postOps("owner", hojaConRangos("1", "2", "3", ""), ""); rec.Code != http.StatusOK {
		t.Fatalf("siembra: %d %s", rec.Code, rec.Body)
	}
	ana := e.abrirWS("ana", "")
	luis := e.abrirWS("luis", "")
	for _, c := range []*canalWS{ana, luis} {
		if m, cerrado := e.envia(c, hojaConRangos("10", "2", "3", ""), nil); cerrado || m.Type != collab.MsgAck {
			t.Fatalf("antes de revocar escriben: %+v", m)
		}
	}
	antes := e.seq()

	// Luis: revocado por el almacén, SIN avisar al canal (otra réplica, un enlace que caduca):
	// su conexión aún cree que es editor y la escritura se deniega igualmente.
	if err := e.st.RevokePermission(e.ctx, e.doc.ID, domain.PrincipalUser, "luis"); err != nil {
		t.Fatal(err)
	}
	if m, cerrado := e.envia(luis, hojaConRangos("HACK", "2", "3", ""), nil); !cerrado && m.Type == collab.MsgAck {
		t.Fatalf("luis escribió con el permiso ya revocado: %+v", m)
	}
	if rec := e.postOps("luis", hojaConRangos("HACK", "2", "3", ""), ""); rec.Code != http.StatusForbidden {
		t.Fatalf("luis por REST con el permiso revocado: %d", rec.Code)
	}

	// Ana: revocada por la ruta del propietario, que corta su canal en vivo.
	rec := httptest.NewRecorder()
	e.hd.revokePermission(rec, e.peticion("DELETE", "owner", "", nil, "/permissions?principal_kind=user&principal_id=ana"))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("revocar: %d %s", rec.Code, rec.Body)
	}
	if m, cerrado := e.envia(ana, hojaConRangos("HACK", "2", "3", ""), nil); !cerrado && m.Type == collab.MsgAck {
		t.Fatalf("ana escribió con el permiso revocado: %+v", m)
	}
	if rec := e.postOps("ana", hojaConRangos("HACK", "2", "3", ""), ""); rec.Code != http.StatusForbidden {
		t.Fatalf("ana por REST con el permiso revocado: %d", rec.Code)
	}
	if e.seq() != antes {
		t.Fatalf("se escribió tras revocar: seq %d -> %d", antes, e.seq())
	}

	// Rebajar de editor a lector corta la escritura sin cortar la lectura.
	e.conceder("comenta", domain.RoleEditor)
	com := e.abrirWS("comenta", "")
	if m, cerrado := e.envia(com, hojaConRangos("11", "2", "3", ""), nil); cerrado || m.Type != collab.MsgAck {
		t.Fatalf("como editor escribe: %+v", m)
	}
	rec = httptest.NewRecorder()
	e.hd.upsertPermission(rec, e.peticion("PUT", "owner", `{"principal_kind":"user","principal_id":"comenta","role":"viewer"}`, nil, "/permissions"))
	if rec.Code >= 300 {
		t.Fatalf("rebajar: %d %s", rec.Code, rec.Body)
	}
	if m, cerrado := e.envia(com, hojaConRangos("HACK", "2", "3", ""), nil); cerrado || m.Type != collab.MsgError {
		t.Fatalf("rebajado a lector aún escribe: %+v cerrado=%v", m, cerrado)
	}
}

// Un enlace caducado, o revocado, no sirve; vigente sí. El token solo se devuelve al crearlo.
func TestPermisosEnlaceCaducadoYRevocadoNoSirven(t *testing.T) {
	e := nuevoEscenarioPermisos(t, domain.KindDocument)
	crear := func(extra string) (token, id string) {
		rec := httptest.NewRecorder()
		e.hd.createShare(rec, e.peticion("POST", "owner", `{"role":"viewer"`+extra+`}`, nil, "/shares"))
		if rec.Code != http.StatusCreated {
			t.Fatalf("crear el enlace: %d %s", rec.Code, rec.Body)
		}
		var out struct {
			Share struct {
				ID    string `json:"id"`
				Token string `json:"token"`
			} `json:"share"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.Share.Token == "" {
			t.Fatalf("el token no vuelve al crearlo: %v %s", err, rec.Body)
		}
		return out.Share.Token, out.Share.ID
	}
	abrir := func(token string) int {
		rec := httptest.NewRecorder()
		e.hd.resolveShare(rec, req(t, "GET", "/v1/share/"+token, nil, map[string]string{"token": token}, ""))
		rec2 := httptest.NewRecorder()
		e.hd.viewShare(rec2, req(t, "GET", "/v1/share/"+token+"/view", nil, map[string]string{"token": token}, ""))
		if rec.Code != rec2.Code {
			t.Fatalf("la ficha (%d) y la vista (%d) del enlace no coinciden", rec.Code, rec2.Code)
		}
		return rec.Code
	}
	vigente, _ := crear("")
	if c := abrir(vigente); c != http.StatusOK {
		t.Fatalf("un enlace vigente: %d", c)
	}
	if c := abrir("no-es-un-token"); c == http.StatusOK {
		t.Fatal("un token inventado abre el documento")
	}
	caduca, _ := crear(`,"expires_at":"` + time.Now().Add(1500*time.Millisecond).UTC().Format(time.RFC3339Nano) + `"`)
	if c := abrir(caduca); c != http.StatusOK {
		t.Fatalf("antes de caducar sirve: %d", c)
	}
	time.Sleep(1700 * time.Millisecond)
	if c := abrir(caduca); c != http.StatusGone {
		t.Fatalf("caducado debía ser 410: %d", c)
	}
	token, id := crear("")
	rec := httptest.NewRecorder()
	e.hd.revokeShare(rec, e.peticion("DELETE", "owner", "", map[string]string{"linkId": id}, "/shares/"+id))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("revocar: %d %s", rec.Code, rec.Body)
	}
	if c := abrir(token); c == http.StatusOK {
		t.Fatalf("un enlace revocado sigue sirviendo: %d", c)
	}
	// Y quien no es propietario no crea enlaces, ni siquiera siendo editor.
	rec = httptest.NewRecorder()
	e.hd.createShare(rec, e.peticion("POST", "ana", `{"role":"editor"}`, nil, "/shares"))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("un editor creó un enlace: %d", rec.Code)
	}
}

// La contraseña de protección no sale por ninguna superficie que se sirve: estado, historial
// de operaciones, canal en vivo (hello/replay), vista pública por enlace y las exportaciones.
func TestPermisosLaContrasenaDeProteccionNoSaleDeNingunaSuperficie(t *testing.T) {
	e := nuevoEscenarioPermisos(t, domain.KindSpreadsheet)
	const secreto = "s3creto-permisos-8c2f"
	modelo := func(a1, protect string) string { return hojaConRangos("1", "2", a1, protect) }
	if rec := e.postOps("owner", modelo("1", ""), ""); rec.Code != http.StatusOK {
		t.Fatalf("siembra: %d %s", rec.Code, rec.Body)
	}
	// Un cliente anterior mandaba la contraseña DENTRO del modelo y de nuevo en el sobre.
	protegida := modelo("1", `,"protect":{"password":"`+secreto+`","formatCells":true}`)
	if rec := e.postOps("ana", protegida, `,"proteccion":{"new":{"hoja:H":"`+secreto+`"}}`); rec.Code != http.StatusOK {
		t.Fatalf("proteger: %d %s", rec.Code, rec.Body)
	}
	// Y un intento fallido con la contraseña mala no la deja en ninguna parte.
	_ = e.postOps("luis", modelo("HACK", `,"protect":{"hasPassword":true}`), `,"proteccion":{"password":"mala-`+secreto+`"}`)

	sin := func(superficie, cuerpo string) {
		t.Helper()
		if strings.Contains(cuerpo, secreto) {
			t.Fatalf("%s lleva la contraseña: %.300s", superficie, cuerpo)
		}
		if strings.Contains(cuerpo, "hashValue") || strings.Contains(cuerpo, "saltValue") {
			t.Fatalf("%s lleva el hash de la contraseña: %.300s", superficie, cuerpo)
		}
	}
	for _, quien := range []string{"lector", "luis", "ana", "owner"} {
		rec := httptest.NewRecorder()
		e.hd.getState(rec, e.peticion("GET", quien, "", nil, "/state"))
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "hasPassword") {
			t.Fatalf("GET /state de %s: %d %.200s", quien, rec.Code, rec.Body)
		}
		sin("GET /state de "+quien, rec.Body.String())
		rec = httptest.NewRecorder()
		e.hd.getOps(rec, e.peticion("GET", quien, "", nil, "/ops?since=0&limit=500"))
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /ops de %s: %d", quien, rec.Code)
		}
		sin("GET /ops de "+quien, rec.Body.String())
	}
	// El canal en vivo: el replay del hello.
	c := e.abrirWS("lector", "")
	if err := wsjson.Write(c.ctx, c.conn, collab.ClientMessage{Type: collab.MsgHello, LastSeq: 0}); err != nil {
		t.Fatal(err)
	}
	var visto strings.Builder
	for visto.Len() < 1<<20 {
		rctx, cancel := context.WithTimeout(c.ctx, 1500*time.Millisecond)
		var raw json.RawMessage
		err := wsjson.Read(rctx, c.conn, &raw)
		cancel()
		if err != nil {
			break
		}
		visto.Write(raw)
	}
	if !strings.Contains(visto.String(), "doc:sheet") {
		t.Fatalf("el replay del canal no trajo el modelo: %.200s", visto.String())
	}
	sin("el replay del canal en vivo", visto.String())
	// Lo que sale en un fichero: la vista pública por enlace y cada formato de descarga.
	loaded, err := e.st.LoadState(e.ctx, e.doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	domain.RedactarEstado(loaded.State)
	for _, f := range []domain.BlobFormat{domain.FormatHTML, domain.FormatMarkdown, domain.FormatOOXML, domain.FormatODF, domain.FormatPDF} {
		b, _, _, err := domain.Export(domain.KindSpreadsheet, "Cuentas", loaded.State, f)
		if err != nil {
			t.Fatalf("exportar %s: %v", f, err)
		}
		if strings.Contains(string(b), secreto) {
			t.Fatalf("la exportación %s lleva la contraseña", f)
		}
	}
	// Y en la base no queda sino el hash.
	var enClaro int
	if err := e.h.Pool.InTenantTx(e.ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(e.ctx, `SELECT count(*) FROM doc_operations WHERE op::text LIKE '%' || $1 || '%'`, secreto).Scan(&enClaro)
	}); err != nil {
		t.Fatal(err)
	}
	if enClaro != 0 {
		t.Fatalf("la contraseña quedó en %d operaciones del registro", enClaro)
	}
}
