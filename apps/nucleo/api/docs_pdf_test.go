// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// driveDePrueba finge los dos servicios en un solo servidor: Drive con (o sin) carpeta
// «PDF», y el motor de Docs. Apunta lo que se sube y lo que le llega al motor.
type driveDePrueba struct {
	t          *testing.T
	conCarpeta bool
	subido     struct{ carpeta, nombre, tipo, cuerpo string }
	creada     string
	motor      struct {
		ruta     string
		campos   map[string]string
		ficheros []string // «nombre=contenido», en el orden en que llegaron
	}
}

func (d *driveDePrueba) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method + " " + r.URL.Path {
	case "GET /api/v1/folders/root":
		_, _ = io.WriteString(w, `{"id":"raiz","is_root":true}`)
	case "GET /api/v1/folders/raiz/children":
		if d.conCarpeta {
			_, _ = io.WriteString(w, `{"folders":[{"id":"otra","name":"Fotos"},{"id":"cpdf","name":"PDF"}],"files":[{"id":"x","name":"suelto.pdf"}]}`)
			return
		}
		_, _ = io.WriteString(w, `{"folders":[],"files":[]}`)
	case "GET /api/v1/folders/cpdf/children":
		if r.URL.Query().Get("cursor") == "" {
			_, _ = io.WriteString(w, `{"folders":[],"files":[{"id":"f1","folder_id":"cpdf","name":"viejo.pdf","updated_at":"2026-01-01T10:00:00Z"},{"id":"f0","folder_id":"cpdf","name":"notas.txt","updated_at":"2026-09-01T10:00:00Z"}],"next_cursor":"p2"}`)
			return
		}
		_, _ = io.WriteString(w, `{"folders":[],"files":[{"id":"f2","folder_id":"cpdf","name":"Contrato.PDF","size_bytes":9,"updated_at":"2026-09-20T10:00:00Z"}]}`)
	case "POST /api/v1/folders":
		cuerpo, _ := io.ReadAll(r.Body)
		d.creada = string(cuerpo)
		_, _ = io.WriteString(w, `{"id":"cpdf","name":"PDF"}`)
	case "POST /api/v1/folders/cpdf/files":
		cuerpo, _ := io.ReadAll(r.Body)
		d.subido.carpeta, d.subido.nombre, d.subido.tipo, d.subido.cuerpo = "cpdf", r.URL.Query().Get("name"), r.Header.Get("Content-Type"), string(cuerpo)
		_, _ = io.WriteString(w, `{"id":"nuevo","folder_id":"cpdf","name":"`+d.subido.nombre+`"}`)
	case "PATCH /api/v1/files/f1":
		cuerpo, _ := io.ReadAll(r.Body)
		if string(cuerpo) != `{"name":"Acta de junio.pdf"}` {
			d.t.Errorf("renombrar mandó %s", cuerpo)
		}
		_, _ = io.WriteString(w, `{"id":"f1","folder_id":"cpdf","name":"Acta de junio.pdf"}`)
	case "DELETE /api/v1/files/f1":
		w.WriteHeader(http.StatusNoContent)
	case "GET /api/v1/files/f1/content":
		_, _ = io.WriteString(w, "%PDF-1.7 hola")
	case "GET /api/v1/folders/otra/children":
		_, _ = io.WriteString(w, `{"folders":[],"files":[{"id":"img1","folder_id":"otra","name":"recibo.JPG"}]}`)
	case "GET /api/v1/files/img1/content":
		_, _ = io.WriteString(w, "JPEG")
	case "POST /api/v1/pdf/content", "POST /api/v1/pdf/text", "POST /api/v1/pdf/pages", "POST /api/v1/pdf/render",
		"POST /api/v1/pdf/annotate", "POST /api/v1/pdf/protect", "POST /api/v1/pdf/sign", "POST /api/v1/pdf/tools":
		d.motorHTTP(w, r)
	default:
		d.t.Errorf("petición inesperada: %s %s", r.Method, r.URL)
		w.WriteHeader(http.StatusNotFound)
	}
}

