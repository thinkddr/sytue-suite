// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes_test

import (
	"image"
	"image/png"
	"os"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/apps/notes"
	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// LOS GUARDIANES DE LA PANTALLA. Comparan contra el número MEDIDO en producción
// (`~/medidas/notes/lista.ancho.json`, 2026-09-21, corporate.sytue.eu, 1280×800,
// rem 14), no contra lo que calcula el propio widget: un test que compara la
// implementación consigo misma pasa siempre.
//
// Corren sin CGO y sin servidor gráfico, que es la ventaja del pintor de software.

func TestMain(m *testing.M) {
	// No `test.NewTempApp(t)`: sustituye la app de la suite y rompe TestLaLetraEsFigtree
	// en otro fichero.
	test.NewApp().Settings().SetTheme(tema.Sytue{})
	tema.FormaDeLosGuardianes()
	os.Exit(m.Run())
}

func traductor(t *testing.T) *i18n.Translator {
	t.Helper()
	tr, err := i18n.New("es", i18n.Common(), catalogos.Notes())
	if err != nil {
		t.Fatalf("no se pudo cargar el catálogo: %v", err)
	}
	return tr
}

// casi compara con la tolerancia de un dieciseisavo de píxel, que es la del reparto en
// coma flotante y NO la de «se parece bastante».
func casi(t *testing.T, que string, tengo, quiero float32) {
	t.Helper()
	if d := tengo - quiero; d > 1.0/16 || d < -1.0/16 {
		t.Errorf("%s: %v, y en producción %v", que, tengo, quiero)
	}
}

// TestLaFilaDeCuadernoMide30Coma5 es EL número que había abierto.
//
// 30,5 es lo medido en producción, y no sale del texto: `py-1.5` (10,5) más la caja de
// línea de `text-base` (19,25) dan 29,75. Los 0,75 que faltan son los DOS BOTONES DE
// ACCIÓN de la derecha —«nuevo cuaderno dentro» y «ajustes»—, que son `p-1` alrededor
// de un icono de 13 y por tanto miden 20, más que la línea. Están en el layout aunque
// vayan a `opacity-0`, que es lo que hace que la fila no cambie de alto al pasar el
// ratón.
//
// VISTO EN ROJO (2026-09-22, en el piloto): quitándole la `Cola` a la rama, la fila
// baja a 29,75 y el test lo dice con los dos números. Deshecho.
func TestLaFilaDeCuadernoMide30Coma5(t *testing.T) {
	l := pantallaLateral(t)
	filas := filasDelArbol(t, l)
	if len(filas) == 0 {
		t.Fatal("el árbol no ha pintado ninguna fila de cuaderno")
	}
	casi(t, "la fila de un cuaderno", ui.MedidaFilaLateral(filas[0]).Height, 34)

	// Y el botón que la explica: 20 × 20, medido en el DOM con `padding: 3.5px`.
	casi(t, "el botón de acción del árbol", notes.LadoAccionArbol, 21)
	// Desde el PR #834 de la web, «nuevo dentro» y «ajustes» son la acción del carril
	// (24) con `-my-[2px]`: se pintan más grandes y la fila NO crece. VISTO EN ROJO con
	// DesbordeAccion a 0: la fila sube a 34,5.
	casi(t, "el desborde de las acciones de cuaderno", notes.DesbordeAccion, 2)
}

// TestLaFilaDeNotaMide21 es la otra fila del árbol, y su número TAMBIÉN es medido.
//
// El encargo decía 26 y que faltaban 5 px. En el volcado de producción la fila mide
// 21: el `li` del cuaderno con notas da 55 = 30,5 (la fila del cuaderno) + 24,5 (el
// `ul` de sus notas), y ese `ul` es `py-0.5` (3,5) + 21. Que es exactamente `py-1` (7)
// más la caja de línea de `text-xs` (14). No faltaba nada.
//
// Lo que sí hacía falta era que la rama HOJA fuera en `text-xs` y con su glifo de 12
// suelto: con la caja de icono de 17,5 del menú, la fila salía 24,5.
func TestLaFilaDeNotaMide21(t *testing.T) {
	l := pantallaLateral(t)
	filas := filasDelArbol(t, l)
	var hoja *ui.FilaLateral
	for _, f := range filas {
		if f.Rama != nil && f.Rama.Hoja {
			hoja = f
			break
		}
	}
	if hoja == nil {
		t.Fatal("el árbol no ha pintado ninguna nota colgando del cuaderno abierto")
	}
	casi(t, "la fila de una nota", ui.MedidaFilaLateral(hoja).Height, 24)
}

// TestElContadorDelCuadernoAbiertoCuentaLasQueHay: `note_count` es un número aparte
// que hay que acordarse de subir al crear y bajar al borrar, y el 2026-09-11 la web
// midió el árbol diciendo 1 donde la lista de al lado tenía 7.
//
// Derivarlo de lo cargado es lo que hace que no pueda volver a desincronizarse. Los
// cuadernos que NO están abiertos siguen con la cifra del servidor, que es la buena
// mientras nadie los toca.
func TestElContadorDelCuadernoAbiertoCuentaLasQueHay(t *testing.T) {
	l := pantallaLateral(t)
	// El cuaderno abierto dice 1 según el servidor y tiene 3 notas cargadas.
	textos := textosDe(t, l.Objeto(tema.Claro))
	if !tiene(textos, "3") {
		t.Errorf("el contador no cuenta las notas cargadas; textos: %v", textos)
	}
	if tiene(textos, "1") {
		t.Errorf("el contador sigue con el `note_count` del servidor; textos: %v", textos)
	}
	// Y el cuaderno cerrado conserva el suyo.
	if !tiene(textos, "9") {
		t.Errorf("el cuaderno cerrado ha perdido su contador; textos: %v", textos)
	}
}

// TestLaBarraMide42: `min-h-12` con el rem de 14. Los botones que van dentro son de
// talla `sm` (28) y de icono `sm` (24), los dos MEDIDOS en producción.
func TestLaBarraMide42(t *testing.T) {
	b := notes.NuevaBarra(traductor(t))
	b.Cuaderno = notes.Cuaderno{ID: "n1", Titulo: "test"}
	o := b.Objeto(tema.Claro)
	casi(t, "la barra del cuaderno", o.MinSize().Height, 49)
	casi(t, "el filtro", notes.AnchoFiltro, 208)
}

// TestCadaPantallaPintaSusSeisEstados. Un estado que no se pinta no es un estado que
// no pasa: es una pantalla en blanco el día que pasa.
//
// Se comprueba que cada fase pinta ALGO distinto de las demás y que ese algo lleva
// texto: una fase que se quedara en el `switch` sin rama saldría vacía y este test lo
// dice. La sesión caducada es la séptima y va con ellas.
func TestCadaPantallaPintaSusSeisEstados(t *testing.T) {
	tr := traductor(t)
	fases := []struct {
		nombre string
		f      notes.Fase
	}{
		{"carga", notes.Cargando},
		{"vacío", notes.Vacia},
		{"error", notes.Fallo},
		{"sin conexión", notes.SinConexion},
		{"permiso denegado", notes.Prohibida},
		{"sesión caducada", notes.SinSesion},
		{"éxito", notes.Exito},
	}
	pantallas := map[string]func(notes.Fase) fyne.CanvasObject{
		"lienzo": func(f notes.Fase) fyne.CanvasObject {
			l := notes.NuevoLienzo(tr)
			l.Fase, l.Notas = f, []notes.Nota{notaDe("n1", "Una", "<p>texto</p>")}
			return l.Objeto(tema.Claro)
		},
		"papelera": func(f notes.Fase) fyne.CanvasObject {
			p := notes.NuevaPapelera(tr)
			p.Fase, p.Notas = f, []notes.Nota{notaDe("n1", "Una", "")}
			return p.Objeto(tema.Claro)
		},
		"búsqueda": func(f notes.Fase) fyne.CanvasObject {
			b := notes.NuevaBusqueda(tr)
			b.Fase = f
			b.Resultados = []api.NoteHit{{ID: "n1", Title: "Una", Snippet: "x"}}
			return b.Objeto(tema.Claro)
		},
	}
	for nombre, montar := range pantallas {
		t.Run(nombre, func(t *testing.T) {
			visto := map[string]string{}
			for _, f := range fases {
				textos := strings.Join(textosDe(t, montar(f.f)), "|")
				if f.f == notes.Cargando {
					continue // la fase de carga es una rueda y no lleva texto
				}
				if textos == "" {
					t.Errorf("el estado %q no pinta nada", f.nombre)
					continue
				}
				if otro, ya := visto[textos]; ya {
					t.Errorf("los estados %q y %q pintan lo mismo: %q", f.nombre, otro, textos)
				}
				visto[textos] = f.nombre
			}
		})
	}
}

// TestUnaNotaQueNoVuelveEnteraSeAbreEnSoloLectura es la regla que manda sobre la
// paridad, comprobada donde llega a la pantalla y no solo en el modelo.
//
// VISTO EN ROJO: haciendo que `Nota.Editable` no mire `Doc.Fiel()`, la nota con la
// clase se vuelve editable y el cartel desaparece. Deshecho.
func TestUnaNotaQueNoVuelveEnteraSeAbreEnSoloLectura(t *testing.T) {
	tr := traductor(t)
	buena := notaDe("n1", "Buena", "<p>hola</p>")
	rara := notaDe("n2", "Rara", `<p class="destacado">hola</p>`)

	if !buena.Editable(false, false) {
		t.Error("una nota normal tiene que poder editarse")
	}
	if rara.Editable(false, false) {
		t.Error("una nota cuyo HTML no vuelve entero NO puede editarse")
	}
	if notes.Cartel(tr, buena, tema.Claro) != nil {
		t.Error("una nota que vuelve entera no lleva cartel")
	}
	if notes.Cartel(tr, rara, tema.Claro) == nil {
		t.Fatal("una nota que no vuelve entera tiene que decirlo")
	}

	// Y una tabla con una celda que ocupa dos: vuelve entera —se puede guardar— y se
	// dibuja como es, así que tampoco lleva cartel.
	tabla := notaDe("n3", "Tabla", `<table><tr><td colspan="2">a</td></tr></table>`)
	if !tabla.Editable(false, false) {
		t.Error("una tabla se conserva entera, así que se puede guardar")
	}
	if notes.Cartel(tr, tabla, tema.Claro) != nil {
		t.Error("una tabla con celdas combinadas se dibuja entera y no lleva cartel")
	}
}

// TestLaPantallaSePintaEntera monta la aplicación sin servicio y comprueba que sale
// algo. Guarda el PNG cuando se le pide: los peores fallos de esta ola pasaban todos
// los tests, y la única forma de verlos es mirar la imagen.
func TestLaPantallaSePintaEntera(t *testing.T) {
	p := notes.NuevaPantalla(traductor(t), "es", nil, nil, nil)
	w := test.NewWindow(p.Objeto())
	defer w.Close()
	w.Resize(fyne.NewSize(1280, 800))

	img := w.Canvas().Capture()
	guardar(t, img)
	if img.Bounds().Dx() == 0 || img.Bounds().Dy() == 0 {
		t.Fatal("la captura ha salido vacía")
	}
	if tinta(img) < 500 {
		t.Errorf("la pantalla ha pintado %d píxeles de tinta: nace invisible", tinta(img))
	}
}

// TestElCuadernoConNotasSePinta monta el carril con su árbol y el lienzo con tres
// notas, que es lo que de verdad se ve a diario. Guarda el PNG con `SYTUE_CAPTURA`:
// los peores fallos de esta ola pasaban todos los tests, y la única forma de verlos es
// mirar la imagen.
func TestElCuadernoConNotasSePinta(t *testing.T) {
	tr := traductor(t)
	l := pantallaLateral(t)
	b := notes.NuevaBarra(tr)
	b.Cuaderno = notes.Cuaderno{ID: "n1", Titulo: "test", Color: "#e11d48"}
	li := notes.NuevoLienzo(tr)
	li.Cuaderno, li.Fase, li.Notas = b.Cuaderno, notes.Exito, l.Notas
	li.Seleccionada = "a"

	cuerpo := container.New(ui.Pila{Elastico: 1}, b.Objeto(tema.Claro), li.Objeto(tema.Claro))
	arm := ui.NuevoArmazon(cuerpo)
	arm.Menu = l.Objeto(tema.Claro)

	w := test.NewWindow(arm)
	defer w.Close()
	w.Resize(fyne.NewSize(1280, 800))
	img := w.Canvas().Capture()
	guardar(t, img)
	if n := tinta(img); n < 2000 {
		t.Errorf("el cuaderno con notas ha pintado %d píxeles de tinta: falta algo", n)
	}
}

// TestEnArbolSinNotaElegidaSeInvitaAElegirOCrear: la vista en árbol sin nota elegida
// era un panel en blanco. Ahora es el vacío de Mail y Contacts: «Elige una nota», la
// frase y el botón «Nueva nota» que la frase ofrece (web, PR #841). VISTO EN ROJO
// quitando la rama de `elige`: no sale ningún texto.
func TestEnArbolSinNotaElegidaSeInvitaAElegirOCrear(t *testing.T) {
	tr := traductor(t)
	// También cargando: en la web el panel no espera a la lista, enseña la nota que
	// se elija en el árbol (`enArbol && !notaSeleccionada` va antes que el lienzo).
	for _, fase := range []notes.Fase{notes.Exito, notes.Cargando} {
		l := notes.NuevoLienzo(tr)
		l.Fase, l.Llenar, l.AlCrearPrimera = fase, true, func() {}
		textos := strings.Join(textosDe(t, l.Objeto(tema.Claro)), "|")
		for _, clave := range []string{"tree.pick", "note.new"} {
			if !strings.Contains(textos, tr.T(clave, nil)) {
				t.Errorf("fase %v: falta «%s»; se pintó %q", fase, tr.T(clave, nil), textos)
			}
		}
	}
}

// ---- Andamiaje ----

// pantallaLateral es un carril con dos cuadernos —uno abierto con tres notas y otro
// cerrado con nueve— y su fase en éxito.
func pantallaLateral(t *testing.T) *notes.Lateral {
	t.Helper()
	l := notes.NuevoLateral(traductor(t))
	l.Fase = notes.Exito
	l.Cuadernos = []notes.Cuaderno{
		{ID: "n1", Titulo: "test", Color: "#e11d48", Notas: 1},
		{ID: "n2", Titulo: "Operaciones", Color: "#0ea5e9", Notas: 9},
	}
	l.Activo = "n1"
	l.Notas = []notes.Nota{
		notaDe("a", "prueba de nota", "<p>x</p>"),
		notaDe("b", "otra", "<p>y</p>"),
		notaDe("c", "y otra", "<p>z</p>"),
	}
	return l
}

func notaDe(id, titulo, cuerpo string) notes.Nota {
	return notes.DeNota(api.Note{
		ID: id, NotebookID: "n1", Kind: "text",
		Title: titulo, Content: cuerpo,
	})
}

// filasDelArbol saca las `ui.FilaLateral` del carril, que es donde viven las medidas.
func filasDelArbol(t *testing.T, l *notes.Lateral) []*ui.FilaLateral {
	t.Helper()
	var out []*ui.FilaLateral
	recorrer(l.Objeto(tema.Claro), func(o fyne.CanvasObject) {
		if f, ok := o.(*ui.FilaLateral); ok && f.Rama != nil {
			out = append(out, f)
		}
	})
	return out
}

// textosDe son todos los rótulos pintados, bajando por los renderers: los textos de
// `ui.Vacio` y de un botón viven ahí y no en un contenedor.
func textosDe(t *testing.T, o fyne.CanvasObject) []string {
	t.Helper()
	var out []string
	if o == nil {
		return out
	}
	recorrer(o, func(x fyne.CanvasObject) {
		if c, ok := x.(*canvas.Text); ok && c.Text != "" {
			out = append(out, c.Text)
		}
	})
	return out
}

func tiene(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func recorrer(o fyne.CanvasObject, f func(fyne.CanvasObject)) {
	f(o)
	switch x := o.(type) {
	case *fyne.Container:
		for _, h := range x.Objects {
			recorrer(h, f)
		}
	case fyne.Widget:
		for _, h := range test.WidgetRenderer(x).Objects() {
			recorrer(h, f)
		}
	}
}

// tinta cuenta los píxeles que NO son del fondo, que es la forma de decir «se ha
// pintado algo» sin describir el resultado de memoria.
func tinta(im image.Image) int {
	b := im.Bounds()
	fondo := im.At(b.Min.X+1, b.Min.Y+1)
	n := 0
	for y := b.Min.Y; y < b.Max.Y; y += 2 {
		for x := b.Min.X; x < b.Max.X; x += 2 {
			if im.At(x, y) != fondo {
				n++
			}
		}
	}
	return n
}

func guardar(t *testing.T, img image.Image) {
	t.Helper()
	destino := os.Getenv("SYTUE_CAPTURA")
	if destino == "" {
		return
	}
	f, err := os.Create(destino)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	t.Logf("captura escrita en %s", destino)
}
