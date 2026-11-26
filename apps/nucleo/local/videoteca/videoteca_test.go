// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package videoteca

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/local/conversor"
)

func opciones(t *testing.T) Opciones {
	t.Helper()
	raiz := t.TempDir()
	return Opciones{
		Carpeta: filepath.Join(raiz, "Vídeos"), Datos: filepath.Join(raiz, "Datos", "cast"), Cache: filepath.Join(raiz, "cache"),
		Autor: "Ana",
		// Sin ffmpeg: se prueba lo que no depende de él.
		Herramientas: func() (conversor.Herramientas, error) { return conversor.Herramientas{}, conversor.ErrSinFFmpeg },
		Sondear: func(_ context.Context, ruta string) (conversor.Info, error) {
			if strings.HasSuffix(ruta, ".webm") {
				return conversor.Info{Contenedor: "matroska,webm", Video: "vp9", Audio: "opus", Duracion: 3 * time.Second, Ancho: 640, Alto: 360}, nil
			}
			return conversor.Info{Contenedor: "mov,mp4", Video: "h264", Audio: "aac", Duracion: 2 * time.Second}, nil
		},
	}
}

func escribir(t *testing.T, ruta, contenido string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(ruta), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ruta, []byte(contenido), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRecogeLaCarpetaYApunta(t *testing.T) {
	o := opciones(t)
	escribir(t, filepath.Join(o.Carpeta, "mi_viaje.webm"), "webm")
	escribir(t, filepath.Join(o.Carpeta, "notas.txt"), "no es vídeo")
	escribir(t, filepath.Join(o.Carpeta, ".oculto", "x.mp4"), "oculto")
	v, err := Abrir(o)
	if err != nil {
		t.Fatal(err)
	}
	l := v.Lista()
	if len(l) != 1 || l[0].Titulo != "mi viaje" || !l[0].Copiado {
		t.Fatalf("recogida: %+v", l)
	}
	// Un fichero de fuera se apunta sin copiarlo, y dos veces es el mismo.
	fuera := filepath.Join(t.TempDir(), "peli.mp4")
	escribir(t, fuera, "mp4")
	e, err := v.Apuntar(context.Background(), fuera)
	if err != nil {
		t.Fatal(err)
	}
	if e.Copiado || e.Ruta != fuera || e.Nativo || e.DuracionMS != 2000 {
		t.Errorf("apuntado: %+v", e)
	}
	otra, _ := v.Apuntar(context.Background(), fuera)
	if otra.ID != e.ID {
		t.Error("apuntar dos veces da dos vídeos")
	}
	// Se reabre igual desde el índice.
	v2, err := Abrir(o)
	if err != nil || len(v2.Lista()) != 2 {
		t.Fatalf("reabrir: %v %d", err, len(v2.Lista()))
	}
}

func TestImportarCopiaYBorrarSoloBorraCopias(t *testing.T) {
	o := opciones(t)
	v, err := Abrir(o)
	if err != nil {
		t.Fatal(err)
	}
	fuera := filepath.Join(t.TempDir(), "clip.webm")
	escribir(t, fuera, "contenido")
	e, err := v.Importar(context.Background(), fuera)
	if err != nil {
		t.Fatal(err)
	}
	if !e.Copiado || filepath.Dir(e.Ruta) != limpia(o.Carpeta) {
		t.Fatalf("importado: %+v", e)
	}
	// Un segundo con el mismo nombre no pisa el primero.
	otro := filepath.Join(t.TempDir(), "clip.webm")
	escribir(t, otro, "otro")
	e2, err := v.Importar(context.Background(), otro)
	if err != nil || e2.Ruta == e.Ruta || filepath.Base(e2.Ruta) != "clip (2).webm" {
		t.Fatalf("segundo: %+v %v", e2, err)
	}
	if err := v.Borrar(e.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(e.Ruta); !os.IsNotExist(err) {
		t.Error("la copia no se borró")
	}
	if _, err := os.Stat(fuera); err != nil {
		t.Error("el original de fuera no se toca")
	}
	// Uno apuntado de fuera no se borra nunca del disco.
	ap, _ := v.Apuntar(context.Background(), fuera)
	if err := v.Borrar(ap.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(fuera); err != nil {
		t.Error("borrar un apuntado borró el fichero de la persona")
	}
}

func TestPublicarYPortada(t *testing.T) {
	o := opciones(t)
	escribir(t, filepath.Join(o.Carpeta, "a.webm"), "a")
	escribir(t, filepath.Join(o.Carpeta, "b.webm"), "b")
	v, _ := Abrir(o)
	ctx := context.Background()
	l := v.Lista()
	si := true
	nuevo := "Título nuevo"
	if _, err := v.Actualizar(l[0].ID, Cambios{Publicado: &si, Titulo: &nuevo}); err != nil {
		t.Fatal(err)
	}
	vacio := " "
	if _, err := v.Actualizar(l[0].ID, Cambios{Titulo: &vacio}); err == nil {
		t.Error("un título vacío no vale")
	}
	ex, _ := v.Explore(ctx, "", 10)
	if len(ex.Videos) != 1 || ex.Videos[0].Title != nuevo || ex.Videos[0].Visibility != api.CastPublic || ex.Videos[0].PublishedAt == nil {
		t.Fatalf("portada: %+v", ex)
	}
	mios, _ := v.Videos(ctx, api.CastMine, "", "", "", 10)
	org, _ := v.Videos(ctx, api.CastOrgScope, "", "", "", 10)
	if len(mios.Videos) != 2 || len(org.Videos) != 0 {
		t.Errorf("mios %d, org %d", len(mios.Videos), len(org.Videos))
	}
	busca, _ := v.Videos(ctx, api.CastMine, "", "nuevo", "", 10)
	if len(busca.Videos) != 1 {
		t.Errorf("búsqueda: %d", len(busca.Videos))
	}
	// Con la regla de apps/nucleo/buscar: «TITULO» encuentra «Título nuevo».
	if sin, _ := v.Videos(ctx, api.CastMine, "", "TITULO", "", 10); len(sin.Videos) != 1 {
		t.Errorf("búsqueda sin acentos: %d", len(sin.Videos))
	}
	pag, _ := v.Videos(ctx, api.CastMine, "", "", "", 1)
	if len(pag.Videos) != 1 || pag.NextCursor != "1" {
		t.Errorf("página: %+v", pag)
	}
	canal, _ := v.Channel(ctx, "local", "")
	if len(canal.Videos) != 1 || canal.Name != "Ana" {
		t.Errorf("canal: %+v", canal)
	}
}

func TestNativoSeVeYSinFFmpegFalla(t *testing.T) {
	o := opciones(t)
	escribir(t, filepath.Join(o.Carpeta, "bien.webm"), "webm de verdad no, pero se lee")
	escribir(t, filepath.Join(o.Carpeta, "otro.mp4"), "mp4")
	v, _ := Abrir(o)
	ctx := context.Background()
	var webm, mp4 Entrada
	for _, e := range v.Lista() {
		if strings.HasSuffix(e.Ruta, ".webm") {
			webm = e
		} else {
			mp4 = e
		}
	}
	vid, err := v.Video(ctx, webm.ID)
	if err != nil || vid.Status != api.CastStatusReady {
		t.Fatalf("webm: %+v %v", vid, err)
	}
	play, err := v.Play(ctx, webm.ID)
	if err != nil || play.WebMURL == "" {
		t.Fatalf("play: %+v %v", play, err)
	}
	m, err := v.OpenMedia(ctx, play.WebMURL)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(m)
	_ = m.Close()
	if string(b) != "webm de verdad no, pero se lee" || m.Size() != int64(len(b)) {
		t.Errorf("media: %q", b)
	}
	vid, _ = v.Video(ctx, mp4.ID)
	if vid.Status != api.CastStatusFailed || !strings.Contains(vid.Failure, "ffmpeg") {
		t.Errorf("mp4 sin ffmpeg: %+v", vid)
	}
	if _, err := v.Play(ctx, mp4.ID); !errors.Is(err, ErrNecesitaFFmpeg) {
		t.Errorf("play mp4: %v", err)
	}
	if _, err := v.OpenMedia(ctx, "https://ejemplo/x"); err == nil {
		t.Error("solo se abren rutas locales")
	}
	if _, err := v.Video(ctx, "nada"); err == nil {
		t.Error("un id que no está")
	}
	// Si el fichero desaparece, se dice.
	_ = os.Remove(webm.Ruta)
	if vid, _ = v.Video(ctx, webm.ID); vid.Status != api.CastStatusFailed {
		t.Errorf("sin fichero: %+v", vid)
	}
}

func TestPosterPropio(t *testing.T) {
	o := opciones(t)
	escribir(t, filepath.Join(o.Carpeta, "a.webm"), "a")
	v, _ := Abrir(o)
	id := v.Lista()[0].ID
	if _, err := v.PonerPoster(id, []byte("gif"), "gif"); err == nil {
		t.Error("GIF no vale de póster")
	}
	e, err := v.PonerPoster(id, []byte{0xFF, 0xD8, 0xFF}, ".jpg")
	if err != nil {
		t.Fatal(err)
	}
	vid, _ := v.Video(context.Background(), id)
	m, err := v.OpenMedia(context.Background(), vid.PosterURL)
	if err != nil || m.Size() != 3 || e.PosterN != 1 {
		t.Fatalf("póster: %v %+v", err, e)
	}
	_ = m.Close()
}

// Con el ffmpeg de verdad: un MP4 se convierte al abrirlo y acaba listo para verse.
func TestIntegracionConvierteAlAbrir(t *testing.T) {
	h, err := conversor.Buscar("")
	if err != nil || h.FFprobe == "" {
		t.Skip("ffmpeg/ffprobe no están instalados")
	}
	cods, _ := h.Codificadores(context.Background())
	if !cods["libvpx-vp9"] || !cods["libopus"] || !cods["libx264"] || !cods["aac"] {
		t.Skip("este ffmpeg no trae VP9/Opus/H.264/AAC")
	}
	dir := t.TempDir()
	clip := filepath.Join(dir, "clip.mp4")
	cmd := exec.Command(h.FFmpeg, "-hide_banner", "-nostdin", "-y", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc=duration=1:size=320x240:rate=10", "-f", "lavfi", "-i", "sine=frequency=440:duration=1",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-shortest", clip)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	o := opciones(t)
	o.Herramientas = func() (conversor.Herramientas, error) { return h, nil }
	o.Sondear = nil
	cambios := make(chan string, 64)
	o.AlCambiar = func(id string) { cambios <- id }
	v, err := Abrir(o)
	if err != nil {
		t.Fatal(err)
	}
	e, err := v.Apuntar(context.Background(), clip)
	if err != nil {
		t.Fatal(err)
	}
	if e.Nativo || e.Ancho != 320 || e.DuracionMS < 900 {
		t.Fatalf("medido: %+v", e)
	}
	vid, _ := v.Video(context.Background(), e.ID)
	if vid.Status != api.CastStatusProcessing {
		t.Fatalf("al abrirlo se prepara: %+v", vid)
	}
	limite := time.After(2 * time.Minute)
	for v.EstadoDe(e.ID).Convirtiendo {
		select {
		case <-cambios:
		case <-limite:
			t.Fatal("la conversión no acaba")
		case <-time.After(200 * time.Millisecond):
		}
	}
	est := v.EstadoDe(e.ID)
	if !est.Listo || est.Fallo != nil {
		t.Fatalf("estado: %+v", est)
	}
	info, err := h.Sondear(context.Background(), est.Ruta)
	if err != nil || !info.Nativo() {
		t.Errorf("el visionado no es WebM VP9/Opus: %+v %v", info, err)
	}
	if vid, _ = v.Video(context.Background(), e.ID); vid.Status != api.CastStatusReady {
		t.Errorf("tras convertir: %+v", vid)
	}
	// La fachada de Cast, sin red, la acepta y la sirve.
	c, err := api.NewPrivateCast(v)
	if err != nil {
		t.Fatal(err)
	}
	play, err := c.Play(context.Background(), e.ID)
	if err != nil {
		t.Fatal(err)
	}
	m, err := c.Media(context.Background(), play.WebMURL)
	if err != nil || m.Size() <= 0 {
		t.Fatalf("media por la fachada: %v", err)
	}
	_ = m.Close()
	if _, err := c.Explore(context.Background(), "", 10); err != nil {
		t.Errorf("portada local por la fachada: %v", err)
	}
	if _, err := c.Quota(context.Background()); !errors.Is(err, api.ErrPrivateCastOperation) {
		t.Errorf("lo del servidor sigue cerrado y sin red: %v", err)
	}
}
