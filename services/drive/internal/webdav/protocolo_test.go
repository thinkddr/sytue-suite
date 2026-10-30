// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package webdav

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/service"
)

// driveFalso es un Drive de una sola carpeta en memoria. Lo que se prueba aquí es
// el protocolo: el manejador es el de verdad, el de la biblioteca.
type driveFalso struct {
	ficheros  map[string][]byte // nombre → contenido
	aperturas int               // veces que se abrió un blob
	listados  int               // veces que se listó una carpeta
}

func (d *driveFalso) fichero(nombre string) domain.File {
	return domain.File{Name: nombre, SizeBytes: int64(len(d.ficheros[nombre])), ContentType: "application/x-de-drive"}
}

func (d *driveFalso) ResolvePath(_ context.Context, p string) (service.Node, error) {
	p = path.Clean("/" + p)
	if p == "/" {
		return service.Node{Folder: &domain.Folder{IsRoot: true}}, nil
	}
	if _, ok := d.ficheros[p[1:]]; !ok {
		return service.Node{}, domain.ErrNotFound
	}
	f := d.fichero(p[1:])
	return service.Node{File: &f}, nil
}

func (d *driveFalso) ListChildren(context.Context, uuid.UUID) ([]domain.Folder, []domain.File, error) {
	d.listados++
	fs := make([]domain.File, 0, len(d.ficheros))
	for nombre := range d.ficheros {
		fs = append(fs, d.fichero(nombre))
	}
	return nil, fs, nil
}

func (d *driveFalso) OpenContentByFile(_ context.Context, f domain.File) (io.ReadCloser, error) {
	d.aperturas++
	return io.NopCloser(bytes.NewReader(d.ficheros[f.Name])), nil
}

func (d *driveFalso) WritePath(_ context.Context, p, _ string, body io.Reader) (domain.File, error) {
	b, err := io.ReadAll(body)
	if err != nil {
		return domain.File{}, err
	}
	nombre := path.Base(p)
	d.ficheros[nombre] = b
	return d.fichero(nombre), nil
}

func (d *driveFalso) MkdirPath(context.Context, string) error { return domain.ErrPermission }
func (d *driveFalso) RemovePath(_ context.Context, p string) error {
	delete(d.ficheros, path.Base(p))
	return nil
}

func (d *driveFalso) RenamePath(_ context.Context, de, a string) error {
	d.ficheros[path.Base(a)] = d.ficheros[path.Base(de)]
	delete(d.ficheros, path.Base(de))
	return nil
}

