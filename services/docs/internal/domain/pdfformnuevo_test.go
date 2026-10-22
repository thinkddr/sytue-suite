// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"strings"
	"testing"
)

// campoPorNombre busca un campo en la lista leída.
func campoPorNombre(campos []CampoFormulario, nombre string) (CampoFormulario, bool) {
	for _, c := range campos {
		if c.Nombre == nombre {
			return c, true
		}
	}
	return CampoFormulario{}, false
}

// TestCamposCreadosSeLeenYSeRellenan es EL check de esta pieza: crear un campo
// que después no se pueda rellenar no convierte un impreso en un formulario, que
// es lo único para lo que existe.
func TestCamposCreadosSeLeenYSeRellenan(t *testing.T) {
	salida, err := AnadirCamposPDF(docDePrueba(t, "solicitud de plaza"), []CampoNuevo{
		{Tipo: CampoTexto, Nombre: "nombre", Pagina: 0, Rect: [4]float64{50, 700, 300, 722}},
		{Tipo: CampoCasilla, Nombre: "acepto", Pagina: 0, Rect: [4]float64{50, 650, 66, 666}},
		{
			Tipo: CampoDesplegable, Nombre: "turno", Pagina: 0, Rect: [4]float64{50, 600, 200, 622},
			Opciones: []string{"mañana", "tarde"},
		},
	})
	if err != nil {
		t.Fatalf("AnadirCamposPDF: %v", err)
	}

	d, err := LeerPDF(salida)
	if err != nil {
		t.Fatalf("el PDF con campos no abre: %v", err)
	}
	campos := d.CamposDelFormulario()
	if len(campos) != 3 {
		t.Fatalf("campos = %+v, quería 3", campos)
	}
	texto, ok := campoPorNombre(campos, "nombre")
	if !ok || texto.Tipo != CampoTexto {
		t.Errorf("el campo de texto salió como %+v", texto)
	}
	if texto.Pagina != 0 {
		t.Errorf("página = %d, quería 0", texto.Pagina)
	}
	if casilla, ok := campoPorNombre(campos, "acepto"); !ok || casilla.Tipo != CampoCasilla {
		t.Errorf("la casilla salió como %+v", casilla)
	}
	lista, ok := campoPorNombre(campos, "turno")
	if !ok || lista.Tipo != CampoDesplegable {
		t.Errorf("el desplegable salió como %+v", lista)
	}
	if len(lista.Opciones) != 2 || lista.Opciones[0] != "mañana" {
		t.Errorf("opciones = %v", lista.Opciones)
	}

	// Y ahora lo que importa: se rellena.
	relleno, err := RellenarFormularioPDF(d, map[string]string{"nombre": "Ana Pérez", "acepto": "true"})
	if err != nil {
		t.Fatalf("RellenarFormularioPDF: %v", err)
	}
	d2, err := LeerPDF(relleno)
	if err != nil {
		t.Fatalf("el formulario relleno no abre: %v", err)
	}
	v, _ := campoPorNombre(d2.CamposDelFormulario(), "nombre")
	if v.Valor != "Ana Pérez" {
		t.Errorf("valor = %q, quería «Ana Pérez»", v.Valor)
	}
	svg, err := d2.RenderizarPaginaSVG(0, OpcionesRender{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(svg, "Ana") {
		t.Error("el valor no se ve en la página: un formulario relleno que no se lee no vale")
	}
}

// TestCamposCreadosNoTocanElFicheroAnterior: se escribe por anexo, así que lo de
// antes tiene que estar intacto byte a byte. Si no, un documento firmado al que
// se le añade un campo perdería la firma.
func TestCamposCreadosNoTocanElFicheroAnterior(t *testing.T) {
	base, err := ImagenesAPDF([]ImagenAPagina{{Datos: pngDePrueba(t, 200, 100, false), Nombre: "s.png"}})
	if err != nil {
		t.Fatalf("ImagenesAPDF: %v", err)
	}
	doc, err := LeerPDF(base)
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	salida, err := AnadirCamposPDF(doc, []CampoNuevo{
		{Tipo: CampoTexto, Nombre: "x", Pagina: 0, Rect: [4]float64{10, 10, 100, 30}},
	})
	if err != nil {
		t.Fatalf("AnadirCamposPDF: %v", err)
	}
	if len(salida) <= len(base) || !bytes.Equal(salida[:len(base)], base) {
		t.Fatal("el anexo tocó los bytes originales")
	}
}

// TestDosCamposEnLaMismaPaginaSobrevivenLosDos: si cada campo reescribiera la
// página por su cuenta, en el anexo quedarían dos versiones del mismo objeto y
// solo valdría la última — el primer campo desaparecería sin ningún error.
func TestDosCamposEnLaMismaPaginaSobrevivenLosDos(t *testing.T) {
	salida, err := AnadirCamposPDF(docDePrueba(t, "impreso"), []CampoNuevo{
		{Tipo: CampoTexto, Nombre: "a", Pagina: 0, Rect: [4]float64{10, 700, 100, 720}},
		{Tipo: CampoTexto, Nombre: "b", Pagina: 0, Rect: [4]float64{10, 660, 100, 680}},
	})
	if err != nil {
		t.Fatalf("AnadirCamposPDF: %v", err)
	}
	d, err := LeerPDF(salida)
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	if n := len(d.CamposDelFormulario()); n != 2 {
		t.Fatalf("campos = %d, quería 2 (se perdió uno al reescribir la página)", n)
	}
	// Estar en /AcroForm no basta: un widget que no cuelga del /Annots de su
	// página no se ve ni se puede pulsar, y el campo se cuenta igual. Se mira la
	// página, que es donde se pierde de verdad.
	annots, _ := d.resolver(d.paginas[0].dict["Annots"]).(pdfArray)
	if len(annots) != 2 {
		t.Fatalf("la página tiene %d anotación(es), quería 2: un campo quedó suelto", len(annots))
	}
}

// TestNombreDeCampoRepetidoSeDesambigua: dos campos con el mismo /T son el MISMO
// campo para un lector, así que escribir en uno cambiaría el otro.
func TestNombreDeCampoRepetidoSeDesambigua(t *testing.T) {
	salida, err := AnadirCamposPDF(docDePrueba(t, "impreso"), []CampoNuevo{
		{Tipo: CampoTexto, Nombre: "dni", Pagina: 0, Rect: [4]float64{10, 700, 100, 720}},
		{Tipo: CampoTexto, Nombre: "dni", Pagina: 0, Rect: [4]float64{10, 660, 100, 680}},
		{Tipo: CampoTexto, Pagina: 0, Rect: [4]float64{10, 620, 100, 640}}, // sin nombre
	})
	if err != nil {
		t.Fatalf("AnadirCamposPDF: %v", err)
	}
	d, err := LeerPDF(salida)
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	vistos := map[string]bool{}
	for _, c := range d.CamposDelFormulario() {
		if c.Nombre == "" {
			t.Error("un campo sin /T no se puede rellenar ni leer")
		}
		if vistos[c.Nombre] {
			t.Errorf("nombre repetido: %q", c.Nombre)
		}
		vistos[c.Nombre] = true
	}
	if len(vistos) != 3 {
		t.Errorf("nombres = %v, quería 3 distintos", vistos)
	}
}

// TestAnadirCamposRechazaLoImposible: decirlo antes es mejor que devolver un PDF
// con un campo de tamaño cero que nadie puede pulsar.
func TestAnadirCamposRechazaLoImposible(t *testing.T) {
	casos := map[string][]CampoNuevo{
		"sin campos": {},
		"página que no existe": {{
			Tipo: CampoTexto, Nombre: "x", Pagina: 9,
			Rect: [4]float64{10, 10, 100, 30},
		}},
		"sin tamaño": {{
			Tipo: CampoTexto, Nombre: "x", Pagina: 0,
			Rect: [4]float64{10, 10, 10, 10},
		}},
	}
	for nombre, campos := range casos {
		if _, err := AnadirCamposPDF(docDePrueba(t, "x"), campos); err == nil {
			t.Errorf("%s: debería fallar", nombre)
		}
	}
}

// TestRellenarConservaElRecuadroDelCampo: rellenar un campo sustituye su /AP
// ENTERO, y hasta el 2026-08-15 lo regeneraba sin recuadro. El campo se creaba
// visible, y en cuanto alguien escribía en él desaparecía la caja: quedaba una
// palabra suelta encima del impreso, sin nada que dijera que aquello era un
// campo. No lo veía ningún test porque el valor sí estaba y el SVG salía
// impecable —lo que faltaba era el `<path>` del borde—.
//
// El borde vuelve porque el widget lo declara en su /MK, que es de dónde lo saca
// también un lector cuando redibuja el campo (§12.5.6.19).
func TestRellenarConservaElRecuadroDelCampo(t *testing.T) {
	conCampo, err := AnadirCamposPDF(docDePrueba(t, "impreso"), []CampoNuevo{{
		Tipo: CampoTexto, Nombre: "nombre", Pagina: 0,
		Rect: [4]float64{50, 700, 300, 722},
	}})
	if err != nil {
		t.Fatalf("AnadirCamposPDF: %v", err)
	}
	d, err := LeerPDF(conCampo)
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	// El borde que pone /MK: 0.45 0.47 0.5 → #737880.
	const borde = `stroke="#737880"`
	antes, err := d.RenderizarPaginaSVG(0, OpcionesRender{})
	if err != nil {
		t.Fatalf("render del campo vacío: %v", err)
	}
	if !strings.Contains(antes, borde) {
		t.Fatalf("un campo recién creado ya salía sin recuadro:\n%s", antes)
	}

	relleno, err := RellenarFormularioPDF(d, map[string]string{"nombre": "Ana"})
	if err != nil {
		t.Fatalf("RellenarFormularioPDF: %v", err)
	}
	d2, err := LeerPDF(relleno)
	if err != nil {
		t.Fatalf("LeerPDF del relleno: %v", err)
	}
	despues, err := d2.RenderizarPaginaSVG(0, OpcionesRender{})
	if err != nil {
		t.Fatalf("render del campo relleno: %v", err)
	}
	if !strings.Contains(despues, borde) {
		t.Errorf("el recuadro se perdió al rellenar:\n%s", despues)
	}
	if !strings.Contains(despues, "Ana") {
		t.Errorf("y el valor tampoco está:\n%s", despues)
	}
}

// TestRellenarNoInventaRecuadroSinMK: el widget de un PDF ajeno que no declara
// /MK no lleva recuadro, y dibujárselo al rellenarlo cambiaría el diseño de un
// impreso que no es nuestro.
func TestRellenarNoInventaRecuadroSinMK(t *testing.T) {
	sinMK := pdfDict{
		"Type": pdfName("Annot"), "Subtype": pdfName("Widget"),
		"FT": pdfName("Tx"), "Rect": rectPDF([4]float64{10, 10, 90, 30}),
	}
	if _, ok := aparienciaDeCampo(sinMK, CampoFormulario{
		Tipo: CampoTexto, Rect: [4]float64{10, 10, 90, 30},
	}, "Ana", "0 0 0 rg /SytueCampoF 0 Tf", marcoDeMK(sinMK)); !ok {
		t.Fatal("sin /MK debe seguir dibujándose el valor")
	}
	if m := marcoDeMK(sinMK); m != nil {
		t.Errorf("marcoDeMK sin /MK = %+v, quería nil", m)
	}
	// Un /MK con los arrays VACÍOS es «sin color» en el estándar, no «por
	// defecto»: tampoco debe pintar nada.
	vacio := pdfDict{"MK": pdfDict{"BC": pdfArray{}, "BG": pdfArray{}}}
	if m := marcoDeMK(vacio); m != nil {
		t.Errorf("marcoDeMK con /MK vacío = %+v, quería nil", m)
	}
}