func (d *driveDePrueba) motorHTTP(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		d.t.Errorf("el motor espera multipart: %v", err)
	}
	d.motor.ruta, d.motor.campos, d.motor.ficheros = r.URL.Path, map[string]string{}, nil
	for k, v := range r.MultipartForm.Value {
		d.motor.campos[k] = v[0]
	}
	for _, ch := range r.MultipartForm.File["file"] {
		f, _ := ch.Open()
		datos, _ := io.ReadAll(f)
		d.motor.ficheros = append(d.motor.ficheros, ch.Filename+"="+string(datos))
	}
	for campo, chs := range r.MultipartForm.File {
		if campo == "file" {
			continue
		}
		f, _ := chs[0].Open()
		datos, _ := io.ReadAll(f)
		d.motor.ficheros = append(d.motor.ficheros, campo+":"+chs[0].Filename+"="+string(datos))
	}
	switch {
	case d.motor.campos["clave"] == "mala":
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"code":"pdf_password_required","message":"needs a password"}`)
	case r.URL.Path == "/api/v1/pdf/content":
		_, _ = io.WriteString(w, `{"paginas":[{"numero":1,"ancho":595.28,"alto":841.89,"rotacion":90,"caja":[0,0,595.28,841.89],"extracto":"Contrato de…"}],"total":1,
			"indice":[{"titulo":"Cláusulas","pagina":0,"nivel":0}],"enlaces":null,"adjuntos":[{"nombre":"factura.xml","tamano_bytes":2048,"pagina":-1}],
			"anotaciones":[{"pagina":0,"tipo":"Text","rect":[0,0,1,1],"texto":"Revisar","autor":"Ada"}],"campos":[{"nombre":"nif","tipo":"texto","pagina":0,"rect":[0,0,1,1],"obligatorio":true,"solo_lectura":false}],
			"firmas":[{"nombre":"Grace Hopper","fecha":"D:20260920","cubre_todo":true}],"cifrado":true,
			"permisos":{"Imprimir":true,"Modificar":false,"Ensamblar":true},"huella":{"Autor":"Ada","Productor":"Sytue","ConJS":true}}`)
	case r.URL.Path == "/api/v1/pdf/render" && d.motor.campos["formato"] == "png":
		// «iVBOR» es la cabecera de un PNG en base64; la página 2 no se pudo dibujar.
		w.Header().Set("X-Sytue-Prueba", "png")
		_, _ = io.WriteString(w, `{"paginas":{"1":"iVBORw0KGgo=","3":"no es base64!"},"total":3,"formato":"png"}`)
	case r.URL.Path == "/api/v1/pdf/render":
		w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
		_, _ = io.WriteString(w, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 595 842"/>`)
	case r.URL.Path == "/api/v1/pdf/text":
		_, _ = io.WriteString(w, `{"paginas":[[{"texto":"Hola","x":72,"y":700,"tamano":12}],null]}`)
	default:
		_, _ = io.WriteString(w, "%PDF-resultado")
	}
}

func TestPDFEnDrive(t *testing.T) {
	d := &driveDePrueba{t: t, conCarpeta: true}
	srv := httptest.NewServer(d)
	defer srv.Close()
	p, ctx := NewPDF(srv.URL, srv.URL, nil, nil), context.Background()

	// Solo los PDF de la carpeta «PDF», el más reciente primero y sin mirar mayúsculas en la
	// extensión; ni el .txt de dentro ni el PDF suelto de la raíz.
	pdfs, err := p.Lista(ctx)
	if err != nil || len(pdfs) != 2 || pdfs[0].ID != "f2" || pdfs[1].ID != "f1" {
		t.Fatalf("Lista = %+v, %v", pdfs, err)
	}
	// «viejo.pdf» ya existe: subir con ese nombre lo SUSTITUIRÍA en Drive.
	f, err := p.Subir(ctx, `C:\Users\ada\viejo.pdf`, strings.NewReader("%PDF-1.4 cuerpo"), 15)
	if err != nil || d.subido.nombre != "viejo (2).pdf" || d.subido.tipo != "application/pdf" || d.subido.cuerpo != "%PDF-1.4 cuerpo" || f.ID != "nuevo" {
		t.Fatalf("Subir = %+v, %v; llegó %+v", f, err, d.subido)
	}
	if _, err := p.GuardarCopia(ctx, pdfs[0], "girado", []byte("%PDF-x")); err != nil || d.subido.nombre != "Contrato (girado).pdf" {
		t.Fatalf("GuardarCopia: %v; llegó %+v", err, d.subido)
	}
	if f, err := p.Renombrar(ctx, "f1", "Acta de junio"); err != nil || f.Name != "Acta de junio.pdf" {
		t.Fatalf("Renombrar = %+v, %v", f, err)
	}
	if err := p.Eliminar(ctx, "f1"); err != nil {
		t.Fatal(err)
	}
	if datos, err := p.Descargar(ctx, "f1"); err != nil || string(datos) != "%PDF-1.7 hola" {
		t.Fatalf("Descargar = %q, %v", datos, err)
	}
}

