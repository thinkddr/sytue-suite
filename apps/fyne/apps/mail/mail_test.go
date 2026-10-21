// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"image/color"
	"image/png"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

func TestMain(m *testing.M) {
	test.NewApp().Settings().SetTheme(tema.Sytue{})
	tema.FormaDeLosGuardianes()
	os.Exit(m.Run())
}

// Contratos CSS actuales con raíz16: fila = 2×py-2 +22+18+gap-0.5+24+border-b;
// panel20rem menos1 de borde; SectionHeader min-h-12; búsqueda Input36+py-2.5+1.
// Texto x=260+border-l-2+px-3+16+gap-1+8+gap-1+40+gap-3=358.
// Lateral260 menos32 de px-4 y1 de borde; fila py-2+22=38.
// Los literales vienen de las clases web, sin consultar los medidores nativos.
const (
	filaMedida       = 83  // li de conversación
	panelMedido      = 319 // la `section` de dentro del panel de 320
	cabeceraMedida   = 48  // SectionHeader (`min-h-12`)
	busquedaMedida   = 57  // la franja del buscador, con su filete
	xTextoMedido     = 358 // dónde empieza el remitente, contando desde x = 260
	yLista           = 161 // dónde empieza la lista, dentro de la ventana
	filaLateral      = 38  // una carpeta del menú
	anchoUtilLateral = 228 // lo que le queda a una fila dentro del carril de 260, que ya no lleva filete
)

// hijos baja un nivel en el árbol de pintado. Fyne no expone un recorrido, así que
// son los dos casos que hay: un contenedor tiene `Objects` y un widget los de su
// renderer.
func hijos(o fyne.CanvasObject) []fyne.CanvasObject {
	switch x := o.(type) {
	case *fyne.Container:
		return x.Objects
	case fyne.Widget:
		return test.WidgetRenderer(x).Objects()
	}
	return nil
}

func traductor(t *testing.T) *i18n.Translator {
	t.Helper()
	tr, err := Traductor("es")
	if err != nil {
		t.Fatal(err)
	}
	return tr
}

func ejemplo() []Conversacion {
	base := time.Date(2026, 9, 11, 9, 30, 0, 0, time.UTC)
	return AgruparPorHilo([]Mensaje{
		{
			ID: "1", HiloID: "h1", Asunto: "Calendario de despliegues",
			Extracto: "Os paso las ventanas de mantenimiento de octubre y noviembre.",
			De:       []Direccion{{Nombre: "Sofía Nogueira Paz", Correo: "sofia@sytue.eu"}},
			Recibido: base, Leido: true,
		},
		{
			ID: "2", HiloID: "h2", Asunto: "Encuesta de satisfacción del trimestre",
			Extracto: "Resultados: 4,6 sobre 5 en soporte y 4,2 en formación.",
			De:       []Direccion{{Nombre: "Ousmane Diallo Bâ", Correo: "ousmane@sytue.eu"}},
			Recibido: base.Add(-time.Hour),
		},
	})
}

// reloj congela la hora para que las fechas relativas no dependan de cuándo corra el
// guardián: con `time.Now` el mismo test da «11 sept» hoy y «11 sept 2026» en enero.
func reloj() func() time.Time {
	return func() time.Time { return time.Date(2026, 9, 11, 18, 0, 0, 0, time.UTC) }
}

// TestLaFilaMideLoQueLaWeb es el guardián de la fila: la medida DECLARADA y la que
// pide Fyne, contra los 75,75 del DOM de producción.
func TestLaFilaMideLoQueLaWeb(t *testing.T) {
	if got := AltoFilaConversacion(); got != filaMedida {
		t.Errorf("AltoFilaConversacion = %v, y en producción la fila mide %v", got, filaMedida)
	}
	f := NuevaFilaConversacion()
	f.Poner(ejemplo()[0], "Sofía Nogueira Paz", "Calendario de despliegues", "11 sept")
	if got := f.MinSize().Height; got != filaMedida {
		t.Errorf("el MinSize de la fila da %v y la medida declarada %v: hay dos cuentas", got, filaMedida)
	}
	// Y el bloque de texto es el que manda, no el avatar: si algún día el avatar
	// creciera, la fila tiene que seguir midiendo lo que mide su texto.
	if AltoTextoFila <= ui.MedidaAvatar(ui.AvatarMD).Height {
		t.Errorf("el bloque de texto (%v) ya no es más alto que el avatar (%v): el 75,75 sale de otro sitio",
			AltoTextoFila, ui.MedidaAvatar(ui.AvatarMD).Height)
	}
}

