// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// videoCast es un vídeo tal cual lo pinta `aDTO` de services/cast: con los null que
// manda cuando aún no se ha transcodificado.
const videoCast = `{"id":"v1","tenant":"acme","owner":{"sub":"u1","name":"Ana Pérez"},"kind":"upload",
"status":"ready","visibility":"public","title":"Presentación","description":"",
"duration_ms":125000,"width":1280,"height":720,"poster_url":"/media/v1/tok.9/poster.jpg",
"comments_enabled":true,"chat_enabled":true,"download_enabled":false,"live":null,"views":12,
"failure":"","published_at":"2026-09-20T10:00:00Z","created_at":"2026-09-20T09:00:00Z",
"updated_at":"2026-09-20T10:00:00Z","can":{"edit":true,"delete":true,"moderate":true}}`

const directoCast = `{"id":"v2","tenant":"acme","owner":{"sub":"u1","name":"Ana"},"kind":"live",
"status":"live","visibility":"org","title":"Directo","description":"","duration_ms":null,
"width":null,"height":null,"poster_url":null,"comments_enabled":true,"chat_enabled":true,
"download_enabled":false,"live":{"source":"rtmp","started_at":"2026-09-28T08:00:00Z",
"ended_at":null,"viewers_now":12,"viewers_peak":40},"views":0,"failure":"","published_at":null,
"created_at":"2026-09-28T07:00:00Z","updated_at":"2026-09-28T08:00:00Z",
"can":{"edit":false,"delete":false,"moderate":false}}`

