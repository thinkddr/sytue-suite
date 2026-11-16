// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

const (
	pemCert  = "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n"
	pemClave = "-----BEGIN PRIVATE KEY-----\nMIIE\n-----END PRIVATE KEY-----\n"
)

// p12 son unos bytes que no son PEM: el asistente no abre el `.p12`, solo decide si
// pide la contraseña.
var p12 = []byte{0x30, 0x82, 0x0a, 0xf8, 0x02, 0x01, 0x03}

func TestExaminarCertificadoMiraElContenidoYNoLaExtension(t *testing.T) {
	for _, c := range []struct {
		que           string
		datos         string
		pem, conClave bool
	}{
		{"un .p12", string(p12), false, false},
		{"un PEM con todo", pemCert + pemClave, true, true},
		{"un PEM con la clave RSA", pemCert + strings.ReplaceAll(pemClave, "PRIVATE", "RSA PRIVATE"), true, true},
		{"un PEM sin la clave", pemCert, true, false},
	} {
		pem, conClave := ExaminarCertificado([]byte(c.datos))
		if pem != c.pem || conClave != c.conClave {
			t.Errorf("%s: pem %v, con clave %v; quiere %v, %v", c.que, pem, conClave, c.pem, c.conClave)
		}
	}
}

// TestElAsistenteDelCertificadoPideLoQueFalta: un `.p12` pide contraseña y no manda clave;
// un PEM sin clave no deja seguir hasta tener la clave y no pide contraseña; otro fichero
// olvida la clave de antes. Hecho, la contraseña se borra.
//
// VISTO EN ROJO: sin `FaltaClave` en `ListoFichero`, el PEM solo pasa al segundo paso y
// el servicio contesta `certificado_invalido` sin decir por qué.
func TestElAsistenteDelCertificadoPideLoQueFalta(t *testing.T) {
	c := NuevoCertificadoVF(traductor(t))
	if c.ListoFichero() {
		t.Fatal("sin fichero no se sigue")
	}
	c.PonerFichero("/home/ana/cert.pem", []byte(pemCert))
	if !c.FaltaClave() || c.ListoFichero() {
		t.Fatalf("un PEM sin clave pide la clave: falta %v, listo %v", c.FaltaClave(), c.ListoFichero())
	}
	if !contiene(textosDe(c.Cuerpo(tema.Claro, 448)), "Clave privada (PEM)") {
		t.Error("no se ofrece el campo de la clave")
	}
	c.PonerClave("/home/ana/cert.key", []byte(pemClave))
	if !c.ListoFichero() || !c.PuedeGuardar() {
		t.Fatal("con la clave, un PEM se guarda sin contraseña")
	}
	if f := c.Peticion(); string(f.Clave) != pemClave || f.Contrasena != "" {
		t.Errorf("el PEM va con su clave y sin contraseña: %+v", f)
	}

	c.PonerFichero("/home/ana/fnmt.p12", p12)
	if c.Clave != nil || c.FaltaClave() || !c.ListoFichero() {
		t.Fatal("otro fichero olvida la clave del anterior")
	}
	c.Ir(PasoCertConfirmar)
	if c.PuedeGuardar() {
		t.Error("un .p12 no se guarda sin contraseña")
	}
	c.contrasena.PonerTexto("secreta")
	c.sello.Marcada = true
	f := c.Peticion()
	if !bytes.Equal(f.Fichero, p12) || f.Clave != nil || f.Contrasena != "secreta" || !f.Sello {
		t.Errorf("petición del .p12: %+v", f)
	}
	c.Fallido(true, "La contraseña del certificado no es correcta.")
	if c.Paso != PasoCertConfirmar || c.Fallo != "" || !c.contrasena.Invalida {
		t.Errorf("la contraseña mala se marca en su campo sin perder el paso: paso %d, fallo %q", c.Paso, c.Fallo)
	}
	c.Hecho(api.ERPEstadoVerifactu{TieneCertificado: true, CertificadoNIF: "B12345678"})
	if c.contrasena.Texto() != "" || c.Paso != PasoCertListo {
		t.Error("guardado, la contraseña no se queda en el campo")
	}
	c.Ir(PasoCertFichero)
	if c.Paso != PasoCertListo {
		t.Error("guardado, ya no se vuelve atrás")
	}
}