func pedir(h http.Handler, metodo, ruta string, cuerpo io.Reader, cabeceras ...string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(metodo, ruta, cuerpo)
	for i := 0; i+1 < len(cabeceras); i += 2 {
		r.Header.Set(cabeceras[i], cabeceras[i+1])
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

// Descargar un fichero por WebDAV daba 500: la biblioteca pregunta el tamaño
// yendo al final del fichero, y el lector no sabía ir.
func TestGETDeUnFichero(t *testing.T) {
	// Más largo que lo que la biblioteca lee para deducir el tipo (512 bytes).
	largo := strings.Repeat("0123456789", 200)
	d := &driveFalso{ficheros: map[string][]byte{
		"nota.txt":    []byte("hola, mundo"),
		"pagina.html": []byte("<script>alert(1)</script>"),
		// Una extensión que no conoce nadie: la biblioteca lee el principio y rebobina.
		"datos.xyzzy": []byte(largo),
	}}
	h := manejador("/dav", &FileSystem{svc: d})

	for nombre, contenido := range d.ficheros {
		rec := pedir(h, http.MethodGet, "/dav/"+nombre, http.NoBody)
		if rec.Code != http.StatusOK || rec.Body.String() != string(contenido) {
			t.Errorf("GET %s = %d con %d bytes, esperaba 200 con %d", nombre, rec.Code, rec.Body.Len(), len(contenido))
		}
		// De extremo a extremo, lo que #524 solo pudo probar en el envoltorio.
		if got := rec.Header().Get("Content-Disposition"); got != "attachment" {
			t.Errorf("GET %s: Content-Disposition = %q", nombre, got)
		}
		if got := rec.Header().Get("Content-Security-Policy"); got != "sandbox" {
			t.Errorf("GET %s: Content-Security-Policy = %q", nombre, got)
		}
	}

	rec := pedir(h, http.MethodHead, "/dav/nota.txt", http.NoBody)
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Length") != "11" || rec.Body.Len() != 0 {
		t.Errorf("HEAD = %d, Content-Length %q, cuerpo de %d bytes", rec.Code, rec.Header().Get("Content-Length"), rec.Body.Len())
	}

	// Reanudar una descarga: un tramo del medio y la cola.
	for rango, quiero := range map[string]string{
		"bytes=1005-1014": "5678901234",
		"bytes=-7":        "3456789",
	} {
		rec = pedir(h, http.MethodGet, "/dav/datos.xyzzy", http.NoBody, "Range", rango)
		if rec.Code != http.StatusPartialContent || rec.Body.String() != quiero {
			t.Errorf("GET con Range %s = %d %q, esperaba 206 %q", rango, rec.Code, rec.Body.String(), quiero)
		}
	}

	if rec = pedir(h, http.MethodGet, "/dav/no-existe.txt", http.NoBody); rec.Code != http.StatusNotFound {
		t.Errorf("GET de lo que no existe = %d", rec.Code)
	}
	// Un fichero conocido por su extensión se abre una vez; no se reabre para nada.
	d.aperturas = 0
	pedir(h, http.MethodGet, "/dav/nota.txt", http.NoBody)
	if d.aperturas != 1 {
		t.Errorf("un GET abrió el blob %d veces", d.aperturas)
	}
}

// Listar una carpeta no descarga nada: el tipo de cada fichero es el que Drive
// tiene guardado. Antes se abría cada fichero, y con una extensión desconocida el
// listado entero fallaba.
func TestPROPFINDNoAbreLosFicheros(t *testing.T) {
	d := &driveFalso{ficheros: map[string][]byte{
		"nota.txt":    []byte("hola"),
		"datos.xyzzy": []byte(strings.Repeat("x", 2000)),
	}}
	h := manejador("/dav", &FileSystem{svc: d})

	rec := pedir(h, "PROPFIND", "/dav/", http.NoBody, "Depth", "1")
	if rec.Code != http.StatusMultiStatus {
		t.Fatalf("PROPFIND = %d: %s", rec.Code, rec.Body.String())
	}
	cuerpo := rec.Body.String()
	for nombre := range d.ficheros {
		if !strings.Contains(cuerpo, "/dav/"+nombre) {
			t.Errorf("el listado no trae %s", nombre)
		}
	}
	if n := strings.Count(cuerpo, "application/x-de-drive"); n != len(d.ficheros) {
		t.Errorf("el tipo guardado en Drive sale %d veces, esperaba %d:\n%s", n, len(d.ficheros), cuerpo)
	}
	if d.aperturas != 0 || d.listados != 1 {
		t.Errorf("listar abrió %d blobs y listó %d veces, esperaba 0 y 1", d.aperturas, d.listados)
	}
}

func TestPUTYCOPYLleganADrive(t *testing.T) {
	d := &driveFalso{ficheros: map[string][]byte{}}
	h := manejador("/dav", &FileSystem{svc: d})

	if rec := pedir(h, http.MethodPut, "/dav/nuevo.txt", strings.NewReader("contenido")); rec.Code != http.StatusCreated {
		t.Fatalf("PUT = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := pedir(h, "COPY", "/dav/nuevo.txt", http.NoBody, "Destination", "/dav/copia.txt"); rec.Code != http.StatusCreated {
		t.Fatalf("COPY = %d: %s", rec.Code, rec.Body.String())
	}
	for _, nombre := range []string{"nuevo.txt", "copia.txt"} {
		if got := string(d.ficheros[nombre]); got != "contenido" {
			t.Errorf("%s = %q", nombre, got)
		}
	}
}

// pedirComo es pedir con la organización de la sesión en el contexto.
func pedirComo(h http.Handler, org, metodo, ruta string, cuerpo io.Reader, cabeceras ...string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(metodo, ruta, cuerpo)
	r = r.WithContext(tenancy.Into(r.Context(), tenancy.Tenant{Slug: org}))
	for i := 0; i+1 < len(cabeceras); i += 2 {
		r.Header.Set(cabeceras[i], cabeceras[i+1])
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

// FILE-05. Las rutas de WebDAV son las mismas cadenas en todas las organizaciones, y
// había un solo sistema de bloqueos: un LOCK de «/» en una dejaba a todas las demás sin
// poder escribir (423), y sin Timeout no caducaba nunca.
func TestUnBloqueoNoCruzaOrganizaciones(t *testing.T) {
	d := &driveFalso{ficheros: map[string][]byte{}}
	h := manejador("/dav", &FileSystem{svc: d})
	const lockinfo = `<?xml version="1.0" encoding="utf-8"?><D:lockinfo xmlns:D="DAV:"><D:lockscope><D:exclusive/></D:lockscope><D:locktype><D:write/></D:locktype><D:owner>acme</D:owner></D:lockinfo>`

	rec := pedirComo(h, "acme", "LOCK", "/dav/", strings.NewReader(lockinfo), "Depth", "infinity")
	if rec.Code != http.StatusOK {
		t.Fatalf("LOCK = %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Second-3600") {
		t.Errorf("un LOCK sin Timeout tiene que acotarse a una hora y decirlo: %s", rec.Body.String())
	}
	if rec := pedirComo(h, "otra", http.MethodPut, "/dav/nuevo.txt", strings.NewReader("x")); rec.Code != http.StatusCreated {
		t.Errorf("el bloqueo de acme dejó a otra organización con %d al escribir", rec.Code)
	}
	if rec := pedirComo(h, "acme", http.MethodPut, "/dav/nuevo.txt", strings.NewReader("x")); rec.Code != http.StatusLocked {
		t.Errorf("dentro de acme, escribir sin el token del bloqueo = %d, quiero 423", rec.Code)
	}
	// Lo que pide el cliente se respeta si es menos de una hora.
	rec = pedirComo(h, "otra", "LOCK", "/dav/corto.txt", strings.NewReader(lockinfo), "Timeout", "Second-60")
	if !strings.Contains(rec.Body.String(), "Second-60") {
		t.Errorf("un LOCK de 60 s: %s", rec.Body.String())
	}

	// Tampoco cruza entre dos personas de la MISMA organización («Mi unidad» es privada).
	pedirPersona := func(sub, metodo, ruta string, cuerpo io.Reader, cabeceras ...string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(metodo, ruta, cuerpo)
		ctx := tenancy.Into(r.Context(), tenancy.Tenant{Slug: "compartida"})
		ctx = authn.Into(ctx, authn.Claims{Subject: sub, Tenant: "compartida"})
		r = r.WithContext(ctx)
		for i := 0; i+1 < len(cabeceras); i += 2 {
			r.Header.Set(cabeceras[i], cabeceras[i+1])
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if rec := pedirPersona("alice", "LOCK", "/dav/", strings.NewReader(lockinfo), "Depth", "infinity"); rec.Code != http.StatusOK {
		t.Fatalf("LOCK de alice = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := pedirPersona("bob", http.MethodPut, "/dav/de-bob.txt", strings.NewReader("x")); rec.Code != http.StatusCreated {
		t.Errorf("el bloqueo de alice dejó a bob (misma org) con %d al escribir", rec.Code)
	}
	if rec := pedirPersona("alice", http.MethodPut, "/dav/de-alice.txt", strings.NewReader("x")); rec.Code != http.StatusLocked {
		t.Errorf("dentro de la unidad de alice, escribir sin el token del bloqueo = %d, quiero 423", rec.Code)
	}
}

// driveQueLee cuenta lo que WritePath va leyendo, o falla sin leer nada.
type driveQueLee struct {
	*driveFalso
	mu    sync.Mutex
	leido int
	falla bool
}

func (d *driveQueLee) WritePath(_ context.Context, p, _ string, body io.Reader) (domain.File, error) {
	if d.falla {
		return domain.File{}, domain.ErrPermission
	}
	buf := make([]byte, 64)
	for {
		n, err := body.Read(buf)
		d.mu.Lock()
		d.leido += n
		d.mu.Unlock()
		if err == io.EOF {
			return domain.File{Name: path.Base(p)}, nil
		}
		if err != nil {
			return domain.File{}, err
		}
	}
}

func (d *driveQueLee) leidos() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.leido
}

// FILE-06. Lo que se escribe por WebDAV llega a Drive mientras se escribe, no al cerrar:
// antes se juntaba el fichero entero en memoria (con un tope de cuerpo de gigas).
func TestLoQueSeEscribeLlegaADriveSinEsperarAlFinal(t *testing.T) {
	ctx := context.Background()
	d := &driveQueLee{driveFalso: &driveFalso{ficheros: map[string][]byte{}}}
	fs := &FileSystem{svc: d}
	f, err := fs.OpenFile(ctx, "/grande.bin", os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(bytes.Repeat([]byte("x"), 1000)); err != nil {
		t.Fatal(err)
	}
	for i := 0; d.leidos() < 1000 && i < 200; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if n := d.leidos(); n != 1000 {
		t.Fatalf("antes de cerrar Drive ha recibido %d bytes de 1000", n)
	}
	if fi, _ := f.Stat(); fi.Size() != 1000 {
		t.Errorf("Stat antes de cerrar = %d bytes", fi.Size())
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	// Si Drive falla sin leer, escribir falla en vez de quedarse esperando.
	d.falla = true
	f, _ = fs.OpenFile(ctx, "/otro.bin", os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o644)
	hecho := make(chan error, 1)
	go func() { _, err := f.Write(bytes.Repeat([]byte("x"), 1<<20)); hecho <- err }()
	select {
	case err := <-hecho:
		if err == nil {
			t.Error("escribir sobre una subida que ha fallado no dio error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("escribir se quedó colgado tras fallar la subida")
	}
	if err := f.Close(); err == nil {
		t.Error("cerrar una subida que ha fallado no dio error")
	}
}

// Un PUT sin cuerpo crea el fichero igual.
func TestUnPUTVacioCreaElFichero(t *testing.T) {
	d := &driveFalso{ficheros: map[string][]byte{}}
	h := manejador("/dav", &FileSystem{svc: d})
	if rec := pedir(h, http.MethodPut, "/dav/vacio.txt", http.NoBody); rec.Code != http.StatusCreated {
		t.Fatalf("PUT vacío = %d", rec.Code)
	}
	if _, ok := d.ficheros["vacio.txt"]; !ok {
		t.Fatal("el PUT vacío no creó el fichero")
	}
}

// Clientes reales (Windows WebClient, Finder, Office): PROPPATCH no debe abrir el
// fichero como writeFile (truncándolo a 0 bytes), MOVE sin cabecera Overwrite
// sobrescribe por defecto (RFC 4918 §9.9.3) y la cabecera If con ruta sin esquema
// ni host (</dav/...>) desbloquea igual que con URL absoluta.
func TestPROPPATCHNoTruncaFicheroYCabecerasClientesReales(t *testing.T) {
	d := &driveFalso{ficheros: map[string][]byte{
		"informe.docx": []byte("contenido intacto"),
		"temp.tmp":     []byte("version nueva"),
	}}
	h := manejador("/dav", &FileSystem{svc: d})

	const proppatchBody = `<?xml version="1.0" encoding="utf-8" ?>
<D:propertyupdate xmlns:D="DAV:" xmlns:Z="urn:schemas-microsoft-com:">
  <D:set><D:prop><Z:Win32LastModifiedTime>Thu, 29 Oct 2026 10:00:00 GMT</Z:Win32LastModifiedTime></D:prop></D:set>
</D:propertyupdate>`
	rec := pedirComo(h, "acme", "PROPPATCH", "/dav/informe.docx", strings.NewReader(proppatchBody))
	if rec.Code != http.StatusMultiStatus {
		t.Fatalf("PROPPATCH = %d: %s", rec.Code, rec.Body.String())
	}
	if got := string(d.ficheros["informe.docx"]); got != "contenido intacto" {
		t.Fatalf("PROPPATCH truncó el fichero: %q", got)
	}

	// Bloquear informe.docx y escribir enviando If con ruta relativa (Windows WebClient / Office).
	const lockinfo = `<?xml version="1.0" encoding="utf-8"?><D:lockinfo xmlns:D="DAV:"><D:lockscope><D:exclusive/></D:lockscope><D:locktype><D:write/></D:locktype><D:owner>office</D:owner></D:lockinfo>`
	recLock := pedirComo(h, "acme", "LOCK", "/dav/informe.docx", strings.NewReader(lockinfo), "Depth", "0")
	if recLock.Code != http.StatusOK {
		t.Fatalf("LOCK = %d: %s", recLock.Code, recLock.Body.String())
	}
	token := strings.Trim(recLock.Header().Get("Lock-Token"), "<>")
	if token == "" {
		t.Fatalf("LOCK sin Lock-Token: %v", recLock.Header())
	}

	recPut := pedirComo(h, "acme", http.MethodPut, "/dav/informe.docx", strings.NewReader("editado con lock"),
		"If", "</dav/informe.docx> (<"+token+">)")
	if recPut.Code != http.StatusNoContent && recPut.Code != http.StatusCreated {
		t.Fatalf("PUT con If de ruta relativa = %d: %s", recPut.Code, recPut.Body.String())
	}

	// Desbloquear y probar MOVE sin cabecera Overwrite sobre un destino existente (Finder / Office).
	pedirComo(h, "acme", "UNLOCK", "/dav/informe.docx", http.NoBody, "Lock-Token", "<"+token+">")
	recMove := pedirComo(h, "acme", "MOVE", "/dav/temp.tmp", http.NoBody, "Destination", "/dav/informe.docx")
	if recMove.Code != http.StatusNoContent && recMove.Code != http.StatusCreated {
		t.Fatalf("MOVE sin Overwrite sobre destino existente = %d: %s", recMove.Code, recMove.Body.String())
	}
	if got := string(d.ficheros["informe.docx"]); got != "version nueva" {
		t.Fatalf("tras MOVE esperaba 'version nueva', obtuve %q", got)
	}
}
