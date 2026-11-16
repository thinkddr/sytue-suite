// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// TestLaTablaDeAtajosEsLaDeLaWeb: los mismos grupos y las mismas filas, en el mismo
// orden, con las mismas teclas, descripciones y necesidad de red, que `GRUPOS` de
// `lib/atajos.ts`. Si una plataforma gana un atajo y la otra no, la hoja de una de las
// dos miente.
func TestLaTablaDeAtajosEsLaDeLaWeb(t *testing.T) {
	fuente, err := os.ReadFile("../../../frontend/apps/drive/src/lib/atajos.ts")
	if err != nil {
		t.Fatal(err)
	}
	bloque := regexp.MustCompile(`(?s)const GRUPOS: [^=]+= \[(.*?)\n\];`).FindSubmatch(fuente)
	if bloque == nil {
		t.Fatal("atajos.ts ya no declara GRUPOS: el espejo hay que rehacerlo")
	}
	// Las teclas con nombre de la web (`KeyboardEvent.key`) y las de Fyne no siempre se
	// escriben igual.
	deLaWeb := map[string]string{"Backspace": string(fyne.KeyBackspace), "Ctrl+a": teclaMarcarTodo}
	var web []string
	for _, g := range regexp.MustCompile(`(?s)titulo: '([^']+)',\s*atajos: \[(.*?)\n {4}\]`).FindAllSubmatch(bloque[1], -1) {
		web = append(web, "# "+string(g[1]))
		for _, f := range regexp.MustCompile(`(?s)\{\s*teclas: \[([^\]]*)\],\s*accion: '[^']+',\s*descripcion: '([^']+)',?\s*(conRed: true)?`).FindAllSubmatch(g[2], -1) {
			var teclas []string
			for _, k := range regexp.MustCompile(`'([^']+)'`).FindAllSubmatch(f[1], -1) {
				tecla := string(k[1])
				if suya, ok := deLaWeb[tecla]; ok {
					tecla = suya
				}
				teclas = append(teclas, tecla)
			}
			web = append(web, strings.Join(teclas, " ")+" → "+string(f[2])+map[bool]string{true: " (red)"}[len(f[3]) > 0])
		}
	}
	var aqui []string
	for _, g := range tablaAtajos {
		aqui = append(aqui, "# "+g.titulo)
		for _, f := range g.filas {
			aqui = append(aqui, strings.Join(f.teclas, " ")+" → "+f.clave+map[bool]string{true: " (red)"}[f.conRed])
		}
	}
	if len(web) < 4 || !slices.Equal(web, aqui) {
		t.Errorf("la tabla no es la de la web:\nweb\n  %s\nFyne\n  %s", strings.Join(web, "\n  "), strings.Join(aqui, "\n  "))
	}
}

// TestCadaAtajoTieneSuFila: lo que llega como runa (en mayúscula también: el bloqueo de
// mayúsculas), como tecla con nombre o como Ctrl+A encuentra su fila, y los nombres de
// tecla de Fyne de las letras no se cruzan con las runas.
func TestCadaAtajoTieneSuFila(t *testing.T) {
	for tecla, quiero := range map[string]accionAtajo{
		teclaDeRuna('u'): atajoSubir, teclaDeRuna('U'): atajoSubir, teclaDeRuna('n'): atajoNuevaCarpeta,
		string(fyne.KeyDelete): atajoPapelera, string(fyne.KeyBackspace): atajoPapelera,
		string(fyne.KeyF2): atajoRenombrar, teclaMarcarTodo: atajoMarcarTodo,
		string(fyne.KeyEscape): atajoQuitarSeleccion, teclaDeRuna('/'): atajoBuscar,
		teclaDeRuna('V'): atajoCambiarVista, teclaDeRuna('?'): atajoAyuda,
		string(fyne.KeyU): atajoNinguno, string(fyne.KeyF3): atajoNinguno,
	} {
		if f, _ := filaDe(tecla); f.accion != quiero {
			t.Errorf("%q: acción %d, tocaba %d", tecla, f.accion, quiero)
		}
	}
}