func TestSubirCreaLaCarpetaYRechazaLoQueNoEsPDF(t *testing.T) {
	if PDFMaxBytes != 100<<20 {
		t.Fatalf("PDFMaxBytes = %d; quiero 100 MiB", PDFMaxBytes)
	}
	d := &driveDePrueba{t: t}
	srv := httptest.NewServer(d)
	defer srv.Close()
	p, ctx := NewPDF(srv.URL, srv.URL, nil, nil), context.Background()

	if pdfs, err := p.Lista(ctx); err != nil || pdfs != nil || d.creada != "" {
		t.Fatalf("sin carpeta la lista es vacía y NO se crea nada: %+v, %v, %q", pdfs, err, d.creada)
	}
	if _, err := p.Subir(ctx, "foto.pdf", strings.NewReader("\x89PNG de verdad"), 14); !errors.Is(err, ErrNoEsPDF) || d.creada != "" {
		t.Fatalf("un PNG con nombre de PDF: %v (carpeta creada: %q)", err, d.creada)
	}
	if _, err := p.Subir(ctx, "enorme.pdf", strings.NewReader("%PDF-"), PDFMaxBytes+1); !errors.Is(err, ErrPDFGrande) {
		t.Fatalf("un PDF mayor que el tope: %v", err)
	}
	if _, err := p.Subir(ctx, "", strings.NewReader("%PDF-1.4"), 8); err != nil || d.creada != `{"name":"PDF","parent_id":"raiz"}` || d.subido.nombre != "documento.pdf" {
		t.Fatalf("Subir: %v; carpeta %q, nombre %q", err, d.creada, d.subido.nombre)
	}
}

