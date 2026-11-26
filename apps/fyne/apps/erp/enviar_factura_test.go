// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// rotulosCabecera son los nombres de los botones de la cabecera de la ficha.
func rotulosCabecera(f *Ficha) []string {
	var out []string
	for _, b := range f.botonesCabecera() {
		if x, ok := b.(*ui.Boton); ok {
			out = append(out, x.Rotulo)
		}
	}
	return out
}

// La ficha no ofrece «PDF» ni «Enviar por correo» si la pantalla no los pide (el valor
// cero, que es la edición conectada); pedidos, salen en la factura de cliente, y enviar
// no sale a quien solo lee ni desde la papelera.
func TestLaFichaOfrecePDFYEnviarSoloSiSeLosPide(t *testing.T) {
	tr := traductor(t)
	r := facturasArnes().Registros[0]
	pdf, enviar := tr.T("form.downloadPdf", nil), tr.T("send.action", nil)

	f := NuevaFicha(tr, facturaArnes(), &r, nil, nil)
	antes := rotulosCabecera(f)
	if slices.Contains(antes, pdf) || slices.Contains(antes, enviar) {
		t.Fatalf("sin pedirlos ya están: %v", antes)
	}
	f.AlPDF, f.AlEnviar = func() {}, func() {}
	con := rotulosCabecera(f)
	if len(con) != len(antes)+2 || !slices.Contains(con, pdf) || !slices.Contains(con, enviar) {
		t.Fatalf("con PDF y enviar: %v", con)
	}
	f.SoloLectura = true
	if got := rotulosCabecera(f); !slices.Contains(got, pdf) || slices.Contains(got, enviar) {
		t.Errorf("solo lectura: el PDF sí, enviar no: %v", got)
	}
	f.SoloLectura, f.EnPapelera = false, true
	if got := rotulosCabecera(f); slices.Contains(got, enviar) {
		t.Errorf("desde la papelera no se envía: %v", got)
	}
	// Una factura de proveedor no se imprime, ni se baja en PDF, ni se envía.
	m := facturaArnes()
	m.Clave = api.ERPFacturaCompra
	g := NuevaFicha(tr, m, &r, nil, nil)
	g.AlPDF, g.AlEnviar = func() {}, func() {}
	if got := rotulosCabecera(g); slices.Contains(got, pdf) || slices.Contains(got, enviar) {
		t.Errorf("factura de proveedor: %v", got)
	}
}

