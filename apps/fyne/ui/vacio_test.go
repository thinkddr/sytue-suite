package ui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// TestElVacioV2EsElDeEmptyStateTsx es el guardián: las clases de las que salen las
// medidas del compacto, del error y de la acción secundaria siguen siendo las de la web.
func TestElVacioV2EsElDeEmptyStateTsx(t *testing.T) {
	fuente := webref.Read(t, "apps/frontend/packages/ui/src/components/EmptyState.tsx")
	for _, c := range []struct{ trozo, porque string }{
		{"gap-3 px-6 py-8 text-center", "el de página: HuecoVacio, MargenXVacio y MargenYVacio"},
		{"gap-2 px-4 py-6 text-center", "el compacto: HuecoVacioCompacto, MargenXVacioCompacto y MargenYVacioCompacto"},
		{"'text-text text-sm font-semibold'", "el título compacto en TextoSM y SemiNegrita"},
		{"'text-text-muted max-w-xs text-sm'", "el párrafo compacto a AnchoDescripcionCompacto"},
		{"'sytue-h4 text-text'", "el título de página, EscalonTitulo(4)"},
		{"EMPTY_ICON_SIZE = 32", "LadoGlifoVacio"},
		{"EMPTY_ICON_SIZE_COMPACT = 20", "LadoGlifoVacioCompacto"},
		{"'text-danger-text'", "la tinta del error"},
		{"'text-text-subtle'", "la tinta neutra"},
		{"? 'danger'", "el glifo de peligro por defecto del error"},
		{"error && (compact || (!spot && !illustration))", "el error respeta la ilustración que se le pide, salvo en compacto"},
		{"!compact && 'mb-2'", "el compacto no suma SeparacionExtra a la marca"},
		{"mt-2 flex flex-wrap items-center justify-center gap-2", "las dos acciones con HuecoAccionesVacio, pasando de fila"},
	} {
		if !strings.Contains(fuente, c.trozo) {
			t.Errorf("EmptyState.tsx ya no dice %q (%s)", c.trozo, c.porque)
		}
	}
	exacto(t, "hueco compacto", HuecoVacioCompacto, 8)
	exacto(t, "margen horizontal compacto", MargenXVacioCompacto, 16)
	exacto(t, "margen vertical compacto", MargenYVacioCompacto, 24)
	exacto(t, "párrafo compacto", AnchoDescripcionCompacto, 320)
	exacto(t, "hueco de acciones", HuecoAccionesVacio, 8)
}

// TestElVacioCompactoMideLoQueSusClases: `py-6` y el título en `text-sm`.
// VISTO EN ROJO sin Compacto: mide los 32×2+26 del de página.
func TestElVacioCompactoMideLoQueSusClases(t *testing.T) {
	v := NuevoVacio("Sin resultados", "")
	v.Compacto = true
	exacto(t, "alto compacto sin descripción", v.MedidaVacio(400), 24*2+tema.TextoSM.Linea)

	v.Descripcion = "Prueba con otra búsqueda"
	exacto(t, "alto compacto con descripción", v.MedidaVacio(400),
		24*2+tema.TextoSM.Linea+8+tema.TextoSM.Linea)
}

// TestElVacioCompactoNoLlevaIlustracion: en un panel no cabe el spot de 120.
func TestElVacioCompactoNoLlevaIlustracion(t *testing.T) {
	v := NuevoVacio("Sin resultados", "")
	v.Ilustracion = iconos.Pintar(iconos.VacioMail, tema.Claro).A(120).Objeto()
	conIlustracion := v.MedidaVacio(400)
	v.Compacto = true
	if v.ilustracion() != nil {
		t.Fatal("el compacto sigue colocando la ilustración")
	}
	if v.MedidaVacio(400) >= conIlustracion {
		t.Errorf("el compacto mide %v, y con ilustración %v: la sigue contando",
			v.MedidaVacio(400), conIlustracion)
	}
	r := test.TempWidgetRenderer(t, v)
	for _, o := range r.Objects() {
		if o == v.Ilustracion {
			t.Error("la ilustración está en el árbol del compacto")
		}
	}
}

