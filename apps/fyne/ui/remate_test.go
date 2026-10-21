// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image"
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	fcanvas "fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/software"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Los guardianes de los defectos que las apps encontraron USANDO el catálogo. Todos
// fallaban antes del arreglo; cada uno dice con qué se vio en rojo.

// TestLaSeccionPlegadaNoPintaSusFilas es el fallo de la lista de ajustes de Sytue Mail:
// plegar la sección redimensionaba las filas a 0×0, y un `canvas.Text` a cero PINTA
// IGUAL —el pintor no recorta un objeto a su caja—, así que la lista se veía entera
// debajo del rótulo plegado.
//
// Se mide contando tinta bajo el encabezado, no preguntándole al widget si está
// plegado: esa comprobación pasaba ya cuando el fallo existía.
func TestLaSeccionPlegadaNoPintaSusFilas(t *testing.T) {
	fila := func(s string) fyne.CanvasObject {
		c := Texto(s, tema.TextoBase, Normal, tema.Color("text", tema.Claro))
		c.Resize(c.MinSize())
		return c
	}
	monta := func(abierta bool, filas ...fyne.CanvasObject) int {
		s := &SeccionLateral{Titulo: "Ajustes", AlPlegar: func() {}, Abierta: abierta, Filas: filas}
		s.ExtendBaseWidget(s)
		s.Resize(fyne.NewSize(240, MedidaSeccion(s).Height))
		return pixelesPintados(pinta(t, 240, 140, s))
	}
	con := []fyne.CanvasObject{fila("Carpetas"), fila("Filtros"), fila("Firmas")}

	// La referencia es la MISMA sección sin filas: una plegada tiene que pintar
	// exactamente lo mismo que si no las tuviera.
	//
	// No vale mirar «debajo del rótulo»: las filas de una sección plegada no se
	// colocan, así que se quedan en (0,0) y pintan ENCIMA del encabezado — que es por
	// donde este guardián se escapó la primera vez que se escribió.
	sinFilas := monta(false)
	plegada := monta(false, con...)
	abierta := monta(true, fila("Carpetas"), fila("Filtros"), fila("Firmas"))

	if plegada != sinFilas {
		t.Errorf("plegada pinta %d píxeles y sin filas %d: las filas siguen ahí", plegada, sinFilas)
	}
	if abierta <= plegada {
		t.Errorf("abierta pinta %d y plegada %d: el guardián no está mirando donde cree", abierta, plegada)
	}
}

// TestElEstadoVacioSeCentraEnVertical es el `justify-center` de EmptyState con el
// `h-full` que le da quien lo coloca: el bloque va EN MEDIO del hueco, no pegado
// arriba.
func TestElEstadoVacioSeCentraEnVertical(t *testing.T) {
	v := NuevoVacio("No hay nada", "")
	const alto, ancho = 400, 300
	v.Resize(fyne.NewSize(ancho, alto))
	img := pinta(t, ancho, alto, v)

	y0, y1 := franjaConTinta(img)
	if y0 < 0 {
		t.Fatal("el estado vacío no ha pintado nada")
	}
	// El centro de lo pintado tiene que caer en el centro de la caja, no arriba.
	centro := float32(y0+y1) / 2 / escala
	if d := centro - alto/2; d < -3 || d > 3 {
		t.Errorf("el bloque está centrado en y=%.1f y la caja de %d lo quiere en %d", centro, alto, alto/2)
	}
}

// TestElEstadoVacioColocaUnaAccionPuestaDespues es el fallo que midió Sytue Mail:
// cambiarle el título o la acción a un estado vacío YA MONTADO dejaba el botón fuera
// del layout, pintado en la esquina de la pantalla.
func TestElEstadoVacioColocaUnaAccionPuestaDespues(t *testing.T) {
	v := NuevoVacio("Bandeja vacía", "Nada que leer por ahora.")
	const alto, ancho = 300, 320
	v.Resize(fyne.NewSize(ancho, alto))
	pinta(t, ancho, alto, v) // monta el renderer

	b := NuevoBoton("Redactar", nil)
	v.Accion = b
	v.Refresh()
	pinta(t, ancho, alto, v)

	if b.Position().X <= 0 || b.Position().Y <= 0 {
		t.Errorf("la acción se ha quedado en %v: nadie la ha colocado al aparecer", b.Position())
	}
	// Y centrada en horizontal, que es el `items-center`.
	if quiero := (ancho - b.Size().Width) / 2; b.Position().X < quiero-2 || b.Position().X > quiero+2 {
		t.Errorf("la acción está en x=%v y centrada cae en %v", b.Position().X, quiero)
	}
}