// El diálogo: la dirección del cliente, la plantilla del servicio si la trae, la cuenta
// elegida y lo que impide enviar, en el orden de la web en la suite y con «Abrir en el
// correo» en Sytue Office sin cuenta.
func TestElDialogoDeEnviarLaFactura(t *testing.T) {
	tr := traductor(t)
	x := NuevoEnviarFactura(tr, "FV-2026-00001")
	if x.Asunto() != tr.T("send.defaultSubject", i18n.Vars{"ref": "FV-2026-00001"}) || x.Puede() {
		t.Fatalf("al abrir: %q, puede %v", x.Asunto(), x.Puede())
	}
	pintar := func() {
		t.Helper()
		for _, v := range []tema.Variante{tema.Claro, tema.Oscuro} {
			if c := x.Cuerpo(v, 448); c.MinSize().Height <= 0 {
				t.Error("el cuerpo no mide nada")
			}
			_ = x.Pie(func() {})
		}
	}
	pintar()

	// La suite: ni cuentas ni plantilla; un despliegue sin correo lo dice antes que el borrador.
	x.Poner(api.ERPEnvioFactura{Para: "pagos@cliente.es", Enviable: false, Disponible: false})
	if x.Para() != "pagos@cliente.es" || x.Bloqueo() != tr.T("send.unavailable", nil) || x.PuedeRedactar() {
		t.Errorf("suite sin correo: %q %q", x.Para(), x.Bloqueo())
	}
	pintar()

	// Sytue Office sin cuenta: se puede abrir en el programa de correo, pero no un borrador.
	x = NuevoEnviarFactura(tr, "FV-2026-00001")
	x.AlRedactar = func() {}
	x.Poner(api.ERPEnvioFactura{Para: "pagos@cliente.es", Enviable: false, PorCuenta: true})
	if x.Bloqueo() != tr.T("send.notIssued", nil) || x.PuedeRedactar() {
		t.Errorf("borrador sin cuenta: %q", x.Bloqueo())
	}
	x.Poner(api.ERPEnvioFactura{Para: "otra@cliente.es", Enviable: true, PorCuenta: true})
	if x.Para() != "pagos@cliente.es" {
		t.Errorf("lo ya escrito no se pisa: %q", x.Para())
	}
	if x.Bloqueo() != textoEnvio("sinCuenta", tr) || !x.PuedeRedactar() || x.Puede() {
		t.Errorf("sin cuenta: %q redactar %v", x.Bloqueo(), x.PuedeRedactar())
	}
	pintar()

	// Con cuentas y plantilla: se elige la cuenta y se manda lo escrito.
	x = NuevoEnviarFactura(tr, "FV-2026-00001")
	var mandado *api.ERPCorreoFactura
	x.AlEnviar = func(c api.ERPCorreoFactura) { mandado = &c }
	x.Poner(api.ERPEnvioFactura{
		Para: "", Enviable: true, Disponible: true, PorCuenta: true,
		Cuentas: []api.ERPCuentaCorreo{{ID: "c1", Direccion: "ana@ejemplo.eu"}, {ID: "c2", Direccion: "pagos@acme.es", Nombre: "Acme"}},
		Asunto:  "Factura FV-2026-00001 de Acme", Mensaje: "Hola:\n\nTe enviamos adjunta la factura FV-2026-00001, por un importe de 1.210,00 €.",
	})
	if x.Asunto() != "Factura FV-2026-00001 de Acme" || x.Mensaje() == tr.T("send.defaultMessage", i18n.Vars{"ref": "FV-2026-00001"}) {
		t.Errorf("la plantilla del servicio: %q", x.Asunto())
	}
	if x.desde == nil || len(x.desde.Opciones) != 2 || x.desde.Opciones[1] != "Acme · pagos@acme.es" {
		t.Fatalf("cuentas: %+v", x.desde)
	}
	pintar()
	// Sin dirección válida no se manda: se marcan los campos.
	x.para.PonerTexto("no es una dirección")
	x.Enviar()
	if mandado != nil || !x.Tocado {
		t.Fatal("se mandó con la dirección mal")
	}
	pintar()
	x.para.PonerTexto("pagos@ferreteria.es")
	x.desde.Elegida = 1
	x.Enviar()
	if mandado == nil || *mandado != (api.ERPCorreoFactura{
		Para: "pagos@ferreteria.es", Asunto: "Factura FV-2026-00001 de Acme",
		Mensaje: x.Mensaje(), Cuenta: "c2",
	}) {
		t.Errorf("mandado: %+v", mandado)
	}
}

// Por qué no salió: un 502 es `send.failed` como en la web, salvo que el ERP local lo
// sepa decir; los demás, por su código.
func TestMensajeDelEnvio(t *testing.T) {
	tr := traductor(t)
	error502 := func(codigo string) error {
		return &api.Error{Status: http.StatusBadGateway, Body: `{"error":{"code":"` + codigo + `","message":"x"}}`}
	}
	for codigo, quiere := range map[string]string{
		"correo_no_enviado":    tr.T("send.failed", nil),
		"upstream_unavailable": tr.T("send.failed", nil),
		"correo_acceso":        ErroresLocales["correo_acceso"][0],
		"correo_sin_red":       ErroresLocales["correo_sin_red"][0],
		"correo_incierto":      ErroresLocales["correo_incierto"][0],
	} {
		if got := MensajeEnvio(error502(codigo), tr); got != quiere {
			t.Errorf("%s: %q", codigo, got)
		}
	}
	e := &api.Error{Status: http.StatusServiceUnavailable, Body: `{"error":{"code":"correo_no_configurado","message":"x"}}`}
	if got := MensajeEnvio(e, tr); got != tr.T("send.unavailable", nil) {
		t.Errorf("503: %q", got)
	}
}

