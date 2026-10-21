// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

func TestRooms(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/teams":
			_, _ = io.WriteString(w, `{"teams":[{"id":"t1","slug":"acme","name":"ACME","role":"admin"}]}`)
		case "GET /api/v1/teams/t1/channels":
			_, _ = io.WriteString(w, `{"channels":[{"id":"c1","name":"general","kind":"public"},{"id":"d1","name":"dm-x","kind":"direct","peers":["Grace Hopper","Linus"]}]}`)
		case "GET /api/v1/teams/t1/unread":
			_, _ = io.WriteString(w, `{"channels":[{"channel_id":"c1","unread":3,"mentions":1}]}`)
		case "GET /api/v1/channels/c1/messages":
			if r.URL.Query().Get("limit") != "50" || r.URL.Query().Get("before") != "2026-09-20T10:00:00Z" {
				t.Errorf("paginación: %s", r.URL.RawQuery)
			}
			// El servicio los da del más nuevo al más viejo.
			_, _ = io.WriteString(w, `{"messages":[{"id":"m2","body":"segundo","created_at":"2026-09-20T09:01:00Z"},{"id":"m1","body":"primero","created_at":"2026-09-20T09:00:00Z"}]}`)
		case "POST /api/v1/channels/c1/messages":
			cuerpo, _ := io.ReadAll(r.Body)
			if string(cuerpo) != `{"body":"hola"}` {
				t.Errorf("cuerpo del mensaje: %s", cuerpo)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"id":"m3","body":"hola"}`)
		case "POST /api/v1/channels/c1/read":
			w.WriteHeader(http.StatusNoContent)
		case "GET /api/v1/me/external-channels":
			_, _ = io.WriteString(w, `{"channels":[{"id":"x1","name":"proveedor","kind":"public","external":true}],"unread":[{"channel_id":"x1","unread":2,"mentions":0}]}`)
		case "GET /api/v1/me/categories":
			_, _ = io.WriteString(w, `{"categories":[{"id":"k1","name":"Clientes","sort_order":1,"collapsed":true}]}`)
		case "POST /api/v1/channels/c1/prefs":
			if cuerpo, _ := io.ReadAll(r.Body); string(cuerpo) != `{"favorite":true}` && string(cuerpo) != `{"notify_level":"mentions"}` {
				t.Errorf("prefs: %s", cuerpo)
			}
			_, _ = io.WriteString(w, `{"id":"c1","favorite":true}`)
		case "POST /api/v1/channels/c1/category":
			if cuerpo, _ := io.ReadAll(r.Body); string(cuerpo) != `{"category_id":"k1"}` {
				t.Errorf("categoría: %s", cuerpo)
			}
			_, _ = io.WriteString(w, `{"id":"c1","category_id":"k1"}`)
		case "GET /api/v1/me/scheduled":
			_, _ = io.WriteString(w, `{"scheduled":[{"id":"p1","channel_id":"c1","author_sub":"u1","body":"luego","send_at":"2026-09-24T08:00:00Z"}]}`)
		case "DELETE /api/v1/scheduled/p1":
			w.WriteHeader(http.StatusNoContent)
		case "GET /api/v1/me/drafts":
			_, _ = io.WriteString(w, `{"drafts":[{"channel_id":"c1","body":"a medias"},{"channel_id":"c1","parent_id":"m1","body":"en el hilo"}]}`)
		case "PUT /api/v1/me/drafts":
			cuerpo, _ := io.ReadAll(r.Body)
			if string(cuerpo) != `{"channel_id":"c1","parent_id":"m1","body":"hola"}` && string(cuerpo) != `{"channel_id":"c1","body":""}` {
				t.Errorf("borrador: %s", cuerpo)
			}
			_, _ = io.WriteString(w, `{"channel_id":"c1","body":"hola"}`)
		case "POST /api/v1/me/categories":
			if cuerpo, _ := io.ReadAll(r.Body); string(cuerpo) != `{"name":"Clientes"}` {
				t.Errorf("crear categoría: %s", cuerpo)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"id":"k9","name":"Clientes","sort_order":3}`)
		case "PATCH /api/v1/me/categories/k9":
			if cuerpo, _ := io.ReadAll(r.Body); string(cuerpo) != `{"collapsed":false}` {
				t.Errorf("cambiar categoría: %s", cuerpo)
			}
			_, _ = io.WriteString(w, `{"id":"k9","name":"Clientes"}`)
		case "DELETE /api/v1/me/categories/k9":
			w.WriteHeader(http.StatusNoContent)
		case "PATCH /api/v1/channels/c1":
			if cuerpo, _ := io.ReadAll(r.Body); string(cuerpo) != `{"archived":true}` {
				t.Errorf("archivar: %s", cuerpo)
			}
			_, _ = io.WriteString(w, `{"id":"c1","archived":true}`)
		case "PUT /api/v1/messages/m1/reactions/👍", "DELETE /api/v1/messages/m1/reactions/👍":
			w.WriteHeader(http.StatusNoContent)
		case "GET /api/v1/messages/m1/thread":
			_, _ = io.WriteString(w, `{"root":{"id":"m1","body":"raíz","reply_count":1},"replies":[{"id":"r1","parent_id":"m1","body":"respuesta"}],"following":true}`)
		case "POST /api/v1/messages/m1/follow", "DELETE /api/v1/messages/m1/follow",
			"DELETE /api/v1/messages/m1", "POST /api/v1/messages/m1/pin", "DELETE /api/v1/messages/m1/pin",
			"POST /api/v1/messages/m1/save", "DELETE /api/v1/messages/m1/save", "POST /api/v1/messages/m1/unread":
			w.WriteHeader(http.StatusNoContent)
		case "PATCH /api/v1/messages/m1":
			if cuerpo, _ := io.ReadAll(r.Body); string(cuerpo) != `{"body":"otro"}` {
				t.Errorf("edición: %s", cuerpo)
			}
			_, _ = io.WriteString(w, `{"id":"m1","body":"otro","edited_at":"2026-09-22T10:00:00Z"}`)
		case "QUERY /api/v1/search":
			cuerpo, _ := io.ReadAll(r.Body)
			if string(cuerpo) != "limit=30&q=in%3A%23general+hola" {
				t.Errorf("búsqueda: %s", cuerpo)
			}
			_, _ = io.WriteString(w, `{"messages":[{"id":"m7","channel_id":"c1","body":"hola"}]}`)
		case "GET /api/v1/channels/c1/messages/around":
			if q := r.URL.Query(); q.Get("message") != "r1" || q.Get("limit") != "50" {
				t.Errorf("alrededor: %s", r.URL.RawQuery)
			}
			_, _ = io.WriteString(w, `{"messages":[{"id":"m1","body":"raíz"}],"thread_root_id":"m1"}`)
		case "POST /api/v1/teams/t1/channels":
			if cuerpo, _ := io.ReadAll(r.Body); string(cuerpo) != `{"name":"Diseño","kind":"private","topic":"tema"}` {
				t.Errorf("crear canal: %s", cuerpo)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"id":"n1","name":"diseno","kind":"private"}`)
		case "GET /api/v1/teams/t1/channels/browse":
			if r.URL.Query().Get("q") != "ven" {
				t.Errorf("explorar: %s", r.URL.RawQuery)
			}
			_, _ = io.WriteString(w, `{"channels":[{"id":"v1","name":"ventas","kind":"public"}]}`)
		case "POST /api/v1/channels/v1/join":
			_, _ = io.WriteString(w, `{"id":"v1","name":"ventas","kind":"public"}`)
		case "GET /api/v1/teams/t1/members":
			_, _ = io.WriteString(w, `{"members":[{"subject":"u2","name":"Ana","role":"member"}]}`)
		case "POST /api/v1/people:resolve":
			if cuerpo, _ := io.ReadAll(r.Body); string(cuerpo) != `{"query":"@grace"}` {
				t.Errorf("resolver: %s", cuerpo)
			}
			_, _ = io.WriteString(w, `{"user_id":"g1","name":"Grace","username":"grace"}`)
		case "GET /api/v1/channels/c1/members":
			_, _ = io.WriteString(w, `{"members":[{"subject":"u2","name":"Ana","role":"admin","external":true}]}`)
		case "POST /api/v1/channels/c1/members":
			if cuerpo, _ := io.ReadAll(r.Body); string(cuerpo) != `{"subject":"u3","name":"Beto"}` {
				t.Errorf("añadir: %s", cuerpo)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"subject":"u3","name":"Beto","role":"member"}`)
		case "DELETE /api/v1/channels/c1/members/u3":
			w.WriteHeader(http.StatusNoContent)
		case "PATCH /api/v1/channels/c1/members/u3":
			if cuerpo, _ := io.ReadAll(r.Body); string(cuerpo) != `{"role":"admin"}` {
				t.Errorf("rol: %s", cuerpo)
			}
			_, _ = io.WriteString(w, `{"subject":"u3","name":"Beto","role":"admin"}`)
		case "POST /api/v1/teams/t1/dms":
			if cuerpo, _ := io.ReadAll(r.Body); string(cuerpo) != `{"subjects":["u2","u3"]}` {
				t.Errorf("directo: %s", cuerpo)
			}
			_, _ = io.WriteString(w, `{"id":"d9","kind":"direct","peers":["Ana","Beto"]}`)
		case "GET /api/v1/me/threads":
			if b := r.URL.Query().Get("before"); b != "" && b != "2026-09-21T08:00:00Z" {
				t.Errorf("hilos: %s", r.URL.RawQuery)
			}
			_, _ = io.WriteString(w, `{"threads":[{"root":{"id":"m1","channel_id":"c1","body":"raíz","reply_count":4},"last_activity":"2026-09-22T09:00:00Z","unread_replies":2}],"next_before":"2026-09-21T08:00:00Z"}`)
		case "GET /api/v1/channels/c1/pins":
			_, _ = io.WriteString(w, `{"pins":[{"id":"m1","body":"fijado"}]}`)
		case "GET /api/v1/me/saved":
			_, _ = io.WriteString(w, `{"messages":[{"id":"m2","body":"guardado"}],"next_before":"2026-09-01T00:00:00Z"}`)
		case "POST /api/v1/messages/m1/replies":
			if cuerpo, _ := io.ReadAll(r.Body); string(cuerpo) != `{"body":"vale"}` {
				t.Errorf("respuesta: %s", cuerpo)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"id":"r2","parent_id":"m1","body":"vale"}`)
		default:
			t.Errorf("petición inesperada: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	c, ctx := NewRooms(srv.URL, nil, nil), context.Background()

	equipos, err := c.Teams(ctx)
	if err != nil || len(equipos) != 1 || equipos[0].Role != "admin" {
		t.Fatalf("equipos: %+v, %v", equipos, err)
	}
	canales, err := c.Channels(ctx, "t1")
	if err != nil || len(canales) != 2 || canales[0].Direct() || !canales[1].Direct() {
		t.Fatalf("canales: %+v, %v", canales, err)
	}
	// Un directo se llama como la gente que está en él, no con su nombre interno.
	if canales[1].Title() != "Grace Hopper, Linus" || canales[0].Title() != "general" {
		t.Errorf("títulos: %q, %q", canales[0].Title(), canales[1].Title())
	}
	pendientes, err := c.Unread(ctx, "t1")
	if err != nil || len(pendientes) != 1 || pendientes[0].Unread != 3 {
		t.Fatalf("pendientes: %+v, %v", pendientes, err)
	}
	mensajes, err := c.Messages(ctx, "c1", time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC), 50)
	if err != nil || len(mensajes) != 2 || mensajes[0].ID != "m1" {
		t.Fatalf("los mensajes no salen en orden de lectura: %+v, %v", mensajes, err)
	}
	if m, err := c.Post(ctx, "c1", "hola"); err != nil || m.ID != "m3" {
		t.Fatalf("Post: %+v, %v", m, err)
	}
	if err := c.MarkRead(ctx, "c1"); err != nil {
		t.Fatal(err)
	}
	fuera, pend, err := c.ExternalChannels(ctx)
	if err != nil || len(fuera) != 1 || !fuera[0].External || len(pend) != 1 || pend[0].Unread != 2 {
		t.Fatalf("de fuera: %+v %+v, %v", fuera, pend, err)
	}
	cats, err := c.Categories(ctx)
	if err != nil || len(cats) != 1 || !cats[0].Collapsed || cats[0].SortOrder != 1 {
		t.Fatalf("categorías: %+v, %v", cats, err)
	}
	if err := c.SetFavorite(ctx, "c1", true); err != nil {
		t.Fatal(err)
	}
	if err := c.React(ctx, "m1", "👍", true); err != nil {
		t.Fatal(err)
	}
	if err := c.React(ctx, "m1", "👍", false); err != nil {
		t.Fatal(err)
	}
	hilo, err := c.Thread(ctx, "m1")
	if err != nil || hilo.Root.ReplyCount != 1 || len(hilo.Replies) != 1 || hilo.Replies[0].ID != "r1" || !hilo.Following {
		t.Fatalf("hilo: %+v, %v", hilo, err)
	}
	if err := c.SetNotifyLevel(ctx, "c1", "mentions"); err != nil {
		t.Fatal(err)
	}
	if err := c.SetChannelCategory(ctx, "c1", "k1"); err != nil {
		t.Fatal(err)
	}
	if err := c.ArchiveChannel(ctx, "c1"); err != nil {
		t.Fatal(err)
	}
	if ps, err := c.Scheduled(ctx); err != nil || len(ps) != 1 || ps[0].SendAt.Hour() != 8 || ps[0].Body != "luego" {
		t.Fatalf("Scheduled: %+v, %v", ps, err)
	}
	if err := c.CancelScheduled(ctx, "p1"); err != nil {
		t.Fatal(err)
	}
	if ds, err := c.Drafts(ctx); err != nil || len(ds) != 2 || ds[1].ParentID == nil || *ds[1].ParentID != "m1" {
		t.Fatalf("Drafts: %+v, %v", ds, err)
	}
	if err := c.PutDraft(ctx, "c1", "m1", "hola"); err != nil {
		t.Fatal(err)
	}
	if err := c.PutDraft(ctx, "c1", "", ""); err != nil {
		t.Fatal(err)
	}
	if k, err := c.CreateCategory(ctx, "Clientes"); err != nil || k.ID != "k9" || k.SortOrder != 3 {
		t.Fatalf("CreateCategory: %+v, %v", k, err)
	}
	abierta := false
	if err := c.UpdateCategory(ctx, "k9", RoomsCategoryPatch{Collapsed: &abierta}); err != nil {
		t.Fatal(err) // un `false` explícito viaja: es desplegar, no «no tocar»
	}
	if err := c.DeleteCategory(ctx, "k9"); err != nil {
		t.Fatal(err)
	}
	if hs, sig, err := c.MyThreads(ctx, ""); err != nil || len(hs) != 1 || hs[0].UnreadReplies != 2 ||
		hs[0].Root.ReplyCount != 4 || hs[0].LastActivity.Hour() != 9 || sig != "2026-09-21T08:00:00Z" {
		t.Fatalf("MyThreads: %+v, %q, %v", hs, sig, err)
	}
	if _, _, err := c.MyThreads(ctx, "2026-09-21T08:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if err := c.Follow(ctx, "m1", true); err != nil {
		t.Fatal(err)
	}
	if err := c.Follow(ctx, "m1", false); err != nil {
		t.Fatal(err)
	}
	if m, err := c.Reply(ctx, "m1", "vale"); err != nil || m.ID != "r2" {
		t.Fatalf("Reply: %+v, %v", m, err)
	}
	if m, err := c.Edit(ctx, "m1", "otro"); err != nil || m.Body != "otro" || m.EditedAt == nil {
		t.Fatalf("Edit: %+v, %v", m, err)
	}
	for _, f := range []func() error{
		func() error { return c.Delete(ctx, "m1") },
		func() error { return c.Pin(ctx, "m1", true) }, func() error { return c.Pin(ctx, "m1", false) },
		func() error { return c.Save(ctx, "m1", true) }, func() error { return c.Save(ctx, "m1", false) },
		func() error { return c.MarkUnread(ctx, "m1") },
	} {
		if err := f(); err != nil {
			t.Fatal(err)
		}
	}
	if fijados, err := c.Pins(ctx, "c1"); err != nil || len(fijados) != 1 || fijados[0].ID != "m1" {
		t.Fatalf("Pins: %+v, %v", fijados, err)
	}
	if guardados, err := c.Saved(ctx); err != nil || len(guardados) != 1 || guardados[0].ID != "m2" {
		t.Fatalf("Saved: %+v, %v", guardados, err)
	}
	if hallados, err := c.Search(ctx, "in:#general hola", 30); err != nil || len(hallados) != 1 || hallados[0].ChannelID != "c1" {
		t.Fatalf("Search: %+v, %v", hallados, err)
	}
	if ventana, raiz, err := c.Around(ctx, "c1", "r1", 50); err != nil || len(ventana) != 1 || raiz != "m1" {
		t.Fatalf("Around: %+v %q, %v", ventana, raiz, err)
	}
	if n, err := c.CreateChannel(ctx, "t1", "Diseño", "tema", true); err != nil || n.ID != "n1" {
		t.Fatalf("CreateChannel: %+v, %v", n, err)
	}
	if hay, err := c.Browse(ctx, "t1", "ven"); err != nil || len(hay) != 1 {
		t.Fatalf("Browse: %+v, %v", hay, err)
	}
	if v, err := c.Join(ctx, "v1"); err != nil || v.ID != "v1" {
		t.Fatalf("Join: %+v, %v", v, err)
	}
	if d, err := c.OpenDirect(ctx, "t1", []string{"u2", "u3"}); err != nil || d.Title() != "Ana, Beto" {
		t.Fatalf("OpenDirect: %+v, %v", d, err)
	}
	if gente, err := c.TeamMembers(ctx, "t1"); err != nil || len(gente) != 1 || gente[0].Name != "Ana" {
		t.Fatalf("TeamMembers: %+v, %v", gente, err)
	}
	if p, err := c.ResolvePerson(ctx, "@grace"); err != nil || p.UserID != "g1" || p.Username != "grace" {
		t.Fatalf("ResolvePerson: %+v, %v", p, err)
	}
	if roster, err := c.ChannelMembers(ctx, "c1"); err != nil || len(roster) != 1 || !roster[0].External || roster[0].Role != "admin" {
		t.Fatalf("ChannelMembers: %+v, %v", roster, err)
	}
	if m, err := c.AddChannelMember(ctx, "c1", "u3", "Beto"); err != nil || m.Role != "member" {
		t.Fatalf("AddChannelMember: %+v, %v", m, err)
	}
	if m, err := c.SetChannelMemberRole(ctx, "c1", "u3", "admin"); err != nil || m.Role != "admin" {
		t.Fatalf("SetChannelMemberRole: %+v, %v", m, err)
	}
	if err := c.RemoveChannelMember(ctx, "c1", "u3"); err != nil {
		t.Fatal(err)
	}
}

func TestLosCamposDeRoomsSonLosDelServicio(t *testing.T) {
	dominio := webref.Read(t, "services/rooms/internal/domain/domain.go")
	for _, campo := range []string{
		`json:"team_id"`, `json:"kind"`, `json:"peers,omitempty"`, `json:"author_name,omitempty"`, `json:"author_sub"`, `json:"body"`,
		`json:"channel_id"`, `json:"unread"`, `json:"mentions"`, `ChannelDirect ChannelKind = "direct"`,
		`json:"peer_subs,omitempty"`, `json:"favorite,omitempty"`, `json:"notify_level,omitempty"`,
		`json:"category_id,omitempty"`, `json:"sort_order,omitempty"`, `json:"external,omitempty"`,
		`json:"priority,omitempty"`, `json:"is_bot,omitempty"`, `json:"reactions,omitempty"`,
		`json:"emoji"`, `json:"count"`, `json:"subs,omitempty"`, `json:"collapsed"`, `json:"purpose,omitempty"`,
		`json:"subject"`, `json:"role"`, `json:"role,omitempty"`, `json:"external,omitempty"`, `json:"mentions,omitempty"`,
		`json:"root"`, `json:"last_activity"`, `json:"unread_replies"`, `json:"parent_id,omitempty"`,
		`json:"send_at"`, `json:"file_ids,omitempty"`,
	} {
		if !strings.Contains(dominio, campo) {
			t.Errorf("services/rooms ya no declara %s: revisar internal/api/rooms.go", campo)
		}
	}
	manejadores := webref.Read(t, "services/rooms/internal/http/handlers.go")
	for _, literal := range []string{
		`"teams": teams`, `"channels": channels`, `"channels": states`, `"messages": msgs`, `Query().Get("before")`, "`json:\"body\"`",
		`"categories": cats`, `"root": rootMsg, "replies": replies, "following": following`, "`json:\"favorite,omitempty\"`",
		`"pins": pins`, `resp := map[string]any{"messages": msgs}`, `params, err := httpx.Consulta(w, r)`,
		`out["thread_root_id"] = root`, `Query().Get("message")`,
		"`json:\"topic,omitempty\"`", "`json:\"subjects,omitempty\"`", `httpx.JSON(w, r, http.StatusCreated, ch)`,
		`resp := map[string]any{"threads": threads}`, `resp["next_before"] = *next`,
		"`json:\"notify_level,omitempty\"`", "`json:\"category_id,omitempty\"`", "`json:\"archived,omitempty\"`",
		"`json:\"sort_order,omitempty\"`", "`json:\"collapsed,omitempty\"`", `httpx.JSON(w, r, http.StatusCreated, cat)`,
		`map[string]any{"drafts": list}`, `map[string]any{"scheduled": list}`, "`json:\"channel_id\"`", "`json:\"parent_id,omitempty\"`",
	} {
		if !strings.Contains(manejadores, literal) {
			t.Errorf("services/rooms/internal/http/handlers.go ya no dice %s", literal)
		}
	}
	// La Persona de people:resolve vive desde cast en el paquete compartido
	// (platform/pkg/personas) y Rooms la usa por alias: se sigue el alias hasta
	// donde está el contrato, en vez de exigir que siga copiado en Rooms.
	if !strings.Contains(webref.Read(t, "services/rooms/internal/service/externos.go"), "type Persona = personas.Persona") {
		t.Error("services/rooms ya no contesta people:resolve con la Persona de platform/pkg/personas")
	}
	if persona := webref.Read(t, "platform/pkg/personas/personas.go"); !strings.Contains(persona, "`json:\"user_id\"`") ||
		!strings.Contains(persona, "`json:\"username,omitempty\"`") {
		t.Error("platform/pkg/personas ya no da user_id y username a people:resolve")
	}
	if !strings.Contains(webref.Read(t, "services/rooms/internal/http/externos.go"), `"channels": channels, "unread": unread`) {
		t.Error("services/rooms ya no contesta las conversaciones de fuera con channels y unread")
	}
	rutas := webref.Read(t, "services/rooms/internal/http/server.go")
	for _, ruta := range []string{
		`r.Get("/ws", wsh.serve)`, `"/teams"`, `"/teams/{teamID}/channels"`, `"/teams/{teamID}/unread"`,
		`"/channels/{channelID}/messages"`, `"/channels/{channelID}/read"`, `api.Get("/me/external-channels"`,
		`api.Get("/me/categories"`, `porCanal.Post("/channels/{channelID}/prefs"`,
		`porMensaje.Put("/messages/{messageID}/reactions/{emoji}"`, `porMensaje.Delete("/messages/{messageID}/reactions/{emoji}"`,
		`porMensaje.Get("/messages/{messageID}/thread"`, `porMensaje.Post("/messages/{messageID}/replies"`,
		`porMensaje.Post("/messages/{messageID}/follow"`, `porMensaje.Delete("/messages/{messageID}/follow"`,
		`porMensaje.Patch("/messages/{messageID}"`, `porMensaje.Delete("/messages/{messageID}"`,
		`porMensaje.Post("/messages/{messageID}/pin"`, `porMensaje.Delete("/messages/{messageID}/pin"`,
		`porMensaje.Post("/messages/{messageID}/save"`, `porMensaje.Delete("/messages/{messageID}/save"`,
		`porMensaje.Post("/messages/{messageID}/unread"`, `porCanal.Get("/channels/{channelID}/pins"`, `api.Get("/me/saved"`,
		`httpx.GetYQuery(api, "/search", h.SearchAll)`, `porCanal.Get("/channels/{channelID}/messages/around"`,
		`api.Post("/teams/{teamID}/channels", h.CreateChannel)`, `api.Get("/teams/{teamID}/channels/browse"`,
		`porCanal.Post("/channels/{channelID}/join"`, `api.Post("/teams/{teamID}/dms", h.OpenDirect)`,
		`api.Get("/teams/{teamID}/members", h.ListTeamMembers)`, `api.Post("/people:resolve", h.ResolvePerson)`,
		`porCanal.Get("/channels/{channelID}/members", h.ListChannelMembers)`, `porCanal.Post("/channels/{channelID}/members", h.AddChannelMember)`,
		`porCanal.Delete("/channels/{channelID}/members/{sub}", h.RemoveChannelMember)`,
		`porCanal.Patch("/channels/{channelID}/members/{sub}", h.SetChannelMemberRole)`, `api.Get("/me/threads", h.ListMyThreads)`,
		`api.Post("/channels/{channelID}/category", h.SetChannelCategory)`, `porCanal.Patch("/channels/{channelID}", h.UpdateChannel)`,
		`api.Post("/me/categories", h.CreateCategory)`, `api.Patch("/me/categories/{categoryID}", h.UpdateCategory)`,
		`api.Delete("/me/categories/{categoryID}", h.DeleteCategory)`, `api.Get("/me/drafts", h.ListDrafts)`,
		`api.Put("/me/drafts", h.PutDraft)`, `api.Get("/me/scheduled", h.ListScheduled)`,
		`api.Delete("/scheduled/{scheduledID}", h.CancelScheduled)`,
	} {
		if !strings.Contains(rutas, ruta) {
			t.Errorf("services/rooms ya no monta %s", ruta)
		}
	}
}

