// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// El flujo entero contra un PostgreSQL real (platform/pkg/pgtest, la store de
// Cast de verdad) y un almacén en memoria, con ffmpeg de verdad: un vídeo de 2 s
// generado con `-f lavfi` entra por la cola como `upload`, sale `ready` con HLS,
// WebM y póster; después un `poster` a un instante y un `live_vod` desde un HLS
// de segmentos TS como el que deja el egress.
//
// SI NO HAY ffmpeg EN EL PATH, EL TEST SE SALTA Y LO DICE. En el contenedor
// sytue-agentes: `apt-get install -y --no-install-recommends ffmpeg`. Para
// correrlo: PGTEST_DSN=… go test -tags integration -p 1 -run TestTranscodificaUnVideoDePuntaAPunta ./services/cast/internal/transcoder/
package transcoder

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cast/db"
	"git.sytue.eu/sytue/one/services/cast/internal/domain"
	"git.sytue.eu/sytue/one/services/cast/internal/service"
	"git.sytue.eu/sytue/one/services/cast/internal/store"
)

// ffmpegDePruebas genera la entrada de prueba, o salta el test si no hay ffmpeg.
func ffmpegDePruebas(t *testing.T) string {
	t.Helper()
	ff, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg no está en el PATH: el test de integración del transcodificador SE SALTA (instala ffmpeg para correrlo: apt-get install -y --no-install-recommends ffmpeg)")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe no está en el PATH: el test de integración del transcodificador SE SALTA")
	}
	return ff
}

func correr(t *testing.T, bin string, args ...string) {
	t.Helper()
	var stderr bytes.Buffer
	cmd := exec.Command(bin, args...)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("%s %s: %v\n%s", filepath.Base(bin), strings.Join(args, " "), err, stderr.String())
	}
}

