package drive

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// pintarSobre captura `o` a `tam` sobre un fondo `fondo` y devuelve el píxel de un
// punto en unidades de Fyne.
func pintarSobre(t *testing.T, o fyne.CanvasObject, fondo color.NRGBA, tam fyne.Size) func(x, y float32) color.NRGBA {
	t.Helper()
	w := test.NewWindow(container.NewStack(canvas.NewRectangle(fondo), container.NewWithoutLayout(o)))
	t.Cleanup(w.Close)
	w.SetPadded(false)
	w.Resize(tam)
	o.Move(fyne.NewPos(0, 0))
	o.Resize(tam)
	img := w.Canvas().Capture()
	escala := float32(img.Bounds().Dx()) / tam.Width
	return func(x, y float32) color.NRGBA {
		return color.NRGBAModel.Convert(img.At(int(x*escala), int(y*escala))).(color.NRGBA)
	}
}

func cercaColor(a, b color.NRGBA) bool {
	d := func(x, y uint8) int { return max(int(x)-int(y), int(y)-int(x)) }
	return d(a.R, b.R) < 6 && d(a.G, b.G) < 6 && d(a.B, b.B) < 6
}

// TestLasCajasDeLosDialogosEnseñanSuBorde: la caja con relleno (la de añadir miembro,
// la confirmación de restaurar, el enlace creado) enseña su borde, y el marco de una
// lista (versiones, compartidos, mover, miembros) es solo el filete: dentro, el panel.
//
// VISTO EN ROJO (2026-11-06): `conMargen` estiraba los dos rectángulos de la caja a
// todo el tamaño, así que el relleno tapaba el borde (salía {232,239,239} pegado al
// blanco) y, sin relleno, la lista entera era del color del borde ({225,227,227}).
func TestLasCajasDeLosDialogosEnseñanSuBorde(t *testing.T) {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	panel := tema.Color("panel-surface", v)
	tam := fyne.NewSize(120, 60)

	relleno := tema.Color("bg-subtle", v)
	caja := ui.NuevaCaja(relleno, tema.Color("border", v), tema.RadioMD)
	px := pintarSobre(t, dentroDeCaja(caja, tema.Espacio(3), canvas.NewRectangle(color.NRGBA{})), panel, tam)
	if p := px(0.5, 30); cercaColor(p, relleno) || cercaColor(p, panel) {
		t.Errorf("la caja con relleno no enseña el borde: %v", p)
	}
	if p := px(60, 30); !cercaColor(p, relleno) {
		t.Errorf("dentro de la caja sale %v y no el relleno %v", p, relleno)
	}

	px = pintarSobre(t, dentroDeCaja(marcoLista(v, tema.RadioMD), tema.Borde, canvas.NewRectangle(color.NRGBA{})), panel, tam)
	if p := px(60, 30); !cercaColor(p, panel) {
		t.Errorf("dentro del marco de la lista sale %v y no el panel %v", p, panel)
	}
	if p := px(0.5, 30); cercaColor(p, panel) {
		t.Errorf("el marco de la lista no enseña el borde: %v", p)
	}
}

// TestLaInsigniaEsSoloElFilete: el `PermBadge` y las de rol son `rounded-full border`
// sin fondo; dentro se ve lo que haya debajo.
//
// VISTO EN ROJO (2026-11-06): con la caja de relleno transparente y sin trazo, la
// píldora entera salía del color del borde (gris, o el primario al 40 %).
func TestLaInsigniaEsSoloElFilete(t *testing.T) {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo := tema.Color("surface", v)
	for _, i := range []*Insignia{{Texto: "Puede ver"}, {Texto: "Puede editar", Acento: true}} {
		o := nuevaInsigniaSuelta(i, v)
		m := MedidaInsignia(i)
		px := pintarSobre(t, o, fondo, m)
		// En el relleno de la izquierda, antes del texto y lejos de la curva.
		if p := px(tema.Borde+margenXInsignia-1, 1.5); !cercaColor(p, fondo) {
			t.Errorf("«%s»: dentro de la píldora sale %v y no el fondo %v", i.Texto, p, fondo)
		}
		if p := px(m.Width/2, 0.5); cercaColor(p, fondo) {
			t.Errorf("«%s»: el filete de arriba no se ve: %v", i.Texto, p)
		}
	}
}

// TestLosBotonesDeTextoConBordeLoEnseñan: «Subir aquí» de una carpeta compartida y
// «Nueva carpeta» de una unidad son `border` en la web.
//
// VISTO EN ROJO (2026-11-06): la caja nacía sin borde y Pintar no se lo añade, así que
// los dos salían sin filete ninguno: el píxel del borde era el de la barra.
func TestLosBotonesDeTextoConBordeLoEnseñan(t *testing.T) {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo := tema.Color("bg-subtle", v)
	b := nuevoBotonTexto("Subir aquí", tema.TextoSM, ui.SemiNegrita, nil)
	b.margenX, b.alto, b.borde = tema.Espacio(3), AltoControlCromo, true
	m := b.MinSize()
	px := pintarSobre(t, b, fondo, m)
	if p := px(0.5, m.Height/2); cercaColor(p, fondo) {
		t.Errorf("el botón no enseña el borde: %v", p)
	}
	if p := px(tema.Espacio(1), m.Height/2); !cercaColor(p, fondo) {
		t.Errorf("en reposo el botón tapa la barra: %v", p)
	}
}

