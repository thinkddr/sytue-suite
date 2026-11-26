// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/portabilidad"
	"git.sytue.eu/sytue/one/services/syphotos/internal/domain"
)

// tusDatosDeOffice es la copia de ejemplo con que prueba «Traer desde Sytue» de Sytue
// Office (`apps/nucleo/local/traer`): los originales en `drive/` y las tablas de Fotos.
var tusDatosDeOffice = filepath.Join("..", "..", "..", "..", "apps", "nucleo", "local", "traer", "testdata", "tusdatos")

// La entrega de Fotos (photos y albums) es lo que lee «Traer desde Sytue» de Sytue
// Office para llevar las fotos a su Photos. Sus pruebas leen estas tablas, escritas
// aquí con el MISMO exportador en los tres formatos que puede elegir la persona. Si la
// entrega cambia, esta prueba falla hasta que se vuelven a escribir
// (SYTUE_REGENERAR=1) y las pruebas de Sytue Office dicen si las sigue leyendo.
func TestLaEntregaDeFotosLaLeeSytueOffice(t *testing.T) {
	tam := func(rel string) int64 {
		fi, err := os.Stat(filepath.Join(tusDatosDeOffice, "drive", filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		return fi.Size()
	}
	en := func(s string) *time.Time {
		v, err := time.Parse(time.RFC3339, s)
		if err != nil {
			t.Fatal(err)
		}
		return &v
	}
	lat, lon := 36.5298, -6.2924
	subida := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	id := func(n byte) uuid.UUID { return uuid.UUID{0x5f, 0x07, 0x05, 15: n} }
	fotos := []fotoExportada{
		fotoDe(domain.Activo{
			ID: id(1), DriveID: "d-playa", Nombre: "playa.jpg", Tipo: "image/jpeg", Bytes: tam("Fotos/playa.jpg"),
			Ancho: 96, Alto: 64, TomadaEn: en("2023-08-15T19:45:00+02:00"), SubidaEn: subida, Lat: &lat, Lon: &lon,
			Favorita: true, Descripcion: "Atardecer en la playa", Etiquetas: []string{"playa", "verano"},
		}),
		// Una descripción que parece una fórmula: el CSV la neutraliza.
		fotoDe(domain.Activo{
			ID: id(2), DriveID: "d-gato", Nombre: "gato.png", Tipo: "image/png", Bytes: tam("Fotos/gato.png"),
			Ancho: 48, Alto: 48, SubidaEn: subida, Descripcion: "=SUMA(1) gatos", Etiquetas: []string{},
		}),
		fotoDe(domain.Activo{
			ID: id(3), DriveID: "d-nieve", Nombre: "nieve.jpg", Tipo: "image/jpeg", Bytes: tam("Viajes/nieve.jpg"),
			Ancho: 64, Alto: 96, TomadaEn: en("2022-01-10T10:00:00Z"), SubidaEn: subida, Etiquetas: []string{"nieve"}, Camara: "Cámara de prueba",
		}),
		fotoDe(domain.Activo{
			ID: id(4), DriveID: "d-clip", Nombre: "clip.mp4", Tipo: "video/mp4", Bytes: tam("Fotos/clip.mp4"),
			Ancho: 1280, Alto: 720, SubidaEn: subida,
		}),
		// Su original no vino en la copia.
		fotoDe(domain.Activo{
			ID: id(5), DriveID: "d-perdida", Nombre: "perdida.jpg", Tipo: "image/jpeg", Bytes: 12345,
			Ancho: 10, Alto: 10, SubidaEn: subida,
		}),
		// Un vídeo sin fecha de captura (va a la de subida), una AVIF y una HEIC.
		fotoDe(domain.Activo{
			ID: id(6), DriveID: "d-paseo", Nombre: "paseo.webm", Tipo: "video/webm", Bytes: tam("Fotos/paseo.webm"),
			Ancho: 320, Alto: 180, SubidaEn: time.Date(2024, 3, 4, 5, 6, 7, 0, time.UTC), Favorita: true, Etiquetas: []string{"paseo"},
		}),
		fotoDe(domain.Activo{
			ID: id(7), DriveID: "d-flor", Nombre: "flor.avif", Tipo: "image/avif", Bytes: tam("Fotos/flor.avif"),
			Ancho: 16, Alto: 16, TomadaEn: en("2021-05-06T07:08:09Z"), SubidaEn: subida, Descripcion: "Una flor",
		}),
		fotoDe(domain.Activo{
			ID: id(8), DriveID: "d-heic", Nombre: "IMG_0001.heic", Tipo: "image/heic", Bytes: tam("Fotos/IMG_0001.heic"),
			Ancho: 40, Alto: 30, TomadaEn: en("2020-02-03T04:05:06Z"), SubidaEn: subida, Favorita: true,
		}),
	}
	albumes := []albumExportado{
		{ID: id(10), Title: "Verano 2023", Photos: []uuid.UUID{id(1), id(2)}, CreatedAt: subida, UpdatedAt: subida},
		{ID: id(11), Title: "Invierno", Shared: true, Photos: []uuid.UUID{id(3), id(4)}, CreatedAt: subida, UpdatedAt: subida},
		{ID: id(12), Title: "Vacío", Photos: []uuid.UUID{}, CreatedAt: subida, UpdatedAt: subida},
		// Dos álbumes con el mismo título son dos.
		{ID: id(13), Title: "Verano 2023", Photos: []uuid.UUID{id(6), id(7)}, CreatedAt: subida, UpdatedAt: subida},
		{ID: id(14), Title: "Flores", Photos: []uuid.UUID{id(7), id(8)}, CreatedAt: subida, UpdatedAt: subida},
	}
	regenerar := os.Getenv("SYTUE_REGENERAR") == "1"
	for _, f := range []portabilidad.Formato{portabilidad.JSON, portabilidad.CSV, portabilidad.XML} {
		for _, tabla := range []struct {
			nombre   string
			escribir func(*bytes.Buffer) error
		}{
			{"photos", func(b *bytes.Buffer) error {
				return portabilidad.EscribirTabla(b, f, "photos", "photo", portabilidad.Todas(fotos))
			}},
			{"albums", func(b *bytes.Buffer) error {
				return portabilidad.EscribirTabla(b, f, "albums", "album", portabilidad.Todas(albumes))
			}},
		} {
			var b bytes.Buffer
			if err := tabla.escribir(&b); err != nil {
				t.Fatal(err)
			}
			ruta := filepath.Join(tusDatosDeOffice, "photos", tabla.nombre+"."+string(f))
			if regenerar {
				if err := os.MkdirAll(filepath.Dir(ruta), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(ruta, b.Bytes(), 0o644); err != nil { //nolint:gosec // datos de prueba
					t.Fatal(err)
				}
				continue
			}
			hay, err := os.ReadFile(ruta)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(hay, b.Bytes()) {
				t.Errorf("%s ya no es lo que entrega Fotos: vuelve a escribirla con SYTUE_REGENERAR=1 y pasa las pruebas de apps/nucleo/local/traer", ruta)
			}
		}
	}
}
