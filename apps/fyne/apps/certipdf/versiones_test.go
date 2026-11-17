// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
)

// servicioDeVersiones finge Drive con /PDF/a.pdf y su historial, y el motor justo para
// volver a leer el documento tras restaurar. `restaurado` es lo que se sirve como
// contenido actual después de restaurar; `denegar`, que el servicio diga 403.
type servicioDeVersiones struct {
	mu         sync.Mutex
	vistas     []string
	restaurado bool
	denegar    bool
	// sinContenido: bajar el fichero falla, como si la red cayera tras restaurar.
	sinContenido bool
}

func (s *servicioDeVersiones) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.vistas = append(s.vistas, r.Method+" "+r.URL.Path)
	switch r.Method + " " + r.URL.Path {
	case "GET /api/v1/folders/root":
		_, _ = io.WriteString(w, `{"id":"raiz"}`)
	case "GET /api/v1/folders/raiz/children":
		_, _ = io.WriteString(w, `{"folders":[{"id":"cpdf","name":"PDF"}],"files":[]}`)
	case "GET /api/v1/folders/cpdf/children":
		_, _ = io.WriteString(w, `{"folders":[],"files":[{"id":"f9","name":"a.pdf","version":4}]}`)
	case "GET /api/v1/files/f9/versions":
		_, _ = io.WriteString(w, `{"versions":[
			{"version":3,"size_bytes":3000,"is_current":true,"created_at":"2026-11-10T10:00:00Z"},
			{"version":2,"size_bytes":2000,"author_sub":"ana","created_at":"2026-11-09T10:00:00Z"}]}`)
	case "GET /api/v1/files/f9/versions/2/content":
		_, _ = io.WriteString(w, "%PDF-1.7 la v2")
	case "POST /api/v1/files/f9/versions/2/restore":
		if s.denegar {
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"error":{"code":"forbidden","message":"x"}}`)
			return
		}
		s.restaurado = true
		_, _ = io.WriteString(w, `{"id":"f9","name":"a.pdf"}`)
	case "GET /api/v1/files/f9/content":
		if s.sinContenido {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		if s.restaurado {
			_, _ = io.WriteString(w, "%PDF-1.7 restaurado")
			return
		}
		_, _ = io.WriteString(w, "%PDF-1.7 actual")
	case "POST /api/v1/pdf/content":
		_, _ = io.WriteString(w, `{"paginas":[{"numero":1,"ancho":100,"alto":100,"caja":[0,0,100,100]}],"total":1,"permisos":{},"huella":{}}`)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (s *servicioDeVersiones) vio(peticion string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, v := range s.vistas {
		if v == peticion {
			return true
		}
	}
	return false
}

// abrirVersiones abre el diálogo sobre /PDF/a.pdf del servicio y espera a la lista.
func abrirVersiones(t *testing.T, s *servicioDeVersiones) (*Pantalla, *dialogoVersiones, *Documento) {
	t.Helper()
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	p, _, _ := dialogoDePrueba(t, DialogoProteger)
	doc := &Documento{
		pdf: api.NewPDF(srv.URL, srv.URL, nil, nil), ruta: "/PDF/a.pdf",
		fichero: api.File{ID: "f9", Name: "a.pdf"}, datos: []byte("%PDF-1.7 actual"),
	}
	var d *dialogoVersiones
	fyne.DoAndWait(func() {
		p.dialogo.cerrar()
		p.doc = doc
		p.abrirDialogo(DialogoVersiones)
		d = nuevoDialogoVersiones(p.dialogo)
		p.dialogo.pintar = d.vista
	})
	hasta(t, func() bool { return d.cargadas && len(d.lista) == 2 })
	return p, d, doc
}

// TestVersionesListaYRestaura: la cinta lleva «Versiones», el diálogo enseña el
// historial con la actual primero y restaurar pide la versión a Drive, vuelve a traer
// el documento —lo que hay en memoria es lo que se deja— y lo dice en la franja.
func TestVersionesListaYRestaura(t *testing.T) {
	s := &servicioDeVersiones{}
	p, d, doc := abrirVersiones(t, s)
	if !d.lista[0].IsCurrent || d.lista[1].Version != 2 || d.lista[1].AuthorSub != "ana" {
		t.Fatalf("lista = %+v", d.lista)
	}
	fyne.DoAndWait(func() { d.restaurar(2) })
	hasta(t, func() bool { return p.dialogo == nil })
	if !s.vio("POST /api/v1/files/f9/versions/2/restore") {
		t.Fatal("no se restauró en Drive")
	}
	// El visor vuelve a leer el documento, y eso lo trae de Drive otra vez.
	hasta(t, func() bool {
		datos, _ := doc.paraElMotor()
		return string(datos.Datos) == "%PDF-1.7 restaurado"
	})
	var aviso string
	fyne.DoAndWait(func() { aviso = p.Aviso })
	if aviso != "Versión 2 restaurada. El estado anterior sigue en la lista de versiones." {
		t.Errorf("aviso = %q", aviso)
	}
}

// TestVersionesRestauradaSinRedAlRecargar: si Drive restaura pero la red cae antes de
// volver a bajar el documento, no se dice que no se restauró —sí se hizo—: el diálogo se
// cierra, el visor lo cuenta y lo siguiente que lea el documento lo vuelve a pedir.
func TestVersionesRestauradaSinRedAlRecargar(t *testing.T) {
	s := &servicioDeVersiones{sinContenido: true}
	p, d, doc := abrirVersiones(t, s)
	fyne.DoAndWait(func() { d.restaurar(2) })
	hasta(t, func() bool { return p.dialogo == nil })
	if !s.vio("POST /api/v1/files/f9/versions/2/restore") {
		t.Fatal("no se restauró en Drive")
	}
	var aviso, fallo string
	hasta(t, func() bool {
		fyne.DoAndWait(func() { aviso, fallo = p.Aviso, p.Error })
		return fallo != ""
	})
	if aviso != "Versión 2 restaurada. El estado anterior sigue en la lista de versiones." {
		t.Errorf("aviso = %q", aviso)
	}
	s.mu.Lock()
	s.sinContenido = false
	s.mu.Unlock()
	fyne.DoAndWait(func() { p.Reintentar() })
	hasta(t, func() bool {
		datos, _ := doc.paraElMotor()
		return string(datos.Datos) == "%PDF-1.7 restaurado"
	})
}

// TestVersionesSinPermisoNoRecarga: quien solo puede ver el PDF lee por qué no se
// puede, y el documento no se toca.
func TestVersionesSinPermisoNoRecarga(t *testing.T) {
	s := &servicioDeVersiones{denegar: true}
	p, d, doc := abrirVersiones(t, s)
	fyne.DoAndWait(func() { d.restaurar(2) })
	hasta(t, func() bool { return d.restaurando == 0 && d.error != "" })
	if d.error != "Solo puedes ver este PDF, así que no puedes restaurar sus versiones." {
		t.Errorf("error = %q", d.error)
	}
	if p.dialogo == nil {
		t.Error("el diálogo se cerró tras un fallo")
	}
	if datos, _ := doc.paraElMotor(); string(datos.Datos) != "%PDF-1.7 actual" {
		t.Errorf("el documento cambió: %q", datos.Datos)
	}
}

// TestVersionesConFirmasAvisa: con firmas, la pregunta avisa de que la versión puede
// no llevar la última; sin ellas no se habla de firmas.
func TestVersionesConFirmasAvisa(t *testing.T) {
	p, d, _ := abrirVersiones(t, &servicioDeVersiones{})
	v := d.lista[1]
	var sin, con string
	fyne.DoAndWait(func() {
		p.Contenido.Firmas = nil
		sin = d.mensajeRestaurar(v)
		p.Contenido.Firmas = []FirmaLeida{{Nombre: "Grace"}}
		con = d.mensajeRestaurar(v)
	})
	firmas := "Este PDF está firmado"
	if strings.Contains(sin, firmas) || !strings.HasPrefix(sin, "El documento quedará exactamente") {
		t.Errorf("sin firmas: %q", sin)
	}
	if !strings.HasPrefix(con, sin) || !strings.Contains(con, firmas) {
		t.Errorf("con firmas: %q", con)
	}
}

// TestVersionesAbrirLaDejaEnDescargas: «Abrir» deja la versión con su número en el
// nombre y la abre con el programa del sistema, sin tocar el documento.
func TestVersionesAbrirLaDejaEnDescargas(t *testing.T) {
	dir := t.TempDir()
	carpetaDescargas = func() (string, error) { return dir, nil }
	defer func() { carpetaDescargas = ficheros.Carpeta }()
	var mu sync.Mutex
	abierta := ""
	abrirConElSistema = func(ruta string) error { mu.Lock(); abierta = ruta; mu.Unlock(); return nil }
	defer func() { abrirConElSistema = ficheros.Abrir }()

	_, d, doc := abrirVersiones(t, &servicioDeVersiones{})
	fyne.DoAndWait(func() { d.abrir(d.lista[1]) })
	hasta(t, func() bool { mu.Lock(); defer mu.Unlock(); return abierta != "" })
	if filepath.Base(abierta) != "a (v2).pdf" {
		t.Fatalf("abrió %q", abierta)
	}
	if b, _ := os.ReadFile(abierta); string(b) != "%PDF-1.7 la v2" {
		t.Errorf("contenido = %q", b)
	}
	if datos, _ := doc.paraElMotor(); string(datos.Datos) != "%PDF-1.7 actual" {
		t.Errorf("abrir cambió el documento: %q", datos.Datos)
	}
}
