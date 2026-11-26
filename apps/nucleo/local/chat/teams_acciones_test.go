// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//nolint:tagliatelle // el servidor de mentira habla con las claves camelCase de Microsoft Graph.
package chat

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	cchat "git.sytue.eu/sytue/one/apps/nucleo/cuentas/chat"
	"git.sytue.eu/sytue/one/apps/nucleo/local/red"
)

const carlaTeams = "55555555-5555-5555-5555-555555555555"

// graphFalso es la parte de Graph de los directos nuevos, los ficheros y el sondeo de
// chats nuevos.
type graphFalso struct {
	t   *testing.T
	srv *httptest.Server

	mu         sync.Mutex
	peticiones []string
	creados    []map[string]any
	subidos    []string // nombre|tipo|contenido
	invitados  []string
	enviados   []map[string]any
	chatNuevo  bool // el sondeo trae un chat que no estaba
}

func nuevoGraphFalso(t *testing.T) *graphFalso {
	f := &graphFalso{t: t}
	f.srv = httptest.NewServer(http.HandlerFunc(f.servir))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *graphFalso) teams(ficheros bool) *Teams {
	g := &cchat.Graph{Base: f.srv.URL + "/v1.0", Token: cchat.TokenFijo("t1"), HTTP: f.srv.Client()}
	tm := NuevaTeams(g, "Teams · ana@empresa.eu", false, Avisos{})
	tm.Ficheros = ficheros
	return tm
}

