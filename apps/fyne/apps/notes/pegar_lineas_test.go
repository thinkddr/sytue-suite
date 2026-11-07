// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// Pegar varias líneas en la captura: los casos de NotebookDashboard.test.tsx de la web.

// capturaContra monta un cuaderno con la captura contra un servicio que, a la n-ésima
// nota (desde 1), la crea si `responder` dice que sí y falla si no. Devuelve lo que ha
// llegado al servicio, en orden.
func capturaContra(t *testing.T, vista string, responder func(n int) bool) (*Pantalla, func() []map[string]any) {
	t.Helper()
	var mu sync.Mutex
	var llegadas []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/notebooks/c1/notes" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		crudo, _ := io.ReadAll(r.Body)
		var c map[string]any
		_ = json.Unmarshal(crudo, &c)
		mu.Lock()
		llegadas = append(llegadas, c)
		n := len(llegadas)
		mu.Unlock()
		if !responder(n) {
			http.Error(w, `{"error":"no"}`, http.StatusInternalServerError)
			return
		}
		_, _ = fmt.Fprintf(w, `{"id":"n%d","notebook_id":"c1","kind":%q}`, n, c["kind"])
	}))
	t.Cleanup(srv.Close)
	p := NuevaPantalla(trInterno(t), "es", api.NewNotes(srv.URL, nil, nil), nil, nil)
	c1 := DeCuaderno(api.Notebook{ID: "c1", Title: "Ideas", Layout: vista})
	p.cuadernos, p.activo = []Cuaderno{c1}, c1
	p.lienzo.Fase = Exito
	w := test.NewWindow(p.Objeto())
	t.Cleanup(w.Close)
	p.win = w
	w.Resize(fyne.NewSize(1280, 800))
	p.Repintar()
	if p.campoCaptura == nil {
		t.Fatal("el cuaderno no tiene captura")
	}
	return p, func() []map[string]any {
		mu.Lock()
		defer mu.Unlock()
		return append([]map[string]any(nil), llegadas...)
	}
}

// pegarEn pega `texto` en el campo con el ⌘V del teclado, que pasa por `AlPegar`.
func pegarEn(campo fyne.Shortcutable, texto string) {
	pp := test.NewClipboard()
	pp.SetContent(texto)
	campo.TypedShortcut(&fyne.ShortcutPaste{Clipboard: pp})
}

// resumenes son las notas que han llegado como «tipo:título-o-contenido».
func resumenes(ls []map[string]any) []string {
	out := make([]string, 0, len(ls))
	for _, c := range ls {
		que := c["content"]
		if c["kind"] == "todo" {
			que = fmt.Sprintf("%v:%v", c["title"], c["checked"])
		}
		out = append(out, fmt.Sprintf("%v:%v", c["kind"], que))
	}
	return out
}

// TestPegarVariasLineasCreaUnaNotaPorLinea: cada línea, con la sintaxis de la captura y
// en orden; las líneas en blanco y el `\r` no son nada, y el campo acaba vacío. Una
// línea sola se pega en el campo, como siempre.
//
// VISTO EN ROJO (2026-11-05, en el frontal): sin `campo.AlPegar` en nuevoCampoCaptura,
// el campo de una línea se quedaba con todo junto («- Comprar pan  [x] Llamar…») y no
// salía ninguna nota.
func TestPegarVariasLineasCreaUnaNotaPorLinea(t *testing.T) {
	p, llegadas := capturaContra(t, "list", func(int) bool { return true })
	pegarEn(p.campoCaptura, "- Comprar pan\n\n[x] Llamar a Ana\r\n---\nLa reunión es el jueves\n")
	esperar(t, func() bool { return len(llegadas()) == 4 && !p.guardandoCaptura })
	quiero := []string{"todo:Comprar pan:false", "todo:Llamar a Ana:true", "text:<hr>", "text:<p>La reunión es el jueves</p>"}
	if got := resumenes(llegadas()); !reflect.DeepEqual(got, quiero) {
		t.Errorf("llegan %v, quería %v", got, quiero)
	}
	if p.campoCaptura.Texto() != "" || p.borradorGuardado("c1") != "" {
		t.Errorf("la captura se queda con %q (borrador %q)", p.campoCaptura.Texto(), p.borradorGuardado("c1"))
	}

	pegarEn(p.campoCaptura, "Una sola idea")
	if p.campoCaptura.Texto() != "Una sola idea" || len(llegadas()) != 4 {
		t.Errorf("una línea sola no se queda en el campo: %q, %d notas", p.campoCaptura.Texto(), len(llegadas()))
	}
}

