// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package transcoder

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/cast/internal/config"
	"git.sytue.eu/sytue/one/services/cast/internal/domain"
)

func configDe(tmp string) config.Config {
	return config.Config{TranscoderTmpDir: tmp, TranscoderThreads: 2, TranscoderMaxMinutes: 5}
}

func videoDePrueba(kind domain.Kind) domain.Video {
	return domain.Video{ID: uuid.New(), Tenant: "acme", OwnerSub: "ana", Kind: kind, Status: domain.StatusProcessing, Title: "Prueba"}
}

// El bucle sin ffmpeg: lo que se decide ANTES de llamarlo se prueba con los
// dobles. Lo que pasa después (la conversión de verdad) está en el test de
// integración, que necesita ffmpeg y se salta si no lo hay.

func TestUnaVueltaSinTrabajosNiConBaseCaida(t *testing.T) {
	ctx := context.Background()
	cola := nuevaColaFalsa()
	w := trabajadorDePrueba(cola, nuevoAlmacenFalso(), t.TempDir())
	w.cfg = configDe(t.TempDir())

	hubo, err := w.unaVuelta(ctx)
	if err != nil || hubo {
		t.Fatalf("cola vacía: hubo=%v err=%v", hubo, err)
	}
	cola.errReclam = errBaseCaida
	if _, err := w.unaVuelta(ctx); !errors.Is(err, errBaseCaida) {
		t.Fatalf("la base caída sube tal cual para que el bucle espere: %v", err)
	}
}

func TestAtenderFallaConMotivoLegible(t *testing.T) {
	ctx := context.Background()
	casos := []struct {
		nombre string
		video  domain.Video
		kind   domain.JobKind
		carga  string
		motivo string
	}{
		{"subida sin original", videoDePrueba(domain.KindUpload), domain.JobUpload, "", ErrSinEntrada.Error()},
		{"grabación sin hls", videoDePrueba(domain.KindLive), domain.JobLiveVOD, "", ErrSinEntrada.Error()},
		{"póster sin nada", videoDePrueba(domain.KindUpload), domain.JobPoster, `{"at_ms": 100}`, ErrSinEntrada.Error()},
		{"póster con payload roto", videoDePrueba(domain.KindUpload), domain.JobPoster, `{"at_ms": `, "payload del póster ilegible"},
		{"clase desconocida", videoDePrueba(domain.KindUpload), domain.JobKind("dance"), "", "clase de trabajo desconocida"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			cola := nuevaColaFalsa()
			w := trabajadorDePrueba(cola, nuevoAlmacenFalso(), t.TempDir())
			w.cfg = configDe(t.TempDir())
			j := cola.encolar(c.video, c.kind, c.carga)
			hubo, err := w.unaVuelta(ctx)
			if err != nil || !hubo {
				t.Fatalf("hubo=%v err=%v", hubo, err)
			}
			motivo, fallo := cola.fallos[j.ID]
			if !fallo || !strings.Contains(motivo, c.motivo) {
				t.Fatalf("motivo %q (fallado: %v), se esperaba que contuviera %q", motivo, fallo, c.motivo)
			}
			if _, ok := cola.terminados[j.ID]; ok {
				t.Fatal("un trabajo fallido no se termina")
			}
		})
	}
}

func TestAtenderConOriginalQueNoEstaEnElAlmacen(t *testing.T) {
	ctx := context.Background()
	cola := nuevaColaFalsa()
	w := trabajadorDePrueba(cola, nuevoAlmacenFalso(), t.TempDir())
	w.cfg = configDe(t.TempDir())
	v := videoDePrueba(domain.KindUpload)
	v.OriginalKey = v.ID.String() + "/original"
	j := cola.encolar(v, domain.JobUpload, "")
	if _, err := w.unaVuelta(ctx); err != nil {
		t.Fatal(err)
	}
	if m := cola.fallos[j.ID]; !strings.Contains(m, "bajar") || !strings.Contains(m, "original") {
		t.Fatalf("el motivo dice qué no se pudo bajar: %q", m)
	}
}

func TestAtenderSinVideoNoTermina(t *testing.T) {
	// El vídeo se borró entre encolar y reclamar: el trabajo falla con ErrNoExiste y
	// la store decide (aquí, la falsa lo anota).
	ctx := context.Background()
	cola := nuevaColaFalsa()
	w := trabajadorDePrueba(cola, nuevoAlmacenFalso(), t.TempDir())
	w.cfg = configDe(t.TempDir())
	j := cola.encolar(videoDePrueba(domain.KindUpload), domain.JobUpload, "")
	delete(cola.videos, j.VideoID)
	if _, err := w.unaVuelta(ctx); err != nil {
		t.Fatal(err)
	}
	if m := cola.fallos[j.ID]; m != domain.ErrNoExiste.Error() {
		t.Fatalf("motivo: %q", m)
	}
}

func TestBucleSeApagaConElContexto(t *testing.T) {
	cola := nuevaColaFalsa()
	w := trabajadorDePrueba(cola, nuevoAlmacenFalso(), t.TempDir())
	w.cfg = configDe(t.TempDir())
	ctx, cancelar := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancelar()
	inicio := time.Now()
	err := w.bucle(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("el bucle devuelve el error del contexto: %v", err)
	}
	if time.Since(inicio) > 2*time.Second {
		t.Fatalf("el bucle tardó %v en apagarse: la espera no escucha el contexto", time.Since(inicio))
	}
	if err := ComprobarLatido(w.latido, time.Minute, time.Now()); err != nil {
		t.Fatalf("el bucle late aunque no haya trabajos: %v", err)
	}
}

func TestRunExigeDependenciasYFfmpeg(t *testing.T) {
	if err := Run(context.Background(), Deps{}); !errors.Is(err, ErrSinDependencias) {
		t.Fatalf("sin store ni almacén: %v", err)
	}
	// Sin ffmpeg en el PATH, `nuevo` lo dice a la primera (el pod se cae a la vista).
	t.Setenv("PATH", t.TempDir())
	if _, err := nuevo(Deps{Cfg: configDe(t.TempDir())}, nuevaColaFalsa(), nuevoAlmacenFalso()); !errors.Is(err, ErrSinFfmpeg) {
		t.Fatalf("sin ffmpeg: %v", err)
	}
}

func TestPosterSalvoPropio(t *testing.T) {
	v := videoDePrueba(domain.KindUpload)
	if k := posterSalvoPropio(v); k == nil || *k != v.ID.String()+"/poster.jpg" {
		t.Fatalf("sin póster previo se fija el generado: %v", k)
	}
	v.PosterKey = v.ID.String() + "/poster.jpg"
	if k := posterSalvoPropio(v); k == nil {
		t.Fatal("el generado anterior se sustituye")
	}
	v.PosterKey = v.ID.String() + "/poster-propio"
	if k := posterSalvoPropio(v); k != nil {
		t.Fatalf("el póster del dueño se respeta: %v", *k)
	}
}

func TestEstadoDe(t *testing.T) {
	e := estadoDe(Sonda{Duracion: 2500 * time.Millisecond, Ancho: 1280, Alto: 720})
	if e.DurationMS == nil || *e.DurationMS != 2500 || e.Width == nil || *e.Width != 1280 || e.Height == nil || *e.Height != 720 {
		t.Fatalf("%+v", e)
	}
	if e := estadoDe(Sonda{}); e.DurationMS != nil || e.Width != nil {
		t.Fatal("sin medidas no se anota nada (COALESCE conserva lo que hubiera)")
	}
}
