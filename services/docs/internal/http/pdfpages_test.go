// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// pet construye una petición con los campos de formulario dados, que es lo que
// `aplicarOperacionDePagina` lee.
func pet(campos map[string]string) *http.Request {
	form := url.Values{}
	for k, v := range campos {
		form.Set(k, v)
	}
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
	r.Header.Set("content-type", "application/x-www-form-urlencoded")
	return r
}

// Lo propio de este endpoint es la VALIDACIÓN de la petición; que las páginas se
// combinen y se corten bien ya lo cubren los tests del motor
// (`domain/pdfwrite_test.go`) y no se duplica aquí.
//
// El parseo de la lista de páginas es donde un despiste borra la equivocada: la
// API cuenta DESDE 1 porque es lo que el usuario ve en su visor y el motor
// cuenta desde 0, así que la conversión vive en un solo sitio y se valida el
// rango antes de tocar nada.

func TestParsearPaginasSueltasYRangos(t *testing.T) {
	casos := []struct {
		spec   string
		total  int
		quiero []int // ya en 0-based
	}{
		{"1", 3, []int{0}},
		{"1,3", 3, []int{0, 2}},
		{"1-3", 3, []int{0, 1, 2}},
		{"2-3,1", 3, []int{1, 2, 0}}, // se conserva el orden pedido
		{" 1 , 2 ", 3, []int{0, 1}},  // espacios alrededor
		{"1,1,1", 3, []int{0, 0, 0}}, // duplicar una página es legítimo
		{"3-1", 3, []int{2, 1, 0}},   // rango al revés: esas páginas invertidas
	}
	for _, c := range casos {
		got, e := parsearPaginas(c.spec, c.total)
		if e != nil {
			t.Errorf("%q: error inesperado: %v", c.spec, e)
			continue
		}
		if len(got) != len(c.quiero) {
			t.Errorf("%q → %v, quería %v", c.spec, got, c.quiero)
			continue
		}
		for i := range got {
			if got[i] != c.quiero[i] {
				t.Errorf("%q → %v, quería %v", c.spec, got, c.quiero)
				break
			}
		}
	}
}

func TestParsearPaginasFueraDeRango(t *testing.T) {
	// Pedir la página 4 de un documento de 3 tiene que fallar ANTES de tocar
	// nada, no producir un PDF a medias.
	for _, spec := range []string{"4", "0", "1-9", "-1", "2,7"} {
		if _, e := parsearPaginas(spec, 3); e == nil {
			t.Errorf("%q sobre 3 páginas debería fallar", spec)
		}
	}
}

func TestParsearPaginasEntradaSinSentido(t *testing.T) {
	// «,» solo, sin ningún número, sí falla: no hay nada que extraer.
	for _, spec := range []string{"", "   ", "abc", "1-", "1-a", ",", ",,"} {
		if _, e := parsearPaginas(spec, 5); e == nil {
			t.Errorf("%q debería fallar", spec)
		}
	}
}

func TestParsearPaginasToleraComasSobrantes(t *testing.T) {
	// «1,,,» es un dedo torpe, no una petición inválida: quien lo escribe quiere
	// la página 1. Se tolera igual que los espacios, mientras quede algún número.
	got, e := parsearPaginas("1,,,2", 5)
	if e != nil {
		t.Fatalf("error inesperado: %v", e)
	}
	if len(got) != 2 || got[0] != 0 || got[1] != 1 {
		t.Errorf("«1,,,2» → %v, quería [0 1]", got)
	}
}

func TestParsearPaginasTopeDeSeguridad(t *testing.T) {
	// Un rango enorme no debe hacer trabajar al servidor sin límite aunque el
	// documento sea grande de verdad.
	_, e := parsearPaginas("1-20000", 30000)
	if e == nil {
		t.Fatal("un rango por encima del tope debería fallar")
	}
	if !strings.Contains(e.Error(), "Too many") {
		t.Errorf("el mensaje debería explicar que son demasiadas páginas: %q", e.Error())
	}
}

func TestParsearGrados(t *testing.T) {
	ok := map[string]int{"90": 90, "180": 180, "270": 270, "-90": 270, "360": 0, "450": 90}
	for entrada, quiero := range ok {
		got, e := parsearGrados(entrada)
		if e != nil {
			t.Errorf("%q: error inesperado: %v", entrada, e)
			continue
		}
		if got != quiero {
			t.Errorf("%q → %d, quería %d", entrada, got, quiero)
		}
	}
	// PDF solo admite múltiplos de 90 en /Rotate: aceptar 45 escribiría un
	// fichero que los visores ignoran en silencio.
	for _, malo := range []string{"", "45", "1", "abc", "9.5"} {
		if _, e := parsearGrados(malo); e == nil {
			t.Errorf("%q debería fallar", malo)
		}
	}
}

func TestAplicarOperacionValidaLaOperacion(t *testing.T) {
	if _, e := aplicarOperacionDePagina("", nil, pet(map[string]string{"paginas": "1"})); e == nil {
		t.Error("sin operación debería fallar")
	}
	if _, e := aplicarOperacionDePagina("destruir", nil, pet(map[string]string{"paginas": "1"})); e == nil {
		t.Error("una operación desconocida debería fallar")
	}
}

func TestAplicarOperacionCuentaFicheros(t *testing.T) {
	// Combinar necesita dos o más; y aplicar «borrar la página 2» a dos ficheros
	// distintos a la vez no significa nada, así que también se rechaza.
	if _, e := aplicarOperacionDePagina("combinar", []*domain.PDFDoc{nil}, pet(nil)); e == nil {
		t.Error("combinar con un solo fichero debería fallar")
	}
	if _, e := aplicarOperacionDePagina("borrar", []*domain.PDFDoc{nil, nil}, pet(map[string]string{"paginas": "1"})); e == nil {
		t.Error("borrar sobre dos ficheros debería fallar")
	}
}

