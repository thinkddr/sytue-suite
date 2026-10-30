// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cast

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/apps/photos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func TestMain(m *testing.M) {
	test.NewApp().Settings().SetTheme(tema.Sytue{})
	tema.FormaDeLosGuardianes()
	m.Run()
}

// La ventana con la que se miden las pantallas: 1280×800, es-ES.
const (
	ventanaAncho float32 = 1280
	ventanaAlto  float32 = 800
)

// ahoraFijo es el reloj de los tests: las fechas relativas salen siempre igual.
var ahoraFijo = time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)

// fixtureWebM es el WebM de 2 s del paquete del reproductor.
const fixtureWebM = "../../video/testdata/dos-segundos.webm"

// ---- Los datos de prueba ----

func fecha(dias int) *time.Time {
	t := ahoraFijo.Add(-time.Duration(dias) * 24 * time.Hour)
	return &t
}

// videoDePrueba es un vídeo listo, con su póster y su duración.
func videoDePrueba(id, titulo, autor string, dias int, dur time.Duration, vistas int64) api.CastVideo {
	return api.CastVideo{
		ID: id, Tenant: "acme", Owner: api.CastOwner{Sub: "u-" + autor, Name: autor}, Kind: "upload",
		Status: api.CastStatusReady, Visibility: api.CastPublic, Title: titulo,
		DurationMS: dur.Milliseconds(), Width: 1280, Height: 720,
		PosterURL: "/media/" + id + "/tok.9/poster.jpg", CommentsEnabled: true, ChatEnabled: true,
		Views: vistas, PublishedAt: fecha(dias), CreatedAt: *fecha(dias), UpdatedAt: *fecha(dias),
	}
}

// Los vídeos del servicio falso: los míos (con uno procesando y otro con error), un
// directo de la organización, lo compartido y lo último de la organización.
func misVideos() []api.CastVideo {
	vs := []api.CastVideo{
		videoDePrueba("v01", "Presentación de resultados del tercer trimestre", "Ana Pérez", 1, 18*time.Minute+42*time.Second, 312),
		videoDePrueba("v02", "Formación: firma electrónica en Sytue Docs", "Ana Pérez", 4, 7*time.Minute+5*time.Second, 1204),
		videoDePrueba("v03", "Bienvenida al equipo de soporte", "Ana Pérez", 12, time.Hour+2*time.Minute+3*time.Second, 88),
		videoDePrueba("v04", "Demostración del nuevo panel de facturas", "Ana Pérez", 0, 0, 0),
	}
	for i := range vs {
		vs[i].Can = api.CastCan{Edit: true, Delete: true, Moderate: true}
	}
	vs[1].Visibility = api.CastOrg
	vs[2].Visibility = api.CastUnlisted
	vs[3].Status, vs[3].PosterURL, vs[3].Visibility = api.CastStatusProcessing, "", api.CastPeople
	return vs
}

func directoDePrueba() api.CastVideo {
	d := videoDePrueba("v10", "Reunión general de septiembre", "Luis Gómez", 0, 0, 0)
	d.Kind, d.Status, d.Visibility, d.DurationMS = "live", api.CastStatusLive, api.CastOrg, 0
	d.Live = &api.CastLive{Source: "rtmp", StartedAt: fecha(0), ViewersNow: 42, ViewersPeak: 57}
	return d
}

func deLaOrganizacion() []api.CastVideo {
	return []api.CastVideo{
		videoDePrueba("v20", "Cómo pedir vacaciones en el portal", "Marta Ruiz", 2, 3*time.Minute+30*time.Second, 540),
		videoDePrueba("v21", "Novedades de Sytue Mail en octubre", "Jon Arrieta", 6, 9*time.Minute+12*time.Second, 76),
		videoDePrueba("v22", "Seguridad: claves de acceso paso a paso", "Irene Soler", 9, 11*time.Minute, 1930),
		videoDePrueba("v23", "Visita a la planta de Zaragoza", "Pau Vidal", 20, 25*time.Minute+8*time.Second, 402),
	}
}

