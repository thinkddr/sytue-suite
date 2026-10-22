// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	fcanvas "fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// TestElContratoDelMenuEsElDeDropdownTsx lee las clases DEL FUENTE de la web. Un menú
// es casi todo CSS, así que su contrato no se puede comprobar midiendo píxeles: lo que
// se comprueba es que los números de los que salen esos píxeles siguen siendo los suyos.
func TestElContratoDelMenuEsElDeDropdownTsx(t *testing.T) {
	fuente := webref.Read(t, "apps/frontend/packages/ui/src/components/Dropdown.tsx")
	for _, c := range []struct{ trozo, porque string }{
		{"const MARGEN_VENTANA = 8", "el aire con el borde de la ventana"},
		{"min-w-[220px]", "el ancho mínimo del panel"},
		{"max-w-[calc(100vw-16px)]", "el tope de ancho, que son dos márgenes"},
		{"rounded-lg", "la esquina del panel: 8"},
		{"p-1 shadow-[var(--shadow-md)]", "el relleno del panel y su elevación"},
		{"d.bottom + 6", "los 6 px entre el disparador y el panel"},
		{"const ALTO_MINIMO = 120", "el suelo del hueco: por debajo, el panel se queda en la ventana"},
		{"const sitioAbajo = ventana.alto - d.bottom - 6 - MARGEN_VENTANA;", "el hueco de abajo"},
		{"const sitioArriba = d.top - 6 - MARGEN_VENTANA;", "el hueco de arriba"},
		{"panel.alto > sitioAbajo && sitioArriba > sitioAbajo", "arriba solo si no cabe abajo y ahí hay más sitio"},
		{"el.scrollHeight - el.scrollTop - el.clientHeight > 1", "cuándo queda más abajo"},
		{"sticky bottom-0 -mt-8 flex h-8 items-end justify-center bg-linear-to-t from-[var(--sytue-menu-surface)] from-35% to-transparent", "el aviso: 32 de alto, macizo el 35 %"},
		{`<Icon name="chevronDown" size={14} />`, "la flecha del aviso"},
		{"px-3 py-2 text-base", "la densidad normal de un ítem"},
		{"px-2.5 py-1.5 text-sm", "la densidad densa de un ítem"},
		{"'flex w-full items-center gap-2 rounded-md text-left", "la base de un ítem"},
		{"-mx-1 my-1 h-px", "el separador, que se come el relleno del panel"},
	} {
		if !strings.Contains(fuente, c.trozo) {
			t.Errorf("Dropdown.tsx ya no dice %q (%s): la medida de aquí ha dejado de ser la suya", c.trozo, c.porque)
		}
	}
	if MinAnchoMenu != 220 || MargenVentana != 8 || SeparacionMenu != 6 || AltoMinimoMenu != 120 ||
		AltoMasAbajo != 32 || GlifoMasAbajo != 14 || macizoMasAbajo != 0.35 {
		t.Errorf("las constantes del menú son %v/%v/%v/%v y las del aviso %v/%v/%v; la web dice 220/8/6/120 y 32/14/0,35",
			MinAnchoMenu, MargenVentana, SeparacionMenu, AltoMinimoMenu, AltoMasAbajo, GlifoMasAbajo, macizoMasAbajo)
	}
}

// TestElElementoDeMenuMideLoDeSuDensidad son las dos cifras absolutas: `px-3 py-2
// text-base` da 33,25 de alto y `px-2.5 py-1.5 text-sm` da 26,25, con el rem de 14.
func TestElElementoDeMenuMideLoDeSuDensidad(t *testing.T) {
	for _, c := range []struct {
		denso  bool
		quiero float32
		porque string
	}{
		{false, 2*8 + 22, "py-2 ×2 + la caja de línea de text-base"},
		{true, 2*6 + 18, "py-1.5 ×2 + la caja de línea de text-sm"},
	} {
		e := NuevoElementoMenu("Descargar", iconos.Ninguno, nil)
		e.Denso = c.denso
		declarada := MedidaElementoMenu(e)
		if declarada.Height != c.quiero {
			t.Errorf("con denso=%v el ítem mide %v de alto y son %v (%s)",
				c.denso, declarada.Height, c.quiero, c.porque)
		}
		if got := e.MinSize(); got != declarada {
			t.Errorf("MinSize() = %v y MedidaElementoMenu dice %v: hay dos cuentas", got, declarada)
		}
	}
}

// TestElIconoDeUnItemLeSumaSuHueco: el `gap-2` no es decoración, es ancho.
func TestElIconoDeUnItemLeSumaSuHueco(t *testing.T) {
	sin := MedidaElementoMenu(NuevoElementoMenu("Renombrar", iconos.Ninguno, nil))
	con := MedidaElementoMenu(NuevoElementoMenu("Renombrar", iconos.Ajustes, nil))
	if d := con.Width - sin.Width; d != iconos.TallaSM+huecoElemento {
		t.Errorf("el icono le suma %v al ítem y son %v (16 del glifo y 7 de `gap-2`)", d, iconos.TallaSM+huecoElemento)
	}
}

// TestUnGlifoPropioOcupaElSitioDelIcono: el menú de acciones de Drive lleva los glifos
// de su `glyphs.tsx`, y tienen que medir lo mismo que un icono del catálogo.
func TestUnGlifoPropioOcupaElSitioDelIcono(t *testing.T) {
	conIcono := MedidaElementoMenu(NuevoElementoMenu("Renombrar", iconos.Ajustes, nil))
	g := NuevoElementoMenu("Renombrar", iconos.Ninguno, nil)
	g.Glifo = iconos.TrazoDeCuerpo(`<path d="M12 20h9" />`, iconos.TallaSM, tema.Color("text", tema.Claro))
	if m := MedidaElementoMenu(g); m != conIcono {
		t.Errorf("con glifo propio mide %v y con icono %v", m, conIcono)
	}
}

