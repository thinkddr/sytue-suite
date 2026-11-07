// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package fuentes

import (
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Lo que comprobaban los widgets_test.go de cada pantalla cuando las pantallas
// publicaban; los constructores se mudaron aquí y sus pruebas con ellos.

func TestCastPoneDelanteElDirectoYDaFormatoALaDuracion(t *testing.T) {
	items := itemsDeCast(
		[]api.CastVideo{{ID: "l1", Title: "Demo trimestral", Owner: api.CastOwner{Name: "Equipo"}, Status: api.CastStatusLive}},
		[]api.CastVideo{{ID: "v1", Title: "Tutorial de Onboarding", Owner: api.CastOwner{Name: "Ada"}, DurationMS: 185000}},
		nil,
	)
	if len(items) != 2 || items[0].Badge != "LIVE" || !items[0].Unread || items[1].Badge != "3:05" {
		t.Fatalf("items de cast inesperados: %+v", items)
	}
	if duracion(3_725_000) != "1:02:05" {
		t.Fatalf("duración de más de una hora: %s", duracion(3_725_000))
	}
}

func TestNotesRechazaConteosInvalidos(t *testing.T) {
	for _, n := range []int{-1, 1_000_000_000_001} {
		if total, valido := totalDeNotas([]api.Notebook{{ID: "one", NoteCount: n}}); valido {
			t.Fatalf("aceptó un contador inválido: %d", total)
		}
	}
	if _, valido := totalDeNotas([]api.Notebook{{ID: "one", NoteCount: 1_000_000_000_000}, {ID: "two", NoteCount: 1}}); valido {
		t.Fatal("aceptó una suma fuera del límite")
	}
	archivado := time.Now()
	if total, valido := totalDeNotas([]api.Notebook{{ID: "archive", ArchivedAt: &archivado, NoteCount: -1}}); !valido || total != 0 {
		t.Fatalf("incluyó un cuaderno archivado: %d, %v", total, valido)
	}
}

func TestNotesPublicaTareasSinLasCifradas(t *testing.T) {
	ahora := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	items := itemsDeNotas(
		api.Notebook{ID: "nb1", Title: "Producto", Color: "#a91fb4"},
		[]api.Note{
			{ID: "n1", Kind: "task", Content: "<p>Revisar lanzamiento</p>", UpdatedAt: ahora},
			{ID: "n2", Kind: "text", Title: "Notas de arquitectura", Content: "<p>Secreto del cuerpo</p>", Pinned: true, UpdatedAt: ahora.Add(-time.Hour)},
			{ID: "n3", Kind: "text", Title: "Cifrada", ContentCipher: "enc:v1:abc", UpdatedAt: ahora},
		},
	)
	if len(items) != 2 || items[0].Title != "Notas de arquitectura" || items[1].Title != "Revisar lanzamiento" || !items[1].Unread {
		t.Fatalf("items de notas inesperados: %+v", items)
	}
}

func TestContactsPublicaFichasConMonograma(t *testing.T) {
	items := itemsDeContactos([]api.Contact{
		{ID: "c1", FormattedName: "Elena Rivas", Organization: "Sytue", UpdatedAt: time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)},
	})
	if len(items) != 1 || items[0].Title != "Elena Rivas" || items[0].Subtitle != "Sytue" || items[0].Badge != "ER" {
		t.Fatalf("items de contactos inesperados: %+v", items)
	}
}

func TestDrivePoneLasCarpetasDelanteYElTamanoDeLosFicheros(t *testing.T) {
	items := itemsDeRaiz("es", api.Children{
		Folders: []api.Folder{{ID: "f", Name: "Proyectos"}},
		Files:   []api.File{{ID: "a", Name: "informe.pdf", SizeBytes: 2048, ContentType: "application/pdf"}},
	})
	if len(items) != 2 || items[0].Badge != "DIR" || items[1].Title != "informe.pdf" || items[1].Badge == "" {
		t.Fatalf("items de la raíz: %+v", items)
	}
}
