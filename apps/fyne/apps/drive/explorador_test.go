// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// cercaDe es `casi` con la tolerancia que pide un ANCHO DE TEXTO medido en el piloto: su
// Chromium redondea el avance de cada glifo a un píxel entero y el error se acumula por
// letra (REGLAS §3). Exigir un dieciseisavo a un rótulo es un guardián que falla por el
// navegador, no por el port.
func cercaDe(t *testing.T, que string, tengo, quiero, tol float32) {
	t.Helper()
	if d := tengo - quiero; d > tol || d < -tol {
		t.Errorf("%s: %v, y en producción %v (±%v)", que, tengo, quiero, tol)
	}
}

// TestSubirLlevaSuFlechaYCaeDondeProduccion: «Subir» es un `Button` con `leadingIcon` y
// mide 71 en producción; sin la flecha salía de 50 y arrastraba todo el grupo. Y el
// conmutador se separa 10,5 (`gap-2` + `ml-1`), no 3,5: «Subir» empieza en 1119,5.
//
// VISTO EN ROJO (2026-09-22, en el frontal): quitando el `HuecoAcciones` de la
// separación del conmutador, «Subir» cae en 1126,5.
func TestSubirLlevaSuFlechaYCaeDondeProduccion(t *testing.T) {
	e := NuevoExplorador(traductor(t))
	e.Ventana = 1280
	cab := e.cabecera(tema.Claro)
	cab.Resize(fyne.NewSize(1280-tema.AnchoLateral, AltoCabeceraVista))
	lay, _ := cab.(*fyne.Container).Layout.(*CabeceraVista)
	subir, ok := lay.Acciones[2].(*ui.Boton)
	if !ok || subir.Glifo == nil {
		t.Fatal("«Subir» no lleva su glifo de Drive")
	}
	cercaDe(t, "el ancho de «Subir»", subir.Size().Width, 78.9375, 0.75)
	cercaDe(t, "«Subir» empieza en x=1119,5", tema.AnchoLateral+subir.Position().X, 1099.0625, 0.75)
	casi(t, "el glifo de «Subir» es el `size={16}`", subir.TamIcono, iconos.TallaSM)
}

// TestElBuscadorYLaListaSobrevivenAlRepintado: la pantalla se rehace con cada cambio de
// estado, y con un buscador nuevo en cada vuelta el foco se perdía en la primera letra;
// con una lista nueva, marcar una casilla devolvía la lista arriba.
//
// VISTO EN ROJO (2026-09-22, en el frontal): creando el campo en cada llamada a
// `campoBusqueda`, el test dice que el buscador ha cambiado.
func TestElBuscadorYLaListaSobrevivenAlRepintado(t *testing.T) {
	e := NuevoExplorador(traductor(t))
	e.Fase, e.Ventana = Exito, 1280
	e.Elementos = elementosDePrueba(time.Date(2026, 9, 6, 9, 0, 0, 0, time.UTC))
	e.Objeto(tema.Claro)
	buscador, lista := e.buscador, e.filas
	if buscador == nil || lista == nil {
		t.Fatal("el explorador no ha montado buscador ni lista")
	}
	e.Seleccion["Contratos"] = true
	e.Objeto(tema.Claro)
	if e.buscador != buscador {
		t.Error("el buscador ha cambiado al repintar: el foco se pierde en cada letra")
	}
	if e.filas != lista {
		t.Error("la lista ha cambiado al repintar: vuelve arriba al marcar una casilla")
	}
}

// TestLaListaPideLaPaginaSiguienteCercaDelFinal: el `onScroll` de la web pide la página
// siguiente a 600 px del final, y UNA vez hasta que llega.
//
// VISTO EN ROJO (2026-09-22, en el frontal): comparando con `len-1` en vez de con
// `len-FilasAntesDelFinal`, la fila 18 de 30 no pide nada.
func TestLaListaPideLaPaginaSiguienteCercaDelFinal(t *testing.T) {
	e := NuevoExplorador(traductor(t))
	e.Fase, e.HayMas = Exito, true
	for i := range 30 {
		e.Elementos = append(e.Elementos, Elemento{ID: fmt.Sprint(i), Nombre: fmt.Sprint(i), Tamano: 1})
	}
	pedidas := 0
	e.AlCargarMas = func() { pedidas++; e.CargandoMas = true }
	f := ui.NuevaFilaDrive("", "")
	e.rellenarFila(30-FilasAntesDelFinal-1, f)
	if pedidas != 0 {
		t.Fatalf("a %d filas del final ya se ha pedido la página siguiente", FilasAntesDelFinal+1)
	}
	e.rellenarFila(30-FilasAntesDelFinal, f)
	if pedidas != 1 {
		t.Fatalf("a %d filas del final no se ha pedido la página siguiente", FilasAntesDelFinal)
	}
	e.rellenarFila(29, f)
	if pedidas != 1 {
		t.Errorf("se ha pedido %d veces; tiene que ser una, y no otra hasta que llegue", pedidas)
	}
	e.CargandoMas, e.HayMas = false, false
	e.rellenarFila(29, f)
	if pedidas != 1 {
		t.Error("sin más páginas se sigue pidiendo")
	}
}