func (f *graphFalso) servir(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.peticiones = append(f.peticiones, r.Method+" "+r.URL.Path)
	if r.URL.Path == "/descarga/acta" {
		_, _ = io.WriteString(w, "contenido del acta")
		return
	}
	if r.Header.Get("Authorization") != "Bearer t1" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	ruta := strings.TrimPrefix(r.URL.Path, "/v1.0")
	chat1 := "/chats/19:chat1@thread.v2"
	ref := map[string]any{"id": "0f9c1a2b-3c4d-4e5f-8a9b-0c1d2e3f4a5b", "contentType": "reference", "contentUrl": "https://empresa-my.sharepoint.com/personal/bea/Documents/acta.pdf", "name": "acta.pdf"}
	img := mensajeGraph("m3", beaTeams, "Bea", `<img src="`+f.srv.URL+`/v1.0`+chat1+`/messages/m3/hostedContents/aWQ9eF8x/$value">`, "2026-11-20T10:03:00Z")
	conRef := mensajeGraph("m2", beaTeams, "Bea", "el acta de ayer", "2026-11-20T10:02:00Z")
	conRef["attachments"] = []any{ref}
	switch {
	case ruta == "/me":
		escribir(w, map[string]any{"id": yoTeams, "displayName": "Ana", "mail": "ana@empresa.eu"})
	case ruta == "/me/chats" && r.URL.Query().Get("$orderby") != "":
		v := []any{map[string]any{"id": "19:chat1@thread.v2", "lastMessagePreview": img}}
		if f.chatNuevo {
			v = append(v, map[string]any{"id": "19:nuevo@thread.v2", "lastMessagePreview": mensajeGraph("n1", carlaTeams, "Carla", "¿hablamos?", "2026-11-20T11:00:00Z")})
		}
		escribir(w, map[string]any{"value": v})
	case ruta == "/me/chats":
		v := []any{map[string]any{
			"id": "19:chat1@thread.v2", "chatType": "oneOnOne", "tenantId": inquilin,
			"members": []any{
				map[string]any{"userId": yoTeams, "displayName": "Ana"},
				map[string]any{"userId": beaTeams, "displayName": "Bea", "email": "bea@empresa.eu"},
			}, "lastMessagePreview": img,
		}}
		if f.chatNuevo {
			v = append(v, map[string]any{
				"id": "19:nuevo@thread.v2", "chatType": "oneOnOne",
				"members": []any{map[string]any{"userId": yoTeams, "displayName": "Ana"}, map[string]any{"userId": carlaTeams, "displayName": "Carla"}},
			})
		}
		escribir(w, map[string]any{"value": v})
	case ruta == "/chats" && r.Method == http.MethodPost:
		var c map[string]any
		_ = json.NewDecoder(r.Body).Decode(&c)
		f.creados = append(f.creados, c)
		escribir(w, map[string]any{"id": "19:carla@thread.v2", "chatType": "oneOnOne"})
	case ruta == "/chats/19:carla@thread.v2" && r.URL.Query().Get("$expand") == "members":
		escribir(w, map[string]any{
			"id": "19:carla@thread.v2", "chatType": "oneOnOne", "createdDateTime": "2026-11-20T10:10:00Z",
			"members": []any{map[string]any{"userId": yoTeams, "displayName": "Ana"}, map[string]any{"userId": carlaTeams, "displayName": "Carla", "email": "carla@empresa.eu"}},
		})
	case ruta == chat1+"/messages" && r.Method == http.MethodGet && r.URL.Query().Get("$filter") != "":
		escribir(w, map[string]any{"value": []any{}}) // el sondeo del abierto: nada nuevo
	case ruta == chat1+"/messages" && r.Method == http.MethodGet:
		escribir(w, map[string]any{"value": []any{img, conRef, mensajeGraph("m1", beaTeams, "Bea", "¿Comemos mañana?", "2026-11-20T10:01:00Z")}})
	case strings.HasPrefix(ruta, "/me/drive/root:/Microsoft Teams Chat Files/") && r.Method == http.MethodPut:
		b, _ := io.ReadAll(r.Body)
		nombre := strings.TrimSuffix(strings.TrimPrefix(ruta, "/me/drive/root:/Microsoft Teams Chat Files/"), ":/content")
		if r.URL.Query().Get("@microsoft.graph.conflictBehavior") != "rename" {
			f.t.Errorf("la subida pisaría otro: %s", r.URL)
		}
		f.subidos = append(f.subidos, nombre+"|"+r.Header.Get("Content-Type")+"|"+string(b))
		escribir(w, map[string]any{
			"id": "ITEM1", "name": nombre, "size": len(b), "eTag": `"{8A1B2C3D-4E5F-4A6B-9C8D-7E6F5A4B3C2D},1"`,
			"webUrl": "https://empresa-my.sharepoint.com/personal/ana/Documents/Microsoft%20Teams%20Chat%20Files/" + nombre,
		})
	case ruta == "/me/drive/items/ITEM1/invite":
		var c struct {
			Recipients []map[string]string `json:"recipients"`
			Roles      []string            `json:"roles"`
			Send       bool                `json:"sendInvitation"`
		}
		_ = json.NewDecoder(r.Body).Decode(&c)
		for _, x := range c.Recipients {
			f.invitados = append(f.invitados, x["objectId"]+":"+strings.Join(c.Roles, ","))
		}
		if c.Send {
			f.t.Error("compartir manda correo")
		}
		escribir(w, map[string]any{"value": []any{}})
	case ruta == chat1+"/messages" && r.Method == http.MethodPost:
		var c map[string]any
		_ = json.NewDecoder(r.Body).Decode(&c)
		f.enviados = append(f.enviados, c)
		escribir(w, map[string]any{
			"id": "m9", "createdDateTime": "2026-11-20T10:09:00Z", "messageType": "message",
			"from": map[string]any{"user": map[string]string{"id": yoTeams, "displayName": "Ana"}}, "body": c["body"], "attachments": c["attachments"],
		})
	case strings.HasPrefix(ruta, "/shares/u!") && strings.HasSuffix(ruta, "/driveItem/content"):
		if cchat.IDCompartido("https://empresa-my.sharepoint.com/personal/bea/Documents/acta.pdf") != strings.TrimSuffix(strings.TrimPrefix(ruta, "/shares/"), "/driveItem/content") {
			f.t.Errorf("enlace compartido: %s", ruta)
		}
		http.Redirect(w, r, "/descarga/acta", http.StatusFound)
	case ruta == chat1+"/messages/m3/hostedContents/aWQ9eF8x/$value":
		w.Header().Set("Content-Type", "image/png")
		_, _ = io.WriteString(w, "png")
	default:
		f.t.Errorf("petición inesperada: %s %s", r.Method, r.URL)
		w.WriteHeader(http.StatusNotFound)
	}
}

