// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/certificados"
	"git.sytue.eu/sytue/one/apps/nucleo/local/archivos"
	localpdf "git.sytue.eu/sytue/one/apps/nucleo/pdf"
	"git.sytue.eu/sytue/one/apps/nucleo/pdfsimple"
)

// textoDe junta los párrafos y rótulos de un objeto.
func textoDe(o fyne.CanvasObject) string {
	var b strings.Builder
	todos(o, func(x fyne.CanvasObject) {
		switch v := x.(type) {
		case *ui.Parrafo:
			b.WriteString(v.Texto + "\n")
		case *ui.Rotulo:
			b.WriteString(v.Texto + "\n")
		}
	})
	return b.String()
}

// buscarEn recorre el árbol y devuelve el primer objeto que cumple la condición.
func buscarEn(o fyne.CanvasObject, si func(fyne.CanvasObject) bool) fyne.CanvasObject {
	if si(o) {
		return o
	}
	var hijos []fyne.CanvasObject
	switch c := o.(type) {
	case *fyne.Container:
		hijos = c.Objects
	case fyne.Widget:
		hijos = test.WidgetRenderer(c).Objects()
	}
	for _, h := range hijos {
		if r := buscarEn(h, si); r != nil {
			return r
		}
	}
	return nil
}

func accionCon(o fyne.CanvasObject, nombre string) *Accion {
	r := buscarEn(o, func(x fyne.CanvasObject) bool { a, ok := x.(*Accion); return ok && a.Nombre == nombre })
	if r == nil {
		return nil
	}
	return r.(*Accion)
}

// TestLaEdicionLocalEligeLaAutoridadDeSelloAlFirmar es P-05 en la pantalla: la lista se
// ofrece con «sin sello» primero, la persona añade otra dirección, la preparación del
// motor LOCAL dice esa autoridad, y sin ninguna lo dice con el texto de la edición.
func TestLaEdicionLocalEligeLaAutoridadDeSelloAlFirmar(t *testing.T) {
	k, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	plantilla := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Irati"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature,
	}
	der, _ := x509.CreateCertificate(rand.Reader, plantilla, plantilla, &k.PublicKey, k)
	cert, _ := x509.ParseCertificate(der)

	disco, err := archivos.Abrir(filepath.Join(t.TempDir(), "Drive"), archivos.Opciones{Instalacion: "prueba"})
	if err != nil {
		t.Fatal(err)
	}
	pdf, err := api.NewPDF("https://sin-servidor.invalid", "https://sin-servidor.invalid", nil, nil).WithPrivateDrive(disco)
	if err != nil {
		t.Fatal(err)
	}
	actual, lista := "", []AutoridadSello{{Nombre: "Izenpe", URL: "http://tsa.izenpe.com"}}
	ajuste := &AjusteSello{
		Leer:  func() (string, []AutoridadSello) { return actual, lista },
		Poner: func(a string, l []AutoridadSello) { actual, lista = a, l },
	}
	pdf.ConLocal(api.PDFLocal{Sello: func() localpdf.OpcionesSello { return localpdf.OpcionesSello{URL: actual} }})
	w := pdfsimple.Nuevo()
	w.Texto("Hoja", "F1", 12)

	p, _, _ := dialogoDePrueba(t, DialogoProteger)
	var d *dialogoFirmar
	fyne.DoAndWait(func() {
		p.dialogo.cerrar()
		p.Almacenes = func() []certificados.Almacen { return []certificados.Almacen{&almacenDePrueba{cert: cert, k: k}} }
		p.SelloDeTiempo = ajuste
		p.Idioma = "es"
		p.doc = &Documento{pdf: pdf, ruta: "/PDF/a.pdf", fichero: api.File{ID: "x", Name: "a.pdf"}, datos: w.Bytes()}
		p.abrirDialogo(DialogoFirmar)
		d = nuevoDialogoFirmar(p.dialogo)
		d.modo = "certificado"
		p.dialogo.pintar = d.vista
		d.buscarCertificados()
	})
	hasta(t, func() bool { return d.sistema.buscado })

	var vista fyne.CanvasObject
	fyne.DoAndWait(func() { vista = d.selloLocal(400) })
	sel := buscarEn(vista, func(o fyne.CanvasObject) bool { _, ok := o.(*ui.Selector); return ok })
	if sel == nil {
		t.Fatal("no hay selector de autoridad")
	}
	s := sel.(*ui.Selector)
	if len(s.Opciones) != 2 || s.Opciones[0] != "Sin sello de tiempo" || s.Elegida != 0 {
		t.Fatal("opciones", s.Opciones, s.Elegida)
	}
	// Sin autoridad: la preparación local no nombra ninguna y la pantalla lo dice.
	fyne.DoAndWait(d.prepararSistema)
	hasta(t, func() bool { return d.sistema.prep != nil || d.sistema.error != "" })
	if d.sistema.prep == nil || d.sistema.prep.Sello != "" {
		t.Fatal("preparar sin sello", d.sistema.error)
	}
	var texto string
	fyne.DoAndWait(func() { texto = textoDe(d.loQueSeFirma(400)) })
	if !strings.Contains(texto, "no has elegido autoridad") {
		t.Fatal("sin autoridad se dice", texto)
	}

	// Otra dirección: se añade a la lista, se elige y tira la preparación.
	fyne.DoAndWait(func() {
		d.sistema.urlSello.PonerTexto("https://tsa.example.eu/tsr")
		accionCon(vista, "Usar y añadir a la lista").AlPulsar()
	})
	if actual != "https://tsa.example.eu/tsr" || len(lista) != 2 || d.sistema.prep != nil {
		t.Fatal("añadir", actual, lista, d.sistema.prep)
	}
	fyne.DoAndWait(d.prepararSistema)
	hasta(t, func() bool { return d.sistema.prep != nil || d.sistema.error != "" })
	if d.sistema.prep == nil || d.sistema.prep.Sello != "tsa.example.eu" {
		t.Fatal("la preparación nombra la autoridad elegida", d.sistema.error)
	}
	// Una dirección que no es http(s) no se acepta.
	fyne.DoAndWait(func() {
		vista = d.selloLocal(400)
		d.sistema.urlSello.PonerTexto("ftp://x")
		accionCon(vista, "Usar y añadir a la lista").AlPulsar()
	})
	if d.sistema.error == "" || len(lista) != 2 {
		t.Fatal("una dirección mala", d.sistema.error, lista)
	}
	// Quitar la elegida vuelve a «sin sello».
	fyne.DoAndWait(func() {
		vista = d.selloLocal(400)
		accionCon(vista, "Quitar de la lista").AlPulsar()
	})
	if actual != "" || len(lista) != 1 {
		t.Fatal("quitar", actual, lista)
	}
}

