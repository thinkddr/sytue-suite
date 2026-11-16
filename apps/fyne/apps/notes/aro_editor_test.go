package notes

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// conAro dice si la zona de escribir, enfocada, pinta el aro de `.sytue-focus-ring`:
// sus franjas son rásteres por fuera de la caja, la primera en (-2, -2).
func conAro(t *testing.T, ed *EditorNota) bool {
	t.Helper()
	z := ed.zona
	r := test.WidgetRenderer(z).(*zonaRenderer)
	z.enfocado = true
	r.Layout(fyne.NewSize(300, 100))
	for _, o := range r.objetos {
		if _, ok := o.(*canvas.Raster); ok && o.Position() == fyne.NewPos(-2, -2) {
			return true
		}
	}
	return false
}

// TestLaHojaYLaFilaSeEscribenSinAro: en la hoja del árbol y en la fila de lista o diario
// el editor es la página, y el recuadro de color alrededor del texto la partía; el foco
// ya lo dicen el cursor y la barra abierta. En una tarjeta de columnas el aro se queda,
// porque allí sí delimita qué ficha se edita. En los dos temas, y con su captura.
func TestLaHojaYLaFilaSeEscribenSinAro(t *testing.T) {
	t.Cleanup(func() { tema.Elegir("") })
	for _, nombre := range []string{"claro", "oscuro"} {
		tema.Elegir(nombre)
		for _, c := range []struct {
			vista, nota string
			aro         bool
		}{
			{"tree", "n1", false},
			{"list", "n4", false},
			{"columns", "n1", true},
		} {
			t.Run(c.vista+"-"+nombre, func(t *testing.T) {
				p, w := montarCaptura(t, 1280, 800, c.vista)
				n, ok := p.nota(c.nota)
				if !ok {
					t.Fatalf("no está la nota %s", c.nota)
				}
				p.abrirEditor(n)
				p.Repintar()
				w.Canvas().Capture()
				if p.edicion == nil {
					t.Fatal("no se ha abierto el editor")
				}
				if got := conAro(t, p.edicion.editor); got != c.aro {
					t.Errorf("en %s el cuerpo enfocado pinta aro = %v; tiene que ser %v", c.vista, got, c.aro)
				}
				// La medida de 300 × 100 era solo para mirar el aro: se vuelve a la de verdad.
				z := p.edicion.editor.zona
				test.WidgetRenderer(z).Layout(z.Size())
				capturar(t, w, "aro-"+c.vista+"-"+nombre)
			})
		}
	}
}