// Contra el servicio: «PDF» baja `/pdf`, lo guarda donde diga el selector y lo abre; y
// enviar pide `/enviar`, manda lo escrito y vuelve a pedir el historial.
func TestPDFYEnviarContraElServicio(t *testing.T) {
	var enviado api.ERPCorreoFactura
	p, sf := pantallaFacturas(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/factura_venta/f1/pdf":
			w.Header().Set("Content-Disposition", `attachment; filename="FV-2026-00001.pdf"`)
			_, _ = io.WriteString(w, "%PDF-1.7 factura")
		case "GET /api/v1/factura_venta/f1/enviar":
			_, _ = io.WriteString(w, `{"para":"pagos@cliente.es","enviable":true,"disponible":true,"por_cuenta":true,`+
				`"cuentas":[{"id":"c1","direccion":"ana@ejemplo.eu"}]}`)
		case "POST /api/v1/factura_venta/f1/enviar":
			_ = json.NewDecoder(r.Body).Decode(&enviado)
			_, _ = io.WriteString(w, `{"enviada":true}`)
		default:
			_, _ = io.WriteString(w, `{}`)
		}
	})
	dir := t.TempDir()
	var guardado, abierto string
	p.Salida = SalidaFactura{
		PDF: true, Enviar: true,
		Guardar: func(titulo, nombre string, datos []byte, listo func(string, error)) {
			if titulo != p.t("form.downloadPdf") || nombre != "FV-2026-00001.pdf" {
				t.Errorf("selector: %q %q", titulo, nombre)
			}
			guardado = filepath.Join(dir, nombre)
			listo(guardado, os.WriteFile(guardado, datos, 0o600))
		},
		Abrir: func(ruta string) { abierto = ruta },
	}
	p.lista.Modelo = p.Catalogo.Modulos[0].Modelos[1]
	p.abrirFicha(&Registro{ID: "f1", Modelo: api.ERPFacturaVenta, Ref: "FV-2026-00001", Estado: "confirmado"}, nil)
	f := p.ficha
	if f.AlPDF == nil || f.AlEnviar == nil {
		t.Fatal("la pantalla no enganchó PDF y enviar")
	}

	f.AlPDF()
	if datos, err := os.ReadFile(guardado); err != nil || string(datos) != "%PDF-1.7 factura" || abierto != guardado {
		t.Fatalf("PDF guardado en %q y abierto %q: %q %v", guardado, abierto, datos, err)
	}
	// Cancelar el selector no abre nada.
	abierto = ""
	p.Salida.Guardar = func(_, _ string, _ []byte, listo func(string, error)) { listo("", nil) }
	f.AlPDF()
	if abierto != "" {
		t.Error("cancelar el selector abrió algo")
	}

	f.AlEnviar()
	x := p.envio
	if x == nil || x.Datos == nil || x.Para() != "pagos@cliente.es" || dialogoArriba(fyneVentana(t, p)) == nil {
		t.Fatalf("el diálogo no arrancó con los datos: %+v", x)
	}
	x.Enviar()
	if enviado.Para != "pagos@cliente.es" || enviado.Cuenta != "c1" || enviado.Asunto == "" {
		t.Errorf("enviado: %+v", enviado)
	}
	if x.Fallo != "" {
		t.Errorf("fallo: %q", x.Fallo)
	}
	pedidas := sf.vistas()
	if !slices.Contains(pedidas, "POST /api/v1/factura_venta/f1/enviar?") || !slices.Contains(pedidas, "GET /api/v1/factura_venta/f1/historial?") {
		t.Errorf("peticiones: %v", pedidas)
	}
}

