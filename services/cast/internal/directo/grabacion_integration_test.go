// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package directo

import (
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/livekit"
	"git.sytue.eu/sytue/one/services/cast/internal/domain"
	"git.sytue.eu/sytue/one/services/cast/internal/service"
)

func emitirGrabando(t *testing.T, b *banco) domain.Video {
	t.Helper()
	v, _, err := b.s.Programar(b.ctx, ana, Programa{Title: "Grabación", Source: domain.LiveSourceWebRTC})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.s.arrancar(b.ctx, v); err != nil {
		t.Fatal(err)
	}
	return b.video(t, v.Tenant, v.ID)
}

func resultadoGrabado(v domain.Video) livekit.EgressInfo {
	return livekit.EgressInfo{
		EgressID: v.LiveEgressID, RoomName: v.LiveRoom, Status: "EGRESS_COMPLETE",
		SegmentResults: []livekit.EgressSegments{{PlaylistName: prefijoGrabacion(v) + "/index.m3u8", SegmentCount: 1}},
	}
}

func finGrabado(v domain.Video) livekit.WebhookEgress {
	return livekit.WebhookEgress{
		EgressID: v.LiveEgressID, RoomName: v.LiveRoom, Status: "EGRESS_COMPLETE",
		SegmentResults: []livekit.WebhookEgressSegments{{PlaylistName: prefijoGrabacion(v) + "/index.m3u8", SegmentCount: 1}},
	}
}

func comprobarVODEnCola(t *testing.T, b *banco, v domain.Video) {
	t.Helper()
	actual := b.video(t, v.Tenant, v.ID)
	jobs, err := b.st.TrabajosDelVideo(b.ctx, v.Tenant, v.ID)
	if err != nil || len(jobs) != 1 || jobs[0].Kind != domain.JobLiveVOD || actual.Status != domain.StatusProcessing || actual.HlsKey != claveGrabacion(v) {
		t.Fatalf("la grabación no está encolada una sola vez: vídeo=%+v trabajos=%+v err=%v", actual, jobs, err)
	}
	// Ni un webhook ni ListEgress pueden afirmar que los objetos existen: ready
	// solo lo dará el transcoder tras descargar y procesar la playlist real.
	if actual.WebmKey != "" || actual.PosterKey != "" {
		t.Error("se publicaron derivadas que el transcoder todavía no ha generado")
	}
}

func TestElSondeoRecuperaLaGrabacionSinWebhookFinal(t *testing.T) {
	b := montar(t)
	v := emitirGrabando(t, b)
	if err := b.s.terminar(b.ctx, v); err != nil {
		t.Fatal(err)
	}
	if actual := b.video(t, v.Tenant, v.ID); actual.Status != domain.StatusEnded || actual.HlsKey != "" {
		t.Fatalf("antes de recibir el resultado debe seguir ended sin grabación: %+v", actual)
	}
	b.falso.mu.Lock()
	b.falso.resultados = map[string]livekit.EgressInfo{v.LiveEgressID: resultadoGrabado(v)}
	b.falso.mu.Unlock()
	if err := b.s.Sondear(b.ctx); err != nil {
		t.Fatal(err)
	}
	comprobarVODEnCola(t, b, v)
	if err := b.s.Sondear(b.ctx); err != nil {
		t.Fatal(err)
	}
	comprobarVODEnCola(t, b, v)
	if n := b.falso.cuenta("ListEgress"); n != 1 {
		t.Errorf("una grabación ya encolada se volvió a consultar: %d", n)
	}
}

func TestElSondeoEncolaElHLSYaAnotadoAntesDeUnaParada(t *testing.T) {
	b := montar(t)
	v := emitirGrabando(t, b)
	// La réplica se detuvo después de las escrituras, antes de encolar. La room
	// y el egress pueden haber caducado: no hacen falta para retomar ese HLS.
	if _, err := b.s.svc.CambiarEstado(b.ctx, v.Tenant, v.ID, service.Estado{A: domain.StatusEnded}); err != nil {
		t.Fatal(err)
	}
	if estado, err := b.s.propio.fijarGrabacion(b.ctx, v.Tenant, v.ID, claveGrabacion(v)); err != nil || estado != domain.StatusEnded {
		t.Fatalf("anotar la grabación debe devolver el estado de su misma escritura: %s %v", estado, err)
	}
	if err := b.s.Sondear(b.ctx); err != nil {
		t.Fatal(err)
	}
	comprobarVODEnCola(t, b, v)
	if n := b.falso.cuenta("ListEgress"); n != 0 {
		t.Errorf("un HLS ya anotado sigue dependiendo del historial de egress: %d", n)
	}
}