func TestElMotorDePDF(t *testing.T) {
	d := &driveDePrueba{t: t}
	srv := httptest.NewServer(d)
	defer srv.Close()
	p, ctx := NewPDF(srv.URL, srv.URL, nil, nil), context.Background()
	doc := PDFFichero{Nombre: "contrato.pdf", Datos: []byte("%PDF-a")}

	c, err := p.Contenido(ctx, doc, "abracadabra")
	if err != nil || d.motor.ruta != "/api/v1/pdf/content" || d.motor.campos["clave"] != "abracadabra" || d.motor.ficheros[0] != "contrato.pdf=%PDF-a" {
		t.Fatalf("Contenido: %v; llegó %+v", err, d.motor)
	}
	if c.Total != 1 || c.Paginas[0].Rotacion != 90 || c.Paginas[0].Extracto != "Contrato de…" || c.Indice[0].Titulo != "Cláusulas" ||
		c.Adjuntos[0].TamanoBytes != 2048 || c.Anotaciones[0].Autor != "Ada" || c.Campos[0].Nombre != "nif" ||
		!c.Firmas[0].CubreTodo || !c.Cifrado || c.Permisos.Modificar || !c.Permisos.Ensamblar || c.Huella.Autor != "Ada" || !c.Huella.ConJS {
		t.Fatalf("Contenido = %+v", c)
	}
	if _, err := p.Contenido(ctx, doc, "mala"); !PDFPideClave(err) || PDFMensaje(err) != "needs a password" {
		t.Fatalf("una clave rechazada tiene que reconocerse: %v", err)
	}
	texto, err := p.Texto(ctx, doc, "")
	if err != nil || len(texto) != 2 || texto[0][0].Texto != "Hola" || texto[1] != nil {
		t.Fatalf("Texto = %+v, %v", texto, err)
	}
	if _, hay := d.motor.campos["clave"]; hay {
		t.Error("una clave vacía no se manda")
	}

	svg, err := p.Render(ctx, doc, 3, 1.5, "")
	if err != nil || !strings.HasPrefix(svg, "<svg") || d.motor.ruta != "/api/v1/pdf/render" || d.motor.campos["pagina"] != "3" || d.motor.campos["escala"] != "1.5" {
		t.Fatalf("Render = %q, %v; llegó %+v", svg, err, d.motor)
	}

	salida, err := p.Paginas(ctx, PDFRotar, "2,1", 270, "", doc)
	if err != nil || string(salida) != "%PDF-resultado" || d.motor.campos["op"] != "rotar" || d.motor.campos["paginas"] != "2,1" || d.motor.campos["grados"] != "270" {
		t.Fatalf("Paginas(rotar) = %q, %v; llegó %+v", salida, err, d.motor.campos)
	}
	// Combinar une los ficheros EN EL ORDEN en que se dan.
	anexo := PDFFichero{Nombre: "anexo.pdf", Datos: []byte("%PDF-b")}
	if _, err := p.Paginas(ctx, PDFCombinar, "", 0, "", doc, anexo); err != nil || strings.Join(d.motor.ficheros, "|") != "contrato.pdf=%PDF-a|anexo.pdf=%PDF-b" {
		t.Fatalf("Paginas(combinar): %v; llegó %v", err, d.motor.ficheros)
	}
	if _, hay := d.motor.campos["grados"]; hay {
		t.Error("los grados solo se mandan al rotar")
	}
	// El tope es del cuerpo ENTERO: dos ficheros que caben por separado no caben juntos.
	medio := PDFFichero{Nombre: "m.pdf", Datos: make([]byte, PDFMaxBytes/2+1)}
	if _, err := p.Paginas(ctx, PDFCombinar, "", 0, "", medio, medio); !errors.Is(err, ErrPDFGrande) {
		t.Fatalf("dos mitades y pico: %v", err)
	}
}

func TestNombreLibre(t *testing.T) {
	ahora := time.Date(2026, 9, 21, 8, 5, 0, 0, time.UTC)
	hay := func(nombres ...string) func(string) bool {
		return func(n string) bool {
			for _, h := range nombres {
				if strings.EqualFold(h, n) {
					return true
				}
			}
			return false
		}
	}
	for _, c := range []struct {
		nombre, sufijo string
		ocupado        func(string) bool
		quiero         string
	}{
		{"a.pdf", "", hay(), "a.pdf"},
		{"a.pdf", "", hay("A.PDF"), "a (2).pdf"},
		{"a.pdf", "", hay("a.pdf", "a (2).pdf"), "a (3).pdf"},
		{"Contrato.PDF", "extraído", hay("Contrato.PDF"), "Contrato (extraído).pdf"},
		{"a.pdf", "girado", hay("a (girado).pdf"), "a (girado 2).pdf"},
		{"a.pdf", "girado", func(string) bool { return true }, "a (girado 2026-09-21-08-05-00).pdf"},
		{"a.pdf", "", func(string) bool { return true }, "a (2026-09-21-08-05-00).pdf"},
	} {
		if got := NombreLibre(c.nombre, c.sufijo, c.ocupado, ahora); got != c.quiero {
			t.Errorf("NombreLibre(%q, %q) = %q, quiero %q", c.nombre, c.sufijo, got, c.quiero)
		}
	}
	for crudo, quiero := range map[string]string{
		"../../etc/passwd": "passwd.pdf", "a\nb.pdf": "a_b.pdf", `que?*.PDF`: "que__.PDF", "  ": "documento.pdf",
		strings.Repeat("ñ", 200) + ".pdf": strings.Repeat("ñ", 116) + ".pdf",
	} {
		if got := NombrePDF(crudo); got != quiero {
			t.Errorf("NombrePDF(%q) = %q, quiero %q", crudo, got, quiero)
		}
	}
}

