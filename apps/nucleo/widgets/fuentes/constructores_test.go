// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package fuentes

import (
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets"
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

// TestCadaFilaLlevaLaRutaDeSuElemento (formato 3): cada constructor pone en la fila la
// ruta de su elemento dentro de la app, con identificadores (o, en Drive y CertiPDF, la
// ruta del fichero), y ninguna se sale de widgets.ValidRoute. La ida y vuelta por el
// receptor de cada app está en apps/fyne (TestCadaFilaDelWidgetAbreSuElemento).
func TestCadaFilaLlevaLaRutaDeSuElemento(t *testing.T) {
	const uuid = "0b5c6f3e-1d2a-4c8e-9f00-112233445566"
	ahora := time.Date(2026, 11, 24, 9, 0, 0, 0, time.UTC)
	casos := map[string]struct {
		items []widgets.Item
		ruta  string
	}{
		"notes": {
			itemsDeNotas(api.Notebook{ID: "nb 1", Title: "P"}, []api.Note{{ID: "n1", Title: "Plan", UpdatedAt: ahora}}),
			"/notebooks/nb%201?note=n1",
		},
		"contacts":          {itemsDeContactos([]api.Contact{{ID: uuid, FormattedName: "Elena"}}), "/contacts/" + uuid},
		"contacts sin uuid": {itemsDeContactos([]api.Contact{{ID: "c1", FormattedName: "Elena"}}), ""},
		"drive carpeta": {
			itemsDeRaiz("es", api.Children{Folders: []api.Folder{{ID: "f", Name: "Q3 & Q4"}}}),
			"/?path=%2FQ3%20%26%20Q4",
		},
		"drive fichero": {
			itemsDeRaiz("es", api.Children{Files: []api.File{{ID: "a", Name: "ñ.pdf", SizeBytes: 1}}}),
			"/?preview=%2F%C3%B1.pdf",
		},
		"pdf":          {itemsDePDF([]api.File{{ID: "a", Name: "Contrato.pdf"}}), "/pdf?path=%2FPDF%2FContrato.pdf"},
		"cast":         {itemsDeCast(nil, []api.CastVideo{{ID: "v1", Title: "Demo"}}, nil), "/watch/v1"},
		"rooms":        {detalleRooms([]api.RoomsChannel{{ID: "!c1:sytue.eu", Kind: "public", Name: "general"}}, nil, true).Items, "/channels/%21c1%3Asytue.eu"},
		"photos album": {itemsDeAlbumes([]api.Album{{ID: "al1", Title: "Viaje", Count: 3}}), "/album?id=al1"},
		"photos foto":  {itemsDeFotos(Comunes{}, []api.Photo{{ID: "p1", Name: "IMG.jpg", DriveFileID: "df1"}}), "/abrir?file=df1"},
		"one reciente": {actividadReciente([]api.HubHit{{Source: "notes", Kind: "note", Title: "Plan"}}), "/actividad"},
		"one agenda":   {agendaDeHoy([]api.EventoDeHoy{{Resumen: "Daily", Inicio: ahora}}, ahora), "/agenda?date=2026-11-24"},
		// Uno de día completo es flotante: su fecha, aunque en la zona de la app fuera otra.
		"one agenda, día completo": {
			agendaDeHoy([]api.EventoDeHoy{{Resumen: "Fiesta", Inicio: time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC), TodoElDia: true}},
				ahora.In(time.FixedZone("Honolulu", -10*3600))),
			"/agenda?date=2026-11-24&all_day=1",
		},
		"mail":          {cabecerasDeEntrada(Comunes{}, []api.MailMessage{{ID: "m1", ThreadID: "t 1", From: "ana@sytue.eu"}}), "/?thread=t%201"},
		"mail sin hilo": {cabecerasDeEntrada(Comunes{}, []api.MailMessage{{ID: "m1", From: "ana@sytue.eu"}}), ""},
	}
	firmas, _ := itemsDeFirmas(api.PDFPeticionesFirma{Recibidas: []api.PDFPeticionFirma{
		{Ruta: "/PDF/Alta.pdf", Estado: "pendiente"}, {Nombre: "De fuera.pdf", NombrePide: "Ana", Estado: "pendiente"},
	}})
	casos["pdf firma"] = struct {
		items []widgets.Item
		ruta  string
	}{firmas[:1], "/pdf?path=%2FPDF%2FAlta.pdf"}
	casos["pdf firma de fuera, sin copia aún"] = struct {
		items []widgets.Item
		ruta  string
	}{firmas[1:], ""}
	for nombre, c := range casos {
		if len(c.items) != 1 || c.items[0].Route != c.ruta || !widgets.ValidRoute(c.items[0].Route) {
			t.Errorf("%s: filas %+v, quiero la ruta %q", nombre, c.items, c.ruta)
			continue
		}
		if strings.Contains(c.items[0].Route, "token") {
			t.Errorf("%s: una ruta no lleva tokens", nombre)
		}
	}
	for _, r := range []string{RutaDocumento("d1"), RutaBase(uuid), RutaTareasERP(), RutaDia(ahora, false)} {
		if r == "" || !widgets.ValidRoute(r) {
			t.Errorf("ruta %q", r)
		}
	}
	if RutaDocumento("d1") != "/edit?id=d1" || RutaBase(uuid) != "/base/"+uuid || RutaTareasERP() != "/?vista=tareas" ||
		RutaDia(ahora, false) != "/?date=2026-11-24" || RutaBase("x") != "" {
		t.Fatal("rutas de Docus, Bases, ERP o Calendar")
	}
}