// TestLaTarjetaOfreceSubirElCertificado: sin certificado, «Subir certificado» junto al
// motivo; con él, de quién es y «Cambiar certificado», también activado; quien no
// administra solo lee de quién es.
//
// VISTO EN ROJO (antes de esta pieza): la tarjeta solo decía «Todavía no está
// disponible» y no había forma de dar el certificado desde la app.
func TestLaTarjetaOfreceSubirElCertificado(t *testing.T) {
	tarjeta := func(e api.ERPEstadoVerifactu, admin bool) []string {
		x := &TarjetaVF{tr: traductor(t), loc: "es", Estado: &e, Administrador: admin, AlCertificado: func() {}}
		o, _ := x.Objeto(tema.Claro, 978)
		return textosDe(o)
	}
	falta := api.ERPEstadoVerifactu{Motivo: "sin_certificado", Entorno: "produccion"}
	con := api.ERPEstadoVerifactu{Activo: true, TieneCertificado: true, CertificadoNIF: "B12345678", Entorno: "produccion"}
	for _, c := range []struct {
		que      string
		e        api.ERPEstadoVerifactu
		admin    bool
		hay, nay []string
	}{
		{"sin certificado", falta, true, []string{"Subir certificado"}, []string{"Todavía no está disponible en este Sytue"}},
		{"en pruebas", api.ERPEstadoVerifactu{Entorno: "pruebas"}, true, []string{"Subir certificado", "Activar VERI*FACTU"}, nil},
		{"con certificado", con, true, []string{"Certificado de B12345678", "Cambiar certificado"}, []string{"Subir certificado"}},
		{"sin administrar", con, false, []string{"Certificado de B12345678"}, []string{"Cambiar certificado"}},
		{"sin certificado ni administrar", falta, false, nil, []string{"Subir certificado"}},
	} {
		got := tarjeta(c.e, c.admin)
		for _, s := range c.hay {
			if !contiene(got, s) {
				t.Errorf("%s: falta «%s» en %v", c.que, s, got)
			}
		}
		for _, s := range c.nay {
			if slices.ContainsFunc(got, func(g string) bool { return strings.Contains(g, s) }) {
				t.Errorf("%s: sobra «%s» en %v", c.que, s, got)
			}
		}
	}
	// Con el certificado, la línea va debajo: `mt-2` y una línea `text-xs`.
	x := &TarjetaVF{tr: traductor(t), loc: "es", Estado: &con}
	_, conLinea := x.Objeto(tema.Claro, 978)
	sinCert := con
	sinCert.TieneCertificado = false
	x.Estado = &sinCert
	_, sin := x.Objeto(tema.Claro, 978)
	cerca(t, "la línea del certificado", conLinea-sin, HuecoDos+tema.TextoXS.Linea)
}