func TestCertiDocsHablaComoSusServicios(t *testing.T) {
	for fichero, literales := range map[string][]string{
		"services/docs/internal/http/handler.go": {
			`pr.Post("/api/v1/pdf/pages", h.pdfPages)`, `pr.Post("/api/v1/pdf/content", h.pdfContenido)`, `pr.Post("/api/v1/pdf/text", h.pdfTexto)`,
			`pr.Post("/api/v1/pdf/render", h.pdfRender)`,
		},
		"services/docs/internal/http/pdfpages.go": {
			`r.MultipartForm.File["file"]`, `r.FormValue("clave")`, `r.FormValue("op")`, `r.FormValue("paginas"), r.FormValue("grados")`,
			`case "combinar":`, `case "extraer", "borrar", "rotar":`, `WithCode("pdf_password_required")`,
		},
		"services/docs/internal/http/pdfstudio.go": {
			`"paginas":     indiceDe(doc, doc.NumPaginas(), presupuestoExtractos),`, `"total":       doc.NumPaginas()`, `"indice":      doc.IndiceDelDocumento()`, `"adjuntos":    doc.AdjuntosDelDocumento()`,
			`"anotaciones": doc.AnotacionesDelDocumento()`, `"campos":      doc.CamposDelFormulario()`, `"firmas":      doc.FirmasDelDocumento()`,
			`"cifrado":     doc.EstaCifrado()`, `"permisos":    permisos`, `"huella":      doc.Huella()`,
			"`json:\"texto\"`", "`json:\"tamano\"`", `map[string]any{"paginas": paginas}`,
			`r.FormValue("escala")`, `enteroOpcional(r.FormValue("pagina"), 1, doc.NumPaginas())`, `"image/svg+xml; charset=utf-8"`,
			`const maxPaginasPNGPorLote = 8`, `base64.StdEncoding.EncodeToString(datos)`, `formato != "png"`,
		},
		"services/docs/internal/http/pdfinspect.go":    {"`json:\"numero\"`", "`json:\"ancho\"`", "`json:\"alto\"`", "`json:\"rotacion\"`", "`json:\"extracto,omitempty\"`"},
		"services/docs/internal/domain/import.go":      {`const MaxImportBytes = 25 << 20`},
		"services/docs/internal/domain/pdfread.go":     {`const MaxPDFBytes = 100 << 20`},
		"services/docs/internal/domain/pdfindice.go":   {"`json:\"titulo\"`", "`json:\"nivel\"`"},
		"services/docs/internal/domain/pdfadjuntos.go": {"`json:\"nombre\"`", "`json:\"tamano_bytes\"`"},
		"services/docs/internal/domain/pdfannot.go":    {"`json:\"tipo\"`", "`json:\"texto,omitempty\"`", "`json:\"autor,omitempty\"`"},
		"services/docs/internal/domain/pdfform.go":     {"`json:\"nombre\"`", "`json:\"valor,omitempty\"`"},
		"services/docs/internal/domain/pdfsign.go":     {"`json:\"motivo,omitempty\"`", "`json:\"lugar,omitempty\"`", "`json:\"fecha,omitempty\"`", "`json:\"cubre_todo\"`"},
		// Huella y Permisos viajan SIN etiquetas JSON: con el nombre del campo de Go.
		"services/docs/internal/domain/pdfprotect.go": {"\tAutor ", "\tCreador ", "\tProductor ", "\tTitulo ", "\tConAdjuntos ", "\tConJS "},
		"services/docs/internal/domain/pdfcrypt.go":   {"\tModificar ", "\tEnsamblar "},
		"services/drive/internal/http/server.go": {
			`api.Patch("/files/{id}", h.UpdateFile)`, `api.Delete("/files/{id}", h.DeleteFile)`,
		},
		"services/drive/internal/http/handlers.go": {"Name        *string `json:\"name,omitempty\"`"},
		// La carpeta es un acuerdo con la web: si allí cambia, los dos frentes dejan de ver lo mismo.
		"apps/frontend/packages/office/src/pages/api/pdf-abrir.ts":     {`const CARPETA = '/PDF';`, `!== '%PDF-'`},
		"apps/frontend/packages/office/src/pages/api/pdf-recientes.ts": {`const CARPETA = '/PDF';`},
	} {
		fuente := webref.Read(t, fichero)
		for _, l := range literales {
			if !strings.Contains(fuente, l) {
				t.Errorf("%s ya no dice %s: revisar internal/api/docs_pdf.go", fichero, l)
			}
		}
	}
}

