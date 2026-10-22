// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// El ciclo completo del directo contra un PostgreSQL real (platform/pkg/pgtest)
// y un LiveKit de mentira (httptest) que anota cada llamada Twirp: la clave de
// OBS, programar, el webhook que lo pone en emisión y arranca la grabación, los
// espectadores, el fin, la grabación encolada, OBS sin programar nada, la clave
// rotada, la resolución por ingress que cruza organizaciones bajo RLS, y el
// sondeo de respaldo. Ejecuta con PGTEST_DSN o con contenedores:
// `go test -tags integration -p 1 ./services/cast/internal/directo/`.
package directo

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/livekit"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/cast/db"
	"git.sytue.eu/sytue/one/services/cast/internal/domain"
	"git.sytue.eu/sytue/one/services/cast/internal/media"
	"git.sytue.eu/sytue/one/services/cast/internal/service"
	"git.sytue.eu/sytue/one/services/cast/internal/store"
)

// livekitFalso es un SFU de mentira: contesta a lo que el directo le pide y
// anota cada llamada con su cuerpo.
type livekitFalso struct {
	mu             sync.Mutex
	llamadas       []llamada
	ingresos       int
	egresos        int
	rooms          []livekit.Room
	presentes      map[string][]livekit.ParticipantInfo
	resultados     map[string]livekit.EgressInfo
	codigoConsulta string
}

type llamada struct {
	Metodo string
	Cuerpo map[string]any
}

func (f *livekitFalso) servidor(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		metodo := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		cuerpo := map[string]any{}
		_ = json.NewDecoder(r.Body).Decode(&cuerpo)
		f.mu.Lock()
		defer f.mu.Unlock()
		f.llamadas = append(f.llamadas, llamada{Metodo: metodo, Cuerpo: cuerpo})
		w.Header().Set("Content-Type", "application/json")
		var out any
		switch metodo {
		case "CreateIngress":
			f.ingresos++
			id := "IN_" + strings.Repeat("x", f.ingresos)
			out = map[string]any{"ingress_id": id, "stream_key": "sk-" + id, "url": "rtmp://livekit/live", "room_name": cuerpo["room_name"]}
		case "UpdateIngress":
			out = map[string]any{"ingress_id": cuerpo["ingress_id"], "stream_key": "sk", "room_name": cuerpo["room_name"]}
		case "StartParticipantEgress":
			f.egresos++
			out = map[string]any{"egress_id": "EG_" + strings.Repeat("y", f.egresos), "status": "EGRESS_STARTING"}
		case "StopEgress":
			out = map[string]any{"egress_id": cuerpo["egress_id"], "status": "EGRESS_ENDING"}
		case "ListEgress":
			if f.codigoConsulta != "" {
				w.WriteHeader(http.StatusNotFound)
				out = map[string]string{"code": f.codigoConsulta, "msg": "egress no disponible"}
			} else {
				items := []livekit.EgressInfo{}
				id, _ := cuerpo["egress_id"].(string)
				if eg, ok := f.resultados[id]; ok {
					items = append(items, eg)
				}
				out = map[string]any{"items": items}
			}
		case "ListRooms":
			out = map[string]any{"rooms": f.rooms}
		case "ListParticipants":
			room, _ := cuerpo["room"].(string)
			out = map[string]any{"participants": f.presentes[room]}
		default:
			out = map[string]any{}
		}
		_ = json.NewEncoder(w).Encode(out)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func (f *livekitFalso) cuenta(metodo string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, l := range f.llamadas {
		if l.Metodo == metodo {
			n++
		}
	}
	return n
}

func (f *livekitFalso) ultima(metodo string) map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.llamadas) - 1; i >= 0; i-- {
		if f.llamadas[i].Metodo == metodo {
			return f.llamadas[i].Cuerpo
		}
	}
	return nil
}

// banco es todo lo que hace falta para probar el directo.
type banco struct {
	ctx   context.Context
	s     *Servicio
	st    *store.Store
	falso *livekitFalso
	firma *livekit.Signer
}