// franjaConTinta devuelve la primera y la última fila con algo pintado.
func franjaConTinta(img image.Image) (int, int) {
	b := img.Bounds()
	y0, y1 := -1, -1
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if p := px(img, x, y); p.R < 250 || p.G < 250 || p.B < 250 {
				if y0 < 0 {
					y0 = y
				}
				y1 = y
				break
			}
		}
	}
	return y0, y1
}

// TestRepartirSinAnchoNoParteElParrafo: llamarlo antes de saber el ancho daba una
// palabra por línea y un panel de veinte renglones. Un ancho desconocido no es un
// ancho de cero.
func TestRepartirSinAnchoNoParteElParrafo(t *testing.T) {
	const p = "Arrastra aquí los ficheros que quieras subir a tu unidad"
	for _, cabe := range []float32{0, -1, -500} {
		if got := Repartir(p, tema.TextoBase, Normal, cabe); len(got) != 1 {
			t.Errorf("Repartir con cabe=%v da %d líneas y sin ancho no se parte", cabe, len(got))
		}
	}
	// Con un ancho de verdad sí parte, o esto no estaría midiendo nada.
	if got := Repartir(p, tema.TextoBase, Normal, 80); len(got) < 2 {
		t.Errorf("con 80 px de ancho salen %d líneas y el párrafo no cabe en una", len(got))
	}
}

// TestElAroVaDelanteDeTodo es el fallo de la tira de miniaturas de Sytue Diapos: el
// anillo es un rectángulo RELLENO más grande que el control, así que puesto al final
// de `Objects()` lo tapa entero.
func TestElAroVaDelanteDeTodo(t *testing.T) {
	a := NuevoAro(tema.RadioMD, ConHueco, tema.Claro)
	caja := fcanvas.NewRectangle(tema.Color("surface", tema.Claro))
	objs := a.Antes(caja)
	if objs[len(objs)-1] != fyne.CanvasObject(caja) {
		t.Error("Antes no ha dejado el contenido al final: el aro lo taparía")
	}
	for _, o := range a.Objetos() {
		if objs[0] == o {
			return
		}
	}
	t.Error("Antes no ha puesto el aro el primero")
}

// TestLaColumnaCentraCuandoSeLePide es la sorpresa de Sytue Diapos: por defecto la
// columna ESTIRA (`items-stretch`, el de CSS) y una miniatura de 128 salía a 145.
func TestLaColumnaCentraCuandoSeLePide(t *testing.T) {
	hijo := func() fyne.CanvasObject {
		r := fcanvas.NewRectangle(tema.Color("primary", tema.Claro))
		r.SetMinSize(fyne.NewSize(128, 20))
		return r
	}
	caja := fyne.NewSize(200, 40)

	estira := hijo()
	Columna{}.Layout([]fyne.CanvasObject{estira}, caja)
	if estira.Size().Width != 200 {
		t.Errorf("por defecto la columna deja el hijo a %v y `items-stretch` lo estira a 200", estira.Size().Width)
	}
	centra := hijo()
	Columna{Encaja: true}.Layout([]fyne.CanvasObject{centra}, caja)
	if centra.Size().Width != 128 {
		t.Errorf("con Encaja el hijo mide %v y su mínimo son 128", centra.Size().Width)
	}
	if centra.Position().X != 36 { // (200-128)/2
		t.Errorf("con Encaja el hijo está en x=%v y centrado cae en 36", centra.Position().X)
	}
}