// TestLaFilaPintaDondeLaWeb compara los PÍXELES: el avatar, la columna de texto y las
// tres líneas caen donde el inspector las midió.
func TestLaFilaPintaDondeLaWeb(t *testing.T) {
	tr := traductor(t)
	b := NuevaBandeja(tr, "es")
	b.Ahora = reloj()
	b.Titulo = "Recibidos"
	b.Estado = Listo
	b.Conversaciones = ejemplo()

	w := test.NewWindow(b)
	t.Cleanup(w.Close)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(ui.AnchoPanelBase, 744))
	// El lienzo tiene que estar pintado para que el `widget.List` monte sus filas.
	w.Canvas().Capture()

	f := primeraFila(t, b)
	// La fila se coloca DENTRO de la lista, que empieza bajo cabecera y búsqueda.
	yEsperado := float32(cabeceraMedida + busquedaMedida)
	if got := AltoCabeceraLista + AltoBusqueda; got != yEsperado {
		t.Errorf("la lista empieza en %v y en producción en %v (%v de la ventana)",
			got, yEsperado, yLista)
	}
	if got := f.Size().Height; got != filaMedida {
		t.Errorf("la fila pintada mide %v y la web %v", got, filaMedida)
	}
	if got := f.Size().Width; got != panelMedido {
		t.Errorf("la fila pintada mide %v de ancho y la sección de la web %v", got, panelMedido)
	}
	r := test.WidgetRenderer(f).(*filaRenderer)
	// Casilla de 16 + gap de 4 + punto de 8 + gap de 4: avatar en 46, texto en 98.
	// El control conserva su tamaño; solo se estrechan los huecos entre objetos.
	if got := r.avatar.Position().X; got != 46 {
		t.Errorf("el avatar empieza en %v y las clases de la web dan 46", got)
	}
	if got := r.nombre.Position().X; got != 98 {
		t.Errorf("el remitente empieza en %v y las clases de la web dan 98", got)
	}
}

// primeraFila saca del árbol la primera fila que el `widget.List` ha montado.
func primeraFila(t *testing.T, o fyne.CanvasObject) *FilaConversacion {
	t.Helper()
	var encontrada *FilaConversacion
	var mirar func(fyne.CanvasObject)
	mirar = func(x fyne.CanvasObject) {
		if encontrada != nil {
			return
		}
		if f, ok := x.(*FilaConversacion); ok {
			encontrada = f
			return
		}
		for _, h := range hijos(x) {
			mirar(h)
		}
	}
	mirar(o)
	if encontrada == nil {
		t.Fatal("el widget.List no ha montado ninguna fila: la lista no está pintando")
	}
	return encontrada
}

// TestLaListaNoMeteHuecoEntreFilas es la trampa cara de `widget.List`: separa las
// filas por el relleno del TEMA (7 px en el de la suite) y también con
// `HideSeparators`. Siete por fila son 35.000 en una bandeja de cinco mil.
//
// Se comprueba sobre lo que pinta la bandeja de verdad, no sobre una lista de
// juguete: el fallo es olvidarse del envoltorio, y solo se ve donde se olvida.
func TestLaListaNoMeteHuecoEntreFilas(t *testing.T) {
	tr := traductor(t)
	b := NuevaBandeja(tr, "es")
	b.Ahora = reloj()
	b.Estado = Listo
	b.Conversaciones = ejemplo()

	w := test.NewWindow(b)
	t.Cleanup(w.Close)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(ui.AnchoPanelBase, 744))
	w.Canvas().Capture()

	var ys []float32
	var mirar func(fyne.CanvasObject, fyne.Position)
	mirar = func(x fyne.CanvasObject, off fyne.Position) {
		if f, ok := x.(*FilaConversacion); ok {
			ys = append(ys, off.Y+f.Position().Y)
			return
		}
		for _, h := range hijos(x) {
			mirar(h, off.Add(x.Position()))
		}
	}
	mirar(b, fyne.NewPos(0, 0))
	if len(ys) < 2 {
		t.Fatalf("solo se han montado %d filas: no se puede medir la separación", len(ys))
	}
	d := ys[1] - ys[0]
	if d < 0 {
		d = -d
	}
	if d != filaMedida {
		t.Errorf("entre dos filas hay %v y la fila mide %v: la lista está metiendo %v de relleno "+
			"del tema (¿falta `ui.EnvolverLista`?)", d, filaMedida, d-filaMedida)
	}
}

