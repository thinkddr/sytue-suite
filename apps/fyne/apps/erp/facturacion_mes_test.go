// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// hoyFacturacion es el 12 de noviembre de 2026, a mediodía local, como en la web.
var hoyFacturacion = time.Date(2026, 11, 12, 12, 0, 0, 0, time.Local)

// TestLaFacturacionPorMesPideLoMismoQueLaWeb: la consulta de `facturacionMensual.ts`,
// campo a campo, y los doce meses rellenos con cero.
func TestLaFacturacionPorMesPideLoMismoQueLaWeb(t *testing.T) {
	meses := MesesHasta(hoyFacturacion, MesesFacturacion)
	if len(meses) != 12 || meses[0] != "2025-12" || meses[11] != "2026-11" {
		t.Fatalf("los doce meses son %v", meses)
	}
	// El día 31 no se salta febrero.
	if got := MesesHasta(time.Date(2027, 3, 31, 0, 0, 0, 0, time.Local), 2); got[0] != "2027-02" || got[1] != "2027-03" {
		t.Errorf("desde un 31, los meses son %v", got)
	}

	inf := InformeFacturacionMensual("Facturación por mes", hoyFacturacion)
	if len(inf.Partes) != 1 || inf.Partes[0].Vista != api.ERPVistaBarras {
		t.Fatalf("el informe es %+v", inf)
	}
	var consulta, quiero any
	_ = json.Unmarshal(inf.Partes[0].Consulta, &consulta)
	_ = json.Unmarshal([]byte(`{"fuente":"factura_venta","filtros":[
		{"campo":"estado","op":"!=","valor":"anulado"},
		{"campo":"fecha","op":">=","valor":"2025-12-01"}],
		"agrupar":"fecha:mes","medida":"suma:total","orden":"grupo"}`), &quiero)
	a, _ := json.Marshal(consulta)
	b, _ := json.Marshal(quiero)
	if !bytes.Equal(a, b) {
		t.Errorf("la consulta es %s; la web pide %s", a, b)
	}

	serie := SerieMensual(api.ERPTablaInforme{Filas: [][]any{
		{"2026-01", 10.0}, {"2026-11", 121.5}, {"2027-02", 99.0},
	}}, hoyFacturacion)
	if len(serie) != 12 || serie[1].Valor != 10 || serie[11] != (MesFacturado{"2026-11", 121.5}) {
		t.Errorf("la serie es %+v", serie)
	}
	ceros := 0
	for _, m := range serie {
		if m.Valor == 0 {
			ceros++
		}
	}
	if ceros != 10 {
		t.Errorf("hay %d meses a cero y deberían ser 10 (la factura futura no cuenta)", ceros)
	}
	if n := TotalCentimos([]MesFacturado{{Valor: 0.1}, {Valor: 0.2}}); n != 30 {
		t.Errorf("0,10 + 0,20 son %d céntimos", n)
	}
}

// barrasDe son los gráficos de barras de un árbol.
func barrasDe(o fyne.CanvasObject) []*Barras {
	var out []*Barras
	recorrer(o, func(x fyne.CanvasObject) {
		if b, ok := x.(*Barras); ok {
			out = append(out, b)
		}
	})
	return out
}