var postersCache sync.Map

// posterDe es una imagen plana de colores de la marca, distinta para cada vídeo: una
// banda, un sol y el suelo. Sin degradados, como pide el listón del producto.
func posterDe(id string) []byte {
	if guardado, ok := postersCache.Load(id); ok {
		return slices.Clone(guardado.([]byte))
	}
	n := 0
	for _, c := range id {
		n = n*31 + int(c)
	}
	paletas := [][3]color.RGBA{
		{{0x1f, 0x3a, 0x4d, 255}, {0xe8, 0xb0, 0x4b, 255}, {0x2d, 0x5e, 0x4f, 255}},
		{{0x9f, 0x12, 0x39, 255}, {0xf4, 0xd3, 0xc4, 255}, {0x5a, 0x0b, 0x22, 255}},
		{{0x24, 0x4b, 0x7a, 255}, {0xf0, 0xf0, 0xe8, 255}, {0x33, 0x33, 0x40, 255}},
		{{0x3d, 0x5a, 0x3c, 255}, {0xf2, 0xc1, 0x4e, 255}, {0x6b, 0x4f, 0x2c, 255}},
	}
	p := paletas[n%len(paletas)]
	const w, h = 640, 360
	m := image.NewRGBA(image.Rect(0, 0, w, h))
	cx, cy := 200+(n%240), 130
	for y := range h {
		for x := range w {
			c := p[0]
			if (x-cx)*(x-cx)+(y-cy)*(y-cy) < 48*48 {
				c = p[1]
			}
			if y > 250-(x/12)%18 {
				c = p[2]
			}
			m.Set(x, y, c)
		}
	}
	var b bytes.Buffer
	_ = jpeg.Encode(&b, m, &jpeg.Options{Quality: 85})
	raw := b.Bytes()
	postersCache.Store(id, raw)
	return slices.Clone(raw)
}

// ---- El servicio falso ----

// servicioFalso contesta como services/cast lo que usan las pantallas, y apunta lo
// que se le pide.
type servicioFalso struct {
	mu          sync.Mutex
	pedido      []string
	mios        []api.CastVideo
	org         []api.CastVideo
	directos    []api.CastVideo
	comp        []api.CastVideo
	personas    []api.CastPerson
	subido      int64
	clave       int
	comentarios []api.CastComment
	// sinSitio contesta a la subida del original con el 507 de la cuota.
	sinSitio bool
	// falla hace que toda la API conteste con este estado.
	falla int
	// calidades es la elección guardada de los vídeos (GET/PUT /qualities); nil, la
	// recomendada de un 1080p.
	calidades []int
	// escalera hace que play traiga los tres WebM (1080, 720 y 360); sin ella, el de
	// antes de la escalera (solo webm_url).
	escalera bool
	// medios son los WebM que se han pedido a /media.
	medios []string
	// Denuncia y bloqueo: la cola de la organización, los bloqueos de quien mira y
	// los de la organización. noAdministra contesta 403 a lo de /moderation.
	denuncias    []api.CastReport
	bloqueos     []api.CastBlocked
	bloqueosOrg  []api.CastBlocked
	noAdministra bool
}

func servicioCompleto() *servicioFalso {
	return &servicioFalso{
		mios: misVideos(), org: deLaOrganizacion(), directos: []api.CastVideo{directoDePrueba()},
		comp: []api.CastVideo{videoDePrueba("v30", "Revisión del contrato con proveedores", "Sara Blanco", 3, 14*time.Minute, 12)},
		comentarios: []api.CastComment{
			{ID: "k1", Author: api.CastAuthor{Sub: "u2", Name: "Marta Ruiz"}, Body: "Muy claro, gracias. ¿Se puede compartir la hoja de la diapositiva 12?", CreatedAt: ahoraFijo.Add(-3 * time.Hour)},
			{ID: "k2", Author: api.CastAuthor{Sub: "u-Ana Pérez", Name: "Ana Pérez"}, Body: "Sí, está en la carpeta del trimestre en Drive.", ReplyTo: "k1", CreatedAt: ahoraFijo.Add(-2 * time.Hour)},
			{ID: "k3", Author: api.CastAuthor{Sub: "u3", Name: "Jon Arrieta"}, Body: "Las cifras de la región norte cuadran con lo que vimos en la reunión.", CreatedAt: ahoraFijo.Add(-40 * time.Minute)},
		},
	}
}