// TestElPanelDeMenuRespetaSuMinimoYSuTope: `min-w-[220px]` por abajo y
// `max-w-[calc(100vw-16px)]` por arriba.
func TestElPanelDeMenuRespetaSuMinimoYSuTope(t *testing.T) {
	corto := NuevoMenu(NuevoElementoMenu("Abrir", iconos.Ninguno, nil))
	if got := MedidaMenu(corto, fyne.NewSize(1280, 800)).Width; got != MinAnchoMenu {
		t.Errorf("un menú de un ítem corto mide %v y el mínimo es %v", got, MinAnchoMenu)
	}

	largo := NuevoMenu(NuevoElementoMenu(strings.Repeat("Exportar como hoja de cálculo ", 4), iconos.Ninguno, nil))
	quiero := MedidaElementoMenu(largo.Elementos[0].(*ElementoMenu)).Width + 2*(rellenoPanelMenu+tema.Borde)
	if got := MedidaMenu(largo, fyne.NewSize(1280, 800)).Width; got != quiero {
		t.Errorf("un menú de un ítem largo mide %v y su ítem más el `p-1` y el borde dan %v", got, quiero)
	}
	// En una ventana estrecha, el tope: el panel no puede ser más ancho que la
	// ventana menos dos márgenes, o el clamp lo sacaría por el otro lado.
	if quiero <= 800-2*MargenVentana {
		t.Fatalf("el ítem largo mide %v y no prueba el tope a 800", quiero)
	}
	if got := MedidaMenu(largo, fyne.NewSize(800, 844)).Width; got != 800-2*MargenVentana {
		t.Errorf("en 800 de ancho el panel mide %v y `max-w-[calc(100vw-16px)]` da %v", got, 800-2*MargenVentana)
	}
	// Por debajo de 768 es la hoja, a todo el ancho.
	if got := MedidaMenu(largo, fyne.NewSize(390, 844)).Width; got != 390 {
		t.Errorf("en la hoja de 390 el panel mide %v y va a todo el ancho", got)
	}
}

// TestElMenuSeEncajaComoEnDropdownTsx son los casos de `encajarPanel` de la web
// (Dropdown.test.tsx), con sus mismos números: el panel se queda en el hueco de un lado
// de su disparador —abajo, o arriba si ahí hay más sitio— y no lo tapa; lo que no cabe
// se desplaza por dentro.
//
// VISTO EN ROJO con la cuenta de antes, que subía el panel para que cupiera: el de 900
// de alto arrancaba en y=8 con sus 900 y tapaba el disparador.
func TestElMenuSeEncajaComoEnDropdownTsx(t *testing.T) {
	ventana := fyne.NewSize(1280, 800)
	avatar, tamAvatar := fyne.NewPos(1220, 10), fyne.NewSize(36, 36)

	// Lo que cabe bajo el disparador no se acota.
	pos, tope := EncajarMenu(avatar, tamAvatar, fyne.NewSize(220, 300), ventana, false, false)
	if pos != fyne.NewPos(1036, 52) || tope < 300 {
		t.Errorf("lo que cabe va en %v con tope %v; quiero (1036, 52) y sin acotar", pos, tope)
	}
	// Cerca del pie se abre hacia arriba, pegado al disparador y sin taparlo.
	pie, tam := fyne.NewPos(600, 740), fyne.NewSize(36, 36)
	if pos, _ := EncajarMenu(pie, tam, fyne.NewSize(220, 300), ventana, true, false); pos.Y != 740-6-300 {
		t.Errorf("cerca del pie el panel va a y=%v y volcado toca en %v", pos.Y, float32(740-6-300))
	}
	// Y si ni arriba cabe entero, se queda con ese hueco y se desplaza por dentro.
	pos, tope = EncajarMenu(pie, tam, fyne.NewSize(220, 900), ventana, true, false)
	if tope != 740-6-8 || pos.Y != 8 || pos.Y+tope > 740-6 {
		t.Errorf("sin sitio para todo, el panel va a y=%v con tope %v; quiero y=8, tope %v y sin tapar el disparador", pos.Y, tope, float32(740-6-8))
	}
	// Sin hueco en ningún lado, el panel se queda dentro de la ventana.
	pos, tope = EncajarMenu(fyne.NewPos(10, 60), tam, fyne.NewSize(220, 400), fyne.NewSize(320, 180), true, false)
	if tope != AltoMinimoMenu || pos.Y < 8 || pos.Y+tope > 180-8 {
		t.Errorf("sin hueco, el panel va a y=%v con tope %v; quiero dentro de la ventana y el mínimo de %v", pos.Y, tope, AltoMinimoMenu)
	}
	// Un teclado puede dejar menos altura que el suelo de 120: manda el área segura.
	pos, tope = EncajarMenu(fyne.NewPos(10, 40), tam, fyne.NewSize(220, 400), fyne.NewSize(320, 96), true, false)
	if tope != 80 || pos.Y < 8 || pos.Y+tope > 88 {
		t.Errorf("el suelo del menú sale del área segura de 96 px: y=%v, alto=%v", pos.Y, tope)
	}
	// El menú de cuenta, a los tamaños en que se midió: empieza bajo su botón y no se sale.
	menu := fyne.NewSize(282, 760)
	for _, v := range []fyne.Size{{Width: 1280, Height: 800}, {Width: 1280, Height: 720}, {Width: 390, Height: 700}} {
		d := fyne.NewPos(v.Width-60, 10)
		pos, tope := EncajarMenu(d, tamAvatar, menu, v, false, false)
		if pos.Y != 10+36+6 || pos.Y+min(menu.Height, tope) > v.Height-8 || pos.X < 8 || pos.X+menu.Width > v.Width-8 {
			t.Errorf("a %v el menú de cuenta va en %v con tope %v", v, pos, tope)
		}
	}
}