// TestLaPortadaPintaLaFacturacionPorMes: bajo las tres cifras, a `gap-3`, la tarjeta con
// su título, el total y doce barras con el mes debajo; mientras llega, su esbozo al
// mismo alto; sin datos o sin facturas de venta, nada.
//
// VISTO EN ROJO: sin `tarjetaFacturacion` en `resumen`, el resumen mide lo de siempre
// (132) con la serie cargada y no hay barras.
func TestLaPortadaPintaLaFacturacionPorMes(t *testing.T) {
	b := portadaArnes(t, 1280)
	_, sinGrafico := b.resumen(tema.Claro, 854)

	b.Facturacion = FacturacionMensual{Serie: SerieMensual(api.ERPTablaInforme{Filas: [][]any{
		{"2026-10", 1000.0}, {"2026-11", 210.5},
	}}, hoyFacturacion)}
	o, conGrafico := b.resumen(tema.Claro, 854)
	tarjeta, alto := b.tarjetaFacturacion(tema.Claro, 854)
	if tarjeta == nil {
		t.Fatal("con la serie cargada la tarjeta no se pinta")
	}
	cerca(t, "el resumen con el gráfico", conGrafico, sinGrafico+HuecoTarjetas+alto)
	// p-4 + borde, la línea del título, su mb-3, las barras y su fila de meses.
	cerca(t, "la tarjeta del gráfico", alto, 2*(tema.Borde+TarjetaPX)+tema.TextoXS.Linea+TituloParteMB+AltoBarras+AltoEjeBarras)

	// El total va en cifras tabulares, colocadas una a una: se lee junto.
	textos := strings.Join(textosDe(o), "")
	for _, s := range []string{"Facturación por mes", "Últimos 12 meses: 1210,50", "dicenefebmar"} {
		if !strings.Contains(textos, s) {
			t.Errorf("el resumen no dice «%s»: %q", s, textos)
		}
	}
	bs := barrasDe(o)
	if len(bs) != 1 || len(bs[0].Puntos) != 12 {
		t.Fatalf("hay %d gráficos; debería haber uno de doce barras", len(bs))
	}
	if p := bs[0].Puntos[11]; p.Eje == "" || !strings.Contains(p.Rotulo, "2026") || p.Valor != 210.5 {
		t.Errorf("la última barra es %+v", p)
	}
	if l := bs[0].AccessibilityLabel(); !strings.Contains(l, "1210,50") || !strings.Contains(l, "210,50") {
		t.Errorf("el nombre accesible no lleva los importes: %q", l)
	}

	b.Facturacion = FacturacionMensual{Serie: SerieMensual(api.ERPTablaInforme{}, hoyFacturacion)}
	if !strings.Contains(strings.Join(textosDe(b.tarjetaFacturacionObjeto(t)), ""), "Sin facturas en los últimos 12 meses") {
		t.Error("sin facturas en la ventana la tarjeta no lo dice")
	}

	b.Facturacion = FacturacionMensual{Cargando: true}
	cargando, altoCargando := b.tarjetaFacturacion(tema.Claro, 854)
	cerca(t, "la tarjeta mientras carga", altoCargando, alto)
	if n, _, _ := esqueletosDe(cargando); n != 1 || len(barrasDe(cargando)) != 0 {
		t.Errorf("mientras carga hay %d esbozos y %d gráficos", n, len(barrasDe(cargando)))
	}

	b.Facturacion = FacturacionMensual{}
	if o, _ := b.tarjetaFacturacion(tema.Claro, 854); o != nil {
		t.Error("sin datos (sin permiso, servicio caído) la tarjeta se pinta igual")
	}
	b.Facturacion = FacturacionMensual{Serie: make([]MesFacturado, 12)}
	b.Activos = []string{"crm", "rrhh", "contactos"}
	if o, _ := b.tarjetaFacturacion(tema.Claro, 854); o != nil {
		t.Error("con la facturación apagada la tarjeta se pinta igual")
	}
}

// TestLaPortadaConLaFacturacionSePinta guarda la portada con el gráfico (SYTUE_PNG) para
// MIRARLA, en el escritorio y en el teléfono, y comprueba que el gráfico está en ella.
func TestLaPortadaConLaFacturacionSePinta(t *testing.T) {
	for _, c := range []struct {
		nombre  string
		ventana fyne.Size
	}{{"erp-portada-facturacion.png", fyne.NewSize(1280, 1000)}, {"erp-portada-facturacion-movil.png", fyne.NewSize(390, 1600)}} {
		p, w := pantallaArnes(t, c.ventana)
		p.Abrir(Modelo{})
		arnes := portadaArnes(t, c.ventana.Width)
		p.Catalogo.Modulos, p.Catalogo.Activos = arnes.Modulos, arnes.Activos
		b := p.inicio
		b.Resumen, b.Hoy = arnes.Resumen, hoyFacturacion
		b.Facturacion = FacturacionMensual{Serie: SerieMensual(api.ERPTablaInforme{Filas: [][]any{
			{"2026-01", 3200.0},
			{"2026-02", 4100.0},
			{"2026-03", 2800.0},
			{"2026-04", 5150.0},
			{"2026-05", 4700.0},
			{"2026-06", 6100.0},
			{"2026-07", 3900.0},
			{"2026-09", 7250.5},
			{"2026-10", 6800.0},
			{"2026-11", 2210.5},
		}}, hoyFacturacion)}
		p.Repintar()
		guardarPNG(t, w.Canvas().Capture(), c.nombre)
		if len(barrasDe(w.Canvas().Content())) != 1 {
			t.Errorf("%s: la portada no lleva el gráfico", c.nombre)
		}
		w.Close()
	}
}

func (b *Bienvenida) tarjetaFacturacionObjeto(t *testing.T) fyne.CanvasObject {
	t.Helper()
	o, _ := b.tarjetaFacturacion(tema.Claro, 854)
	if o == nil {
		t.Fatal("la tarjeta no se pinta")
	}
	return o
}