// TestLaCasillaParcialPintaLaRaya es el `indeterminate` del «seleccionar todo».
//
// Se mide contando tinta en las DOS FILAS CENTRALES del cuadradito y normalizando por
// la distancia fondo→tinta, NUNCA por 255: la raya son 1,17 px de alto y nunca llega
// al blanco puro, así que «contar píxeles blancos» da cero con la raya bien pintada.
// Es la receta que dejó medida el agente de Sytue Drive.
func TestLaCasillaParcialPintaLaRaya(t *testing.T) {
	// Cuánta MARCA hay en las dos filas centrales del cuadradito. Se normaliza por la
	// distancia fondo→tinta y NUNCA por 255: la raya son 1,17 px de alto y nunca llega
	// al blanco puro, así que «contar píxeles blancos» da cero con la raya bien
	// pintada. Es la receta que dejó medida el agente de Sytue Drive.
	marca := func(e *Eleccion, fondo, tinta_ color.NRGBA) float64 {
		e.SinEtiqueta = true
		e.ExtendBaseWidget(e)
		m := e.MinSize()
		e.Resize(m)
		img := pinta(t, m.Width+6, m.Height+6, e)
		y := int(m.Height * escala / 2)
		var suma float64
		for dy := range 2 {
			for x := range int(LadoEleccion * escala) {
				p := px(img, x, y-1+dy)
				d := float64(int(p.R)-int(fondo.R)) / float64(int(tinta_.R)-int(fondo.R))
				if d > 0.05 {
					suma += d
				}
			}
		}
		return suma
	}
	// Marcada y parcial pintan sobre el acento sólido; sin marcar, sobre la superficie.
	acento := tema.Color("primary", tema.Claro)
	blanca := tema.Color("on-primary", tema.Claro)
	superficie := tema.Color("surface", tema.Claro)
	borde := CasillaNativa(tema.Claro).Borde

	vacia := marca(&Eleccion{}, superficie, borde)
	parcial := marca(&Eleccion{Parcial: true}, acento, blanca)
	tic := marca(&Eleccion{Marcada: true}, acento, blanca)
	ambas := marca(&Eleccion{Marcada: true, Parcial: true}, acento, blanca)

	// 1. La raya cruza la banda central de lado a lado; el tic solo la roza.
	if parcial <= tic {
		t.Errorf("la banda central pinta %.1f con la raya y %.1f con el tic: la raya no es una raya", parcial, tic)
	}
	// 2. Y hay raya de verdad: más de media anchura del cuadradito, a doble escala.
	if parcial < float64(LadoEleccion) {
		t.Errorf("la raya pinta %.1f y cruzando el cuadradito de %v tendría que pasar de eso", parcial, LadoEleccion)
	}
	// 3. Gana al marcado, como `indeterminate` en el DOM.
	if ambas < parcial-2 || ambas > parcial+2 {
		t.Errorf("marcada Y parcial pinta %.1f y solo parcial %.1f: el tercer estado no gana", ambas, parcial)
	}
	// 4. Y la caja sin marcar no tiene ninguna de las dos por dentro: los dos lados
	//    del contorno y nada más. Sin esto, el guardián pasaría con las tres iguales.
	if vacia > 8 {
		t.Errorf("la casilla sin marcar pinta %.1f en su banda central y ahí solo va el contorno", vacia)
	}
}

// TestLaTarjetaLlevaSuSombraYNoOcupaSitio es `shadow-[var(--shadow-sm)]` de Card.tsx:
// la sombra se pinta FUERA de la caja y NO cambia la medida, exactamente como un
// `box-shadow` en CSS.
func TestLaTarjetaLlevaSuSombraYNoOcupaSitio(t *testing.T) {
	cuerpo := fcanvas.NewRectangle(tema.Color("surface", tema.Claro))
	cuerpo.SetMinSize(fyne.NewSize(120, 40))
	c := NuevaTarjeta("", cuerpo)
	m := MedidaTarjeta("", cuerpo)
	if got := c.MinSize(); got != m {
		t.Errorf("MinSize() = %v y MedidaTarjeta = %v: la sombra no puede ocupar layout", got, m)
	}

	// Se pinta con hueco alrededor y se mira JUSTO debajo de la tarjeta: ahí tiene que
	// haber tinta, que es la sombra desbordando.
	const margen = 20
	c.Move(fyne.NewPos(margen, margen))
	c.Resize(m)
	img := pinta(t, m.Width+2*margen, m.Height+2*margen, c)

	x := int((margen + m.Width/2) * escala)
	bajo := int((margen + m.Height + 2) * escala)
	if p := px(img, x, bajo); p.R > 252 && p.G > 252 && p.B > 252 {
		t.Errorf("dos píxeles por debajo de la tarjeta el lienzo sigue blanco (%v): no hay sombra", p)
	}
	// Y la plana no la lleva.
	llana := &Tarjeta{Cuerpo: fcanvas.NewRectangle(tema.Color("surface", tema.Claro)), Plana: true}
	llana.ExtendBaseWidget(llana)
	llana.Move(fyne.NewPos(margen, margen))
	llana.Resize(m)
	img = pinta(t, m.Width+2*margen, m.Height+2*margen, llana)
	if p := px(img, x, bajo); p.R < 252 || p.G < 252 || p.B < 252 {
		t.Errorf("la tarjeta plana pinta %v debajo y `raised={false}` quiere el lienzo limpio", p)
	}
}

