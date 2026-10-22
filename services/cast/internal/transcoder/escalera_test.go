// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package transcoder

import (
	"errors"
	"slices"
	"testing"
)

func TestEscaleraPara(t *testing.T) {
	casos := []struct {
		nombre      string
		ancho, alto int
		quiere      []string
	}{
		{"4K da los tres", 3840, 2160, []string{"1080p", "720p", "360p"}},
		{"1080p da los tres", 1920, 1080, []string{"1080p", "720p", "360p"}},
		{"720p no sube a 1080", 1280, 720, []string{"720p", "360p"}},
		{"480p solo da 360", 854, 480, []string{"360p"}},
		{"vertical de móvil, por el lado corto", 1080, 1920, []string{"1080p", "720p", "360p"}},
		{"vertical 720×1280", 720, 1280, []string{"720p", "360p"}},
		{"un 1080p con un píxel menos no llega", 1918, 1079, []string{"720p", "360p"}},
		{"más pequeño que el menor: un peldaño a su tamaño", 426, 240, []string{"240p"}},
		{"impar se redondea a par", 320, 181, []string{"180p"}},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			esc, err := escaleraPara(c.ancho, c.alto, nil)
			if err != nil {
				t.Fatal(err)
			}
			if got := nombres(esc); !slices.Equal(got, c.quiere) {
				t.Fatalf("%d×%d → %v, se esperaba %v", c.ancho, c.alto, got, c.quiere)
			}
			for _, p := range esc {
				if p.Lado > min(c.ancho, c.alto) {
					t.Errorf("el peldaño %s (%d) escala hacia arriba desde %d×%d", p.Nombre, p.Lado, c.ancho, c.alto)
				}
				if p.Tasa == "" || p.Maxima == "" || p.Bufer == "" {
					t.Errorf("el peldaño %s va sin tasas", p.Nombre)
				}
			}
		})
	}
}

func TestEscaleraSinDimensiones(t *testing.T) {
	if _, err := escaleraPara(0, 720, nil); !errors.Is(err, ErrSinVideo) {
		t.Fatalf("sin ancho: %v", err)
	}
	if _, err := escaleraPara(1280, 0, nil); !errors.Is(err, ErrSinVideo) {
		t.Fatalf("sin alto: %v", err)
	}
}

func TestPeldanosSonLosDelDueno(t *testing.T) {
	// Decisión 2 y §7: 1080p 5 Mbps, 720p 2,5, 360p 0,8 en H.264; el tope y el
	// búfer como siempre (×1,07 y ×1,5).
	quiere := map[string][3]string{
		"1080p": {"5000k", "5350k", "7500k"},
		"720p":  {"2500k", "2675k", "3750k"},
		"360p":  {"800k", "856k", "1200k"},
	}
	esc, _ := escaleraPara(1920, 1080, nil)
	for _, c := range esc {
		if q := quiere[c.Nombre]; [3]string{c.Tasa, c.Maxima, c.Bufer} != q {
			t.Errorf("%s a %s/%s/%s; el dueño pidió %v", c.Nombre, c.Tasa, c.Maxima, c.Bufer, q)
		}
		if c.TasaWebM == "" {
			t.Errorf("%s sin tasa de VP9", c.Nombre)
		}
	}
}

func TestEscaleraConEleccion(t *testing.T) {
	casos := []struct {
		nombre      string
		ancho, alto int
		elegidas    []int
		quiere      []string
	}{
		{"solo 720 de un 1080p", 1920, 1080, []int{720}, []string{"720p"}},
		{"1080 y 360", 1920, 1080, []int{1080, 360}, []string{"1080p", "360p"}},
		{"1080 de un 720p: la mayor que da", 1280, 720, []int{1080}, []string{"720p"}},
		{"1080 y 360 de un 720p: se recorta 1080", 1280, 720, []int{1080, 360}, []string{"360p"}},
	}
	for _, c := range casos {
		esc, err := escaleraPara(c.ancho, c.alto, c.elegidas)
		if err != nil {
			t.Fatal(err)
		}
		if got := nombres(esc); !slices.Equal(got, c.quiere) {
			t.Errorf("%s: %v, se esperaba %v", c.nombre, got, c.quiere)
		}
	}
}

func TestFiltroEscala(t *testing.T) {
	if f := filtroEscala(720, false); f != "scale=-2:720" {
		t.Errorf("horizontal fija el alto: %s", f)
	}
	if f := filtroEscala(720, true); f != "scale=720:-2" {
		t.Errorf("vertical fija el ancho: %s", f)
	}
}
