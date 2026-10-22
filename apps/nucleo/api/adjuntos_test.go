// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// driveFalso es un Drive de mentira con UNA carpeta de adjuntos y sus ficheros. Apunta
// a qué carpeta fue cada subida, que es lo que vigila el guardián de permisos.
type driveFalso struct {
	t *testing.T
	// carpetas: nombre en la raíz → id. Vacío = la raíz no tiene ninguna.
	carpetas map[string]string
	// ficheros de la carpeta de adjuntos, en su orden de listado.
	ficheros string
	// subidas apuntadas: a qué carpeta y con qué nombre.
	subidas []struct{ carpeta, nombre string }
}

func (d *driveFalso) servidor() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ruta := r.URL.Path
		switch {
		case ruta == "/api/v1/folders/root":
			_, _ = io.WriteString(w, `{"id":"raiz","is_root":true}`)
		case ruta == "/api/v1/folders/raiz/children":
			carpetas := make([]string, 0, len(d.carpetas))
			for nombre, id := range d.carpetas {
				carpetas = append(carpetas, `{"id":"`+id+`","name":"`+nombre+`"}`)
			}
			_, _ = io.WriteString(w, `{"folders":[`+strings.Join(carpetas, ",")+`],"files":[]}`)
		case ruta == "/api/v1/folders/adj/children":
			_, _ = io.WriteString(w, `{"folders":[],"files":[`+d.ficheros+`]}`)
		case ruta == "/api/v1/folders" && r.Method == http.MethodPost:
			d.carpetas["Attachments"] = "adj"
			_, _ = io.WriteString(w, `{"id":"adj","parent_id":"raiz","name":"Attachments"}`)
		case strings.HasSuffix(ruta, "/files") && r.Method == http.MethodPost:
			carpeta := strings.TrimSuffix(strings.TrimPrefix(ruta, "/api/v1/folders/"), "/files")
			nombre := r.URL.Query().Get("name")
			d.subidas = append(d.subidas, struct{ carpeta, nombre string }{carpeta, nombre})
			// El nombre se serializa DE VERDAD. Pegado a mano, un nombre con `\`
			// —justo el que manda un remitente malicioso— da un JSON inválido y el
			// guardián de más abajo muere en el primer caso en vez de enseñarlos
			// todos. Visto al ponerlo en rojo.
			crudo, _ := json.Marshal(nombre)
			_, _ = io.WriteString(w, `{"id":"nuevo","folder_id":"`+carpeta+`","name":`+string(crudo)+`}`)
		default:
			d.t.Errorf("petición inesperada: %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func nuevoDriveFalso(t *testing.T, carpetas map[string]string, ficheros string) (*Drive, *driveFalso) {
	t.Helper()
	d := &driveFalso{t: t, carpetas: carpetas, ficheros: ficheros}
	srv := d.servidor()
	t.Cleanup(srv.Close)
	return NewDrive(srv.URL, &sesionFalsa{tokens: []string{"tok"}}, nil), d
}

// Guardar un adjunto crea la carpeta la primera vez, sube el contenido y devuelve una
// referencia con la que ABRIR ya funciona: es el punto entero del encargo, así que se
// comprueba hasta la URL que recibe la otra aplicación.
func TestGuardarAdjuntoDaUnaReferenciaQueAbreEnSuApp(t *testing.T) {
	d, falso := nuevoDriveFalso(t, map[string]string{}, "")

	ref, err := d.GuardarAdjunto(context.Background(), "informe.pdf", "application/pdf", []byte("%PDF-1.7"))
	if err != nil {
		t.Fatalf("GuardarAdjunto: %v", err)
	}
	if ref.ID != "nuevo" || ref.Ruta != "/Attachments/informe.pdf" {
		t.Fatalf("referencia = %+v", ref)
	}
	if len(falso.subidas) != 1 || falso.subidas[0].carpeta != "adj" {
		t.Fatalf("subidas = %+v", falso.subidas)
	}

	a := &abridorDoble{falla: func(u string) bool { return strings.HasPrefix(u, "sytue-") }}
	if !AbrirFichero(a, "sytue.eu", "", "informe.pdf", ref) {
		t.Fatal("AbrirFichero = false: la referencia de un adjunto tiene que abrir como la de cualquier fichero")
	}
	quiere := "https://certipdf.sytue.eu/certipdf/pdf?path=%2FAttachments%2Finforme.pdf&volver=%2FAttachments"
	if a.urls[len(a.urls)-1] != quiere {
		t.Fatalf("abrió %q, se esperaba %q", a.urls[len(a.urls)-1], quiere)
	}
}

// Abrir el mismo adjunto dos veces no puede dejar dos ficheros: la segunda vez se
// reconoce por el checksum y no se sube nada.
func TestElMismoAdjuntoDosVecesNoCreaDosFicheros(t *testing.T) {
	contenido := []byte("%PDF-1.7")
	suma := sha256.Sum256(contenido)
	ya := `{"id":"f1","folder_id":"adj","name":"informe.pdf","checksum":"` + hex.EncodeToString(suma[:]) + `"}`
	d, falso := nuevoDriveFalso(t, map[string]string{"Attachments": "adj"}, ya)

	ref, err := d.GuardarAdjunto(context.Background(), "informe.pdf", "application/pdf", contenido)
	if err != nil {
		t.Fatalf("GuardarAdjunto: %v", err)
	}
	if ref.ID != "f1" {
		t.Fatalf("no reutilizó el fichero ya guardado: %+v", ref)
	}
	if len(falso.subidas) != 0 {
		t.Fatalf("subió otra copia: %+v", falso.subidas)
	}
}

// Un adjunto con el MISMO nombre pero otro contenido no se come al que ya estaba: se
// guarda al lado, como hace la web.
func TestDosAdjuntosDistintosConElMismoNombreNoSePisan(t *testing.T) {
	d, falso := nuevoDriveFalso(t, map[string]string{"Attachments": "adj"},
		`{"id":"f1","folder_id":"adj","name":"informe.pdf","checksum":"0a0a"}`)

	ref, err := d.GuardarAdjunto(context.Background(), "informe.pdf", "application/pdf", []byte("otro"))
	if err != nil {
		t.Fatalf("GuardarAdjunto: %v", err)
	}
	if ref.Ruta != "/Attachments/informe (2).pdf" {
		t.Fatalf("ruta = %q", ref.Ruta)
	}
	if len(falso.subidas) != 1 || falso.subidas[0].nombre != "informe (2).pdf" {
		t.Fatalf("subidas = %+v", falso.subidas)
	}
}

// ⛔ EL GUARDIÁN QUE IMPORTA: materializar un adjunto no puede darle más alcance del
// que tenía. El nombre viene de una cabecera MIME —lo escribe QUIEN MANDA EL CORREO—,
// así que si con él se pudiera elegir carpeta, el remitente decidiría dónde acaba el
// fichero y qué ruta viaja después a otra aplicación de la suite. Todo tiene que caer
// dentro de la carpeta de adjuntos y con un nombre que Drive acepte.
func TestUnAdjuntoNoEligeDondeSeGuarda(t *testing.T) {
	malos := []string{
		"../../nomina.pdf",
		"/etc/passwd",
		`..\..\Windows\system32\x.pdf`,
		"Contabilidad/cierre.xlsx",
		"factura\x00.pdf",
		"linea\nrota.pdf",
		"..",
		".",
		"   ",
		"",
		strings.Repeat("á", 400) + ".pdf",
	}
	for _, malo := range malos {
		d, falso := nuevoDriveFalso(t, map[string]string{"Attachments": "adj"}, "")
		ref, err := d.GuardarAdjunto(context.Background(), malo, "application/pdf", []byte("x"))
		if err != nil {
			t.Errorf("%q: %v", malo, err)
			continue
		}
		if len(falso.subidas) != 1 {
			t.Errorf("%q: subidas = %+v", malo, falso.subidas)
			continue
		}
		s := falso.subidas[0]
		if s.carpeta != "adj" {
			t.Errorf("%q: se subió a la carpeta %q, no a la de adjuntos", malo, s.carpeta)
		}
		// Lo que `domain.ValidName` de services/drive acepta. No se puede importar
		// (otro módulo, y `internal`), así que se repite aquí la condición.
		switch {
		case s.nombre == "" || len(s.nombre) > 255:
			t.Errorf("%q: nombre de %d bytes que Drive rechazaría: %q", malo, len(s.nombre), s.nombre)
		case s.nombre == "." || s.nombre == "..":
			t.Errorf("%q: nombre %q", malo, s.nombre)
		case strings.ContainsAny(s.nombre, "/\\\x00"):
			t.Errorf("%q: el nombre lleva separador: %q", malo, s.nombre)
		case strings.TrimSpace(s.nombre) != s.nombre:
			t.Errorf("%q: nombre con espacios en los bordes: %q", malo, s.nombre)
		}
		for _, r := range s.nombre {
			if r < 0x20 {
				t.Errorf("%q: el nombre lleva un carácter de control: %q", malo, s.nombre)
				break
			}
		}
		// Y la ruta que viajaría a la otra aplicación tampoco sale de la carpeta.
		if !strings.HasPrefix(ref.Ruta, "/"+CarpetaAdjuntos+"/") ||
			strings.Contains(strings.TrimPrefix(ref.Ruta, "/"+CarpetaAdjuntos+"/"), "/") {
			t.Errorf("%q: la referencia apunta fuera de la carpeta: %q", malo, ref.Ruta)
		}
	}
}

// La carpeta destino es la MISMA que la de la web, o la persona acabaría con los
// adjuntos repartidos en dos sitios según desde dónde los guarde.
func TestLaCarpetaDeAdjuntosEsLaDeLaWeb(t *testing.T) {
	fuente := webref.Read(t, "apps/frontend/apps/mail/src/pages/api/attachment-to-drive.ts")
	if !strings.Contains(fuente, "const FOLDER = '/"+CarpetaAdjuntos+"';") {
		t.Fatalf("la web ya no guarda en /%s: revisar el espejo", CarpetaAdjuntos)
	}
}