// TestElMenuSeDesplazaPorDentro: un menú más alto que su hueco no se sale de la ventana
// ni tapa su disparador; se desplaza por dentro hasta el final —con la rueda y al llevar
// el foco con las flechas— y, mientras queda algo por ver, el borde de abajo se funde con
// la superficie y lleva una flecha.
//
// VISTO EN ROJO sin el desplazamiento en el Layout: la rueda no mueve nada y el último
// elemento no se alcanza.
func TestElMenuSeDesplazaPorDentro(t *testing.T) {
	elementos := make([]fyne.CanvasObject, 30)
	for i := range elementos {
		elementos[i] = NuevoElementoMenu("Opción", iconos.Ninguno, nil)
	}
	m := NuevoMenu(elementos...)
	disparador := NuevoBoton("Abrir", nil)
	w := test.NewTempWindow(t, container.NewWithoutLayout(disparador))
	w.Resize(fyne.NewSize(800, 400)) // de 768 para arriba: colgado de su disparador
	disparador.Move(fyne.NewPos(300, 20))
	disparador.Resize(fyne.NewSize(80, 32))
	m.Mostrar(disparador)
	t.Cleanup(m.Cerrar)
	r := test.WidgetRenderer(m).(*menuRenderer)

	natural := MedidaMenu(m, w.Canvas().Size()).Height
	// Donde está de verdad el disparador: la ventana de prueba lleva su relleno.
	debajo := fyne.CurrentApp().Driver().AbsolutePositionForObject(disparador).Y + 32 + SeparacionMenu
	hueco := w.Canvas().Size().Height - debajo - MargenVentana
	if natural <= hueco {
		t.Fatalf("el menú de prueba mide %v y cabe en %v: no prueba nada", natural, hueco)
	}
	if m.Position().Y != debajo || m.Size().Height != hueco {
		t.Fatalf("el menú va en y=%v con %v de alto; quiero bajo su disparador (y=%v) y el hueco, %v", m.Position().Y, m.Size().Height, debajo, hueco)
	}
	if !r.aviso.velo.Visible() {
		t.Error("queda menú por debajo y el panel no avisa")
	}
	ultimo := elementos[len(elementos)-1]
	// Con las flechas: del primero al último da la vuelta, y el último queda a la vista.
	m.mover(m.elemento(0), -1)
	dentro := tema.Borde + rellenoPanelMenu
	if fin := ultimo.Position().Y + ultimo.Size().Height; fin != hueco-dentro {
		t.Errorf("con el foco en el último elemento, acaba en y=%v de un panel de %v; quiero %v", fin, hueco, hueco-dentro)
	}
	if r.aviso.velo.Visible() {
		t.Error("al llegar al final el aviso sigue ahí y tapa el último elemento")
	}
	// Y con la rueda, de vuelta arriba.
	m.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.Delta{DY: 1e6}})
	if elementos[0].Position().Y != dentro || !r.aviso.velo.Visible() {
		t.Errorf("la rueda no devuelve el menú arriba: el primero está en y=%v", elementos[0].Position().Y)
	}
}

// Un selector compuesto recibe el hueco real y conserva el único Scroll bajo el
// dedo. Si el menú lo redimensionase a su MinSize, volvería a recortarse por abajo.
func TestElMenuPuedeEntregarSuAltoVisibleAlContenido(t *testing.T) {
	contenido := fcanvas.NewRectangle(tema.Color("bg", tema.Claro))
	contenido.SetMinSize(fyne.NewSize(260, 900))
	m := NuevoMenu(contenido)
	m.ContenidoAjustable = true
	tam := fyne.NewSize(280, 110)
	m.Resize(tam)
	r := test.WidgetRenderer(m).(*menuRenderer)
	r.Layout(tam)
	dentro := tema.Borde + rellenoPanelMenu
	if contenido.Size().Height != tam.Height-2*dentro || contenido.Position().Y != dentro {
		t.Fatalf("el contenido no recibió el hueco del menú: %v/%v", contenido.Position(), contenido.Size())
	}
	m.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.Delta{DY: -100}})
	if m.desplazado != 0 || contenido.Position().Y != dentro {
		t.Fatal("el menú y el contenido compiten por desplazar el mismo panel")
	}
	if r.aviso.velo.Visible() {
		t.Fatal("el aviso del menú tapa el final del desplazador que pertenece al contenido")
	}
}

// TestElMenuNoSeSaleDeLaVentana es el clamp: un menú ancho colgado de un disparador que
// no está en el borde arranca en coordenada NEGATIVA, y lo recortado no se alcanza
// desplazando.
func TestElMenuNoSeSaleDeLaVentana(t *testing.T) {
	ventana := fyne.NewSize(1280, 800)
	panel := fyne.NewSize(330, 200) // el centro de notificaciones
	tamDisparador := fyne.NewSize(32, 32)

	// `align='end'` colgando de un disparador pegado a la izquierda: x sería −298.
	izq, _ := EncajarMenu(fyne.NewPos(0, 100), tamDisparador, panel, ventana, false, false)
	if izq.X != MargenVentana {
		t.Errorf("pegado al borde izquierdo el panel va a x=%v y el tope lo deja en %v", izq.X, MargenVentana)
	}
	// `align='start'` colgando de uno pegado a la derecha: se saldría por el otro.
	der, _ := EncajarMenu(fyne.NewPos(1248, 100), tamDisparador, panel, ventana, true, false)
	if quiero := ventana.Width - panel.Width - MargenVentana; der.X != quiero {
		t.Errorf("pegado al borde derecho el panel va a x=%v y el tope lo deja en %v", der.X, quiero)
	}
	// Y la alineación por defecto es `end`: el panel termina donde el disparador.
	fin, _ := EncajarMenu(fyne.NewPos(600, 100), tamDisparador, panel, ventana, false, false)
	if quiero := 600 + 32 - panel.Width; fin.X != quiero {
		t.Errorf("con `align='end'` el panel va a x=%v y su borde derecho toca en %v", fin.X, quiero)
	}
}

