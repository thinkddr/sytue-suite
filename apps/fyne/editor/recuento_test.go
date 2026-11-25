// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"encoding/json"
	"os"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Los vectores son los de la web: el mismo texto da las mismas cifras en las dos.
const rutaVectoresRecuento = "../../frontend/packages/office/test/vectores/docus-recuento.json"

// TestElRecuentoCuentaComoLaWeb recorre los vectores compartidos con
// `packages/office/test/docs-recuento.test.tsx`.
// VISTO EN ROJO (2026-11-12): con `unicode.IsSpace` en vez del `\s` de JavaScript, «a
// U+0085 b» eran dos palabras y la marca de orden de bytes contaba como letra.
func TestElRecuentoCuentaComoLaWeb(t *testing.T) {
	datos, err := os.ReadFile(rutaVectoresRecuento)
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Casos []struct {
			Nombre   string `json:"nombre"`
			Texto    string `json:"texto"`
			Esperado struct {
				Palabras              int `json:"palabras"`
				Caracteres            int `json:"caracteres"`
				CaracteresSinEspacios int `json:"caracteresSinEspacios"` //nolint:tagliatelle // el JSON es el de la web
				Parrafos              int `json:"parrafos"`
			} `json:"esperado"`
		} `json:"casos"`
	}
	if err := json.Unmarshal(datos, &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Casos) < 10 {
		t.Fatalf("solo %d casos: el fichero de vectores no se ha leído bien", len(v.Casos))
	}
	for _, c := range v.Casos {
		e := c.Esperado
		quiero := Recuento{Palabras: e.Palabras, Caracteres: e.Caracteres, CaracteresSinEspacios: e.CaracteresSinEspacios, Parrafos: e.Parrafos}
		if got := RecuentoDelTexto(c.Texto); got != quiero {
			t.Errorf("%s: %+v, quiero %+v", c.Nombre, got, quiero)
		}
	}
}

// lienzoConSeleccion es un [Lienzo] de Docus con 340 palabras y 12 seleccionadas.
type lienzoConSeleccion struct {
	lienzoDePrueba
	sel *Recuento
}

func (l *lienzoConSeleccion) Estadisticas() (int, int, int) { return 2, 3, 340 }

func (l *lienzoConSeleccion) Recuento() (Recuento, *Recuento) {
	return Recuento{Palabras: 340, Caracteres: 2100, CaracteresSinEspacios: 1760, Parrafos: 14}, l.sel
}

func (l *lienzoConSeleccion) PalabrasSeleccionadas() (int, bool) {
	if l.sel == nil {
		return 0, false
	}
	return l.sel.Palabras, true
}

func contextoDeRecuento(t *testing.T, tipo Tipo, l Lienzo) *Contexto {
	t.Helper()
	tr, err := i18n.New("es", i18n.Common(), catalogos.Docs())
	if err != nil {
		t.Fatal(err)
	}
	return &Contexto{Tipo: tipo, Zoom: 100, Estado: Listo, T: tr.T, Loc: "es", Lienzo: l}
}

// TestLaBarraDiceLasPalabrasDeLaSeleccion: con selección, «Palabras: 12 de 340», como
// StatusBar.tsx; sin ella, las del documento. Y la cifra es un botón.
func TestLaBarraDiceLasPalabrasDeLaSeleccion(t *testing.T) {
	defer ui.TactilDePrueba(false)()
	l := &lienzoConSeleccion{}
	ctx := contextoDeRecuento(t, Documento, l)
	b := NuevaBarraEstado(ctx)
	w := test.NewTempWindow(t, b)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(800, AltoEstado))
	if got := b.palabras.Texto; got != "Palabras: 340" {
		t.Fatalf("sin selección la barra dice %q", got)
	}
	l.sel = &Recuento{Palabras: 12}
	b.Refresh()
	if got := b.palabras.Texto; got != "Palabras: 12 de 340" {
		t.Fatalf("con selección la barra dice %q y quiero «Palabras: 12 de 340»", got)
	}
	if b.palabras.AccessibilityRole() != fyne.AccessibleRoleButton || b.palabras.AccessibilityLabel() != b.palabras.Texto {
		t.Fatalf("la cifra tiene que ser un botón que se llama como lo que se lee")
	}
	if !b.palabras.Visible() || b.palabras.Size().Width <= 0 {
		t.Fatal("la cifra de palabras no se coloca")
	}
}