// TestLaTarjetaSinRellenoCuelgaElCuerpoDelBorde: el `<dl>` de ContactDetail.tsx va
// dentro de `<Card>` sin CardBody, así que el cuerpo empieza en el píxel de después del
// borde y la tarjeta mide lo que él más los dos bordes. Con relleno, 14 más por lado.
func TestLaTarjetaSinRellenoCuelgaElCuerpoDelBorde(t *testing.T) {
	cuerpo := fcanvas.NewRectangle(tema.Color("surface", tema.Claro))
	cuerpo.SetMinSize(fyne.NewSize(120, 40))
	c := NuevaTarjeta("", cuerpo)
	c.SinRelleno = true
	if got, quiero := c.MinSize(), fyne.NewSize(120+2*tema.Borde, 40+2*tema.Borde); got != quiero {
		t.Errorf("sin relleno la tarjeta mide %v y debería %v", got, quiero)
	}
	c.Resize(c.MinSize())
	if got := cuerpo.Position(); got != fyne.NewPos(tema.Borde, tema.Borde) {
		t.Errorf("el cuerpo empieza en %v y sin relleno va pegado al borde", got)
	}
	if con := MedidaTarjeta("", cuerpo); con.Width != 120+2*tema.Borde+2*tema.Espacio(4) {
		t.Errorf("con relleno la tarjeta mide %v: el defecto sigue siendo el de CardBody", con)
	}
}