// TestPegarParaEnElFalloYDejaElResto: lo que no ha salido se queda en la captura. El
// campo es de una línea y enseña espacios, pero el borrador guarda los saltos, e Intro
// sigue sacando una nota por línea. Lo pegado va donde está el cursor.
//
// VISTO EN ROJO (2026-11-05, en el frontal): con `resto := strings.Join(…, " ")` en
// quedanEnCaptura —guardando el resto como lo enseña el campo— el borrador perdía los
// saltos e Intro mandaba «dos tres» como una nota.
func TestPegarParaEnElFalloYDejaElResto(t *testing.T) {
	p, llegadas := capturaContra(t, "list", func(n int) bool { return n != 2 })
	pegarEn(p.campoCaptura, "uno\ndos\ntres")
	esperar(t, func() bool { return len(llegadas()) == 2 && !p.guardandoCaptura })
	if got, borrador := p.campoCaptura.Texto(), p.borradorGuardado("c1"); got != "dos tres" || borrador != "dos\ntres" {
		t.Fatalf("tras el fallo, el campo dice %q y el borrador %q", got, borrador)
	}
	p.campoCaptura.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	esperar(t, func() bool { return len(llegadas()) == 4 && !p.guardandoCaptura })
	if got := resumenes(llegadas()[2:]); !reflect.DeepEqual(got, []string{"text:<p>dos</p>", "text:<p>tres</p>"}) {
		t.Errorf("Intro manda %v", got)
	}

	// En mitad de lo escrito: lo de antes y lo de después del cursor son parte de la
	// primera y la última línea.
	q, llegadas2 := capturaContra(t, "list", func(int) bool { return true })
	q.campoCaptura.PonerTexto("Ver  hoy")
	pegarEnCursor(q, 4, "el plan\n[ ] llamar")
	esperar(t, func() bool { return len(llegadas2()) == 2 && !q.guardandoCaptura })
	if got := resumenes(llegadas2()); !reflect.DeepEqual(got, []string{"text:<p>Ver el plan</p>", "todo:llamar hoy:false"}) {
		t.Errorf("pegar en mitad manda %v", got)
	}
}

// pegarEnCursor pega con el cursor en la runa `col`.
func pegarEnCursor(p *Pantalla, col int, texto string) {
	p.campoCaptura.TypedKey(&fyne.KeyEvent{Name: fyne.KeyHome})
	for range col {
		p.campoCaptura.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	}
	pegarEn(p.campoCaptura, texto)
}

// TestComoMuchoCincuentaDeUnaVez: el resto se queda en la captura, se pegue en ella o
// sobre la lista.
//
// VISTO EN ROJO (2026-11-05, en el frontal): sin `MaxLineasPegadas` en el `min` de
// `enviar`, lo pegado sobre la lista —que pide tantas como líneas trae— salían las
// cincuenta y tres.
func TestComoMuchoCincuentaDeUnaVez(t *testing.T) {
	lineas := make([]string, 53)
	for i := range lineas {
		lineas[i] = fmt.Sprintf("l%d", i+1)
	}
	for _, sobreLaLista := range []bool{false, true} {
		p, llegadas := capturaContra(t, "list", func(int) bool { return true })
		if sobreLaLista {
			p.win.Canvas().Unfocus()
			pp := test.NewClipboard()
			pp.SetContent(strings.Join(lineas, "\n"))
			p.pegar(&fyne.ShortcutPaste{Clipboard: pp})
		} else {
			pegarEn(p.campoCaptura, strings.Join(lineas, "\n"))
		}
		// Cincuenta altas en serie, cada una con su repintado: con `-race` en el runner
		// ARM pasaban de los 10 s de `esperar` (CI de escritorio, 2026-11-05), y en el
		// frontal tardan unos 32 s por caso. El tope deja margen; no es lo que mide.
		esperarDurante(t, 3*time.Minute, func() bool { return len(llegadas()) >= 50 && !p.guardandoCaptura })
		if n := len(llegadas()); n != 50 || p.campoCaptura.Texto() != "l51 l52 l53" {
			t.Errorf("sobre la lista %v: salen %d y se queda %q", sobreLaLista, n, p.campoCaptura.Texto())
		}
	}
}