func (s *servicioFalso) vistas() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string{}, s.pedido...)
}

func (s *servicioFalso) buscar(id string) (api.CastVideo, bool) {
	for _, l := range [][]api.CastVideo{s.mios, s.org, s.directos, s.comp} {
		for _, x := range l {
			if x.ID == id {
				return x, true
			}
		}
	}
	return api.CastVideo{}, false
}

func (s *servicioFalso) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ruta := r.URL.Path
	// Los ficheros van aparte: el reproductor pide rangos y HEAD.
	if strings.HasPrefix(ruta, "/media/") {
		partes := strings.Split(ruta, "/")
		switch {
		case strings.HasSuffix(ruta, "/poster.jpg"):
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write(posterDe(partes[2]))
		case strings.HasSuffix(ruta, ".webm"):
			s.mu.Lock()
			if len(s.medios) == 0 || s.medios[len(s.medios)-1] != ruta {
				s.medios = append(s.medios, ruta)
			}
			s.mu.Unlock()
			datos, _ := os.ReadFile(fixtureWebM)
			http.ServeContent(w, r, "720p.webm", time.Time{}, bytes.NewReader(datos))
		default:
			http.NotFound(w, r)
		}
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var cuerpo []byte
	if r.Method == http.MethodPut && strings.HasSuffix(ruta, "/original") {
		n, _ := io.Copy(io.Discard, r.Body)
		s.subido = n
		cuerpo = []byte(fmt.Sprintf("<%d bytes %s>", n, r.Header.Get("Content-Type")))
	} else {
		cuerpo, _ = io.ReadAll(r.Body)
	}
	s.pedido = append(s.pedido, strings.TrimSpace(r.Method+" "+ruta+"?"+r.URL.RawQuery+" "+string(cuerpo)))
	escribir := func(v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	if s.falla != 0 {
		w.WriteHeader(s.falla)
		escribir(map[string]any{"error": map[string]string{"code": "x", "message": ""}})
		return
	}
	q := r.URL.Query()
	id := ""
	if p := strings.Split(strings.TrimPrefix(ruta, "/api/v1/"), "/"); len(p) > 1 {
		id = p[1]
		if p[0] == "public" && len(p) > 2 {
			id = p[2]
		}
	}
	switch {
	case strings.HasPrefix(ruta, "/api/v1/moderation/") && s.noAdministra:
		w.WriteHeader(http.StatusForbidden)
		escribir(map[string]any{"error": map[string]string{"code": "forbidden"}})
	case ruta == "/api/v1/public/reports/altcha":
		// SHA-256("sal" + "7"): la solución es 7.
		h := sha256.Sum256([]byte("sal7"))
		escribir(map[string]any{"algorithm": "SHA-256", "challenge": hex.EncodeToString(h[:]), "maxnumber": 50, "salt": "sal", "signature": "firma"})
	case r.Method == http.MethodPost && (strings.HasSuffix(ruta, "/reports") || strings.HasSuffix(ruta, "/blocks") || strings.HasPrefix(ruta, "/api/v1/moderation/reports/")):
		w.WriteHeader(http.StatusNoContent)
	case ruta == "/api/v1/blocks":
		escribir(map[string]any{"blocks": s.bloqueos})
	case ruta == "/api/v1/moderation/blocks":
		escribir(map[string]any{"blocks": s.bloqueosOrg})
	case ruta == "/api/v1/moderation/summary":
		escribir(map[string]int{"open": len(s.denuncias)})
	case ruta == "/api/v1/moderation/reports" && q.Get("status") == "resolved":
		escribir(map[string]any{"reports": []api.CastReport{}})
	case ruta == "/api/v1/moderation/reports":
		escribir(map[string]any{"reports": s.denuncias})
	case ruta == "/api/v1/videos:estimate":
		// Como domain.Estimar sin duración: un original de 8 Mbit/s, las tres permitidas.
		tam, _ := strconv.ParseInt(q.Get("size_bytes"), 10, 64)
		ms := tam * 8 / 8000
		escribir(api.CastEstimate{
			Options: []api.CastQualityOption{
				{Quality: 1080, Allowed: true, Recommended: true, Bytes: 8224 * ms / 8},
				{Quality: 720, Allowed: true, Recommended: true, Bytes: 4524 * ms / 8},
				{Quality: 360, Allowed: true, Recommended: true, Bytes: 1524 * ms / 8},
			},
			Recommended: []int{1080, 720, 360}, OriginalBytes: tam, DurationMS: ms, Approximate: true,
		})
	case strings.HasSuffix(ruta, "/qualities"):
		if r.Method == http.MethodPut {
			var in struct{ Qualities []int }
			_ = json.Unmarshal(cuerpo, &in)
			s.calidades = in.Qualities
		}
		elegidas := s.calidades
		if elegidas == nil {
			elegidas = []int{1080, 720, 360}
		}
		ms := int64(1_380_000) // 23 min
		escribir(api.CastVideoQualities{
			CastEstimate: api.CastEstimate{
				Options: []api.CastQualityOption{
					{Quality: 1080, Allowed: true, Recommended: true, Bytes: 8224 * ms / 8},
					{Quality: 720, Allowed: true, Recommended: true, Bytes: 4524 * ms / 8},
					{Quality: 360, Allowed: true, Recommended: true, Bytes: 1524 * ms / 8},
				},
				Recommended: []int{1080, 720, 360}, OriginalBytes: 1_900 << 20, DurationMS: ms,
			},
			Qualities: elegidas, HLSQualities: []int{1080, 720, 360}, WebMQualities: []int{720},
			Pending: r.Method == http.MethodPut,
		})
	case ruta == "/api/v1/videos:from-drive" && r.Method == http.MethodGet && q.Get("file_id") == idDriveNoVideo:
		w.WriteHeader(http.StatusUnsupportedMediaType)
		escribir(map[string]any{"error": map[string]string{"code": "not_video", "message": "no es un vídeo"}})
	case ruta == "/api/v1/videos:from-drive" && r.Method == http.MethodGet:
		escribir(api.CastDriveFile{FileID: q.Get("file_id"), Name: "Demo de producto.mp4", SizeBytes: 734 << 20, ContentType: "video/mp4", Title: "Demo de producto"})
	case ruta == "/api/v1/videos:from-drive":
		var n api.CastFromDrive
		_ = json.Unmarshal(cuerpo, &n)
		v := videoDePrueba("v01", n.Title, "Ana Pérez", 0, 0, 0)
		v.Status = api.CastStatusUploading
		w.WriteHeader(http.StatusAccepted)
		escribir(v)
	case r.Method == http.MethodGet && ruta == "/api/v1/videos":
		switch {
		case q.Get("status") == api.CastStatusLive:
			escribir(api.CastPage{Videos: s.directos})
		case q.Get("scope") == api.CastMine:
			escribir(api.CastPage{Videos: s.mios})
		default:
			escribir(api.CastPage{Videos: append(append([]api.CastVideo{}, s.directos...), s.org...)})
		}
	case ruta == "/api/v1/videos/shared":
		escribir(api.CastPage{Videos: s.comp})
	case ruta == "/api/v1/public/explore":
		escribir(api.CastExplore{Live: s.directos, Videos: append(append([]api.CastVideo{}, s.org...), s.mios[:2]...), NextCursor: "e2"})
	case strings.HasPrefix(ruta, "/api/v1/public/c/"):
		escribir(api.CastChannel{Org: "acme", Name: "Acme Ingeniería", Videos: s.org})
	case r.Method == http.MethodPost && ruta == "/api/v1/videos":
		var n api.CastNew
		_ = json.Unmarshal(cuerpo, &n)
		v := videoDePrueba("v99", n.Title, "Ana Pérez", 0, 0, 0)
		v.Status, v.Visibility = api.CastStatusUploading, n.Visibility
		s.mios = append([]api.CastVideo{v}, s.mios...)
		w.WriteHeader(http.StatusCreated)
		escribir(v)
	case r.Method == http.MethodPut && strings.HasSuffix(ruta, "/original") && s.sinSitio:
		w.WriteHeader(http.StatusInsufficientStorage)
		escribir(map[string]any{"error": map[string]any{"code": "quota_exceeded", "details": map[string]int64{"available_bytes": 2048}}})
	case r.Method == http.MethodPut && strings.HasSuffix(ruta, "/original"):
		v, _ := s.buscar(id)
		v.Status = api.CastStatusProcessing
		escribir(v)
	case strings.HasSuffix(ruta, "/play"):
		v, _ := s.buscar(id)
		p := api.CastPlay{Kind: v.Kind, WebMURL: "/media/" + id + "/tok/webm/720p.webm", PosterURL: v.PosterURL, DownloadURL: "/media/" + id + "/tok/original"}
		if s.escalera {
			p.WebMQualities = []int{1080, 720, 360}
		}
		if v.EnDirecto() {
			p = api.CastPlay{Kind: "live", Live: &api.CastLiveConn{ServerURL: "wss://sfu.test", Token: "jwt", Room: "cast-acme-" + id, Identity: "u1"}}
		}
		escribir(p)
	case strings.HasSuffix(ruta, "/comments") && r.Method == http.MethodGet:
		escribir(map[string]any{"comments": s.comentarios})
	case strings.HasSuffix(ruta, "/comments") && r.Method == http.MethodPost:
		var in struct{ Body, ReplyTo string }
		_ = json.Unmarshal(cuerpo, &in)
		k := api.CastComment{ID: fmt.Sprintf("k%d", len(s.comentarios)+1), Author: api.CastAuthor{Name: "Ana Pérez"}, Body: in.Body, CreatedAt: ahoraFijo}
		k.Can.Delete = true
		w.WriteHeader(http.StatusCreated)
		escribir(k)
	case strings.HasSuffix(ruta, "/chat"):
		escribir(map[string]any{"messages": []api.CastChatMessage{}})
	case strings.HasSuffix(ruta, "/people") && r.Method == http.MethodGet:
		escribir(map[string]any{"people": s.personas})
	case strings.HasSuffix(ruta, "/people") && r.Method == http.MethodPost:
		p := api.CastPerson{UserID: "u9", Name: "Bea Martín", Username: "bea"}
		s.personas = append(s.personas, p)
		w.WriteHeader(http.StatusCreated)
		escribir(p)
	case ruta == "/api/v1/quota":
		escribir(api.CastQuota{LimitBytes: 200 << 30, UsedBytes: 57 << 30, AvailableBytes: 143 << 30, CastBytes: 12 << 30})
	case ruta == "/api/v1/studio/stream-key" || ruta == "/api/v1/studio/stream-key:rotate":
		if strings.HasSuffix(ruta, ":rotate") {
			s.clave++
		}
		escribir(api.CastStreamKey{RTMPURL: "rtmp://cast-ingest.sytue.eu/live", StreamKey: fmt.Sprintf("sk_live_7Qf2mX9kR4tB8wNz%d", s.clave), IngressID: "IN_1"})
	case ruta == "/api/v1/lives":
		var n api.CastNew
		_ = json.Unmarshal(cuerpo, &n)
		v := directoDePrueba()
		v.ID, v.Title, v.Status, v.Visibility, v.Live.Source = "v50", n.Title, api.CastStatusScheduled, n.Visibility, n.Source
		out := map[string]any{"video": v}
		if n.Source == "webrtc" {
			out["publish"] = api.CastLiveConn{ServerURL: "wss://sfu.test", Token: "pub", Room: "cast-acme-v50", Identity: "emisor-u1"}
		}
		w.WriteHeader(http.StatusCreated)
		escribir(out)
	case strings.HasSuffix(ruta, "/poster:frame"):
		v, _ := s.buscar(id)
		w.WriteHeader(http.StatusAccepted)
		escribir(v)
	case strings.HasSuffix(ruta, "/publish"):
		escribir(api.CastLiveConn{ServerURL: "wss://sfu.test", Token: "pub-" + id, Room: "cast-acme-" + id, Identity: "emisor-u1"})
	case strings.HasSuffix(ruta, "/end"):
		v := directoDePrueba()
		v.Status = api.CastStatusEnded
		escribir(v)
	case r.Method == http.MethodPatch:
		v, _ := s.buscar(id)
		var c api.CastChanges
		_ = json.Unmarshal(cuerpo, &c)
		if c.Title != nil {
			v.Title = *c.Title
		}
		if c.Visibility != nil {
			v.Visibility = *c.Visibility
		}
		if c.CommentsEnabled != nil {
			v.CommentsEnabled = *c.CommentsEnabled
		}
		escribir(v)
	case r.Method == http.MethodDelete:
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodGet:
		v, ok := s.buscar(id)
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			escribir(map[string]any{"error": map[string]string{"code": "not_found"}})
			return
		}
		escribir(v)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// ---- La app de prueba ----

var errSinMedia = errors.New("cast: sin reproductor en este test")

func vistaDePrueba(t *testing.T, v tema.Variante) Vista {
	t.Helper()
	tr, err := Traductor("es")
	if err != nil {
		t.Fatal(err)
	}
	return Vista{T: tr, Ventana: ventanaAncho, Variante: v, Ahora: ahoraFijo}
}

// appDePrueba monta Cast contra el servicio falso, con sesión, en una ventana de
// pruebas, con las llamadas en línea (sin goroutines que lean el estado a la vez).
func appDePrueba(t *testing.T, s *servicioFalso, sesion bool) (*App, fyne.Window) {
	t.Helper()
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	w := test.NewWindow(nil)
	w.SetPadded(false)
	t.Cleanup(w.Close)
	w.Resize(fyne.NewSize(ventanaAncho, ventanaAlto))
	v := vistaDePrueba(t, tema.Claro)
	teca := photos.NuevaFototeca()
	v.Teca = teca
	a := Nueva(api.NewCast(srv.URL, nil, nil), v, w)
	teca.Pedir = a.PedirPoster
	a.lanzar = func(f func()) { f() }
	a.sinSondeo = true
	a.ConSesion = func() bool { return sesion }
	a.WebDe = func(ruta string) string { return "https://cast.sytue.eu/cast" + ruta }
	// El reproductor de verdad tiene sus propias rutinas: aquí no se abre (lo prueba
	// TestVerReproduceElWebM, que vacía la cola de la interfaz a mano).
	a.AbrirMedia = func(context.Context, string) (Fuente, error) { return nil, errSinMedia }
	w.SetContent(a.Objeto())
	t.Cleanup(func() { fyne.DoAndWait(a.Cerrar) })
	return a, w
}

// guardar deja la captura en SYTUE_CAPTURA para poder MIRARLA.
func guardar(t *testing.T, nombre string, img image.Image) {
	t.Helper()
	dir := os.Getenv("SYTUE_CAPTURA")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	if err := pngEncode(&b, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, nombre+".png"), b.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

// idDriveNoVideo es el fichero de Drive que el servicio falso rechaza por no ser vídeo.
const idDriveNoVideo = "00000000-0000-4000-8000-000000000415"