// TestCastHablaElCableDelServicio recorre las rutas del cliente contra un doble que
// contesta lo que contesta services/cast/internal/http, y comprueba método, ruta,
// consulta y cuerpo de cada una.
func TestCastHablaElCableDelServicio(t *testing.T) {
	var vistas []string
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cuerpo, _ := io.ReadAll(r.Body)
		mu.Lock()
		vistas = append(vistas, r.Method+" "+r.URL.RequestURI()+" "+string(cuerpo))
		mu.Unlock()
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/videos":
			_, _ = io.WriteString(w, `{"videos":[`+videoCast+`,`+directoCast+`],"next_cursor":"c2"}`)
		case "GET /api/v1/videos/shared":
			_, _ = io.WriteString(w, `{"videos":[]}`)
		case "GET /api/v1/videos/v1", "PATCH /api/v1/videos/v1", "POST /api/v1/videos", "POST /api/v1/videos/v1/poster:frame":
			_, _ = io.WriteString(w, videoCast)
		case "DELETE /api/v1/videos/v1", "DELETE /api/v1/videos/v1/people/u9", "DELETE /api/v1/videos/v1/comments/k1":
			w.WriteHeader(http.StatusNoContent)
		case "GET /api/v1/videos/v1/play":
			_, _ = io.WriteString(w, `{"kind":"upload","hls_url":"/media/v1/t/hls/master.m3u8","webm_url":"/media/v1/t/webm/720p.webm","poster_url":null,"duration_ms":125000}`)
		case "GET /api/v1/public/videos/v2/play":
			_, _ = io.WriteString(w, `{"kind":"live","hls_url":null,"webm_url":null,"poster_url":null,"duration_ms":null,"live":{"server_url":"wss://sfu","token":"jwt","room":"cast-acme-v2","identity":"anon-1"}}`)
		case "GET /api/v1/videos/v1/people":
			_, _ = io.WriteString(w, `{"people":[{"user_id":"u9","name":"Bea","username":"bea","added_at":"2026-09-20T10:00:00Z"}]}`)
		case "POST /api/v1/videos/v1/people":
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"user_id":"u9","name":"Bea","username":"bea","added_at":"2026-09-20T10:00:00Z"}`)
		case "GET /api/v1/quota":
			_, _ = io.WriteString(w, `{"limit_bytes":5368709120,"used_bytes":1073741824,"available_bytes":4294967296,"cast_bytes":536870912}`)
		case "GET /api/v1/studio/stream-key", "POST /api/v1/studio/stream-key:rotate":
			_, _ = io.WriteString(w, `{"rtmp_url":"rtmp://cast-ingest.sytue.eu/live","stream_key":"sk_1","ingress_id":"IN_1"}`)
		case "POST /api/v1/lives":
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"video":`+directoCast+`,"publish":{"server_url":"wss://sfu","token":"pub","room":"cast-acme-v2","identity":"emisor-u1"}}`)
		case "GET /api/v1/lives/v2/publish":
			_, _ = io.WriteString(w, `{"server_url":"wss://sfu","token":"pub","room":"cast-acme-v2","identity":"emisor-u1"}`)
		case "POST /api/v1/lives/v2/end":
			_, _ = io.WriteString(w, directoCast)
		case "GET /api/v1/videos/v1/comments", "GET /api/v1/public/videos/v1/comments":
			_, _ = io.WriteString(w, `{"comments":[{"id":"k1","author":{"sub":"u1","name":"Ana"},"body":"Hola","created_at":"2026-09-20T10:00:00Z","can":{"delete":true}}],"next_cursor":""}`)
		case "POST /api/v1/videos/v1/comments":
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"id":"k2","author":{"sub":"u1","name":"Ana"},"body":"Otra","reply_to":"k1","created_at":"2026-09-20T10:00:00Z","can":{"delete":true}}`)
		case "GET /api/v1/videos/v2/chat":
			_, _ = io.WriteString(w, `{"messages":[{"id":"m1","author":{"sub":"u1","name":"Ana"},"body":"hola","created_at":"2026-09-28T08:01:00Z"}]}`)
		case "GET /api/v1/public/explore":
			_, _ = io.WriteString(w, `{"live":[`+directoCast+`],"videos":[`+videoCast+`],"next_cursor":"e2"}`)
		case "GET /api/v1/public/c/acme":
			_, _ = io.WriteString(w, `{"org":"acme","name":"acme","videos":[`+videoCast+`],"next_cursor":""}`)
		case "GET /api/v1/public/videos/v1":
			_, _ = io.WriteString(w, videoCast)
		default:
			t.Errorf("petición inesperada: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	c, ctx := NewCast(srv.URL, nil, nil), context.Background()

	p, err := c.Videos(ctx, CastMine, CastStatusReady, "pres", "c1", 24)
	if err != nil || p.NextCursor != "c2" || len(p.Videos) != 2 {
		t.Fatalf("biblioteca: %+v, %v", p, err)
	}
	v, d := p.Videos[0], p.Videos[1]
	if v.Title != "Presentación" || v.DurationMS != 125000 || v.Width != 1280 || v.PosterURL == "" || !v.Can.Edit || v.Live != nil || v.Owner.Name != "Ana Pérez" {
		t.Errorf("vídeo mal leído: %+v", v)
	}
	if !d.EnDirecto() || d.Live == nil || d.Live.ViewersNow != 12 || d.DurationMS != 0 || d.PosterURL != "" || !d.Fecha().Equal(d.CreatedAt) {
		t.Errorf("directo mal leído: %+v", d)
	}
	if _, err := c.Shared(ctx, ""); err != nil {
		t.Error(err)
	}
	if _, err := c.Create(ctx, CastNew{Title: "Nuevo", Visibility: CastOrg, Source: "rtmp"}); err != nil {
		t.Error(err)
	}
	titulo, sin := "Otro", false
	if _, err := c.Update(ctx, "v1", CastChanges{Title: &titulo, CommentsEnabled: &sin}); err != nil {
		t.Error(err)
	}
	if err := c.Delete(ctx, "v1"); err != nil {
		t.Error(err)
	}
	if _, err := c.PosterFrame(ctx, "v1", 12000); err != nil {
		t.Error(err)
	}
	play, err := c.Play(ctx, "v1")
	if err != nil || play.WebMURL != "/media/v1/t/webm/720p.webm" || play.Live != nil || play.PosterURL != "" {
		t.Errorf("play: %+v, %v", play, err)
	}
	pp, err := c.PublicPlay(ctx, "v2")
	if err != nil || pp.Live == nil || pp.Live.Token != "jwt" || pp.Live.ServerURL != "wss://sfu" {
		t.Errorf("play público: %+v, %v", pp, err)
	}
	if ps, err := c.People(ctx, "v1"); err != nil || len(ps) != 1 || ps[0].Username != "bea" {
		t.Errorf("personas: %+v, %v", ps, err)
	}
	if _, err := c.AddPerson(ctx, "v1", "@bea"); err != nil {
		t.Error(err)
	}
	if err := c.RemovePerson(ctx, "v1", "u9"); err != nil {
		t.Error(err)
	}
	if q, err := c.Quota(ctx); err != nil || q.AvailableBytes != 4294967296 || q.CastBytes != 536870912 {
		t.Errorf("cuota: %+v, %v", q, err)
	}
	if k, err := c.StreamKey(ctx); err != nil || k.StreamKey != "sk_1" || k.RTMPURL == "" {
		t.Errorf("clave: %+v, %v", k, err)
	}
	if _, err := c.RotateStreamKey(ctx); err != nil {
		t.Error(err)
	}
	vd, pub, err := c.ScheduleLive(ctx, CastNew{Title: "Directo", Source: "webrtc"})
	if err != nil || vd.ID != "v2" || pub == nil || pub.Identity != "emisor-u1" {
		t.Errorf("programar: %+v %+v %v", vd, pub, err)
	}
	if _, err := c.PublishToken(ctx, "v2"); err != nil {
		t.Error(err)
	}
	if _, err := c.EndLive(ctx, "v2"); err != nil {
		t.Error(err)
	}
	if cs, _, err := c.Comments(ctx, "v1", ""); err != nil || len(cs) != 1 || !cs[0].Can.Delete {
		t.Errorf("comentarios: %+v %v", cs, err)
	}
	if cs, _, err := c.PublicComments(ctx, "v1", ""); err != nil || len(cs) != 1 {
		t.Errorf("comentarios públicos: %+v %v", cs, err)
	}
	if k, err := c.Comment(ctx, "v1", "Otra", "k1"); err != nil || k.ReplyTo != "k1" {
		t.Errorf("comentar: %+v %v", k, err)
	}
	if err := c.DeleteComment(ctx, "v1", "k1"); err != nil {
		t.Error(err)
	}
	if ms, err := c.ChatHistory(ctx, "v2", time.Date(2026, 9, 28, 8, 0, 0, 0, time.UTC)); err != nil || len(ms) != 1 {
		t.Errorf("chat: %+v %v", ms, err)
	}
	if e, err := c.Explore(ctx, "", 30); err != nil || len(e.Live) != 1 || len(e.Videos) != 1 || e.NextCursor != "e2" {
		t.Errorf("explorar: %+v %v", e, err)
	}
	if ch, err := c.Channel(ctx, "acme", ""); err != nil || ch.Org != "acme" || len(ch.Videos) != 1 {
		t.Errorf("canal: %+v %v", ch, err)
	}
	if _, err := c.PublicVideo(ctx, "v1"); err != nil {
		t.Error(err)
	}

	quiero := []string{
		"GET /api/v1/videos?cursor=c1&limit=24&q=pres&scope=mine&status=ready ",
		"GET /api/v1/videos/shared ",
		`POST /api/v1/videos {"title":"Nuevo","visibility":"org"}`,
		`PATCH /api/v1/videos/v1 {"title":"Otro","comments_enabled":false}`,
		"DELETE /api/v1/videos/v1 ",
		`POST /api/v1/videos/v1/poster:frame {"at_ms":12000}`,
		"GET /api/v1/videos/v1/play ",
		"GET /api/v1/public/videos/v2/play ",
		"GET /api/v1/videos/v1/people ",
		`POST /api/v1/videos/v1/people {"query":"@bea"}`,
		"DELETE /api/v1/videos/v1/people/u9 ",
		"GET /api/v1/quota ",
		"GET /api/v1/studio/stream-key ",
		"POST /api/v1/studio/stream-key:rotate ",
		`POST /api/v1/lives {"title":"Directo","source":"webrtc"}`,
		"GET /api/v1/lives/v2/publish ",
		"POST /api/v1/lives/v2/end ",
		"GET /api/v1/videos/v1/comments ",
		"GET /api/v1/public/videos/v1/comments ",
		`POST /api/v1/videos/v1/comments {"body":"Otra","reply_to":"k1"}`,
		"DELETE /api/v1/videos/v1/comments/k1 ",
		"GET /api/v1/videos/v2/chat?since=2026-09-28T08%3A00%3A00Z ",
		"GET /api/v1/public/explore?limit=30 ",
		"GET /api/v1/public/c/acme ",
		"GET /api/v1/public/videos/v1 ",
	}
	if len(vistas) != len(quiero) {
		t.Fatalf("%d peticiones y se esperaban %d:\n%s", len(vistas), len(quiero), strings.Join(vistas, "\n"))
	}
	for i := range quiero {
		if vistas[i] != quiero[i] {
			t.Errorf("petición %d:\n  es      %s\n  debería %s", i, vistas[i], quiero[i])
		}
	}
}

// TestCastDiceCuantoQuedaConUn507: el `details.available_bytes` del sobre de httpx es
// lo que la pantalla de subir enseña cuando no cabe.
func TestCastDiceCuantoQuedaConUn507(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "video/webm" || r.ContentLength != 5 {
			t.Errorf("subida con tipo %q y tamaño %d", r.Header.Get("Content-Type"), r.ContentLength)
		}
		w.WriteHeader(http.StatusInsufficientStorage)
		_, _ = io.WriteString(w, `{"error":{"code":"quota_exceeded","message":"Not enough storage left in your plan.","details":{"available_bytes":1024}},"request_id":"r"}`)
	}))
	defer srv.Close()
	_, err := NewCast(srv.URL, nil, nil).Upload(context.Background(), "v1", strings.NewReader("12345"), 5, "video/webm")
	quedan, ok := CastQuotaAvailable(err)
	if !ok || quedan != 1024 {
		t.Fatalf("CastQuotaAvailable(%v) = %d, %v", err, quedan, ok)
	}
	if _, ok := CastQuotaAvailable(&Error{Status: 500}); ok {
		t.Error("un 500 no es falta de cuota")
	}
}

// TestCastMediaLeePorRangos: el reproductor busca en el WebM sin bajarlo entero. Se
// comprueba que un salto pide solo el rango nuevo y que lo leído es lo que hay.
func TestCastMediaLeePorRangos(t *testing.T) {
	datos := bytes.Repeat([]byte("0123456789"), 300_000) // 3 MB
	var rangos []string
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/media/v1/tok/webm/720p.webm" {
			t.Errorf("ruta %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "" {
			t.Error("los ficheros firmados van sin sesión")
		}
		mu.Lock()
		rangos = append(rangos, r.Method+" "+r.Header.Get("Range"))
		mu.Unlock()
		http.ServeContent(w, r, "720p.webm", time.Time{}, bytes.NewReader(datos))
	}))
	defer srv.Close()
	m, err := NewCast(srv.URL, nil, nil).Media(context.Background(), "/media/v1/tok/webm/720p.webm")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Close() }()
	if m.Size() != int64(len(datos)) {
		t.Fatalf("tamaño %d", m.Size())
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(m, buf); err != nil || string(buf) != "0123" {
		t.Fatalf("primer tramo: %q %v", buf, err)
	}
	if _, err := m.Seek(2_500_003, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadFull(m, buf); err != nil || string(buf) != "3456" {
		t.Fatalf("tras el salto: %q %v", buf, err)
	}
	resto, err := io.ReadAll(m)
	if err != nil || len(resto) != len(datos)-2_500_007 {
		t.Fatalf("hasta el final: %d bytes, %v", len(resto), err)
	}
	quiero := []string{"HEAD ", "GET bytes=0-1048575", "GET bytes=2500003-2999999"}
	if strings.Join(rangos, "|") != strings.Join(quiero, "|") {
		t.Errorf("rangos pedidos %q y se esperaban %q", rangos, quiero)
	}
}

// TestCastChatEnVivo: el WebSocket del chat, con el token en la cabecera, entrega las
// tramas y manda `send`.
func TestCastChatEnVivo(t *testing.T) {
	recibido := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ws/videos/v2/chat" || r.Header.Get("Authorization") != "Bearer tk" {
			t.Errorf("handshake en %s con %q", r.URL.Path, r.Header.Get("Authorization"))
		}
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()
		ctx := r.Context()
		_ = wsjson.Write(ctx, conn, map[string]any{"type": "viewers", "count": 7})
		_ = wsjson.Write(ctx, conn, map[string]any{"type": "message", "id": "m1", "author": map[string]string{"sub": "u1", "name": "Ana"}, "body": "hola", "created_at": "2026-09-28T08:01:00Z"})
		var in map[string]string
		if err := wsjson.Read(ctx, conn, &in); err == nil {
			b, _ := json.Marshal(in)
			recibido <- string(b)
		}
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}))
	defer srv.Close()
	tramas := make(chan CastChatFrame, 4)
	cerrado := make(chan error, 1)
	c := NewCast(srv.URL, sesionFija("tk"), nil)
	ch, err := c.Chat(context.Background(), "v2", func(f CastChatFrame) { tramas <- f }, func(err error) { cerrado <- err })
	if err != nil {
		t.Fatal(err)
	}
	defer ch.Close()
	if f := <-tramas; f.Type != "viewers" || f.Count != 7 {
		t.Errorf("primera trama %+v", f)
	}
	if f := <-tramas; f.Type != "message" || f.Author == nil || f.Author.Name != "Ana" || f.Body != "hola" {
		t.Errorf("segunda trama %+v", f)
	}
	if err := ch.Send(context.Background(), "buenas"); err != nil {
		t.Fatal(err)
	}
	if got := <-recibido; got != `{"body":"buenas","type":"send"}` {
		t.Errorf("enviado %s", got)
	}
	select {
	case err := <-cerrado:
		if err != nil {
			t.Errorf("cierre normal con error %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("el cierre del servidor no llegó")
	}
}

// sesionFija es una sesión que siempre da el mismo token.
type sesionFija string

func (s sesionFija) AccessToken(context.Context) (string, error) { return string(s), nil }
func (sesionFija) Invalidate(string)                             {}