// TestSiSeEscribeOtraCosaSePara: lo que se escribe en la captura mientras salen las
// líneas no se toca, y lo pegado que quedaba no sale.
//
// VISTO EN ROJO (2026-11-05, en el frontal): sin comparar el borrador con `enCampo` en
// quedanEnCaptura, lo escrito se perdía y salían las tres.
func TestSiSeEscribeOtraCosaSePara(t *testing.T) {
	var p *Pantalla
	p, llegadas := capturaContra(t, "list", func(n int) bool {
		if n == 1 {
			// La persona escribe en la captura mientras la primera está en el aire.
			fyne.DoAndWait(func() { p.campoCaptura.PonerTexto("otra cosa") })
		}
		return true
	})
	pegarEn(p.campoCaptura, "uno\ndos\ntres")
	esperar(t, func() bool { return len(llegadas()) >= 1 && !p.guardandoCaptura })
	if n, got := len(llegadas()), p.campoCaptura.Texto(); n != 1 || got != "otra cosa" || p.borradorGuardado("c1") != "otra cosa" {
		t.Errorf("salen %d y la captura dice %q", n, got)
	}
}

// TestPegarSobreLaListaSaleSinLoQueEstabaAMedias: pegado con nada enfocado sobre una
// lista, cada línea es una nota y lo que había en la captura se queda; en columnas, un
// texto pegado es UNA tarjeta.
//
// VISTO EN ROJO (2026-11-05, en el frontal): con `enviar(nuevo, MaxLineasPegadas)` en
// pegarLineas salía también «a medias».
func TestPegarSobreLaListaSaleSinLoQueEstabaAMedias(t *testing.T) {
	p, llegadas := capturaContra(t, "list", func(int) bool { return true })
	p.campoCaptura.PonerTexto("a medias")
	p.win.Canvas().Unfocus()
	if p.pegarLineas("una sola") {
		t.Fatal("una línea sola no va por la captura")
	}
	pp := test.NewClipboard()
	pp.SetContent("uno\ndos")
	p.pegar(&fyne.ShortcutPaste{Clipboard: pp})
	esperar(t, func() bool { return len(llegadas()) == 2 && !p.guardandoCaptura })
	if got := resumenes(llegadas()); !reflect.DeepEqual(got, []string{"text:<p>uno</p>", "text:<p>dos</p>"}) {
		t.Errorf("llegan %v", got)
	}
	if p.campoCaptura.Texto() != "a medias" {
		t.Errorf("la captura se queda con %q", p.campoCaptura.Texto())
	}

	q, llegadas2 := capturaContra(t, "columns", func(int) bool { return true })
	q.win.Canvas().Unfocus()
	q.pegar(&fyne.ShortcutPaste{Clipboard: pp})
	esperar(t, func() bool { return len(llegadas2()) == 1 && !q.creando })
	if got := resumenes(llegadas2()); !reflect.DeepEqual(got, []string{"text:<p>uno<br>dos</p>"}) {
		t.Errorf("en columnas llega %v", got)
	}
}

// TestLosTopesSonLosDeLaWeb: las cincuenta líneas de una vez, las 400 runas del título de
// una tarea y los sesenta caracteres tras «[[» se leen de client/diario.ts y
// client/comandos.ts: si la web cambia uno, esto se pone en rojo.
//
// VISTO EN ROJO (2026-11-05, en el frontal): con `MaxLineasPegadas = 40`.
func TestLosTopesSonLosDeLaWeb(t *testing.T) {
	diario := webref.Read(t, "apps/frontend/apps/notes/src/client/diario.ts")
	comandos := webref.Read(t, "apps/frontend/apps/notes/src/client/comandos.ts")
	tope := func(fuente, nombre string) int {
		m := regexp.MustCompile(`const ` + nombre + ` = (\d+);`).FindStringSubmatch(fuente)
		if m == nil {
			t.Fatalf("la web ya no define %s", nombre)
		}
		n, _ := strconv.Atoi(m[1])
		return n
	}
	for _, c := range []struct {
		web, fyne int
		que       string
	}{
		{tope(diario, "MAX_LINEAS_PEGADAS"), MaxLineasPegadas, "líneas de una vez"},
		{tope(diario, "MAX_TITULO_TAREA"), MaxTituloTarea, "runas del título de una tarea"},
		{tope(comandos, "MAX_CONSULTA_ENLACE"), maxConsultaEnlace, "letras tras «[[»"},
	} {
		if c.web != c.fyne {
			t.Errorf("%s: la web dice %d y Fyne %d", c.que, c.web, c.fyne)
		}
	}
}
