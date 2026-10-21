// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Checks de anotaciones y formularios. Casi todos van hasta el final —se escribe
// el PDF, se vuelve a leer y se RENDERIZA— porque los dos fallos que de verdad
// duelen aquí son silenciosos: una anotación que se guarda y no se ve, y un
// campo que se rellena en el dato y sale vacío en la pantalla.
package domain

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func anotarYLeer(t *testing.T, doc *PDFDoc, anots []Anotacion) *PDFDoc {
	t.Helper()
	salida, err := AnadirAnotacionesPDF(doc, anots)
	if err != nil {
		t.Fatalf("AnadirAnotacionesPDF: %v", err)
	}
	d, err := LeerPDF(salida)
	if err != nil {
		t.Fatalf("el PDF anotado no abre: %v", err)
	}
	return d
}

// --- Anotaciones -------------------------------------------------------------

func TestResaltadoSeGuardaYSeVe(t *testing.T) {
	d := anotarYLeer(t, docDePrueba(t, "un parrafo cualquiera"), []Anotacion{{
		Tipo:     AnotResaltado,
		Cuadros:  [][4]float64{{100, 700, 300, 715}},
		Color:    [3]float64{1, 0.9, 0.2},
		Opacidad: 0.4,
	}})

	leidas := d.AnotacionesDelDocumento()
	if len(leidas) != 1 || leidas[0].Tipo != "Highlight" {
		t.Fatalf("anotaciones leídas = %+v", leidas)
	}

	// Y sobre todo: que se PINTE. Una anotación sin apariencia se guarda igual y
	// no la enseña ni el visor del navegador ni el del móvil.
	svg, err := d.RenderizarPaginaSVG(0, OpcionesRender{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(svg, "#ffe633") {
		t.Errorf("el resaltado no llegó al render: %s", primerasLineas(svg))
	}
	// Multiplicar es lo que deja leer el texto por debajo del color.
	if !strings.Contains(svg, "mix-blend-mode:multiply") {
		t.Errorf("un resaltado que TAPA el texto no sirve: %s", primerasLineas(svg))
	}
}

func TestTintaCalculaSuCajaSola(t *testing.T) {
	// Sin /Rect: es el error más fácil desde el cliente, y una anotación con caja
	// vacía no la enseña ningún lector.
	d := anotarYLeer(t, docDePrueba(t, "hoja"), []Anotacion{{
		Tipo:   AnotTinta,
		Trazos: [][][2]float64{{{100, 100}, {150, 160}, {200, 100}}},
		Color:  [3]float64{0, 0, 1},
		Grosor: 3,
	}})
	leidas := d.AnotacionesDelDocumento()
	if len(leidas) != 1 {
		t.Fatalf("anotaciones = %+v", leidas)
	}
	r := leidas[0].Rect
	if r[0] > 100 || r[1] > 100 || r[2] < 200 || r[3] < 160 {
		t.Errorf("la caja %v no contiene el trazo", r)
	}
	svg, _ := d.RenderizarPaginaSVG(0, OpcionesRender{})
	if !strings.Contains(svg, "#0000ff") {
		t.Errorf("el trazo azul no se pintó: %s", primerasLineas(svg))
	}
}

func TestAnotacionSeImprime(t *testing.T) {
	// El bit 3 de /F. Sin él la anotación se ve en pantalla y DESAPARECE al
	// imprimir, que es la queja clásica de quien revisa en papel.
	salida, err := AnadirAnotacionesPDF(docDePrueba(t, "x"), []Anotacion{{
		Tipo: AnotRectangulo, Rect: [4]float64{10, 10, 50, 50},
	}})
	if err != nil {
		t.Fatalf("AnadirAnotacionesPDF: %v", err)
	}
	if !strings.Contains(string(salida), "/F 4") {
		t.Error("la anotación no lleva la bandera de imprimir")
	}
}

func TestAnotacionesConservanLasQueYaHabia(t *testing.T) {
	d := anotarYLeer(t, docDePrueba(t, "hoja"), []Anotacion{{
		Tipo: AnotNota, Rect: [4]float64{10, 10, 30, 30}, Texto: "primera",
	}})
	d2 := anotarYLeer(t, d, []Anotacion{{
		Tipo: AnotNota, Rect: [4]float64{50, 50, 70, 70}, Texto: "segunda",
	}})
	leidas := d2.AnotacionesDelDocumento()
	if len(leidas) != 2 {
		t.Fatalf("esperaba 2 anotaciones, hay %d: %+v", len(leidas), leidas)
	}
}

func TestAnotacionConservaAutorYTextoConTildes(t *testing.T) {
	d := anotarYLeer(t, docDePrueba(t, "hoja"), []Anotacion{{
		Tipo:  AnotNota,
		Rect:  [4]float64{10, 10, 30, 30},
		Texto: "Revisar la cláusula 3ª — año 2026",
		Autor: "Begoña Ríos",
	}})
	leidas := d.AnotacionesDelDocumento()
	if len(leidas) != 1 {
		t.Fatalf("anotaciones = %+v", leidas)
	}
	if leidas[0].Autor != "Begoña Ríos" {
		t.Errorf("autor = %q", leidas[0].Autor)
	}
	if !strings.Contains(leidas[0].Texto, "cláusula 3ª") {
		t.Errorf("texto = %q", leidas[0].Texto)
	}
}

func TestBorrarAnotacion(t *testing.T) {
	d := anotarYLeer(t, docDePrueba(t, "hoja"), []Anotacion{
		{Tipo: AnotNota, Rect: [4]float64{10, 10, 30, 30}, Texto: "una"},
		{Tipo: AnotNota, Rect: [4]float64{50, 50, 70, 70}, Texto: "otra"},
	})
	salida, err := BorrarAnotacionesPDF(d, []int{0})
	if err != nil {
		t.Fatalf("BorrarAnotacionesPDF: %v", err)
	}
	d2, err := LeerPDF(salida)
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	leidas := d2.AnotacionesDelDocumento()
	if len(leidas) != 1 || leidas[0].Texto != "otra" {
		t.Errorf("tras borrar la primera quedan: %+v", leidas)
	}
}

func TestAnotacionEnPaginaInexistenteFalla(t *testing.T) {
	_, err := AnadirAnotacionesPDF(docDePrueba(t, "una"), []Anotacion{{
		Tipo: AnotNota, Pagina: 7, Rect: [4]float64{0, 0, 10, 10},
	}})
	if err == nil {
		t.Error("una anotación en una página que no existe debe fallar, no perderse en silencio")
	}
}

func TestFlechaYMedidaSeDibujan(t *testing.T) {
	d := anotarYLeer(t, docDePrueba(t, "plano"), []Anotacion{
		{Tipo: AnotFlecha, Puntos: [][2]float64{{100, 100}, {200, 200}}, Color: [3]float64{1, 0, 0}},
		{
			Tipo: AnotMedida, Puntos: [][2]float64{{100, 300}, {300, 300}}, Etiqueta: "2,80 m",
			Color: [3]float64{0, 0.5, 1},
		},
	})
	svg, err := d.RenderizarPaginaSVG(0, OpcionesRender{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(svg, "#ff0000") || !strings.Contains(svg, "#0080ff") {
		t.Errorf("faltan la flecha o la cota: %s", primerasLineas(svg))
	}
	if !strings.Contains(svg, "2,80 m") {
		t.Errorf("la etiqueta de la medición no se pintó: %s", primerasLineas(svg))
	}
}

func TestAplanarAnotacionesLasDejaEnElContenido(t *testing.T) {
	d := anotarYLeer(t, docDePrueba(t, "hoja"), []Anotacion{{
		Tipo: AnotRectangulo, Rect: [4]float64{50, 50, 150, 150}, Color: [3]float64{1, 0, 0},
	}})
	salida, err := AplanarAnotacionesPDF(d)
	if err != nil {
		t.Fatalf("AplanarAnotacionesPDF: %v", err)
	}
	d2, err := LeerPDF(salida)
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	if len(d2.AnotacionesDelDocumento()) != 0 {
		t.Error("después de aplanar no puede quedar ninguna anotación suelta")
	}
	// Y sin embargo se tiene que seguir viendo: ahí está la gracia de aplanar.
	svg, err := d2.RenderizarPaginaSVG(0, OpcionesRender{SinAnotaciones: true})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(svg, "#ff0000") {
		t.Errorf("aplanar perdió el dibujo: %s", primerasLineas(svg))
	}
}

// --- Formularios -------------------------------------------------------------

// pdfConFormulario arma un PDF con un campo de texto y una casilla cuyo estado
// activado NO se llama «Yes», que es donde falla casi todo el mundo.
func pdfConFormulario(t *testing.T) []byte {
	t.Helper()
	apOn := "0 g 0 0 14 14 re f"
	apOff := "1 g 0 0 14 14 re f"
	objs := []string{
		`<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [5 0 R 6 0 R] /DA (/Helv 0 Tf 0 g) >> >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 >>`,
		`<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 300] /Contents 4 0 R /Annots [5 0 R 6 0 R] >>`,
		"<< /Length 0 >>\nstream\n\nendstream",
		`<< /Type /Annot /Subtype /Widget /FT /Tx /T (nombre) /TU (Nombre completo) ` +
			`/Rect [20 200 200 220] /DA (/Helv 10 Tf 0 g) /Ff 2 >>`,
		`<< /Type /Annot /Subtype /Widget /FT /Btn /T (acepto) /Rect [20 150 34 164] ` +
			`/AP << /N << /Marcado 7 0 R /Off 8 0 R >> >> /AS /Off >>`,
		fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 14 14] /Length %d >>\nstream\n%s\nendstream",
			len(apOn), apOn),
		fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 14 14] /Length %d >>\nstream\n%s\nendstream",
			len(apOff), apOff),
	}
	return pdfMinimo(t, `<< /Size 9 /Root 1 0 R >>`, objs)
}

