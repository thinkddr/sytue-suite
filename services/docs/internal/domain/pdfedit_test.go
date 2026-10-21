// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Checks de la edición de contenido (P5). Lo que se comprueba de verdad no es
// que se pueda escribir: es que lo que NO se toca sigue exactamente igual. Un
// editor de PDF que cambia una palabra y mueve una tabla tres páginas más allá
// es peor que no tener editor.
package domain

import (
	"bytes"
	"errors"
	"slices"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pdfsimple"
)

// TestAnalizarEdicionAvisaDeLaImagenQueNoSabePintar: el render deja un recuadro
// gris donde va una imagen JPEG 2000 —no finge que ahí no hay nada— pero un gris
// sin explicación se lee como un defecto del documento. El aviso convierte «esto
// está roto» en «esto no lo sé pintar», que además dice qué hacer.
func TestAnalizarEdicionAvisaDeLaImagenQueNoSabePintar(t *testing.T) {
	jpx := "<< /Type /XObject /Subtype /Image /Width 2 /Height 2 /Filter /JPXDecode " +
		"/ColorSpace /DeviceRGB /BitsPerComponent 8 /Length 4 >>\nstream\nabcd\nendstream"
	d, err := LeerPDF(pdfConContenido(t,
		"q 50 0 0 50 10 10 cm /Im0 Do Q",
		`/Resources << /XObject << /Im0 5 0 R >> >>`, jpx))
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	a, err := d.AnalizarEdicion(0)
	if err != nil {
		t.Fatalf("AnalizarEdicion: %v", err)
	}
	if !slices.Contains(a.Avisos, "avisoImagenSinPintar") {
		t.Errorf("avisos = %v, faltaba el de la imagen que no se puede pintar", a.Avisos)
	}

	// Y al revés: un PNG normal NO debe disparar el aviso, o se vuelve ruido que
	// nadie lee —que es como se pierde el que sí importa—.
	base, err := ImagenesAPDF([]ImagenAPagina{{Datos: pngDePrueba(t, 40, 40, false), Nombre: "x.png"}})
	if err != nil {
		t.Fatalf("ImagenesAPDF: %v", err)
	}
	d2, err := LeerPDF(base)
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	a2, err := d2.AnalizarEdicion(0)
	if err != nil {
		t.Fatalf("AnalizarEdicion: %v", err)
	}
	if slices.Contains(a2.Avisos, "avisoImagenSinPintar") {
		t.Errorf("una imagen normal no debe avisar: %v", a2.Avisos)
	}
}

func TestAnalizarEdicionListaLosFragmentos(t *testing.T) {
	d := docDePrueba(t, "Primera linea")
	a, err := d.AnalizarEdicion(0)
	if err != nil {
		t.Fatalf("AnalizarEdicion: %v", err)
	}
	if len(a.Fragmentos) == 0 {
		t.Fatal("no se encontró ningún fragmento editable")
	}
	f := a.Fragmentos[0]
	if !strings.Contains(f.Texto, "Primera") {
		t.Errorf("fragmento = %q", f.Texto)
	}
	if f.AnchoDisponible <= 0 {
		t.Error("sin anchura no se puede decir si el texto nuevo cabe")
	}
	if f.Tamano <= 0 {
		t.Errorf("tamaño = %v", f.Tamano)
	}
}

// TestAnalizarAvisaDeQueNoHayReflujo es el aviso que evita la decepción: un PDF
// no guarda párrafos, así que el texto nuevo no empuja al de al lado.
func TestAnalizarAvisaDeQueNoHayReflujo(t *testing.T) {
	a, err := docDePrueba(t, "algo").AnalizarEdicion(0)
	if err != nil {
		t.Fatalf("AnalizarEdicion: %v", err)
	}
	if !contieneAviso(a.Avisos, "avisoSinReflujo") {
		t.Errorf("faltó el aviso de que no hay reflujo: %v", a.Avisos)
	}
}

func contieneAviso(avisos []string, cual string) bool {
	for _, a := range avisos {
		if a == cual {
			return true
		}
	}
	return false
}