func TestElBFFDelPDFEnElCliente(t *testing.T) {
	d := &driveDePrueba{t: t, conCarpeta: true}
	srv := httptest.NewServer(d)
	defer srv.Close()
	p, ctx := NewPDF(srv.URL, srv.URL, nil, nil), context.Background()
	doc := PDFFichero{Nombre: "viejo.pdf", Datos: []byte("%PDF-a")}

	// La ruta con la que se abre CertiPDF, carpeta a carpeta y con el nombre EXACTO.
	f, err := p.PorRuta(ctx, "/PDF/Contrato.PDF")
	if err != nil || f.ID != "f2" {
		t.Fatalf("PorRuta = %+v, %v", f, err)
	}
	for _, ruta := range []string{"/PDF/contrato.pdf", "/Otra/viejo.pdf", "/", ""} {
		var e *Error
		if _, err := p.PorRuta(ctx, ruta); !errors.As(err, &e) || e.Status != http.StatusNotFound {
			t.Errorf("PorRuta(%q) = %v; tiene que ser un 404 como los del servicio", ruta, err)
		}
	}

	// El PNG por lotes: base64 decodificado, y lo que no se pudo dibujar no está.
	png, err := p.RenderPNG(ctx, doc, "1-3", 2.5, "")
	if err != nil || d.motor.campos["formato"] != "png" || d.motor.campos["paginas"] != "1-3" || d.motor.campos["escala"] != "2.5" {
		t.Fatalf("RenderPNG: %v; llegó %+v", err, d.motor.campos)
	}
	if len(png) != 1 || !bytes.HasPrefix(png[1], []byte("\x89PNG")) {
		t.Fatalf("RenderPNG = %v", png)
	}

	// Anotar SOBRESCRIBE: mismo nombre, misma carpeta; el JSON va en el campo de la tabla.
	a, err := p.Aplicar(ctx, File{ID: "f1", FolderID: "cpdf", Name: "viejo.pdf"}, doc, "", PDFCambio{
		Op: "anotar", Datos: []map[string]any{{"tipo": "nota"}}, Sufijo: "no se usa",
	})
	if err != nil || a.Copia || d.subido.nombre != "viejo.pdf" || d.motor.campos["op"] != "poner" ||
		d.motor.campos["anotaciones"] != `[{"tipo":"nota"}]` || string(a.Datos) != "%PDF-resultado" {
		t.Fatalf("Aplicar(anotar) = %+v, %v; subido %+v, motor %+v", a, err, d.subido, d.motor.campos)
	}
	// Censurar deja SIEMPRE copia aunque no se pida, con la palabra que se dé.
	a, err = p.Aplicar(ctx, File{ID: "f1", FolderID: "cpdf", Name: "viejo.pdf"}, doc, "k", PDFCambio{
		Op: "redactar", Datos: []int{1}, Sufijo: "censurado",
	})
	if err != nil || !a.Copia || d.subido.nombre != "viejo (censurado).pdf" || d.motor.campos["op"] != "redactar" ||
		d.motor.campos["areas"] != "[1]" || d.motor.campos["clave"] != "k" {
		t.Fatalf("Aplicar(redactar) = %+v, %v; subido %+v, motor %+v", a, err, d.subido, d.motor.campos)
	}
	// Los ficheros sueltos viajan en SU campo, detrás del documento.
	if _, err := p.Aplicar(ctx, File{FolderID: "cpdf", Name: "viejo.pdf"}, doc, "", PDFCambio{
		Op: "firmar", Partes: []PDFParte{{Campo: "imagen", Nombre: "firma.png", Datos: []byte("png")}},
	}); err != nil || strings.Join(d.motor.ficheros, "|") != "viejo.pdf=%PDF-a|imagen:firma.png=png" {
		t.Fatalf("Aplicar(firmar): %v; llegó %v", err, d.motor.ficheros)
	}
	if _, err := p.Aplicar(ctx, File{}, doc, "", PDFCambio{Op: "inventada"}); err == nil {
		t.Error("una operación que no está en la tabla no se manda")
	}
	if !PDFSiempreCopia("redactar") || PDFSiempreCopia("anotar") {
		t.Error("PDFSiempreCopia no lee la tabla")
	}
}