// TestSubirElCertificadoDesdeLaPantalla: elegir el `.p12`, equivocarse de contraseña
// (se dice en su campo y se reintenta) y guardar; la tarjeta pasa a decir de quién es.
// Lo que viaja es el fichero tal cual, en base64, con la contraseña y sin clave.
func TestSubirElCertificadoDesdeLaPantalla(t *testing.T) {
	var mu sync.Mutex
	var cuerpos []api.ERPFicheroCertificado
	p, _ := pantallaConServicio(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/v1/verifactu/certificado" {
			_, _ = io.WriteString(w, `{"registros":[]}`)
			return
		}
		var f api.ERPFicheroCertificado
		_ = json.NewDecoder(r.Body).Decode(&f)
		mu.Lock()
		cuerpos = append(cuerpos, f)
		mu.Unlock()
		if f.Contrasena != "secreta" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = io.WriteString(w, `{"error":{"code":"certificado_contrasena","message":"wrong password"}}`)
			return
		}
		_, _ = io.WriteString(w, `{"activo":false,"tiene_certificado":true,"certificado_nif":"B12345678","pendientes":0,"con_errores":0,"rechazadas":0,"entorno":"produccion"}`)
	})
	w := test.NewWindow(p.raiz)
	w.Resize(fyne.NewSize(1280, 800))
	t.Cleanup(w.Close)
	p.lienzo = w.Canvas()
	p.Ventana = fyne.NewSize(1280, 800)
	p.Permisos.Administrador = true
	p.verifactu = &api.ERPEstadoVerifactu{Motivo: "sin_certificado", Entorno: "produccion"}
	ruta := filepath.Join(t.TempDir(), "fnmt.p12")
	if err := os.WriteFile(ruta, p12, 0o600); err != nil {
		t.Fatal(err)
	}
	p.ElegirFicheros = func(listo func([]string)) { listo([]string{"/tmp/notas.txt", ruta}) }

	p.abrirCertificadoVF()
	d := dialogoArriba(w)
	if d == nil {
		t.Fatal("no se abrió el asistente")
	}
	guardarPNG(t, w.Canvas().Capture(), "erp-verifactu-certificado-1.png")
	pulsarDelCuerpo(t, d, "Elegir…")
	if !contiene(textosDe(d.Cuerpo), "fnmt.p12") {
		t.Fatalf("no se ve el fichero elegido: %v", textosDe(d.Cuerpo))
	}
	pulsarDelPie(t, d, "Continuar")
	entrada := primeroDe[*ui.Entrada](d.Cuerpo)
	if entrada == nil {
		t.Fatal("un .p12 pide la contraseña")
	}
	entrada.PonerTexto("mala")
	pulsarDelPie(t, d, "Guardar certificado")
	if txt := textosDe(d.Cuerpo); !contiene(txt, "La contraseña del certificado no es correcta.") {
		t.Fatalf("la contraseña mala no se dice: %v", txt)
	}
	guardarPNG(t, w.Canvas().Capture(), "erp-verifactu-certificado-2.png")
	primeroDe[*ui.Entrada](d.Cuerpo).PonerTexto("secreta")
	pulsarDelPie(t, d, "Guardar certificado")
	if txt := textosDe(d.Cuerpo); !contiene(txt, "Certificado guardado") {
		t.Fatalf("no se llega a «Listo»: %v", txt)
	}
	guardarPNG(t, w.Canvas().Capture(), "erp-verifactu-certificado-3.png")

	mu.Lock()
	defer mu.Unlock()
	if len(cuerpos) != 2 {
		t.Fatalf("%d peticiones; quiere 2", len(cuerpos))
	}
	if f := cuerpos[1]; !bytes.Equal(f.Fichero, p12) || f.Clave != nil || f.Contrasena != "secreta" || f.Sello {
		t.Errorf("cuerpo enviado: %+v", f)
	}
	if e := p.verifactu; e == nil || !e.TieneCertificado || e.CertificadoNIF != "B12345678" {
		t.Errorf("la tarjeta no se entera: %+v", p.verifactu)
	}
}

// TestUnFicheroEnormeNoEsUnCertificado: no se lee entero en memoria ni se manda.
func TestUnFicheroEnormeNoEsUnCertificado(t *testing.T) {
	ruta := filepath.Join(t.TempDir(), "grande.p12")
	if err := os.WriteFile(ruta, make([]byte, TamMaxCertificado+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := leerAcotado(ruta, TamMaxCertificado); err == nil {
		t.Error("un fichero de más de 128 KiB se lee como certificado")
	}
	if _, err := leerAcotado(ruta, TamMaxCertificado+1); err != nil {
		t.Errorf("hasta el tope se lee: %v", err)
	}
}

// pulsarDelCuerpo pulsa el botón del cuerpo del diálogo cuyo rótulo es `rotulo`.
func pulsarDelCuerpo(t *testing.T, d *ui.Dialogo, rotulo string) {
	t.Helper()
	var b *ui.Boton
	recorrer(d.Cuerpo, func(o fyne.CanvasObject) {
		if x, ok := o.(*ui.Boton); ok && b == nil && contiene(textosDe(x), rotulo) {
			b = x
		}
	})
	if b == nil {
		t.Fatalf("el cuerpo no tiene «%s»: %v", rotulo, textosDe(d.Cuerpo))
	}
	b.Tapped(nil)
}

// primeroDe es el primer objeto de ese tipo.
func primeroDe[T fyne.CanvasObject](o fyne.CanvasObject) T {
	var r T
	hallado := false
	recorrer(o, func(x fyne.CanvasObject) {
		if y, ok := x.(T); ok && !hallado {
			r, hallado = y, true
		}
	})
	return r
}
