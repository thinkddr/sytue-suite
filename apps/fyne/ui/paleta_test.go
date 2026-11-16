// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"context"
	"errors"
	"image"
	"image/color"
	"slices"
	"strings"
	"sync"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// capaDePrueba son dos acciones y tres áreas, para ver el filtro, el orden y el tope.
func capaDePrueba(ejecutado *[]string) (acciones, areas CapaDePaleta) {
	hacer := func(id string) func() { return func() { *ejecutado = append(*ejecutado, id) } }
	acciones = CapaDePaleta{Comandos: []ComandoDePaleta{
		{ID: "nuevo", Rotulo: "Nuevo proyecto", Grupo: "Acciones", Icono: iconos.Mas, Ejecutar: hacer("nuevo")},
		{ID: "ajustes", Rotulo: "Ajustes", Grupo: "Acciones", Palabras: "preferencias", Ejecutar: hacer("ajustes")},
	}}
	areas = CapaDePaleta{Limite: 2, Comandos: []ComandoDePaleta{
		{ID: "fab", Rotulo: "Órdenes de fabricación", Grupo: "Áreas", Pista: "Producción", Ejecutar: hacer("fab")},
		{ID: "cli", Rotulo: "Clientes", Grupo: "Áreas", Pista: "Ventas", Ejecutar: hacer("cli")},
		{ID: "fac", Rotulo: "Facturas de cliente", Grupo: "Áreas", Pista: "Ventas", Ejecutar: hacer("fac")},
	}}
	return acciones, areas
}

func idsDe(cs []ComandoDePaleta) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.ID)
	}
	return out
}

// TestLaPaletaFiltraComoLaWeb es `comandosQueCasan` con `filterCommands`: cada palabra
// tiene que estar (en el rótulo, las palabras, la pista o el grupo), sin tildes ni
// mayúsculas; capa a capa en su orden, con el tope de cada una, y lo que ya filtró el
// servidor pasa tal cual.
func TestLaPaletaFiltraComoLaWeb(t *testing.T) {
	var nada []string
	acciones, areas := capaDePrueba(&nada)
	capas := []CapaDePaleta{acciones, areas}
	casos := []struct {
		consulta string
		quiero   []string
	}{
		// Sin consulta, todo; el tope de dos se come la tercera área.
		{"", []string{"nuevo", "ajustes", "fab", "cli"}},
		{"fabricacion", []string{"fab"}},
		{"PREFERENCIAS", []string{"ajustes"}},
		// La pista también se busca: «ventas» encuentra las dos de Ventas.
		{"ventas", []string{"cli", "fac"}},
		// Y el grupo: «acciones» encuentra el grupo entero.
		{"acciones", []string{"nuevo", "ajustes"}},
		// Todas las palabras (AND), en cualquier orden.
		{"cliente factura", []string{"fac"}},
		{"cliente zzz", nil},
	}
	for _, c := range casos {
		if got := idsDe(ComandosQueCasan(capas, c.consulta)); !slices.Equal(got, c.quiero) {
			t.Errorf("«%s» casa con %q; se esperaba %q", c.consulta, got, c.quiero)
		}
	}
	regs := CapaDePaleta{Comandos: []ComandoDePaleta{{ID: "r1", Rotulo: "Acme S.L.", Grupo: "Registros", YaFiltrado: true}}}
	if got := idsDe(ComandosQueCasan([]CapaDePaleta{regs}, "FV-2026")); !slices.Equal(got, []string{"r1"}) {
		t.Errorf("lo ya filtrado por el servidor se vuelve a filtrar: %q", got)
	}
}

// TestLasCapasSeRegistranYSeQuitan: el orden es el de registro, cambiar una capa la deja
// en su sitio, y quitarla la saca de la lista.
func TestLasCapasSeRegistranYSeQuitan(t *testing.T) {
	var nada []string
	acciones, areas := capaDePrueba(&nada)
	p := NuevaPaletaDeLaSuite(traductorPrueba(t))
	p.PonerCapa("app", acciones)
	p.PonerCapa("app:areas", areas)
	p.PonerCapa("app", CapaDePaleta{Comandos: acciones.Comandos[:1]})
	if got := idsDe(ComandosQueCasan(p.Capas(), "")); !slices.Equal(got, []string{"nuevo", "fab", "cli"}) {
		t.Errorf("tras cambiar la primera capa la lista es %q", got)
	}
	p.QuitarCapa("app")
	if got := idsDe(ComandosQueCasan(p.Capas(), "")); !slices.Equal(got, []string{"fab", "cli"}) {
		t.Errorf("la capa quitada sigue en la lista: %q", got)
	}
}