// TestElSeparadorLlegaDeBordeABorde: el `-mx-1` se come el `p-1` del panel. Si no lo
// hiciera, la línea quedaría 3,5 px corta por cada lado y se vería.
func TestElSeparadorLlegaDeBordeABorde(t *testing.T) {
	m := NuevoMenu(
		NuevoElementoMenu("Compartir", iconos.Ninguno, nil),
		NuevoSeparadorMenu(),
		NuevoElementoMenu("Eliminar", iconos.Ninguno, nil),
	)
	tam := MedidaMenu(m, fyne.NewSize(1280, 800))
	m.Resize(tam)
	img := pinta(t, tam.Width+10, tam.Height+10, m)

	// Se mide la FILA del separador y no la caja del color en toda la imagen: el
	// antialias de las cuatro esquinas redondeadas del panel deja píxeles a media
	// cobertura del borde que caen justo en el tono de `border-subtle`, y eso
	// ensanchaba la medida dos píxeles. Es el mismo error de lectura del que avisa
	// `pinta`: una captura mal leída da la conclusión contraria.
	linea := BordeSobre("border-subtle", "menu-surface", tema.Claro)
	y := int((tema.Borde + rellenoPanelMenu + MedidaElementoMenu(m.Elementos[0].(*ElementoMenu)).Height +
		tema.Espacio(1)) * escala)
	// La línea mide UN píxel de CSS, o sea dos filas de dispositivo a ×2, y cae en un
	// medio píxel (41,25), así que el pintor elige. Se busca en la banda y se toma la
	// fila más ancha.
	x0, x1 := -1, -1
	for dy := -2; dy <= 3; dy++ {
		a, b := -1, -1
		for x := range int((tam.Width + 10) * escala) {
			if cerca(px(img, x, y+dy), linea) {
				if a < 0 {
					a = x
				}
				b = x
			}
		}
		if b-a > x1-x0 {
			x0, x1 = a, b
		}
	}
	if x0 < 0 {
		t.Fatalf("no hay separador en la fila %d: la línea no se ha pintado", y)
	}
	if ancho := float32(x1-x0+1) / escala; ancho != tam.Width-2*tema.Borde {
		t.Errorf("el separador pinta %v de ancho y el interior del panel mide %v",
			ancho, tam.Width-2*tema.Borde)
	}
	if izq := float32(x0) / escala; izq != tema.Borde {
		t.Errorf("el separador empieza en %v y el borde del panel ocupa el primer %v: el `-mx-1` no se está comiendo el `p-1`", izq, tema.Borde)
	}
	if got := AltoSeparadorMenu(); got != tema.Borde+2*tema.Espacio(1) {
		t.Errorf("el separador ocupa %v y son 1 de línea más `my-1` por los dos lados", got)
	}
}

// TestElPanelDeMenuPintaLoQueDiceMedir comprueba que el panel ocupa exactamente
// la talla que anuncia: si `Medir` y lo pintado se separan, el menú tapa o deja
// hueco y nadie lo ve hasta que se abre encima de otra cosa.
func TestElPanelDeMenuPintaLoQueDiceMedir(t *testing.T) {
	m := NuevoMenu(
		NuevoElementoMenu("Descargar", iconos.Ninguno, nil),
		NuevoElementoMenu("Cerrar sesión", iconos.Ninguno, nil),
	)
	tam := MedidaMenu(m, fyne.NewSize(1280, 800))
	m.Resize(tam)
	img := pinta(t, tam.Width+20, tam.Height+20, m)
	x0, x1, y0, y1 := caja(img, BordeSobre("border", "menu-surface", tema.Claro))
	mismaCaja(t, "el panel del menú", tam, x0, x1, y0, y1)
}

// TestLasFlechasCirculanYSaltanLosInertes: la navegación de `onKeyDown`, incluido el
// `:not([disabled])` del selector, que es lo que deja fuera a un ítem apagado.
func TestLasFlechasCirculanYSaltanLosInertes(t *testing.T) {
	disparador := NuevoBoton("Acciones", nil)
	w := test.NewTempWindow(t, disparador)
	w.Resize(fyne.NewSize(1280, 800))

	uno := NuevoElementoMenu("Abrir", iconos.Ninguno, nil)
	dos := NuevoElementoMenu("Renombrar", iconos.Ninguno, nil)
	apagado := NuevoElementoMenu("Restaurar", iconos.Ninguno, nil)
	apagado.Inerte = true
	tres := NuevoElementoMenu("Eliminar", iconos.Ninguno, nil)
	tres.Peligro = true
	m := NuevoMenu(uno, dos, NuevoSeparadorMenu(), apagado, tres)
	m.Mostrar(disparador)

	if got := w.Canvas().Focused(); got != fyne.Focusable(uno) {
		t.Fatalf("al abrir el foco está en %T y tiene que estar en el primer ítem", got)
	}
	for _, quiero := range []*ElementoMenu{dos, tres, uno} { // el inerte se salta, y circula
		w.Canvas().Focused().TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown})
		if got := w.Canvas().Focused(); got != fyne.Focusable(quiero) {
			t.Fatalf("la flecha abajo ha llevado el foco a %v y tocaba %q",
				got, quiero.Texto)
		}
	}
	// Y hacia arriba, circulando por el otro extremo.
	w.Canvas().Focused().TypedKey(&fyne.KeyEvent{Name: fyne.KeyUp})
	if got := w.Canvas().Focused(); got != fyne.Focusable(tres) {
		t.Errorf("la flecha arriba desde el primero ha llevado el foco a %v y tocaba el último", got)
	}
}

// TestEnUnDialogoElMenuSeApilaEncimaYEscapeSeLoLlevaPrimero es la regla «primero lo de
// encima», y es la que más se agradece: sin ella, escribir tres letras en el
// autocompletado de direcciones y pulsar Escape cerraba la redacción entera.
func TestEnUnDialogoElMenuSeApilaEncimaYEscapeSeLoLlevaPrimero(t *testing.T) {
	w := test.NewTempWindow(t, fcanvas.NewRectangle(tema.Color("bg", tema.Claro)))
	w.Resize(fyne.NewSize(1280, 800))
	c := w.Canvas()

	disparador := NuevoBoton("Más opciones", nil)
	cerrado := 0
	d := NuevoDialogo("Redactar", disparador)
	d.AlCerrar = func() { cerrado++ }
	d.Mostrar(c)

	m := NuevoMenu(NuevoElementoMenu("Adjuntar de Drive", iconos.Ninguno, nil))
	m.Mostrar(disparador)
	if n := len(c.Overlays().List()); n != 2 {
		t.Fatalf("con diálogo y menú abiertos hay %d capas y tienen que ser dos", n)
	}

	// El primer Escape se lleva el MENÚ y deja el diálogo en pie.
	if !Descartar(c) {
		t.Fatal("Descartar no ha encontrado la capa de arriba")
	}
	if cerrado != 0 {
		t.Error("el primer Escape ha cerrado el diálogo: tenía que llevarse solo el menú")
	}
	if n := len(c.Overlays().List()); n != 1 {
		t.Fatalf("tras cerrar el menú quedan %d capas y tiene que quedar el diálogo", n)
	}
	// El segundo ya es del diálogo.
	Descartar(c)
	if cerrado != 1 {
		t.Errorf("el segundo Escape ha cerrado el diálogo %d veces", cerrado)
	}
}