func TestEsSubconjunto(t *testing.T) {
	if !esSubconjunto("ABCDEF+Arial") {
		t.Error("«ABCDEF+Arial» es un subconjunto")
	}
	for _, n := range []string{"Arial", "abcdef+Arial", "ABCDE+Arial", "ABCDEF+", ""} {
		if esSubconjunto(n) {
			t.Errorf("%q NO es un subconjunto", n)
		}
	}
}

func TestEditarTextoSustituyeEnSuSitio(t *testing.T) {
	d := docDePrueba(t, "importe mil euros")
	salida, err := EditarTextoPDF(d, []CambioDeTexto{{Pagina: 0, ID: 0, Nuevo: "importe dos euros"}})
	if err != nil {
		t.Fatalf("EditarTextoPDF: %v", err)
	}
	// El flujo NO puede seguir conteniendo la cadena vieja. Comprobarlo solo con
	// el texto extraído no bastaba: en notación postfija un operando huérfano se
	// descarta solo, así que dejarlo dentro no cambiaba lo que se lee — pero sí
	// deja el dato original en el fichero, que es lo que nadie espera al editar.
	if bytes.Contains(salida, []byte("importe mil euros")) {
		t.Error("el texto viejo sigue escrito en el flujo de contenido")
	}

	d2, err := LeerPDF(salida)
	if err != nil {
		t.Fatalf("el PDF editado no abre: %v", err)
	}
	texto, err := d2.TextoPlano(0)
	if err != nil {
		t.Fatalf("TextoPlano: %v", err)
	}
	if !strings.Contains(texto, "importe dos euros") {
		t.Errorf("el texto nuevo no está: %q", texto)
	}
	if strings.Contains(texto, "mil euros") {
		t.Errorf("el texto viejo sigue ahí: %q", texto)
	}
}

// TestEditarNoTocaLoQueNoSePidio es EL check de esta ola. Se edita el primer
// fragmento de una página con dos y el segundo tiene que quedar intacto, letra
// por letra y en la misma posición.
func TestEditarNoTocaLoQueNoSePidio(t *testing.T) {
	w := pdfsimple.Nuevo()
	w.Texto("linea uno", "F1", 12)
	w.Texto("linea dos intacta", "F1", 12)
	d, err := LeerPDF(w.Bytes())
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	antes, err := d.TextoDePagina(0)
	if err != nil {
		t.Fatalf("TextoDePagina: %v", err)
	}
	if len(antes) < 2 {
		t.Fatalf("el documento de prueba tiene %d fragmentos y hacen falta 2", len(antes))
	}

	salida, err := EditarTextoPDF(d, []CambioDeTexto{{Pagina: 0, ID: 0, Nuevo: "linea UNO"}})
	if err != nil {
		t.Fatalf("EditarTextoPDF: %v", err)
	}
	d2, err := LeerPDF(salida)
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	despues, err := d2.TextoDePagina(0)
	if err != nil {
		t.Fatalf("TextoDePagina: %v", err)
	}
	if len(despues) != len(antes) {
		t.Fatalf("cambió el número de fragmentos: %d → %d", len(antes), len(despues))
	}
	if despues[1].Texto != antes[1].Texto {
		t.Errorf("el segundo fragmento cambió: %q → %q", antes[1].Texto, despues[1].Texto)
	}
	if despues[1].X != antes[1].X || despues[1].Y != antes[1].Y {
		t.Errorf("el segundo fragmento se movió: (%v,%v) → (%v,%v)",
			antes[1].X, antes[1].Y, despues[1].X, despues[1].Y)
	}
}

func TestEditarConTextoVacioBorra(t *testing.T) {
	d := docDePrueba(t, "esto sobra")
	salida, err := EditarTextoPDF(d, []CambioDeTexto{{Pagina: 0, ID: 0, Nuevo: ""}})
	if err != nil {
		t.Fatalf("EditarTextoPDF: %v", err)
	}
	d2, err := LeerPDF(salida)
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	texto, _ := d2.TextoPlano(0)
	if strings.Contains(texto, "esto sobra") {
		t.Errorf("no se borró: %q", texto)
	}
}