// TestLaPaletaSeRecorreConElTeclado: se abre en blanco, las flechas dan la vuelta, Inicio
// y Fin van a los extremos, Intro ejecuta lo resaltado CERRANDO antes, Escape cierra, y
// una letra no es de la paleta sino del campo.
func TestLaPaletaSeRecorreConElTeclado(t *testing.T) {
	var hecho []string
	acciones, areas := capaDePrueba(&hecho)
	p := NuevaPaletaDeLaSuite(traductorPrueba(t))
	p.PonerCapa("acciones", acciones)
	p.PonerCapa("areas", areas)
	var oido []string
	p.Oir(func(abierta bool, q string) {
		if abierta {
			oido = append(oido, q)
		}
	})
	p.Abrir()
	p.Escribir("ventas")
	tecla := func(n fyne.KeyName) bool { return p.Teclear(&fyne.KeyEvent{Name: n}) }
	tecla(fyne.KeyDown)
	if p.Cursor() != 1 {
		t.Fatalf("la flecha abajo deja el cursor en %d", p.Cursor())
	}
	tecla(fyne.KeyDown)
	if p.Cursor() != 0 {
		t.Errorf("la flecha abajo en la última no da la vuelta: %d", p.Cursor())
	}
	tecla(fyne.KeyEnd)
	if p.Cursor() != 1 {
		t.Errorf("Fin deja el cursor en %d", p.Cursor())
	}
	var abiertaAlEjecutar bool
	p.PonerCapa("areas", CapaDePaleta{Comandos: []ComandoDePaleta{
		areas.Comandos[1],
		{ID: "fac", Rotulo: "Facturas de cliente", Grupo: "Áreas", Pista: "Ventas", Ejecutar: func() { abiertaAlEjecutar = p.Abierta() }},
	}})
	tecla(fyne.KeyEnd)
	tecla(fyne.KeyReturn)
	if p.Abierta() || abiertaAlEjecutar {
		t.Errorf("Intro deja la paleta abierta (%v) o ejecuta con ella abierta (%v)", p.Abierta(), abiertaAlEjecutar)
	}
	if !slices.Equal(oido, []string{"", "ventas"}) {
		t.Errorf("quien oye la paleta recibió %q", oido)
	}

	p.Abrir()
	if p.Consulta() != "" || p.Cursor() != 0 {
		t.Errorf("la paleta reabre con la consulta %q y el cursor en %d", p.Consulta(), p.Cursor())
	}
	if tecla(fyne.KeyA) {
		t.Error("la paleta se come una tecla que es del campo")
	}
	tecla(fyne.KeyEscape)
	if p.Abierta() {
		t.Error("Escape no cierra la paleta")
	}
}

// TestLaPaletaNoAfirmaQueNoHayNada: sin escribir invita a buscar; con una capa que aún
// espera a su servidor dice «buscando», y solo entonces «sin resultados».
func TestLaPaletaNoAfirmaQueNoHayNada(t *testing.T) {
	tr := traductorPrueba(t)
	p := NuevaPaletaDeLaSuite(tr)
	p.Abrir()
	if got := p.Vacio(); got != tr.T("suitePalette.typeToSearch", nil) {
		t.Errorf("sin escribir se lee %q", got)
	}
	p.PonerCapa("registros", CapaDePaleta{Buscando: true})
	p.Escribir("acme")
	if got := p.Vacio(); got != tr.T("search.busy", nil) {
		t.Errorf("con una capa buscando se lee %q", got)
	}
	p.PonerCapa("registros", CapaDePaleta{})
	if got := p.Vacio(); !strings.Contains(got, "acme") {
		t.Errorf("sin nada que casar se lee %q", got)
	}
}

