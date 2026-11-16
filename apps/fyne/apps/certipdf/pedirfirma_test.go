// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// servicioDeFirmas finge Drive (la carpeta «PDF» con a.pdf) y las peticiones de firma
// del servicio de Docs, y apunta lo que le llega.
type servicioDeFirmas struct {
	mu     sync.Mutex
	vistas []string
	cuerpo map[string]any
}

func (s *servicioDeFirmas) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.vistas = append(s.vistas, r.Method+" "+r.URL.Path)
	switch r.Method + " " + r.URL.Path {
	case "GET /api/v1/folders/root":
		_, _ = io.WriteString(w, `{"id":"raiz"}`)
	case "GET /api/v1/folders/raiz/children":
		_, _ = io.WriteString(w, `{"folders":[{"id":"cpdf","name":"PDF"}],"files":[]}`)
	case "GET /api/v1/folders/cpdf/children":
		_, _ = io.WriteString(w, `{"folders":[],"files":[{"id":"f9","name":"a.pdf"}]}`)
	case "GET /api/v1/pdf/signature-requests":
		_, _ = io.WriteString(w, `{"received":[{"id":"p1","status":"pendiente","path":"/PDF/a.pdf","requester_name":"Ana","message":"antes del viernes"}],
			"sent":[{"id":"p2","status":"firmada","path":"/PDF/a.pdf","recipient_email":"irati@acme.eu","signed_path":"/PDF/a (firmado).pdf"}]}`)
	case "POST /api/v1/pdf/signature-requests", "POST /api/v1/pdf/signature-requests/p1/decline",
		"POST /api/v1/pdf/signature-requests/p1/sign":
		s.cuerpo = nil
		_ = json.NewDecoder(r.Body).Decode(&s.cuerpo)
		if s.cuerpo["email"] == "@leire" && s.cuerpo["confirm"] != "s-leire" {
			w.WriteHeader(http.StatusConflict)
			_, _ = io.WriteString(w, `{"error":{"code":"pdf_signature_recipient_external","message":"x","details":{"name":"Leire","confirm":"s-leire"}}}`)
			return
		}
		_, _ = io.WriteString(w, `{"id":"p3","status":"pendiente"}`)
	case "POST /api/v1/pdf/signature-requests/p5/block", "DELETE /api/v1/pdf/signature-blocks/u-ana":
		w.WriteHeader(http.StatusNoContent)
	case "POST /api/v1/pdf/signature-requests/p4/open":
		_, _ = io.WriteString(w, `{"id":"p4","status":"pendiente","external":true,"name":"b.pdf","path":"/Sytue Docs/b.pdf"}`)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (s *servicioDeFirmas) vio(peticion string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, v := range s.vistas {
		if v == peticion {
			return true
		}
	}
	return false
}

// hasta espera, con el hilo de Fyne, a que `cond` se cumpla.
func hasta(t *testing.T, cond func() bool) {
	t.Helper()
	for range 200 {
		ok := false
		fyne.DoAndWait(func() { ok = cond() })
		if ok {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("no llegó")
}

// TestLaPeticionDeFirmaSaleEnSuFranjaYSeRechaza: al abrir un PDF que alguien me ha
// pedido firmar, la franja lo dice con su mensaje, y «Rechazar» la rechaza.
func TestLaPeticionDeFirmaSaleEnSuFranjaYSeRechaza(t *testing.T) {
	s := &servicioDeFirmas{}
	srv := httptest.NewServer(s)
	defer srv.Close()
	p, _, _ := dialogoDePrueba(t, DialogoProteger)
	fyne.DoAndWait(func() { p.dialogo.cerrar() })
	doc := &Documento{pdf: api.NewPDF(srv.URL, srv.URL, nil, nil), ruta: "/PDF/a.pdf", fichero: api.File{Name: "a.pdf"}}
	fyne.DoAndWait(func() { p.doc = doc; p.buscarPeticion(doc) })
	hasta(t, func() bool { return p.peticion != nil })

	var f *franja
	fyne.DoAndWait(func() {
		if x, ok := p.franjaPeticion().(*franja); ok {
			f = x
		}
	})
	if f == nil || f.texto != "Ana te pide firmar este documento · antes del viernes" || len(f.acciones) != 2 {
		t.Fatalf("franja = %+v", f)
	}
	rechazar, _ := f.acciones[0].(*Accion)
	fyne.DoAndWait(func() { rechazar.Tapped(nil) })
	hasta(t, func() bool { return p.peticion == nil })
	if !s.vio("POST /api/v1/pdf/signature-requests/p1/decline") {
		t.Error("no se rechazó en el servicio")
	}
}

// TestPedirFirmaDesdeElDialogo: el correo y el mensaje van al servicio con el fichero
// de Drive resuelto, y la lista enseña cómo van las ya pedidas de este documento.
func TestPedirFirmaDesdeElDialogo(t *testing.T) {
	s := &servicioDeFirmas{}
	srv := httptest.NewServer(s)
	defer srv.Close()
	p, _, _ := dialogoDePrueba(t, DialogoProteger)
	fyne.DoAndWait(func() {
		p.dialogo.cerrar()
		p.doc = &Documento{pdf: api.NewPDF(srv.URL, srv.URL, nil, nil), ruta: "/PDF/a.pdf", fichero: api.File{Name: "a.pdf"}}
		p.abrirDialogo(DialogoPedirFirma)
	})
	var d *dialogoPedirFirma
	fyne.DoAndWait(func() { d = nuevoDialogoPedirFirma(p.dialogo); p.dialogo.pintar = d.vista })
	hasta(t, func() bool { return d.cargadas && len(d.pedidas) == 1 })
	fyne.DoAndWait(func() {
		d.email.PonerTexto("irati@acme.eu")
		d.mensaje.PonerTexto("antes del viernes")
		d.enviar()
	})
	hasta(t, func() bool { return !d.enviando && d.hecho != "" })
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cuerpo["file_id"] != "f9" || s.cuerpo["email"] != "irati@acme.eu" || s.cuerpo["message"] != "antes del viernes" {
		t.Errorf("cuerpo = %v", s.cuerpo)
	}
}

// TestLoFirmadoFueraSeEntregaAlVolver: el firmado que vuelve de la aplicación oficial
// se abre como otro documento, y al abrirlo se entrega a quien lo pidió.
func TestLoFirmadoFueraSeEntregaAlVolver(t *testing.T) {
	s := &servicioDeFirmas{}
	srv := httptest.NewServer(s)
	defer srv.Close()
	p, _, _ := dialogoDePrueba(t, DialogoProteger)
	fyne.DoAndWait(func() {
		p.dialogo.cerrar()
		p.porEntregar = &api.PDFPeticionFirma{ID: "p1", NombrePide: "Ana"}
		p.Abrir(&Documento{pdf: api.NewPDF(srv.URL, srv.URL, nil, nil), ruta: "/PDF/a.pdf", fichero: api.File{Name: "a.pdf"}})
	})
	hasta(t, func() bool { return s.vio("POST /api/v1/pdf/signature-requests/p1/sign") })
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cuerpo["signed_file_id"] != "f9" || s.cuerpo["signed_path"] != "/PDF/a.pdf" {
		t.Errorf("entregado = %v", s.cuerpo)
	}
}

// TestPedirFirmaAFueraPreguntaAntes (ADR 0057): a alguien de otra organización no se le
// pide nada hasta confirmar que el PDF sale por un enlace de Drive.
func TestPedirFirmaAFueraPreguntaAntes(t *testing.T) {
	s := &servicioDeFirmas{}
	srv := httptest.NewServer(s)
	defer srv.Close()
	p, _, _ := dialogoDePrueba(t, DialogoProteger)
	fyne.DoAndWait(func() {
		p.dialogo.cerrar()
		p.doc = &Documento{pdf: api.NewPDF(srv.URL, srv.URL, nil, nil), ruta: "/PDF/a.pdf", fichero: api.File{Name: "a.pdf"}}
		p.abrirDialogo(DialogoPedirFirma)
	})
	var d *dialogoPedirFirma
	fyne.DoAndWait(func() { d = nuevoDialogoPedirFirma(p.dialogo); p.dialogo.pintar = d.vista })
	fyne.DoAndWait(func() { d.email.PonerTexto("@leire"); d.enviar() })
	hasta(t, func() bool { return !d.enviando && d.fuera == "Leire" })
	if d.hecho != "" || d.error != "" {
		t.Fatalf("sin confirmar ya dice algo: %q %q", d.hecho, d.error)
	}
	fyne.DoAndWait(func() { d.enviar() })
	hasta(t, func() bool { return !d.enviando && d.hecho != "" })
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cuerpo["confirm"] != "s-leire" || d.fuera != "" {
		t.Errorf("cuerpo = %v, fuera = %q", s.cuerpo, d.fuera)
	}
}

// TestLaPortadaTraeLaPeticionDeFuera: la fila de una petición de otra organización trae
// el PDF a mi unidad y lo abre desde allí.
func TestLaPortadaTraeLaPeticionDeFuera(t *testing.T) {
	s := &servicioDeFirmas{}
	srv := httptest.NewServer(s)
	defer srv.Close()
	p, _, _ := dialogoDePrueba(t, DialogoProteger)
	var abierta string
	fyne.DoAndWait(func() {
		p.dialogo.cerrar()
		p.AlAbrirRuta = func(r string) { abierta = r }
		p.MostrarPortada(nil, "es")
		p.abrirPeticion(api.NewPDF(srv.URL, srv.URL, nil, nil), api.PDFPeticionFirma{ID: "p4", Estado: api.PDFFirmaPendiente, Externo: true, Nombre: "b.pdf"}, true)
	})
	hasta(t, func() bool { return abierta != "" })
	if abierta != "/Sytue Docs/b.pdf" || !s.vio("POST /api/v1/pdf/signature-requests/p4/open") {
		t.Errorf("abierta = %q", abierta)
	}
	fyne.DoAndWait(func() {
		p.Portada.Peticiones = &api.PDFPeticionesFirma{Recibidas: []api.PDFPeticionFirma{{ID: "p4", Estado: api.PDFFirmaPendiente, Externo: true, Nombre: "b.pdf"}}}
		if p.Portada.seccionPeticiones() == nil {
			t.Error("sin sección con una pendiente")
		}
		p.Portada.Peticiones = &api.PDFPeticionesFirma{}
		if p.Portada.seccionPeticiones() != nil {
			t.Error("sección sin peticiones")
		}
	})
}

// TestBloquearDesdeLaFranjaYDesbloquear (ADR 0059): en una petición de fuera, la franja
// ofrece «Bloquear», pide confirmarlo en la misma franja y lo hace; y desde la portada
// se quita el bloqueo.
func TestBloquearDesdeLaFranjaYDesbloquear(t *testing.T) {
	s := &servicioDeFirmas{}
	srv := httptest.NewServer(s)
	defer srv.Close()
	p, _, _ := dialogoDePrueba(t, DialogoProteger)
	pdf := api.NewPDF(srv.URL, srv.URL, nil, nil)
	pet := api.PDFPeticionFirma{ID: "p5", Estado: api.PDFFirmaPendiente, Externo: true, NombrePide: "Ana", Ruta: "/Sytue Docs/b.pdf"}
	var f *franja
	fyne.DoAndWait(func() {
		p.dialogo.cerrar()
		p.doc = &Documento{pdf: pdf, ruta: "/Sytue Docs/b.pdf", fichero: api.File{Name: "b.pdf"}}
		p.peticion = &pet
		f, _ = p.franjaPeticion().(*franja)
	})
	if f == nil || len(f.acciones) != 3 {
		t.Fatalf("franja de fuera = %+v", f)
	}
	bloquear, _ := f.acciones[0].(*Accion)
	fyne.DoAndWait(func() { bloquear.Tapped(nil); f, _ = p.franjaPeticion().(*franja) })
	if !p.bloqueando || f == nil || !strings.Contains(f.texto, "Ana") || len(f.acciones) != 2 || s.vio("POST /api/v1/pdf/signature-requests/p5/block") {
		t.Fatalf("sin confirmar: %+v", f)
	}
	confirmar, _ := f.acciones[0].(*Accion)
	fyne.DoAndWait(func() { confirmar.Tapped(nil) })
	hasta(t, func() bool { return p.peticion == nil })
	if !s.vio("POST /api/v1/pdf/signature-requests/p5/block") || p.bloqueando {
		t.Error("no se bloqueó en el servicio")
	}

	fyne.DoAndWait(func() {
		p.AlAbrirRuta = func(string) {}
		p.MostrarPortada(nil, "es")
		p.Portada.Peticiones = &api.PDFPeticionesFirma{Bloqueados: []api.PDFBloqueo{{Sub: "u-ana", Nombre: "Ana"}}}
		if p.Portada.seccionPeticiones() == nil {
			t.Error("sin sección con alguien bloqueado")
		}
		p.desbloquear(pdf, "u-ana")
	})
	hasta(t, func() bool { return len(p.Portada.Peticiones.Bloqueados) == 0 })
	if !s.vio("DELETE /api/v1/pdf/signature-blocks/u-ana") {
		t.Error("no se desbloqueó en el servicio")
	}
}

// deTipo junta, con `todos`, los objetos de tipo T del árbol de `o`, en orden.
func deTipo[T fyne.CanvasObject](o fyne.CanvasObject) []T {
	var out []T
	todos(o, func(x fyne.CanvasObject) {
		if y, ok := x.(T); ok {
			out = append(out, y)
		}
	})
	return out
}

// TestPedirFirmaConLasPiezasDeLaSuite (pieza CERT-07): el diálogo es el de la web nueva
// —campos de la suite en su `FieldGroup`, el aviso de fuera como `InlineHelp` informativo
// y el estado de cada pedida como insignia de su tono—, con las medidas de aquellas.
func TestPedirFirmaConLasPiezasDeLaSuite(t *testing.T) {
	for estado, tono := range map[string]ui.Tono{
		api.PDFFirmaPendiente: ui.InsigniaInfo, api.PDFFirmaFirmada: ui.InsigniaExito,
		api.PDFFirmaRechazada: ui.InsigniaPeligro, api.PDFFirmaCancelada: ui.InsigniaNeutra,
		"en_espera": ui.InsigniaNeutra, "caducada": ui.InsigniaAviso,
	} {
		if got := tonoDeEstado(estado); got != tono {
			t.Errorf("tonoDeEstado(%q) = %v, quiero %v", estado, got, tono)
		}
	}

	s := &servicioDeFirmas{}
	srv := httptest.NewServer(s)
	defer srv.Close()
	p, _, _ := dialogoDePrueba(t, DialogoProteger)
	fyne.DoAndWait(func() {
		p.dialogo.cerrar()
		p.doc = &Documento{pdf: api.NewPDF(srv.URL, srv.URL, nil, nil), ruta: "/PDF/a.pdf", fichero: api.File{Name: "a.pdf"}}
		p.abrirDialogo(DialogoPedirFirma)
	})
	var d *dialogoPedirFirma
	fyne.DoAndWait(func() { d = nuevoDialogoPedirFirma(p.dialogo); p.dialogo.pintar = d.vista })
	hasta(t, func() bool { return d.cargadas && len(d.pedidas) == 1 })

	const ancho float32 = 360
	fyne.DoAndWait(func() {
		v := d.vista(ancho)
		// El correo es `<Input>` md y el mensaje `<Textarea>`, a todo el ancho.
		if got, quiero := d.email.MinSize().Height, ui.AltoCampo(ui.CampoMD); got != quiero {
			t.Errorf("alto del correo = %v, quiero el de Input md %v", got, quiero)
		}
		if d.email.MinSize().Width != ancho || d.mensaje.MinSize().Width != ancho {
			t.Errorf("campos a %v y %v, quiero %v", d.email.MinSize().Width, d.mensaje.MinSize().Width, ancho)
		}
		// La pedida firmada lleva su insignia de éxito y «Abrir el firmado» es un botón
		// fantasma xs.
		ins := deTipo[*ui.Insignia](v)
		if len(ins) != 1 || ins[0].Tono != ui.InsigniaExito || ins[0].Texto != d.a.t("pdf.signatureRequest.status.firmada") {
			t.Fatalf("insignias = %+v", ins)
		}
		var abrir *ui.Boton
		for _, b := range deTipo[*ui.Boton](v) {
			if b.Texto == d.a.t("pdf.dialog.pedirFirma.openSigned") {
				abrir = b
			}
		}
		if abrir == nil || abrir.MinSize() != ui.MedidaBotonDe(abrir) || abrir.MinSize().Height != ui.MedidaBoton(abrir.Texto, ui.TallaXS, 0).Height {
			t.Errorf("«Abrir el firmado» = %+v", abrir)
		}
		if len(deTipo[*ui.AyudaEnLinea](v)) != 0 {
			t.Error("aviso de fuera sin que nadie sea de fuera")
		}
	})

	fyne.DoAndWait(func() { d.email.PonerTexto("@leire"); d.enviar() })
	hasta(t, func() bool { return !d.enviando && d.fuera == "Leire" })
	fyne.DoAndWait(func() {
		avisos := deTipo[*ui.AyudaEnLinea](d.vista(ancho))
		if len(avisos) != 1 {
			t.Fatalf("avisos de fuera = %d", len(avisos))
		}
		a := avisos[0]
		if a.Tono != ui.AyudaInfo || !strings.Contains(a.Titulo, "Leire") || a.Cuerpo != d.a.t("pdf.dialog.pedirFirma.external.help") {
			t.Errorf("aviso = %+v", a)
		}
		// Medido a su ancho desde el principio: con el ancho a cero se repartía palabra
		// a palabra y salía altísimo.
		if got, quiero := a.MinSize().Height, ui.AltoAyudaEnLinea(a, ancho); got != quiero {
			t.Errorf("alto del aviso = %v, quiero %v", got, quiero)
		}
	})
}