// TestElPanelDeFirmasDiceLasAnclasYLaRevocacion es P-06 en la pantalla.
func TestElPanelDeFirmasDiceLasAnclasYLaRevocacion(t *testing.T) {
	p := NuevaPantalla(traductorES(t))
	p.Idioma = "es"
	elegida := false
	n := 0
	p.Anclas = &AjusteAnclas{
		Carpeta: func() string { return "/home/ana/anclas" },
		Cuantos: func() int { return n },
		Elegir:  func(listo func()) { elegida = true; n = 3; listo() },
	}
	l := &PanelLateral{pant: p, t: p.tv, c: &Contenido{}}
	f := &panelFirmas{l: l, vista: container.NewVBox()}
	var texto string
	fyne.DoAndWait(func() { texto = textoDe(container.NewVBox(f.anclasLocales(true)...)) })
	if !strings.Contains(texto, "No hay certificados de confianza en /home/ana/anclas") || !strings.Contains(texto, "revocado") {
		t.Fatal(texto)
	}
	fyne.DoAndWait(func() {
		accionCon(container.NewVBox(f.anclasLocales(true)...), "Elegir carpeta…").AlPulsar()
		texto = textoDe(container.NewVBox(f.anclasLocales(true)...))
	})
	if !elegida || !strings.Contains(texto, "3 certificados de confianza en /home/ana/anclas") {
		t.Fatal(texto)
	}
	// La conectada no lo enseña.
	p.Anclas = nil
	if f.anclasLocales(false) != nil {
		t.Fatal("sin ajuste de anclas no hay nada que decir")
	}
}