// TestElRecuentoSeAbreConLaCifraYConElAtajo: pulsar la cifra o Ctrl+Mayús+C abre el
// diálogo; en Diapos ni hay cifra ni el atajo es suyo.
func TestElRecuentoSeAbreConLaCifraYConElAtajo(t *testing.T) {
	defer ui.TactilDePrueba(false)()
	ctx := contextoDeRecuento(t, Documento, &lienzoConSeleccion{sel: &Recuento{Palabras: 12}})
	b := NuevaBarraEstado(ctx)
	w := test.NewTempWindow(t, b)
	w.Resize(fyne.NewSize(800, 600))
	abierto := func() bool {
		d := dialogoArriba(w.Canvas())
		return d != nil && d.Titulo == "Recuento de palabras"
	}
	test.Tap(b.palabras)
	if !abierto() {
		t.Fatal("pulsar la cifra no abre el recuento")
	}
	dialogoArriba(w.Canvas()).Cerrar()
	if abierto() {
		t.Fatal("el recuento no se cierra")
	}
	atajo := &desktop.CustomShortcut{KeyName: fyne.KeyC, Modifier: fyne.KeyModifierShortcutDefault | fyne.KeyModifierShift}
	if !ctx.AtajoDelEditor(atajo) || !abierto() {
		t.Fatal("Ctrl+Mayús+C no abre el recuento")
	}
	sinMayus := &desktop.CustomShortcut{KeyName: fyne.KeyC, Modifier: fyne.KeyModifierShortcutDefault}
	if ctx.AtajoDelEditor(sinMayus) {
		t.Fatal("Ctrl+C es copiar, no el recuento")
	}

	diapos := contextoDeRecuento(t, Presentacion, &lienzoConSeleccion{})
	bd := NuevaBarraEstado(diapos)
	wd := test.NewTempWindow(t, bd)
	wd.Resize(fyne.NewSize(800, AltoEstado))
	if bd.palabras.Visible() || diapos.AbrirRecuento != nil || diapos.AtajoDelEditor(atajo) {
		t.Fatal("en Diapos no hay recuento de palabras")
	}
}

// TestElDialogoDelRecuentoEnseñaLasDosColumnas: con selección, una columna para ella
// antes de la del documento; las páginas de la selección se quedan en blanco.
func TestElDialogoDelRecuentoEnseñaLasDosColumnas(t *testing.T) {
	ctx := contextoDeRecuento(t, Documento, nil)
	doc := Recuento{Palabras: 1340, Caracteres: 8100, CaracteresSinEspacios: 6900, Parrafos: 41}
	tabla := CuerpoRecuento(ctx, 3, doc, &Recuento{Palabras: 12, Caracteres: 70, CaracteresSinEspacios: 59, Parrafos: 2}).(*fyne.Container).Objects[0].(*ui.Tabla)
	rotulos := make([]string, 0, len(tabla.Columnas))
	for _, c := range tabla.Columnas {
		rotulos = append(rotulos, c.Rotulo)
	}
	if len(rotulos) != 3 || rotulos[1] != "Selección" || rotulos[2] != "Documento" {
		t.Fatalf("columnas %q", rotulos)
	}
	if n := tabla.Filas(); n != 6 {
		t.Fatalf("%d filas y son seis cifras", n)
	}
	// 1340 palabras a 200 por minuto son 7 minutos (hacia arriba); las 12 de la selección, 1.
	if got := [3]string{tabla.Celda(5, 0), tabla.Celda(5, 1), tabla.Celda(5, 2)}; got != [3]string{"Tiempo de lectura (minutos)", "1", "7"} {
		t.Errorf("tiempo de lectura: %q", got)
	}
	fila := func(i int) [3]string { return [3]string{tabla.Celda(i, 0), tabla.Celda(i, 1), tabla.Celda(i, 2)} }
	if got := fila(0); got != [3]string{"Páginas", "", "3"} {
		t.Errorf("páginas: %q", got)
	}
	if got := fila(1); got != [3]string{"Palabras", "12", "1340"} && got != [3]string{"Palabras", "12", "1.340"} {
		t.Errorf("palabras: %q", got)
	}
	if got := fila(3); got[0] != "Caracteres sin espacios" || got[1] != "59" {
		t.Errorf("caracteres sin espacios: %q", got)
	}

	sin := CuerpoRecuento(ctx, 3, doc, nil).(*fyne.Container).Objects[0].(*ui.Tabla)
	if len(sin.Columnas) != 2 {
		t.Fatalf("sin selección sobran columnas: %d", len(sin.Columnas))
	}
}

