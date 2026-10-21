// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// hojaVentas es un origen pequeño con importes en euros.
const hojaVentas = `{"active":0,"sheets":[{"name":"Ventas","cells":{"A1":"Zona","B1":"Importe","A2":"Norte","B2":"100",` +
	`"A3":"Sur","B3":"50","A4":"Norte","B4":"25"},"formats":{"B2":{"id":"currency"},"B3":{"id":"currency"},"B4":{"id":"currency"}}}]}`

// TestLaDinamicaSeEscribeComoEnLaWeb: el comando `pivot` escribe la tabla con la negrita
// y los formatos de la web, guarda la definición con su rango, se actualiza, se quita, y
// el libro sigue siendo editable y sale igual al guardarlo.
func TestLaDinamicaSeEscribeComoEnLaWeb(t *testing.T) {
	lz := NuevoLienzo(NuevoLibro())
	lz.T = nil
	if !lz.Cargar(hojaVentas) {
		t.Fatal("no se entiende el origen")
	}
	lz.Mandar(CmdDinamica, `{"source":"A1:B4","anchor":"D1","rows":[{"field":0}],"cols":[],`+
		`"values":[{"field":1,"fn":"sum","fmt":{"id":"currency"}},{"field":1,"fn":"sum","show":"pctGrand"}]}`)
	l := lz.Libro
	ds := l.Dinamicas()
	if len(ds) != 1 || ds[0].Name != "pivot.namePrefix1" || ds[0].Range != "D1:F4" {
		t.Fatalf("la tabla guardada es %+v", ds)
	}
	for _, c := range []struct {
		ref  Celda
		html string
		fmt  string
	}{
		{Celda{3, 0}, "<b>Zona</b>", ""},
		{Celda{3, 1}, "Norte", ""},
		{Celda{4, 1}, "125", "currency"},
		{Celda{5, 1}, "0.7142857142857143", "percent"}, // 125 / 175, con los decimales de FormatNumber
		{Celda{3, 3}, "<b>sheet.total.grand</b>", ""},
		{Celda{4, 3}, "<b>175</b>", "currency"},
	} {
		// El porcentaje sale con las 15 cifras de FormatNumber.
		if got := l.HTML(c.ref.C, c.ref.F); got != c.html && (c.fmt != "percent" || got != "0.714285714285714") {
			t.Errorf("%s es %q y no %q", Ref(c.ref.C, c.ref.F), got, c.html)
		}
		if got := l.FormatoNumero(c.ref.C, c.ref.F).ID; c.fmt != "" && got != c.fmt {
			t.Errorf("%s lleva el formato %q y no %q", Ref(c.ref.C, c.ref.F), got, c.fmt)
		}
	}
	// Actualizar recoge el cambio del origen; encoger no deja filas viejas.
	l.CargarEn(0, 1, 3, "5")
	lz.Mandar(CmdDinamicaAct, "")
	if got := l.HTML(4, 3); got != "<b>155</b>" {
		t.Errorf("tras actualizar, el total es %q", got)
	}
	lz.Mandar(CmdDinamica, `{"name":"pivot.namePrefix1","source":"A1:B4","anchor":"D1","rows":[],"cols":[],"values":[{"field":1,"fn":"count"}]}`)
	if l.HTML(3, 3) != "" || l.Dinamicas()[0].Range != "D1:E2" {
		t.Errorf("al encoger quedan %q en D4 y el rango es %s", l.HTML(3, 3), l.Dinamicas()[0].Range)
	}
	// Se guarda y se vuelve a leer igual, y el libro no se cierra.
	otro := NuevoLienzo(NuevoLibro())
	if !otro.Cargar(lz.Serializar()) {
		t.Fatal("un libro con tabla dinámica se abre en solo lectura")
	}
	var a, b any
	_ = json.Unmarshal([]byte(lz.Serializar()), &a)
	_ = json.Unmarshal([]byte(otro.Serializar()), &b)
	if !reflect.DeepEqual(a, b) {
		t.Errorf("la tabla no sale igual que entra:\n%s\n%s", lz.Serializar(), otro.Serializar())
	}
	lz.Mandar(CmdDinamicaQuitarla, "pivot.namePrefix1")
	if l.Dinamicas() != nil || l.HTML(3, 0) != "" {
		t.Error("quitar la tabla deja algo")
	}
	// Una que no cabe no toca nada.
	lz.Mandar(CmdDinamica, `{"source":"A1:B4","anchor":"Z99","rows":[{"field":0}],"cols":[],"values":[{"field":1,"fn":"sum"}]}`)
	if l.Dinamicas() != nil || l.HTML(25, 98) != "" {
		t.Error("una tabla que no cabe se escribe igual")
	}
	// La de antes (la de las macros grabadas) sigue entendiéndose.
	lz.Mandar(CmdDinamica, `{"source":"A1:B4","target":"D1","rowField":0,"valueField":1,"agg":"sum"}`)
	if l.HTML(4, 1) != "125" && l.HTML(4, 1) != "105" {
		t.Errorf("el pivot antiguo escribe %q", l.HTML(4, 1))
	}
}