// hubDePrueba contesta lo que se le diga y apunta lo que le preguntan.
type hubDePrueba struct {
	mu      sync.Mutex
	pedidas []string
	hits    []api.HubHit
	err     error
	// soltar, si lo hay, retiene la respuesta hasta que se cierra: así se ve el
	// «buscando» sin depender de lo que tarde el reloj (con -race, la espera de cero
	// dispara antes de que el test llegue a mirar).
	soltar chan struct{}
}

func (h *hubDePrueba) buscar(ctx context.Context, q string) ([]api.HubHit, error) {
	h.mu.Lock()
	soltar := h.soltar
	h.mu.Unlock()
	if soltar != nil {
		select {
		case <-soltar:
		case <-ctx.Done():
		}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pedidas = append(h.pedidas, q)
	return h.hits, h.err
}

func (h *hubDePrueba) pedido() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.pedidas)
}

// TestLaPaletaBuscaEnElHub: con dos letras pregunta al Hub (con una no), dice que busca
// mientras tanto y pone debajo de las capas sus resultados, agrupados por producto con
// el nombre corto. Elegir uno lo abre; si el Hub falla, lo dice.
func TestLaPaletaBuscaEnElHub(t *testing.T) {
	esperaAntes := EsperaPaleta
	EsperaPaleta = 0
	t.Cleanup(func() { EsperaPaleta = esperaAntes })
	tr := traductorPrueba(t)
	hub := &hubDePrueba{soltar: make(chan struct{}), hits: []api.HubHit{
		{Source: "mail", ID: "1", Title: "Factura de septiembre", URL: "/mail/1"},
		{Source: "drive", ID: "2", Title: "factura.pdf", URL: "/drive/2"},
		{Source: "mail", ID: "3", Title: "Re: factura", URL: "/mail/3"},
	}}
	p := NuevaPaletaDeLaSuite(tr)
	t.Cleanup(p.Parar)
	p.BuscarEnHub = hub.buscar
	var abierto []string
	p.AlAbrirResultado = func(h api.HubHit) { abierto = append(abierto, h.URL) }
	p.PonerCapa("app", CapaDePaleta{Comandos: []ComandoDePaleta{{ID: "f", Rotulo: "Facturas", Grupo: "Áreas"}}})
	p.Abrir()
	p.Escribir("f")
	p.mu.Lock()
	espera := p.espera
	p.mu.Unlock()
	if espera != nil || p.Buscando() || len(hub.pedido()) != 0 {
		t.Fatalf("con una letra se pregunta al Hub: %q", hub.pedido())
	}
	p.Escribir("factura")
	if !p.Buscando() {
		t.Error("mientras el Hub contesta la paleta no dice que busca")
	}
	close(hub.soltar)
	esperar(t, func() bool { return !p.Buscando() })
	opciones := p.Opciones()
	vistos := make([]string, 0, len(opciones))
	for _, o := range opciones {
		vistos = append(vistos, o.Grupo+":"+o.Rotulo)
	}
	quiero := []string{"Áreas:Facturas", "Mail:Factura de septiembre", "Mail:Re: factura", "Drive:factura.pdf"}
	if !slices.Equal(vistos, quiero) {
		t.Errorf("opciones = %q; se esperaba %q", vistos, quiero)
	}
	p.Elegir(p.Opciones()[3])
	if p.Abierta() || !slices.Equal(abierto, []string{"/drive/2"}) {
		t.Errorf("elegir un resultado deja la paleta abierta=%v y abre %q", p.Abierta(), abierto)
	}

	hub.mu.Lock()
	hub.hits, hub.err = nil, errors.New("caído")
	hub.mu.Unlock()
	p.Abrir()
	p.Escribir("zzz")
	esperar(t, func() bool { return !p.Buscando() })
	if got := p.Vacio(); got != tr.T("search.failedShort", nil) {
		t.Errorf("con el Hub caído se lee %q", got)
	}
}