func TestElSondeoNoPublicaNiReintentaUnEgressTerminalSinGrabacion(t *testing.T) {
	for _, estado := range []string{"EGRESS_FAILED", "EGRESS_ABORTED", "EGRESS_LIMIT_REACHED", "EGRESS_COMPLETE", "not_found", "vacio"} {
		t.Run(estado, func(t *testing.T) {
			b := montar(t)
			v := emitirGrabando(t, b)
			if err := b.s.terminar(b.ctx, v); err != nil {
				t.Fatal(err)
			}
			b.falso.mu.Lock()
			if estado == "not_found" {
				b.falso.codigoConsulta = estado
			} else if estado != "vacio" {
				b.falso.resultados = map[string]livekit.EgressInfo{v.LiveEgressID: {
					EgressID: v.LiveEgressID, RoomName: v.LiveRoom, Status: estado, Error: "destino no disponible",
				}}
			}
			b.falso.mu.Unlock()
			for range 2 {
				if err := b.s.Sondear(b.ctx); err != nil {
					t.Fatal(err)
				}
			}
			actual := b.video(t, v.Tenant, v.ID)
			jobs, err := b.st.TrabajosDelVideo(b.ctx, v.Tenant, v.ID)
			if err != nil || len(jobs) != 0 || actual.Status != domain.StatusEnded || actual.HlsKey != "" || actual.Failure == "" {
				t.Fatalf("un egress sin objetos inventó un VOD o perdió el motivo: %+v jobs=%+v err=%v", actual, jobs, err)
			}
			if n := b.falso.cuenta("ListEgress"); n != 1 {
				t.Errorf("el fallo terminal se sigue consultando: %d", n)
			}
		})
	}
}

func TestElSondeoReintentaUnaRutaInvalidaSinConfundirlaConEgressCaducado(t *testing.T) {
	b := montar(t)
	v := emitirGrabando(t, b)
	if err := b.s.terminar(b.ctx, v); err != nil {
		t.Fatal(err)
	}
	b.falso.mu.Lock()
	b.falso.codigoConsulta = "bad_route"
	b.falso.mu.Unlock()
	if err := b.s.Sondear(b.ctx); err != nil {
		t.Fatal(err)
	}
	if actual := b.video(t, v.Tenant, v.ID); actual.Failure != "" || actual.Status != domain.StatusEnded {
		t.Fatalf("un 404 de ruta descartó la grabación: %+v", actual)
	}
	b.falso.mu.Lock()
	b.falso.codigoConsulta = ""
	b.falso.resultados = map[string]livekit.EgressInfo{v.LiveEgressID: resultadoGrabado(v)}
	b.falso.mu.Unlock()
	if err := b.s.Sondear(b.ctx); err != nil {
		t.Fatal(err)
	}
	comprobarVODEnCola(t, b, v)
}

func TestLaGrabacionSeEncolaConCualquieraDeLosDosOrdenesDeCierre(t *testing.T) {
	for _, primero := range []string{"directo", "grabacion"} {
		t.Run(primero, func(t *testing.T) {
			b := montar(t)
			v := emitirGrabando(t, b)
			fin := finGrabado(v)
			grabar := func() error { return b.s.grabacionTerminada(b.ctx, v.Tenant, fin) }
			terminar := func() error { return b.s.terminar(b.ctx, v) }
			antes, despues := terminar, grabar
			if primero == "grabacion" {
				antes, despues = grabar, terminar
			}
			if err := antes(); err != nil {
				t.Fatal(err)
			}
			// terminar conserva el snapshot live/sin HLS obtenido ANTES del primer
			// cierre: reproduce la lectura atrasada que antes dejaba ended sin job.
			if err := despues(); err != nil {
				t.Fatal(err)
			}
			comprobarVODEnCola(t, b, v)
		})
	}
}

func TestElFinConcurrenteDelDirectoYDelEgressEncolaUnaVez(t *testing.T) {
	b := montar(t)
	for range 4 {
		v := emitirGrabando(t, b)
		inicio := make(chan struct{})
		errores := make(chan error, 2)
		go func() { <-inicio; errores <- b.s.terminar(b.ctx, v) }()
		go func() { <-inicio; errores <- b.s.grabacionTerminada(b.ctx, v.Tenant, finGrabado(v)) }()
		close(inicio)
		for range 2 {
			if err := <-errores; err != nil {
				t.Fatal(err)
			}
		}
		comprobarVODEnCola(t, b, v)
	}
}
