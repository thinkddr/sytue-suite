// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package verifactu

import (
	"context"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Los tres ejemplos del documento de huella de la AEAT (v0.1.2): primer alta,
// alta encadenada y anulación encadenada. Si uno falla, nada de lo que se envíe
// casará con lo que calcula la Agencia.
func TestHuellaEjemplosOficiales(t *testing.T) {
	h1 := HuellaAlta("89890001K", "12345678/G33", "01-01-2024", "F1", "12.35", "123.45", "", "2024-01-01T19:20:30+01:00")
	if h1 != "3C464DAF61ACB827C65FDA19F352A4E3BDC2C640E9E9FC4CC058073F38F12F60" {
		t.Fatalf("primer alta: %s", h1)
	}
	h2 := HuellaAlta("89890001K", "12345679/G34", "01-01-2024", "F1", "12.35", "123.45", h1, "2024-01-01T19:20:35+01:00")
	if h2 != "F7B94CFD8924EDFF273501B01EE5153E4CE8F259766F88CF6ACB8935802A2B97" {
		t.Fatalf("alta encadenada: %s", h2)
	}
	h3 := HuellaAnulacion("89890001K", "12345679/G34", "01-01-2024", h2, "2024-01-01T19:20:40+01:00")
	if h3 != "177547C0D57AC74748561D054A9CEC14B4C4EA23D1BEFD6F2E69E3A388F90C68" {
		t.Fatalf("anulación: %s", h3)
	}
}

func TestFormatos(t *testing.T) {
	for _, c := range []struct{ got, want string }{
		{Importe(12345), "123.45"},
		{Importe(-1235), "-12.35"},
		{Importe(5), "0.05"},
		{Importe(0), "0.00"},
		{Porcentaje(2100), "21"},
		{Porcentaje(550), "5.5"},
		{Porcentaje(750), "7.5"},
		{Porcentaje(1025), "10.25"},
		{Fecha(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)), "01-01-2024"},
		// El huso es el de Madrid en cada fecha: +01:00 en invierno, +02:00 en verano.
		{FechaHora(time.Date(2024, 1, 1, 18, 20, 30, 0, time.UTC)), "2024-01-01T19:20:30+01:00"},
		{FechaHora(time.Date(2024, 7, 1, 10, 0, 0, 0, time.UTC)), "2024-07-01T12:00:00+02:00"},
	} {
		if c.got != c.want {
			t.Errorf("%q, se esperaba %q", c.got, c.want)
		}
	}
}

func TestTipoIVA(t *testing.T) {
	for _, c := range []struct {
		base, cuota, tipo int64
		ok                bool
	}{
		{10000, 2100, 2100, true},
		{-10000, -2100, 2100, true},
		{99, 21, 2100, true},    // redondeo de céntimos
		{1000, 100, 1000, true}, // 10 %
		{12345, 494, 400, true}, // 4 %
		{10000, 1500, 0, false}, // 15 % no existe
		{100000, 21030, 2100, false},
	} {
		tipo, ok := TipoIVA(c.base, c.cuota)
		if ok != c.ok || (ok && tipo != c.tipo) {
			t.Errorf("TipoIVA(%d, %d) = %d, %v", c.base, c.cuota, tipo, ok)
		}
	}
}

func alta() Alta {
	return Alta{
		Ref:          "0b8c7f2e-4a57-4a8e-9a53-6f0c1d2e3f40",
		Emisor:       Parte{Nombre: "Panadería Ejemplo, S.L.", NIF: "89890001K"},
		Factura:      IDFactura{NIF: "89890001K", NumSerie: "FV-2026-00001", Fecha: time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)},
		TipoFactura:  "F1",
		Descripcion:  "Pan & bollería <septiembre>",
		Destinatario: &Parte{Nombre: "Cafetería Cliente, S.L.", NIF: "B12345674"},
		Desglose:     []Detalle{{Calificacion: "S1", Tipo: 1000, Base: 10000, Cuota: 1000}},
		CuotaTotal:   1000,
		ImporteTotal: 11000,
		Sistema:      Sistema{Version: "v2.90.0", NumeroInstalacion: "20260915101500"},
		Generado:     time.Date(2026, 9, 15, 8, 15, 0, 0, time.UTC),
	}
}

