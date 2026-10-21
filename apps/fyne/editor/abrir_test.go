// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestAbrirLlevaCadaFicheroASuApp: lo de esta app va al MISMO `cargar` que «Abrir con»,
// con la URI local; lo de otro editor se lo pasa al sistema; y lo que no es de nadie se
// dice. Se vio en ROJO cambiando `producto != a.Tipo.Clave()` por `==`: el `.docx` salía
// por el sistema y el `.xlsx` iba a convertirse en Docus.
func TestAbrirLlevaCadaFicheroASuApp(t *testing.T) {
	var cargados, fuera, fallos []string
	a := &AbrirArchivo{
		Tipo: Documento,
		Elegir: func(listo func([]string)) {
			listo([]string{"/tmp/Acta.docx", "/tmp/Cuentas.xlsx", "/tmp/copia.zip"})
		},
		Cargar: func(_ context.Context, ap api.Apertura) error {
			cargados = append(cargados, ap.Local)
			return nil
		},
		Fuera:    func(r string) error { fuera = append(fuera, r); return nil },
		AlFallar: func(r string, err error) { fallos = append(fallos, r+": "+err.Error()) },
		Lanzar:   func(f func()) { f() },
	}
	a.Abrir()
	if len(cargados) != 1 || cargados[0] != "file:///tmp/Acta.docx" {
		t.Errorf("a Docus le llega %v y tendría que llegarle el .docx por su URI local", cargados)
	}
	if len(fuera) != 1 || fuera[0] != "/tmp/Cuentas.xlsx" {
		t.Errorf("el .xlsx lo abre Calcus, por el sistema: %v", fuera)
	}
	if len(fallos) != 1 || !strings.HasPrefix(fallos[0], "/tmp/copia.zip") {
		t.Errorf("un .zip no lo abre ninguna app de la suite y se tiene que decir: %v", fallos)
	}
	// Una foto no es de ningún editor, pero sí de SyPhotos: también se le pasa al sistema.
	fuera = nil
	a.AbrirRuta("/tmp/foto.png")
	if len(fuera) != 1 {
		t.Errorf("un .png lo abre SyPhotos, por el sistema: %v", fuera)
	}
	// Lo que nadie reclama pero la portada de la web importa va al `cargar` de esta app.
	cargados, fallos = nil, nil
	a.Importa = []string{"csv"}
	a.AbrirRuta("/tmp/Ventas.CSV")
	a.AbrirRuta("/tmp/notas.txt")
	if len(cargados) != 1 || cargados[0] != "file:///tmp/Ventas.CSV" || len(fallos) != 1 {
		t.Errorf("con Importa csv: cargados %v, fallos %v", cargados, fallos)
	}
	// Un fallo al convertir también se dice.
	fallos = nil
	a.Cargar = func(context.Context, api.Apertura) error { return errors.New("413") }
	a.AbrirRuta("/tmp/grande.odt")
	if len(fallos) != 1 {
		t.Errorf("una conversión que falla no se dice: %v", fallos)
	}
}

// TestElMenuArchivoLlevaNuevoYAbrir: con los dos ganchos, las dos primeras entradas son
// «Nuevo <tipo>» y «Abrir un archivo», y detrás el separador; sin ganchos, no salen.
func TestElMenuArchivoLlevaNuevoYAbrir(t *testing.T) {
	for tipo, clave := range map[Tipo]string{
		Documento: "file.new.document", Hoja: "file.new.spreadsheet", Presentacion: "file.new.presentation",
	} {
		nuevo := 0
		ctx := &Contexto{Tipo: tipo, Nuevo: func() { nuevo++ }, Abrir: func() {}}
		m := NuevoBotonArchivo(ctx).Menu()
		got := textos(m)
		if got[0] != clave || got[1] != "list.new.open" {
			t.Errorf("tipo %d: el menú empieza por %v", tipo, got[:2])
		}
		if _, ok := m.Elementos[2].(*ui.SeparadorMenu); !ok {
			t.Errorf("tipo %d: tras «Abrir» va un separador", tipo)
		}
		m.Elementos[0].(*ui.ElementoMenu).AlPulsar()
		if nuevo != 1 {
			t.Error("«Nuevo» no llama a su gancho")
		}
		if tipo.Clave() == "" {
			t.Error("tipo sin clave de producto")
		}
	}
	if got := textos(NuevoBotonArchivo(&Contexto{}).Menu()); got[0] == "file.new.document" || got[0] == "list.new.open" {
		t.Errorf("sin ganchos no hay «Nuevo» ni «Abrir»: %v", got)
	}
}