// TestLaRejillaMideSusFilasAlAnchoDeVerdad: `fyne.Layout.MinSize` no recibe el ancho, y
// la rejilla contaba las filas con la tarjeta a su ancho de reserva. El desplazable
// creía el contenido más corto y las últimas filas no se alcanzaban.
func TestLaRejillaMideSusFilasAlAnchoDeVerdad(t *testing.T) {
	ancho := float32(1020 - 2*CanalonRejilla)
	r := &rejillaDrive{rej: ui.RejillaTarjetas(1280, HuecoRejilla, CortesRejillaDrive), ancho: &ancho}
	objs := make([]fyne.CanvasObject, 12)
	for i := range objs {
		objs[i] = NuevaTarjetaDrive("x", "pdf", false)
	}
	col := r.rej.AnchoColumna(ancho)
	alto := MedidaTarjetaDrive(col).Height
	// Cinco columnas a 1280 (`xl:grid-cols-5`): doce tarjetas son tres filas.
	casi(t, "el alto de la rejilla", r.MinSize(objs).Height, 3*alto+2*HuecoRejilla)
	r.Layout(objs, fyne.NewSize(ancho, 0))
	casi(t, "la tarjeta 6 empieza la segunda fila", objs[5].Position().Y, alto+HuecoRejilla)
	casi(t, "y mide la columna", objs[5].Size().Width, col)
}

func TestCorteDe(t *testing.T) {
	for _, c := range []struct {
		ancho float32
		corte int
	}{{390, 0}, {640, 1}, {767, 1}, {768, 2}, {1024, 3}, {1279, 3}, {1280, 4}, {1920, 4}} {
		if got := CorteDe(c.ancho); got != c.corte {
			t.Errorf("a %v de ventana el corte es %d, y sale %d", c.ancho, c.corte, got)
		}
	}
}

// TestNombreZipEsElDelServicio: la descarga se guarda con el nombre que calcula el
// cliente, así que tiene que ser el que `zipDownloadName` da en services/drive.
func TestNombreZipEsElDelServicio(t *testing.T) {
	for _, c := range []struct{ base, quiero string }{
		{"", "sytue-drive.zip"},
		{"  Contratos ", "Contratos.zip"},
		{"informe.zip", "informe.zip"},
	} {
		if got := NombreZip(c.base); got != c.quiero {
			t.Errorf("NombreZip(%q) = %q, quiero %q", c.base, got, c.quiero)
		}
	}
	if got := NombreZip(string(make([]byte, 200))); len(got) != 124 {
		t.Errorf("la base se recorta a 120 bytes: %d", len(got))
	}
}

// TestElResumenDeLaSubidaEsElDeLaWeb: si algo falló es un PELIGRO, y si fue la cuota se
// dice, porque la salida es liberar espacio y no reintentar.
func TestElResumenDeLaSubidaEsElDeLaWeb(t *testing.T) {
	tr := traductor(t)
	tono, titulo := resumenSubida(tr, 3, 3, nil)
	if tono != ui.AvisoExito || titulo != tr.T("files.upload.done", i18n.Vars{"count": 3}) {
		t.Errorf("todo subido: %v %q", tono, titulo)
	}
	tono, titulo = resumenSubida(tr, 1, 3, &api.Error{Status: 507})
	if tono != ui.AvisoPeligro || titulo != tr.T("files.upload.quota", i18n.Vars{"uploaded": 1, "count": 3}) {
		t.Errorf("cuota: %v %q", tono, titulo)
	}
	tono, titulo = resumenSubida(tr, 0, 2, errors.New("red"))
	if tono != ui.AvisoPeligro || titulo != tr.T("files.upload.partial", i18n.Vars{"uploaded": 0, "count": 2, "failed": 2}) {
		t.Errorf("fallo: %v %q", tono, titulo)
	}
}

// TestSinSesionNoSePideNada: sin nadie dentro se enseña cómo entrar, sin gastar una
// petición en un 401 seguro. El servicio es nil: si se pidiera algo, reventaría.
func TestSinSesionNoSePideNada(t *testing.T) {
	p := NuevaPantalla(traductor(t), nil, nil, nil)
	p.Arrancar(false)
	if p.explor.Fase != SinSesion || p.papelera.Fase != SinSesion {
		t.Errorf("sin sesión la fase es %d / %d", p.explor.Fase, p.papelera.Fase)
	}
	p.Sesion(false) // sin cambio: no hace nada
}