// servicioAtajos hace de Drive con una carpeta raíz vacía y apunta las papeleras.
type servicioAtajos struct {
	mu        sync.Mutex
	papeleras []string
}

func (s *servicioAtajos) servidor(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/folders/root":
			_, _ = io.WriteString(w, `{"id":"raiz","is_root":true}`)
		case "POST /api/v1/batch/trash":
			cuerpo, _ := io.ReadAll(r.Body)
			s.mu.Lock()
			s.papeleras = append(s.papeleras, strings.TrimSpace(string(cuerpo)))
			s.mu.Unlock()
			_, _ = io.WriteString(w, `{"trashed_files":1,"trashed_folders":0}`)
		default:
			_, _ = io.WriteString(w, `{"folders":[],"files":[]}`)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// pantallaConAtajos es «Mi unidad» con tres elementos cargados, pintada en su ventana y
// con el teclado enganchado al lienzo como en `cmd/drive`.
func pantallaConAtajos(t *testing.T, s *servicioAtajos) (*Pantalla, *lienzoEspia) {
	t.Helper()
	p := pantallaDePrueba(t, s.servidor(t))
	e := p.explor
	e.Fase = Exito
	e.Elementos = []Elemento{{ID: "f1", Nombre: "acta.odt"}, {ID: "f2", Nombre: "plan.ods"}, {ID: "c1", Nombre: "Fotos", Carpeta: true}}
	p.win.SetContent(p.Objeto())
	c := &lienzoEspia{Canvas: p.win.Canvas(), atajos: map[string]func(fyne.Shortcut){}}
	con := func(fn func(*Pantalla)) { fn(p) }
	EngancharAtajos(c, con, con)
	return p, c
}

// lienzoEspia es el lienzo de pruebas con sus atajos a la vista: el de `fyne/test` no
// deja entregarle uno, y el driver de verdad se los entrega por su nombre.
type lienzoEspia struct {
	fyne.Canvas
	atajos map[string]func(fyne.Shortcut)
}

func (c *lienzoEspia) AddShortcut(s fyne.Shortcut, f func(fyne.Shortcut)) {
	c.atajos[s.ShortcutName()] = f
	c.Canvas.AddShortcut(s, f)
}

// pulsar entrega una tecla con nombre a la ranura de teclas sueltas, como el driver
// cuando no hay nada enfocado.
func pulsar(c fyne.Canvas, k fyne.KeyName) {
	if f := c.OnTypedKey(); f != nil {
		f(&fyne.KeyEvent{Name: k})
	}
}

// marcarTodoCon es ⌘/Ctrl+A tal como lo entrega el driver al lienzo: `ShortcutSelectAll`
// con la tecla de atajos del sistema.
func marcarTodoCon(c *lienzoEspia) { pulsarAtajo(c, &fyne.ShortcutSelectAll{}) }

func pulsarAtajo(c *lienzoEspia, s fyne.Shortcut) {
	if f := c.atajos[s.ShortcutName()]; f != nil {
		f(s)
	}
}

// dialogoEncima es el título del diálogo abierto encima de todo, o "".
func dialogoEncima(c fyne.Canvas) string {
	titulo := ""
	var buscar func(o fyne.CanvasObject)
	buscar = func(o fyne.CanvasObject) {
		switch x := o.(type) {
		case *ui.Dialogo:
			titulo = x.Titulo
		case *fyne.Container:
			for _, h := range x.Objects {
				buscar(h)
			}
		case fyne.Widget:
			for _, h := range test.WidgetRenderer(x).Objects() {
				buscar(h)
			}
		}
	}
	if top := c.Overlays().Top(); top != nil {
		buscar(top)
	}
	return titulo
}

func marcadosDe(p *Pantalla) []string {
	sel := p.seleccionados()
	ids := make([]string, 0, len(sel))
	for _, el := range sel {
		ids = append(ids, el.ID)
	}
	return ids
}

// TestLosAtajosDeExploradorHacenLoQueDiceLaHoja: Ctrl+A marca todo lo cargado (y otra vez
// no lo desmarca), Esc lo vacía, F2 renombra el único marcado, Supr manda uno a la
// papelera sin preguntar y varios con su confirmación, «v» cambia la vista, «/» lleva al
// buscador y «?» abre la hoja con los grupos de la tabla.
//
// VISTO EN ROJO (2026-11-13): sin `EngancharAtajos`, ni Ctrl+A ni Supr ni F2 ni Esc
// llegaban a Drive —solo «u» y «n»—; y con `marcarTodo` en vez del bucle propio, el
// segundo Ctrl+A dejaba la selección vacía.
func TestLosAtajosDeExploradorHacenLoQueDiceLaHoja(t *testing.T) {
	s := &servicioAtajos{}
	p, c := pantallaConAtajos(t, s)

	// En el Mac el driver da ⌘A como `ShortcutSelectAll` y Ctrl+A como atajo propio; en
	// el resto, al revés con ⌘ (la tecla Súper). La web atiende las dos.
	pulsarAtajo(c, &desktop.CustomShortcut{KeyName: fyne.KeyA, Modifier: fyne.KeyModifierControl})
	if got := marcadosDe(p); len(got) != 3 {
		t.Fatalf("Ctrl+A como atajo propio marca %q", got)
	}
	pulsar(c, fyne.KeyEscape)
	pulsarAtajo(c, &desktop.CustomShortcut{KeyName: fyne.KeyA, Modifier: fyne.KeyModifierSuper})
	if got := marcadosDe(p); len(got) != 3 {
		t.Fatalf("⌘A como atajo propio marca %q", got)
	}
	pulsar(c, fyne.KeyEscape)
	marcarTodoCon(c)
	marcarTodoCon(c)
	if got := marcadosDe(p); !slices.Equal(got, []string{"f1", "f2", "c1"}) {
		t.Fatalf("Ctrl+A dos veces deja marcado %q", got)
	}
	pulsar(c, fyne.KeyF2)
	if got := dialogoEncima(c); got != "" {
		t.Errorf("F2 con tres marcados abre «%s»: solo renombra uno", got)
	}
	pulsar(c, fyne.KeyDelete)
	if got := dialogoEncima(c); got != p.tr.T("files.batch.trashTitle", nil) {
		t.Fatalf("Supr con tres marcados no pide confirmación (encima: %q)", got)
	}
	pulsar(c, fyne.KeyEscape) // cierra el diálogo, no la selección
	if dialogoEncima(c) != "" || len(marcadosDe(p)) != 3 {
		t.Fatalf("Esc con el diálogo abierto no lo cierra o vacía la selección: %q", marcadosDe(p))
	}
	pulsar(c, fyne.KeyEscape)
	if got := marcadosDe(p); len(got) != 0 {
		t.Fatalf("Esc no vacía la selección: %q", got)
	}

	p.marcar("f2")
	pulsar(c, fyne.KeyF2)
	if got := dialogoEncima(c); got != p.tr.T("files.rename.fileTitle", nil) {
		t.Fatalf("F2 con uno marcado no abre «Renombrar» (encima: %q)", got)
	}
	ui.Descartar(c)
	pulsar(c, fyne.KeyDelete)
	if got := dialogoEncima(c); got != "" {
		t.Errorf("Supr con uno solo pregunta («%s»): va directo, con «Deshacer»", got)
	}
	for i := 0; i < 200; i++ {
		s.mu.Lock()
		n := len(s.papeleras)
		s.mu.Unlock()
		if n > 0 {
			break
		}
		esperar()
	}
	s.mu.Lock()
	papeleras := slices.Clone(s.papeleras)
	s.mu.Unlock()
	if len(papeleras) != 1 || !strings.Contains(papeleras[0], `"f2"`) || strings.Contains(papeleras[0], `"f1"`) {
		t.Errorf("Supr manda a la papelera %q; tocaba solo f2", papeleras)
	}

	rejilla := p.explor.Rejilla
	test.TypeOnCanvas(c, "v")
	if p.explor.Rejilla == rejilla {
		t.Error("«v» no cambia de vista")
	}
	test.TypeOnCanvas(c, "/")
	if c.Focused() == nil || c.Focused() != fyne.Focusable(p.explor.buscador) {
		t.Errorf("«/» no lleva al buscador: el foco está en %T", c.Focused())
	}
	c.Unfocus()

	test.TypeOnCanvas(c, "?")
	if got := dialogoEncima(c); got != p.tr.T("shortcuts.title", nil) {
		t.Fatalf("«?» no abre la hoja de atajos (encima: %q)", got)
	}
}

// TestLaHojaEsLaTablaConLasTeclasDelSistema: tres grupos con todas sus filas; en un Mac
// solo ⌫ para la papelera y ⌘A, fuera de él solo Supr.
func TestLaHojaEsLaTablaConLasTeclasDelSistema(t *testing.T) {
	p := pantallaDePrueba(t, (&servicioAtajos{}).servidor(t))
	for _, mac := range []bool{false, true} {
		grupos := p.gruposDeAtajos(mac)
		if len(grupos) != len(tablaAtajos) {
			t.Fatalf("la hoja tiene %d grupos", len(grupos))
		}
		for i, g := range grupos {
			if g.Titulo != p.tr.T(tablaAtajos[i].titulo, nil) || len(g.Atajos) != len(tablaAtajos[i].filas) {
				t.Errorf("el grupo %d de la hoja no es el de la tabla: %+v", i, g)
			}
		}
		papelera := grupos[0].Atajos[2].Teclas
		quiero := []string{p.tr.T("shortcuts.key.delete", nil)}
		if mac {
			quiero = []string{"⌫"}
		}
		if !slices.Equal(papelera, quiero) {
			t.Errorf("mac=%v: la papelera se escribe %q", mac, papelera)
		}
		if got := grupos[1].Atajos[0].Teclas; len(got) != 1 || got[0] != ui.AtajoDelSistema("Ctrl+A") {
			t.Errorf("marcar todo se escribe %q", got)
		}
	}
}

// TestNingunAtajoConUnaCapaEncimaNiSinRedLosQueEscriben: con un diálogo abierto el
// teclado es suyo; sin red no se marca ni se tira nada, pero la hoja y la vista sí; y
// fuera de «Mi unidad» Drive no se queda ninguna tecla.
func TestNingunAtajoConUnaCapaEncimaNiSinRedLosQueEscriben(t *testing.T) {
	p, c := pantallaConAtajos(t, &servicioAtajos{})

	p.pedirCarpeta()
	marcarTodoCon(c)
	test.TypeOnCanvas(c, "?")
	if len(marcadosDe(p)) != 0 || dialogoEncima(c) != p.tr.T("files.newFolder.title", nil) {
		t.Fatalf("con «Nueva carpeta» abierta los atajos siguen: marcados %q, encima %q", marcadosDe(p), dialogoEncima(c))
	}
	ui.Descartar(c)

	p.explor.Desconectado = true
	marcarTodoCon(c)
	if got := marcadosDe(p); len(got) != 0 {
		t.Errorf("sin red Ctrl+A marca %q", got)
	}
	test.TypeOnCanvas(c, "n")
	if got := dialogoEncima(c); got != "" {
		t.Errorf("sin red «n» abre «%s»", got)
	}
	test.TypeOnCanvas(c, "?")
	if got := dialogoEncima(c); got != p.tr.T("shortcuts.title", nil) {
		t.Errorf("sin red «?» no abre la hoja (encima: %q)", got)
	}
	ui.Descartar(c)

	p.explor.Desconectado = false
	p.seccion = Papelera
	if p.AtajoMarcarTodo() || p.Atajo('?') {
		t.Error("fuera de «Mi unidad» Drive se queda los atajos del explorador")
	}
}

// teclaComoElDriver entrega una tecla como el driver de escritorio: al control enfocado
// si lo hay y, si no, a la ranura de teclas sueltas del lienzo.
func teclaComoElDriver(c fyne.Canvas, k fyne.KeyName) {
	if f := c.Focused(); f != nil {
		f.TypedKey(&fyne.KeyEvent{Name: k})
		return
	}
	pulsar(c, k)
}

// casillasDe son las casillas de marcar del árbol pintado, en orden.
func casillasDe(o fyne.CanvasObject) []*ui.Eleccion {
	var todas []*ui.Eleccion
	var buscar func(o fyne.CanvasObject)
	buscar = func(o fyne.CanvasObject) {
		switch x := o.(type) {
		case *ui.Eleccion:
			todas = append(todas, x)
		case *fyne.Container:
			for _, h := range x.Objects {
				buscar(h)
			}
		case fyne.Widget:
			for _, h := range test.WidgetRenderer(x).Objects() {
				buscar(h)
			}
		}
	}
	buscar(o)
	return todas
}

// TestLosAtajosSiguenTrasMarcarConUnClic: en el escritorio se marca con un clic en la
// casilla, y el clic le da el foco. Con el foco en ella, Supr, F2, Esc y las letras de
// la tabla tienen que seguir llegando a Drive, como en la web el `keydown` de la casilla
// sube al documento.
//
// VISTO EN ROJO (2026-11-13): sin `SubeTeclas`, F2 y Esc con la casilla recién pulsada
// no hacían nada: el driver las entrega al control enfocado y la casilla se las comía.
func TestLosAtajosSiguenTrasMarcarConUnClic(t *testing.T) {
	for _, rejilla := range []bool{false, true} {
		p, c := pantallaConAtajos(t, &servicioAtajos{})
		p.explor.Rejilla = rejilla
		p.Repintar()
		esperar()
		p.win.Canvas().Capture() // la lista pinta sus filas al dibujarse
		casillas := casillasDe(p.win.Content())
		// En la lista la primera es la de la cabecera («marcar todo»).
		var fila *ui.Eleccion
		for _, k := range casillas {
			// La de la tarjeta se anuncia con su nombre («Seleccionar acta.odt»).
			if k.Etiqueta == p.tr.T("files.select", i18n.Vars{"name": "acta.odt"}) || (!rejilla && k != casillas[0]) {
				fila = k
				break
			}
		}
		if fila == nil {
			t.Fatalf("rejilla=%v: no hay casilla de fila entre %d", rejilla, len(casillas))
		}
		fila.Show() // la destapa el ratón al pasar por encima para pulsarla
		test.Tap(fila)
		if c.Focused() != fyne.Focusable(fila) || len(marcadosDe(p)) != 1 {
			t.Fatalf("rejilla=%v: el clic no marca y enfoca (foco %T, marcados %q)", rejilla, c.Focused(), marcadosDe(p))
		}
		teclaComoElDriver(c, fyne.KeyF2)
		if got := dialogoEncima(c); got != p.tr.T("files.rename.fileTitle", nil) {
			t.Fatalf("rejilla=%v: F2 con el foco en la casilla no renombra (encima: %q)", rejilla, got)
		}
		ui.Descartar(c)
		c.Focus(fila)
		teclaComoElDriver(c, fyne.KeyEscape)
		if got := marcadosDe(p); len(got) != 0 {
			t.Errorf("rejilla=%v: Esc con el foco en la casilla no vacía la selección: %q", rejilla, got)
		}
		fila.TypedRune('?')
		if got := dialogoEncima(c); got != p.tr.T("shortcuts.title", nil) {
			t.Errorf("rejilla=%v: «?» con el foco en la casilla no abre la hoja (encima: %q)", rejilla, got)
		}
		ui.Descartar(c)
	}
}

// TestSuprMantenidaNoRepiteLaPapelera: el driver repite la tecla mientras se mantiene;
// con un solo elemento, la primera pulsación lo manda y la siguiente ya no encuentra nada.
//
// VISTO EN ROJO (2026-11-13): sin soltar la selección, cada repetición pedía otra vez
// la papelera para el mismo fichero.
func TestSuprMantenidaNoRepiteLaPapelera(t *testing.T) {
	p, _ := pantallaConAtajos(t, &servicioAtajos{})
	p.marcar("f1")
	if !p.AtajoTecla(&fyne.KeyEvent{Name: fyne.KeyDelete}) {
		t.Fatal("Supr con uno marcado no hace nada")
	}
	if p.AtajoTecla(&fyne.KeyEvent{Name: fyne.KeyDelete}) {
		t.Errorf("la repetición de Supr vuelve a mandar a la papelera (marcados %q)", marcadosDe(p))
	}
}