// TestElAtajoAbreLaPaletaDeLaSuite: ⌘K abre la paleta de la suite —ya no enfoca el
// buscador de la barra, como en la web— con el foco en su campo; otro ⌘K la cierra. Si
// la app se lo queda (`CederAtajoSi`), no se abre; y con otro diálogo encima, ⌘K es de
// ese diálogo.
//
// VISTO EN ROJO: con el `pulsarAtajo` de antes, el foco acababa en el buscador de la
// barra y no había capa.
func TestElAtajoAbreLaPaletaDeLaSuite(t *testing.T) {
	b := barraPrueba(t)
	w := test.NewWindow(b)
	defer w.Close()
	w.Resize(fyne.NewSize(1280, 800))
	c := w.Canvas()

	b.buscador.pulsarAtajo(c)
	if !b.Paleta().Abierta() || c.Overlays().Top() == nil {
		t.Fatal("⌘K no abre la paleta de la suite")
	}
	if c.Focused() != b.Paleta().Campo() {
		t.Errorf("la paleta se abre con el foco en %T y no en su campo", c.Focused())
	}
	b.buscador.pulsarAtajo(c)
	if b.Paleta().Abierta() || c.Overlays().Top() != nil {
		t.Error("el segundo ⌘K no cierra la paleta")
	}

	quiere := true
	b.CederAtajoSi(func() bool { return quiere })
	b.buscador.pulsarAtajo(c)
	if b.Paleta().Abierta() {
		t.Error("la app se quedó ⌘K y aun así se abre la paleta")
	}
	quiere = false

	d := NuevoDialogo("Otro", NuevoParrafo("cuerpo", tema.TextoSM, Normal, "text", tema.Claro))
	d.Mostrar(c)
	b.buscador.pulsarAtajo(c)
	if b.Paleta().Abierta() {
		t.Error("⌘K abre la paleta encima de otro diálogo")
	}
	d.Cerrar()
	b.buscador.pulsarAtajo(c)
	if !b.Paleta().Abierta() {
		t.Error("la app devolvió ⌘K y no se abre la paleta")
	}
	b.Paleta().Cerrar()
}

// abrirPaletaEn monta un fondo en una ventana de `tam`, abre la paleta encima con una
// capa de ejemplo y devuelve la captura.
func abrirPaletaEn(t *testing.T, tam fyne.Size, fondo fyne.CanvasObject) (*PaletaDeLaSuite, fyne.Window) {
	t.Helper()
	var nada []string
	acciones, areas := capaDePrueba(&nada)
	p := NuevaPaletaDeLaSuite(traductorPrueba(t))
	p.PonerCapa("acciones", acciones)
	p.PonerCapa("areas", areas)
	w := test.NewTempWindow(t, fondo)
	w.SetPadded(false)
	w.Resize(tam)
	p.EnLienzo(w.Canvas())
	p.Abrir()
	t.Cleanup(p.Cerrar)
	return p, w
}

// TestLaPaletaSeColocaComoEnChromium: a 1280×800 el panel va en (360, 96) con 560 de
// ancho, el velo atenúa lo de detrás y el pie lleva las teclas; a 390×844 es la hoja,
// desde 40 y a todo el ancho, con «Cancelar» y sin pie.
func TestLaPaletaSeColocaComoEnChromium(t *testing.T) {
	papel := canvas.NewRectangle(tema.Color("bg", tema.Claro))
	p, w := abrirPaletaEn(t, fyne.NewSize(1280, 800), papel)
	pos, tam := p.panel.Position(), p.panel.Size()
	if pos != fyne.NewPos(360, 96) || tam.Width != AnchoPaleta {
		t.Errorf("el panel va en %v con %v; Chromium lo pone en (360, 96) con 560", pos, tam)
	}
	// Sin espacios: el rótulo de grupo va en versalitas, letra a letra.
	textos := strings.ReplaceAll(textosDeArbol(p.panel), " ", "")
	for _, s := range []string{"↑↓", "↵", "esc", "ACCIONES", "ÁREAS", "Nuevoproyecto"} {
		if !strings.Contains(textos, s) {
			t.Errorf("no se lee %q en la paleta: %q", s, textos)
		}
	}
	img := w.Canvas().Capture()
	if got := pixelEn(img, w, 100, 700); got == tema.Color("bg", tema.Claro) {
		t.Error("con la paleta abierta el fondo se pinta igual: el velo no atenúa")
	}
	if p.disp.altoNatural() > FraccionAltoPaleta*800 {
		t.Errorf("la paleta de ejemplo no debería tocar el tope del 70 %%: %v", p.disp.altoNatural())
	}
	p.Cerrar()

	restaurar := TactilDePrueba(true)
	defer restaurar()
	p, _ = abrirPaletaEn(t, fyne.NewSize(390, 844), canvas.NewRectangle(tema.Color("bg", tema.Claro)))
	pos, tam = p.panel.Position(), p.panel.Size()
	if pos != fyne.NewPos(0, HuecoHoja) || tam.Width != 390 {
		t.Errorf("la hoja va en %v con %v; Chromium la pone en (0, 40) a todo el ancho", pos, tam)
	}
	if p.disp.cancelar == nil || p.disp.pie != nil {
		t.Errorf("la hoja lleva «Cancelar»=%v y pie=%v", p.disp.cancelar != nil, p.disp.pie != nil)
	}
	if AltoFilaPaleta() < SueloTactilBarra {
		t.Errorf("con el dedo la fila mide %v, menos de 44", AltoFilaPaleta())
	}
}