// TestLaBandejaTieneSusSeisEstados: los seis pintan algo distinto, y ninguno pinta la
// lista. Confundir «no hay correos» con «no he podido preguntar» es lo que hace que
// alguien deje de buscar un correo que sí existe.
func TestLaBandejaTieneSusSeisEstados(t *testing.T) {
	tr := traductor(t)
	vistos := map[string]Estado{}
	for _, e := range []Estado{Cargando, Listo, Vacio, Error, SinConexion, SinPermiso} {
		b := NuevaBandeja(tr, "es")
		b.Ahora = reloj()
		b.Titulo = "Recibidos"
		b.Estado = e
		if e == Listo {
			b.Conversaciones = ejemplo()
		}
		w := test.NewWindow(b)
		w.SetPadded(false)
		w.Resize(fyne.NewSize(ui.AnchoPanelBase, 744))
		w.Canvas().Capture()
		firma := huella(t, b)
		if otro, ya := vistos[firma]; ya {
			t.Errorf("el estado %d pinta lo mismo que el %d: uno de los dos no existe", e, otro)
		}
		vistos[firma] = e
		w.Close()
	}
}

// huella es el texto que la pantalla enseña, que es lo que distingue un estado de
// otro para quien la mira.
func huella(t *testing.T, o fyne.CanvasObject) string {
	t.Helper()
	var sb strings.Builder
	var mirar func(fyne.CanvasObject)
	mirar = func(x fyne.CanvasObject) {
		if !x.Visible() {
			return
		}
		if a, ok := x.(fyne.Accessible); ok {
			if s := a.AccessibilityLabel(); s != "" {
				sb.WriteString(s + "|")
			}
		}
		for _, h := range hijos(x) {
			mirar(h)
		}
	}
	mirar(o)
	return sb.String()
}

// TestElMenuMideLoQueLaWeb: el carril y sus filas, contra el DOM.
func TestElMenuMideLoQueLaWeb(t *testing.T) {
	tr := traductor(t)
	m := &Menu{
		T: tr, Locale: "es", EstadoCarpetas: Listo,
		Carpetas: []Carpeta{
			{ID: "in", Nombre: "Inbox", Rol: "inbox", SinLeer: 17},
			{ID: "dr", Nombre: "Drafts", Rol: "drafts"},
		},
		CarpetaID:       "in",
		CuotaPorcentaje: "0 %", CuotaUso: "48,1 KB",
		AlElegirCarpeta: func(Carpeta) {},
	}
	l := m.Construir()
	if got := ui.MedidaLateral(l).Width; got != tema.AnchoLateral {
		t.Errorf("el carril mide %v y la web %v", got, tema.AnchoLateral)
	}
	if got := ui.AnchoUtilMenu; got != anchoUtilLateral {
		t.Errorf("el ancho útil es %v y se midió %v", got, anchoUtilLateral)
	}
	f := m.filaCarpeta(m.Carpetas[0], tema.Claro)
	if got := ui.MedidaFilaLateral(f).Height; got != filaLateral {
		t.Errorf("una carpeta mide %v de alto y la web %v", got, filaLateral)
	}
	if !f.Activa {
		t.Error("la carpeta seleccionada no sale activa: no se vería dónde está uno")
	}
	if f.Contador != "17" {
		t.Errorf("el contador de no leídos es %q y tenía que ser 17", f.Contador)
	}
}

// TestElLectorVacioEsElMedido: el estado que producción enseñaba, con su título, su
// descripción y su botón.
func TestElLectorVacioEsElMedido(t *testing.T) {
	tr := traductor(t)
	x := NuevoLector(tr, "es")
	x.Estado = Vacio
	x.AlRedactar = func() {}
	w := test.NewWindow(x)
	t.Cleanup(w.Close)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(740, 744))
	w.Canvas().Capture()

	h := huella(t, x)
	for _, quiero := range []string{
		tr.T("reader.emptyTitle", nil),
		tr.T("nav.compose", nil),
	} {
		if !strings.Contains(h, quiero) {
			t.Errorf("el lector vacío no dice %q; dice %q", quiero, h)
		}
	}
}