// TestLaFilaEstrechaYLaDeBusquedaLlevanSusLineas: por debajo de `sm` las columnas se
// esconden y el tamaño y la fecha pasan bajo el nombre; en una búsqueda va además la
// carpeta donde vive. Con las dos líneas la fila crece a 65,75 (19,25 + 2 × 15,75 +
// `py-2` + el filete) y VirtualList la coloca cada 66, medido en la web a 360; con una,
// manda la tesela y se queda en 51.
func TestLaFilaEstrechaYLaDeBusquedaLlevanSusLineas(t *testing.T) {
	casi(t, "sin líneas", ui.AltoFilaDriveCon(0), 53)
	casi(t, "una línea", ui.AltoFilaDriveCon(1), 57)
	casi(t, "dos líneas", ui.AltoFilaDriveCon(2), 75)
	e := NuevoExplorador(traductor(t))
	e.Fase, e.Ventana, e.EnBusqueda = Exito, 360, true
	e.Elementos = []Elemento{{
		ID: "c1", Nombre: "Contratos", Carpeta: true, Tamano: -1, Ruta: "/Legal/Contratos",
		Modificado: time.Date(2026, 9, 6, 9, 0, 0, 0, time.UTC),
	}}
	obj := e.Objeto(tema.Claro)
	w := test.NewWindow(obj)
	defer w.Close()
	w.Resize(fyne.NewSize(360, 400))
	if !contieneTexto(obj, "Legal") || !contieneTexto(obj, "Carpeta · 06 sept 2026") {
		t.Error("la fila estrecha de una búsqueda no dice dónde vive ni su fecha")
	}
	if e.filasLineas != 2 {
		t.Errorf("la lista se creó con %d líneas", e.filasLineas)
	}
	var filas int
	recorrer(obj, func(o fyne.CanvasObject) {
		if f, ok := o.(*ui.FilaDrive); ok && f.Nombre == "Contratos" {
			filas++
			casi(t, "la fila misma", f.MinSize().Height, ui.AltoFilaDriveCon(2))
		}
	})
	if filas == 0 {
		t.Error("no hay fila de «Contratos» en la lista")
	}
}

// TestPorDebajoDe768LaCabeceraVaEnDosFilas: es la rejilla de la web
// (`max-md:grid max-md:grid-cols-[minmax(0,1fr)_auto_auto]` en FileBrowser.tsx): las migas
// y el conmutador de vista arriba; el buscador, que crece, «Nuevo» y «Subir» abajo. Las
// posiciones son las medidas en la web a 360, en español, sobre su captura (2026-10-05).
//
// Antes envolvía como podía y a 360 daba tres filas; y antes aún se quedaba en una, se
// salía por la izquierda y tapaba «Mi unidad» (visto en una ventana real el 2026-09-27).
// VISTO EN ROJO sin el ramo de la rejilla en `Layout`: a 360 «Subir» baja a una tercera
// fila (y = 95) y a 700 todo se queda en una, con 55 de alto.
func TestPorDebajoDe768LaCabeceraVaEnDosFilas(t *testing.T) {
	web := webref.Read(t, "apps/frontend/apps/drive/src/components/FileBrowser.tsx")
	for _, c := range []struct{ trozo, porque string }{
		{`'bg-surface text-text border-border flex flex-wrap items-center gap-x-3 gap-y-2 border-b px-4 py-2.5 sm:px-6',`, "la cabecera y sus huecos"},
		{`'max-md:grid max-md:grid-cols-[minmax(0,1fr)_auto_auto] max-md:gap-x-2',`, "por debajo de 768, tres columnas: la primera crece"},
		{`class="flex min-w-0 grow items-center gap-0.5 text-base max-md:col-span-2"`, "las migas, en las dos primeras"},
		{`class="ml-auto flex flex-wrap items-center justify-end gap-2 max-md:contents"`, "el grupo de acciones no pinta caja: sus hijos son celdas"},
		{`max-md:col-start-3 max-md:row-start-1 max-md:justify-self-end md:ml-1`, "el conmutador sube a la primera fila, al final"},
	} {
		if !strings.Contains(web, c.trozo) {
			t.Errorf("FileBrowser.tsx ya no dice %q (%s): la cabecera de aquí ha dejado de ser la suya", c.trozo, c.porque)
		}
	}

	e := NuevoExplorador(traductor(t))
	e.Ventana = 360
	cab := e.cabecera(tema.Claro)
	lay := cab.(*fyne.Container).Layout.(*CabeceraVista)
	cab.Resize(fyne.NewSize(360, cab.MinSize().Height))
	casi(t, "el alto de la cabecera en dos filas", cab.MinSize().Height, 95)
	canalon := CanalonCabecera(360)
	if x := lay.Ruta.Position().X; x != canalon || lay.Ruta.Size().Width < lay.Ruta.MinSize().Width {
		t.Errorf("la ruta está en x=%g con %g de ancho; quiero x=%g y entera", x, lay.Ruta.Size().Width, canalon)
	}
	buscador, nuevo, subir := lay.Acciones[0], lay.Acciones[1], lay.Acciones[2]
	for _, m := range []struct {
		que            string
		o              fyne.CanvasObject
		x0, x1, y, alt float32
	}{
		{"el conmutador de vista", lay.Conmutador, 278, 344, 10, 34},
		{"el buscador", buscador, 16, 164, 52, 32},
		{"«Nuevo»", nuevo, 172, 257, 52, 32},
		{"«Subir»", subir, 265, 344, 52, 32},
	} {
		p, s := m.o.Position(), m.o.Size()
		cercaDe(t, m.que+", donde empieza", p.X, m.x0, 1.5)
		cercaDe(t, m.que+", donde acaba", p.X+s.Width, m.x1, 1.5)
		cercaDe(t, m.que+", su fila", p.Y, m.y, 0.01)
		cercaDe(t, m.que+", su alto", s.Height, m.alt, 0.01)
	}

	// Entre 640 y 767 sigue en dos filas, con el canalón de `sm:px-6`; desde 768, una.
	e.Ventana = 700
	cab = e.cabecera(tema.Claro)
	cab.Resize(fyne.NewSize(700, cab.MinSize().Height))
	lay = cab.(*fyne.Container).Layout.(*CabeceraVista)
	casi(t, "el alto de la cabecera a 700", cab.MinSize().Height, 95)
	casi(t, "el buscador a 700, desde el canalón", lay.Acciones[0].Position().X, 24)
	casi(t, "«Subir» a 700, hasta el canalón", lay.Acciones[2].Position().X+lay.Acciones[2].Size().Width, 700-24)
	e.Ventana = 1280
	cab = e.cabecera(tema.Claro)
	cab.Resize(fyne.NewSize(1280-tema.AnchoLateral, cab.MinSize().Height))
	casi(t, "el alto de la cabecera en ancho", cab.MinSize().Height, AltoCabeceraVista)
}