func TestLeerCamposDelFormulario(t *testing.T) {
	d, err := LeerPDF(pdfConFormulario(t))
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	campos := d.CamposDelFormulario()
	if len(campos) != 2 {
		t.Fatalf("esperaba 2 campos, hay %d: %+v", len(campos), campos)
	}

	texto := campos[0]
	if texto.Nombre != "nombre" || texto.Tipo != CampoTexto {
		t.Errorf("campo de texto = %+v", texto)
	}
	if texto.Etiqueta != "Nombre completo" {
		t.Errorf("la etiqueta (/TU) no se leyó: %q", texto.Etiqueta)
	}
	if !texto.Obligatorio {
		t.Error("el bit 2 de /Ff marca obligatorio y no se leyó")
	}
	if texto.Rect != [4]float64{20, 200, 200, 220} {
		t.Errorf("rect = %v", texto.Rect)
	}

	casilla := campos[1]
	if casilla.Tipo != CampoCasilla {
		t.Errorf("tipo de la casilla = %q", casilla.Tipo)
	}
	// El estado activado NO es «Yes»: cada generador elige el suyo, y ponerlo mal
	// deja la casilla marcada en el dato y vacía en la pantalla.
	if casilla.EstadoActivado != "Marcado" {
		t.Errorf("estado activado = %q, quería «Marcado»", casilla.EstadoActivado)
	}
}