// TestPulsarUnItemRecogeElMenu: en la web el panel se queda abierto salvo que el ítem
// llame a `close()`, y los catorce ítems de la suite que abren algo lo llaman. Aquí lo
// hace el propio ítem, que es lo que evita catorce sitios donde olvidarlo.
func TestPulsarUnItemRecogeElMenu(t *testing.T) {
	disparador := NuevoBoton("Acciones", nil)
	w := test.NewTempWindow(t, disparador)
	w.Resize(fyne.NewSize(1280, 800))

	pulsado := 0
	item := NuevoElementoMenu("Duplicar", iconos.Ninguno, func() { pulsado++ })
	m := NuevoMenu(item)
	m.Mostrar(disparador)
	test.Tap(item)
	if pulsado != 1 {
		t.Errorf("el ítem ha llamado a su acción %d veces", pulsado)
	}
	if n := len(w.Canvas().Overlays().List()); n != 0 {
		t.Errorf("tras pulsar un ítem quedan %d capas: el menú no se ha recogido", n)
	}

	// Un ítem inerte no hace nada y no recoge nada.
	apagado := NuevoElementoMenu("Restaurar", iconos.Ninguno, func() { pulsado++ })
	apagado.Inerte = true
	m2 := NuevoMenu(apagado)
	m2.Mostrar(disparador)
	test.Tap(apagado)
	if pulsado != 1 {
		t.Error("un ítem inerte ha llamado a su acción")
	}
	if n := len(w.Canvas().Overlays().List()); n != 1 {
		t.Errorf("un ítem inerte ha recogido el menú (%d capas)", n)
	}
	m2.Cerrar()
}

// TestElGlifoPropioOcupaElHuecoDelIconoYNoSeTine: Sytue Mail pinta su triaje con glifos
// PROPIOS (`<Glyph name="unread" />`) y el punto de color de una etiqueta como hijo del
// `MenuItem`. Los dos van por `Glifo`: suma su lado más el `gap-2`, igual que un icono
// del catálogo, y conserva el color con el que se construyó —el punto de una etiqueta
// es un dato de la cuenta, y teñirlo de `text-muted` lo borraría—.
//
// VISTO EN ROJO (2026-09-22): quitando el `!r.propio` de Refresh, el punto sale gris
// (`text-muted`) y el test señala el píxel del centro.
func TestElGlifoPropioOcupaElHuecoDelIconoYNoSeTine(t *testing.T) {
	const lado = 2.5 * 0.25 * tema.Rem // el `h-2.5 w-2.5` del punto de la etiqueta
	rojo := color.NRGBA{R: 0xa4, G: 0x62, B: 0x3f, A: 255}
	e := NuevoElementoMenu("Obra", iconos.Ninguno, nil)
	e.TamIcono = lado
	e.Glifo = iconos.TrazoDeRutaRelleno("M12 1a11 11 0 1 0 0 22a11 11 0 1 0 0-22Z", lado, rojo)
	sin := MedidaElementoMenu(NuevoElementoMenu("Obra", iconos.Ninguno, nil))
	if d := MedidaElementoMenu(e).Width - sin.Width; d != lado+huecoElemento {
		t.Errorf("el glifo le suma %v al ítem y son %v (su lado y el `gap-2`)", d, lado+huecoElemento)
	}
	tam := MedidaElementoMenu(e)
	e.Resize(tam)
	e.Refresh()
	img := pinta(t, tam.Width, tam.Height, e)
	// El centro del punto: `px-3` más medio lado, a media altura; a la escala del pintor.
	x := int((tema.Espacio(3) + lado/2) * escala)
	y := int(tam.Height / 2 * escala)
	got := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
	if difByte(got.R, rojo.R) > 2 || difByte(got.G, rojo.G) > 2 || difByte(got.B, rojo.B) > 2 {
		t.Errorf("el centro del punto sale %v y se construyó %v: alguien lo ha teñido", got, rojo)
	}
}

func difByte(a, b uint8) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}

// TestElPuntoDelMenuPuedeSerUnDato: el filtro por etiqueta de Notepads pinta el punto
// con el color del ESTADO, que no es un token. VISTO EN ROJO sin `PuntoColor`: el ítem
// no llevaba punto.
func TestElPuntoDelMenuPuedeSerUnDato(t *testing.T) {
	e := NuevoElementoMenu("Estado: Hecho", iconos.Ninguno, nil)
	e.PuntoColor, e.TamIcono = color.NRGBA{R: 0x16, G: 0xa3, B: 0x4a, A: 255}, LadoPuntoMenu
	tam := e.MinSize()
	e.Resize(tam)
	r := test.WidgetRenderer(e).(*elementoRenderer)
	r.Layout(tam)
	if r.punto == nil || r.punto.FillColor != e.PuntoColor {
		t.Fatal("el ítem no pinta el punto con el color del dato")
	}
}