// Un directo nuevo: buscar por correo a quien no está en ningún chat, abrir el 1:1
// (Graph lo crea con los dos dentro) y, la segunda vez, el que ya hay sin crear otro.
func TestTeamsAbreUnDirectoNuevo(t *testing.T) {
	f := nuevoGraphFalso(t)
	tm := f.teams(false)
	ctx := t.Context()
	gente, err := tm.Gente(ctx)
	if err != nil || len(gente) != 1 || gente[0].Subject != beaTeams || gente[0].Name != "Bea" {
		t.Fatal(gente, err)
	}
	if p, err := tm.Resolver(ctx, "bea@empresa.eu"); err != nil || p.UserID != beaTeams {
		t.Fatal("el correo de alguien de un chat es él:", p, err)
	}
	p, err := tm.Resolver(ctx, "Carla@Empresa.eu")
	if err != nil || p.UserID != "carla@empresa.eu" {
		t.Fatal(p, err)
	}
	ch, err := tm.AbrirDirecto(ctx, []string{p.UserID})
	if err != nil || ch.ID != "chat/19:carla@thread.v2" || ch.Kind != "direct" || ch.Name != "Carla" {
		t.Fatal(ch, err)
	}
	f.mu.Lock()
	if len(f.creados) != 1 || f.creados[0]["chatType"] != "oneOnOne" {
		t.Fatalf("chat creado: %v", f.creados)
	}
	ms, _ := json.Marshal(f.creados[0]["members"])
	if !strings.Contains(string(ms), "/users('"+yoTeams+"')") || !strings.Contains(string(ms), "/users('carla@empresa.eu')") || !strings.Contains(string(ms), "aadUserConversationMember") {
		t.Fatalf("miembros: %s", ms)
	}
	f.mu.Unlock()
	// Con Bea ya hay un 1:1: se abre ese, sin pedir otro.
	if ch, err := tm.AbrirDirecto(ctx, []string{beaTeams}); err != nil || ch.ID != "chat/19:chat1@thread.v2" {
		t.Fatal(ch, err)
	}
	if ch, err := tm.AbrirDirecto(ctx, []string{carlaTeams}); err != nil || ch.ID != "chat/19:carla@thread.v2" {
		t.Fatal("el nuevo, ya en la lista:", ch, err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.creados) != 1 {
		t.Fatalf("se crearon %d chats", len(f.creados))
	}
	if _, err := tm.G.CrearChat(ctx, yoTeams, []string{"x') or ('1"}); !errors.Is(err, cchat.ErrEntrada) {
		t.Fatal("una persona que se sale de users('…'):", err)
	}
}

// Los adjuntos: subir al OneDrive de quien manda, compartir con los del chat sin mandar
// correo y enlazar en el mensaje; los que llegan, el de OneDrive por `shares` (que
// redirige a su servidor de descarga) y la imagen pegada del propio mensaje. Sin el
// permiso de los ficheros, solo la imagen.
func TestTeamsAdjuntosSeMandanYSeBajan(t *testing.T) {
	f := nuevoGraphFalso(t)
	tm := f.teams(true)
	ctx := t.Context()
	if _, _, err := tm.Canales(ctx); err != nil {
		t.Fatal(err)
	}
	fichero := Fichero{Nombre: "plan: 2027?.pdf", Tipo: "application/pdf", Tamano: 4, Abrir: func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader("%PDF")), nil }}
	m, err := tm.EnviarFicheros(ctx, "chat/19:chat1@thread.v2", "", "el plan <b>nuevo</b>", []Fichero{fichero})
	if err != nil || len(m.Attachments) != 1 || m.Attachments[0].Name != "plan_ 2027_.pdf" || m.AuthorSub != Yo || m.Body != "el plan <b>nuevo</b>" {
		t.Fatalf("%+v %v", m, err)
	}
	f.mu.Lock()
	if strings.Join(f.subidos, ";") != "plan_ 2027_.pdf|application/pdf|%PDF" || strings.Join(f.invitados, ";") != beaTeams+":read" || len(f.enviados) != 1 {
		t.Fatal(f.subidos, f.invitados, f.enviados)
	}
	cuerpo, _ := json.Marshal(f.enviados[0])
	var enviado struct {
		Body struct {
			Tipo    string `json:"contentType"`
			Content string `json:"content"`
		} `json:"body"`
		Adjuntos []map[string]string `json:"attachments"`
	}
	_ = json.Unmarshal(cuerpo, &enviado)
	if enviado.Body.Tipo != "html" || enviado.Body.Content != `el plan &lt;b&gt;nuevo&lt;/b&gt;<attachment id="8a1b2c3d-4e5f-4a6b-9c8d-7e6f5a4b3c2d"></attachment>` ||
		len(enviado.Adjuntos) != 1 || enviado.Adjuntos[0]["contentType"] != "reference" || enviado.Adjuntos[0]["id"] != "8a1b2c3d-4e5f-4a6b-9c8d-7e6f5a4b3c2d" ||
		!strings.HasPrefix(enviado.Adjuntos[0]["contentUrl"], "https://empresa-my.sharepoint.com/") {
		t.Fatalf("mensaje: %s", cuerpo)
	}
	f.mu.Unlock()

	ms, err := tm.Mensajes(ctx, "chat/19:chat1@thread.v2", time.Time{}, 50)
	if err != nil || len(ms) != 3 {
		t.Fatal(ms, err)
	}
	ref, img := ms[1].Attachments, ms[2].Attachments
	if len(ref) != 1 || ref[0].Name != "acta.pdf" || ref[0].ContentType != "application/pdf" || len(img) != 1 || !strings.HasPrefix(img[0].ContentType, "image/") {
		t.Fatalf("adjuntos: %+v %+v", ref, img)
	}
	leer := func(id string) (string, error) {
		rc, err := tm.Bajar(ctx, "chat/19:chat1@thread.v2", id, false)
		if err != nil {
			return "", err
		}
		defer rc.Close()
		b, err := io.ReadAll(rc)
		return string(b), err
	}
	if s, err := leer(ref[0].FileID); err != nil || s != "contenido del acta" {
		t.Fatal(s, err)
	}
	if s, err := leer(img[0].FileID); err != nil || s != "png" {
		t.Fatal(s, err)
	}
	if _, err := leer("t-que-no-existe"); !errors.Is(err, ErrNoDisponible) {
		t.Fatal(err)
	}
	// Sin el permiso de los ficheros.
	sin := f.teams(false)
	if _, err := sin.EnviarFicheros(ctx, "chat/19:chat1@thread.v2", "", "x", []Fichero{fichero}); !errors.Is(err, ErrSinFicherosTeams) {
		t.Fatal(err)
	}
	ms, _ = sin.Mensajes(ctx, "chat/19:chat1@thread.v2", time.Time{}, 50)
	if _, err := sin.Bajar(ctx, "", ms[1].Attachments[0].FileID, false); !errors.Is(err, ErrSinFicherosTeams) {
		t.Fatal(err)
	}
	if rc, err := sin.Bajar(ctx, "", ms[2].Attachments[0].FileID, false); err != nil {
		t.Fatal("la imagen pegada va con el chat:", err)
	} else {
		rc.Close()
	}
}

