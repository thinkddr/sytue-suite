// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"fmt"
	"image/color"
	"os"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// montar crea el editor con el cromo de verdad alrededor, como lo monta `cmd/diapos`,
// en una ventana de prueba de 1280 × 800.
func montar(t *testing.T, contenido string) (*Editor, *editor.Contexto, fyne.Window) {
	t.Helper()
	tr := traductor(t)
	ctx := &editor.Contexto{Tipo: editor.Presentacion, Estado: editor.Listo, Zoom: 100, T: tr}
	e := NuevoEditor(ctx)
	// Sin reloj: el folio de una caja repinta la cinta desde el suyo, que en las pruebas
	// correría en su propia rutina (ahí `fyne.Do` no cambia de hilo).
	e.Programar = func(time.Duration, func()) {}
	ctx.Lienzo = e
	ctx.SoloLectura = !e.Cargar(contenido)
	marco := editor.NuevoMarco(ctx, Registro(e), nil)
	cromo := ctx.Refrescar
	ctx.Refrescar = func() { cromo(); e.Refrescar() }
	ctx.Refrescar()
	w := test.NewTempWindow(t, container.NewStack(marco, e.Avisos()))
	w.SetPadded(false)
	w.Resize(fyne.NewSize(1280, 800))
	return e, ctx, w
}

// TestElEditorEnseñaLaPresentacionEntera: la muestra se entiende entera, se enseña con
// sus nueve diapositivas y serializar la devuelve sin perder nada.
func TestElEditorEnseñaLaPresentacionEntera(t *testing.T) {
	e, ctx, w := montar(t, string(muestra(t)))
	if ctx.SoloLectura {
		t.Fatal("la muestra se escribe entera y no puede abrirse en solo lectura")
	}
	if u, total, _ := e.Estadisticas(); u != 1 || total != len(e.Modelo().Diapos) {
		t.Errorf("la barra de estado diría %d de %d", u, total)
	}
	if !e.Modelo().Fiel([]byte(e.Serializar())) {
		t.Error("lo que se guardaría no reproduce lo que se enseña")
	}
	guarda(t, "editor.png", w.Canvas().Capture())
}

// TestElCarrilAñadeDuplicaYBorra: los tres botones del pie del carril, con el diálogo
// de confirmación del borrado, y cada cambio marca el documento como sin guardar y
// avisa a quien guarda.
func TestElCarrilAñadeDuplicaYBorra(t *testing.T) {
	e, ctx, w := montar(t, `{"slides":[{"id":"a","boxes":[]}]}`)
	cambios := 0
	e.AlCambiar = func() { cambios++ }

	pie := e.carril.pie
	test.Tap(pie[0].(*editor.BotonCinta)) // nueva
	if len(e.Modelo().Diapos) != 2 || e.Activa() != 1 {
		t.Fatalf("«Nueva diapositiva» deja %d diapositivas y la activa en %d", len(e.Modelo().Diapos), e.Activa())
	}
	if ctx.Guardado != editor.Sucio || cambios != 1 {
		t.Errorf("tras un cambio el guardado es %v y se ha avisado %d veces", ctx.Guardado, cambios)
	}
	test.Tap(e.carril.pie[1].(*editor.BotonCinta)) // duplicar
	if len(e.Modelo().Diapos) != 3 || e.Activa() != 2 {
		t.Fatalf("«Duplicar» deja %d diapositivas y la activa en %d", len(e.Modelo().Diapos), e.Activa())
	}
	test.Tap(e.carril.pie[2].(*editor.BotonCinta)) // eliminar: pregunta
	capas := w.Canvas().Overlays().List()
	if len(capas) != 1 || len(e.Modelo().Diapos) != 3 {
		t.Fatalf("eliminar tiene que PREGUNTAR antes (%d capas, %d diapositivas)", len(capas), len(e.Modelo().Diapos))
	}
	ui.Descartar(w.Canvas()) // Escape: no se borra nada
	if len(e.Modelo().Diapos) != 3 {
		t.Error("cancelar el borrado ha borrado la diapositiva")
	}
}