// TestElClicQueCierraElMenuNoSePierde: en la web el clic fuera cierra el menú al apretar
// y lo de debajo recibe su clic. Aquí el velo cierra en `MouseDown` y la capa se declara
// `TapThrough`, que es lo que el driver del fork pregunta para volver a despachar el clic
// a lo de debajo (su guardián con una ventana GLFW de verdad está en el fork). La capa de
// un diálogo NO lo deja pasar: su velo es el `onClick` del fondo.
func TestElClicQueCierraElMenuNoSePierde(t *testing.T) {
	disparador := NuevoBoton("Más", nil)
	w := test.NewTempWindow(t, container.NewWithoutLayout(disparador))
	w.Resize(fyne.NewSize(800, 600))
	disparador.Resize(disparador.MinSize())
	m := NuevoMenu(NuevoElementoMenu("Renombrar", iconos.Ninguno, nil))
	m.Mostrar(disparador)
	cp := m.capa
	if !cp.TapThrough() || !cp.ScrollThrough() {
		t.Error("la capa de un menú se come el clic y la rueda que la cierran")
	}
	m.capa.fondo.MouseDown(&desktop.MouseEvent{PointEvent: fyne.PointEvent{AbsolutePosition: fyne.NewPos(410, 410)}})
	if m.Abierto() {
		t.Fatal("apretar fuera no cierra el menú")
	}
	d := NuevoDialogo("Reglas", cuerpoDe(20))
	d.Mostrar(w.Canvas())
	t.Cleanup(d.Cerrar)
	if d.capa.TapThrough() || d.capa.ScrollThrough() {
		t.Error("la capa de un diálogo deja pasar el clic de su velo: en la web es modal")
	}
}

// TestUnItemPuedeDejarElMenuAbierto: en Dropdown.tsx el menú solo se cierra con
// Escape, el clic fuera o `close()`. La presencia del menú de cuenta cambia y el menú se
// queda. VISTO EN ROJO con el cierre en todo ítem.
func TestUnItemPuedeDejarElMenuAbierto(t *testing.T) {
	disparador := NuevoBoton("Cuenta", nil)
	w := test.NewTempWindow(t, container.NewWithoutLayout(disparador))
	w.Resize(fyne.NewSize(800, 600))
	disparador.Resize(disparador.MinSize())
	queda := NuevoElementoMenu("Ausente", iconos.Ninguno, nil)
	queda.MantieneAbierto = true
	m := NuevoMenu(queda, NuevoElementoMenu("Mi cuenta", iconos.Ninguno, nil))
	m.Mostrar(disparador)
	t.Cleanup(m.Cerrar)
	queda.Tapped(nil)
	if !m.Abierto() {
		t.Error("el ítem que mantiene abierto ha cerrado el menú")
	}
}

// TestLaOpcionEnfocadaConElTecladoLlevaElAro: la opción de menú enfocada con el teclado
// lleva el aro global de theme.css —2 px de `focus-ring`, pegado y con la esquina del
// ítem— además del fondo de `hover`: es lo que la distingue de la que está bajo el
// puntero. Abierto con el ratón, la primera opción recibe el foco sin fondo ni aro, y la
// primera tecla lo enciende (medido en Chromium el 2026-10-05: `:focus-visible` hereda
// la modalidad cuando el foco lo mueve el código).
//
// VISTO EN ROJO sin el aro en el renderer, con `sinAro` siempre en falso y con el fondo
// atado a `enfocado` en vez de a `focoVisible`.
func TestLaOpcionEnfocadaConElTecladoLlevaElAro(t *testing.T) {
	hoja := webref.Read(t, "apps/frontend/packages/config/src/theme.css")
	fuente := webref.Read(t, "apps/frontend/packages/ui/src/components/Dropdown.tsx")
	for _, c := range []struct{ donde, trozo string }{
		{hoja, ":where(:focus-visible) {\n  outline: none;\n  box-shadow: 0 0 0 2px var(--sytue-focus-ring);\n}"},
		{fuente, `'flex w-full items-center gap-2 rounded-md text-left transition-colors duration-150',`},
		{fuente, `'active:bg-border focus-visible:outline-none',`},
		{fuente, `: 'text-text hover:bg-bg-subtle focus-visible:bg-bg-subtle',`},
	} {
		if !strings.Contains(c.donde, c.trozo) {
			t.Errorf("la web ya no dice %q: el aro de la opción de menú de aquí ha dejado de ser el suyo", c.trozo)
		}
	}

	disparador := NuevoBoton("Más", nil)
	w := test.NewTempWindow(t, container.NewWithoutLayout(disparador))
	w.Resize(fyne.NewSize(800, 600))
	disparador.Resize(disparador.MinSize())
	c := w.Canvas()
	abrir := func() (m *Menu, uno, dos *ElementoMenu) {
		uno = NuevoElementoMenu("Abrir", iconos.Ninguno, nil)
		dos = NuevoElementoMenu("Renombrar", iconos.Ninguno, nil)
		m = NuevoMenu(uno, dos)
		m.Mostrar(disparador)
		return m, uno, dos
	}
	// El aro es el abierto de `ui.Aro`: cuatro franjas por fuera de la caja.
	aro := func(e *ElementoMenu) *Aro { return test.WidgetRenderer(e).(*elementoRenderer).aro }
	anillo := func(e *ElementoMenu) fyne.CanvasObject { return aro(e).Objetos()[0] }
	fondo := func(e *ElementoMenu) color.NRGBA { f, _, _ := e.colores(tema.Claro); return f }
	encima := tema.Color("menu-hover", tema.Claro)

	// Con el ratón: nada tenía el foco.
	m, uno, dos := abrir()
	if c.Focused() != fyne.Focusable(uno) || anillo(uno).Visible() || fondo(uno) != (color.NRGBA{}) {
		t.Errorf("abierto con el ratón, la primera opción (foco en %T) lleva aro=%v y fondo %v; recibe el foco sin nada", c.Focused(), anillo(uno).Visible(), fondo(uno))
	}
	// Cualquier tecla enciende el foco visible, aunque no lo mueva.
	uno.TypedKey(&fyne.KeyEvent{Name: fyne.KeyLeft})
	if !anillo(uno).Visible() || fondo(uno) != encima {
		t.Errorf("tras una tecla, la opción enfocada lleva aro=%v y fondo %v; lleva los dos", anillo(uno).Visible(), fondo(uno))
	}
	uno.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown})
	a := anillo(dos)
	if c.Focused() != fyne.Focusable(dos) || !a.Visible() || fondo(dos) != encima || anillo(uno).Visible() {
		t.Fatalf("tras la flecha el foco está en %T, con aro=%v y fondo %v; la segunda opción lleva los dos y la primera ninguno", c.Focused(), a.Visible(), fondo(dos))
	}
	// 2 px de `focus-ring` por fuera de la caja, pegados y con su esquina: la franja de
	// arriba sobresale 2 por cada lado y lleva las dos esquinas.
	if x := aro(dos); !x.abierto || x.sep != SinHueco || x.tinta != tema.Color("focus-ring", tema.Claro) || x.arriba != tema.RadioMD ||
		a.Position() != fyne.NewPos(-2, -2) || a.Size() != fyne.NewSize(dos.Size().Width+4, 2+tema.RadioMD) {
		t.Errorf("el aro (abierto=%v, hueco %v, %v, esquina %v) tiene su franja de arriba en %v y de %v; son 2 px de `focus-ring` alrededor de la opción (%v)",
			x.abierto, x.sep, x.tinta, x.arriba, a.Position(), a.Size(), dos.Size())
	}
	// Y cabe en el `p-1` del panel: no lo recorta.
	abs := fyne.CurrentApp().Driver().AbsolutePositionForObject
	if abs(dos).X-2 < abs(m).X+tema.Borde || abs(dos).Y+dos.Size().Height+2 > abs(m).Y+m.Size().Height-tema.Borde {
		t.Errorf("el aro de la opción (%v, %v) se sale del panel (%v, %v)", abs(dos), dos.Size(), abs(m), m.Size())
	}
	m.Cerrar()

	// Con el teclado: el disparador tenía el foco.
	c.Focus(disparador)
	m, uno, dos = abrir()
	if !anillo(uno).Visible() || fondo(uno) != encima {
		t.Errorf("abierto con el teclado, la primera opción lleva aro=%v y fondo %v; lleva los dos", anillo(uno).Visible(), fondo(uno))
	}
	// El puntero sobre otra le pone su fondo, y el aro sigue en la enfocada.
	dos.MouseIn(nil)
	if anillo(dos).Visible() || fondo(dos) != encima || !anillo(uno).Visible() {
		t.Errorf("con el puntero en la segunda: su aro=%v, su fondo %v y el aro de la enfocada=%v; el aro es solo del foco", anillo(dos).Visible(), fondo(dos), anillo(uno).Visible())
	}
	m.Cerrar()
}