// TestLaTablaDeAplicarEsLaDeLaWeb lee `OPERACIONES` y `CAMPO_DATOS` de `pdf-aplicar.ts` y
// los compara con [pdfOperaciones] por IGUALDAD, en los dos sentidos: una operación que
// la web añada, o que pase a dejar copia, sale aquí en rojo en vez de sobrescribir en
// nativo lo que en la web se guarda aparte.
func TestLaTablaDeAplicarEsLaDeLaWeb(t *testing.T) {
	fuente := webref.Read(t, "apps/frontend/packages/office/src/pages/api/pdf-aplicar.ts")
	fila := regexp.MustCompile(`(?m)^\s+'?([a-z-]+)'?: \{ ruta: '([^']+)'(?:, motor: '([^']+)')?(, siempreCopia: true)? \},`)
	web := map[string]pdfOperacion{}
	for _, m := range fila.FindAllStringSubmatch(fuente, -1) {
		web[m[1]] = pdfOperacion{ruta: m[2], motor: m[3], siempreCopia: m[4] != ""}
	}
	i := strings.Index(fuente, "const CAMPO_DATOS")
	if i < 0 {
		t.Fatal("pdf-aplicar.ts ya no tiene CAMPO_DATOS")
	}
	campo := regexp.MustCompile(`'?([a-z-]+)'?: '([a-z]+)',`)
	for _, m := range campo.FindAllStringSubmatch(fuente[i:strings.Index(fuente[i:], "};")+i], -1) {
		o := web[m[1]]
		o.campoDatos = m[2]
		web[m[1]] = o
	}
	if len(web) < 20 {
		t.Fatalf("solo se leyeron %d operaciones de la web: el patrón ya no casa", len(web))
	}
	for op, w := range web {
		if pdfOperaciones[op] != w {
			t.Errorf("%s: la web dice %+v y el cliente %+v", op, w, pdfOperaciones[op])
		}
	}
	for op := range pdfOperaciones {
		if _, ok := web[op]; !ok {
			t.Errorf("%s está en el cliente y no en la web", op)
		}
	}
}

// TestElPDFEnBlancoEsElDeLaWeb: el mismo fichero byte a byte que `pdfEnBlanco` de la web
// (se compara con la cadena que escribe `blanco.ts`), y cada entrada de la xref apunta de
// verdad al principio de su objeto —que es lo que se descuadra solo—.
func TestElPDFEnBlancoEsElDeLaWeb(t *testing.T) {
	b := string(PDFEnBlanco())
	if !strings.HasPrefix(b, "%PDF-1.4\n") || !strings.HasSuffix(b, "%%EOF\n") {
		t.Fatalf("cabecera o pie: %q", b)
	}
	i := strings.Index(b, "xref\n")
	for n, linea := range strings.Split(b[i:], "\n")[3:6] {
		var off int
		if _, err := fmt.Sscanf(linea, "%010d 00000 n ", &off); err != nil {
			t.Fatalf("línea %q: %v", linea, err)
		}
		if quiero := fmt.Sprintf("%d 0 obj", n+1); !strings.HasPrefix(b[off:], quiero) {
			t.Errorf("la xref del objeto %d apunta a %q", n+1, b[off:off+10])
		}
	}
	var sx int
	if _, err := fmt.Sscanf(b[strings.Index(b, "startxref\n")+10:], "%d", &sx); err != nil || sx != i {
		t.Errorf("startxref = %d, la xref está en %d", sx, i)
	}
	fuente := webref.Read(t, "apps/frontend/packages/office/src/pdf/blanco.ts")
	for _, l := range []string{"const A4 = [595.28, 841.89]", "'<< /Type /Catalog /Pages 2 0 R >>'", "'%PDF-1.4\\n'", "/Resources << >> >>"} {
		if !strings.Contains(fuente, l) {
			t.Errorf("blanco.ts ya no dice %s: revisar PDFEnBlanco", l)
		}
	}
}