// TestNingunWidgetNaceInvisible es el guardián de la regla 7 del paquete, y existe
// porque DOS widgets del catálogo la incumplían a la vez sin que nadie lo notara.
//
// Fyne no llama al `Refresh` del renderer antes del primer pintado: llama a `Layout` y
// pinta lo que haya. Un widget que se complete en `Refresh` sale invisible la primera
// vez y se arregla solo en cuanto algo toque la pantalla — que es exactamente por qué
// no se ve desarrollando y sí se ve al abrir la app.
//
// Se monta cada widget dos veces: una se pinta tal cual y la otra tras un `Refresh`.
// Si pintan distinto, el widget nace a medias.
//
// Y se REDIMENSIONA antes de pintar, que no es un detalle del montaje: un widget que
// envuelva `widget.Entry` sin `r.Refresh()` no nace invisible, **revienta con SIGSEGV
// en el primer `Resize`** (`internal/widget/scroller.go`), porque `Entry` materializa
// su desplazador en su propio `Refresh`. Contando píxeles eso no se ve: se ve porque
// el test se cae. Si alguien quita el `Resize` de aquí, ese caso deja de mirarse.
func TestNingunWidgetNaceInvisible(t *testing.T) {
	casos := []struct {
		que   string
		monta func() fyne.CanvasObject
	}{
		{"Boton", func() fyne.CanvasObject { return NuevoBoton("Guardar", nil) }},
		{"Boton fantasma", func() fyne.CanvasObject {
			return NuevoBoton("Cancelar", nil).Con(Fantasma, TallaSM)
		}},
		{"Entrada con marca de agua", func() fyne.CanvasObject { return NuevaEntrada("Buscar contactos") }},
		{"AreaTexto", func() fyne.CanvasObject { return NuevaAreaTexto("Escribe una nota") }},
		{"Pestana", func() fyne.CanvasObject { return NuevaPestana("Colaboración", nil) }},
		{"Casilla", func() fyne.CanvasObject { return NuevaCasilla("Recordarme", nil) }},
		{"Tarjeta", func() fyne.CanvasObject { return NuevaTarjeta("Resumen", nil) }},
		{"Vacio", func() fyne.CanvasObject { return NuevoVacio("Nada aquí", "Prueba a subir algo.") }},
		{"Esqueleto", func() fyne.CanvasObject { return NuevoEsqueleto(80, 12) }},
		{"BarraProgreso", func() fyne.CanvasObject { return NuevaBarraProgreso("Subiendo", 0.4) }},
		{"Deslizador", func() fyne.CanvasObject { return NuevoDeslizador(-1, 1, 0, nil) }},
		{"AyudaEnLinea", func() fyne.CanvasObject { return NuevaAyudaEnLinea("Se borra en 30 días.") }},
		{"PistaCampo", func() fyne.CanvasObject {
			return NuevaPistaCampo("Ese espacio no existe.").ConTono(AyudaPeligro)
		}},
		{"BarraDeLote", func() fyne.CanvasObject {
			return NuevaBarraDeLote("3 seleccionados", "Acciones", "Quitar",
				[]*Boton{NuevoBoton("Papelera", nil).Con(Peligro, TallaSM)}, nil)
		}},
		{"Enlace", func() fyne.CanvasObject {
			return NuevoEnlace("Ver el calendario", tema.TextoSM, Medio, "primary-text", nil)
		}},
		{"Parrafo", func() fyne.CanvasObject {
			return NuevoParrafo("Se guarda como texto alternativo de la foto.",
				tema.TextoSM, Normal, "text", tema.Claro)
		}},
		{"BarraMovil", func() fyne.CanvasObject {
			b := NuevaBarraMovil()
			b.Titulo, b.EnDetalle = "Contactos", true
			return b
		}},
		{"BotonFlotante", func() fyne.CanvasObject { return NuevoBotonFlotante("Nuevo contacto", iconos.Mas, nil) }},
		{"SelectorAvatar", func() fyne.CanvasObject { return NuevoSelectorAvatar(NuevoAvatar("Ana Ruiz").Con(AvatarLG)) }},
		{"FilaSelector", func() fyne.CanvasObject {
			return filaDePrueba(api.Elegible{ID: "c1", Nombre: "General", Meta: "Anuncios", URL: "u"})
		}},
	}
	for _, c := range casos {
		crudo, refrescado := c.monta(), c.monta()
		m := crudo.MinSize()
		if m.Width <= 0 || m.Height <= 0 {
			m = fyne.NewSize(200, 40)
		}
		crudo.Resize(m)
		refrescado.Resize(m)
		sin := pixelesPintados(pinta(t, m.Width+6, m.Height+6, crudo))
		refrescado.Refresh()
		con := pixelesPintados(pinta(t, m.Width+6, m.Height+6, refrescado))
		if sin != con {
			t.Errorf("%s pinta %d píxeles recién montado y %d tras un Refresh: nace a medias, "+
				"y Fyne no repinta antes del primer pintado. `CreateRenderer` tiene que "+
				"terminar con `r.Refresh()` (regla 7 de ui.go).\n"+
				"  Y si lo que ves no es este error sino un SIGSEGV en `scroller.go`, es lo "+
				"mismo: un widget con `widget.Entry` dentro y sin ese `Refresh` no nace "+
				"invisible, se cae al redimensionarse.", c.que, sin, con)
		}
		if con == 0 {
			t.Errorf("%s no pinta nada ni con Refresh: el guardián no está midiendo lo que cree", c.que)
		}
	}
}

// TestElCampoPuedeSerElFocoDeUnDialogo es el defecto que midió Sytue Drive: lo
// enfocable era el EDITOR INTERNO, que vive dentro de un `container.ThemeOverride` y
// al que no llega nadie de fuera, así que un campo no podía declararse como
// `Dialogo.Foco` y el diálogo «Nueva carpeta» abría con el foco en el aspa.
func TestElCampoPuedeSerElFocoDeUnDialogo(t *testing.T) {
	e := NuevaEntrada("Nombre de la carpeta")
	var c Campo = e // no compila si Entrada no satisface fyne.Focusable + EsCampo

	d := &Dialogo{Titulo: "Nueva carpeta", Cuerpo: e, Foco: c, RotuloCerrar: "Cerrar"}
	d.ExtendBaseWidget(d)
	if got := d.focoInicial(); got != fyne.Focusable(e) {
		t.Errorf("el foco inicial es %T y se pidió el campo", got)
	}

	// Y sin declararlo, el recorrido tiene que encontrarlo solo: es el primer campo
	// del cuerpo, que es el peldaño de Modal.tsx.
	d2 := &Dialogo{Titulo: "Nueva carpeta", Cuerpo: NuevaEntrada("Nombre"), RotuloCerrar: "Cerrar"}
	d2.ExtendBaseWidget(d2)
	if got := d2.focoInicial(); got == fyne.Focusable(d2) {
		t.Error("el diálogo se ha quedado el foco: no ha reconocido el campo de su cuerpo")
	}

	// Y el foco de verdad llega al editor, no se queda en la cáscara.
	e.FocusGained()
	if !e.enfocado {
		t.Error("FocusGained no ha llegado al editor: la cáscara se ha comido el foco")
	}
}

