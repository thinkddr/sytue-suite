// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chat

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/local/nube"
)

const (
	yoTeams  = "11111111-1111-1111-1111-111111111111"
	beaTeams = "22222222-2222-2222-2222-222222222222"
	equipo1  = "33333333-3333-3333-3333-333333333333"
	inquilin = "44444444-4444-4444-4444-444444444444"
)

// microsoftFalso es Microsoft de mentira: el OAuth (autorizar lo hace el «navegador» de
// la prueba) y la parte de Graph que usa Teams.
type microsoftFalso struct {
	t   *testing.T
	srv *httptest.Server

	mu          sync.Mutex
	peticiones  []string
	renovados   int
	caducado    bool // el primer token de acceso ya no vale (401)
	sinCanales  bool // el administrador no consintió leer los canales (403)
	enviados    []string
	editados    []string
	borrados    []string
	reacciones  []string
	leidos      []string
	cambios     []map[string]any // lo que devuelve el sondeo de chat-1
	delta       []map[string]any
	ultimoChat1 map[string]any
	reunion     string
}

func nuevoMicrosoftFalso(t *testing.T) *microsoftFalso {
	f := &microsoftFalso{t: t, reunion: "https://teams.ejemplo.eu/l/meetup-join/abc"}
	f.ultimoChat1 = mensajeGraph("m2", beaTeams, "Bea", "¿comemos?", "2026-11-20T10:01:00Z")
	f.srv = httptest.NewServer(http.HandlerFunc(f.servir))
	t.Cleanup(f.srv.Close)
	viejo := APIGraph
	APIGraph = f.srv.URL + "/v1.0"
	ajustarOAuth = func(o *nube.OAuth) {
		o.Autorizar = f.srv.URL + "/authorize"
		o.Token = f.srv.URL + "/token"
	}
	t.Cleanup(func() { APIGraph, ajustarOAuth = viejo, nil })
	return f
}

func mensajeGraph(id, de, nombre, texto, creado string) map[string]any {
	return map[string]any{
		"id": id, "messageType": "message", "createdDateTime": creado, "lastModifiedDateTime": creado,
		"from": map[string]any{"user": map[string]string{"id": de, "displayName": nombre}},
		"body": map[string]string{"contentType": "html", "content": "<p>" + texto + "</p>"},
	}
}

