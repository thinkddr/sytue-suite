// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chat

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/local/nube"
)

// googleFalso es Google de mentira: el OAuth, el `userinfo`, la API de Chat y la de Meet.
type googleFalso struct {
	t   *testing.T
	srv *httptest.Server

	mu         sync.Mutex
	renovados  int
	caducado   bool
	clientes   []string
	enviados   []string
	filtros    []string
	actividad  string // lastActiveTime de spaces/BBB
	nuevos     []map[string]any
	reunion    string
	sinLectura bool // el token no lleva el permiso de leer (403 al listar mensajes)
}

func nuevoGoogleFalso(t *testing.T) *googleFalso {
	f := &googleFalso{t: t, actividad: "2026-11-20T07:00:00Z", reunion: "https://meet.google.com/abc-defg-hij"}
	f.srv = httptest.NewServer(http.HandlerFunc(f.servir))
	t.Cleanup(f.srv.Close)
	chat, meet, usuario, personas := APIChatGoogle, APIMeet, APIUsuarioGoogle, APIPersonasGoogle
	APIChatGoogle, APIMeet, APIUsuarioGoogle, APIPersonasGoogle = f.srv.URL+"/chat/v1", f.srv.URL+"/meet/v2", f.srv.URL+"/userinfo", f.srv.URL+"/people/v1"
	ajustarOAuth = func(o *nube.OAuth) {
		o.Autorizar = f.srv.URL + "/authorize"
		o.Token = f.srv.URL + "/token"
	}
	t.Cleanup(func() {
		APIChatGoogle, APIMeet, APIUsuarioGoogle, APIPersonasGoogle, ajustarOAuth = chat, meet, usuario, personas, nil
	})
	return f
}

func mensajeGoogle(espacio, id, de, texto, creado, hilo string, respuesta bool) map[string]any {
	return map[string]any{
		"name": espacio + "/messages/" + id, "text": texto, "createTime": creado,
		"sender": map[string]string{"name": de, "displayName": map[string]string{"users/123": "Ana", "users/456": "Bea"}[de], "type": "HUMAN"},
		"thread": map[string]string{"name": espacio + "/threads/" + hilo}, "threadReply": respuesta,
	}
}

