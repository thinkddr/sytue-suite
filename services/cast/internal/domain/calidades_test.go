// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"errors"
	"slices"
	"testing"
)

func TestOpcionesNuncaHaciaArriba(t *testing.T) {
	casos := []struct {
		nombre      string
		ancho, alto int
		quiere      []int
	}{
		{"4K da las tres", 3840, 2160, []int{1080, 720, 360}},
		{"1080p da las tres", 1920, 1080, []int{1080, 720, 360}},
		{"720p no da 1080", 1280, 720, []int{720, 360}},
		{"un píxel menos no llega", 1918, 1079, []int{720, 360}},
		{"vertical, por el lado corto", 1080, 1920, []int{1080, 720, 360}},
		{"480p solo da 360", 854, 480, []int{360}},
		{"más pequeño que 360: una a su tamaño", 426, 240, []int{240}},
		{"impar a par", 320, 181, []int{180}},
		{"sin medir: las tres", 0, 0, []int{1080, 720, 360}},
	}
	for _, c := range casos {
		if got := Opciones(c.ancho, c.alto); !slices.Equal(got, c.quiere) {
			t.Errorf("%s: %d×%d → %v, se esperaba %v", c.nombre, c.ancho, c.alto, got, c.quiere)
		}
	}
}

func TestValidarCalidades(t *testing.T) {
	tres := []int{1080, 720, 360}
	if got, err := ValidarCalidades([]int{360, 1080}, tres); err != nil || !slices.Equal(got, []int{1080, 360}) {
		t.Fatalf("válida y ordenada de mayor a menor: %v %v", got, err)
	}
	for _, c := range []struct {
		nombre   string
		elegidas []int
		opciones []int
	}{
		{"ninguna", nil, tres},
		{"lista vacía", []int{}, tres},
		{"repetida", []int{720, 720}, tres},
		{"1080 de un original de 720", []int{1080, 720}, []int{720, 360}},
		{"una que no es de la tabla", []int{480}, tres},
	} {
		if _, err := ValidarCalidades(c.elegidas, c.opciones); !errors.Is(err, ErrEntradaInvalida) {
			t.Errorf("%s: se esperaba entrada inválida, fue %v", c.nombre, err)
		}
	}
}

func TestResolver(t *testing.T) {
	tres := []int{1080, 720, 360}
	if got := Resolver(nil, tres); !slices.Equal(got, tres) {
		t.Errorf("sin elección, la recomendada (todas): %v", got)
	}
	if got := Resolver([]int{360}, tres); !slices.Equal(got, []int{360}) {
		t.Errorf("lo elegido: %v", got)
	}
	// Eligió 1080 al subir y el original resultó 720: la mayor que permite.
	if got := Resolver([]int{1080}, []int{720, 360}); !slices.Equal(got, []int{720}) {
		t.Errorf("al menos una: %v", got)
	}
	if got := Resolver([]int{1080, 360}, []int{720, 360}); !slices.Equal(got, []int{360}) {
		t.Errorf("lo que no llega se recorta: %v", got)
	}
}