// TestElDialogoDeLaDinamicaCrea: «Tabla dinámica» abre el diálogo con la selección de
// origen; añadir Zona a Filas e Importe a Valores enciende «Crear», y crear escribe la
// tabla. Con `SYTUE_CAPTURA` deja el diálogo a ancho de escritorio y de móvil.
func TestElDialogoDeLaDinamicaCrea(t *testing.T) {
	for _, tam := range []fyne.Size{fyne.NewSize(1280, 800), fyne.NewSize(360, 740)} {
		lz := NuevoLienzo(NuevoLibro())
		lz.T = nil
		if !lz.Cargar(hojaVentas) {
			t.Fatal("no se entiende el origen")
		}
		lz.Rejilla.sel, lz.Rejilla.ext = Celda{}, &Celda{C: 1, F: 3}
		ctx := &editor.Contexto{Tipo: editor.Hoja, Lienzo: lz}
		cuerpo := lz.cuerpoDatos(ctx)
		w := test.NewTempWindow(t, container.NewBorder(cuerpo, nil, nil, nil, lz))
		w.Resize(tam)
		boton := botonDeCinta(cuerpo, "data.pivot")
		if boton == nil {
			t.Fatal("Datos no tiene «Tabla dinámica»")
		}
		boton.Tapped(nil)
		dialogo := func() fyne.CanvasObject { return w.Canvas().Overlays().Top() }
		if dialogo() == nil {
			t.Fatal("el botón no abre el diálogo")
		}
		anadir := func() []*ui.Selector {
			var out []*ui.Selector
			for _, o := range todos(dialogo()) {
				if s, ok := o.(*ui.Selector); ok && len(s.Opciones) > 0 && s.Opciones[0] == "pivot.addField" {
					out = append(out, s)
				}
			}
			return out
		}
		if len(anadir()) != 4 {
			t.Fatalf("el diálogo tiene %d zonas", len(anadir()))
		}
		anadir()[2].AlElegir(1) // Filas: Zona
		anadir()[3].AlElegir(2) // Valores: Importe
		var crear *ui.Boton
		for _, o := range todos(dialogo()) {
			if b, ok := o.(*ui.Boton); ok && b.Texto == "data.pivot.create" {
				crear = b
			}
		}
		if crear == nil || crear.Inerte {
			t.Fatal("con campos puestos, «Crear» sigue apagado")
		}
		if tam.Width == 1280 {
			guarda(t, w.Canvas().Capture())
		} else if d := os.Getenv("SYTUE_CAPTURA"); d != "" {
			t.Setenv("SYTUE_CAPTURA", strings.TrimSuffix(d, ".png")+"-360.png")
			guarda(t, w.Canvas().Capture())
		}
		crear.Tapped(nil)
		if len(lz.Libro.Dinamicas()) != 1 || lz.Libro.HTML(4, 1) != "125" {
			t.Errorf("crear no escribe la tabla: %v, E2 = %q", lz.Libro.Dinamicas(), lz.Libro.HTML(4, 1))
		}
	}
}

// hojaSegmentada es un origen con un campo (Producto) que no va en la tabla.
const hojaSegmentada = `{"active":0,"sheets":[{"name":"Ventas","cells":{"A1":"Zona","B1":"Producto","C1":"Importe",` +
	`"A2":"Norte","B2":"Sillas","C2":"150","A3":"Norte","B3":"Mesas","C3":"300",` +
	`"A4":"Sur","B4":"Mesas","C4":"600","A5":"Sur","B5":"Sillas","C5":"200"}}]}`