// TestElVacioDeErrorLlevaElGlifoDePeligro: sin Glifo, el error pinta el de peligro a 32
// (20 en compacto) en `danger-text`. VISTO EN ROJO sin Tono: no hay glifo.
func TestElVacioDeErrorLlevaElGlifoDePeligro(t *testing.T) {
	v := NuevoVacio("No se pudo cargar", "")
	v.Tono = VacioError
	g := v.glifo()
	if g == nil {
		t.Fatal("el error no tiene glifo")
	}
	exacto(t, "lado del glifo de error", g.Medida().Width, LadoGlifoVacio)
	if g.Tinta() != tema.Color("danger-text", tema.Claro) {
		t.Errorf("el glifo de error es %v y tiene que ser `danger-text`", g.Tinta())
	}
	exacto(t, "alto con el glifo", v.MedidaVacio(400), 32*2+LadoGlifoVacio+8+12+26)

	c := NuevoVacio("No se pudo cargar", "")
	c.Tono, c.Compacto = VacioError, true
	exacto(t, "lado del glifo de error compacto", c.glifo().Medida().Width, LadoGlifoVacioCompacto)
	// En compacto la marca no lleva `mb-2`: solo el `gap-2`.
	exacto(t, "alto compacto con el glifo", c.MedidaVacio(400), 24*2+LadoGlifoVacioCompacto+8+tema.TextoSM.Linea)

	img := pinta(t, 300, 200, func() fyne.CanvasObject { v.Resize(fyne.NewSize(300, 200)); return v }())
	quiero := tema.Color("danger-text", tema.Claro)
	b, hay := img.Bounds(), 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if cerca(px(img, x, y), quiero) {
				hay++
			}
		}
	}
	if hay == 0 {
		t.Error("el estado de error no pinta nada en `danger-text`")
	}
}

// TestElVacioDeErrorTineElGlifoQueSeLePasa: el glifo propio también va en `danger-text`.
func TestElVacioDeErrorTineElGlifoQueSeLePasa(t *testing.T) {
	v := NuevoVacio("Sin conexión", "")
	v.Tono = VacioError
	v.Glifo = iconos.Trazo(iconos.Candado, LadoGlifoVacio, "text", tema.Claro)
	test.TempWidgetRenderer(t, v)
	if v.glifo() != v.Glifo {
		t.Fatal("el error ha cambiado el glifo que se le pasó")
	}
	if v.Glifo.Tinta() != tema.Color("danger-text", tema.Claro) {
		t.Errorf("el glifo del error es %v y tiene que ser `danger-text`", v.Glifo.Tinta())
	}
}

// TestElVacioColocaLaAccionSecundaria: a la derecha de la principal con el `gap-2`, y
// debajo cuando no caben (`flex-wrap`). VISTO EN ROJO sin AccionSecundaria: no se coloca.
func TestElVacioColocaLaAccionSecundaria(t *testing.T) {
	v := NuevoVacio("Sin resultados", "")
	a, b := NuevoBoton("Nueva búsqueda", nil), NuevoBoton("Quitar filtro", nil)
	v.Accion, v.AccionSecundaria = a, b
	const ancho = 480
	v.Resize(fyne.NewSize(ancho, v.MedidaVacio(ancho)))
	test.TempWidgetRenderer(t, v).Layout(v.Size())
	if a.Position().Y != b.Position().Y {
		t.Errorf("con sitio, las acciones van en una fila: %v y %v", a.Position(), b.Position())
	}
	exacto(t, "hueco entre acciones", b.Position().X-(a.Position().X+a.Size().Width), HuecoAccionesVacio)
	izq, der := a.Position().X, ancho-(b.Position().X+b.Size().Width)
	if d := izq - der; d < -0.5 || d > 0.5 {
		t.Errorf("la fila no está centrada: %v a la izquierda y %v a la derecha", izq, der)
	}
	exacto(t, "alto con una fila", v.MedidaVacio(ancho), 32*2+26+12+8+a.MinSize().Height)

	// Estrecho: la secundaria pasa debajo.
	estrecho := a.MinSize().Width + 2*MargenXVacio + 10
	v.Resize(fyne.NewSize(estrecho, v.MedidaVacio(estrecho)))
	test.TempWidgetRenderer(t, v).Layout(v.Size())
	if b.Position().Y <= a.Position().Y {
		t.Errorf("sin sitio, la secundaria tiene que pasar debajo: %v y %v", a.Position(), b.Position())
	}
	exacto(t, "alto con dos filas", v.MedidaVacio(estrecho),
		32*2+26+12+8+a.MinSize().Height+HuecoAccionesVacio+b.MinSize().Height)
}

// TestElVacioDeErrorRespetaLaIlustracion: como el `spot="errorRetry"` de la web, la
// ilustración que se pide gana al glifo de peligro; en compacto, que no lleva
// ilustración, vuelve el glifo. VISTO EN ROJO con el glifo impuesto siempre.
func TestElVacioDeErrorRespetaLaIlustracion(t *testing.T) {
	v := NuevoVacio("No se pudo cargar", "")
	v.Tono = VacioError
	v.Ilustracion = iconos.Pintar(iconos.VacioMail, tema.Claro).A(120).Objeto()
	if v.glifo() != nil || v.ilustracion() != v.Ilustracion {
		t.Error("el error ha tapado la ilustración que se le pidió con el glifo de peligro")
	}
	v.Compacto = true
	if g := v.glifo(); g == nil || g.Medida().Width != LadoGlifoVacioCompacto {
		t.Error("el error compacto no lleva el glifo de peligro a LadoGlifoVacioCompacto")
	}
}