// TestLaPaletaNoPasaDelSetentaPorCiento: muchas opciones no estiran el panel más allá
// del `max-h-[70dvh]`; la lista desplaza y la fila del cursor queda a la vista.
func TestLaPaletaNoPasaDelSetentaPorCiento(t *testing.T) {
	cs := make([]ComandoDePaleta, 0, 40)
	for i := range 40 {
		cs = append(cs, ComandoDePaleta{ID: string(rune('a' + i)), Rotulo: "Opción " + string(rune('A'+i)), Grupo: "Muchas"})
	}
	p := NuevaPaletaDeLaSuite(traductorPrueba(t))
	p.PonerCapa("muchas", CapaDePaleta{Comandos: cs})
	w := test.NewTempWindow(t, canvas.NewRectangle(color.NRGBA{}))
	w.SetPadded(false)
	w.Resize(fyne.NewSize(1280, 800))
	p.EnLienzo(w.Canvas())
	p.Abrir()
	defer p.Cerrar()
	if alto := p.panel.Size().Height; alto > FraccionAltoPaleta*800+0.5 {
		t.Errorf("el panel mide %v y pasa del 70 %% (%v)", alto, FraccionAltoPaleta*800)
	}
	p.Teclear(&fyne.KeyEvent{Name: fyne.KeyEnd})
	p.panel.Refresh()
	if p.disp.desplaza.Offset.Y == 0 {
		t.Error("con el cursor en la última opción la lista no ha bajado")
	}
}

// TestLaPaletaTraslucidaOcultaLaPantalla: en el modo traslúcido la paleta va sobre
// `menu-surface`, opaco: la lista tiene que leerse sobre cualquier pantalla.
func TestLaPaletaTraslucidaOcultaLaPantalla(t *testing.T) {
	tema.Elegir("traslucido")
	t.Cleanup(func() { tema.Elegir("") })
	debajo := canvas.NewRectangle(color.NRGBA{R: 255, A: 255})
	p, w := abrirPaletaEn(t, fyne.NewSize(800, 600), container.NewStack(debajo))
	img := w.Canvas().Capture()
	pos := p.panel.Position()
	// En mitad de la segunda fila (la primera lleva el `bg-selected` del cursor), a la
	// derecha, donde no hay texto.
	x := pos.X + AnchoPaleta - 10
	y := pos.Y + tema.Borde + AltoCampoPaleta + tema.Borde + ListaPaletaP + AltoGrupoPaleta(true) + 1.5*AltoFilaPaleta()
	if got, want := pixelEn(img, w, x, y), tema.Color("menu-surface", tema.Translucido); !cercaDe(got, want, 3) {
		t.Errorf("la pantalla se mezcla con la lista: %v; el papel opaco es %v", got, want)
	}
}

// pixelEn lee el color en (x, y) del lienzo, en px de CSS.
func pixelEn(img image.Image, w fyne.Window, x, y float32) color.NRGBA {
	escala := float32(img.Bounds().Dx()) / w.Canvas().Size().Width
	return px(img, int(x*escala), int(y*escala))
}

