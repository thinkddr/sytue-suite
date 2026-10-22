// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes_test

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/apps/notes"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// EL GUARDIÁN DE «LOS DOCUMENTOS NO SE CORROMPEN».
//
// La regla es que esta vista solo deja GUARDAR una nota cuyo HTML sepa volver a
// escribir entero. Lo que lo demuestra es este viaje de ida y vuelta sobre documentos
// REALES del repositorio: los casos de
// `services/notes/internal/domain/html_test.go`, que son la salida literal del saneador
// del servicio —lo que de verdad hay guardado en una nota— y no un ejemplo inventado
// aquí que dijera lo que me conviene.
//
// VISTO EN ROJO (2026-09-22, en el piloto). Tres inversiones a mano, deshechas después:
//
//  1. Quitando `target="_blank" rel="noopener noreferrer"` de `escribirTramo`: el caso
//     del enlace deja de ser fiel. Es el fallo que habría hecho de TODA nota con un
//     enlace una nota de solo lectura.
//  2. Devolviendo `"p"` siempre en vez de la etiqueta que traía: cae el caso de la
//     tarea, que es un `div`.
//  3. Emitiendo desde el árbol original en vez de desde los bloques: TODO sale verde,
//     incluido `TestLoQueNoSeEntiendeNoSePuedeGuardar`. Ese es el modo de fallo que
//     esta forma de escribirlo evita, y por eso el emisor sale del modelo.

// leerCasos saca de un fichero de test del servicio las cadenas que el saneador
// CONSERVA, que son las que una nota guardada puede contener.
//
// Se leen del fichero en vez de copiarlas: copiadas, el día que el servicio cambie una
// etiqueta este test seguiría en verde contra el HTML de ayer, que es exactamente la
// duplicación que describe `docs/CONVENTIONS.md` §13.2.
func leerCasos(t *testing.T) []string {
	t.Helper()
	fuente := webref.Read(t, "services/notes/internal/domain/html_test.go")
	i := strings.Index(fuente, "func TestSanitizeHTMLConservaLoPermitido")
	if i < 0 {
		t.Fatal("el test de «conserva lo permitido» ha cambiado de nombre: revisar este espejo")
	}
	fin := strings.Index(fuente[i:], "\n}\n")
	if fin < 0 {
		fin = len(fuente) - i
	}
	cuerpo := fuente[i : i+fin]
	// Cada caso es `{"nombre", entrada, quiero}`; lo que nos vale es el TERCER campo,
	// la salida del saneador. Las cadenas van con acento grave o con comillas.
	re := regexp.MustCompile("(?s)`([^`]*)`,\\s*\\n\\s*\\}|`([^`]*)`\\},")
	var casos []string
	for _, m := range re.FindAllStringSubmatch(cuerpo, -1) {
		s := m[1] + m[2]
		if strings.Contains(s, "<") {
			casos = append(casos, s)
		}
	}
	if len(casos) < 5 {
		t.Fatalf("solo se han leído %d documentos de html_test.go: el patrón ya no vale", len(casos))
	}
	return casos
}

// TestLaVueltaNoPierdeNadaDeUnDocumentoReal es el contrato entero sobre los documentos
// del servicio: se leen, se vuelven a escribir y tiene que salir lo mismo.
func TestLaVueltaNoPierdeNadaDeUnDocumentoReal(t *testing.T) {
	for _, crudo := range leerCasos(t) {
		t.Run(recortar(crudo), func(t *testing.T) {
			d := notes.Leer(crudo)
			if !d.Fiel() {
				t.Errorf("no es fiel, así que esta nota se abriría en SOLO LECTURA\n"+
					"  original: %s\n  vuelta:   %s", crudo, d.HTML())
			}
		})
	}
}

// TestLoQueNoSeEntiendeNoSePuedeGuardar es la otra mitad, y sin ella el test de arriba
// lo pasaría un lector que devolviera siempre el original.
//
// Cada uno de estos documentos trae algo que el saneador del servicio DEJA PASAR y este
// modelo no lleva: si alguno saliera fiel, se estaría ofreciendo guardar encima de algo
// que al escribirlo se perdería.
func TestLoQueNoSeEntiendeNoSePuedeGuardar(t *testing.T) {
	casos := map[string]string{
		"una clase que no es de tarea":   `<p class="destacado">a</p>`,
		"un atributo que no se modela":   `<p data-id="7">a</p>`,
		"una etiqueta que no se modela":  `<figure>foto</figure>`,
		"un comentario del editor":       `<p>a</p><!-- borrador -->`,
		"una casilla fuera de su fila":   `<p><input type="checkbox"/> suelta</p>`,
		"un estilo que no está modelado": `<p style="cursor:pointer">a</p>`,
	}
	for nombre, crudo := range casos {
		t.Run(nombre, func(t *testing.T) {
			if d := notes.Leer(crudo); d.Fiel() {
				t.Errorf("dice que es fiel y se perdería algo al guardar\n"+
					"  original: %s\n  vuelta:   %s", crudo, d.HTML())
			}
		})
	}
}