// TestEscapeDevuelveElFocoAlDisparador: en Dropdown.tsx Escape cierra y enfoca el
// disparador, aunque el foco estuviera en otro sitio al abrir. VISTO EN ROJO con el
// foco devuelto a lo de antes.
func TestEscapeDevuelveElFocoAlDisparador(t *testing.T) {
	antes, disparador := NuevoBoton("Antes", nil), NuevoBoton("Más", nil)
	w := test.NewTempWindow(t, container.NewWithoutLayout(antes, disparador))
	w.Resize(fyne.NewSize(800, 600))
	antes.Resize(antes.MinSize())
	disparador.Resize(disparador.MinSize())
	disparador.Move(fyne.NewPos(200, 0))
	w.Canvas().Focus(antes)
	item := NuevoElementoMenu("Renombrar", iconos.Ninguno, nil)
	m := NuevoMenu(item)
	m.Mostrar(disparador)
	item.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if m.Abierto() || w.Canvas().Focused() != fyne.Focusable(disparador) {
		t.Errorf("Escape: abierto=%v y foco en %T; cierra y va al disparador", m.Abierto(), w.Canvas().Focused())
	}
}

// TestElMenuPuedeTenerSuAncho: el «…» de un mensaje de Sytue Rooms mide 232 fijos
// (`MENU_W`). VISTO EN ROJO sin `Menu.Ancho`: medía su natural.
func TestElMenuPuedeTenerSuAncho(t *testing.T) {
	m := NuevoMenu(NuevoElementoMenu("Fijar", iconos.Ninguno, nil))
	m.Ancho = 232
	if got := MedidaMenu(m, fyne.NewSize(1280, 800)).Width; got != 232 {
		t.Errorf("el menú mide %v y con su ancho propio son 232", got)
	}
}

// TestElTabSaleDelMenuPorSuUltimoItem: en la web los ítems son `<button>` corrientes y el
// panel no atrapa el foco; en Fyne la capa lo atrapa y da la vuelta. Desde el último ítem
// el Tab cierra el menú y sigue tras el disparador; en medio va de ítem en ítem, que ya lo
// hace la capa. VISTO EN ROJO sin `AcceptsTab`: el Tab volvía al primer ítem.
func TestElTabSaleDelMenuPorSuUltimoItem(t *testing.T) {
	disparador, despues := NuevoBoton("Más", nil), NuevoBoton("Después", nil)
	w := test.NewTempWindow(t, container.NewWithoutLayout(disparador, despues))
	w.Resize(fyne.NewSize(800, 600))
	disparador.Resize(disparador.MinSize())
	despues.Resize(despues.MinSize())
	despues.Move(fyne.NewPos(200, 0))
	uno, dos := NuevoElementoMenu("Renombrar", iconos.Ninguno, nil), NuevoElementoMenu("Borrar", iconos.Ninguno, nil)
	m := NuevoMenu(uno, dos)
	m.Mostrar(disparador)
	t.Cleanup(m.Cerrar)
	if uno.AcceptsTab() {
		t.Error("en medio del menú el Tab lo mueve la capa, no el ítem")
	}
	w.Canvas().Focus(dos)
	if !dos.AcceptsTab() {
		t.Fatal("el último ítem no se queda el Tab: la capa lo devolvería al primero")
	}
	dos.TypedKey(&fyne.KeyEvent{Name: fyne.KeyTab})
	if m.Abierto() {
		t.Error("el Tab desde el último ítem no cierra el menú")
	}
	if got := w.Canvas().Focused(); got != fyne.Focusable(despues) {
		t.Errorf("tras salir del menú el foco está en %T y va a lo siguiente al disparador", got)
	}
}