// TestEscanearEsUnSoloPDF: las fotos viajan en UNA petición, como `hoja-N`, con
// `op=imagenes-a-pdf`, y el resultado se guarda en «PDF» con el nombre que se da.
func TestEscanearEsUnSoloPDF(t *testing.T) {
	d := &driveDePrueba{t: t, conCarpeta: true}
	srv := httptest.NewServer(d)
	defer srv.Close()
	p, ctx := NewPDF(srv.URL, srv.URL, nil, nil), context.Background()
	fotos := []PDFFichero{{Nombre: "IMG_1.jpg", Datos: []byte("a")}, {Nombre: "IMG_2.jpg", Datos: []byte("b")}}
	f, err := p.Escanear(ctx, fotos, "escaneo")
	if err != nil || d.motor.ruta != "/api/v1/pdf/tools" || d.motor.campos["op"] != "imagenes-a-pdf" ||
		strings.Join(d.motor.ficheros, "|") != "hoja-1=a|hoja-2=b" || d.subido.nombre != "escaneo.pdf" || f.ID != "nuevo" {
		t.Fatalf("Escanear = %+v, %v; motor %+v, subido %+v", f, err, d.motor, d.subido)
	}
	if _, err := p.Escanear(ctx, nil, "x"); err == nil {
		t.Error("un escaneo sin fotos no se manda")
	}
}

// TestCombinarTraeLasRutasEnSuOrden es el bucle de la combinación de `pdf-aplicar.ts`:
// detrás del actual, cada ruta en el orden escrito; la imagen, convertida antes en PDF
// (`op=imagenes-a-pdf`); las líneas vacías y el propio documento, fuera. Y con más de
// veinte ficheros no se descarga ninguno.
func TestCombinarTraeLasRutasEnSuOrden(t *testing.T) {
	d := &driveDePrueba{t: t, conCarpeta: true}
	srv := httptest.NewServer(d)
	defer srv.Close()
	p, ctx := NewPDF(srv.URL, srv.URL, nil, nil), context.Background()
	original := File{ID: "f2", FolderID: "cpdf", Name: "Contrato.PDF"}
	doc := PDFFichero{Nombre: "Contrato.PDF", Datos: []byte("%PDF-actual")}
	_, err := p.Aplicar(ctx, original, doc, "", PDFCambio{
		Op: "paginas", Campos: map[string]string{"op": "combinar"}, Sufijo: "combinado",
		Rutas: []string{"/Fotos/recibo.JPG", "  ", "/PDF/Contrato.PDF", "/PDF/viejo.pdf"},
	})
	if err != nil {
		t.Fatal(err)
	}
	quiero := []string{"Contrato.PDF=%PDF-actual", "recibo.JPG.pdf=%PDF-resultado", "viejo.pdf=%PDF-1.7 hola"}
	if d.motor.ruta != "/api/v1/pdf/pages" || !slices.Equal(d.motor.ficheros, quiero) {
		t.Errorf("al motor le llega %s %v; quiero %v", d.motor.ruta, d.motor.ficheros, quiero)
	}
	muchas := make([]string, PDFMaxCombinar+1)
	for i := range muchas {
		muchas[i] = "/PDF/viejo.pdf"
	}
	if _, err := p.Aplicar(ctx, original, doc, "", PDFCambio{Op: "paginas", Rutas: muchas}); !errors.Is(err, ErrPDFDemasiados) {
		t.Errorf("con %d ficheros devuelve %v", len(muchas), err)
	}
	if _, err := p.Aplicar(ctx, original, doc, "", PDFCambio{Op: "paginas", Rutas: []string{"/PDF/no-esta.pdf"}}); !errors.Is(err, ErrPDFDescarga) || !strings.Contains(err.Error(), "/PDF/no-esta.pdf") {
		t.Errorf("uno que no está devuelve %v; tiene que decir cuál", err)
	}
}