// El tiempo real: el token va en la cabecera, se suscribe a cada canal y entrega las
// tramas tal cual llegan. Y habla el protocolo del servicio, no uno parecido.
func TestRoomsListen(t *testing.T) {
	suscritos := make(chan string, 2)
	escrito := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ws" || r.Header.Get("Authorization") != "Bearer tok-1" {
			t.Errorf("al tiempo real llega %s con %q", r.URL.Path, r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = c.CloseNow() }()
		_ = wsjson.Write(r.Context(), c, map[string]string{"type": "ready", "subject": "u1"})
		for range 2 {
			var orden struct {
				Type      string `json:"type"`
				ChannelID string `json:"channel_id"`
			}
			if err := wsjson.Read(r.Context(), c, &orden); err != nil || orden.Type != "subscribe" {
				t.Errorf("orden %+v, %v", orden, err)
				return
			}
			suscritos <- orden.ChannelID
		}
		var orden struct {
			Type      string `json:"type"`
			ChannelID string `json:"channel_id"`
		}
		// Con plazo: si Typing no escribe, el test cae en 5 s y no se cuelga.
		plazo, cancelar := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancelar()
		if err := wsjson.Read(plazo, c, &orden); err != nil || orden.Type != "typing" {
			t.Errorf("«escribiendo»: %+v, %v", orden, err)
			return
		}
		escrito <- orden.ChannelID
		_ = wsjson.Write(r.Context(), c, map[string]any{"type": "message", "channel_id": "c1", "subject": "u2", "name": "Ana", "payload": map[string]string{"id": "m9"}})
		_ = c.Close(websocket.StatusNormalClosure, "")
	}))
	defer srv.Close()

	var tramas []RoomsEvent
	rooms := NewRooms(srv.URL, &sesionFalsa{tokens: []string{"tok-1"}}, nil)
	err := rooms.Listen(context.Background(), []string{"c1", "d1"}, func(ev RoomsEvent) {
		tramas = append(tramas, ev)
		if ev.Type == RoomsReady {
			if err := rooms.Typing(context.Background(), "c1"); err != nil {
				t.Errorf("Typing: %v", err)
			}
		}
	})
	if err == nil {
		t.Fatal("Listen tiene que volver con el porqué cuando la conexión se cierra")
	}
	if a, b := <-suscritos, <-suscritos; a != "c1" || b != "d1" {
		t.Fatalf("suscrito a %q y %q", a, b)
	}
	if c := <-escrito; c != "c1" {
		t.Errorf("«escribiendo» sale en %q", c)
	}
	if err := rooms.Typing(context.Background(), "c1"); err != nil {
		t.Errorf("sin tiempo real, Typing no hace nada y no falla: %v", err)
	}
	if len(tramas) != 2 || tramas[0].Type != RoomsReady || tramas[1].Type != "message" || tramas[1].ChannelID != "c1" ||
		tramas[1].Subject != "u2" || tramas[1].Name != "Ana" || string(tramas[1].Payload) != `{"id":"m9"}` {
		t.Fatalf("tramas: %+v", tramas)
	}

	protocolo := webref.Read(t, "services/rooms/internal/ws/protocol.go")
	for _, literal := range []string{
		`ClientSubscribe = "subscribe"`, `ServerReady = "ready"`, `ServerMessage = "message"`, "`json:\"channel_id,omitempty\"`", "`json:\"type\"`",
		"`json:\"subject,omitempty\"`", "`json:\"name,omitempty\"`", `ServerTyping = "typing"`, `ServerMessageEdited = "message_edited"`,
		`ServerMessageDeleted = "message_deleted"`, `ServerReaction = "reaction"`, `ServerCallStarted = "call_started"`, `ServerCallEnded = "call_ended"`,
		"`json:\"payload,omitempty\"`", `ClientTyping = "typing"`,
	} {
		if !strings.Contains(protocolo, literal) {
			t.Errorf("services/rooms/internal/ws/protocol.go ya no dice %s", literal)
		}
	}
}