func TestEditarRechazaLoQueNoExiste(t *testing.T) {
	d := docDePrueba(t, "x")
	if _, err := EditarTextoPDF(d, nil); err == nil {
		t.Error("sin cambios no hay nada que hacer")
	}
	if _, err := EditarTextoPDF(d, []CambioDeTexto{{Pagina: 9, ID: 0, Nuevo: "y"}}); err == nil {
		t.Error("una página que no existe debe fallar")
	}
	if _, err := EditarTextoPDF(d, []CambioDeTexto{{Pagina: 0, ID: 99, Nuevo: "y"}}); err == nil {
		t.Error("un fragmento que no existe debe fallar en vez de no hacer nada en silencio")
	}
}

// Una fuente compuesta sin /ToUnicode no dice qué letras usa: el análisis lo
// marca y reescribir da un error que lo explica (antes: «la fuente no tiene la
// letra «​»», con el carácter invisible). Borrar sí se puede.
func TestEditarLetrasDesconocidas(t *testing.T) {
	fuente := []string{
		`<< /Type /Font /Subtype /Type0 /BaseFont /P /Encoding /Identity-H /DescendantFonts [6 0 R] >>`,
		`<< /Type /Font /Subtype /CIDFontType2 /BaseFont /P /CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >> /DW 500 >>`,
	}
	d, err := LeerPDF(pdfConContenido(t, `BT /F1 12 Tf 10 50 Td <00110012> Tj ET`, helvetica, fuente...))
	if err != nil {
		t.Fatal(err)
	}
	an, err := d.AnalizarEdicion(0)
	if err != nil || len(an.Fragmentos) != 1 || !an.Fragmentos[0].LetrasDesconocidas {
		t.Fatalf("análisis = %+v, %v; quería un fragmento con letras desconocidas", an.Fragmentos, err)
	}
	if _, err := EditarTextoPDF(d, []CambioDeTexto{{Pagina: 0, ID: an.Fragmentos[0].ID, Nuevo: "hola"}}); !errors.Is(err, ErrLetrasDesconocidas) {
		t.Errorf("reescribir: err = %v, quería ErrLetrasDesconocidas", err)
	}
	if _, err := EditarTextoPDF(d, []CambioDeTexto{{Pagina: 0, ID: an.Fragmentos[0].ID, Nuevo: ""}}); err != nil {
		t.Errorf("borrar tiene que seguir funcionando: %v", err)
	}
}

// --- Imágenes ----------------------------------------------------------------