func TestTraducirErrorPDFDaMensajesAccionables(t *testing.T) {
	// Un PDF con contraseña no es «error del servidor»: el usuario puede
	// resolverlo, y el mensaje tiene que decírselo y nombrar el fichero, porque
	// puede haber enviado varios de golpe.
	//
	// Desde que el motor sabe descifrar, los dos casos se separan: «hace falta
	// una contraseña» —que la interfaz puede PEDIR, y por eso lleva código
	// propio— y «esta protección no la sabemos abrir», que no tiene arreglo por
	// parte del usuario. Confundirlos hacía que se pidiera una contraseña que no
	// iba a servir de nada.
	e := traducirErrorPDF(domain.ErrPDFClaveIncorrecta, "nomina.pdf")
	if !strings.Contains(e.Error(), "password") || !strings.Contains(e.Error(), "nomina.pdf") {
		t.Errorf("mensaje de contraseña = %q", e.Error())
	}
	// El código va en el error para que el cliente lo distinga sin leer el texto,
	// que está traducido: es lo que le permite abrir el diálogo de contraseña en
	// vez de enseñar un mensaje y quedarse ahí.
	if !strings.Contains(e.Error(), "pdf_password_required") {
		t.Errorf("falta el código pdf_password_required en %q", e.Error())
	}
	if p := traducirErrorPDF(domain.ErrPDFCifrado, "raro.pdf").Error(); !strings.Contains(p, "raro.pdf") {
		t.Errorf("mensaje de protección no soportada = %q", p)
	}
	if !strings.Contains(traducirErrorPDF(domain.ErrPDFNoEsPDF, "foto.png").Error(), "is not a PDF") {
		t.Error("el mensaje de «no es PDF» no lo dice")
	}
	if !strings.Contains(traducirErrorPDF(errors.New("otra cosa"), "x.pdf").Error(), "x.pdf") {
		t.Error("el mensaje genérico debería nombrar el fichero")
	}

	// Y cada caso con SU código: la frase está en inglés y solo el código se traduce.
	// Con los cuatro como `bad_request`, «no es un PDF» y «está dañado» se veían igual.
	for err, quiero := range map[error]string{
		domain.ErrPDFNoEsPDF:     "pdf_not_pdf",
		domain.ErrPDFCifrado:     "pdf_encryption_unsupported",
		domain.ErrPDFNoSoportado: "pdf_unsupported",
		errors.New("otra cosa"):  "pdf_unreadable",
	} {
		if got := httpx.AsError(traducirErrorPDF(err, "x.pdf")).Code; got != quiero {
			t.Errorf("%v: código %q, quería %q", err, got, quiero)
		}
	}
}

func TestAplicarCapaExigeTextoSalvoAlNumerar(t *testing.T) {
	// La marca de agua y el encabezado necesitan qué escribir; numerar y Bates
	// tienen formato por defecto, así que el texto vacío es legítimo ahí.
	for _, op := range []string{"marca-agua", "encabezado", "pie"} {
		if _, e := aplicarOperacionDePagina(op, []*domain.PDFDoc{nil}, pet(nil)); e == nil {
			t.Errorf("%q sin texto debería fallar", op)
		}
	}
}

func TestAplicarCapaAcotaElTexto(t *testing.T) {
	// Una cadena enorme se dibujaría en CADA página; el fichero resultante sería
	// absurdo y el trabajo, proporcional a lo que mande el cliente.
	largo := strings.Repeat("x", 501)
	_, e := aplicarOperacionDePagina("marca-agua", []*domain.PDFDoc{nil}, pet(map[string]string{"texto": largo}))
	if e == nil {
		t.Fatal("un texto por encima del máximo debería fallar")
	}
	if !strings.Contains(e.Error(), "too long") {
		t.Errorf("mensaje = %q", e.Error())
	}
}

func TestAplicarCapaSobreVariosFicherosFalla(t *testing.T) {
	campos := pet(map[string]string{"texto": "BORRADOR"})
	if _, e := aplicarOperacionDePagina("marca-agua", []*domain.PDFDoc{nil, nil}, campos); e == nil {
		t.Error("una capa sobre dos ficheros a la vez debería fallar")
	}
}

func TestEnteroOpcional(t *testing.T) {
	if n, e := enteroOpcional("", 6, 12); e != nil || n != 6 {
		t.Errorf("vacío → %d, %v; quería el valor por defecto 6", n, e)
	}
	if n, e := enteroOpcional("3", 6, 12); e != nil || n != 3 {
		t.Errorf("«3» → %d, %v", n, e)
	}
	// Fuera de rango y no numérico fallan: un Bates de 900 dígitos o de «abc»
	// no es un descuido que convenga tragarse en silencio.
	for _, malo := range []string{"0", "13", "abc", "-1", "1e3"} {
		if _, e := enteroOpcional(malo, 6, 12); e == nil {
			t.Errorf("%q debería fallar", malo)
		}
	}
}

func TestPosicionDeCaeEnElPieAnteLoDesconocido(t *testing.T) {
	// Es una preferencia estética: fallar por ella tiraría el trabajo entero.
	if got := posicionDe("no-existe"); got != domain.CapaInferiorCen {
		t.Errorf("posición desconocida → %q, quería el pie centrado", got)
	}
	if got := posicionDe("superior-derecha"); got != domain.CapaSuperiorDer {
		t.Errorf("«superior-derecha» → %q", got)
	}
}