func (f *googleFalso) servir(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.URL.Path == "/token" {
		_ = r.ParseForm()
		f.clientes = append(f.clientes, r.Form.Get("client_id")+":"+r.Form.Get("client_secret"))
		if r.Form.Get("grant_type") == "refresh_token" {
			f.renovados++
			escribir(w, map[string]any{"access_token": "g2", "expires_in": 3600})
			return
		}
		escribir(w, map[string]any{"access_token": "g1", "refresh_token": "gr1", "expires_in": 3600})
		return
	}
	tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if tok != "g1" && tok != "g2" || tok == "g1" && f.caducado {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	q := r.URL.Query()
	var cuerpo map[string]any
	_ = json.NewDecoder(r.Body).Decode(&cuerpo)
	switch ruta := r.URL.Path; {
	case ruta == "/userinfo":
		escribir(w, map[string]any{"sub": "123", "email": "ana@empresa.eu", "name": "Ana"})
	case ruta == "/chat/v1/spaces":
		escribir(w, map[string]any{"spaces": []any{
			map[string]any{"name": "spaces/AAA", "displayName": "Equipo", "spaceType": "SPACE", "lastActiveTime": "2026-11-20T09:00:00Z", "spaceDetails": map[string]string{"description": "lo del equipo"}},
			map[string]any{"name": "spaces/BBB", "spaceType": "DIRECT_MESSAGE", "lastActiveTime": f.actividad},
			map[string]any{"name": "../../otra", "displayName": "trampa"},
		}})
	case ruta == "/chat/v1/spaces/AAA/messages" && r.Method == http.MethodGet:
		if f.sinLectura {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		f.filtros = append(f.filtros, q.Get("filter"))
		switch filtro := q.Get("filter"); {
		case strings.HasPrefix(filtro, "createTime > "):
			escribir(w, map[string]any{"messages": f.nuevos})
			f.nuevos = nil
		case strings.HasPrefix(filtro, "thread.name = "):
			escribir(w, map[string]any{"messages": []any{
				mensajeGoogle("spaces/AAA", "r2", "users/456", "respuesta", "2026-11-20T08:05:00Z", "h1", true),
				mensajeGoogle("spaces/AAA", "m1", "users/456", "¿quién viene?", "2026-11-20T08:00:00Z", "h1", false),
			}})
		default:
			if q.Get("orderBy") != "createTime desc" {
				f.t.Errorf("orden: %s", r.URL)
			}
			escribir(w, map[string]any{"messages": []any{ // el más nuevo primero
				mensajeGoogle("spaces/AAA", "m2", "users/123", "yo", "2026-11-20T08:10:00Z", "h2", false),
				mensajeGoogle("spaces/AAA", "r2", "users/456", "respuesta", "2026-11-20T08:05:00Z", "h1", true),
				mensajeGoogle("spaces/AAA", "m1", "users/456", "¿quién viene?", "2026-11-20T08:00:00Z", "h1", false),
			}})
		}
	case ruta == "/chat/v1/spaces/BBB/messages" && r.Method == http.MethodGet:
		escribir(w, map[string]any{"messages": []any{mensajeGoogle("spaces/BBB", "d1", "users/456", "¿tienes un momento?", f.actividad, "h9", false)}})
	case strings.HasSuffix(ruta, "/messages") && r.Method == http.MethodPost:
		hilo := ""
		if h, ok := cuerpo["thread"].(map[string]any); ok {
			hilo = h["name"].(string)
			if q.Get("messageReplyOption") != "REPLY_MESSAGE_OR_FAIL" {
				f.t.Errorf("respuesta sin su opción: %s", r.URL)
			}
		}
		espacio := strings.TrimSuffix(strings.TrimPrefix(ruta, "/chat/v1/"), "/messages")
		f.enviados = append(f.enviados, espacio+"|"+hilo+"|"+cuerpo["text"].(string))
		escribir(w, map[string]any{
			"name": espacio + "/messages/n1", "text": cuerpo["text"], "createTime": "2026-11-20T08:20:00Z",
			"sender": map[string]string{"name": "users/123"}, "thread": map[string]string{"name": cmpOr(hilo, espacio+"/threads/n1")},
		})
	case ruta == "/chat/v1/spaces/AAA/members":
		escribir(w, map[string]any{"memberships": []any{
			map[string]any{"role": "ROLE_MANAGER", "member": map[string]string{"name": "users/123", "displayName": "Ana"}},
			map[string]any{"role": "ROLE_MEMBER", "member": map[string]string{"name": "users/456", "displayName": "Bea"}},
		}})
	case ruta == "/chat/v1/spaces/BBB/members":
		// Con la autenticación de la persona, Google no da los nombres: los da People.
		escribir(w, map[string]any{"memberships": []any{
			map[string]any{"member": map[string]string{"name": "users/123", "type": "HUMAN"}},
			map[string]any{"member": map[string]string{"name": "users/456", "type": "HUMAN"}},
		}})
	case ruta == "/people/v1/people/456" && q.Get("personFields") == "names,emailAddresses":
		escribir(w, map[string]any{"names": []any{map[string]string{"displayName": "Bea Gil"}}, "emailAddresses": []any{map[string]string{"value": "bea@empresa.eu"}}})
	case ruta == "/meet/v2/spaces" && r.Method == http.MethodPost:
		escribir(w, map[string]any{"name": "spaces/xyz", "meetingUri": f.reunion})
	default:
		f.t.Errorf("petición inesperada: %s %s", r.Method, r.URL)
		w.WriteHeader(http.StatusNotFound)
	}
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// altaGoogle da de alta la cuenta de la prueba (con cliente propio si cliente no está
// vacío) y la abre.
func altaGoogle(t *testing.T, cliente string) (*Registro, Cuenta, *GoogleChat, *secretosMemoria, string) {
	t.Helper()
	sec := &secretosMemoria{v: map[string]string{}}
	reg, err := AbrirRegistro(filepath.Join(t.TempDir(), "chat-cuentas.json"), sec)
	if err != nil {
		t.Fatal(err)
	}
	// Sus servidores escuchan en este equipo: sin cliente (el del portero, en la
	// edición) no se sale a la red (R-RED).
	reg.Cliente = &http.Client{}
	reg.Clientes = nube.ClientesOAuth{Google: "compilado", GoogleSecreto: "secreto-compilado"}
	reg.Avisos = Avisos{SinLectura: "para leer, un cliente propio"}
	var alcances string
	c, p, err := reg.AltaGoogle(t.Context(), AltaOAuth{Cliente: cliente, Secreto: "secreto-propio"}, navegadorQueAcepta(t, &alcances))
	if err != nil {
		t.Fatal(err)
	}
	return reg, c, p.(*GoogleChat), sec, alcances
}

// Con el cliente de la compilación (D-22) se escribe y no se lee: cada espacio lo dice y
// enseña lo mandado desde aquí; no se pide el permiso restringido ni se sondea.
func TestGoogleChatSinLecturaEnvia(t *testing.T) {
	f := nuevoGoogleFalso(t)
	_, c, g, sec, alcances := altaGoogle(t, "")
	if c.Lectura || c.ClienteID != "compilado" || c.Rotulo != "Google Chat" || g.Rotulo() != "Google Chat · ana@empresa.eu" || len(sec.v) != 1 {
		t.Fatalf("cuenta %+v, llavero %v", c, sec.v)
	}
	if strings.Contains(alcances, "chat.messages.readonly") || !strings.Contains(alcances, "chat.messages.create") || !strings.Contains(alcances, "meetings.space.created") {
		t.Fatalf("permisos: %q", alcances)
	}
	ctx := t.Context()
	cs, _, err := g.Canales(ctx)
	if err != nil || len(cs) != 2 || cs[0].Purpose != "lo del equipo" || cs[1].Kind != "direct" {
		t.Fatal(cs, err)
	}
	if _, err := g.Enviar(ctx, "spaces/AAA", "", "hola"); err != nil {
		t.Fatal(err)
	}
	ms, err := g.Mensajes(ctx, "spaces/AAA", time.Time{}, 50)
	if err != nil || len(ms) != 2 || ms[0].AuthorSub != AutorSistema || ms[1].Body != "hola" || ms[1].AuthorSub != Yo {
		t.Fatalf("mensajes: %+v %v", ms, err)
	}
	if _, err := g.Hilo(ctx, "spaces/AAA", "n1"); !errors.Is(err, ErrNoDisponible) {
		t.Fatal(err)
	}
	if ms, _ := g.Miembros(ctx, "spaces/AAA"); len(ms) != 1 || ms[0].Subject != Yo {
		t.Fatal(ms)
	}
	escucha, parar := context.WithTimeout(ctx, 50*time.Millisecond)
	defer parar()
	if err := g.Escuchar(escucha, func(api.RoomsEvent) { t.Error("sondeó sin poder leer") }); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if strings.Join(f.enviados, ";") != "spaces/AAA||hola" || f.clientes[0] != "compilado:secreto-compilado" {
		t.Fatal(f.enviados, f.clientes)
	}
}

// Con el cliente propio del Workspace se lee: raíces con sus respuestas contadas, el
// hilo, responder en él, quién está y el sondeo por fecha.
func TestGoogleChatConClientePropioLee(t *testing.T) {
	f := nuevoGoogleFalso(t)
	_, c, g, sec, alcances := altaGoogle(t, "cliente-del-workspace")
	if !c.Lectura || sec.v[secretoCliente(c.ID)] != "secreto-propio" || !strings.Contains(alcances, "chat.messages.readonly") {
		t.Fatalf("cuenta %+v, llavero %v, permisos %q", c, sec.v, alcances)
	}
	ctx := t.Context()
	g.Ahora = func() time.Time { return time.Date(2026, 11, 20, 8, 15, 0, 0, time.UTC) }
	cs, _, err := g.Canales(ctx)
	if err != nil || len(cs) != 2 || cs[0].ID != "spaces/AAA" || cs[0].Name != "Equipo" || cs[0].Purpose != "lo del equipo" || cs[1].Name != "Bea Gil" {
		t.Fatal(cs, err)
	}
	ms, err := g.Mensajes(ctx, "spaces/AAA", time.Time{}, 50)
	if err != nil || len(ms) != 2 || ms[0].ID != "m1" || ms[0].ReplyCount != 1 || ms[1].AuthorSub != Yo || ms[0].AuthorName != "Bea" {
		t.Fatalf("mensajes: %+v %v", ms, err)
	}
	h, err := g.Hilo(ctx, "spaces/AAA", "m1")
	if err != nil || h.Root.ID != "m1" || len(h.Replies) != 1 || *h.Replies[0].ParentID != "m1" {
		t.Fatal(h, err)
	}
	if _, err := g.Enviar(ctx, "spaces/AAA", "m1", "voy"); err != nil {
		t.Fatal(err)
	}
	if ms, err := g.Miembros(ctx, "spaces/AAA"); err != nil || len(ms) != 2 || ms[0].Subject != Yo || ms[0].Role != "admin" {
		t.Fatal(ms, err)
	}
	// El sondeo: lo nuevo del abierto por fecha, y el último del directo, que tuvo
	// actividad, como no leído.
	f.mu.Lock()
	f.nuevos = []map[string]any{mensajeGoogle("spaces/AAA", "m3", "users/456", "otra", "2026-11-20T08:30:00Z", "h3", false)}
	f.actividad = "2026-11-20T08:40:00Z"
	f.mu.Unlock()
	var evs []string
	n, err := g.sondear(ctx, func(ev api.RoomsEvent) {
		var m api.RoomsMessage
		_ = json.Unmarshal(ev.Payload, &m)
		evs = append(evs, ev.ChannelID+":"+m.ID)
	})
	if err != nil || n != 2 || strings.Join(evs, ",") != "spaces/AAA:m3,spaces/BBB:d1" {
		t.Fatal(n, err, evs)
	}
	if _, us, _ := g.Canales(ctx); len(us) != 1 || us[0].ChannelID != "spaces/BBB" {
		t.Fatal(us)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if strings.Join(f.enviados, ";") != "spaces/AAA|spaces/AAA/threads/h1|voy" {
		t.Fatal(f.enviados)
	}
	if !slicesContiene(f.filtros, `createTime > "2026-11-20T08:20:00Z"`) || !slicesContiene(f.filtros, "thread.name = spaces/AAA/threads/h1") {
		t.Fatal("filtros:", f.filtros)
	}
}

func slicesContiene(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

// Meet: crear el enlace, y nada que no sea de Meet.
func TestGoogleMeetCreaElEnlace(t *testing.T) {
	f := nuevoGoogleFalso(t)
	_, _, g, _, _ := altaGoogle(t, "")
	if u, err := g.CrearReunion(t.Context(), ""); err != nil || u != "https://meet.google.com/abc-defg-hij" {
		t.Fatal(u, err)
	}
	f.mu.Lock()
	f.reunion = "https://evil.example/abc"
	f.mu.Unlock()
	if _, err := g.CrearReunion(t.Context(), ""); err == nil {
		t.Fatal("se aceptó un enlace que no es de Meet")
	}
}

// Un 401 renueva el token y repite; con la red cortada, la cuenta lo dice.
func TestGoogleChatTokenCaducadoYRedCortada(t *testing.T) {
	f := nuevoGoogleFalso(t)
	_, _, g, _, _ := altaGoogle(t, "cliente-del-workspace")
	f.mu.Lock()
	f.caducado = true
	f.mu.Unlock()
	if _, _, err := g.Canales(t.Context()); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	if f.renovados != 1 || f.clientes[len(f.clientes)-1] != "cliente-del-workspace:secreto-propio" {
		t.Fatal(f.renovados, f.clientes)
	}
	f.mu.Unlock()
	f.srv.Close()
	if _, _, err := g.Canales(t.Context()); err == nil {
		t.Fatal("sin red no falla")
	}
}

// El Agregador: «Reunión» en un canal crea la del proveedor de su cuenta o, si la cuenta
// no las tiene (unas notas, Slack), la de la primera que sí, y publica el enlace en ese
// canal; la presencia se pregunta a quien la tiene.
func TestAgregadorReunionesYPresencia(t *testing.T) {
	nuevoGoogleFalso(t)
	_, cg, g, _, _ := altaGoogle(t, "")
	notas, err := AbrirNotas(t.TempDir(), "Notas", "notas", "")
	if err != nil {
		t.Fatal(err)
	}
	a := NuevoAgregador("")
	a.Poner(CuentaNotas, notas)
	cs, err := a.Channels(t.Context(), Equipo)
	if err != nil || len(cs) == 0 {
		t.Fatal(cs, err)
	}
	canalNotas := cs[0].ID
	if p := a.ProveedorReunion(canalNotas); p != "" {
		t.Fatal("sin cuentas de reuniones ofrece", p)
	}
	if _, err := a.CrearReunion(t.Context(), canalNotas, "x"); !errors.Is(err, ErrNoDisponible) {
		t.Fatal(err)
	}
	a.Poner(cg.ID, g)
	if p := a.ProveedorReunion(canalNotas); p != "Google Meet" {
		t.Fatal(p)
	}
	m, err := a.CrearReunion(t.Context(), canalNotas, "notas")
	if err != nil || m.Body != "https://meet.google.com/abc-defg-hij" || m.ChannelID != canalNotas {
		t.Fatal(m, err)
	}
	if p := a.ProveedorReunion(IDCanal(cg.ID, "spaces/AAA")); p != "Google Meet" {
		t.Fatal(p)
	}
	ps, err := a.Presencias(t.Context(), "alguien")
	if err != nil || len(ps) != 0 {
		t.Fatal("presencia de nadie:", ps, err)
	}
}