// TestElPieDeCargaVaSobreLaHoja: el «Cargando más…» de debajo de la lista es la hoja,
// como en la web (`bg-[var(--sytue-sheet-bg)]`).
//
// VISTO EN ROJO (2026-11-06): con `bg`, sobre papel salía una franja gris dentro de la
// hoja blanca.
func TestElPieDeCargaVaSobreLaHoja(t *testing.T) {
	tema.ElegirForma("marco")
	tema.ElegirCromo(tema.CromoPapel)
	t.Cleanup(func() { tema.ElegirCromo(""); tema.FormaDeLosGuardianes() })
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	e := NuevoExplorador(traductor(t))
	pie := e.pieCargando(v).(*fyne.Container)
	if got, quiero := pie.Objects[0].(*canvas.Rectangle).FillColor, color.Color(tema.Color("sheet-bg", v)); got != quiero {
		t.Errorf("el pie va sobre %v y no sobre la hoja %v", got, quiero)
	}
}

// TestElEnlaceSeApilaEnElTelefonoYSusPistasNoSePisan: «¿Hasta cuándo?» y la contraseña
// son dos `w-56` en un `flex-wrap`: en el teléfono la segunda baja de línea con su
// ancho, y en el escritorio «Crear el enlace» queda debajo de las pistas de dos renglones.
//
// VISTO EN ROJO (2026-11-06): en el teléfono la contraseña seguía al lado y se salía
// del diálogo por la derecha; en el escritorio el botón pisaba el segundo renglón.
func TestElEnlaceSeApilaEnElTelefonoYSusPistasNoSePisan(t *testing.T) {
	abs := func(o fyne.CanvasObject) fyne.Position {
		return fyne.CurrentApp().Driver().AbsolutePositionForObject(o)
	}
	for _, ancho := range []float32{393, 1280} {
		p := pantallaDePrueba(t, (&servicioCompartir{}).servidor(t))
		p.win.SetContent(p.Objeto())
		p.win.Resize(fyne.NewSize(ancho, 800))
		p.explor.Ventana = ancho
		c := p.pedirCompartir(Elemento{ID: "f1", Nombre: "acta.odt"})
		p.win.Canvas().Capture()
		caduca, clave := abs(c.caduca), abs(c.clave)
		if ancho < ui.Estrecho {
			if clave.X != caduca.X || clave.Y <= caduca.Y {
				t.Errorf("a %v la contraseña no baja de línea: caducidad %v, contraseña %v", ancho, caduca, clave)
			}
			if c.clave.Size().Width != AnchoCampoCompartir {
				t.Errorf("a %v la contraseña se estira a %v", ancho, c.clave.Size().Width)
			}
			continue
		}
		bajo := caduca.Y + c.caduca.Size().Height + ui.HuecoGrupoCampo + 2*tema.TextoXS.Linea
		if y := abs(c.crear).Y; y < bajo {
			t.Errorf("a %v «Crear el enlace» empieza en %v y pisa la pista, que acaba en %v", ancho, y, bajo)
		}
	}
}

// TestLaNotaDeMiembrosParteYElRolSeLeeEntero: la nota de debajo es un párrafo, y el
// selector del rol de cada miembro mide su opción más larga, como un `<select>`.
//
// VISTO EN ROJO (2026-11-06): la nota era una línea que se cortaba en el borde del
// diálogo y el selector, con el mínimo, enseñaba «G…».
func TestLaNotaDeMiembrosParteYElRolSeLeeEntero(t *testing.T) {
	p := pantallaDePrueba(t, (&servicioEquipos{}).servidor(t))
	p.win.SetContent(p.Objeto())
	p.sec.equipos.Abierta = &api.UnidadEquipo{ID: "u2", Name: "Legal", Role: api.RolGestionar}
	dm := p.pedirMiembros()
	p.win.Canvas().Capture()
	nota := p.tr.T("team.member.note", nil)
	parrafo := false
	var rol *ui.Selector
	recorrer(dm.cuerpo, func(o fyne.CanvasObject) {
		if x, ok := o.(*ui.Parrafo); ok && x.Texto == nota {
			parrafo = true
		}
		if s, ok := o.(*ui.Selector); ok && s != dm.rol {
			rol = s
		}
	})
	if !parrafo {
		t.Error("la nota no es un párrafo que parta")
	}
	if rol == nil {
		t.Fatal("no está el selector del rol del miembro")
	}
	if necesita := ui.Medir(rol.Valor(), tema.TextoBase, ui.Normal).Width + tema.Espacio(9); rol.Size().Width < necesita {
		t.Errorf("el selector mide %v y «%s» necesita %v", rol.Size().Width, rol.Valor(), necesita)
	}
}