// TestElContratoDeLaPaletaEsElDeLaWeb ata la paleta a sus dos fuentes de la web: si la
// web cambia la espera, el mínimo de letras, las teclas del pie, los textos o el filtro,
// este test se pone en rojo en vez de dejar que las dos se separen en silencio.
func TestElContratoDeLaPaletaEsElDeLaWeb(t *testing.T) {
	suite := webref.Read(t, "apps/frontend/packages/shell/src/PaletaDeLaSuite.tsx")
	capas := webref.Read(t, "apps/frontend/packages/shell/src/paleta.ts")
	comandos := webref.Read(t, "apps/frontend/packages/ui/src/components/PaletaDeComandos.tsx")
	for _, c := range []struct{ fuente, nombre, trozo, porque string }{
		{suite, "PaletaDeLaSuite.tsx", "ESPERA_BUSQUEDA_MS = 250", "la espera antes de preguntar al Hub (EsperaPaleta)"},
		{suite, "PaletaDeLaSuite.tsx", "q.length < 2", "las dos letras desde las que se pregunta (MinimoBuscador)"},
		{suite, "PaletaDeLaSuite.tsx", "<Kbd>↑↓</Kbd> {t('suitePalette.hint.move')}", "la primera tecla del pie"},
		{suite, "PaletaDeLaSuite.tsx", "<Kbd>↵</Kbd> {t('suitePalette.hint.open')}", "la segunda tecla del pie"},
		{suite, "PaletaDeLaSuite.tsx", "<Kbd>esc</Kbd> {t('suitePalette.hint.close')}", "la tercera tecla del pie"},
		{suite, "PaletaDeLaSuite.tsx", "useAtajo('Ctrl K')", "el atajo a la derecha del pie"},
		{suite, "PaletaDeLaSuite.tsx", "if (!q) return t('suitePalette.typeToSearch');", "el vacío sin escribir"},
		{suite, "PaletaDeLaSuite.tsx", "if (buscando || registro.capas().some((c) => c.buscando)) return t('search.busy');", "el «buscando» de cualquier capa"},
		{suite, "PaletaDeLaSuite.tsx", "if (fallo) return t('search.failedShort');", "el fallo del Hub"},
		{suite, "PaletaDeLaSuite.tsx", "return t('search.emptyTitle', { query: q });", "el «sin resultados»"},
		{suite, "PaletaDeLaSuite.tsx", "placeholder={t('suitePalette.placeholder')}", "la marca de agua del campo"},
		{suite, "PaletaDeLaSuite.tsx", "cerrar();\n        if ('comando' in o)", "que la paleta se cierra ANTES de ejecutar"},
		{capas, "paleta.ts", "keywords: [c.palabras, c.pista].filter(Boolean).join(' ')", "que la pista y las palabras se buscan"},
		{capas, "paleta.ts", "...capa.comandos.filter((c) => c.yaFiltrado)", "que lo ya filtrado va detrás, tal cual"},
		{capas, "paleta.ts", "casan.slice(0, capa.limite)", "el tope por capa"},
		{comandos, "PaletaDeComandos.tsx", "md:max-h-[70dvh] md:max-w-[560px]", "el tope de alto y el ancho (FraccionAltoPaleta, AnchoPaleta)"},
		{comandos, "PaletaDeComandos.tsx", "md:pt-[12dvh]", "el 12 % de arriba (FraccionArribaPaleta)"},
		{comandos, "PaletaDeComandos.tsx", "case 'Home':", "Inicio va a la primera"},
		{comandos, "PaletaDeComandos.tsx", "case 'End':", "Fin va a la última"},
		{comandos, "PaletaDeComandos.tsx", "border-t px-4 py-2 max-md:hidden", "el pie, solo en escritorio"},
	} {
		if !strings.Contains(c.fuente, c.trozo) {
			t.Errorf("%s ya no dice %q (%s)", c.nombre, c.trozo, c.porque)
		}
	}
	if EsperaPaleta.Milliseconds() != 250 || MinimoBuscador != 2 || AnchoPaleta != 560 ||
		FraccionAltoPaleta != 0.70 || FraccionArribaPaleta != 0.12 {
		t.Errorf("la paleta dice %v/%v/%v/%v/%v y la web 250 ms, 2, 560, 70 %% y 12 %%",
			EsperaPaleta, MinimoBuscador, AnchoPaleta, FraccionAltoPaleta, FraccionArribaPaleta)
	}
}