func TestRellenarCampoDeTexto(t *testing.T) {
	d, err := LeerPDF(pdfConFormulario(t))
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	salida, err := RellenarFormularioPDF(d, map[string]string{"nombre": "Ana Pérez"})
	if err != nil {
		t.Fatalf("RellenarFormularioPDF: %v", err)
	}
	d2, err := LeerPDF(salida)
	if err != nil {
		t.Fatalf("el PDF relleno no abre: %v", err)
	}

	campos := d2.CamposDelFormulario()
	if len(campos) != 2 {
		t.Fatalf("el formulario se perdió al rellenarlo: %+v", campos)
	}
	if campos[0].Valor != "Ana Pérez" {
		t.Errorf("valor = %q", campos[0].Valor)
	}
	// Y que se VEA: un campo relleno solo en el dato sale vacío al imprimir.
	svg, err := d2.RenderizarPaginaSVG(0, OpcionesRender{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(svg, "Ana P") {
		t.Errorf("el valor no se dibujó: %s", primerasLineas(svg))
	}
}

func TestMarcarCasillaUsaSuEstadoPropio(t *testing.T) {
	d, err := LeerPDF(pdfConFormulario(t))
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	salida, err := RellenarFormularioPDF(d, map[string]string{"acepto": "si"})
	if err != nil {
		t.Fatalf("RellenarFormularioPDF: %v", err)
	}
	if !strings.Contains(string(salida), "/Marcado") {
		t.Error("la casilla no se marcó con SU estado")
	}
	d2, err := LeerPDF(salida)
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	if v := d2.CamposDelFormulario()[1].Valor; v != "Marcado" {
		t.Errorf("valor de la casilla = %q", v)
	}
}

func TestRellenarCampoInexistenteFalla(t *testing.T) {
	d, err := LeerPDF(pdfConFormulario(t))
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	if _, err := RellenarFormularioPDF(d, map[string]string{"noexiste": "x"}); err == nil {
		t.Error("rellenar un campo que no existe debe fallar, no perderse en silencio")
	}
}

// TestElFormularioSobreviveAReordenarPaginas cubre un fallo latente del
// ensamblador: sin conservar el /AcroForm, el documento deja de SER un
// formulario aunque los campos se sigan viendo, y nada lo dice.
func TestElFormularioSobreviveAReordenarPaginas(t *testing.T) {
	d, err := LeerPDF(pdfConFormulario(t))
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	e := NuevoEnsambladorPDF()
	if err := e.AnadirPagina(d, 0, 0); err != nil {
		t.Fatalf("AnadirPagina: %v", err)
	}
	e.ConservarFormulario(d)
	salida, err := e.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	d2, err := LeerPDF(salida)
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	if n := len(d2.CamposDelFormulario()); n != 2 {
		t.Errorf("tras reensamblar quedan %d campos de 2", n)
	}
}

func TestAplanarFormularioDejaDeSerFormulario(t *testing.T) {
	d, err := LeerPDF(pdfConFormulario(t))
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	relleno, err := RellenarFormularioPDF(d, map[string]string{"nombre": "Ana"})
	if err != nil {
		t.Fatalf("RellenarFormularioPDF: %v", err)
	}
	d2, err := LeerPDF(relleno)
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	plano, err := AplanarFormularioPDF(d2)
	if err != nil {
		t.Fatalf("AplanarFormularioPDF: %v", err)
	}
	d3, err := LeerPDF(plano)
	if err != nil {
		t.Fatalf("LeerPDF del aplanado: %v", err)
	}
	if n := len(d3.CamposDelFormulario()); n != 0 {
		t.Errorf("después de aplanar quedan %d campos y no debería quedar ninguno", n)
	}
	// El dato se queda escrito en la página, que es justo el objetivo: quien lo
	// recibe ya no puede cambiarlo sin dejar rastro.
	svg, err := d3.RenderizarPaginaSVG(0, OpcionesRender{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(svg, "Ana") {
		t.Errorf("aplanar perdió el valor: %s", primerasLineas(svg))
	}
}

func TestFirmarDibujado(t *testing.T) {
	salida, err := FirmarDibujadoPDF(docDePrueba(t, "contrato"), Firma{
		Rect:   [4]float64{100, 100, 300, 160},
		Trazos: [][][2]float64{{{110, 120}, {150, 145}, {190, 110}, {230, 140}}},
		Pie:    "Ana Pérez — 13/08/2026",
	})
	if err != nil {
		t.Fatalf("FirmarDibujadoPDF: %v", err)
	}
	d, err := LeerPDF(salida)
	if err != nil {
		t.Fatalf("el PDF firmado no abre: %v", err)
	}
	// Aplanada: una firma que se puede arrastrar a otro sitio no es una firma.
	if n := len(d.AnotacionesDelDocumento()); n != 0 {
		t.Errorf("la firma quedó como anotación movible (%d)", n)
	}
	svg, err := d.RenderizarPaginaSVG(0, OpcionesRender{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(svg, "Ana P") {
		t.Errorf("el pie de firma no se ve: %s", primerasLineas(svg))
	}
}

func TestFirmaSinTrazoNiImagenFalla(t *testing.T) {
	if _, err := FirmarDibujadoPDF(docDePrueba(t, "x"), Firma{Rect: [4]float64{0, 0, 10, 10}}); err == nil {
		t.Error("una firma vacía no es una firma")
	}
}

func TestInterpretarDA(t *testing.T) {
	tam, color := interpretarDA("/Helv 12 Tf 0.2 0.4 0.6 rg")
	if tam != 12 {
		t.Errorf("tamaño = %v, quería 12", tam)
	}
	if color[0] < 0.19 || color[2] > 0.61 {
		t.Errorf("color = %v", color)
	}
	if tam, c := interpretarDA("/Helv 0 Tf 0 g"); tam != 0 || c != [3]float64{0, 0, 0} {
		t.Errorf("tamaño 0 (auto) y gris: %v %v", tam, c)
	}
}

func TestEsAfirmativo(t *testing.T) {
	for _, v := range []string{"si", "yes", "1", "true", "on", "Marcado"} {
		if !esAfirmativo(v) {
			t.Errorf("%q debería contar como marcado", v)
		}
	}
	for _, v := range []string{"", "no", "0", "off", "false"} {
		if esAfirmativo(v) {
			t.Errorf("%q NO debería contar como marcado", v)
		}
	}
}

// TestFirmaEnTodasLasPaginas: un contrato de varias hojas firmado solo en la
// última deja las demás sin marcar, y ahí es donde se cambian.
func TestFirmaEnTodasLasPaginas(t *testing.T) {
	salida, err := FirmarDibujadoPDF(docDePrueba(t, "hoja 1", "hoja 2", "hoja 3"), Firma{
		Rect:            [4]float64{100, 100, 300, 160},
		Trazos:          [][][2]float64{{{110, 120}, {150, 145}, {190, 110}}},
		Pie:             "Ana Pérez",
		TodasLasPaginas: true,
	})
	if err != nil {
		t.Fatalf("FirmarDibujadoPDF: %v", err)
	}
	d, err := LeerPDF(salida)
	if err != nil {
		t.Fatalf("el PDF firmado no abre: %v", err)
	}
	if n := d.NumPaginas(); n != 3 {
		t.Fatalf("páginas = %d, quería 3", n)
	}
	for i := 0; i < 3; i++ {
		svg, err := d.RenderizarPaginaSVG(i, OpcionesRender{})
		if err != nil {
			t.Fatalf("render de la página %d: %v", i+1, err)
		}
		if !strings.Contains(svg, "Ana P") {
			t.Errorf("la página %d se quedó sin rúbrica: %s", i+1, primerasLineas(svg))
		}
	}
}

// Y sin pedirlo, solo la suya: firmar todas las hojas de un documento de
// cincuenta páginas cuando se quería una es tan malo como lo contrario.
func TestFirmaSoloEnSuPagina(t *testing.T) {
	salida, err := FirmarDibujadoPDF(docDePrueba(t, "hoja 1", "hoja 2"), Firma{
		Pagina: 1,
		Rect:   [4]float64{100, 100, 300, 160},
		Trazos: [][][2]float64{{{110, 120}, {150, 145}}},
		Pie:    "Ana Pérez",
	})
	if err != nil {
		t.Fatalf("FirmarDibujadoPDF: %v", err)
	}
	d, err := LeerPDF(salida)
	if err != nil {
		t.Fatalf("el PDF firmado no abre: %v", err)
	}
	primera, err := d.RenderizarPaginaSVG(0, OpcionesRender{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(primera, "Ana P") {
		t.Error("la rúbrica salió en una página que no se pidió")
	}
}

// Con /NeedAppearances el lector tiraba nuestras apariencias y redibujaba con la
// fuente de /DA (Poppler: «couldn't find a font for character U+00D1», «Unknown
// font tag 'ZaDb'»). Ahora se dibujan todos los campos con valor —también los
// que ya lo tenían— y la marca se quita.
func TestRellenarConNeedAppearancesDibujaTodoYQuitaLaMarca(t *testing.T) {
	datos := bytes.Replace(pdfConFormulario(t), []byte("/AcroForm << /Fields [5 0 R 6 0 R]"),
		[]byte("/AcroForm << /NeedAppearances true /Fields [5 0 R 6 0 R 9 0 R]"), 1)
	datos = bytes.Replace(datos, []byte("/Annots [5 0 R 6 0 R]"), []byte("/Annots [5 0 R 6 0 R 9 0 R]"), 1)
	// Un tercer campo que ya trae valor y no trae dibujo: lo dibujaba el lector.
	i := bytes.Index(datos, []byte("xref\n"))
	ciudad := []byte("9 0 obj\n<< /Type /Annot /Subtype /Widget /FT /Tx /T (ciudad) /V (Sevilla) " +
		"/Rect [20 100 200 120] /DA (/Helv 10 Tf 0 g) >>\nendobj\n")
	datos = append(datos[:i:i], append(ciudad, datos[i:]...)...)
	d, err := LeerPDF(datos) // la xref ya no cuadra: se reconstruye
	if err != nil {
		t.Fatal(err)
	}
	salida, err := RellenarFormularioPDF(d, map[string]string{"nombre": "Ñuño", "acepto": "Marcado"})
	if err != nil {
		t.Fatal(err)
	}
	d2, err := LeerPDF(salida)
	if err != nil {
		t.Fatal(err)
	}
	raiz, _ := d2.resolver(d2.trailer["Root"]).(pdfDict)
	forma, _ := d2.resolver(raiz["AcroForm"]).(pdfDict)
	if forma["NeedAppearances"] != nil {
		t.Error("el formulario sigue pidiendo al lector que redibuje los campos")
	}
	p, _ := d2.Pagina(0)
	annots, _ := d2.resolver(p.dict["Annots"]).(pdfArray)
	for _, a := range annots {
		w, _ := d2.resolver(a).(pdfDict)
		ap, _ := d2.resolver(w["AP"]).(pdfDict)
		if ap == nil || ap["N"] == nil {
			t.Errorf("el campo %v no lleva apariencia", w["T"])
		}
	}
}

// El valor de un campo relleno se ve en la página, pero vive en la apariencia
// del widget: el texto de la página no lo traía y buscarlo no daba nada
// (formularios de PDFBox y pdf.js en scripts/certipdf-compat, Poppler sí lo saca).
func TestTextoDePaginaIncluyeLasAnotaciones(t *testing.T) {
	d, err := LeerPDF(pdfConFormulario(t))
	if err != nil {
		t.Fatal(err)
	}
	relleno, err := RellenarFormularioPDF(d, map[string]string{"nombre": "Sevilla"})
	if err != nil {
		t.Fatal(err)
	}
	d2, err := LeerPDF(relleno)
	if err != nil {
		t.Fatal(err)
	}
	if txt, err := d2.TextoPlano(0); err != nil || !strings.Contains(txt, "Sevilla") {
		t.Errorf("TextoPlano = %q, %v; quería el valor del campo", txt, err)
	}
	if txt, err := d2.TextoDelContenido(0); err != nil || strings.Contains(txt, "Sevilla") {
		t.Errorf("TextoDelContenido = %q, %v; no debía traer el campo", txt, err)
	}
}

// En un grupo de radio se marcaban TODOS los widgets con el estado elegido,
// aunque solo uno lo dibuja: los otros quedaban sin dibujo y Poppler los rehacía
// con la ZapfDingbats del /DA (annotations-rotated-180 de qpdf, 36 salidas del
// corpus con «Unknown font tag 'ZaDb'»). Ahora se marca solo el que lo dibuja, y
// el valor puede nombrar el estado («2»).
func TestRellenarRadioMarcaSoloSuWidget(t *testing.T) {
	ap := "0 g 0 0 10 10 re f"
	objs := []string{
		`<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [5 0 R] >> >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 >>`,
		`<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 300] /Contents 4 0 R /Annots [6 0 R 7 0 R 8 0 R] >>`,
		"<< /Length 0 >>\nstream\n\nendstream",
		`<< /FT /Btn /Ff 49152 /T (r1) /V /1 /Kids [6 0 R 7 0 R 8 0 R] >>`,
		`<< /Type /Annot /Subtype /Widget /Parent 5 0 R /Rect [10 10 20 20] /AS /1 /AP << /N << /1 9 0 R /Off 9 0 R >> >> >>`,
		`<< /Type /Annot /Subtype /Widget /Parent 5 0 R /Rect [30 10 40 20] /AS /Off /AP << /N << /2 9 0 R /Off 9 0 R >> >> >>`,
		`<< /Type /Annot /Subtype /Widget /Parent 5 0 R /Rect [50 10 60 20] /AS /Off /AP << /N << /3 9 0 R /Off 9 0 R >> >> >>`,
		fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 10 10] /Length %d >>\nstream\n%s\nendstream", len(ap), ap),
	}
	d, err := LeerPDF(pdfMinimo(t, `<< /Size 10 /Root 1 0 R >>`, objs))
	if err != nil {
		t.Fatal(err)
	}
	salida, err := RellenarFormularioPDF(d, map[string]string{"r1": "2"})
	if err != nil {
		t.Fatal(err)
	}
	d2, err := LeerPDF(salida)
	if err != nil {
		t.Fatal(err)
	}
	p, _ := d2.Pagina(0)
	annots, _ := d2.resolver(p.dict["Annots"]).(pdfArray)
	estados := make([]pdfName, 0, len(annots))
	for _, a := range annots {
		w, _ := d2.resolver(a).(pdfDict)
		as, _ := w["AS"].(pdfName)
		estados = append(estados, as)
		padre, _ := d2.resolver(w["Parent"]).(pdfDict)
		if v, _ := padre["V"].(pdfName); v != "2" {
			t.Errorf("/V = %q, quería /2", v)
		}
	}
	if fmt.Sprint(estados) != "[Off 2 Off]" {
		t.Errorf("/AS de los tres widgets = %v, quería [Off 2 Off]", estados)
	}
}

// Con /Opt el valor es el de exportación: «Female» en ["Male" "Female"] marca el
// segundo widget, cuyo estado se llama «1» (AcrobatMerge-* de PDFBox, opt_demo
// de pdf.js: se marcaba siempre el primero).
func TestRellenarRadioConOpt(t *testing.T) {
	ap := "0 g 0 0 10 10 re f"
	objs := []string{
		`<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [5 0 R] >> >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 >>`,
		`<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 300] /Contents 4 0 R /Annots [6 0 R 7 0 R] >>`,
		"<< /Length 0 >>\nstream\n\nendstream",
		`<< /FT /Btn /Ff 49152 /T (Gender) /Opt [(Male) (Female)] /V /Off /Kids [6 0 R 7 0 R] >>`,
		`<< /Type /Annot /Subtype /Widget /Parent 5 0 R /Rect [10 10 20 20] /AS /Off /AP << /N << /0 8 0 R /Off 8 0 R >> >> >>`,
		`<< /Type /Annot /Subtype /Widget /Parent 5 0 R /Rect [30 10 40 20] /AS /Off /AP << /N << /1 8 0 R /Off 8 0 R >> >> >>`,
		fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 10 10] /Length %d >>\nstream\n%s\nendstream", len(ap), ap),
	}
	d, err := LeerPDF(pdfMinimo(t, `<< /Size 9 /Root 1 0 R >>`, objs))
	if err != nil {
		t.Fatal(err)
	}
	salida, err := RellenarFormularioPDF(d, map[string]string{"Gender": "Female"})
	if err != nil {
		t.Fatal(err)
	}
	d2, err := LeerPDF(salida)
	if err != nil {
		t.Fatal(err)
	}
	p, _ := d2.Pagina(0)
	annots, _ := d2.resolver(p.dict["Annots"]).(pdfArray)
	estados := make([]pdfName, 0, len(annots))
	for _, a := range annots {
		w, _ := d2.resolver(a).(pdfDict)
		as, _ := w["AS"].(pdfName)
		estados = append(estados, as)
	}
	if fmt.Sprint(estados) != "[Off 1]" {
		t.Errorf("/AS = %v, quería [Off 1]", estados)
	}
}

// Con /NeedAppearances el documento avisa de que sus apariencias no valen (aquí
// vacías, como en los formularios de qpdf): el valor se dibuja desde /V, y una
// lista enseña sus opciones. Antes la página salía sin «abc» ni las opciones.
func TestNeedAppearancesRedibujaDesdeElValor(t *testing.T) {
	vacia := "/Tx BMC\nEMC\n"
	pdf := func(need string) []byte {
		objs := []string{
			`<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [5 0 R 6 0 R] /DA (/Helv 0 Tf 0 g)` + need + ` >> >>`,
			`<< /Type /Pages /Kids [3 0 R] /Count 1 >>`,
			`<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 300] /Contents 4 0 R /Annots [5 0 R 6 0 R] >>`,
			"<< /Length 0 >>\nstream\n\nendstream",
			`<< /Type /Annot /Subtype /Widget /FT /Tx /T (t) /V (abc) /Rect [20 200 200 220] /AP << /N 7 0 R >> >>`,
			`<< /Type /Annot /Subtype /Widget /FT /Ch /T (l) /V (six) /Opt [(five) (six)] /Rect [20 100 200 160] /AP << /N 7 0 R >> >>`,
			fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 180 20] /Length %d >>\nstream\n%s\nendstream", len(vacia), vacia),
		}
		return pdfMinimo(t, `<< /Size 8 /Root 1 0 R >>`, objs)
	}
	for _, caso := range []struct {
		need, quiero string
	}{{" /NeedAppearances true", "abc\nfive\nsix"}, {"", ""}} {
		d, err := LeerPDF(pdf(caso.need))
		if err != nil {
			t.Fatal(err)
		}
		texto, err := d.TextoPlano(0)
		if err != nil {
			t.Fatal(err)
		}
		if texto != caso.quiero {
			t.Errorf("con %q: texto = %q, quería %q", caso.need, texto, caso.quiero)
		}
	}
}

// Un /FreeText sin /AP (así lo escribe pypdf) lo dibuja el lector desde
// /Contents; antes no se veía ni salía en el texto.
func TestTextoLibreSinAparienciaSeDibuja(t *testing.T) {
	frags := fragmentosDe(t, ``, `/Annots [5 0 R]`,
		`<< /Type /Annot /Subtype /FreeText /Rect [50 50 250 80] /Contents (A\361adido despu\351s) /DA (0 0 1 rg /Helv 10 Tf) >>`)
	if len(frags) != 1 || frags[0].Texto != "Añadido después" {
		t.Fatalf("fragmentos = %+v, quería «Añadido después»", frags)
	}
}