// TestElColorDeUnaEtiquetaEsElDeLaWeb ata la paleta de etiquetas a `utils.ts`, que es
// la que tiene que coincidir con `domain.LabelColors` del backend. Ocho colores que
// no son tokens del tema: si alguien los reescribe «a juego», dejan de casar con los
// del servidor y la misma etiqueta sale de dos colores.
func TestElColorDeUnaEtiquetaEsElDeLaWeb(t *testing.T) {
	fuente := webref.Read(t, "apps/frontend/apps/mail/src/components/utils.ts")
	i := strings.Index(fuente, "LABEL_COLORS: Record<string, string> = {")
	if i < 0 {
		t.Fatal("utils.ts ya no declara LABEL_COLORS: el espejo hay que rehacerlo")
	}
	j := strings.Index(fuente[i:], "};")
	re := regexp.MustCompile(`(\w+):\s*'#([0-9a-fA-F]{6})'`)
	web := map[string]color.NRGBA{}
	for _, m := range re.FindAllStringSubmatch(fuente[i:i+j], -1) {
		var r, g, b uint8
		for k, dst := range []*uint8{&r, &g, &b} {
			dst2 := dst
			*dst2 = uint8(hex(m[2][2*k])<<4 | hex(m[2][2*k+1]))
		}
		web[m[1]] = color.NRGBA{R: r, G: g, B: b, A: 255}
	}
	if len(web) != len(coloresEtiqueta) {
		t.Errorf("la web declara %d colores de etiqueta y aquí hay %d", len(web), len(coloresEtiqueta))
	}
	for k, quiero := range web {
		if got := coloresEtiqueta[k]; got != quiero {
			t.Errorf("la etiqueta %q es %v en la web y %v aquí", k, quiero, got)
		}
	}
}

func hex(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	}
	return int(c-'A') + 10
}

// TestAgruparPorHiloEsGroupByThread: el orden lo fija el mensaje más reciente, un
// mensaje sin hilo es un hilo de uno, y el total es el del hilo ENTERO.
func TestAgruparPorHiloEsGroupByThread(t *testing.T) {
	c := AgruparPorHilo([]Mensaje{
		{ID: "a", HiloID: "h1", EnHilo: 5},
		{ID: "b", HiloID: "h2", Destacado: true},
		{ID: "c", HiloID: "h1", ConAdjunto: true},
		{ID: "d"}, // sin hilo
	})
	if len(c) != 3 {
		t.Fatalf("salen %d conversaciones y tenían que ser 3", len(c))
	}
	if c[0].HiloID != "h1" || len(c[0].Mensajes) != 2 {
		t.Errorf("el primer hilo es %q con %d mensajes", c[0].HiloID, len(c[0].Mensajes))
	}
	if c[0].Total != 5 {
		t.Errorf("el total del hilo es %d y el servidor dijo 5: la fila diría «2» para un hilo de cinco", c[0].Total)
	}
	if !c[0].ConAdjunto {
		t.Error("el hilo no dice que trae adjunto y uno de sus mensajes sí")
	}
	if !c[1].Destacada {
		t.Error("el hilo no sale destacado y su mensaje sí")
	}
	if c[2].HiloID != "msg:d" {
		t.Errorf("un mensaje sin hilo es %q y tenía que ser msg:d: si no, todos caen bajo la clave vacía", c[2].HiloID)
	}
	if !c[0].SinLeer {
		t.Error("un mensaje sin leer tiene que dejar el hilo sin leer")
	}
}

// TestLaCapturaSeGuarda deja el PNG de la pantalla entera cuando se pide, que es
// como se mira al lado de la captura de la web.
func TestLaCapturaSeGuarda(t *testing.T) {
	destino := os.Getenv("SYTUE_CAPTURA")
	if destino == "" {
		t.Skip("sin SYTUE_CAPTURA: el guardián de píxeles ya corrió arriba")
	}
	tr := traductor(t)
	p := NuevaPantalla(tr, "es")
	p.Reloj(reloj())
	p.Menu.EstadoCarpetas = Listo
	p.Menu.Carpetas = []Carpeta{
		{ID: "in", Nombre: "Inbox", Rol: "inbox", SinLeer: 17},
		{ID: "dr", Nombre: "Drafts", Rol: "drafts"},
		{ID: "se", Nombre: "Sent", Rol: "sent"},
		{ID: "ar", Nombre: "Archive", Rol: "archive"},
		{ID: "ju", Nombre: "Junk", Rol: "junk"},
		{ID: "tr", Nombre: "Trash", Rol: "trash"},
	}
	p.Menu.CarpetaID = "in"
	p.Menu.EstadoCuota, p.Menu.CuotaPorcentaje, p.Menu.CuotaUso = Listo, "0 %", "48,1 KB"
	p.Menu.AlElegirCarpeta = func(Carpeta) {}
	p.Bandeja.Titulo = "Recibidos"
	p.Bandeja.Estado = Listo
	p.Bandeja.Conversaciones = ejemplo()
	p.Refrescar()

	w := test.NewWindow(p.Objeto())
	t.Cleanup(w.Close)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(1280, 744))
	img := w.Canvas().Capture()
	f, err := os.Create(destino)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	t.Logf("captura en %s", destino)
}