func montar(t *testing.T) *banco {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	svc := service.New(service.Deps{Store: st, Firmante: media.New("clave-de-prueba")})
	falso := &livekitFalso{presentes: map[string][]livekit.ParticipantInfo{}}
	srv := falso.servidor(t)
	firma := livekit.NewSigner("api-key", "api-secret")
	s := New(Deps{
		Pool: h.Pool, Store: st, Servicio: svc,
		LiveKit: livekit.NewClient(srv.URL, firma), Firmante: firma,
		Cfg: Config{
			LiveKitURL: "wss://sfu.sytue.test", IngestRTMPURL: "rtmp://cast-ingest.sytue.test/live", PublicURL: "https://cast.sytue.test",
			Grabacion: livekit.DestinoS3{AccessKey: "ak", Secret: "sk", Bucket: "cast-medios", Endpoint: "https://s3.test"},
		},
	})
	return &banco{ctx: ctx, s: s, st: st, falso: falso, firma: firma}
}

// aviso firma y entrega un webhook como haría LiveKit.
func (b *banco) aviso(t *testing.T, cuerpo string) {
	t.Helper()
	tok, err := b.firma.SignWebhook([]byte(cuerpo), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.s.Webhook(b.ctx, tok, []byte(cuerpo)); err != nil {
		t.Fatalf("webhook %s: %v", cuerpo, err)
	}
}

func (b *banco) video(t *testing.T, tenant string, id uuid.UUID) domain.Video {
	t.Helper()
	v, err := b.st.EnOrganizacion(b.ctx, tenant, id)
	if err != nil {
		t.Fatalf("leer vídeo: %v", err)
	}
	return v
}

var (
	ana = service.Quien{Sub: "ana", Tenant: "acme", Nombre: "Ana Pérez"}
	zoe = service.Quien{Sub: "zoe", Tenant: "globex", Nombre: "Zoe"}
)

func TestMigracionesYRLSDelDirecto(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	pgtest.ExigirRLSForzada(t, h, nil)
}

func TestCicloDelDirectoPorRTMP(t *testing.T) {
	b := montar(t)

	// La clave de OBS se crea al primer GET y se repite después sin crear otra.
	c1, err := b.s.StreamKey(b.ctx, ana)
	if err != nil {
		t.Fatalf("StreamKey: %v", err)
	}
	c2, err := b.s.StreamKey(b.ctx, ana)
	if err != nil || c2 != c1 || b.falso.cuenta("CreateIngress") != 1 {
		t.Fatalf("la segunda clave debería ser la misma sin otro ingress: %+v %+v %v", c1, c2, err)
	}
	if c1.RTMPURL != "rtmp://cast-ingest.sytue.test/live" || c1.StreamKey != "sk-IN_x" || c1.IngressID != "IN_x" {
		t.Errorf("clave: %+v", c1)
	}
	creado := b.falso.ultima("CreateIngress")
	if creado["participant_identity"] != "emisor-ana" || creado["participant_name"] != "Ana Pérez" {
		t.Errorf("ingress creado: %v", creado)
	}
	roomEspera, _ := creado["room_name"].(string)
	if tenant, ok := roomDe(roomEspera); !ok || tenant != "acme" {
		t.Errorf("la room de espera no tiene la forma de Cast: %q", roomEspera)
	}

	// Programar por OBS apunta el ingress a la room del directo.
	v, emision, err := b.s.Programar(b.ctx, ana, Programa{Title: "Charla", Visibility: domain.VisibilityPublic})
	if err != nil {
		t.Fatalf("Programar: %v", err)
	}
	if emision != nil {
		t.Error("un directo por OBS no lleva token de emisor")
	}
	if v.Status != domain.StatusScheduled || v.LiveSource != domain.LiveSourceRTMP || v.LiveRoom != nombreRoom("acme", v.ID) {
		t.Fatalf("programado: %+v", v)
	}
	if u := b.falso.ultima("UpdateIngress"); u["ingress_id"] != "IN_x" || u["room_name"] != v.LiveRoom {
		t.Errorf("el ingress no se apuntó al directo: %v", u)
	}

	// OBS conecta: LiveKit avisa con el ingress y la room. El directo pasa a live
	// y arranca la grabación en el prefijo del tenant.
	b.aviso(t, `{"event":"ingress_started","ingressInfo":{"ingressId":"IN_x","roomName":"`+v.LiveRoom+`","participantIdentity":"emisor-ana","state":{"status":"ENDPOINT_PUBLISHING"}}}`)
	v = b.video(t, "acme", v.ID)
	if v.Status != domain.StatusLive || v.LiveStartedAt == nil || v.PublishedAt == nil {
		t.Fatalf("tras ingress_started: %+v", v)
	}
	if v.LiveEgressID != "EG_y" {
		t.Fatalf("la grabación no se anotó: %q", v.LiveEgressID)
	}
	eg := b.falso.ultima("StartParticipantEgress")
	if eg["room_name"] != v.LiveRoom || eg["identity"] != "emisor-ana" {
		t.Errorf("egress: %v", eg)
	}
	seg := eg["segment_outputs"].([]any)[0].(map[string]any)
	if seg["playlist_name"] != "tenants/acme/"+v.ID.String()+"/live/index.m3u8" {
		t.Errorf("playlist de la grabación: %v", seg["playlist_name"])
	}
	// Repetir el aviso (o que llegue también la pista) no arranca dos grabaciones.
	b.aviso(t, `{"event":"track_published","room":{"name":"`+v.LiveRoom+`"},"participant":{"identity":"emisor-ana","kind":"INGRESS"},"track":{"type":"VIDEO"}}`)
	if b.falso.cuenta("StartParticipantEgress") != 1 {
		t.Error("se arrancó una segunda grabación")
	}

	// Espectadores: cuentan las personas, no el emisor, el ingress ni el egress.
	b.aviso(t, `{"event":"participant_joined","room":{"name":"`+v.LiveRoom+`"},"participant":{"identity":"zoe","kind":"STANDARD"}}`)
	b.aviso(t, `{"event":"participant_joined","room":{"name":"`+v.LiveRoom+`"},"participant":{"identity":"emisor-ana","kind":"INGRESS"}}`)
	b.aviso(t, `{"event":"participant_joined","room":{"name":"`+v.LiveRoom+`"},"participant":{"identity":"EG_y","kind":"EGRESS"}}`)
	b.aviso(t, `{"event":"participant_joined","room":{"name":"`+v.LiveRoom+`"},"participant":{"identity":"anon-0123456789abcdef"}}`)
	b.aviso(t, `{"event":"participant_joined","room":{"name":"`+v.LiveRoom+`"},"participant":{"identity":"zoe","kind":"STANDARD"}}`)
	v = b.video(t, "acme", v.ID)
	if v.ViewersNow != 2 || v.ViewersPeak != 2 {
		t.Fatalf("espectadores = %d (pico %d), quería 2", v.ViewersNow, v.ViewersPeak)
	}
	b.aviso(t, `{"event":"participant_left","room":{"name":"`+v.LiveRoom+`"},"participant":{"identity":"zoe","kind":"STANDARD"}}`)
	v = b.video(t, "acme", v.ID)
	if v.ViewersNow != 1 || v.ViewersPeak != 2 {
		t.Fatalf("tras salir zoe = %d (pico %d)", v.ViewersNow, v.ViewersPeak)
	}

	// OBS desconecta: termina, para la grabación, cierra la room y vacía la lista.
	b.aviso(t, `{"event":"ingress_ended","ingressInfo":{"ingressId":"IN_x","roomName":"`+v.LiveRoom+`"}}`)
	v = b.video(t, "acme", v.ID)
	if v.Status != domain.StatusEnded || v.LiveEndedAt == nil || v.ViewersNow != 0 || v.ViewersPeak != 2 {
		t.Fatalf("tras ingress_ended: %+v", v)
	}
	if b.falso.ultima("StopEgress")["egress_id"] != "EG_y" || b.falso.ultima("DeleteRoom")["room"] != v.LiveRoom {
		t.Error("no se paró la grabación o no se cerró la room")
	}
	// Sin grabación terminada aún, no hay nada que encolar.
	if jobs, _ := b.st.TrabajosDelVideo(b.ctx, "acme", v.ID); len(jobs) != 0 {
		t.Fatalf("se encoló antes de tener la grabación: %+v", jobs)
	}

	// La grabación llega: hls_key y un trabajo live_vod, una sola vez.
	egressEnded := `{"event":"egress_ended","egressInfo":{"egressId":"EG_y","roomName":"` + v.LiveRoom + `","status":"EGRESS_COMPLETE",` +
		`"segmentResults":[{"playlistName":"tenants/acme/` + v.ID.String() + `/live/index.m3u8","duration":"125000000000","size":"48000000"}]}}`
	b.aviso(t, egressEnded)
	b.aviso(t, egressEnded)
	v = b.video(t, "acme", v.ID)
	if v.HlsKey != v.ID.String()+"/live/index.m3u8" {
		t.Errorf("hls_key = %q", v.HlsKey)
	}
	if v.Status != domain.StatusProcessing {
		t.Errorf("encolar la grabación debería dejar el vídeo processing: %s", v.Status)
	}
	jobs, err := b.st.TrabajosDelVideo(b.ctx, "acme", v.ID)
	if err != nil || len(jobs) != 1 || jobs[0].Kind != domain.JobLiveVOD {
		t.Fatalf("trabajos: %+v %v", jobs, err)
	}
	// Los avisos tardíos de una room ya cerrada no rompen nada.
	b.aviso(t, `{"event":"room_finished","room":{"name":"`+v.LiveRoom+`"}}`)
	b.aviso(t, `{"event":"participant_left","room":{"name":"`+v.LiveRoom+`"},"participant":{"identity":"anon-0123456789abcdef"}}`)
}

func TestOBSSinDirectoProgramadoCreaUno(t *testing.T) {
	b := montar(t)
	if _, err := b.s.StreamKey(b.ctx, ana); err != nil {
		t.Fatal(err)
	}
	roomEspera, _ := b.falso.ultima("CreateIngress")["room_name"].(string)

	b.aviso(t, `{"event":"ingress_started","ingressInfo":{"ingressId":"IN_x","roomName":"`+roomEspera+`","participantIdentity":"emisor-ana"}}`)

	vs, err := b.st.Biblioteca(b.ctx, "acme", "ana", false, store.Filtro{OwnerSub: "ana", Limite: 10})
	if err != nil || len(vs) != 1 {
		t.Fatalf("debería haber UN directo creado: %v %v", vs, err)
	}
	v := vs[0]
	if v.Title != "Directo de Ana Pérez" || v.Visibility != domain.VisibilityOrg || v.Kind != domain.KindLive ||
		v.LiveSource != domain.LiveSourceRTMP || v.LiveRoom != roomEspera || v.Status != domain.StatusLive {
		t.Fatalf("directo improvisado: %+v", v)
	}
	// Un segundo aviso del mismo ingress no crea otro.
	b.aviso(t, `{"event":"ingress_started","ingressInfo":{"ingressId":"IN_x","roomName":"`+roomEspera+`","participantIdentity":"emisor-ana"}}`)
	if vs, _ = b.st.Biblioteca(b.ctx, "acme", "ana", false, store.Filtro{OwnerSub: "ana", Limite: 10}); len(vs) != 1 {
		t.Fatalf("se creó un segundo directo: %d", len(vs))
	}
	// Un ingress que no es de Cast se ignora.
	b.aviso(t, `{"event":"ingress_started","ingressInfo":{"ingressId":"IN_ajeno","roomName":"otra"}}`)
}

func TestClavePorIngressCruzaOrganizacionesYSoloEso(t *testing.T) {
	b := montar(t)
	if _, err := b.s.StreamKey(b.ctx, ana); err != nil {
		t.Fatal(err)
	}
	if _, err := b.s.StreamKey(b.ctx, zoe); err != nil {
		t.Fatal(err)
	}
	// Por ingress se resuelve la de cualquiera (política app.cast_ingest).
	c, err := b.s.propio.clavePorIngress(b.ctx, "IN_xx")
	if err != nil || c.Tenant != "globex" || c.OwnerSub != "zoe" {
		t.Fatalf("clavePorIngress: %+v %v", c, err)
	}
	if _, err := b.s.propio.clavePorIngress(b.ctx, "IN_nadie"); !errors.Is(err, domain.ErrNoExiste) {
		t.Fatalf("un ingress desconocido: %v", err)
	}
	// Por organización, cada una ve solo la suya (RLS).
	if _, err := b.s.propio.clave(b.ctx, "globex", "ana"); !errors.Is(err, domain.ErrNoExiste) {
		t.Fatalf("globex leyó la clave de ana (acme): %v", err)
	}
	if c, err := b.s.propio.clave(b.ctx, "acme", "ana"); err != nil || c.IngressID != "IN_x" {
		t.Fatalf("acme no ve la de ana: %+v %v", c, err)
	}
	// Y sin sesión no hay clave.
	if _, err := b.s.StreamKey(b.ctx, service.Nadie); !errors.Is(err, domain.ErrProhibido) {
		t.Fatalf("anónimo pidió clave: %v", err)
	}
}

func TestRotarClaveBorraLaAnterior(t *testing.T) {
	b := montar(t)
	antes, err := b.s.StreamKey(b.ctx, ana)
	if err != nil {
		t.Fatal(err)
	}
	despues, err := b.s.RotarClave(b.ctx, ana)
	if err != nil {
		t.Fatalf("RotarClave: %v", err)
	}
	if despues.StreamKey == antes.StreamKey || despues.IngressID == antes.IngressID {
		t.Fatalf("la clave no cambió: %+v → %+v", antes, despues)
	}
	if b.falso.ultima("DeleteIngress")["ingress_id"] != antes.IngressID {
		t.Error("el ingress anterior no se borró en LiveKit: su clave seguiría valiendo")
	}
	if c, _ := b.s.StreamKey(b.ctx, ana); c != despues {
		t.Errorf("la base no guarda la nueva: %+v", c)
	}
	// Rotar sin tener clave es crear la primera.
	if c, err := b.s.RotarClave(b.ctx, zoe); err != nil || c.StreamKey == "" {
		t.Fatalf("rotar sin clave: %+v %v", c, err)
	}
}

func TestEmisionPorWebRTCYSondeo(t *testing.T) {
	b := montar(t)
	v, emision, err := b.s.Programar(b.ctx, ana, Programa{Title: "Desde el navegador", Source: domain.LiveSourceWebRTC})
	if err != nil {
		t.Fatalf("Programar: %v", err)
	}
	if emision == nil || emision.Identity != "emisor-ana" || emision.Room != v.LiveRoom || emision.ServerURL != "wss://sfu.sytue.test" {
		t.Fatalf("token de emisor: %+v", emision)
	}
	if b.falso.cuenta("UpdateIngress") != 0 {
		t.Error("por WebRTC no hay ingress que apuntar")
	}
	// El token de emisor lo pide su dueño; un admin de la organización, no.
	if _, err := b.s.Publicacion(b.ctx, ana, v.ID); err != nil {
		t.Errorf("la dueña no pudo pedir su token: %v", err)
	}
	admin := service.Quien{Sub: "root", Tenant: "acme", Admin: true}
	if _, err := b.s.Publicacion(b.ctx, admin, v.ID); !errors.Is(err, domain.ErrProhibido) {
		t.Errorf("un administrador consiguió el token de emisor: %v", err)
	}
	if _, err := b.s.Publicacion(b.ctx, zoe, v.ID); !errors.Is(err, domain.ErrNoExiste) {
		t.Errorf("otra organización: %v", err)
	}

	// Sin webhooks: el sondeo ve la room con el emisor publicando y dos personas.
	b.falso.mu.Lock()
	b.falso.rooms = []livekit.Room{{Name: v.LiveRoom}, {Name: "sala-de-rooms"}}
	b.falso.presentes[v.LiveRoom] = []livekit.ParticipantInfo{
		{Identity: "emisor-ana", Kind: livekit.ParticipanteEstandar, Tracks: []livekit.TrackInfo{{SID: "TR_1", Type: "VIDEO"}}},
		{Identity: "zoe", Kind: livekit.ParticipanteEstandar},
		{Identity: "anon-00000000deadbeef"},
		{Identity: "EG_y", Kind: livekit.ParticipanteEgress},
	}
	b.falso.mu.Unlock()
	if err := b.s.Sondear(b.ctx); err != nil {
		t.Fatalf("Sondear: %v", err)
	}
	v = b.video(t, "acme", v.ID)
	if v.Status != domain.StatusLive || v.ViewersNow != 2 || v.LiveEgressID == "" {
		t.Fatalf("tras el sondeo: status=%s viewers=%d egress=%q", v.Status, v.ViewersNow, v.LiveEgressID)
	}
	// Otra ronda igual no cambia nada ni arranca otra grabación; el webhook
	// tardío tampoco.
	if err := b.s.Sondear(b.ctx); err != nil {
		t.Fatal(err)
	}
	b.aviso(t, `{"event":"track_published","room":{"name":"`+v.LiveRoom+`"},"participant":{"identity":"emisor-ana"},"track":{"type":"AUDIO"}}`)
	if b.falso.cuenta("StartParticipantEgress") != 1 {
		t.Errorf("grabaciones arrancadas: %d", b.falso.cuenta("StartParticipantEgress"))
	}
	// Se va una persona: la lista queda como la de LiveKit.
	b.falso.mu.Lock()
	b.falso.presentes[v.LiveRoom] = b.falso.presentes[v.LiveRoom][:2]
	b.falso.mu.Unlock()
	if err := b.s.Sondear(b.ctx); err != nil {
		t.Fatal(err)
	}
	if v = b.video(t, "acme", v.ID); v.ViewersNow != 1 || v.ViewersPeak != 2 {
		t.Fatalf("viewers = %d pico %d", v.ViewersNow, v.ViewersPeak)
	}
	// La room desaparece de LiveKit: el directo termina.
	b.falso.mu.Lock()
	b.falso.rooms = nil
	b.falso.mu.Unlock()
	if err := b.s.Sondear(b.ctx); err != nil {
		t.Fatal(err)
	}
	if v = b.video(t, "acme", v.ID); v.Status != domain.StatusEnded || v.ViewersNow != 0 {
		t.Fatalf("tras perder la room: %+v", v)
	}
	if b.falso.cuenta("StopEgress") != 1 {
		t.Error("la grabación no se paró al terminar")
	}
}

func TestTerminarLoHaceQuienEdita(t *testing.T) {
	b := montar(t)
	v, _, err := b.s.Programar(b.ctx, ana, Programa{Title: "Charla"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.s.Terminar(b.ctx, zoe, v.ID); !errors.Is(err, domain.ErrNoExiste) {
		t.Errorf("otra organización terminó el directo: %v", err)
	}
	otro := service.Quien{Sub: "luis", Tenant: "acme"}
	if _, err := b.s.Terminar(b.ctx, otro, v.ID); !errors.Is(err, domain.ErrNoExiste) {
		// Un directo programado aún no se ve: para quien no lo edita no existe.
		t.Errorf("alguien de la organización sin permiso terminó el directo: %v", err)
	}
	admin := service.Quien{Sub: "root", Tenant: "acme", Admin: true}
	v2, err := b.s.Terminar(b.ctx, admin, v.ID)
	if err != nil || v2.Status != domain.StatusEnded {
		t.Fatalf("el administrador debería poder terminarlo: %+v %v", v2, err)
	}
	if _, err := b.s.Terminar(b.ctx, ana, v.ID); !errors.Is(err, domain.ErrConflicto) {
		t.Errorf("terminar dos veces: %v", err)
	}
}