// Con margen de área segura en un teléfono compacto quedan 272–256 px útiles de hoja;
// la rejilla no puede consumir todo el campo con su icono y relleno.
func TestLaRotacionDelBuscadorDeDriveConCarrilYAreaSeguraConservaLaConsulta(t *testing.T) {
	defer ui.TactilDePrueba(true)()
	forma := "carril"
	if tema.EsMarco() {
		forma = "marco"
	}
	t.Cleanup(func() { tema.ElegirForma(forma) })
	tema.ElegirForma("marco")
	e := NuevoExplorador(traductor(t))
	e.Ventana = 320
	nuevos, subidas := 0, 0
	e.AlNuevo = func(fyne.CanvasObject) { nuevos++ }
	e.AlSubir = func() { subidas++ }
	vista := e.Objeto(tema.Claro).(*fyne.Container)
	cab := vista.Objects[0].(*fyne.Container)
	lay := cab.Layout.(*CabeceraVista)
	arm := ui.NuevoArmazon(vista)
	arm.Menu = ui.NuevoLateral("Drive", nil)
	w := test.NewTempWindow(t, arm)
	w.SetPadded(false)
	e.buscador.PonerTexto("acta")
	for _, ancho := range []float32{272, 256, 360, 320, 272} {
		w.Resize(fyne.NewSize(ancho, 700))
		buscador := e.buscador
		minimo := ui.MedidaEntrada(ui.CampoCromo, 0, true).Width
		if buscador.Size().Width < minimo || buscador.Texto() != "acta" {
			t.Fatalf("ventana útil %g: buscador=%v, consulta=%q", ancho, buscador.Size(), buscador.Texto())
		}
		if ancho <= 272 && buscador.Position().Y+buscador.Size().Height > lay.Acciones[1].Position().Y {
			t.Fatal("Nuevo invade el buscador cuando este necesita una fila completa")
		}
		for _, o := range lay.Acciones[1:] {
			pos, tam := o.Position(), o.Size()
			if pos.X < 0 || pos.Y < 0 || pos.X+tam.Width > cab.Size().Width || pos.Y+tam.Height > cab.Size().Height {
				t.Fatalf("un mando queda fuera de la cabecera: %v/%v en %v", pos, tam, cab.Size())
			}
			test.Tap(o.(fyne.Tappable))
		}
	}
	if nuevos != 5 || subidas != 5 {
		t.Fatal("Nuevo o Subir dejó de responder tras cambiar el área útil")
	}
}
