// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/certificados"
)

// almacenDePrueba es «la tarjeta»: un certificado y su clave, que firma el resumen.
type almacenDePrueba struct {
	cert *x509.Certificate
	k    *ecdsa.PrivateKey
	pins []string
}

func (a *almacenDePrueba) Certificados(context.Context) ([]certificados.Certificado, error) {
	return []certificados.Certificado{{Cadena: []*x509.Certificate{a.cert}, Almacen: "Tarjeta de prueba", PidePIN: true}}, nil
}

func (a *almacenDePrueba) Firmar(_ context.Context, _ certificados.Certificado, prep api.PDFFirmaPreparada, pin string) ([]byte, string, error) {
	a.pins = append(a.pins, pin)
	if pin != "1234" {
		return nil, "", certificados.ErrPIN
	}
	f, err := ecdsa.SignASN1(rand.Reader, a.k, prep.Resumen)
	return f, api.PDFFirmaDER, err
}

// motorDeFirmaDePrueba finge el servicio de Docs (los dos pasos, con la autoridad caída
// la primera vez) y Drive (donde se guarda la copia), y apunta lo que le llega.
type motorDeFirmaDePrueba struct {
	mu       sync.Mutex
	completa []map[string]string
	subido   string
}

func (m *motorDeFirmaDePrueba) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch r.Method + " " + r.URL.Path {
	case "POST /api/v1/pdf/sign/prepare":
		_, _ = io.WriteString(w, `{"id":"p1","preparado":"`+base64.StdEncoding.EncodeToString([]byte("%PDF-hueco"))+
			`","resumen":"`+base64.StdEncoding.EncodeToString(make([]byte, 32))+
			`","algoritmo":"ecdsa","nivel":"cualificada","paginas":3,"sello":"tsa.izenpe.com",
			"firmante":{"nombre":"Irati Etxeberria","emisor":"AC FNMT Usuarios","valido_hasta":"2028-01-31T00:00:00Z"}}`)
	case "POST /api/v1/pdf/sign/complete":
		_ = r.ParseMultipartForm(1 << 20)
		c := map[string]string{"id": r.FormValue("id"), "formato": r.FormValue("formato"), "sin_sello": r.FormValue("sin_sello"), "firma": r.FormValue("firma")}
		m.completa = append(m.completa, c)
		if c["sin_sello"] != "1" {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = io.WriteString(w, `{"error":{"code":"pdf_timestamp_unavailable","message":"x"}}`)
			return
		}
		w.Header().Set("X-Sytue-Sello", "no")
		_, _ = io.WriteString(w, "%PDF-firmado")
	case "GET /api/v1/folders/cpdf/children":
		_, _ = io.WriteString(w, `{"folders":[],"files":[{"id":"f9","name":"a.pdf"}]}`)
	case "POST /api/v1/folders/cpdf/files":
		cuerpo, _ := io.ReadAll(r.Body)
		m.subido = string(cuerpo)
		_, _ = io.WriteString(w, `{"id":"f10","folder_id":"cpdf","name":"`+r.URL.Query().Get("name")+`"}`)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// TestFirmarConElCertificadoDelSistema recorre los tres pasos: elegir, ver lo que dice
// el SERVICIO de lo que se firma, el PIN (uno malo primero) y firmar; con la autoridad
// caída no se firma a escondidas, y la MISMA firma sirve para seguir sin sello.
func TestFirmarConElCertificadoDelSistema(t *testing.T) {
	k, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	plantilla := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Irati Etxeberria"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageContentCommitment,
	}
	der, _ := x509.CreateCertificate(rand.Reader, plantilla, plantilla, &k.PublicKey, k)
	cert, _ := x509.ParseCertificate(der)
	tarjeta := &almacenDePrueba{cert: cert, k: k}
	motor := &motorDeFirmaDePrueba{}
	srv := httptest.NewServer(motor)
	defer srv.Close()

	p, _, _ := dialogoDePrueba(t, DialogoProteger)
	var d *dialogoFirmar
	fyne.DoAndWait(func() {
		p.dialogo.cerrar()
		p.Almacenes = func() []certificados.Almacen { return []certificados.Almacen{tarjeta} }
		p.doc = &Documento{
			pdf: api.NewPDF(srv.URL, srv.URL, nil, nil), ruta: "/PDF/a.pdf",
			fichero: api.File{ID: "f9", Name: "a.pdf", FolderID: "cpdf"}, datos: []byte("%PDF-1.7"),
		}
		p.abrirDialogo(DialogoFirmar)
		d = nuevoDialogoFirmar(p.dialogo)
		d.modo = "certificado"
		p.dialogo.pintar = d.vista
		d.buscarCertificados()
	})
	hasta(t, func() bool { return d.sistema.buscado })
	if len(d.sistema.certs) != 1 {
		t.Fatalf("certificados = %d", len(d.sistema.certs))
	}
	fyne.DoAndWait(d.prepararSistema)
	hasta(t, func() bool { return d.sistema.prep != nil })
	fyne.DoAndWait(p.dialogo.rehacer)
	if d.sistema.prep.Firmante.Emisor != "AC FNMT Usuarios" || d.sistema.prep.Nivel != "cualificada" || d.sistema.pin == nil {
		t.Fatalf("antes del PIN: %+v pin=%v", d.sistema.prep, d.sistema.pin)
	}

	fyne.DoAndWait(func() { d.sistema.pin.PonerTexto("0000"); d.firmarSistema() })
	hasta(t, func() bool { return !d.sistema.trabajando && d.sistema.error != "" })
	if d.sistema.firma != nil {
		t.Fatal("con el PIN malo no hay firma")
	}
	fyne.DoAndWait(func() { d.sistema.pin.PonerTexto("1234"); d.firmarSistema() })
	hasta(t, func() bool { return d.sistema.selloCaido && !d.sistema.trabajando })
	if p.Copia != "" {
		t.Fatal("con la autoridad caída se ha firmado sin preguntar")
	}
	firma := d.sistema.firma
	fyne.DoAndWait(func() { d.completarSistema(true) })
	hasta(t, func() bool { return p.Copia != "" })

	motor.mu.Lock()
	defer motor.mu.Unlock()
	if len(motor.completa) != 2 || motor.completa[1]["firma"] != base64.StdEncoding.EncodeToString(firma) ||
		motor.completa[1]["formato"] != api.PDFFirmaDER || motor.subido != "%PDF-firmado" {
		t.Errorf("completar = %v, subido %q", motor.completa, motor.subido)
	}
	if len(tarjeta.pins) != 2 {
		t.Errorf("la tarjeta firmó %d veces: reintentar no vuelve a pedir el PIN", len(tarjeta.pins))
	}
	if !strings.HasPrefix(p.Copia, "/PDF/a (") {
		t.Errorf("copia = %q", p.Copia)
	}
}