// TestElSoloLecturaNoDejaTocarNada: un documento que no se reescribe igual se enseña,
// y ni el carril, ni la mini-barra ni las notas lo cambian.
//
// VISTO EN ROJO (2026-09-22, en el frontal): quitando la comprobación de `cambiar`, el
// botón de nueva diapositiva añadía una a un documento en solo lectura. Deshecho.
func TestElSoloLecturaNoDejaTocarNada(t *testing.T) {
	e, ctx, _ := montar(t, `{"slides":[{"id":"a","boxes":[],"futuro":1}]}`)
	if !ctx.SoloLectura {
		t.Fatal("un campo que esta versión no conoce tiene que abrir en solo lectura")
	}
	test.Tap(e.carril.pie[0].(*editor.BotonCinta))
	e.NuevaDiapositiva()
	e.notas.area.SetText("escrito")
	if len(e.Modelo().Diapos) != 1 || e.Modelo().Diapos[0].Notas != "" || ctx.Guardado != editor.Limpio {
		t.Errorf("en solo lectura el documento ha cambiado: %d diapositivas, notas %q, guardado %v",
			len(e.Modelo().Diapos), e.Modelo().Diapos[0].Notas, ctx.Guardado)
	}
}

// TestLasNotasSeEscribenEnSuDiapositiva: lo tecleado va a las notas de la ACTIVA, y al
// cambiar de diapositiva el panel enseña las suyas.
func TestLasNotasSeEscribenEnSuDiapositiva(t *testing.T) {
	e, _, _ := montar(t, `{"slides":[{"id":"a","boxes":[],"notes":"uno"},{"id":"b","boxes":[]}]}`)
	if e.notas.area.Texto() != "uno" {
		t.Fatalf("el panel enseña %q y las notas de la 1 son «uno»", e.notas.area.Texto())
	}
	e.Elegir(1)
	if e.notas.area.Texto() != "" {
		t.Errorf("en la 2, que no tiene notas, el panel enseña %q", e.notas.area.Texto())
	}
	test.Type(e.notas.area, "dos")
	if got := e.Modelo().Diapos[1].Notas; got != "dos" {
		t.Errorf("lo tecleado en la 2 ha dado notas %q", got)
	}
	if got := e.Modelo().Diapos[0].Notas; got != "uno" {
		t.Errorf("teclear en la 2 ha cambiado las notas de la 1: %q", got)
	}
}

// TestElPanelDeNotasMideLoDeLaWeb: `border-t`, la cabecera en una línea de `text-xs`
// tras `pt-1` y el `<textarea>` de `h-20`: 1 + 4 + 16 + 80 = 101 con rem 16.
func TestElPanelDeNotasMideLoDeLaWeb(t *testing.T) {
	if AltoNotas != 1+4+16+80 {
		t.Errorf("el panel de notas mide %v y la web da 101", AltoNotas)
	}
	if AltoMiniBarra != 36 || AltoPieCarril != 37 {
		t.Errorf("mini-barra %v (h-9 = 36) y pie del carril %v (1 + 2 × 4 + 28 = 37)",
			AltoMiniBarra, AltoPieCarril)
	}
}

// TestElEscenarioCabeEnSuArea: con el zoom al 100 % la diapositiva se encaja en el área
// dejando el aire de FIT_GAP; al 200 % el área se desplaza.
func TestElEscenarioCabeEnSuArea(t *testing.T) {
	e, ctx, _ := montar(t, string(muestra(t)))
	vis := e.desliz.Size()
	esc := e.escenario.Size()
	if esc.Width > vis.Width-HuecoEncaje+1 || esc.Height > vis.Height-HuecoEncaje+1 {
		t.Errorf("al 100 %% el escenario mide %v en un área de %v: no deja FIT_GAP", esc, vis)
	}
	ctx.PonerZoom(200)
	if got := e.escenario.Size(); got.Width < 1.9*esc.Width {
		t.Errorf("al 200 %% el escenario mide %v y al 100 %% medía %v", got, esc)
	}
	if e.desliz.Content.MinSize().Width <= vis.Width {
		t.Error("al 200 % el contenido tiene que desbordar el área para poder desplazarse")
	}
}