// TestAbrirTraeUnDocumentoNuevoYEditable: con Importar, Svc y AbrirID, «Abrir» hace lo de
// la portada de la web (`DocList.onImportPick`): convierte, crea un documento NUEVO con el
// título de la conversión, lo siembra con UNA op `set` (actor propio, contador y reloj a
// 1, sin la marca `:lww`) y abre ese documento, editable. Si la siembra falla, no abre un
// documento vacío: lo dice. Se vio en ROJO quitando la siembra: se abría «d9» vacío.
func TestAbrirTraeUnDocumentoNuevoYEditable(t *testing.T) {
	var pedidos []string
	fallaSiembra := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cuerpo, _ := io.ReadAll(r.Body)
		pedidos = append(pedidos, r.Method+" "+r.URL.Path+" "+string(cuerpo))
		switch {
		case r.URL.Path == "/api/v1/documents":
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"document":{"id":"d9","kind":"document","title":"Acta"},"role":"owner"}`)
		case fallaSiembra:
			w.WriteHeader(http.StatusInternalServerError)
		default:
			_, _ = io.WriteString(w, `{"seq":1}`)
		}
	}))
	defer srv.Close()
	var abiertos, fallos []string
	a := &AbrirArchivo{
		Tipo: Documento, Svc: api.NewDocs(srv.URL, sesionFija{}, srv.Client()),
		Importar: func(_ context.Context, ap api.Apertura) (api.Importado, error) {
			if ap.Local != "file:///tmp/Acta.docx" {
				t.Errorf("se importa %q", ap.Local)
			}
			return api.Importado{Kind: api.DocText, Title: "Acta", Content: "<p>hola</p>"}, nil
		},
		AbrirID: func(id, titulo string) { abiertos = append(abiertos, id+" "+titulo) },
		Cargar: func(context.Context, api.Apertura) error {
			t.Error("con Importar no se enseña la conversión")
			return nil
		},
		AlFallar: func(r string, err error) { fallos = append(fallos, r) },
		Lanzar:   func(f func()) { f() },
	}
	a.AbrirRuta("/tmp/Acta.docx")
	if len(abiertos) != 1 || abiertos[0] != "d9 Acta" || len(fallos) != 0 {
		t.Fatalf("se abre %v (fallos %v)", abiertos, fallos)
	}
	if len(pedidos) != 2 || !strings.Contains(pedidos[0], `"kind":"document"`) || !strings.Contains(pedidos[0], `"title":"Acta"`) {
		t.Fatalf("lo pedido al servicio: %v", pedidos)
	}
	siembra := pedidos[1]
	for _, quiero := range []string{"POST /api/v1/documents/d9/ops", `"key":"doc:html"`, `"value":"\u003cp\u003ehola\u003c/p\u003e"`, `"lamport":1`, `"c":1`} {
		if !strings.Contains(siembra, quiero) {
			t.Errorf("la siembra no lleva %s: %s", quiero, siembra)
		}
	}
	if strings.Contains(siembra, ":lww") {
		t.Errorf("la siembra de la web no lleva la marca :lww: %s", siembra)
	}

	abiertos, pedidos, fallaSiembra = nil, nil, true
	a.AbrirRuta("/tmp/Acta.docx")
	if len(abiertos) != 0 || len(fallos) != 1 {
		t.Errorf("con la siembra fallida se abre %v y se dice %v", abiertos, fallos)
	}
}

// TestAbrirSinElegirUsaElDelSistema: sin `Elegir`, «Abrir» usa el selector del sistema; en
// la máquina de pruebas no hay, así que no abre nada y no se cuelga.
func TestAbrirSinElegirUsaElDelSistema(t *testing.T) {
	a := &AbrirArchivo{Tipo: Documento, Cargar: func(context.Context, api.Apertura) error {
		t.Error("sin nada elegido no se carga nada")
		return nil
	}}
	a.Abrir()
}

// TestAbrirGuardaLosComentariosDelFicheroAntesDeSembrar: con ImportarGuardando, los
// comentarios del fichero se guardan en el documento nuevo ANTES de sembrar, y el contenido
// sembrado lleva el ancla de los que quedaron guardados y de ningún otro. Si no se pueden
// guardar, el documento se abre igual, sin comentarios y sin marks.
func TestAbrirGuardaLosComentariosDelFicheroAntesDeSembrar(t *testing.T) {
	var pedidos []string
	fallaComentarios := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cuerpo, _ := io.ReadAll(r.Body)
		pedidos = append(pedidos, r.Method+" "+r.URL.Path+" "+string(cuerpo))
		switch {
		case r.URL.Path == "/api/v1/documents":
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"document":{"id":"d9","kind":"document","title":"Acta"},"role":"owner"}`)
		case strings.HasSuffix(r.URL.Path, "/comments/import") && fallaComentarios:
			w.WriteHeader(http.StatusInternalServerError)
		case strings.HasSuffix(r.URL.Path, "/comments/import"):
			_, _ = io.WriteString(w, `{"saved":2,"anchored":["aa11"]}`)
		default:
			_, _ = io.WriteString(w, `{"seq":1}`)
		}
	}))
	defer srv.Close()
	var abiertos []string
	a := &AbrirArchivo{
		Tipo: Documento, Svc: api.NewDocs(srv.URL, sesionFija{}, srv.Client()),
		Importar: func(context.Context, api.Apertura) (api.Importado, error) {
			t.Error("con ImportarGuardando no se usa Importar")
			return api.Importado{}, nil
		},
		ImportarGuardando: func(context.Context, api.Apertura) (api.Importado, error) {
			return api.Importado{
				Kind:    api.DocText,
				Title:   "Acta",
				Content: `<p><mark data-comment-id="aa11">uno</mark> <mark data-comment-id="bb22">dos</mark></p>`,
				Comments: []api.ImportadoComentario{
					{ID: "aa11", Text: "Ana, 2026-09-30: revisar", Quote: "uno", Anchored: true},
					{ID: "bb22", Text: "Luis: otro", Quote: "dos", Anchored: true},
				},
			}, nil
		},
		AbrirID:  func(id, titulo string) { abiertos = append(abiertos, id) },
		AlFallar: func(r string, err error) { t.Errorf("falló %s: %v", r, err) },
		Lanzar:   func(f func()) { f() },
	}
	a.AbrirRuta("/tmp/Acta.docx")
	if len(abiertos) != 1 || len(pedidos) != 3 || !strings.Contains(pedidos[1], "POST /api/v1/documents/d9/comments/import") || !strings.Contains(pedidos[1], "Ana, 2026-09-30: revisar") {
		t.Fatalf("abiertos %v, pedidos %v", abiertos, pedidos)
	}
	siembra := pedidos[2]
	if !strings.Contains(siembra, `data-comment-id=\"aa11\"`) || strings.Contains(siembra, "bb22") {
		t.Errorf("la siembra tiene que llevar solo el mark guardado: %s", siembra)
	}

	abiertos, pedidos, fallaComentarios = nil, nil, true
	a.AbrirRuta("/tmp/Acta.docx")
	if len(abiertos) != 1 || strings.Contains(pedidos[len(pedidos)-1], "data-comment-id") {
		t.Errorf("sin poder guardar los comentarios, no debe quedar ningún mark: abiertos %v, pedidos %v", abiertos, pedidos)
	}
}