// lienzoConIdioma es un lienzo de Docus que guarda el idioma del documento.
type lienzoConIdioma struct {
	lienzoConSeleccion
	idioma   string
	editable bool
	puestos  []string
}

func (l *lienzoConIdioma) IdiomaDocumento() (string, bool) { return l.idioma, l.editable }

func (l *lienzoConIdioma) PonerIdiomaDocumento(idioma string) {
	l.idioma = idioma
	l.puestos = append(l.puestos, idioma)
}

// TestLaBarraEligeElIdiomaDelDocumento: el idioma de la barra es el DEL DOCUMENTO, como el
// `<select>` de StatusBar.tsx: sin el suyo, el de la interfaz; la lista son los 26, cada
// uno con su nombre en su idioma; elegir uno lo guarda; en solo lectura no se cambia.
func TestLaBarraEligeElIdiomaDelDocumento(t *testing.T) {
	defer ui.TactilDePrueba(false)()
	l := &lienzoConIdioma{editable: true}
	ctx := contextoDeRecuento(t, Documento, l)
	b := NuevaBarraEstado(ctx)
	w := test.NewTempWindow(t, b)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(800, 600))
	b.Refresh()
	if got := b.idioma.Texto; got != "Español" {
		t.Fatalf("sin idioma propio la barra dice %q", got)
	}
	if !b.idioma.Visible() || b.idioma.Position().X <= b.palabras.Position().X {
		t.Fatal("el idioma no va detrás de las palabras")
	}
	if b.idioma.AccessibilityRole() != fyne.AccessibleRoleButton || b.idioma.AccessibilityLabel() != "Idioma del documento" {
		t.Fatalf("el idioma se anuncia como %v %q", b.idioma.AccessibilityRole(), b.idioma.AccessibilityLabel())
	}
	test.Tap(b.idioma)
	if b.menuIdioma == nil || !b.menuIdioma.Abierto() {
		t.Fatal("pulsar el idioma no abre la lista")
	}
	var elegir *ui.ElementoMenu
	marcados := 0
	elems := 0
	for _, o := range b.menuIdioma.Elementos {
		e, ok := o.(*ui.ElementoMenu)
		if !ok {
			continue
		}
		elems++
		if e.Marcada {
			marcados++
		}
		if e.Texto == "Deutsch" {
			elegir = e
		}
	}
	if elems != 26 || marcados != 1 || elegir == nil {
		t.Fatalf("la lista tiene %d idiomas, %d marcados y Deutsch %v", elems, marcados, elegir != nil)
	}
	test.Tap(elegir)
	if len(l.puestos) != 1 || l.puestos[0] != "de" || b.idioma.Texto != "Deutsch" {
		t.Fatalf("elegir Deutsch guarda %v y la barra dice %q", l.puestos, b.idioma.Texto)
	}

	if b.menuIdioma.Abierto() {
		b.menuIdioma.Cerrar()
	}
	l.editable = false
	b.Refresh()
	b.abrirIdiomas()
	if !b.idioma.Inerte || b.menuIdioma.Abierto() {
		t.Fatal("en solo lectura el idioma se puede cambiar")
	}

	// Por debajo de `sm` se va, como el `max-sm:hidden` de la web.
	w.Resize(fyne.NewSize(390, 600))
	b.Refresh()
	if b.idioma.Visible() {
		t.Error("en un móvil el idioma sigue en la barra")
	}

	diapos := NuevaBarraEstado(contextoDeRecuento(t, Presentacion, &lienzoConIdioma{editable: true}))
	test.NewTempWindow(t, diapos).Resize(fyne.NewSize(800, 600))
	diapos.Refresh()
	if diapos.idioma.Visible() {
		t.Error("en Diapos no hay idioma de documento")
	}
}
