// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"errors"
	"math"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func fotoDe(id string, cuando time.Time, exif bool, lugar *api.PhotoPlace, huella string) fotoLocal {
	return fotoLocal{vista: api.Photo{ID: id, TakenAt: cuando, TakenFromEXIF: exif, Location: lugar}, huella: huella}
}

func dia(a, m, d int) time.Time { return time.Date(a, time.Month(m), d, 10, 0, 0, 0, time.UTC) }

// Mes y día, solo fecha de captura, solo años anteriores, por año de más reciente a más
// antiguo y con doce como mucho por año: las reglas de `Recuerdos` del servicio.
func TestRecuerdosComoElServicio(t *testing.T) {
	hoy := time.Date(2026, 11, 23, 9, 0, 0, 0, time.Local)
	fotos := make([]fotoLocal, 0, 18)
	for i := range 14 {
		fotos = append(fotos, fotoDe("2024-"+string(rune('a'+i)), dia(2024, 11, 23), true, nil, ""))
	}
	fotos = append(fotos,
		fotoDe("hoy", dia(2026, 11, 23), true, nil, ""),      // este año: no es un recuerdo
		fotoDe("subida", dia(2025, 11, 23), false, nil, ""),  // por fecha de subida: tampoco
		fotoDe("otro-dia", dia(2023, 11, 22), true, nil, ""), // otro día
		fotoDe("2019", dia(2019, 11, 23), true, nil, ""),
	)
	got := recuerdosDe(fotos, hoy)
	if len(got) != 2 || got[0].Year != 2024 || got[1].Year != 2019 {
		t.Fatalf("años %+v", got)
	}
	if len(got[0].Items) != MaxPorRecuerdo || got[1].Items[0].ID != "2019" {
		t.Fatalf("fotos de cada año: %d, %+v", len(got[0].Items), got[1].Items)
	}
}

func TestAnyosDeLaLinea(t *testing.T) {
	fotos := []fotoLocal{fotoDe("a", dia(2026, 1, 1), true, nil, ""), fotoDe("b", dia(2026, 3, 1), true, nil, ""), fotoDe("c", dia(2019, 7, 1), false, nil, "")}
	got := anyosDe(fotos)
	if len(got) != 2 || got[0] != (api.TimelineYear{Year: 2026, Count: 2}) || got[1] != (api.TimelineYear{Year: 2019, Count: 1}) {
		t.Fatalf("%+v", got)
	}
}

// Las repetidas son las del mismo original; sin huella no se comparan.
func TestRepetidasPorHuella(t *testing.T) {
	fotos := []fotoLocal{
		fotoDe("a1", dia(2026, 1, 3), true, nil, "aa"), fotoDe("b1", dia(2026, 1, 2), true, nil, "bb"),
		fotoDe("a2", dia(2025, 1, 1), true, nil, "aa"), fotoDe("x", dia(2025, 1, 1), true, nil, ""),
		fotoDe("y", dia(2025, 1, 1), true, nil, ""),
	}
	fotos[0].asset.UploadedAt = dia(2026, 2, 1)
	fotos[2].asset.UploadedAt = dia(2026, 1, 1)
	got := repetidasDe(fotos, 50)
	if len(got) != 1 || len(got[0]) != 2 || got[0][0].ID != "a2" || got[0][1].ID != "a1" {
		t.Fatalf("%+v", got)
	}
}

// «Por aquí»: dentro de dos kilómetros, de la más cerca a la más lejos y sin ella; una
// foto sin GPS da una lista vacía y una que no existe, ErrNotFound.
func TestCercaDeDosKilometros(t *testing.T) {
	sol := &api.PhotoPlace{Lat: 40.4169, Lon: -3.7035}
	fotos := []fotoLocal{
		fotoDe("sol", dia(2026, 1, 1), true, sol, ""),
		fotoDe("retiro", dia(2026, 1, 1), true, &api.PhotoPlace{Lat: 40.4153, Lon: -3.6845}, ""), // 1,6 km
		fotoDe("mayor", dia(2026, 1, 1), true, &api.PhotoPlace{Lat: 40.4155, Lon: -3.7074}, ""),  // 0,4 km
		fotoDe("barajas", dia(2026, 1, 1), true, &api.PhotoPlace{Lat: 40.4983, Lon: -3.5676}, ""),
		fotoDe("sin-gps", dia(2026, 1, 1), true, nil, ""),
	}
	got, err := cercaDe(fotos, "sol", 48)
	if err != nil || len(got) != 2 || got[0].ID != "mayor" || got[1].ID != "retiro" {
		t.Fatalf("%+v %v", got, err)
	}
	if got, err := cercaDe(fotos, "sin-gps", 48); err != nil || len(got) != 0 {
		t.Fatal("sin GPS", got, err)
	}
	if _, err := cercaDe(fotos, "otra", 48); !errors.Is(err, ErrNotFound) {
		t.Fatal("una que no está", err)
	}
	if km := DistanciaKm(40.4169, -3.7035, 41.3874, 2.1686); math.Abs(km-505) > 3 {
		t.Fatalf("Madrid-Barcelona %.1f km", km)
	}
}

func TestLimiteDeFecha(t *testing.T) {
	for desde, want := range map[string]time.Time{
		"2019":       time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		"2019-12":    time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		"2019-07-14": time.Date(2019, 7, 15, 0, 0, 0, 0, time.UTC),
	} {
		if got, err := limiteDeFecha(desde); err != nil || !got.Equal(want) {
			t.Errorf("%s: %v %v", desde, got, err)
		}
	}
	for _, mal := range []string{"", "19", "2019-13", "ayer"} {
		if _, err := limiteDeFecha(mal); err == nil {
			t.Errorf("%q se acepta", mal)
		}
	}
}

func TestNombreDeFichero(t *testing.T) {
	for in, want := range map[string]string{"playa.jpg": "playa.jpg", "../../etc/passwd": "passwd", `C:\fotos\a:b.png`: "a_b.png", "": "id", "..": "id"} {
		if got := nombreDeFichero(in, "id"); got != want {
			t.Errorf("%q → %q, se esperaba %q", in, got, want)
		}
	}
}