func (f *microsoftFalso) servir(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.peticiones = append(f.peticiones, r.Method+" "+r.URL.Path)
	if r.URL.Path == "/token" {
		_ = r.ParseForm()
		switch r.Form.Get("grant_type") {
		case "authorization_code":
			if r.Form.Get("code") != "c0d1g0" || r.Form.Get("code_verifier") == "" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			escribir(w, map[string]any{"access_token": "t1", "refresh_token": "r1", "expires_in": 3600})
		case "refresh_token":
			f.renovados++
			if r.Form.Get("refresh_token") == "revocado" {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = io.WriteString(w, `{"error":"invalid_grant"}`)
				return
			}
			escribir(w, map[string]any{"access_token": "t2", "refresh_token": "r2", "expires_in": 3600})
		}
		return
	}
	tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if tok != "t1" && tok != "t2" || tok == "t1" && f.caducado {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	ruta := strings.TrimPrefix(r.URL.Path, "/v1.0")
	q := r.URL.Query()
	var cuerpo map[string]any
	_ = json.NewDecoder(r.Body).Decode(&cuerpo)
	chat1 := "/chats/19:chat1@thread.v2"
	canal := "/teams/" + equipo1 + "/channels/19:general@thread.tacv2"
	switch {
	case ruta == "/me":
		escribir(w, map[string]any{"id": yoTeams, "displayName": "Ana", "mail": "ana@empresa.eu"})
	case ruta == "/me/chats" && q.Get("$orderby") != "":
		escribir(w, map[string]any{"value": []any{
			map[string]any{"id": "19:chat1@thread.v2", "lastMessagePreview": f.ultimoChat1},
			map[string]any{"id": "19:grupo@thread.v2", "lastMessagePreview": mensajeGraph("g9", beaTeams, "Bea", "nuevo en el grupo", "2026-11-20T11:00:00Z")},
		}})
	case ruta == "/me/chats":
		if !strings.Contains(q.Get("$expand"), "members") {
			f.t.Errorf("los chats sin sus miembros: %s", r.URL)
		}
		escribir(w, map[string]any{"value": []any{
			map[string]any{
				"id": "19:chat1@thread.v2", "chatType": "oneOnOne", "tenantId": inquilin, "createdDateTime": "2026-11-01T09:00:00Z",
				"members":            []any{map[string]any{"userId": yoTeams, "displayName": "Ana"}, map[string]any{"userId": beaTeams, "displayName": "Bea"}},
				"lastMessagePreview": f.ultimoChat1, "viewpoint": map[string]any{"lastMessageReadDateTime": "2026-11-20T10:00:00Z"},
			},
			map[string]any{
				"id": "19:grupo@thread.v2", "chatType": "group", "topic": "Proyecto", "createdDateTime": "2026-11-02T09:00:00Z",
				"lastMessagePreview": mensajeGraph("g1", yoTeams, "Ana", "hola", "2026-11-19T10:00:00Z"),
			},
			map[string]any{"id": "19:oculto@thread.v2", "chatType": "group", "topic": "Oculto", "viewpoint": map[string]any{"isHidden": true}},
		}, "@odata.nextLink": ""})
	case ruta == chat1+"/messages" && r.Method == http.MethodGet && q.Get("$filter") != "" && strings.HasPrefix(q.Get("$filter"), "lastModifiedDateTime gt"):
		escribir(w, map[string]any{"value": f.cambios})
		f.cambios = nil
	case ruta == chat1+"/messages" && r.Method == http.MethodGet:
		if q.Get("$orderby") != "createdDateTime desc" {
			f.t.Errorf("orden de los mensajes: %s", r.URL)
		}
		borrado := mensajeGraph("m0", beaTeams, "Bea", "borrado", "2026-11-20T09:59:00Z")
		borrado["deletedDateTime"] = "2026-11-20T10:00:30Z"
		sistema := mensajeGraph("s1", "", "", "Ana añadió a Bea", "2026-11-20T09:58:00Z")
		sistema["messageType"] = "systemEventMessage"
		m1 := mensajeGraph("m1", yoTeams, "Ana", "hola <b>Bea</b>", "2026-11-20T10:00:00Z")
		m1["reactions"] = []any{map[string]any{"reactionType": "like", "user": map[string]any{"user": map[string]string{"id": beaTeams}}}}
		escribir(w, map[string]any{"value": []any{f.ultimoChat1, m1, borrado, sistema}}) // el más nuevo primero
	case ruta == chat1+"/messages" && r.Method == http.MethodPost:
		texto := cuerpo["body"].(map[string]any)["content"].(string)
		f.enviados = append(f.enviados, "chat1|"+texto)
		escribir(w, map[string]any{
			"id": "m9", "createdDateTime": "2026-11-20T10:05:00Z", "messageType": "message",
			"from": map[string]any{"user": map[string]string{"id": yoTeams, "displayName": "Ana"}}, "body": map[string]string{"contentType": "text", "content": texto},
		})
	case ruta == chat1+"/messages/m1" && r.Method == http.MethodPatch:
		f.editados = append(f.editados, cuerpo["body"].(map[string]any)["content"].(string))
		w.WriteHeader(http.StatusNoContent)
	case ruta == chat1+"/messages/m1" && r.Method == http.MethodGet:
		m := mensajeGraph("m1", yoTeams, "Ana", "editado", "2026-11-20T10:00:00Z")
		m["lastEditedDateTime"] = "2026-11-20T10:06:00Z"
		escribir(w, m)
	case ruta == "/users/"+yoTeams+chat1+"/messages/m1/softDelete":
		f.borrados = append(f.borrados, "m1")
		w.WriteHeader(http.StatusNoContent)
	case ruta == chat1+"/messages/m2/setReaction":
		f.reacciones = append(f.reacciones, "+"+cuerpo["reactionType"].(string))
		w.WriteHeader(http.StatusNoContent)
	case ruta == chat1+"/markChatReadForUser":
		u := cuerpo["user"].(map[string]any)
		f.leidos = append(f.leidos, u["id"].(string)+"@"+u["tenantId"].(string))
		w.WriteHeader(http.StatusNoContent)
	case ruta == "/me/joinedTeams":
		escribir(w, map[string]any{"value": []any{map[string]any{"id": equipo1, "displayName": "Ventas"}}})
	case ruta == "/teams/"+equipo1+"/channels":
		escribir(w, map[string]any{"value": []any{map[string]any{"id": "19:general@thread.tacv2", "displayName": "General", "membershipType": "standard"}}})
	case strings.HasPrefix(ruta, canal) && f.sinCanales:
		w.WriteHeader(http.StatusForbidden)
	case ruta == canal+"/messages" && r.Method == http.MethodGet:
		if q.Get("$expand") != "replies" {
			f.t.Errorf("sin las respuestas: %s", r.URL)
		}
		raiz := mensajeGraph("c1", beaTeams, "Bea", "plan del trimestre", "2026-11-18T09:00:00Z")
		raiz["replies"] = []any{
			mensajeGraph("c2", yoTeams, "Ana", "de acuerdo", "2026-11-18T09:05:00Z"),
			mensajeGraph("c3", beaTeams, "Bea", "gracias", "2026-11-18T09:06:00Z"),
		}
		escribir(w, map[string]any{"value": []any{raiz}})
	case ruta == canal+"/messages/delta":
		if q.Get("$deltatoken") == "" && q.Get("$skiptoken") == "" && !strings.HasPrefix(q.Get("$filter"), "lastModifiedDateTime gt") {
			f.t.Errorf("la primera vuelta de delta sin fecha: %s", r.URL)
		}
		if q.Get("$deltatoken") == "" && q.Get("$skiptoken") == "" {
			escribir(w, map[string]any{"value": []any{}, "@odata.nextLink": f.srv.URL + "/v1.0" + canal + "/messages/delta?$skiptoken=p2"})
			return
		}
		valor := f.delta
		f.delta = nil
		escribir(w, map[string]any{"value": valor, "@odata.deltaLink": f.srv.URL + "/v1.0" + canal + "/messages/delta?$deltatoken=d1"})
	case ruta == canal+"/messages/c1/replies" && r.Method == http.MethodPost:
		f.enviados = append(f.enviados, "c1|"+cuerpo["body"].(map[string]any)["content"].(string))
		escribir(w, map[string]any{"id": "c4", "replyToId": "c1", "createdDateTime": "2026-11-20T10:07:00Z", "from": map[string]any{"user": map[string]string{"id": yoTeams}}})
	case ruta == "/me/onlineMeetings":
		if cuerpo["subject"] == "" || cuerpo["startDateTime"] == nil {
			f.t.Errorf("reunión sin asunto ni hora: %v", cuerpo)
		}
		escribir(w, map[string]any{"joinWebUrl": f.reunion})
	case ruta == "/communications/getPresencesByUserId":
		escribir(w, map[string]any{"value": []any{map[string]any{"id": beaTeams, "availability": "Busy", "activity": "InACall"}}})
	default:
		f.t.Errorf("petición inesperada: %s %s", r.Method, r.URL)
		w.WriteHeader(http.StatusNotFound)
	}
}

// navegadorQueAcepta es la persona que entra y acepta: sigue la redirección con el
// código, como haría el navegador.
func navegadorQueAcepta(t *testing.T, alcances *string) func(string) error {
	return func(destino string) error {
		u, err := url.Parse(destino)
		if err != nil {
			return err
		}
		q := u.Query()
		if q.Get("code_challenge_method") != "S256" {
			t.Error("sin PKCE", q)
		}
		if alcances != nil {
			*alcances = q.Get("scope")
		}
		go func() {
			resp, err := http.Get(q.Get("redirect_uri") + "?code=c0d1g0&state=" + url.QueryEscape(q.Get("state")))
			if err == nil {
				resp.Body.Close()
			}
		}()
		return nil
	}
}

// tmRotulo es el rótulo en el carril de la cuenta abierta otra vez.
func tmRotulo(t *testing.T, reg *Registro, c Cuenta) string {
	t.Helper()
	p, err := reg.Abrir(c)
	if err != nil {
		t.Fatal(err)
	}
	return p.Rotulo()
}

// altaTeams da de alta la cuenta de la prueba por OAuth y la abre.
func altaTeams(t *testing.T, canales bool) (*Registro, Cuenta, *Teams, *secretosMemoria) {
	t.Helper()
	sec := &secretosMemoria{v: map[string]string{}}
	reg, err := AbrirRegistro(filepath.Join(t.TempDir(), "chat-cuentas.json"), sec)
	if err != nil {
		t.Fatal(err)
	}
	// Sus servidores escuchan en este equipo: sin cliente (el del portero, en la
	// edición) no se sale a la red (R-RED).
	reg.Cliente = &http.Client{}
	reg.Clientes = nube.ClientesOAuth{Microsoft: "cliente-de-la-compilacion"}
	reg.Avisos = Avisos{SinConsentimiento: "falta el consentimiento"}
	var alcances string
	c, p, err := reg.AltaTeams(t.Context(), AltaOAuth{Canales: canales}, navegadorQueAcepta(t, &alcances))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(alcances, "ChannelMessage.Read.All") != canales || !strings.Contains(alcances, "Chat.ReadWrite") {
		t.Errorf("permisos pedidos: %q", alcances)
	}
	return reg, c, p.(*Teams), sec
}

// El alta: OAuth con PKCE, quién es, la cuenta guardada sin secretos y la sesión en el
// llavero; y al abrirla otra vez, con el token de refresco.
func TestTeamsAltaPorOAuth(t *testing.T) {
	f := nuevoMicrosoftFalso(t)
	reg, c, _, sec := altaTeams(t, false)
	if c.Proveedor != ProveedorTeams || c.Rotulo != "Microsoft Teams" || c.Usuario != "ana@empresa.eu" || tmRotulo(t, reg, c) != "Teams · ana@empresa.eu" || c.ClienteID != "cliente-de-la-compilacion" || c.OAuth != "127.0.0.1" {
		t.Fatalf("cuenta: %+v", c)
	}
	if sec.v[secretoRefresco(c.ID)] != "r1" || len(sec.v) != 1 {
		t.Fatalf("llavero: %v", sec.v)
	}
	raw, _ := os.ReadFile(reg.ruta)
	if strings.Contains(string(raw), "r1") || strings.Contains(string(raw), "t1") || !strings.Contains(string(raw), f.srv.URL+"/v1.0") {
		t.Fatalf("chat-cuentas.json: %s", raw)
	}
	p, err := reg.Abrir(c)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := p.Canales(t.Context()); err != nil {
		t.Fatal(err)
	}
	if f.renovados != 1 || sec.v[secretoRefresco(c.ID)] != "r2" {
		t.Fatalf("al abrir se renueva con el de refresco y se guarda el rotado: %d %v", f.renovados, sec.v)
	}
	if err := reg.Baja(c.ID); err != nil || len(sec.v) != 0 {
		t.Fatal("quedaron secretos", sec.v, err)
	}
}

// Los chats como canales (1:1 como directo, el de grupo con tema por su nombre, el
// oculto fuera), lo no leído por la vista de cada chat, y los mensajes con su HTML en
// texto, sin los borrados ni los de sistema, con sus reacciones y lo propio como «yo».
func TestTeamsChatsYMensajes(t *testing.T) {
	nuevoMicrosoftFalso(t)
	_, _, tm, _ := altaTeams(t, false)
	ctx := t.Context()
	cs, us, err := tm.Canales(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 2 || cs[0].ID != "chat/19:chat1@thread.v2" || cs[0].Kind != "direct" || cs[0].Name != "Bea" || len(cs[0].PeerSubs) != 1 || cs[0].PeerSubs[0] != beaTeams ||
		cs[1].Kind != "private" || cs[1].Name != "Proyecto" {
		t.Fatalf("canales: %+v", cs)
	}
	if len(us) != 1 || us[0].ChannelID != "chat/19:chat1@thread.v2" || us[0].Unread != 1 {
		t.Fatalf("sin leer: %+v", us)
	}
	ms, err := tm.Mensajes(ctx, "chat/19:chat1@thread.v2", time.Time{}, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) != 2 || ms[0].ID != "m1" || ms[0].AuthorSub != Yo || ms[0].Body != "hola Bea" || ms[1].Body != "¿comemos?" || ms[1].AuthorSub != beaTeams {
		t.Fatalf("mensajes: %+v", ms)
	}
	if r := ms[0].Reactions; len(r) != 1 || r[0].Emoji != "👍" || r[0].Count != 1 {
		t.Fatalf("reacciones: %+v", r)
	}
}

// Enviar, editar, borrar y reaccionar en un chat; los chats no tienen hilos; leído.
func TestTeamsEnviarEditarBorrarEnChats(t *testing.T) {
	f := nuevoMicrosoftFalso(t)
	_, _, tm, _ := altaTeams(t, false)
	ctx := t.Context()
	if _, _, err := tm.Canales(ctx); err != nil {
		t.Fatal(err)
	}
	m, err := tm.Enviar(ctx, "chat/19:chat1@thread.v2", "", "hola")
	if err != nil || m.ID != "m9" || m.AuthorSub != Yo || m.Body != "hola" {
		t.Fatal(m, err)
	}
	if _, err := tm.Enviar(ctx, "chat/19:chat1@thread.v2", "m1", "en hilo"); !errors.Is(err, ErrSinHilosTeams) {
		t.Fatal("un chat de Teams con hilo:", err)
	}
	e, err := tm.Editar(ctx, "chat/19:chat1@thread.v2", "m1", "editado")
	if err != nil || e.Body != "editado" || e.EditedAt == nil {
		t.Fatal(e, err)
	}
	if err := tm.Borrar(ctx, "chat/19:chat1@thread.v2", "m1"); err != nil {
		t.Fatal(err)
	}
	if err := tm.Reaccionar(ctx, "chat/19:chat1@thread.v2", "m2", "👍", true); err != nil {
		t.Fatal(err)
	}
	if err := tm.MarcarLeido(ctx, "chat/19:chat1@thread.v2"); err != nil {
		t.Fatal(err)
	}
	if _, us, _ := tm.Canales(ctx); len(us) != 0 {
		t.Fatalf("sigue sin leer: %+v", us)
	}
	if _, err := tm.Editar(ctx, equipo1+"/19:general@thread.tacv2", "c1", "x"); !errors.Is(err, ErrNoDisponible) {
		t.Fatal("editar en un canal:", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if strings.Join(f.enviados, ";") != "chat1|hola" || strings.Join(f.editados, ";") != "editado" || strings.Join(f.borrados, ";") != "m1" ||
		strings.Join(f.reacciones, ";") != "+like" || strings.Join(f.leidos, ";") != yoTeams+"@"+inquilin {
		t.Fatalf("enviados %v editados %v borrados %v reacciones %v leídos %v", f.enviados, f.editados, f.borrados, f.reacciones, f.leidos)
	}
}

// El sondeo: un chat cerrado con mensaje nuevo avisa con su vista previa y cuenta como
// no leído; uno abierto trae lo nuevo, lo editado y lo borrado por fecha.
func TestTeamsSondeoPorFechaYVistaPrevia(t *testing.T) {
	f := nuevoMicrosoftFalso(t)
	_, _, tm, _ := altaTeams(t, false)
	ctx := t.Context()
	ahora := time.Date(2026, 11, 20, 10, 2, 0, 0, time.UTC)
	tm.Ahora = func() time.Time { return ahora }
	if _, _, err := tm.Canales(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := tm.Mensajes(ctx, "chat/19:chat1@thread.v2", time.Time{}, 50); err != nil {
		t.Fatal(err)
	}
	editado := mensajeGraph("m1", yoTeams, "Ana", "corregido", "2026-11-20T10:00:00Z")
	editado["lastModifiedDateTime"], editado["lastEditedDateTime"] = "2026-11-20T10:03:00Z", "2026-11-20T10:03:00Z"
	borrado := mensajeGraph("m2", beaTeams, "Bea", "", "2026-11-20T10:01:00Z")
	borrado["deletedDateTime"], borrado["lastModifiedDateTime"] = "2026-11-20T10:04:00Z", "2026-11-20T10:04:00Z"
	f.mu.Lock()
	f.cambios = []map[string]any{borrado, mensajeGraph("m3", beaTeams, "Bea", "nuevo", "2026-11-20T10:05:00Z"), editado}
	f.mu.Unlock()
	var evs []api.RoomsEvent
	n, err := tm.sondear(ctx, func(ev api.RoomsEvent) { evs = append(evs, ev) })
	if err != nil {
		t.Fatal(err)
	}
	tipos := make([]string, 0, len(evs))
	for _, ev := range evs {
		var m api.RoomsMessage
		_ = json.Unmarshal(ev.Payload, &m)
		tipos = append(tipos, ev.ChannelID+":"+ev.Type+":"+m.ID+":"+m.Body)
	}
	quiero := "chat/19:grupo@thread.v2:message:g9:nuevo en el grupo|chat/19:chat1@thread.v2:message_edited:m1:corregido|" +
		"chat/19:chat1@thread.v2:message_deleted:m2:|chat/19:chat1@thread.v2:message:m3:nuevo"
	if n != 4 || strings.Join(tipos, "|") != quiero {
		t.Fatalf("sondeo (%d):\n%s\nquiero:\n%s", n, strings.Join(tipos, "|"), quiero)
	}
	_, us, _ := tm.Canales(ctx)
	if len(us) != 2 {
		t.Fatalf("sin leer: %+v", us)
	}
	// Otra vuelta sin novedades no entrega nada.
	if n, err := tm.sondear(ctx, func(api.RoomsEvent) { t.Error("entregó de más") }); err != nil || n != 0 {
		t.Fatal(n, err)
	}
}

// Los canales de los equipos: sus raíces con el hilo resumido, responder en el hilo y el
// sondeo con `delta` (la primera vuelta por fecha y página a página, luego el enlace).
func TestTeamsCanalesConDelta(t *testing.T) {
	f := nuevoMicrosoftFalso(t)
	_, _, tm, _ := altaTeams(t, true)
	ctx := t.Context()
	cs, _, err := tm.Canales(ctx)
	if err != nil {
		t.Fatal(err)
	}
	canal := equipo1 + "/19:general@thread.tacv2"
	if len(cs) != 3 || cs[2].ID != canal || cs[2].Name != "Ventas / General" || cs[2].Kind != "public" {
		t.Fatalf("canales: %+v", cs)
	}
	ms, err := tm.Mensajes(ctx, canal, time.Time{}, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) != 1 || ms[0].ReplyCount != 2 || ms[0].LastReplyAt == nil || len(ms[0].ThreadParticipants) != 2 || ms[0].ThreadParticipants[0].Sub != beaTeams {
		t.Fatalf("raíces: %+v", ms)
	}
	if _, err := tm.Enviar(ctx, canal, "c1", "en el hilo"); err != nil {
		t.Fatal(err)
	}
	// La primera vuelta solo trae el chat de grupo (su vista previa es nueva): el canal
	// empieza su `delta` por fecha y no tiene nada.
	if _, err := tm.sondear(ctx, func(ev api.RoomsEvent) {
		if ev.ChannelID == canal {
			t.Errorf("primera vuelta de delta: %+v", ev)
		}
	}); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.delta = []map[string]any{mensajeGraph("c5", beaTeams, "Bea", "nueva raíz", "2030-01-01T00:00:00Z")}
	f.mu.Unlock()
	var evs []api.RoomsEvent
	if n, err := tm.sondear(ctx, func(ev api.RoomsEvent) { evs = append(evs, ev) }); err != nil || n != 1 || evs[0].ChannelID != canal || evs[0].Type != "message" {
		t.Fatal("delta", n, err, evs)
	}
	tm.mu.Lock()
	enlace := tm.deltas[canal]
	tm.mu.Unlock()
	if !strings.Contains(enlace, "$deltatoken=d1") {
		t.Fatalf("deltaLink: %q", enlace)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if strings.Join(f.enviados, ";") != "c1|en el hilo" {
		t.Fatal(f.enviados)
	}
}

// Sin el consentimiento del administrador para leer los canales, el canal lo dice (y
// deja de sondearse) y los chats siguen.
func TestTeamsSinConsentimientoParaLosCanales(t *testing.T) {
	f := nuevoMicrosoftFalso(t)
	f.sinCanales = true
	_, _, tm, _ := altaTeams(t, true)
	ctx := t.Context()
	if _, _, err := tm.Canales(ctx); err != nil {
		t.Fatal(err)
	}
	canal := equipo1 + "/19:general@thread.tacv2"
	ms, err := tm.Mensajes(ctx, canal, time.Time{}, 50)
	if err != nil || len(ms) != 1 || ms[0].AuthorSub != AutorSistema || ms[0].Body != "falta el consentimiento" {
		t.Fatalf("aviso: %+v %v", ms, err)
	}
	if again, err := tm.Mensajes(ctx, canal, time.Time{}, 50); err != nil || len(again) != 1 || !again[0].CreatedAt.Equal(ms[0].CreatedAt) {
		t.Fatalf("el aviso cambia al recargar: %+v %v", again, err)
	}
	if ms, err := tm.Mensajes(ctx, "chat/19:chat1@thread.v2", time.Time{}, 50); err != nil || len(ms) != 2 {
		t.Fatal("los chats siguen:", ms, err)
	}
	f.mu.Lock()
	antes := len(f.peticiones)
	f.mu.Unlock()
	if _, err := tm.sondear(ctx, func(api.RoomsEvent) {}); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, p := range f.peticiones[antes:] {
		if strings.Contains(p, "/channels/") {
			t.Errorf("se sondeó un canal sin consentimiento: %s", p)
		}
	}
}

// Un 401 (el token caducó antes de tiempo) renueva con el de refresco y repite; un token
// de refresco rechazado deja la cuenta en «volver a entrar», y volver a entrar la arregla.
func TestTeamsTokenCaducadoYRenovado(t *testing.T) {
	f := nuevoMicrosoftFalso(t)
	reg, c, tm, sec := altaTeams(t, false)
	f.mu.Lock()
	f.caducado = true
	f.mu.Unlock()
	if _, _, err := tm.Canales(t.Context()); err != nil {
		t.Fatal(err)
	}
	if f.renovados != 1 || sec.v[secretoRefresco(c.ID)] != "r2" {
		t.Fatalf("renovado %d veces, llavero %v", f.renovados, sec.v)
	}
	// El proveedor rechaza el de refresco.
	_ = sec.Guardar(secretoRefresco(c.ID), "revocado")
	p, err := reg.Abrir(c)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := p.Canales(t.Context()); !errors.Is(err, ErrReconectar) {
		t.Fatal("sin reconectar:", err)
	}
	if cs := reg.Cuentas(); cs[0].Estado != EstadoReconectar {
		t.Fatalf("estado: %+v", cs)
	}
	raw, _ := os.ReadFile(reg.ruta)
	if !strings.Contains(string(raw), `"estado": "reconectar"`) {
		t.Fatalf("no quedó escrito para Ajustes → Cuentas: %s", raw)
	}
	f.mu.Lock()
	f.caducado = false
	f.mu.Unlock()
	c2, p2, err := reg.Reconectar(t.Context(), c.ID, navegadorQueAcepta(t, nil))
	if err != nil || c2.ID != c.ID || c2.Estado != "" || sec.v[secretoRefresco(c.ID)] != "r1" {
		t.Fatal(c2, err, sec.v)
	}
	if _, _, err := p2.Canales(t.Context()); err != nil {
		t.Fatal(err)
	}
}

// Crear reunión y presencia.
func TestTeamsReunionYPresencia(t *testing.T) {
	f := nuevoMicrosoftFalso(t)
	_, _, tm, _ := altaTeams(t, false)
	enlace, err := tm.CrearReunion(t.Context(), "Bea")
	if err != nil || enlace != f.reunion {
		t.Fatal(enlace, err)
	}
	f.mu.Lock()
	f.reunion = "javascript:alert(1)"
	f.mu.Unlock()
	if _, err := tm.CrearReunion(t.Context(), "Bea"); err == nil {
		t.Fatal("se aceptó un enlace que no es https")
	}
	ps, err := tm.Presencias(t.Context(), []string{beaTeams, "no-es-de-teams"})
	if err != nil || len(ps) != 1 || ps[0].Subject != beaTeams || ps[0].Estado != api.NoMolestar {
		t.Fatal(ps, err)
	}
}

// Con la red cortada, la cuenta falla y lo dice; el Agregador sigue con las demás.
func TestTeamsRedCortada(t *testing.T) {
	f := nuevoMicrosoftFalso(t)
	_, c, tm, _ := altaTeams(t, false)
	tm.Sondeo = 10 * time.Millisecond
	f.srv.Close()
	ctx, parar := context.WithTimeout(t.Context(), 5*time.Second)
	defer parar()
	tm.mu.Lock()
	tm.listado = time.Time{}
	tm.mu.Unlock()
	if _, _, err := tm.Canales(ctx); err == nil {
		t.Fatal("sin red no falla")
	}
	if err := tm.Escuchar(ctx, func(api.RoomsEvent) {}); err == nil || ctx.Err() != nil {
		t.Fatal("la escucha no vuelve con el porqué:", err)
	}
	notas, err := AbrirNotas(t.TempDir(), "Notas", "notas", "")
	if err != nil {
		t.Fatal(err)
	}
	a := NuevoAgregador("")
	a.Poner(CuentaNotas, notas)
	a.Poner(c.ID, tm)
	cs, err := a.Channels(ctx, Equipo)
	if err != nil || len(cs) == 0 {
		t.Fatal("las notas no siguen:", cs, err)
	}
}
