// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ia

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/syerp/internal/catalogo"
)

var facturas = Fuente{Clave: "factura_venta", Etiqueta: "Facturas", Campos: []Campo{
	{Clave: "fecha", Tipo: TipoFecha, Etiqueta: "Fecha"},
	{Clave: "cliente", Tipo: TipoTexto, Etiqueta: "Cliente"},
	{Clave: "total", Tipo: TipoDinero, Etiqueta: "Total"},
	{Clave: "estado", Tipo: TipoTexto, Etiqueta: "Estado", Valores: map[string]string{"emitida": "Emitida", "cobrada": "Cobrada"}},
}}

var filas = []Fila{
	{"fecha": "2026-08-20", "cliente": "Acme", "total": int64(10), "estado": "cobrada"},
	{"fecha": "2026-09-02", "cliente": "Acme", "total": int64(20), "estado": "emitida"},
	{"fecha": "2026-09-10", "cliente": "Bolt", "total": int64(30050), "estado": "emitida"},
	{"fecha": "2026-09-12", "cliente": "Bolt", "estado": "emitida"}, // sin importe
}

func TestInforme(t *testing.T) {
	t.Parallel()
	ahora := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	inf, err := InformeDe("Aquí tienes:\n```json\n" + `{"titulo":"Ventas","partes":[
		{"titulo":"Por mes","vista":"barras","consulta":{"fuente":"factura_venta","agrupar":"fecha:mes","medida":"suma:total"}},
		{"titulo":"Inventado","consulta":{"fuente":"factura_venta","columnas":["margen"]}},
		{"titulo":"Mal","vista":"cifra","consulta":{"fuente":"factura_venta","agrupar":"cliente"}},
		{"titulo":"Mejor cliente","consulta":{"fuente":"factura_venta","agrupar":"cliente","medida":"suma:total","limite":1}},
		{"titulo":"Emitidas este mes","vista":"cifra","consulta":{"fuente":"factura_venta","medida":"cuenta","filtros":[{"campo":"estado","op":"=","valor":"Emitida"},{"campo":"fecha","op":">=","valor":"inicio_mes"}]}},
		{"titulo":"Lista","consulta":{"fuente":"factura_venta","columnas":["cliente","total","estado"],"orden":"-total","filtros":[{"campo":"total","op":">","valor":"0,1"}]}}
	]}` + "\n```")
	if err != nil {
		t.Fatal(err)
	}
	errs := inf.Comprobar(map[string]Fuente{facturas.Clave: facturas})
	for i, quiere := range []bool{true, false, false, true, true, true} {
		if (errs[i] == nil) != quiere {
			t.Fatalf("parte %d (%s): error %v", i, inf.Partes[i].Titulo, errs[i])
		}
	}

	o := Opciones{Ahora: ahora, Tope: 100, RotuloCuenta: "Número"}
	tabla := func(i int) string {
		b, _ := json.Marshal(inf.Partes[i].Consulta.Ejecutar(facturas, filas, o).Filas)
		return string(b)
	}
	// Los importes salen en unidades exactas: 20 + 30050 céntimos son 300.70, no 300.69999.
	for i, quiere := range map[int]string{
		0: `[["2026-08",0.10],["2026-09",300.70]]`,
		3: `[["Bolt",300.50]]`,
		4: `[[3]]`, // «Emitida» vale por su rótulo, y la del 12 cuenta aunque no tenga importe
		5: `[["Bolt",300.50,"Emitida"],["Acme",0.20,"Emitida"]]`,
	} {
		if got := tabla(i); got != quiere {
			t.Errorf("parte %d: %s, quiero %s", i, got, quiere)
		}
	}
	if c := inf.Partes[4].Consulta; c.Ejecutar(facturas, filas, Opciones{Ahora: ahora.AddDate(0, 1, 0), Tope: 1}).Filas[0][0] != 0.0 {
		t.Error("«inicio_mes» se resuelve al ejecutar: el mes que viene no hay emitidas")
	}
	if tb := (Consulta{Fuente: "factura_venta"}).Ejecutar(facturas, filas, Opciones{Tope: 2}); !tb.Recortada || len(tb.Filas) != 2 {
		t.Errorf("el tope recorta y lo dice: %+v", tb)
	}
}