// TestUnFicheroDeFueraSeAbreEditable: lo que llega por «Abrir con», el Finder o Drive se
// abre como DOCUMENTO, no en solo lectura. El de la máquina, nuevo; el de Drive, anclado a
// su fichero y, si ya existía con contenido, el mismo sin convertir ni sembrar otra vez. Sin
// poder crear, la conversión de solo lectura de siempre. Se vio en ROJO volviendo a
// registrar `cargar` en vez de AbrirApertura: los cuatro casos acababan en Cargar.
func TestUnFicheroDeFueraSeAbreEditable(t *testing.T) {
	var pedidos []string
	anclado, fallaCrear := "", false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cuerpo, _ := io.ReadAll(r.Body)
		pedidos = append(pedidos, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery+" "+string(cuerpo))
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/documents":
			_, _ = io.WriteString(w, `{"documents":[`+anclado+`]}`)
		case r.URL.Path == "/api/v1/documents" && fallaCrear:
			w.WriteHeader(http.StatusForbidden)
		case r.URL.Path == "/api/v1/documents":
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"document":{"id":"d9","kind":"document","title":"Acta"},"role":"owner"}`)
		default:
			_, _ = io.WriteString(w, `{"seq":1}`)
		}
	}))
	defer srv.Close()
	var abiertos []string
	importados, cargados := 0, 0
	a := &AbrirArchivo{
		Tipo: Documento, Svc: api.NewDocs(srv.URL, sesionFija{}, srv.Client()),
		Importar: func(context.Context, api.Apertura) (api.Importado, error) {
			importados++
			return api.Importado{Kind: api.DocText, Title: "Acta", Content: "<p>hola</p>"}, nil
		},
		AbrirID: func(id, titulo string) { abiertos = append(abiertos, id+" "+titulo) },
		Cargar:  func(context.Context, api.Apertura) error { cargados++; return nil },
	}
	abrir := func(ap api.Apertura) {
		t.Helper()
		if err := a.AbrirApertura(context.Background(), ap); err != nil {
			t.Fatalf("%+v: %v", ap, err)
		}
	}

	abrir(api.Apertura{Local: "file:///tmp/Acta.docx"})
	if len(abiertos) != 1 || abiertos[0] != "d9 Acta" || cargados != 0 || strings.Contains(strings.Join(pedidos, "\n"), "source_ref") {
		t.Fatalf("el fichero de la máquina: abre %v, en solo lectura %d, pide %v", abiertos, cargados, pedidos)
	}

	abiertos, pedidos = nil, nil
	abrir(api.Apertura{Ref: api.Referencia{ID: "f1"}})
	if len(abiertos) != 1 || !strings.Contains(strings.Join(pedidos, "\n"), `"source_ref":"drive:f1"`) {
		t.Fatalf("el de Drive, nuevo: abre %v, pide %v", abiertos, pedidos)
	}

	abiertos, pedidos, anclado, importados = nil, nil, `{"id":"d7","kind":"document","title":"Acta","seq":4}`, 0
	abrir(api.Apertura{Ref: api.Referencia{ID: "f1"}})
	if len(abiertos) != 1 || abiertos[0] != "d7 Acta" || importados != 0 || len(pedidos) != 1 {
		t.Fatalf("el de Drive ya abierto: abre %v, convierte %d veces, pide %v", abiertos, importados, pedidos)
	}

	abiertos, anclado, fallaCrear = nil, "", true
	abrir(api.Apertura{Local: "file:///tmp/Acta.docx"})
	if len(abiertos) != 0 || cargados != 1 {
		t.Fatalf("sin poder crear: abre %v y en solo lectura %d", abiertos, cargados)
	}
}