// Un chat que abre otra persona aparece sin recargar: el sondeo lo ve en los recientes,
// avisa UNA vez con EventoCanales (además del aviso de su mensaje) y la lista siguiente
// lo trae. La búsqueda encuentra en lo cargado, sin acentos.
func TestTeamsChatNuevoApareceSoloYBusqueda(t *testing.T) {
	f := nuevoGraphFalso(t)
	tm := f.teams(false)
	ctx := t.Context()
	if cs, _, err := tm.Canales(ctx); err != nil || len(cs) != 1 {
		t.Fatal(cs, err)
	}
	if _, err := tm.Mensajes(ctx, "chat/19:chat1@thread.v2", time.Time{}, 50); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.chatNuevo = true
	f.mu.Unlock()
	var tipos []string
	sondeo := func() {
		tipos = nil
		if _, err := tm.sondear(ctx, func(ev api.RoomsEvent) { tipos = append(tipos, ev.Type+"@"+ev.ChannelID) }); err != nil {
			t.Fatal(err)
		}
	}
	sondeo()
	if strings.Join(tipos, ",") != "message@chat/19:nuevo@thread.v2,"+EventoCanales+"@" {
		t.Fatal(tipos)
	}
	cs, us, err := tm.Canales(ctx)
	if err != nil || len(cs) != 2 || cs[1].ID != "chat/19:nuevo@thread.v2" || cs[1].Name != "Carla" {
		t.Fatal(cs, err)
	}
	if !slices.ContainsFunc(us, func(u api.RoomsUnread) bool { return u.ChannelID == "chat/19:nuevo@thread.v2" && u.Unread == 1 }) {
		t.Fatal(us)
	}
	sondeo()
	if len(tipos) != 0 {
		t.Fatal("se avisó otra vez:", tipos)
	}
	if r, _ := tm.Buscar(ctx, "comemos MANANA", 10); len(r) != 1 || r[0].ID != "m1" {
		t.Fatal(r)
	}
	if r, _ := tm.Buscar(ctx, "hablamos", 10); len(r) != 1 || r[0].ChannelID != "chat/19:nuevo@thread.v2" {
		t.Fatal("lo que llegó por el sondeo también se busca:", r)
	}
}

// proveedorQueAvisa es una cuenta que, al escuchar, dice que tiene canales nuevos y
// entrega un mensaje.
type proveedorQueAvisa struct{ Notas }