// nombres devuelve los elementos del XML en orden de aparición, con su texto: el
// orden es lo que valida el esquema de la AEAT.
func nombres(t *testing.T, doc string) []string {
	t.Helper()
	d := xml.NewDecoder(strings.NewReader(doc))
	var out []string
	var texto string
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			return out
		}
		if err != nil {
			t.Fatalf("XML mal formado: %v\n%s", err, doc)
		}
		switch v := tok.(type) {
		case xml.StartElement:
			if v.Name.Space != nsSuministroInformacion {
				t.Fatalf("%s fuera del espacio de nombres de la AEAT", v.Name.Local)
			}
			out = append(out, v.Name.Local)
			texto = ""
		case xml.CharData:
			texto = string(v)
		case xml.EndElement:
			if texto != "" {
				out[len(out)-1] = out[len(out)-1] + "=" + texto
			}
			texto = ""
		}
	}
}

func TestAltaXML(t *testing.T) {
	a := alta()
	doc, err := a.XML()
	if err != nil {
		t.Fatal(err)
	}
	huella := HuellaAlta("89890001K", "FV-2026-00001", "15-09-2026", "F1", "10.00", "110.00", "", "2026-09-15T10:15:00+02:00")
	want := []string{
		"RegistroAlta", "IDVersion=1.0", "IDFactura", "IDEmisorFactura=89890001K", "NumSerieFactura=FV-2026-00001",
		"FechaExpedicionFactura=15-09-2026", "RefExterna=0b8c7f2e-4a57-4a8e-9a53-6f0c1d2e3f40",
		"NombreRazonEmisor=Panadería Ejemplo, S.L.", "TipoFactura=F1",
		"DescripcionOperacion=Pan & bollería <septiembre>",
		"Destinatarios", "IDDestinatario", "NombreRazon=Cafetería Cliente, S.L.", "NIF=B12345674",
		"Desglose", "DetalleDesglose", "Impuesto=01", "ClaveRegimen=01", "CalificacionOperacion=S1",
		"TipoImpositivo=10", "BaseImponibleOimporteNoSujeto=100.00", "CuotaRepercutida=10.00",
		"CuotaTotal=10.00", "ImporteTotal=110.00", "Encadenamiento", "PrimerRegistro=S",
		"SistemaInformatico", "NombreRazon=Sytue Projects, S.L.", "NIF=B88729900", "NombreSistemaInformatico=Sytue ERP",
		"IdSistemaInformatico=SE", "Version=v2.90.0", "NumeroInstalacion=20260915101500",
		"TipoUsoPosibleSoloVerifactu=S", "TipoUsoPosibleMultiOT=S", "IndicadorMultiplesOT=N",
		"FechaHoraHusoGenRegistro=2026-09-15T10:15:00+02:00", "TipoHuella=01", "Huella=" + huella,
	}
	if got := nombres(t, doc); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("alta:\n%s\n\nse esperaba:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// Subsanación tras rechazo de una rectificativa encadenada, con cliente de fuera
// y una línea exenta: los bloques opcionales, en su sitio.
func TestAltaRectificativaPorRechazoXML(t *testing.T) {
	a := alta()
	a.TipoFactura = "R1"
	a.Subsanacion, a.PorRechazo = true, true
	a.Rectificadas = []IDFactura{{NIF: "89890001K", NumSerie: "FV-2026-00001", Fecha: a.Factura.Fecha}}
	a.Factura.NumSerie = "FV-2026-00002"
	a.Destinatario = &Parte{Nombre: "Kunde GmbH", CodigoPais: "DE", TipoID: "02", ID: "DE123456789"}
	a.Desglose = []Detalle{{Exencion: "E5", Base: -5000}}
	a.CuotaTotal, a.ImporteTotal = 0, -5000
	a.Anterior = &Anterior{IDFactura: IDFactura{NIF: "89890001K", NumSerie: "FV-2026-00001", Fecha: a.Factura.Fecha}, Huella: "ABC"}
	doc, err := a.XML()
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(nombres(t, doc), "|")
	for _, tramo := range []string{
		"NombreRazonEmisor=Panadería Ejemplo, S.L.|Subsanacion=S|RechazoPrevio=X|TipoFactura=R1|TipoRectificativa=I|FacturasRectificadas|IDFacturaRectificada|IDEmisorFactura=89890001K|NumSerieFactura=FV-2026-00001|FechaExpedicionFactura=15-09-2026|DescripcionOperacion=",
		"IDDestinatario|NombreRazon=Kunde GmbH|IDOtro|CodigoPais=DE|IDType=02|ID=DE123456789|Desglose",
		"ClaveRegimen=01|OperacionExenta=E5|BaseImponibleOimporteNoSujeto=-50.00|CuotaTotal=0.00|ImporteTotal=-50.00",
		"Encadenamiento|RegistroAnterior|IDEmisorFactura=89890001K|NumSerieFactura=FV-2026-00001|FechaExpedicionFactura=15-09-2026|Huella=ABC|SistemaInformatico",
	} {
		if !strings.Contains(got, tramo) {
			t.Errorf("falta %q en\n%s", tramo, got)
		}
	}
	if !strings.HasSuffix(got, "Huella="+a.Huella()) || a.Huella() == HuellaAlta("89890001K", "FV-2026-00002", "15-09-2026", "R1", "0.00", "-50.00", "", "2026-09-15T10:15:00+02:00") {
		t.Error("la huella no encadena con la anterior")
	}
}

func TestAnulacionXML(t *testing.T) {
	a := Anulacion{
		Ref:               "r2",
		Factura:           IDFactura{NIF: "89890001K", NumSerie: "12345679/G34", Fecha: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
		SinRegistroPrevio: true,
		Anterior:          &Anterior{IDFactura: IDFactura{NIF: "89890001K", NumSerie: "12345679/G34", Fecha: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}, Huella: "F7B94CFD8924EDFF273501B01EE5153E4CE8F259766F88CF6ACB8935802A2B97"},
		Sistema:           Sistema{Version: "v1", NumeroInstalacion: "1"},
		Generado:          time.Date(2024, 1, 1, 18, 20, 40, 0, time.UTC),
	}
	doc, err := a.XML()
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(nombres(t, doc), "|")
	want := "RegistroAnulacion|IDVersion=1.0|IDFactura|IDEmisorFacturaAnulada=89890001K|NumSerieFacturaAnulada=12345679/G34|FechaExpedicionFacturaAnulada=01-01-2024|RefExterna=r2|SinRegistroPrevio=S|Encadenamiento|RegistroAnterior|"
	if !strings.HasPrefix(got, want) {
		t.Fatalf("anulación:\n%s", got)
	}
	// Con los datos del tercer ejemplo oficial, la huella es la oficial.
	if !strings.HasSuffix(got, "Huella=177547C0D57AC74748561D054A9CEC14B4C4EA23D1BEFD6F2E69E3A388F90C68") {
		t.Fatalf("huella de la anulación:\n%s", got)
	}
}

func TestValidar(t *testing.T) {
	for nombre, cambiar := range map[string]func(*Alta){
		"sin NIF del cliente":     func(a *Alta) { a.Destinatario.NIF = "" },
		"cliente español sin NIF": func(a *Alta) { *a.Destinatario = Parte{Nombre: "X", CodigoPais: "ES", TipoID: "04", ID: "123"} },
		"cuota que no suma":       func(a *Alta) { a.CuotaTotal = 999 },
		"total que no suma":       func(a *Alta) { a.ImporteTotal = 1 },
		"inversión con IVA":       func(a *Alta) { a.Desglose[0].Calificacion = "S2" },
		"exenta con IVA":          func(a *Alta) { a.Desglose[0] = Detalle{Exencion: "E1", Base: 10000, Cuota: 1000} },
		"rectificativa sin qué":   func(a *Alta) { a.TipoFactura = "R1" },
		"rechazo sin subsanación": func(a *Alta) { a.PorRechazo = true },
		"número con acentos":      func(a *Alta) { a.Factura.NumSerie = "FACTURA-Ñ" },
		"sin descripción":         func(a *Alta) { a.Descripcion = "  " },
		"sin referencia":          func(a *Alta) { a.Ref = "" },
		"fecha de mañana":         func(a *Alta) { a.Factura.Fecha = a.Generado.AddDate(0, 0, 1) },
	} {
		a := alta()
		d := *a.Destinatario
		a.Destinatario = &d
		a.Desglose = append([]Detalle(nil), a.Desglose...)
		cambiar(&a)
		if _, err := a.XML(); !errors.Is(err, ErrDatos) {
			t.Errorf("%s: %v", nombre, err)
		}
	}
}

func TestURLQR(t *testing.T) {
	// El ejemplo de «URL encoding» de la especificación del QR, con el importe a
	// dos decimales.
	got := URLQR(false, "89890001K", "12345678&G33", time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), 24140)
	if got != "https://prewww2.aeat.es/wlpl/TIKE-CONT/ValidarQR?nif=89890001K&numserie=12345678%26G33&fecha=01-01-2024&importe=241.40" {
		t.Fatal(got)
	}
	if !strings.HasPrefix(URLQR(true, "B", "1", time.Now(), 1), "https://www2.agenciatributaria.gob.es/wlpl/TIKE-CONT/ValidarQR?nif=B&") {
		t.Fatal("producción")
	}
}

const respuestaAEAT = `<?xml version="1.0" encoding="UTF-8"?>
<env:Envelope xmlns:env="http://schemas.xmlsoap.org/soap/envelope/"><env:Header/><env:Body Id="Body">
<tikR:RespuestaRegFactuSistemaFacturacion xmlns:tikR="https://www2.agenciatributaria.gob.es/static_files/common/internet/dep/aplicaciones/es/aeat/tike/cont/ws/RespuestaSuministro.xsd" xmlns:tik="https://www2.agenciatributaria.gob.es/static_files/common/internet/dep/aplicaciones/es/aeat/tike/cont/ws/SuministroInformacion.xsd">
<tikR:CSV>A-YDSW8NLFLANWPM</tikR:CSV>
<tikR:DatosPresentacion><tik:NIFPresentador>B88729900</tik:NIFPresentador><tik:TimestampPresentacion>2026-09-15T10:15:02+02:00</tik:TimestampPresentacion></tikR:DatosPresentacion>
<tikR:Cabecera><tik:ObligadoEmision><tik:NombreRazon>Panadería</tik:NombreRazon><tik:NIF>89890001K</tik:NIF></tik:ObligadoEmision></tikR:Cabecera>
<tikR:TiempoEsperaEnvio>90</tikR:TiempoEsperaEnvio>
<tikR:EstadoEnvio>ParcialmenteCorrecto</tikR:EstadoEnvio>
<tikR:RespuestaLinea><tikR:IDFactura><tik:IDEmisorFactura>89890001K</tik:IDEmisorFactura><tik:NumSerieFactura>FV-1</tik:NumSerieFactura><tik:FechaExpedicionFactura>15-09-2026</tik:FechaExpedicionFactura></tikR:IDFactura><tikR:Operacion><tik:TipoOperacion>Alta</tik:TipoOperacion></tikR:Operacion><tikR:RefExterna>r1</tikR:RefExterna><tikR:EstadoRegistro>Correcto</tikR:EstadoRegistro></tikR:RespuestaLinea>
<tikR:RespuestaLinea><tikR:IDFactura><tik:IDEmisorFactura>89890001K</tik:IDEmisorFactura><tik:NumSerieFactura>FV-2</tik:NumSerieFactura><tik:FechaExpedicionFactura>15-09-2026</tik:FechaExpedicionFactura></tikR:IDFactura><tikR:Operacion><tik:TipoOperacion>Alta</tik:TipoOperacion></tikR:Operacion><tikR:RefExterna>r2</tikR:RefExterna><tikR:EstadoRegistro>AceptadoConErrores</tikR:EstadoRegistro><tikR:CodigoErrorRegistro>2000</tikR:CodigoErrorRegistro><tikR:DescripcionErrorRegistro>El NIF del destinatario no está identificado.</tikR:DescripcionErrorRegistro></tikR:RespuestaLinea>
<tikR:RespuestaLinea><tikR:IDFactura><tik:IDEmisorFactura>89890001K</tik:IDEmisorFactura><tik:NumSerieFactura>FV-3</tik:NumSerieFactura><tik:FechaExpedicionFactura>15-09-2026</tik:FechaExpedicionFactura></tikR:IDFactura><tikR:Operacion><tik:TipoOperacion>Alta</tik:TipoOperacion></tikR:Operacion><tikR:RefExterna>r3</tikR:RefExterna><tikR:EstadoRegistro>Incorrecto</tikR:EstadoRegistro><tikR:CodigoErrorRegistro>3000</tikR:CodigoErrorRegistro><tikR:DescripcionErrorRegistro>Registro de facturación duplicado.</tikR:DescripcionErrorRegistro><tikR:RegistroDuplicado><tik:IdPeticionRegistroDuplicado>202609151015</tik:IdPeticionRegistroDuplicado><tik:EstadoRegistroDuplicado>Correcta</tik:EstadoRegistroDuplicado></tikR:RegistroDuplicado></tikR:RespuestaLinea>
</tikR:RespuestaRegFactuSistemaFacturacion></env:Body></env:Envelope>`

func TestLeerRespuesta(t *testing.T) {
	r, err := LeerRespuesta([]byte(respuestaAEAT), 200)
	if err != nil {
		t.Fatal(err)
	}
	if r.CSV != "A-YDSW8NLFLANWPM" || r.EsperaDe() != 90*time.Second || r.Estado != "ParcialmenteCorrecto" || len(r.Lineas) != 3 {
		t.Fatalf("%+v", r)
	}
	for i, want := range []string{"Correcto/0/", "AceptadoConErrores/2000/El NIF del destinatario no está identificado.", "Correcto/0/"} {
		e, c, d := r.Lineas[i].EstadoFinal()
		if got := e + "/" + strconv.Itoa(c) + "/" + d; got != want || r.Lineas[i].Ref != "r"+strconv.Itoa(i+1) {
			t.Errorf("línea %d: %s (%s)", i, got, r.Lineas[i].Ref)
		}
	}

	fault := `<env:Envelope xmlns:env="http://schemas.xmlsoap.org/soap/envelope/"><env:Body><env:Fault><faultcode>env:Client</faultcode><faultstring>Codigo[4104].El NIF del titular en la cabecera no está identificado.</faultstring></env:Fault></env:Body></env:Envelope>`
	var ee *ErrorEnvio
	if _, err := LeerRespuesta([]byte(fault), 500); !errors.As(err, &ee) || !ee.Cliente || !strings.Contains(ee.Detalle, "4104") {
		t.Fatalf("fault de cliente: %v", err)
	}
	if _, err := LeerRespuesta([]byte(strings.Replace(fault, "env:Client", "env:Server", 1)), 500); !errors.As(err, &ee) || ee.Cliente {
		t.Fatalf("fault de servidor: %v", err)
	}
	if _, err := LeerRespuesta([]byte("<html>502</html>"), 502); !errors.As(err, &ee) || ee.Cliente {
		t.Fatalf("respuesta que no es SOAP: %v", err)
	}
}

func TestEnviar(t *testing.T) {
	var recibido string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cuerpo, _ := io.ReadAll(r.Body)
		recibido = string(cuerpo)
		if r.Header.Get("SOAPAction") != `""` || !strings.HasPrefix(r.Header.Get("Content-Type"), "text/xml") {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = io.WriteString(w, respuestaAEAT)
	}))
	defer srv.Close()

	c := &Cliente{http: srv.Client(), url: srv.URL}
	a := alta()
	frag, err := a.XML()
	if err != nil {
		t.Fatal(err)
	}
	r, err := c.Enviar(context.Background(), Parte{Nombre: "Panadería & Co", NIF: "89890001K"}, []string{frag})
	if err != nil || len(r.Lineas) != 3 {
		t.Fatalf("%v %+v", err, r)
	}
	// El sobre entero tiene que ser XML válido y llevar la cabecera antes del registro.
	var sobre struct {
		Body struct {
			Reg struct {
				Nombre   string `xml:"Cabecera>ObligadoEmision>NombreRazon"`
				Registro struct {
					Alta struct {
						Huella string `xml:"Huella"`
					} `xml:"RegistroAlta"`
				} `xml:"RegistroFactura"`
			} `xml:"RegFactuSistemaFacturacion"`
		} `xml:"Body"`
	}
	if err := xml.Unmarshal([]byte(recibido), &sobre); err != nil {
		t.Fatalf("sobre: %v\n%s", err, recibido)
	}
	if sobre.Body.Reg.Nombre != "Panadería & Co" || sobre.Body.Reg.Registro.Alta.Huella != a.Huella() {
		t.Fatalf("%+v", sobre)
	}
	if _, err := c.Enviar(context.Background(), Parte{}, nil); err == nil {
		t.Fatal("un envío vacío no se manda")
	}
}