// TestLaSegmentacionFiltraLaDinamica: la segmentación de una tabla dinámica admite
// cualquier campo del origen (como Excel); el de fuera de la tabla filtra con su filtro
// oculto, que se va con la tarjeta, y el de dentro comparte los ocultos del campo.
func TestLaSegmentacionFiltraLaDinamica(t *testing.T) {
	lz := lienzoDePrueba()
	lz.T = nil
	if !lz.Cargar(hojaSegmentada) {
		t.Fatal("no se entiende el origen")
	}
	l := lz.Libro
	lz.Mandar(CmdDinamica, `{"source":"A1:C5","anchor":"E1","rows":[{"field":0}],"cols":[],"values":[{"field":2,"fn":"sum"}]}`)
	nombre := l.Dinamicas()[0].Name
	total := func() string { return l.HTML(5, 3) }
	if total() != "<b>1250</b>" {
		t.Fatalf("el total de partida es %q", total())
	}
	lz.Mandar(CmdSegAnadir, `{"pivot":"`+nombre+`","col":1}`)
	lz.Mandar(CmdSegAnadir, `{"pivot":"`+nombre+`","col":1}`)
	lz.Mandar(CmdSegAnadir, `{"pivot":"`+nombre+`","col":9}`) // fuera del origen
	segs := l.Segmentaciones()
	if len(segs) != 1 || segs[0].Pivot != nombre || segs[0].Col != 1 || l.Autofiltro() != nil {
		t.Fatalf("las segmentaciones son %+v (y el autofiltro %+v)", segs, l.Autofiltro())
	}
	s := segs[0]
	activa, campo, entradas := l.estadoDinamica(s, lz.t)
	if !activa || campo != "Producto" || len(entradas) != 2 || entradas[0].Valor != "Mesas" || entradas[0].Oculto {
		t.Fatalf("la tarjeta enseña %v %q %+v", activa, campo, entradas)
	}
	lz.segmentarDinamica(s, map[string]bool{"Sillas": true})
	d := l.Dinamicas()[0]
	if total() != "<b>350</b>" || len(d.HiddenFilters) != 1 || d.HiddenFilters[0].Field != 1 ||
		!reflect.DeepEqual(d.HiddenFilters[0].Hidden, []string{"Mesas"}) || len(d.Rows[0].Hidden) != 0 {
		t.Fatalf("con solo Sillas el total es %q y la tabla %+v", total(), d)
	}
	// Se guarda y se vuelve a leer igual: la tarjeta y el filtro oculto.
	otro := lienzoDePrueba()
	if !otro.Cargar(lz.Serializar()) || !igual(t, lz.Serializar(), otro.Serializar()) {
		t.Error("la segmentación de la tabla dinámica no sale igual que entra")
	}
	// Quitar la tarjeta se lleva el filtro oculto, en un solo paso de deshacer.
	if q, _ := l.QuitarSegmentacion(s.ID, lz.t); !q || len(l.Segmentaciones()) != 0 || l.Dinamicas()[0].HiddenFilters != nil || total() != "<b>1250</b>" {
		t.Fatalf("tras quitarla quedan %+v y el total es %q", l.Segmentaciones(), total())
	}
	l.Deshacer()
	if len(l.Segmentaciones()) != 1 || len(l.Dinamicas()[0].HiddenFilters) != 1 || total() != "<b>350</b>" {
		t.Errorf("deshacer no devuelve la tarjeta con su filtro: %+v, %q", l.Segmentaciones(), total())
	}
	// Con la hoja protegida (y permiso de objetos), la de fuera de la tabla no se quita:
	// habría que reescribirla. Se avisa, y nada cambia; la de un campo de la tabla, sí.
	var aviso string
	lz.AlAviso = func(_ ui.TonoAviso, _, detalle string) { aviso = detalle }
	lz.Mandar(CmdProteger, `{"objects":true}`)
	lz.quitarSegmentacion(s.ID)
	if len(l.Segmentaciones()) != 1 || len(l.Dinamicas()[0].HiddenFilters) != 1 || aviso != "sheet.msg.slicerPivotProtected" {
		t.Errorf("protegida, quitar la deja en %+v, filtro %+v y avisa %q", l.Segmentaciones(), l.Dinamicas()[0].HiddenFilters, aviso)
	}
	lz.Mandar(CmdDesproteger, "")
	// La de un campo que está en la tabla oculta sus elementos ahí.
	lz.Mandar(CmdSegAnadir, `{"pivot":"`+nombre+`","col":0}`)
	zona := l.Segmentaciones()[1]
	lz.segmentarDinamica(zona, map[string]bool{"Norte": true})
	if d := l.Dinamicas()[0]; !reflect.DeepEqual(d.Rows[0].Hidden, []string{"Sur"}) || l.HTML(5, 2) != "<b>150</b>" {
		t.Errorf("con solo Norte (y Sillas) los ocultos de las filas son %v y el total %q", d.Rows[0].Hidden, l.HTML(5, 2))
	}
	// Sin la tabla, la tarjeta queda inerte.
	lz.Mandar(CmdDinamicaQuitarla, nombre)
	if activa, _, _ := l.estadoDinamica(zona, lz.t); activa {
		t.Error("la segmentación de una tabla que ya no está sigue viva")
	}
}