func TestSustituirImagen(t *testing.T) {
	origen, err := ImagenesAPDF([]ImagenAPagina{{Datos: pngDePrueba(t, 40, 40, false), Nombre: "a.png"}})
	if err != nil {
		t.Fatalf("ImagenesAPDF: %v", err)
	}
	d, err := LeerPDF(origen)
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	a, err := d.AnalizarEdicion(0)
	if err != nil {
		t.Fatalf("AnalizarEdicion: %v", err)
	}
	if len(a.Imagenes) != 1 {
		t.Fatalf("imágenes detectadas = %d, quería 1", len(a.Imagenes))
	}
	if a.Imagenes[0].Ancho != 40 {
		t.Errorf("ancho = %d, quería 40", a.Imagenes[0].Ancho)
	}

	salida, err := SustituirImagenPDF(d, 0, a.Imagenes[0].ID, jpegDePrueba(t, 80, 80))
	if err != nil {
		t.Fatalf("SustituirImagenPDF: %v", err)
	}
	d2, err := LeerPDF(salida)
	if err != nil {
		t.Fatalf("el PDF con la imagen cambiada no abre: %v", err)
	}
	a2, err := d2.AnalizarEdicion(0)
	if err != nil {
		t.Fatalf("AnalizarEdicion: %v", err)
	}
	if len(a2.Imagenes) != 1 || a2.Imagenes[0].Ancho != 80 {
		t.Errorf("la imagen no se sustituyó: %+v", a2.Imagenes)
	}
	// El hueco se conserva: la página sigue midiendo lo que medía.
	p, _ := d2.Pagina(0)
	if p.Ancho != 40 || p.Alto != 40 {
		t.Errorf("la página cambió de tamaño: %.0f×%.0f", p.Ancho, p.Alto)
	}
	svg, err := d2.RenderizarPaginaSVG(0, OpcionesRender{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(svg, "<image ") {
		t.Errorf("la imagen nueva no se ve: %s", primerasLineas(svg))
	}
}

func TestQuitarImagenNoRompeElFlujo(t *testing.T) {
	origen, err := ImagenesAPDF([]ImagenAPagina{{Datos: pngDePrueba(t, 20, 20, false), Nombre: "a.png"}})
	if err != nil {
		t.Fatalf("ImagenesAPDF: %v", err)
	}
	d, _ := LeerPDF(origen)
	salida, err := SustituirImagenPDF(d, 0, "Im0", nil)
	if err != nil {
		t.Fatalf("SustituirImagenPDF: %v", err)
	}
	d2, err := LeerPDF(salida)
	if err != nil {
		t.Fatalf("quitar la imagen dejó un PDF ilegible: %v", err)
	}
	// El `Do` sigue apuntando a un objeto válido: borrarlo del todo dejaría una
	// referencia rota que unos lectores ignoran y otros pintan en negro.
	if _, err := d2.RenderizarPaginaSVG(0, OpcionesRender{}); err != nil {
		t.Errorf("la página no se puede dibujar: %v", err)
	}
}

func TestSustituirImagenQueNoExiste(t *testing.T) {
	if _, err := SustituirImagenPDF(docDePrueba(t, "sin imagenes"), 0, "Im0", nil); err == nil {
		t.Error("sustituir una imagen que no existe debe fallar con un mensaje")
	}
}

// --- Capas -------------------------------------------------------------------

// pdfConCapas arma un documento con dos grupos de contenido opcional.
func pdfConCapas(t *testing.T) []byte {
	t.Helper()
	objs := []string{
		`<< /Type /Catalog /Pages 2 0 R /OCProperties << /OCGs [5 0 R 6 0 R] ` +
			`/D << /Order [5 0 R 6 0 R] /OFF [6 0 R] >> >> >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 >>`,
		`<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Contents 4 0 R >>`,
		"<< /Length 0 >>\nstream\n\nendstream",
		`<< /Type /OCG /Name (Cotas) >>`,
		`<< /Type /OCG /Name (Instalacion electrica) >>`,
	}
	return pdfMinimo(t, `<< /Size 7 /Root 1 0 R >>`, objs)
}

func TestLeerCapas(t *testing.T) {
	d, err := LeerPDF(pdfConCapas(t))
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	capas := d.CapasDelDocumento()
	if len(capas) != 2 {
		t.Fatalf("capas = %d, quería 2", len(capas))
	}
	if capas[0].Nombre != "Cotas" || !capas[0].Visible {
		t.Errorf("primera capa = %+v", capas[0])
	}
	// La segunda está en /OFF: apagada.
	if capas[1].Visible {
		t.Errorf("la segunda capa debería estar apagada: %+v", capas[1])
	}
}

func TestCambiarCapasEnciendeYApaga(t *testing.T) {
	d, err := LeerPDF(pdfConCapas(t))
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	capas := d.CapasDelDocumento()
	// Se enciende la segunda y se apaga la primera: justo al revés que antes.
	salida, err := CambiarCapasPDF(d, []int{capas[1].ID})
	if err != nil {
		t.Fatalf("CambiarCapasPDF: %v", err)
	}
	d2, err := LeerPDF(salida)
	if err != nil {
		t.Fatalf("el PDF con capas cambiadas no abre: %v", err)
	}
	nuevas := d2.CapasDelDocumento()
	if len(nuevas) != 2 {
		t.Fatalf("se perdieron capas: %d", len(nuevas))
	}
	if nuevas[0].Visible {
		t.Errorf("«%s» tenía que quedar apagada", nuevas[0].Nombre)
	}
	if !nuevas[1].Visible {
		t.Errorf("«%s» tenía que quedar encendida", nuevas[1].Nombre)
	}
	// Y los nombres sobreviven: sin ellos, la lista de capas es inservible.
	if nuevas[0].Nombre != "Cotas" {
		t.Errorf("nombre = %q", nuevas[0].Nombre)
	}
}

func TestCambiarCapasSinCapasFalla(t *testing.T) {
	if _, err := CambiarCapasPDF(docDePrueba(t, "x"), nil); err == nil {
		t.Error("un documento sin capas no puede cambiar capas")
	}
}

// Un flujo compacto pega la cadena al operador anterior («Td(hola)Tj»). La
// sustitución se escribía detrás sin separador y el Td se fundía con el número
// siguiente («Td0.5»): un operador desconocido, el texto a (0,0) y Poppler
// protestando por cada página editada.
func TestEditarEnUnFlujoCompacto(t *testing.T) {
	d, err := LeerPDF(pdfConContenido(t, `BT /F1 12 Tf 10 50 Td(hola mundo)Tj ET`, helvetica, objHelvetica))
	if err != nil {
		t.Fatal(err)
	}
	salida, err := EditarTextoPDF(d, []CambioDeTexto{{Pagina: 0, ID: 0, Nuevo: "hola todos"}})
	if err != nil {
		t.Fatalf("EditarTextoPDF: %v", err)
	}
	frags := fragmentosDeSalida(t, salida)
	if len(frags) != 1 || frags[0].Texto != "hola todos" || frags[0].X != 10 || frags[0].Y != 50 {
		t.Fatalf("fragmentos = %+v, quería «hola todos» en (10, 50)", frags)
	}
}

// En una fuente compuesta cada letra son dos bytes de SU codificación. Se
// escribía WinAnsi, que en el BOE pinta otros glifos o ninguno.
func TestEditarEnFuenteCID(t *testing.T) {
	d, err := LeerPDF(pdfConContenido(t, `BT /F1 12 Tf 10 50 Td <0011001200130014> Tj ET`, helvetica, objsFuenteCID()...))
	if err != nil {
		t.Fatal(err)
	}
	salida, err := EditarTextoPDF(d, []CambioDeTexto{{Pagina: 0, ID: 0, Nuevo: "mloH"}})
	if err != nil {
		t.Fatalf("EditarTextoPDF: %v", err)
	}
	if frags := fragmentosDeSalida(t, salida); len(frags) != 1 || frags[0].Texto != "mloH" {
		t.Fatalf("fragmentos = %+v, quería «mloH»", frags)
	}
	// Una letra que la fuente no trae no se puede escribir: se dice cuál.
	if _, err := EditarTextoPDF(d, []CambioDeTexto{{Pagina: 0, ID: 0, Nuevo: "Hola"}}); err == nil ||
		!strings.Contains(err.Error(), "«a»") {
		t.Errorf("err = %v, quería que dijera que falta la «a»", err)
	}
}

func fragmentosDeSalida(t *testing.T, salida []byte) []PDFFragmento {
	t.Helper()
	d, err := LeerPDF(salida)
	if err != nil {
		t.Fatalf("releer: %v", err)
	}
	frags, err := d.TextoDePagina(0)
	if err != nil {
		t.Fatalf("TextoDePagina: %v", err)
	}
	return frags
}

// Los números de fragmento cuentan cada operador de texto, también los que no
// muestran nada; al reescribir solo se contaban los que sí, así que un «() Tj»
// vacío delante desplazaba la cuenta: se cambiaba otro texto o ninguno («no se
// encontró el texto a cambiar», rotation.pdf y issue925 de pdf.js).
func TestEditarConUnTjVacioDelante(t *testing.T) {
	d, err := LeerPDF(pdfConContenido(t, `BT /F1 12 Tf 10 50 Td () Tj (hola mundo) Tj ET`, helvetica, objHelvetica))
	if err != nil {
		t.Fatal(err)
	}
	an, err := d.AnalizarEdicion(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(an.Fragmentos) != 1 {
		t.Fatalf("fragmentos = %+v, quería uno", an.Fragmentos)
	}
	salida, err := EditarTextoPDF(d, []CambioDeTexto{{Pagina: 0, ID: an.Fragmentos[0].ID, Nuevo: "hola todos"}})
	if err != nil {
		t.Fatalf("EditarTextoPDF: %v", err)
	}
	if frags := fragmentosDeSalida(t, salida); len(frags) != 1 || frags[0].Texto != "hola todos" {
		t.Fatalf("fragmentos = %+v, quería «hola todos»", frags)
	}
}