// TestLaPresentacionRevelaPorPasos: una caja con entrada no está en el paso 0 y sí en el
// 1; la flecha derecha avanza paso a paso y AvPág salta de diapositiva entera.
func TestLaPresentacionRevelaPorPasos(t *testing.T) {
	m, err := Leer([]byte(`{"slides":[
		{"id":"a","boxes":[{"id":"t","x":0,"y":0,"w":50,"h":20,"html":"<p>t</p>"},
			{"id":"e","x":0,"y":50,"w":50,"h":20,"html":"<p>e</p>","anims":[{"type":"fade","order":1}]}]},
		{"id":"b","boxes":[]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	p := NuevaPresentacion(m, 0, traductor(t))
	w := test.NewTempWindow(t, p)
	w.Resize(fyne.NewSize(960, 540))
	if n := len(p.lienzo.cajasVisibles()); n != 1 {
		t.Fatalf("en el paso 0 se ven %d cajas y la de la entrada no puede estar", n)
	}
	p.Tecla(fyne.KeyRight)
	if p.Diapo != 0 || p.Paso != 1 || len(p.lienzo.cajasVisibles()) != 2 {
		t.Errorf("→ lleva a (%d, %d) con %d cajas; tocaba (0, 1) con las dos", p.Diapo, p.Paso,
			len(p.lienzo.cajasVisibles()))
	}
	p.Tecla(fyne.KeyPageUp)
	p.Tecla(fyne.KeyPageDown)
	if p.Diapo != 1 || p.Paso != 0 {
		t.Errorf("AvPág lleva a (%d, %d) y salta a la siguiente entera: (1, 0)", p.Diapo, p.Paso)
	}
	p.Tecla(fyne.KeyDown)
	if p.Diapo != 1 {
		t.Error("↓ no hace nada en la web, y aquí tampoco")
	}
	salido := false
	p.AlSalir = func() { salido = true }
	p.Tecla(fyne.KeyEscape)
	if !salido {
		t.Error("Escape tiene que salir de la presentación")
	}
	guarda(t, "presentacion.png", w.Canvas().Capture())
}

// TestLasTransicionesSonLasDeLaWeb: «deslizar» entra en 300 ms desde un 4 % a la
// derecha —a la izquierda al retroceder— y desde opacidad 0, que sobre la tinta del
// visor es un velo de tinta; «fundido» solo el velo. Cambiar de paso dentro de la misma
// diapositiva o llegar a una sin transición no anima nada.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin el signo de `dir`, al retroceder la
// diapositiva también entraba desde la derecha (x = 38,4).
func TestLasTransicionesSonLasDeLaWeb(t *testing.T) {
	m, err := Leer([]byte(`{"slides":[{"id":"a","boxes":[]},
		{"id":"b","transition":"slide","boxes":[
			{"id":"e","x":0,"y":0,"w":50,"h":20,"html":"<p>e</p>","anims":[{"type":"fade","order":1}]}]},
		{"id":"c","transition":"fade","boxes":[]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	p := NuevaPresentacion(m, 0, traductor(t))
	w := test.NewTempWindow(t, p)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(960, 540))
	velo := func() uint8 { return p.r.velo.FillColor.(color.NRGBA).A }
	if p.anim != nil {
		t.Error("una diapositiva sin transición no anima")
	}
	p.Avanzar(1)
	if p.anim == nil || p.anim.Duration != DuracionTransicion {
		t.Fatal("llegar a una diapositiva con «deslizar» anima 300 ms")
	}
	p.anim.Tick(0)
	if x := p.lienzo.Position().X; x != 960*DesplazaTransicion || velo() != 255 {
		t.Errorf("al empezar, la diapositiva está en x = %v con el velo a %d; quiero %v y 255",
			x, velo(), 960*DesplazaTransicion)
	}
	p.anim.Tick(0.5)
	if x := p.lienzo.Position().X; x != 960*DesplazaTransicion/2 || velo() != 128 {
		t.Errorf("a la mitad, x = %v y velo %d", x, velo())
	}
	p.anim.Tick(1)
	if x := p.lienzo.Position().X; x != 0 || velo() != 0 {
		t.Errorf("al terminar, x = %v y velo %d; en su sitio y sin velo", x, velo())
	}
	anim := p.anim
	p.Avanzar(1)
	if p.Diapo != 1 || p.Paso != 1 || p.anim != anim {
		t.Error("revelar un paso no vuelve a animar la diapositiva")
	}
	p.Avanzar(1)
	p.anim.Tick(0)
	if x := p.lienzo.Position().X; p.Diapo != 2 || x != 0 || velo() != 255 {
		t.Errorf("«fundido» empieza sin moverse (x = %v) y tapado (velo %d)", x, velo())
	}
	p.Avanzar(-1)
	p.anim.Tick(0)
	if x := p.lienzo.Position().X; x != -960*DesplazaTransicion {
		t.Errorf("al volver a «deslizar», entra desde la izquierda: x = %v", x)
	}
	p.anim.Tick(0.5)
	guarda(t, "transicion.png", w.Canvas().Capture())
}

// TestLosMandosDelVisorSonLosDeLaWeb: `h-11 w-11`, 44 × 44, en la esquina inferior
// derecha con `right-3 bottom-3`, 12 de margen; «anterior» apagado al principio y
// «siguiente» al final; el de notas
// solo si la diapositiva tiene.
func TestLosMandosDelVisorSonLosDeLaWeb(t *testing.T) {
	m, _ := Leer([]byte(`{"slides":[{"id":"a","boxes":[],"notes":"hola"},{"id":"b","boxes":[]}]}`))
	p := NuevaPresentacion(m, 0, traductor(t))
	w := test.NewTempWindow(t, p)
	w.SetPadded(false) // la presentación ocupa la ventana entera, sin el margen del tema
	w.Resize(fyne.NewSize(1000, 600))
	cerrar := p.mandos[3]
	if pos := cerrar.Position(); pos != fyne.NewPos(1000-12-44, 600-12-44) {
		t.Errorf("el botón de salir está en %v", pos)
	}
	if !p.mandos[1].Inerte || p.mandos[2].Inerte {
		t.Error("al principio «anterior» va apagado y «siguiente» no")
	}
	test.Tap(p.mandos[0])
	if !p.Notas {
		t.Error("el botón de notas tiene que abrir la capa de notas")
	}
	test.Tap(p.mandos[2])
	if p.Diapo != 1 || p.Paso != 0 {
		t.Errorf("«siguiente» lleva a (%d, %d)", p.Diapo, p.Paso)
	}
	if !p.mandos[2].Inerte {
		t.Error("en la última, «siguiente» va apagado")
	}
}

// TestLaReferenciaSeVeComoLaWeb deja las capturas de `testdata/referencia.json`, la
// misma presentación que pinta el arnés de la web, diapositiva a diapositiva, para
// compararlas a ojo con las de Playwright (no hay número que diga «se parece»).
func TestLaReferenciaSeVeComoLaWeb(t *testing.T) {
	crudo, err := os.ReadFile("testdata/referencia.json")
	if err != nil {
		t.Fatal(err)
	}
	e, ctx, w := montar(t, string(crudo))
	if ctx.SoloLectura {
		t.Fatal("la referencia se entiende entera: no puede abrirse en solo lectura")
	}
	for i := range e.Modelo().Diapos {
		e.Elegir(i)
		guarda(t, fmt.Sprintf("ref-%d.png", i+1), w.Canvas().Capture())
	}
	w.Resize(fyne.NewSize(360, 740))
	e.Elegir(0)
	guarda(t, "ref-movil.png", w.Canvas().Capture())
}

// TestEnUnTelefonoElCarrilEsUnaTiraArriba: con la ventana por debajo de 640 (`max-sm:`)
// el carril pasa a ser una tira horizontal encima de la zona —la miniatura con su
// `p-2` y el `border-b`: 72 + 16 + 1 = 89 de alto en 16:9—, con el pie a la derecha, y
// la mini-barra deja de centrarse: su primer control empieza en el `px-1` (4) y el
// resto se desliza. Al volver a una ventana ancha, el carril vuelve a la columna.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin `enTira`, a 360 el carril seguía en
// columna de 160 y a la diapositiva le quedaban 120 px de ancho.
func TestEnUnTelefonoElCarrilEsUnaTiraArriba(t *testing.T) {
	e, _, w := montar(t, `{"slides":[{"id":"a","boxes":[]},{"id":"b","boxes":[]},{"id":"c","boxes":[]}]}`)
	w.Resize(fyne.NewSize(360, 740))
	if got := e.carril.Size(); got.Height != 89 || got.Width != e.raiz.Size().Width {
		t.Errorf("la tira mide %v y tiene que ir a lo ancho con 89 de alto", got)
	}
	if e.deslizZona.Position().Y != 89 || e.deslizZona.Position().X != 0 {
		t.Errorf("la zona empieza en %v y va debajo de la tira", e.deslizZona.Position())
	}
	if !e.carril.tira || e.carril.desliz.Direction != container.ScrollHorizontalOnly {
		t.Error("las miniaturas de la tira se deslizan de lado")
	}
	if x := e.miniBarra.Objects[0].Position().X; x != 4 {
		t.Errorf("el primer control de la mini-barra está en x=%v y, si no cabe, va en el px-1 (4)", x)
	}
	guarda(t, "telefono.png", w.Canvas().Capture())

	w.Resize(fyne.NewSize(1280, 800))
	if e.carril.tira || e.carril.Size().Width != AnchoCarrilDiapos {
		t.Errorf("de vuelta en escritorio el carril mide %v y es la columna de 160", e.carril.Size())
	}
}

// TestLaSeleccionSeVeComoLaWeb deja la captura de la estrella de la segunda
// diapositiva de la referencia elegida, para compararla con la de la web.
func TestLaSeleccionSeVeComoLaWeb(t *testing.T) {
	crudo, err := os.ReadFile("testdata/referencia.json")
	if err != nil {
		t.Fatal(err)
	}
	e, _, w := montar(t, string(crudo))
	e.Elegir(1)
	e.SeleccionarSolo("f2")
	guarda(t, "ref-seleccion.png", w.Canvas().Capture())
}

// TestLosPanelesSeVenComoLaWeb deja las capturas de los paneles del gráfico, del
// diagrama y de animaciones sobre la segunda diapositiva de la referencia.
func TestLosPanelesSeVenComoLaWeb(t *testing.T) {
	crudo, err := os.ReadFile("testdata/referencia.json")
	if err != nil {
		t.Fatal(err)
	}
	e, _, w := montar(t, string(crudo))
	e.Elegir(1)
	e.SeleccionarSolo("g1")
	e.alternarAbajo(abajoGrafico)
	guarda(t, "ref-panel-grafico.png", w.Canvas().Capture())
	e.SeleccionarSolo("s1")
	e.alternarAbajo(abajoSmart)
	guarda(t, "ref-panel-smart.png", w.Canvas().Capture())
	e.cambiar(func() { e.m.AñadirAnim(1, "f2", "fade"); e.m.AñadirAnim(1, "s1", "zoom") }, true)
	e.SeleccionarSolo("f2")
	e.alternarAbajo(abajoAnim)
	guarda(t, "ref-panel-anim.png", w.Canvas().Capture())
}

// TestInsertarSeVeComoLaWeb deja la captura de la pestaña «Insertar» y la del menú
// «Organizar» con dos cajas elegidas.
func TestInsertarSeVeComoLaWeb(t *testing.T) {
	crudo, err := os.ReadFile("testdata/referencia.json")
	if err != nil {
		t.Fatal(err)
	}
	e, _, w := montar(t, string(crudo))
	e.Elegir(1)
	for _, p := range buscar[*editor.PestanaCinta](w.Canvas().Content()) {
		if p.Rotulo == "Insertar" {
			test.Tap(p)
		}
	}
	guarda(t, "ref-insertar.png", w.Canvas().Capture())
	for _, p := range buscar[*editor.PestanaCinta](w.Canvas().Content()) {
		if p.Rotulo == "Inicio" {
			test.Tap(p)
		}
	}
	e.SeleccionarSolo("f1")
	e.Alternar("f2")
	for _, b := range buscar[*editor.BotonCinta](w.Canvas().Content()) {
		if b.Icono == iconos.EdOrganizar {
			test.Tap(b)
		}
	}
	guarda(t, "ref-organizar.png", w.Canvas().Capture())
}

// TestEscribirSeVeComoLaWeb deja la captura de la caja del cuerpo de la primera
// diapositiva de la referencia mientras se escribe.
func TestEscribirSeVeComoLaWeb(t *testing.T) {
	crudo, err := os.ReadFile("testdata/referencia.json")
	if err != nil {
		t.Fatal(err)
	}
	e, _, w := montar(t, string(crudo))
	e.EmpezarEscritura("t2")
	guarda(t, "ref-escribir.png", w.Canvas().Capture())
}

func TestLaRotacionDeDiaposConservaElEscenarioYLasNotasConPocoAlto(t *testing.T) {
	defer ui.TactilDePrueba(true)()
	e, _, w := montar(t, `{"slides":[{"id":"a","boxes":[],"notes":"borrador"}]}`)
	for _, tam := range []fyne.Size{{Width: 320, Height: 700}, {Width: 568, Height: 320}, {Width: 844, Height: 390}, {Width: 320, Height: 700}} {
		w.Resize(tam)
		if tam.Width > tam.Height && e.carril.tira {
			t.Fatal("la tira de miniaturas consume el alto del teléfono horizontal")
		}
		if e.desliz.Size().Height < 160 || e.notas.area.Texto() != "borrador" {
			t.Fatalf("a %v: escenario %v, notas %q", tam, e.desliz.Size(), e.notas.area.Texto())
		}
		e.deslizZona.ScrollToBottom()
		d := fyne.CurrentApp().Driver()
		area, viewport := e.notas.area.Objeto(), e.deslizZona
		p, origen := d.AbsolutePositionForObject(area), d.AbsolutePositionForObject(viewport)
		if p.Y < origen.Y-1 || p.Y+area.Size().Height > origen.Y+viewport.Size().Height+1 {
			t.Fatalf("a %v: no se alcanzan las notas: %v/%v en %v/%v", tam, p, area.Size(), origen, viewport.Size())
		}
		e.deslizZona.ScrollToTop()
	}
}