func (p *proveedorQueAvisa) Escuchar(ctx context.Context, alLlegar func(api.RoomsEvent)) error {
	alLlegar(api.RoomsEvent{Type: EventoCanales})
	raw, _ := json.Marshal(api.RoomsMessage{ID: "m1", ChannelID: "c1"})
	alLlegar(api.RoomsEvent{Type: "message", ChannelID: "c1", Payload: raw})
	<-ctx.Done()
	return ctx.Err()
}

// El Agregador no pasa EventoCanales a la pantalla: llama a AlCambiarCanales con la
// cuenta.
func TestElAgregadorPasaLosCanalesNuevosAAlCambiarCanales(t *testing.T) {
	a := NuevoAgregador("")
	a.Poner("c1", &proveedorQueAvisa{})
	cambios := make(chan string, 2)
	a.AlCambiarCanales = func(cuenta string) { cambios <- cuenta }
	ctx, parar := context.WithCancel(t.Context())
	llegan := make(chan string, 4)
	go func() {
		_ = a.Listen(ctx, nil, func(ev api.RoomsEvent) { llegan <- ev.Type })
	}()
	if c := <-cambios; c != "c1" {
		t.Fatal(c)
	}
	if ev := <-llegan; ev != api.RoomsReady {
		t.Fatal(ev)
	}
	if ev := <-llegan; ev != "message" {
		t.Fatal("llegó a la pantalla:", ev)
	}
	parar()
}

// Los permisos nuevos solo se piden si se marcan; el portero deja ir a SharePoint solo
// con una cuenta de Teams con adjuntos, y una cuenta de Google guardada antes de la
// People API la gana al abrirse (el portero solo deja ir a lo que dice la cuenta).
func TestLosPermisosNuevosSonOpcionalesYElPorteroLosSabe(t *testing.T) {
	if a := strings.Join(OAuthTeams("c", false).Alcances, " "); strings.Contains(a, "Files.") {
		t.Fatal(a)
	}
	if a := strings.Join(OAuthTeamsCon("c", false, true).Alcances, " "); !strings.Contains(a, "Files.ReadWrite.All") {
		t.Fatal(a)
	}
	if a := strings.Join(OAuthGoogleChat("c", "s", true).Alcances, " "); strings.Contains(a, "chat.spaces.create") {
		t.Fatal(a)
	}
	if a := strings.Join(OAuthGoogleChatCon("c", "s", false, true).Alcances, " "); !strings.Contains(a, "chat.spaces.create") {
		t.Fatal(a)
	}
	ruta := filepath.Join(t.TempDir(), "chat-cuentas.json")
	viejas := []Cuenta{
		{ID: "t1", Proveedor: ProveedorTeams, Rotulo: "Microsoft Teams", Usuario: "ana@empresa.eu", Ficheros: true, ClienteID: "c", Servidores: []string{APIGraph}},
		{ID: "t2", Proveedor: ProveedorTeams, Rotulo: "Microsoft Teams", Usuario: "otra@empresa.eu", ClienteID: "c", Servidores: []string{APIGraph}},
		{ID: "g1", Proveedor: ProveedorGoogle, Rotulo: "Google Chat", Usuario: "ana@empresa.eu", ClienteID: "c", Servidores: []string{APIChatGoogle, APIMeet, APIUsuarioGoogle}},
	}
	raw, _ := json.Marshal(viejas)
	if err := os.WriteFile(ruta, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	sec := &secretosMemoria{v: map[string]string{secretoRefresco("t1"): "r", secretoRefresco("t2"): "r", secretoRefresco("g1"): "r"}}
	reg, err := AbrirRegistro(ruta, sec)
	if err != nil {
		t.Fatal(err)
	}
	// Sus servidores escuchan en este equipo: sin cliente (el del portero, en la
	// edición) no se sale a la red (R-RED).
	reg.Cliente = &http.Client{}
	ds := reg.DestinosDescarga()
	if len(ds) != 1 || ds[0].Host != ".sharepoint.com" || ds[0].Proposito != red.Chat || ds[0].Motivo != "Microsoft Teams — ana@empresa.eu" {
		t.Fatal(ds)
	}
	p, err := reg.Abrir(viejas[2])
	if err != nil || p.(*GoogleChat).P == nil {
		t.Fatal(p, err)
	}
	if p, _ := reg.Abrir(viejas[0]); !p.(*Teams).Ficheros {
		t.Fatal("la cuenta de Teams abre sin sus adjuntos")
	}
	escrito, _ := os.ReadFile(ruta)
	if !strings.Contains(string(escrito), APIPersonasGoogle) || slices.Contains(reg.Cuentas()[0].Servidores, APIPersonasGoogle) {
		t.Fatalf("chat-cuentas.json: %s", escrito)
	}
}
