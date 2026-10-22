// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Checks del intercambio de datos de formulario. Lo que importa es la VUELTA
// COMPLETA: exportar y volver a importar tiene que devolver exactamente lo
// mismo, porque el uso real es sacar los datos, tocarlos fuera y meterlos otra
// vez. Si por el camino se pierde un acento o una comilla, se pierde un dato.
package domain

import (
	"strings"
	"testing"
)

// formularioDePrueba deja un documento con tres campos rellenos.
func formularioDePrueba(t *testing.T, valores map[string]string) *PDFDoc {
	t.Helper()
	base, err := AnadirCamposPDF(docDePrueba(t, "solicitud"), []CampoNuevo{
		{Tipo: CampoTexto, Nombre: "nombre", Pagina: 0, Rect: [4]float64{10, 80, 200, 96}},
		{Tipo: CampoTexto, Nombre: "notas", Pagina: 0, Rect: [4]float64{10, 50, 200, 70}},
		{Tipo: CampoCasilla, Nombre: "acepto", Pagina: 0, Rect: [4]float64{10, 20, 26, 36}},
	})
	if err != nil {
		t.Fatalf("AnadirCamposPDF: %v", err)
	}
	d, err := LeerPDF(base)
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	if len(valores) == 0 {
		return d
	}
	relleno, err := RellenarFormularioPDF(d, valores)
	if err != nil {
		t.Fatalf("RellenarFormularioPDF: %v", err)
	}
	d2, err := LeerPDF(relleno)
	if err != nil {
		t.Fatalf("LeerPDF del relleno: %v", err)
	}
	return d2
}

// TestXFDFVaYVuelveSinPerderNada es EL check: el uso real es exportar, tocar
// fuera y volver a importar.
func TestXFDFVaYVuelveSinPerderNada(t *testing.T) {
	// Con lo que rompe un XML mal escrito: acentos, comillas, ampersand y menor.
	quiero := map[string]string{
		"nombre": `Ana "Pérez" & Cía. <sección>`,
		"notas":  "línea uno\nlínea dos",
	}
	d := formularioDePrueba(t, quiero)

	xfdf, err := d.ExportarXFDF("solicitud.pdf")
	if err != nil {
		t.Fatalf("ExportarXFDF: %v", err)
	}
	if !strings.Contains(string(xfdf), `xmlns="http://ns.adobe.com/xfdf/"`) {
		t.Errorf("falta el espacio de nombres del estándar: %s", xfdf)
	}
	if !strings.Contains(string(xfdf), `href="solicitud.pdf"`) {
		t.Errorf("falta el nombre del documento: %s", xfdf)
	}

	vuelta, err := ImportarXFDF(xfdf)
	if err != nil {
		t.Fatalf("ImportarXFDF: %v", err)
	}
	for k, v := range quiero {
		if vuelta[k] != v {
			t.Errorf("%s = %q, quería %q", k, vuelta[k], v)
		}
	}
}

// Y lo que se importa se puede APLICAR: sin esto la vuelta es un ejercicio de
// serialización que no rellena nada.
func TestLoImportadoRellenaElFormulario(t *testing.T) {
	d := formularioDePrueba(t, nil)
	xfdf := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<xfdf xmlns="http://ns.adobe.com/xfdf/"><fields>
  <field name="nombre"><value>Luis Gil</value></field>
  <field name="acepto"><value>Yes</value></field>
</fields></xfdf>`)

	valores, err := ImportarXFDF(xfdf)
	if err != nil {
		t.Fatalf("ImportarXFDF: %v", err)
	}
	salida, err := RellenarFormularioPDF(d, valores)
	if err != nil {
		t.Fatalf("RellenarFormularioPDF: %v", err)
	}
	d2, err := LeerPDF(salida)
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	c, ok := campoPorNombre(d2.CamposDelFormulario(), "nombre")
	if !ok || c.Valor != "Luis Gil" {
		t.Errorf("el valor importado no llegó al documento: %+v", c)
	}
}

// TestXFDFConCamposAnidados: el estándar permite agrupar y el nombre completo se
// forma con puntos. Un impreso de administración usa esto para «solicitante.dni».
func TestXFDFConCamposAnidados(t *testing.T) {
	valores, err := ImportarXFDF([]byte(`<?xml version="1.0"?>
<xfdf xmlns="http://ns.adobe.com/xfdf/"><fields>
  <field name="solicitante">
    <field name="dni"><value>12345678Z</value></field>
    <field name="nombre"><value>Ana</value></field>
  </field>
</fields></xfdf>`))
	if err != nil {
		t.Fatalf("ImportarXFDF: %v", err)
	}
	if valores["solicitante.dni"] != "12345678Z" {
		t.Errorf("valores = %v, quería solicitante.dni", valores)
	}
	if valores["solicitante.nombre"] != "Ana" {
		t.Errorf("valores = %v, quería solicitante.nombre", valores)
	}
}

// TestXFDFRechazaLoQueNoLoEs: el fichero lo aporta quien llama.
func TestXFDFRechazaLoQueNoLoEs(t *testing.T) {
	casos := map[string][]byte{
		"vacío":            {},
		"no es XML":        []byte("esto no es xml"),
		"XML sin campos":   []byte(`<?xml version="1.0"?><xfdf xmlns="http://ns.adobe.com/xfdf/"><fields/></xfdf>`),
		"demasiado grande": make([]byte, maxXFDFBytes+1),
	}
	for nombre, datos := range casos {
		if _, err := ImportarXFDF(datos); err == nil {
			t.Errorf("%s: debería fallar", nombre)
		}
	}
}

// TestXFDFNoExpandeEntidades: la defensa contra XXE y «billion laughs» es que el
// lector de la stdlib no sigue DOCTYPE ni resuelve entidades externas. Se
// comprueba porque es la clase de propiedad que se pierde al cambiar de lector.
func TestXFDFNoExpandeEntidades(t *testing.T) {
	ataque := []byte(`<?xml version="1.0"?>
<!DOCTYPE xfdf [<!ENTITY xxe SYSTEM "file:///etc/passwd">]>
<xfdf xmlns="http://ns.adobe.com/xfdf/"><fields>
  <field name="x"><value>&xxe;</value></field>
</fields></xfdf>`)
	valores, err := ImportarXFDF(ataque)
	if err != nil {
		return // rechazarlo también es correcto
	}
	if strings.Contains(valores["x"], "root:") {
		t.Fatalf("se resolvió una entidad externa: %q", valores["x"])
	}
}

// TestExportarSinCamposNoInventaUnFichero: un PDF normal no es un formulario, y
// darle un XFDF vacío haría creer que sí.
func TestExportarSinCamposNoInventaUnFichero(t *testing.T) {
	if _, err := docDePrueba(t, "no es un formulario").ExportarXFDF("x.pdf"); err == nil {
		t.Error("un documento sin campos no puede exportar datos de formulario")
	}
}