// TestElAroNoTineElCentroDeUnControlSinCaja: el anillo de foco es un `box-shadow`, que
// solo pinta FUERA de la caja. Un control transparente en reposo enfocado tiene que
// pintar por dentro exactamente lo mismo que sin foco; con el aro relleno, cada uno de
// estos era un bloque del color del anillo con el rótulo encima.
//
// Se pinta sobre un gris que no es ningún token: sobre blanco, el anillo blanco del
// menú de tinta (`chrome-focus`) no se vería y el caso pasaría sin mirar nada.
func TestElAroNoTineElCentroDeUnControlSinCaja(t *testing.T) {
	gris := color.NRGBA{R: 120, G: 130, B: 140, A: 255}
	casos := []struct {
		que   string
		monta func() fyne.Focusable
	}{
		{"Enlace", func() fyne.Focusable {
			return NuevoEnlace("Ver el calendario", tema.TextoSM, Medio, "primary-text", nil)
		}},
		{"Miga", func() fyne.Focusable { return NuevaMiga("Carpeta", nil) }},
		{"Pestana", func() fyne.Focusable { return NuevaPestana("Colaboración", nil) }},
		{"FilaLateral", func() fyne.Focusable { return NuevaFilaLateral("Bandeja", nil, nil) }},
		{"AccionLateral", func() fyne.Focusable { return NuevaAccionLateral("Nueva", nil, nil) }},
		{"BotonIcono", func() fyne.Focusable { return NuevoBotonIcono(iconos.Buscar, "Buscar", nil) }},
		{"Boton fantasma", func() fyne.Focusable { return NuevoBoton("Cancelar", nil).Con(Fantasma, TallaSM) }},
	}
	for _, c := range casos {
		sin, con := c.monta(), c.monta()
		m := sin.(fyne.CanvasObject).MinSize()
		for _, o := range []fyne.Focusable{sin, con} {
			o.(fyne.CanvasObject).Move(fyne.NewPos(8, 8))
			o.(fyne.CanvasObject).Resize(m)
		}
		con.FocusGained()
		a := pintaSobre(t, gris, m.Width+16, m.Height+16, sin.(fyne.CanvasObject))
		b := pintaSobre(t, gris, m.Width+16, m.Height+16, con.(fyne.CanvasObject))
		// Un píxel de dispositivo hacia dentro por cada lado —el borde de la caja es donde
		// empieza el anillo, y ahí el suavizado sí puede cambiar— y sin las cuatro
		// esquinas: fuera de la curva de una caja redondeada el anillo SÍ pinta.
		dentro, fuera := 0, 0
		x0, y0 := int(8*escala)+1, int(8*escala)+1
		x1, y1 := int((8+m.Width)*escala)-1, int((8+m.Height)*escala)-1
		k := int(min(m.Width, m.Height) / 2 * escala)
		for y := a.Bounds().Min.Y; y < a.Bounds().Max.Y; y++ {
			for x := a.Bounds().Min.X; x < a.Bounds().Max.X; x++ {
				if px(a, x, y) == px(b, x, y) {
					continue
				}
				caja := x >= x0 && x < x1 && y >= y0 && y < y1
				esquina := (x < x0+k || x >= x1-k) && (y < y0+k || y >= y1-k)
				switch {
				case caja && !esquina:
					dentro++
				case !caja:
					fuera++
				}
			}
		}
		if dentro != 0 {
			t.Errorf("%s enfocado cambia %d píxeles DENTRO de su caja: el aro le está "+
				"pintando el centro (le falta `Abierto()`)", c.que, dentro)
		}
		if fuera == 0 {
			t.Errorf("%s enfocado no pinta nada fuera de su caja: el guardián no está "+
				"viendo el aro", c.que)
		}
	}
}

// pintaSobre es pinta con otro fondo.
func pintaSobre(t *testing.T, fondo color.NRGBA, w, h float32, o fyne.CanvasObject) image.Image {
	t.Helper()
	f := fcanvas.NewRectangle(fondo)
	f.Resize(fyne.NewSize(w, h))
	c := software.NewCanvas()
	c.SetPadded(false)
	c.SetContent(container.NewWithoutLayout(f, o))
	c.SetScale(escala)
	c.Resize(fyne.NewSize(w, h))
	return c.Capture()
}
