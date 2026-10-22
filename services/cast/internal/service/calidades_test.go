// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/cast/internal/domain"
	"git.sytue.eu/sytue/one/services/cast/internal/media"
)

func videoConWebM(webm []int32) domain.Video {
	w, h := int32(1920), int32(1080)
	id := uuid.New()
	v := domain.Video{
		ID: id, Kind: domain.KindUpload, Status: domain.StatusReady, Width: &w, Height: &h,
		HlsKey: id.String() + "/hls/master.m3u8", WebmKey: id.String() + "/webm/720p.webm", WebmQualities: webm,
	}
	return v
}

func TestMediaSirveCadaCalidadWebM(t *testing.T) {
	v := videoConWebM([]int32{1080, 720, 360})
	for _, q := range []int{1080, 720, 360} {
		clave, err := claveDe(v, media.RutaWebMDe(q))
		if err != nil || clave != Prefijo(v)+media.RutaWebMDe(q) {
			t.Errorf("%dp: %q %v", q, clave, err)
		}
	}
	// Lo que no existe, o no es una ruta de calidad limpia, no abre nada.
	for _, ruta := range []string{"webm/480p.webm", "webm/../original", "webm/0720p.webm", "webm/720.webm", "webm/-1p.webm", "webm/"} {
		if _, err := claveDe(v, ruta); !errors.Is(err, domain.ErrNoExiste) {
			t.Errorf("%s: se esperaba no existe, fue %v", ruta, err)
		}
	}
	// Quitada una calidad, su ruta deja de servirse aunque el fichero siguiera ahí.
	v.WebmQualities = []int32{720}
	if _, err := claveDe(v, "webm/1080p.webm"); !errors.Is(err, domain.ErrNoExiste) {
		t.Errorf("1080 quitada: %v", err)
	}
}

func TestMediaVideoAntiguoSoloTiene720(t *testing.T) {
	// Un vídeo de antes de la escalera: webm_qualities NULL y su único WebM en
	// la ruta vieja, que sigue valiendo.
	v := videoConWebM(nil)
	clave, err := claveDe(v, media.RutaWebM)
	if err != nil || clave != v.WebmKey {
		t.Fatalf("la ruta vieja: %q %v", clave, err)
	}
	if _, err := claveDe(v, "webm/360p.webm"); !errors.Is(err, domain.ErrNoExiste) {
		t.Fatalf("un vídeo antiguo no tiene 360p: %v", err)
	}
}

func TestPlayDaLasCalidadesYLaDePorDefecto(t *testing.T) {
	s := &Servicio{firmante: media.New("clave")}
	v := videoConWebM([]int32{1080, 720, 360})
	p := s.play(v, Permisos{Ver: true})
	if !slices.Equal(p.WebMCalidades, []int{1080, 720, 360}) || !strings.HasSuffix(p.WebMURL, "/webm/720p.webm") {
		t.Fatalf("play: %v %q", p.WebMCalidades, p.WebMURL)
	}
	// Sin 720: la mayor por debajo.
	v.WebmQualities = []int32{1080, 360}
	if p := s.play(v, Permisos{Ver: true}); !strings.HasSuffix(p.WebMURL, "/webm/360p.webm") {
		t.Fatalf("sin 720: %q", p.WebMURL)
	}
	// Antiguo: [720] y la ruta de siempre.
	v.WebmQualities = nil
	if p := s.play(v, Permisos{Ver: true}); !slices.Equal(p.WebMCalidades, []int{720}) || !strings.HasSuffix(p.WebMURL, "/"+media.RutaWebM) {
		t.Fatalf("antiguo: %v %q", p.WebMCalidades, p.WebMURL)
	}
	// Sin WebM (un directo en curso): ni URL ni calidades, pero lista y no nil.
	v.WebmKey = ""
	if p := s.play(v, Permisos{Ver: true}); p.WebMURL != "" || p.WebMCalidades == nil || len(p.WebMCalidades) != 0 {
		t.Fatalf("sin WebM: %v %q", p.WebMCalidades, p.WebMURL)
	}
}

func TestPideTrabajoDeCalidades(t *testing.T) {
	v := videoConWebM([]int32{1080, 720, 360})
	v.HlsQualities = []int32{1080, 720, 360}
	if PideTrabajoDeCalidades(v) {
		t.Fatal("todo generado: nada que hacer")
	}
	v.Qualities = []int32{720}
	if !PideTrabajoDeCalidades(v) {
		t.Fatal("elegir menos: hay que borrar")
	}
	v.Status = domain.StatusProcessing
	if PideTrabajoDeCalidades(v) {
		t.Fatal("mientras se procesa, lo hará el trabajo de la subida")
	}
	// Un vídeo antiguo pide la escalera WebM completa.
	antiguo := videoConWebM(nil)
	if !PideTrabajoDeCalidades(antiguo) {
		t.Fatal("un vídeo de antes pide sus WebM de 1080 y 360")
	}
}