func TestCalidadesDeUnVideoAntiguo(t *testing.T) {
	w, h := int32(1920), int32(1080)
	v := Video{Kind: KindUpload, HlsKey: "x/hls/master.m3u8", WebmKey: "x/webm/720p.webm", Width: &w, Height: &h}
	if got := v.CalidadesHLS(); !slices.Equal(got, []int{1080, 720, 360}) {
		t.Errorf("HLS de antes: lo que permitía el original: %v", got)
	}
	if got := v.CalidadesWebM(); !slices.Equal(got, []int{720}) {
		t.Errorf("WebM de antes: solo 720: %v", got)
	}
	if got := v.Elegidas(); !slices.Equal(got, []int{1080, 720, 360}) {
		t.Errorf("sin elección, todas: %v", got)
	}
	v.Qualities, v.HlsQualities, v.WebmQualities = []int32{720}, []int32{720}, []int32{720}
	if !slices.Equal(v.Elegidas(), []int{720}) || !slices.Equal(v.CalidadesHLS(), []int{720}) || !slices.Equal(v.CalidadesWebM(), []int{720}) {
		t.Errorf("con columnas, las columnas: %v %v %v", v.Elegidas(), v.CalidadesHLS(), v.CalidadesWebM())
	}
	directo := Video{Kind: KindLive, HlsKey: "x/live/index.m3u8", Width: &w, Height: &h}
	if got := directo.CalidadesHLS(); len(got) != 0 {
		t.Errorf("el HLS de una grabación no es de la escalera: %v", got)
	}
	if got := (Video{}).CalidadesWebM(); got == nil || len(got) != 0 {
		t.Errorf("sin WebM, lista vacía (no nil: el cable pinta []): %v", got)
	}
}

func TestWebMPorDefecto(t *testing.T) {
	for _, c := range []struct {
		qs     []int
		quiere int
	}{
		{[]int{1080, 720, 360}, 720},
		{[]int{1080, 360}, 360},
		{[]int{1080}, 1080},
		{[]int{240}, 240},
		{nil, 0},
	} {
		if got := WebMPorDefecto(c.qs); got != c.quiere {
			t.Errorf("%v → %d, se esperaba %d", c.qs, got, c.quiere)
		}
	}
}

func TestEstimar(t *testing.T) {
	// Un minuto a 1080p con la duración medida.
	ms := int64(60_000)
	e := Estimar(500<<20, &ms, 1920, 1080)
	if e.Aproximada || e.DuracionMS != 60_000 {
		t.Fatalf("con duración no es aproximada: %+v", e)
	}
	if !slices.Equal(e.Recomendadas, []int{1080, 720, 360}) {
		t.Fatalf("recomendadas: todas las que permite: %v", e.Recomendadas)
	}
	// 1080p: (5000+128+3000+96) kbit/s × 60 s / 8 = 61,68 MB.
	if e.Opciones[0].Lado != 1080 || e.Opciones[0].Bytes != 61_680_000 {
		t.Fatalf("1080p en un minuto: %+v", e.Opciones[0])
	}
	// 360p: (800+128+500+96) × 60 / 8 = 11,43 MB.
	if e.Opciones[2].Lado != 360 || e.Opciones[2].Bytes != 11_430_000 {
		t.Fatalf("360p en un minuto: %+v", e.Opciones[2])
	}
	for i := 1; i < len(e.Opciones); i++ {
		if e.Opciones[i].Bytes >= e.Opciones[i-1].Bytes {
			t.Errorf("una calidad menor no puede ocupar más: %+v", e.Opciones)
		}
	}

	// Un 720p: 1080 aparece, pero no permitida ni recomendada, y sin bytes.
	e = Estimar(0, &ms, 1280, 720)
	if o := e.Opciones[0]; o.Lado != 1080 || o.Permitida || o.Recomendada || o.Bytes != 0 {
		t.Fatalf("1080 de un 720p: %+v", o)
	}
	if !slices.Equal(e.Recomendadas, []int{720, 360}) {
		t.Fatalf("recomendadas de un 720p: %v", e.Recomendadas)
	}

	// Sin duración: se deduce del tamaño con la tasa típica y lo dice.
	e = Estimar(60_000_000, nil, 0, 0)
	if !e.Aproximada || e.DuracionMS != 60_000 {
		t.Fatalf("60 MB a 8 Mbit/s son 60 s, aproximados: %+v", e)
	}

	// Un original de 240p: las tres de la tabla no, la suya sí.
	e = Estimar(1<<20, &ms, 426, 240)
	if len(e.Opciones) != 4 || e.Opciones[3].Lado != 240 || !e.Opciones[3].Permitida || e.Opciones[0].Permitida {
		t.Fatalf("240p: %+v", e.Opciones)
	}
}