// TestElItemCambiaDeColorEnCientoCincuenta: `transition-colors duration-150` de
// MenuItem. El ítem nace con su color y, al pasar el ratón, va al del `hover` con una
// animación (en el driver de pruebas, un único fotograma a p = 1). VISTO EN ROJO con el
// `Pintar` directo que había: no quedaba animación que parar.
func TestElItemCambiaDeColorEnCientoCincuenta(t *testing.T) {
	e := NuevoElementoMenu("Renombrar", iconos.Ninguno, nil)
	r := test.WidgetRenderer(e).(*elementoRenderer)
	if r.fondo.anim != nil {
		t.Error("el ítem nace animando: su color de reposo se pone sin transición")
	}
	e.MouseIn(nil)
	if r.fondo.anim == nil {
		t.Fatal("al pasar el ratón el fondo salta: falta la transición de 150 ms")
	}
	if got, quiero := r.fondo.visto, tema.Color("menu-hover", tema.Claro); got != quiero {
		t.Errorf("al acabar la transición el fondo es %v y el de `hover` es %v", got, quiero)
	}
}

// TestLaTransicionMezclaEnPremultiplicado: CSS interpola `transition-colors` con el alfa
// premultiplicado, así que de `transparent` a un velo el tono NO pasa por el negro.
func TestLaTransicionMezclaEnPremultiplicado(t *testing.T) {
	velo := color.NRGBA{R: 200, G: 100, B: 50, A: 255}
	medio := mezclaColor(color.NRGBA{}, velo, 0.5)
	if medio.R != 200 || medio.G != 100 || medio.B != 50 || medio.A < 127 || medio.A > 128 {
		t.Errorf("a mitad de camino desde transparente sale %v: el tono tiene que ser el del destino", medio)
	}
	if got := mezclaColor(velo, velo, 0.3); got != velo {
		t.Errorf("entre dos colores iguales sale %v", got)
	}
	if got := CurvaColores(0.5); got < 0.77 || got > 0.78 {
		t.Errorf("cubic-bezier(0.4, 0, 0.2, 1) a la mitad da %v y es 0,7756", got)
	}
}

// La opción vigente (`aria-current`) se marca con `menu-selected`, sin tic; pulsada
// conserva un relleno opaco que la separa del resto del menú.
func TestLaOpcionActualLlevaFondo(t *testing.T) {
	e := NuevoElementoMenu("En línea", iconos.Ninguno, nil)
	if f, _, _ := e.colores(tema.Claro); f.A != 0 {
		t.Fatalf("en reposo ya lleva fondo %v", f)
	}
	e.Actual = true
	if f, _, _ := e.colores(tema.Claro); f != tema.Color("menu-selected", tema.Claro) {
		t.Errorf("la actual lleva %v y no `menu-selected`", f)
	}
	e.pulsado = true
	if f, _, _ := e.colores(tema.Claro); f != tema.Color("menu-selected", tema.Claro) {
		t.Errorf("pulsada lleva %v y no `menu-selected`", f)
	}
}

// TestElTicDelMenuTieneSuTalla: el menú de fondos de Photos lleva el tic a 14. VISTO EN
// ROJO sin `TamTic`: medía los 16 de siempre.
func TestElTicDelMenuTieneSuTalla(t *testing.T) {
	e := NuevoElementoMenu("Blanco", iconos.Ninguno, nil)
	e.Marcada, e.TamTic = true, 14
	tam := e.MinSize()
	e.Resize(tam)
	r := test.WidgetRenderer(e).(*elementoRenderer)
	r.Layout(tam)
	if got := r.tic.Medida(); got != fyne.NewSize(14, 14) {
		t.Errorf("el tic mide %v y se pidió a 14", got)
	}
}

// ArchivoMenu coloca su botón h-6 dentro de un contenedor h-full de 31 píxeles.
// El panel usa top-full del contenedor + mt-1, pero Escape devuelve el foco al botón.
func TestElAnclaDelMenuSoloCambiaSuGeometria(t *testing.T) {
	boton := NuevoBoton("Archivo", nil).Con(Secundario, TallaXS)
	boton.Resize(fyne.NewSize(68, 24))
	boton.Move(fyne.NewPos(0, 3.5))
	ancla := container.NewWithoutLayout(boton)
	ancla.Resize(fyne.NewSize(68, 31))
	ancla.Move(fyne.NewPos(40, 60))
	w := test.NewTempWindow(t, container.NewWithoutLayout(ancla))
	w.SetPadded(false)
	w.Resize(fyne.NewSize(800, 400)) // de 768 para arriba: colgado de su disparador
	menu := NuevoMenu(NuevoElementoMenu("Descargar", iconos.Ninguno, func() {}))
	menu.Ancla, menu.AlInicio, menu.Separacion = ancla, true, 4
	menu.Mostrar(boton)
	// La posición depende del ancla; no de y63,5 + h24 del botón.
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(menu)
	if pos != fyne.NewPos(40, 95) {
		t.Errorf("el panel está en %v; top-full del ancla + mt-1 es (40,95)", pos)
	}
	if menu.disparador != fyne.CanvasObject(boton) {
		t.Error("el ancla ha sustituido al disparador accesible")
	}
	menu.cerrarConEscape()
	if w.Canvas().Focused() != fyne.Focusable(boton) {
		t.Error("Escape no devuelve el foco al botón")
	}
	if menu.Abierto() {
		t.Error("Escape ha dejado abierto el menú")
	}
	// Sin ancla ni separación propia sigue el contrato común: bottom del botón +6.
	menu.Ancla, menu.Separacion = nil, 0
	menu.Mostrar(boton)
	pos = fyne.CurrentApp().Driver().AbsolutePositionForObject(menu)
	if pos != fyne.NewPos(40, 93.5) {
		t.Errorf("sin ancla el menú está en %v; antes estaba en (40,93.5)", pos)
	}
	menu.Cerrar()
}