// TestLasBarrasConEjeLlevanSuFilaDeMeses: con rótulos cortos el gráfico crece la fila
// de `mt-1 text-xs` y pinta un texto bajo cada barra, centrado en su hueco; sin ellos,
// mide lo de siempre.
func TestLasBarrasConEjeLlevanSuFilaDeMeses(t *testing.T) {
	_ = traductor(t) // arranca la app de pruebas
	sin := NuevasBarras("x", []PuntoBarras{{Rotulo: "a", Valor: 1}})
	cerca(t, "las barras sin eje", sin.MinSize().Height, AltoBarras)

	con := NuevasBarras("x", []PuntoBarras{{Rotulo: "octubre", Eje: "oct", Valor: 1}, {Rotulo: "noviembre", Eje: "nov"}})
	cerca(t, "las barras con eje", con.MinSize().Height, AltoBarras+AltoEjeBarras)
	r := con.CreateRenderer()
	r.Layout(fyne.NewSize(200, AltoBarras+AltoEjeBarras))
	var rotulos []*canvas.Text
	for _, o := range r.Objects() {
		if x, ok := o.(*canvas.Text); ok {
			rotulos = append(rotulos, x)
		}
	}
	if len(rotulos) != 2 || rotulos[0].Text != "oct" || rotulos[1].Text != "nov" {
		t.Fatalf("los rótulos son %v", rotulos)
	}
	if rotulos[0].Position().Y < AltoBarras || rotulos[1].Position().X < 100 {
		t.Errorf("los rótulos no caen bajo su barra: %v y %v", rotulos[0].Position(), rotulos[1].Position())
	}
}

// TestLaPortadaPideLaFacturacionPorMes: al abrir la portada con las facturas de venta
// encendidas se pide el informe y la serie llega a la portada; con el servicio que
// contesta un error de la parte, la tarjeta se queda fuera.
func TestLaPortadaPideLaFacturacionPorMes(t *testing.T) {
	var cuerpo string
	p, sf := pantallaConServicio(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/informes/datos" {
			c, _ := io.ReadAll(r.Body)
			cuerpo = string(c)
			_, _ = io.WriteString(w, `{"datos":[{"columnas":[],"filas":[["2026-11",50]]}]}`)
			return
		}
		_, _ = io.WriteString(w, `{}`)
	})
	p.inicio.Hoy = hoyFacturacion
	p.Abrir(Modelo{})
	if !contiene(sf.vistas(), "POST /api/v1/informes/datos") {
		t.Fatalf("la portada no pide la facturación por mes: %v", sf.vistas())
	}
	if !strings.Contains(cuerpo, `"agrupar":"fecha:mes"`) || !strings.Contains(cuerpo, `"fuente":"factura_venta"`) {
		t.Errorf("el informe pedido es %s", cuerpo)
	}
	f := p.inicio.Facturacion
	if f.Cargando || len(f.Serie) != 12 || f.Serie[11].Valor != 50 {
		t.Errorf("la facturación de la portada es %+v", f)
	}

	p2, _ := pantallaConServicio(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/informes/datos" {
			_, _ = io.WriteString(w, `{"datos":[{"columnas":null,"filas":null,"error":"sin_permiso"}]}`)
			return
		}
		_, _ = io.WriteString(w, `{}`)
	})
	p2.Abrir(Modelo{})
	if f := p2.inicio.Facturacion; f.Cargando || f.Serie != nil {
		t.Errorf("sin permiso la facturación es %+v y no debería pintarse", f)
	}
}

// TestEncenderLasFacturasTraeSuGrafico: con las facturas apagadas la portada no pide
// nada; al encenderlas desde ella se pide la facturación sin esperar a recargarla.
//
// VISTO EN ROJO: sin la llamada de `cambiarModulo`, el gráfico no salía hasta volver a
// abrir la portada.
func TestEncenderLasFacturasTraeSuGrafico(t *testing.T) {
	p, sf := pantallaConServicio(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/informes/datos":
			_, _ = io.WriteString(w, `{"datos":[{"columnas":[],"filas":[["2026-11",50]]}]}`)
		case "/api/v1/modulos/facturacion":
			_, _ = io.WriteString(w, `{"activos":["facturacion","contactos"]}`)
		default:
			_, _ = io.WriteString(w, `{}`)
		}
	})
	p.Catalogo.Activos = []string{"contactos"}
	p.inicio.Hoy = hoyFacturacion
	p.Abrir(Modelo{})
	if contiene(sf.vistas(), "POST /api/v1/informes/datos") || p.inicio.Facturacion.Serie != nil {
		t.Fatalf("con las facturas apagadas no se pide la facturación: %v", sf.vistas())
	}
	p.cambiarModulo(p.Catalogo.Modulos[0], true)
	if !contiene(sf.vistas(), "POST /api/v1/informes/datos") {
		t.Fatalf("al encender las facturas no se pide la facturación: %v", sf.vistas())
	}
	if f := p.inicio.Facturacion; f.Cargando || len(f.Serie) != 12 || f.Serie[11].Valor != 50 {
		t.Errorf("la facturación tras encender las facturas es %+v", f)
	}
}