// videoDeDosSegundos escribe un MP4 H.264/AAC de 2 s a 1280×720.
func videoDeDosSegundos(t *testing.T, ff, ruta string) []byte {
	t.Helper()
	correr(t, ff, "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=1280x720:rate=30",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000",
		"-t", "2", "-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-c:a", "aac", "-b:a", "96k",
		"-movflags", "+faststart", ruta)
	b, err := os.ReadFile(ruta)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestTranscodificaUnVideoDePuntaAPunta(t *testing.T) {
	ff := ffmpegDePruebas(t)
	ctx, cancelar := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancelar()

	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	alm := nuevoAlmacenFalso()
	acme := tenancy.Tenant{Slug: "acme"}
	tmp := t.TempDir()
	w, err := nuevo(Deps{Cfg: configDe(filepath.Join(tmp, "trabajo")), Logger: slog.New(slog.NewTextHandler(os.Stderr, nil))}, st, alm)
	if err != nil {
		t.Fatal(err)
	}

	original := videoDeDosSegundos(t, ff, filepath.Join(tmp, "original.mp4"))

	// ── upload ────────────────────────────────────────────────────────────
	v, err := st.Crear(ctx, store.Nuevo{Tenant: "acme", OwnerSub: "ana", OwnerName: "Ana", Kind: domain.KindUpload, Visibility: domain.VisibilityOrg, Title: "Prueba"})
	if err != nil {
		t.Fatal(err)
	}
	claveOriginal := v.ID.String() + "/original"
	alm.poner(acme, claveOriginal, "video/mp4", original)
	tam := int64(len(original))
	if _, err := st.CambiarEstado(ctx, "acme", v.ID, store.Estado{A: domain.StatusProcessing, OriginalKey: &claveOriginal, BytesOriginal: &tam, BytesTotal: &tam}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Encolar(ctx, "acme", v.ID, domain.JobUpload, nil); err != nil {
		t.Fatal(err)
	}

	hubo, err := w.unaVuelta(ctx)
	if err != nil || !hubo {
		t.Fatalf("unaVuelta: hubo=%v err=%v", hubo, err)
	}
	listo, err := st.EnOrganizacion(ctx, "acme", v.ID)
	if err != nil {
		t.Fatal(err)
	}
	trabajos, _ := st.TrabajosDelVideo(ctx, "acme", v.ID)
	if listo.Status != domain.StatusReady {
		t.Fatalf("el vídeo no quedó ready: %s (%q); trabajos: %+v", listo.Status, listo.Failure, trabajos)
	}
	pref := v.ID.String() + "/"
	if listo.HlsKey != pref+"hls/master.m3u8" || listo.WebmKey != pref+"webm/720p.webm" || listo.PosterKey != pref+"poster.jpg" {
		t.Fatalf("claves: hls=%q webm=%q poster=%q", listo.HlsKey, listo.WebmKey, listo.PosterKey)
	}
	if listo.Width == nil || *listo.Width != 1280 || listo.Height == nil || *listo.Height != 720 {
		t.Fatalf("dimensiones: %v×%v", listo.Width, listo.Height)
	}
	if listo.DurationMS == nil || *listo.DurationMS < 1800 || *listo.DurationMS > 2300 {
		t.Fatalf("duración: %v", listo.DurationMS)
	}
	if listo.BytesTotal <= tam {
		t.Fatalf("bytes_total %d tendría que sumar las derivadas a los %d del original", listo.BytesTotal, tam)
	}
	master, tipo, ok := alm.leer(acme, listo.HlsKey)
	if !ok || tipo != "application/vnd.apple.mpegurl" {
		t.Fatalf("master: %v %q", ok, tipo)
	}
	// 720p de entrada: 720p y 360p, nunca 1080p; las variantes relativas al master.
	if !strings.Contains(string(master), "720p/index.m3u8") || !strings.Contains(string(master), "360p/index.m3u8") || strings.Contains(string(master), "1080p") {
		t.Fatalf("master:\n%s", master)
	}
	variante, _, ok := alm.leer(acme, pref+"hls/720p/index.m3u8")
	if !ok || !strings.Contains(string(variante), "#EXT-X-MAP:URI=\"init") || !strings.Contains(string(variante), ".m4s") || !strings.Contains(string(variante), "#EXT-X-ENDLIST") {
		t.Fatalf("variante 720p:\n%s", variante)
	}
	// Los segmentos y el init existen bajo el directorio de la variante, como /media los resuelve.
	var segmentos, inits int
	for _, k := range alm.claves() {
		switch {
		case strings.HasPrefix(k, acme.StoragePrefix()+pref+"hls/720p/") && strings.HasSuffix(k, ".m4s"):
			segmentos++
		case strings.HasPrefix(k, acme.StoragePrefix()+pref+"hls/") && strings.Contains(k, "init"):
			inits++
		}
	}
	if segmentos == 0 || inits != 2 {
		t.Fatalf("segmentos 720p %d, inits %d; claves: %v", segmentos, inits, alm.claves())
	}
	if webm, tipo, ok := alm.leer(acme, listo.WebmKey); !ok || tipo != "video/webm" || len(webm) < 1000 || !bytes.HasPrefix(webm, []byte{0x1a, 0x45, 0xdf, 0xa3}) {
		t.Fatalf("webm: %v %q %d bytes", ok, tipo, len(webm))
	}
	// La escalera WebM tiene las MISMAS calidades que el HLS: 720p y 360p.
	if webm, _, ok := alm.leer(acme, pref+"webm/360p.webm"); !ok || !bytes.HasPrefix(webm, []byte{0x1a, 0x45, 0xdf, 0xa3}) {
		t.Fatal("falta el WebM de 360p")
	}
	if _, _, ok := alm.leer(acme, pref+"webm/1080p.webm"); ok {
		t.Fatal("un original de 720p no da un WebM de 1080p")
	}
	if !slices.Equal(listo.HlsQualities, []int32{720, 360}) || !slices.Equal(listo.WebmQualities, []int32{720, 360}) {
		t.Fatalf("calidades anotadas: hls %v webm %v", listo.HlsQualities, listo.WebmQualities)
	}
	if jpg, tipo, ok := alm.leer(acme, listo.PosterKey); !ok || tipo != "image/jpeg" || !bytes.HasPrefix(jpg, []byte{0xff, 0xd8}) {
		t.Fatalf("poster.jpg: %v %q", ok, tipo)
	}
	if webp, tipo, ok := alm.leer(acme, pref+"poster.webp"); !ok || tipo != "image/webp" || !bytes.HasPrefix(webp, []byte("RIFF")) {
		t.Fatalf("poster.webp: %v %q", ok, tipo)
	}
	if hubo, err := w.unaVuelta(ctx); err != nil || hubo {
		t.Fatalf("la cola tenía que estar vacía: hubo=%v err=%v", hubo, err)
	}

	// ── quitar una calidad: se borra y se libera ─────────────────────────
	solo720, err := st.ElegirCalidades(ctx, "acme", v.ID, []int32{720}, service.PideTrabajoDeCalidades)
	if err != nil {
		t.Fatal(err)
	}
	if !service.PideTrabajoDeCalidades(solo720) {
		t.Fatal("elegir menos de lo que hay pide un trabajo")
	}
	if hubo, err := w.unaVuelta(ctx); err != nil || !hubo {
		t.Fatalf("calidades (quitar): hubo=%v err=%v", hubo, err)
	}
	sinTrescientos, _ := st.EnOrganizacion(ctx, "acme", v.ID)
	if !slices.Equal(sinTrescientos.HlsQualities, []int32{720}) || !slices.Equal(sinTrescientos.WebmQualities, []int32{720}) {
		trabajos, _ := st.TrabajosDelVideo(ctx, "acme", v.ID)
		t.Fatalf("tras quitar 360: hls %v webm %v; %+v", sinTrescientos.HlsQualities, sinTrescientos.WebmQualities, trabajos)
	}
	if sinTrescientos.BytesTotal >= listo.BytesTotal {
		t.Fatalf("quitar una calidad libera espacio: %d → %d", listo.BytesTotal, sinTrescientos.BytesTotal)
	}
	if _, _, ok := alm.leer(acme, pref+"webm/360p.webm"); ok {
		t.Fatal("el WebM de 360p tenía que borrarse")
	}
	for _, k := range alm.claves() {
		if strings.HasPrefix(k, acme.StoragePrefix()+pref+"hls/360p/") {
			t.Fatalf("quedó %s", k)
		}
	}
	if m, _, _ := alm.leer(acme, listo.HlsKey); strings.Contains(string(m), "360p") || !strings.Contains(string(m), "720p/index.m3u8") {
		t.Fatalf("master tras quitar 360:\n%s", m)
	}

	// ── volver a añadirla: se genera con ffmpeg y el master la anuncia ────
	if _, err := st.ElegirCalidades(ctx, "acme", v.ID, []int32{720, 360}, service.PideTrabajoDeCalidades); err != nil {
		t.Fatal(err)
	}
	if hubo, err := w.unaVuelta(ctx); err != nil || !hubo {
		t.Fatalf("calidades (añadir): hubo=%v err=%v", hubo, err)
	}
	conTrescientos, _ := st.EnOrganizacion(ctx, "acme", v.ID)
	if !slices.Equal(conTrescientos.HlsQualities, []int32{720, 360}) || !slices.Equal(conTrescientos.WebmQualities, []int32{720, 360}) {
		t.Fatalf("tras añadir 360: hls %v webm %v", conTrescientos.HlsQualities, conTrescientos.WebmQualities)
	}
	m, _, _ := alm.leer(acme, listo.HlsKey)
	if i, j := bytes.Index(m, []byte("720p/index.m3u8")), bytes.Index(m, []byte("360p/index.m3u8")); i < 0 || j < i {
		t.Fatalf("master tras añadir 360:\n%s", m)
	}
	if _, _, ok := alm.leer(acme, pref+"hls/360p/index.m3u8"); !ok {
		t.Fatal("la variante 360p no se generó")
	}
	if conTrescientos.BytesTotal <= sinTrescientos.BytesTotal {
		t.Fatalf("añadir una calidad ocupa: %d → %d", sinTrescientos.BytesTotal, conTrescientos.BytesTotal)
	}

	// ── un vídeo de antes de la escalera: solo webm/720p.webm ────────────
	// Se deja como estaban: sin columnas de calidades y sin el WebM de 360p.
	if err := h.Pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE videos SET hls_qualities = NULL, webm_qualities = NULL, qualities = NULL WHERE id = $1`, v.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := alm.Delete(ctx, acme, pref+"webm/360p.webm"); err != nil {
		t.Fatal(err)
	}
	n, err := st.EncolarCalidadesPendientes(ctx, 100)
	if err != nil || n != 1 {
		t.Fatalf("encolar-calidades: %d %v", n, err)
	}
	if n, _ := st.EncolarCalidadesPendientes(ctx, 100); n != 0 {
		t.Fatalf("encolar dos veces no duplica: %d", n)
	}
	// El barrido rellena la cola mirando cuántos hay: aquí, el recién encolado.
	if n, err := st.CalidadesEnCola(ctx); err != nil || n != 1 {
		t.Fatalf("calidades en cola: %d %v", n, err)
	}
	if hubo, err := w.unaVuelta(ctx); err != nil || !hubo {
		t.Fatalf("calidades (vídeo antiguo): hubo=%v err=%v", hubo, err)
	}
	antiguo, _ := st.EnOrganizacion(ctx, "acme", v.ID)
	if !slices.Equal(antiguo.WebmQualities, []int32{720, 360}) || !slices.Equal(antiguo.HlsQualities, []int32{720, 360}) {
		t.Fatalf("el vídeo antiguo recibe la escalera WebM: hls %v webm %v", antiguo.HlsQualities, antiguo.WebmQualities)
	}
	if _, _, ok := alm.leer(acme, pref+"webm/360p.webm"); !ok {
		t.Fatal("el WebM de 360p del vídeo antiguo no se generó")
	}
	if n, _ := st.CalidadesEnCola(ctx); n != 0 {
		t.Fatalf("con el trabajo hecho quedan %d en cola", n)
	}

	// ── un vídeo antiguo cuyo trabajo de calidades ya falló no vuelve ─────
	// Si no, el barrido lo reencolaría cada hora para siempre.
	if err := h.Pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE videos SET webm_qualities = NULL WHERE id = $1`, v.ID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO transcode_jobs (tenant, video_id, kind, status, error) VALUES ('acme', $1, 'qualities', 'failed', 'de prueba')`, v.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if n, err := st.EncolarCalidadesPendientes(ctx, 100); err != nil || n != 0 {
		t.Fatalf("un vídeo con el trabajo de calidades fallido se reencoló: %d %v", n, err)
	}
	if err := h.Pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE videos SET webm_qualities = '{720,360}' WHERE id = $1`, v.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	// ── poster a un instante ──────────────────────────────────────────────
	posterAntes, _, _ := alm.leer(acme, pref+"poster.jpg")
	if _, err := st.Encolar(ctx, "acme", v.ID, domain.JobPoster, []byte(`{"at_ms": 1500}`)); err != nil {
		t.Fatal(err)
	}
	if hubo, err := w.unaVuelta(ctx); err != nil || !hubo {
		t.Fatalf("poster: hubo=%v err=%v", hubo, err)
	}
	conPoster, _ := st.EnOrganizacion(ctx, "acme", v.ID)
	if conPoster.Status != domain.StatusReady || conPoster.PosterKey != pref+"poster.jpg" {
		t.Fatalf("tras el póster: %s %q %q", conPoster.Status, conPoster.PosterKey, conPoster.Failure)
	}
	posterDespues, _, _ := alm.leer(acme, pref+"poster.jpg")
	if bytes.Equal(posterAntes, posterDespues) {
		t.Fatal("el póster a 1,5 s tendría que ser otro fotograma que el del 10 %")
	}
	if conPoster.BytesTotal != listo.BytesTotal-int64(len(posterAntes))+int64(len(posterDespues)) {
		// Se sobrescribió también el .webp; la cuenta exacta incluye los dos. Basta
		// con que siga por encima del original y cerca del total anterior.
		if conPoster.BytesTotal <= tam || conPoster.BytesTotal > listo.BytesTotal*2 {
			t.Fatalf("bytes_total tras el póster: %d (antes %d)", conPoster.BytesTotal, listo.BytesTotal)
		}
	}

	// ── live_vod desde un HLS de segmentos TS como el del egress ─────────
	dirLive := filepath.Join(tmp, "live")
	if err := os.MkdirAll(dirLive, 0o750); err != nil {
		t.Fatal(err)
	}
	correr(t, ff, "-hide_banner", "-loglevel", "error", "-y", "-i", filepath.Join(tmp, "original.mp4"),
		"-c", "copy", "-f", "hls", "-hls_time", "1", "-hls_list_size", "0",
		"-hls_segment_filename", filepath.Join(dirLive, "cast-acme_%05d.ts"), filepath.Join(dirLive, "index.m3u8"))
	directo, err := st.Crear(ctx, store.Nuevo{Tenant: "acme", OwnerSub: "ana", OwnerName: "Ana", Kind: domain.KindLive, Visibility: domain.VisibilityOrg, Title: "Directo", LiveSource: domain.LiveSourceRTMP, LiveRoom: "cast-acme-x"})
	if err != nil {
		t.Fatal(err)
	}
	prefLive := directo.ID.String() + "/live/"
	var bytesGrabacion int64
	entradas, _ := os.ReadDir(dirLive)
	for _, e := range entradas {
		b, err := os.ReadFile(filepath.Join(dirLive, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		alm.poner(acme, prefLive+e.Name(), service.TipoDe(e.Name()), b)
		bytesGrabacion += int64(len(b))
	}
	hlsKey := prefLive + "index.m3u8"
	if _, err := st.CambiarEstado(ctx, "acme", directo.ID, store.Estado{A: domain.StatusLive}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CambiarEstado(ctx, "acme", directo.ID, store.Estado{A: domain.StatusEnded, HlsKey: &hlsKey}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Encolar(ctx, "acme", directo.ID, domain.JobLiveVOD, nil); err != nil {
		t.Fatal(err)
	}
	if hubo, err := w.unaVuelta(ctx); err != nil || !hubo {
		t.Fatalf("live_vod: hubo=%v err=%v", hubo, err)
	}
	grabado, _ := st.EnOrganizacion(ctx, "acme", directo.ID)
	if grabado.Status != domain.StatusReady {
		trabajos, _ := st.TrabajosDelVideo(ctx, "acme", directo.ID)
		t.Fatalf("la grabación no quedó ready: %s (%q); %+v", grabado.Status, grabado.Failure, trabajos)
	}
	if grabado.Kind != domain.KindLive || grabado.HlsKey != hlsKey {
		t.Fatalf("la grabación sigue siendo un directo con su HLS: %s %q", grabado.Kind, grabado.HlsKey)
	}
	if grabado.WebmKey != directo.ID.String()+"/webm/720p.webm" || grabado.PosterKey != directo.ID.String()+"/poster.jpg" {
		t.Fatalf("derivadas de la grabación: %q %q", grabado.WebmKey, grabado.PosterKey)
	}
	if !slices.Equal(grabado.WebmQualities, []int32{720, 360}) || grabado.HlsQualities != nil {
		t.Fatalf("la grabación: WebM en escalera, su HLS tal cual: webm %v hls %v", grabado.WebmQualities, grabado.HlsQualities)
	}
	if grabado.Width == nil || *grabado.Width != 1280 || grabado.DurationMS == nil || *grabado.DurationMS < 1500 {
		t.Fatalf("medidas de la grabación: %v %v", grabado.Width, grabado.DurationMS)
	}
	if grabado.BytesOriginal != bytesGrabacion || grabado.BytesTotal <= bytesGrabacion {
		t.Fatalf("la grabación cuenta como original (%d) y las derivadas encima (%d)", grabado.BytesOriginal, grabado.BytesTotal)
	}
	if _, _, ok := alm.leer(acme, grabado.WebmKey); !ok {
		t.Fatal("el webm de la grabación no se subió")
	}

	// El temporal del trabajo se recoge.
	restos, _ := os.ReadDir(filepath.Join(tmp, "trabajo"))
	for _, r := range restos {
		if r.IsDir() {
			t.Errorf("quedó un temporal sin borrar: %s", r.Name())
		}
	}
}

// claves lista las claves completas que hay, en el orden del mapa. Vive aquí, con
// la etiqueta `integration`, porque solo la usa este test: en dobles_test.go el
// lint sin etiquetas la veía sin usar.
func (a *almacenFalso) claves() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]string, 0, len(a.objs))
	for k := range a.objs {
		out = append(out, k)
	}
	return out
}

// TestUnaSubidaNoAbreOtrasRutas (FILE-12): un «vídeo» subido que en realidad es
// un playlist HLS no hace que ffprobe abra lo que nombra —el vídeo de otra
// organización en el temporal del trabajador, o una URL interna—.
func TestUnaSubidaNoAbreOtrasRutas(t *testing.T) {
	ff := ffmpegDePruebas(t)
	ctx, cancelar := context.WithTimeout(context.Background(), time.Minute)
	defer cancelar()
	tmp := t.TempDir()
	w, err := nuevo(Deps{Cfg: configDe(filepath.Join(tmp, "trabajo")), Logger: slog.New(slog.NewTextHandler(os.Stderr, nil))}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	ajeno := filepath.Join(tmp, "de-otra-organizacion.ts")
	correr(t, ff, "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=size=320x240:rate=10",
		"-t", "1", "-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-f", "mpegts", ajeno)
	var pedidas atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { pedidas.Add(1) }))
	defer srv.Close()

	for nombre, segmento := range map[string]string{"fichero local": ajeno, "red": srv.URL + "/interno.ts"} {
		subida := filepath.Join(tmp, "original-"+strings.ReplaceAll(nombre, " ", "-"))
		playlist := "#EXTM3U\n#EXT-X-TARGETDURATION:1\n#EXTINF:1,\n" + segmento + "\n#EXT-X-ENDLIST\n"
		if err := os.WriteFile(subida, []byte(playlist), 0o600); err != nil {
			t.Fatal(err)
		}
		if s, err := w.sondear(ctx, entrada{ruta: subida}); err == nil {
			t.Errorf("%s: la subida se abrió como playlist y leyó %q (%dx%d)", nombre, segmento, s.Ancho, s.Alto)
		}
	}
	if pedidas.Load() != 0 {
		t.Errorf("ffprobe pidió %d veces la URL interna", pedidas.Load())
	}
}