func TestEscribirXLSX(t *testing.T) {
	t.Parallel()
	tabla := Consulta{Fuente: "factura_venta", Orden: "fecha"}.Ejecutar(facturas, filas, Opciones{Tope: 10})
	var buf bytes.Buffer
	if err := EscribirXLSX(&buf, []Hoja{{Nombre: "Ventas: 2026/09 [todas]", Tabla: tabla}, {Nombre: "ventas: 2026/09 [todas]", Tabla: Tabla{}}}); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	leer := func(nombre string) string {
		f, err := zr.Open(nombre)
		if err != nil {
			t.Fatalf("falta %s: %v", nombre, err)
		}
		b, _ := io.ReadAll(f)
		return string(b)
	}
	libro, hoja := leer("xl/workbook.xml"), leer("xl/worksheets/sheet1.xml")
	fecha := fmt.Sprintf(`<c r="A2" s="3"><v>%d</v></c>`, 46254) // 2026-08-20 en días de Excel
	for _, quiere := range []string{`name="Ventas- 2026-09 -todas-"`, `name="ventas- 2026-09 -todas- (2)"`} {
		if !strings.Contains(libro, quiere) {
			t.Errorf("workbook sin %s: %s", quiere, libro)
		}
	}
	for _, quiere := range []string{`t="inlineStr" s="1"><is><t>Cliente</t>`, fecha, `<c r="C4" s="2"><v>300.50</v></c>`, `<t xml:space="preserve">Cobrada</t>`} {
		if !strings.Contains(hoja, quiere) {
			t.Errorf("hoja sin %s", quiere)
		}
	}
	_ = leer("[Content_Types].xml")
	_ = leer("xl/styles.xml")
	_ = leer("xl/worksheets/sheet2.xml")
}

func TestFichaDe(t *testing.T) {
	t.Parallel()
	m := &catalogo.Modelo{Clave: "factura_venta", Campos: []catalogo.Campo{
		{Clave: "total", Tipo: catalogo.TipoDinero},
		{Clave: "fecha", Tipo: catalogo.TipoFecha},
		{Clave: "estado_pago", Tipo: catalogo.TipoSeleccion, Opciones: []string{"pendiente", "pagada"}},
		{Clave: "cliente", Tipo: catalogo.TipoRelacion, Modelo: "contacto"},
		{Clave: "correo", Tipo: catalogo.TipoCorreo},
		{Clave: "notas", Tipo: catalogo.TipoTexto},
	}}
	vista := catalogo.ModeloVista{Campos: []catalogo.CampoVista{
		{Clave: "total", Etiqueta: "Total"},
		{Clave: "fecha", Etiqueta: "Fecha"},
		{Clave: "estado_pago", Etiqueta: "Estado del pago"},
		{Clave: "cliente", Etiqueta: "Cliente"},
		{Clave: "correo", Etiqueta: "Correo"},
		{Clave: "notas", Etiqueta: "Notas"},
	}}
	data, rel := FichaDe(m, vista, "```json\n"+`{"total":"1.234,5","fecha":"2026-09-15","estado_pago":"cobrada","cliente":" Acme ","correo":"no es un correo","inventado":"x"}`+"\n```")
	if len(data) != 1 || data["fecha"] != "2026-09-15" || rel["cliente"] != "Acme" {
		t.Fatalf("data=%v rel=%v", data, rel)
	}
	// Como respondió el modelo interactivo de verdad: rótulos por claves. Y si
	// vienen los dos, manda la clave.
	data, _ = FichaDe(m, vista, `{"Total":1234.56,"Estado del pago":"pagada","NOTAS":"del rótulo","notas":"hola"}`)
	if data["total"] != int64(123456) || data["estado_pago"] != "pagada" || data["notas"] != "hola" {
		t.Fatalf("data=%v", data)
	}
}

func TestFechaDe(t *testing.T) {
	t.Parallel()
	ahora := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	for entrada, quiere := range map[string]string{
		"hoy": "2026-09-15", "hoy-30d": "2026-08-16", "hoy+7d": "2026-09-22", "2026-01-31": "2026-01-31",
		"inicio_mes": "2026-09-01", "inicio_mes-9": "2025-12-01",
		"inicio_trimestre": "2026-07-01", "inicio_trimestre-1": "2026-04-01", "inicio_trimestre-3": "2025-10-01",
		"inicio_anio": "2026-01-01", "inicio_anio-1": "2025-01-01",
		"inicio_mes-x": "", "ayer": "", "hoy-d": "",
	} {
		got, err := fechaDe(entrada, ahora)
		if got != quiere || (quiere == "") != (err != nil) {
			t.Errorf("fechaDe(%q) = %q, %v; quiero %q", entrada, got, err, quiere)
		}
	}
	gastos := Fuente{Clave: "apuntes", Campos: []Campo{{Clave: "cuenta", Tipo: TipoTexto}, {Clave: "debe", Tipo: TipoDinero}}}
	c := Consulta{Fuente: "apuntes", Medida: "suma:debe", Filtros: []Filtro{{Campo: "cuenta", Op: "empieza", Valor: "6"}}}
	if err := c.comprobar(gastos, VistaCifra); err != nil {
		t.Fatal(err)
	}
	filas := []Fila{{"cuenta": "600000", "debe": int64(100)}, {"cuenta": "629000", "debe": int64(50)}, {"cuenta": "706000", "debe": int64(999)}}
	if got := c.Ejecutar(gastos, filas, Opciones{Tope: 1}).Filas[0][0]; got != Unidades(150) {
		t.Errorf("gastos (cuentas 6…) = %v, quiero 1.50", got)
	}
}