// TestFielYNoPintadoNoSonLaMismaPregunta es el motivo de que sean dos funciones y no
// una, que es el contrato que Diapos dejó escrito.
func TestFielYNoPintadoNoSonLaMismaPregunta(t *testing.T) {
	// Una tabla —sencilla o con una celda que ocupa dos— se conserva entera y se dibuja
	// como es (tabla.go reparte `colspan` y `rowspan`): ni solo lectura ni cartel.
	for _, crudo := range []string{
		`<table><tr><td colspan="2">Enero</td></tr><tr><td>1</td><td>120</td></tr></table>`,
		`<table><tr><td>Enero</td><td style="background-color:#fef08a">120</td></tr></table>`,
	} {
		if s := notes.Leer(crudo); !s.Fiel() || len(s.NoPintado()) != 0 {
			t.Errorf("la tabla se pinta y se conserva: fiel %v, no pintado %v\n  vuelta: %s", s.Fiel(), s.NoPintado(), s.HTML())
		}
	}

	// Y al revés: un párrafo con una clase se pinta perfectamente y NO se conserva.
	clase := notes.Leer(`<p class="destacado">a</p>`)
	if clase.Fiel() {
		t.Error("un párrafo con clase no se puede reescribir sin perder la clase")
	}
	if len(clase.NoPintado()) != 0 {
		t.Errorf("no hay nada que no se sepa dibujar, y dice %v", clase.NoPintado())
	}
}

// TestUnaNotaVaciaSePuedeEscribir: una nota recién creada no tiene nada que perder y
// tiene que nacer editable, o no se podría escribir la primera línea.
func TestUnaNotaVaciaSePuedeEscribir(t *testing.T) {
	for _, s := range []string{"", "   ", "\n"} {
		if d := notes.Leer(s); !d.Fiel() || !d.Vacio() {
			t.Errorf("Leer(%q): fiel=%v vacío=%v; quiero las dos", s, d.Fiel(), d.Vacio())
		}
	}
}

// TestLosBloquesSonLosDelDocumento comprueba que además de conservarse, se ENTIENDE:
// sin esto, un lector que guardara el original y no leyera nada pasaría todo lo demás.
func TestLosBloquesSonLosDelDocumento(t *testing.T) {
	d := notes.Leer(`<h2>Compra</h2><ul><li>Platos</li><li>Vasos</li></ul>` +
		`<p>Y <strong>pagar</strong></p>`)
	quiero := []struct {
		tipo  notes.TipoBloque
		texto string
	}{
		{notes.Titulo, "Compra"},
		{notes.Vineta, "Platos"},
		{notes.Vineta, "Vasos"},
		{notes.Parrafo, "Y pagar"},
	}
	if len(d.Bloques) != len(quiero) {
		t.Fatalf("%d bloques, quiero %d: %+v", len(d.Bloques), len(quiero), d.Bloques)
	}
	for i, q := range quiero {
		if b := d.Bloques[i]; b.Tipo != q.tipo || b.Texto() != q.texto {
			t.Errorf("bloque %d: %v %q; quiero %v %q", i, b.Tipo, b.Texto(), q.tipo, q.texto)
		}
	}
	if d.Bloques[0].Nivel != 2 {
		t.Errorf("el h2 dice nivel %d", d.Bloques[0].Nivel)
	}
	// La negrita tiene que llegar como MARCA del tramo, con su etiqueta literal, y no
	// como texto con etiquetas dentro.
	tramos := d.Bloques[3].Tramos
	if len(tramos) != 2 || len(tramos[0].Marcas) != 0 ||
		!slices.Equal(tramos[1].Marcas, []string{"strong"}) {
		t.Errorf("los tramos del párrafo no llevan la negrita donde toca: %+v", tramos)
	}
}

// TestEditarUnaTareaLaDevuelveIgualSalvoLaMarca: marcar una tarea es la única escritura
// que esta vista hace hoy sobre el cuerpo, y tiene que tocar SOLO eso.
func TestEditarUnaTareaLaDevuelveIgualSalvoLaMarca(t *testing.T) {
	const crudo = `<div class="task-row"><input type="checkbox"/> <span>Comprar pan</span></div>`
	d := notes.Leer(crudo)
	if !d.Fiel() {
		t.Fatalf("la tarea no vuelve entera\n  original: %s\n  vuelta:   %s", crudo, d.HTML())
	}
	if len(d.Bloques) != 1 || d.Bloques[0].Tipo != notes.Tarea || d.Bloques[0].Marcada {
		t.Fatalf("no se ha leído como una tarea sin marcar: %+v", d.Bloques)
	}
	d.Bloques[0].Marcada = true
	got := d.HTML()
	if !strings.Contains(got, "checked") {
		t.Errorf("marcarla no ha escrito el `checked`: %s", got)
	}
	if !strings.Contains(got, "Comprar pan") || !strings.Contains(got, `class="task-row"`) {
		t.Errorf("marcarla se ha llevado algo por delante: %s", got)
	}
}

func recortar(s string) string {
	if len(s) > 40 {
		return s[:40]
	}
	return s
}