// «Abrir en el correo» sin cuenta: con RedactarConPDF (Sytue Office), el PDF de la
// factura va a él, ya adjunto, y no se guarda nada en Descargas ni se abre `mailto:`;
// si falla (Mail no está), lo de siempre: el PDF en Descargas y el programa de correo.
func TestAbrirEnElCorreoConElPDFAdjunto(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DOWNLOAD_DIR", "")
	t.Setenv("TMPDIR", t.TempDir())
	p, _ := pantallaFacturas(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/factura_venta/f1/pdf":
			w.Header().Set("Content-Disposition", `attachment; filename="FV-2026-00001.pdf"`)
			_, _ = io.WriteString(w, "%PDF-1.7 factura")
		case "GET /api/v1/factura_venta/f1/enviar":
			_, _ = io.WriteString(w, `{"para":"pagos@cliente.es","enviable":true,"disponible":false,"por_cuenta":true,"cuentas":[],`+
				`"asunto":"Factura FV-2026-00001","mensaje":"Adjunta la factura."}`)
		default:
			_, _ = io.WriteString(w, `{}`)
		}
	})
	type llamada struct{ para, asunto, mensaje, nombre, pdf, ruta string }
	var conPDF, sinPDF []llamada
	var fallo error
	p.Salida = SalidaFactura{
		PDF: true, Enviar: true,
		Redactar: func(para, asunto, mensaje, ruta string) error {
			datos, _ := os.ReadFile(ruta)
			sinPDF = append(sinPDF, llamada{para: para, asunto: asunto, mensaje: mensaje, ruta: ruta, pdf: string(datos)})
			return nil
		},
		RedactarConPDF: func(para, asunto, mensaje, nombre string, pdf []byte) error {
			conPDF = append(conPDF, llamada{para: para, asunto: asunto, mensaje: mensaje, nombre: nombre, pdf: string(pdf)})
			return fallo
		},
	}
	p.lista.Modelo = p.Catalogo.Modulos[0].Modelos[1]
	p.abrirFicha(&Registro{ID: "f1", Modelo: api.ERPFacturaVenta, Ref: "FV-2026-00001", Estado: "confirmado"}, nil)
	f := p.ficha
	abrirEnCorreo := func() {
		t.Helper()
		f.AlEnviar()
		x := p.envio
		if x == nil || !x.PuedeRedactar() || x.AlRedactar == nil {
			t.Fatalf("sin cuenta no se ofrece «Abrir en el correo»: %+v", x)
		}
		x.AlRedactar()
		if x.Fallo != "" || x.Enviando {
			t.Fatalf("fallo: %q", x.Fallo)
		}
	}

	abrirEnCorreo()
	quiero := llamada{para: "pagos@cliente.es", asunto: "Factura FV-2026-00001", mensaje: "Adjunta la factura.", nombre: "FV-2026-00001.pdf", pdf: "%PDF-1.7 factura"}
	if len(conPDF) != 1 || conPDF[0] != quiero || len(sinPDF) != 0 {
		t.Fatalf("con el PDF: %+v; sin él: %+v", conPDF, sinPDF)
	}
	if dialogoArriba(fyneVentana(t, p)) != nil {
		t.Error("el diálogo sigue abierto")
	}

	// Mail no está: el PDF a Descargas y el programa de correo, como antes.
	fallo = errors.New("sin Mail")
	abrirEnCorreo()
	if len(conPDF) != 2 || len(sinPDF) != 1 || sinPDF[0].pdf != "%PDF-1.7 factura" || sinPDF[0].para != "pagos@cliente.es" ||
		filepath.Base(sinPDF[0].ruta) != "FV-2026-00001.pdf" {
		t.Fatalf("lo de siempre: %+v", sinPDF)
	}
}

// fyneVentana es la ventana de la pantalla del arnés (pantallaFacturas la abre).
func fyneVentana(t *testing.T, p *Pantalla) fyne.Window {
	t.Helper()
	for _, w := range fyne.CurrentApp().Driver().AllWindows() {
		if w.Canvas() == p.lienzo {
			return w
		}
	}
	t.Fatal("sin ventana")
	return nil
}
