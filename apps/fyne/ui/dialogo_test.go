// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"fmt"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	fcanvas "fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// cuerpoDe devuelve un cuerpo de prueba de un alto exacto, para poder hacer la cuenta
// del diálogo a mano. Un rectángulo NO sirve: su MinSize es cero.
func cuerpoDe(alto float32) fyne.CanvasObject {
	r := fcanvas.NewRectangle(tema.Color("bg-subtle", tema.Claro))
	r.SetMinSize(fyne.NewSize(100, alto))
	return r
}

// TestLasTallasSonLasDeModalTsx lee los cuatro anchos DEL FUENTE de la web, no de una
// tabla copiada aquí. Es la diferencia entre un guardián y un espejo: si alguien cambia
// `max-w-[560px]` en Modal.tsx, esto se pone rojo.
func TestLasTallasSonLasDeModalTsx(t *testing.T) {
	fuente := webref.Read(t, "apps/frontend/packages/ui/src/components/Modal.tsx")
	web := map[string]float32{}
	for _, m := range regexp.MustCompile(`(sm|md|lg|xl):\s*'max-w-\[(\d+)px]'`).FindAllStringSubmatch(fuente, -1) {
		v, err := strconv.ParseFloat(m[2], 32)
		if err != nil {
			t.Fatal(err)
		}
		web[m[1]] = float32(v)
	}
	if len(web) != 4 {
		t.Fatalf("de Modal.tsx salen %d tallas y son cuatro: revisar cómo se extraen", len(web))
	}
	for nombre, talla := range map[string]TallaDialogo{"sm": DialogoSM, "md": DialogoMD, "lg": DialogoLG, "xl": DialogoXL} {
		if anchosDialogo[talla] != web[nombre] {
			t.Errorf("la talla %s mide %v aquí y %v en Modal.tsx", nombre, anchosDialogo[talla], web[nombre])
		}
	}
	// Y el valor cero es `md`, como el `size = 'md'` de la firma.
	if anchosDialogo[TallaDialogo(0)] != web["md"] {
		t.Errorf("el valor cero de TallaDialogo mide %v y `md` son %v", anchosDialogo[0], web["md"])
	}
}

// La cabecera de Modal.tsx mide 61: `pt-5 pb-2`, la fila de 32 del aspa y el
// `border-b` de 1. La posición del filete pintado comprueba la misma cifra sin consultar
// su helper.
//
// Y el filete solo se ve cuando el cuerpo se desplaza: con una frase, tres bandas con dos
// líneas pesaban más que lo que decían. VISTO EN ROJO con el filete siempre pintado.
func TestLaCabeceraDelDialogoMide61YSuFileteSoloSaleSiDesplaza(t *testing.T) {
	tema.Elegir("oscuro")
	t.Cleanup(func() { tema.Elegir("") })
	const medidoEnChromium = 61
	if got := AltoCabeceraDialogo(); got != medidoEnChromium {
		t.Errorf("cabecera=%v y Chromium=%v", got, float32(medidoEnChromium))
	}
	tinta := BordeSobre("border-subtle", "panel-surface", tema.Oscuro)
	filete := func(cuerpo float32) (arriba, alto float32) {
		d := NuevoDialogo("Compartir «Presupuesto2026»", cuerpoDe(cuerpo))
		m := MedidaDialogo(d, fyne.NewSize(1280, 800))
		d.Resize(m)
		img := pinta(t, m.Width+20, m.Height+20, d)
		// Muestrea lejos del título y del contorno: el color de un filete puede
		// coincidir con el borde exterior al cambiar de paleta.
		y0, y1 := -1, -1
		for y := int(50 * escala); y < int(75*escala); y++ {
			if cerca(px(img, int(8*escala), y), tinta) {
				if y0 < 0 {
					y0 = y
				}
				y1 = y
			}
		}
		if y0 < 0 {
			return -1, 0
		}
		return float32(y0) / escala, float32(y1-y0+1) / escala
	}
	if arriba, alto := filete(2000); arriba != 61 || alto != 1 {
		t.Errorf("con el cuerpo desplazándose, filete de cabecera y=%v h=%v y Chromium 61/1", arriba, alto)
	}
	if arriba, _ := filete(40); arriba >= 0 {
		t.Errorf("con un cuerpo que cabe, el filete de cabecera se pinta en y=%v", arriba)
	}
}

// TestElPieMideLoQueMidaSuBotonera: los 60,5 px que se miden en producción son los de
// una botonera de talla `md`; con botones `sm` mide 57, que es lo que medía el de
// ConfirmDialog hasta que la web lo pasó a la talla por defecto (PR #836). El pie no es
// una constante, y escribirlo como tal es cómo se le mete un diálogo 3,5 px más alto a
// un tercio de la suite.
func TestElPieMideLoQueMidaSuBotonera(t *testing.T) {
	for _, c := range []struct {
		talla  Talla
		quiero float32
	}{
		{TallaMD, 65}, // `border-t` + `pt-2` + h-9 + `pb-5`
		{TallaSM, 61}, // `border-t` + `pt-2` + h-8 + `pb-5`
	} {
		pie := container.New(Fila{Hueco: HuecoPieDialogo},
			NuevoBoton("Cancelar", nil).Con(Fantasma, c.talla),
			NuevoBoton("Enviar", nil).Con(Primario, c.talla))
		if got := AltoPieDialogo(pie); got != c.quiero {
			t.Errorf("con botones de talla %d el pie mide %v y son %v", c.talla, got, c.quiero)
		}
	}
	if got := AltoPieDialogo(nil); got != 0 {
		t.Errorf("sin pie el diálogo se deja %v px de pie: en la web no hay nada", got)
	}
}

// TestLaConfirmacionLlevaElPieDeCualquierDialogo: con botones `sm` una confirmación se
// veía más pequeña que el formulario de al lado (28 contra 31,5 de botón; 57 contra
// 60,5 de pie). La web la igualó en el PR #836. VISTO EN ROJO con `TallaSM`: 57.
func TestLaConfirmacionLlevaElPieDeCualquierDialogo(t *testing.T) {
	d := Confirmacion("¿Mover a la papelera?", "Cancelar", "Mover a la papelera", true, nil, nil, nil)
	if got := AltoPieDialogo(d.Pie); got != 65 {
		t.Errorf("el pie de la confirmación mide %v y el de cualquier diálogo 65", got)
	}
}

// TestElDialogoSeAjustaAlContenidoYNoAlTope es EL guardián de esta familia, y está
// escrito invirtiendo el fallo que ya ocurrió: `max-h-[90%]` se tomó por el alto y una
// confirmación de dos líneas salía a pantalla completa, 690 px donde tocaban 176.
//
// Comprueba las dos mitades, porque una sola no basta:
//
//  1. Con poco contenido el panel mide lo SUYO, y bastante menos que el tope. Un
//     diálogo que devolviera el tope pasaría cualquier prueba que solo mirase «¿cabe?».
//  2. Con mucho, se queda EXACTAMENTE en el tope y ni un píxel más.
func TestElDialogoSeAjustaAlContenidoYNoAlTope(t *testing.T) {
	lienzo := fyne.NewSize(1280, 800)
	tope := FraccionAltoDialogo * (lienzo.Height - 2*MargenDialogo) // 0,9 × (800 − 28) = 694,8

	// Dos líneas de `text-base` en el cuerpo, y el pie de un ConfirmDialog.
	pie := container.New(Fila{Hueco: HuecoPieDialogo},
		NuevoBoton("Cancelar", nil).Con(Fantasma, TallaSM),
		NuevoBoton("Eliminar", nil).Con(Peligro, TallaSM))
	corto := NuevoDialogo("¿Eliminar la carpeta?", cuerpoDe(2*tema.TextoBase.Linea)).Con(DialogoSM, pie)

	// La cuenta se escribe entera, con los números de la web, para que se lea de dónde
	// sale cada sumando: borde + cabecera + (py-3 ×2 + dos líneas) + pie + borde.
	quiero := tema.Borde + 61 + (2*12 + 2*tema.TextoBase.Linea) + 61 + tema.Borde
	got := MedidaDialogo(corto, lienzo)
	if got.Height != quiero {
		t.Errorf("una confirmación de dos líneas mide %v de alto y la cuenta de la web da %v", got.Height, quiero)
	}
	if got.Height >= tope {
		t.Errorf("mide %v y el tope es %v: el diálogo se ha estirado hasta el tope, que es el fallo del port anterior", got.Height, tope)
	}
	if got.Width != 400 {
		t.Errorf("una talla `sm` mide %v de ancho y son 400", got.Width)
	}

	// Y ahora al revés: un cuerpo que no cabe se recorta EN el tope.
	largo := NuevoDialogo("Actividad", cuerpoDe(4000)).Con(DialogoLG, pie)
	if got := MedidaDialogo(largo, lienzo).Height; got != tope {
		t.Errorf("un cuerpo de 4000 px deja el diálogo en %v y `max-h-[90%%]` de 800 − 28 son %v", got, tope)
	}
}

// TestElDialogoEsWFullEnUnaVentanaEstrecha: desde 768 cabe cualquier talla con su `p-4`
// (la mayor, `lg`, pide 720 + 32), así que mide su `max-w`; por debajo, la hoja va a
// TODO el ancho y no a 390 menos `p-4`, que era la tarjeta de antes.
func TestElDialogoEsWFullEnUnaVentanaEstrecha(t *testing.T) {
	d := NuevoDialogo("Nueva nota", cuerpoDe(60)).Con(DialogoLG, nil)
	if got := MedidaDialogo(d, fyne.NewSize(Estrecho, 844)).Width; got != 720 {
		t.Errorf("a 768 de ancho el diálogo lg mide %v y son 720", got)
	}
	if got := MedidaDialogo(d, fyne.NewSize(390, 844)).Width; got != 390 {
		t.Errorf("en 390 de ancho la hoja mide %v y es a todo el ancho", got)
	}
}

// TestElPanelPintaLoQueDiceMedirDialogo: lo declarado y lo pintado son lo mismo.
func TestElPanelPintaLoQueDiceMedirDialogo(t *testing.T) {
	d := NuevoDialogo("Ajustes", cuerpoDe(80))
	m := MedidaDialogo(d, fyne.NewSize(1280, 800))
	d.Resize(m)
	img := pinta(t, m.Width+20, m.Height+20, d)
	// El borde delimita la caja completa también con superficies claras.
	x0, x1, y0, y1 := caja(img, BordeSobre("border", "panel-surface", tema.Claro))
	mismaCaja(t, "el panel", m, x0, x1, y0, y1)
}

// TestSinDialogoNoHayVeloQueEstorbe es la otra trampa ya pisada: un velo puesto siempre
// deja la aplicación entera sin poder pulsar, y en una captura no se ve.
//
// Está escrito invirtiéndolo: primero comprueba que NO hay ninguna capa, luego que al
// abrir hay una, y luego que al cerrar vuelve a no haberla.
func TestSinDialogoNoHayVeloQueEstorbe(t *testing.T) {
	w := test.NewTempWindow(t, fcanvas.NewRectangle(tema.Color("bg", tema.Claro)))
	w.Resize(fyne.NewSize(1280, 800))
	c := w.Canvas()

	if n := len(c.Overlays().List()); n != 0 {
		t.Fatalf("una ventana recién abierta ya tiene %d capas encima", n)
	}
	d := NuevoDialogo("Invitar a alguien", cuerpoDe(60))
	d.Mostrar(c)
	if n := len(c.Overlays().List()); n != 1 {
		t.Fatalf("con el diálogo abierto hay %d capas y tiene que haber una", n)
	}
	d.Cerrar()
	if n := len(c.Overlays().List()); n != 0 {
		t.Fatalf("al cerrar quedan %d capas: el velo se ha quedado puesto y nada se puede pulsar", n)
	}
}

// TestElVeloCierraYElPanelNo: el clic FUERA cierra, el clic DENTRO no. Sin el
// `Tapped` vacío del panel, el segundo también cerraría —el recorrido de Fyne seguiría
// bajando hasta el velo— y el diálogo se cerraría al pulsar en su propio fondo.
func TestElVeloCierraYElPanelNo(t *testing.T) {
	w := test.NewTempWindow(t, fcanvas.NewRectangle(tema.Color("bg", tema.Claro)))
	w.Resize(fyne.NewSize(1280, 800))

	esperaVelo = esperaDobleClic
	t.Cleanup(func() { esperaVelo = 0 })
	cerrado := 0
	d := NuevoDialogo("Mover a…", cuerpoDe(60))
	d.AlCerrar = func() { cerrado++ }
	d.Mostrar(w.Canvas())

	test.Tap(d)
	if cerrado != 0 {
		t.Error("pulsar en el propio panel ha cerrado el diálogo")
	}
	// El segundo clic del doble clic que lo abrió cae aquí mismo y no cierra. El
	// «recién puesto» se fija a mano: con el reloj de verdad, un runner cargado tardaba
	// más de los 300 ms de la ventana entre Mostrar y este clic, y el test caía (v3.7.15).
	d.capa.fondo.nacido = time.Now().Add(time.Hour)
	test.Tap(d.capa.fondo)
	if cerrado != 0 {
		t.Error("un clic en el velo recién puesto (el segundo de un doble clic) ha cerrado el diálogo")
	}
	d.capa.fondo.nacido = time.Time{} // pasado ese momento, el clic fuera sí cierra
	test.Tap(d.capa.fondo)
	if cerrado != 1 {
		t.Errorf("pulsar el velo ha cerrado %d veces y tiene que cerrar una", cerrado)
	}

	// Y `dismissable={false}`: el velo deja de cerrar, el aspa no.
	sin := NuevoDialogo("Subiendo 240 ficheros", cuerpoDe(60))
	sin.NoDescartable = true
	tocado := 0
	sin.AlCerrar = func() { tocado++ }
	sin.Mostrar(w.Canvas())
	test.Tap(sin.capa.fondo)
	if tocado != 0 {
		t.Error("un diálogo no descartable se ha cerrado al pulsar el velo")
	}
	test.Tap(sin.aspa)
	if tocado != 1 {
		t.Errorf("el aspa de un diálogo no descartable ha cerrado %d veces y tiene que cerrar una", tocado)
	}
}

// campoDePrueba es un control enfocable que se declara CAMPO, como lo hará un `input`.
type campoDePrueba struct {
	Boton
}

func (c *campoDePrueba) EsCampo()                {}
func (c *campoDePrueba) FocusGained()            {}
func (c *campoDePrueba) FocusLost()              {}
func (c *campoDePrueba) TypedRune(rune)          {}
func (c *campoDePrueba) TypedKey(*fyne.KeyEvent) {}

var _ Campo = (*campoDePrueba)(nil)

// TestElFocoInicialCaeEnElPrimerCampo: con el aspa siempre primero, cada diálogo se
// abría con ese botón resaltado y había que pulsar Tab para empezar a escribir.
func TestElFocoInicialCaeEnElPrimerCampo(t *testing.T) {
	w := test.NewTempWindow(t, fcanvas.NewRectangle(tema.Color("bg", tema.Claro)))
	w.Resize(fyne.NewSize(1280, 800))

	campo := &campoDePrueba{Boton{Texto: "Nombre"}}
	campo.ExtendBaseWidget(campo)
	cuerpo := container.New(Columna{Hueco: tema.Espacio(3)}, cuerpoDe(20), campo)

	d := NuevoDialogo("Nuevo contacto", cuerpo)
	d.Mostrar(w.Canvas())
	if got := w.Canvas().Focused(); got != fyne.Focusable(campo) {
		t.Errorf("el foco ha caído en %T y tiene que caer en el primer campo", got)
	}
	d.Cerrar()

	// Sin campos, el primer enfocable, que es el aspa (`first ?? panel` de Modal.tsx).
	// Abierto con el ratón —nada enfocado antes—, sin anillo: `:focus-visible` hereda la
	// modalidad. VISTO EN ROJO con el foco en el panel, que es lo que había.
	sin := NuevoDialogo("¿Vaciar la papelera?", cuerpoDe(20))
	sin.Mostrar(w.Canvas())
	if got := w.Canvas().Focused(); got != fyne.Focusable(sin.aspa) {
		t.Errorf("sin campos el foco ha caído en %T y tiene que caer en el aspa", got)
	}
	if sin.aspa.aroVisible() {
		t.Error("abierto con el ratón, el aspa enciende el anillo: en la web no")
	}
	sin.aspa.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown})
	if !sin.aspa.aroVisible() {
		t.Error("tras una tecla, el aspa enfocada tiene que enseñar el anillo")
	}
	sin.Cerrar()

	// Abierto con el teclado —había un control enfocado—, con anillo.
	antes := NuevoBoton("Vaciar", nil)
	w.SetContent(antes)
	w.Canvas().Focus(antes)
	otro := NuevoDialogo("¿Vaciar la papelera?", cuerpoDe(20))
	otro.Mostrar(w.Canvas())
	if w.Canvas().Focused() != fyne.Focusable(otro.aspa) || !otro.aspa.aroVisible() {
		t.Error("abierto con el teclado, el aspa tiene que recibir el foco con su anillo")
	}
	otro.Cerrar()
}

// TestUnaConfirmacionNuevaCancelaLaPendiente es `useConfirm`: si ya había una pregunta
// abierta, se contesta «cancelar» antes de abrir la nueva. Una Pregunta no cancela una
// Confirmacion, que en la web es otro hook. VISTO EN ROJO sin `cancelarPendiente`.
func TestUnaConfirmacionNuevaCancelaLaPendiente(t *testing.T) {
	w := test.NewTempWindow(t, fcanvas.NewRectangle(tema.Color("bg", tema.Claro)))
	w.Resize(fyne.NewSize(1280, 800))
	cancelada := 0
	var primera *Dialogo
	primera = Confirmacion("¿Borrar?", "Cancelar", "Borrar", true, cuerpoDe(20),
		func() { cancelada++; primera.Cerrar() }, func() {})
	primera.Mostrar(w.Canvas())
	p := Pregunta("Nombre", "", "Cancelar", "Aceptar", "obligatorio", func(string) {})
	p.Mostrar(w.Canvas())
	if cancelada != 0 || len(w.Canvas().Overlays().List()) != 2 {
		t.Fatal("una Pregunta ha cancelado una Confirmacion: en la web son dos hooks")
	}
	segunda := Confirmacion("¿Vaciar?", "Cancelar", "Vaciar", true, cuerpoDe(20), func() {}, func() {})
	segunda.Mostrar(w.Canvas())
	t.Cleanup(func() { segunda.Cerrar(); p.Cerrar() })
	if cancelada != 1 {
		t.Errorf("la confirmación pendiente se ha cancelado %d veces y tiene que ser una", cancelada)
	}
	if primera.capa != nil {
		t.Error("la confirmación pendiente sigue abierta debajo de la nueva")
	}
}

// TestElFocoVuelveAlCerrar: sin esto, cerrar un diálogo deja el teclado en ninguna
// parte y el siguiente Tab empieza por el principio de la pantalla.
func TestElFocoVuelveAlCerrar(t *testing.T) {
	antes := &campoDePrueba{Boton{Texto: "Buscar"}}
	antes.ExtendBaseWidget(antes)
	w := test.NewTempWindow(t, antes)
	w.Resize(fyne.NewSize(1280, 800))
	w.Canvas().Focus(antes)

	d := NuevoDialogo("Filtros", cuerpoDe(20))
	d.Mostrar(w.Canvas())
	d.Cerrar()
	if got := w.Canvas().Focused(); got != fyne.Focusable(antes) {
		t.Errorf("al cerrar el foco ha quedado en %T y estaba en el buscador", got)
	}
}

// TestEscapeCierraLaCapaDeArriba: Escape sobre el panel, y `Descartar` sobre el lienzo,
// que es la subida que usa un control cuando la tecla no es suya.
func TestEscapeCierraLaCapaDeArriba(t *testing.T) {
	w := test.NewTempWindow(t, fcanvas.NewRectangle(tema.Color("bg", tema.Claro)))
	w.Resize(fyne.NewSize(1280, 800))

	cerrado := 0
	d := NuevoDialogo("Reglas de correo", cuerpoDe(60))
	d.AlCerrar = func() { cerrado++ }
	d.Mostrar(w.Canvas())

	d.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if cerrado != 1 {
		t.Errorf("Escape sobre el panel ha cerrado %d veces", cerrado)
	}
	if !Descartar(w.Canvas()) {
		t.Error("Descartar no ha encontrado la capa de arriba")
	}
	if cerrado != 2 {
		t.Errorf("Descartar sobre el lienzo ha cerrado %d veces en total y son dos", cerrado)
	}
	d.Cerrar()
	if Descartar(w.Canvas()) {
		t.Error("Descartar ha dicho que había capa cuando no hay ninguna")
	}
}

// TestEscapeCierraConElFocoEnCualquierControl: en la web el diálogo escucha Escape en
// `document` y en fase de captura, así que cierra con el foco en un campo, en un botón o
// en cualquier otra cosa. Aquí cada control SUBE la tecla (`SubirEscape`). VISTO EN ROJO
// antes de la subida: con el foco en el campo, en el botón o en la fila, Escape no hacía
// nada.
func TestEscapeCierraConElFocoEnCualquierControl(t *testing.T) {
	w := test.NewTempWindow(t, fcanvas.NewRectangle(tema.Color("bg", tema.Claro)))
	w.Resize(fyne.NewSize(1280, 800))

	campo := NuevaEntrada("Nombre")
	boton := NuevoBoton("Guardar", nil)
	aspa := NuevoBotonIcono(iconos.Cerrar, "Cerrar", nil)
	fila := NuevaFilaLateral("Recibidos", nil, nil)
	casilla := NuevaCasilla("Recordar", nil)
	cerrado := 0
	d := NuevoDialogo("Reglas de correo", container.New(Columna{}, campo, boton, aspa, fila, casilla))
	d.AlCerrar = func() { cerrado++ }
	d.Mostrar(w.Canvas())
	t.Cleanup(d.Cerrar)

	for i, f := range []fyne.Focusable{campo, boton, aspa, fila, casilla} {
		w.Canvas().Focus(f)
		f.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
		if cerrado != i+1 {
			t.Errorf("con el foco en %T, Escape no ha pedido cerrar el diálogo", f)
			cerrado = i + 1
		}
	}
}

// TestElVeloEsElTokenOverlay: el velo es alfa, y comparar alfa pide tolerancia de un
// nivel de 255 (`cerca`). Lo que no se tolera es el color equivocado.
func TestElVeloEsElTokenOverlay(t *testing.T) {
	v := NuevoVelo(true, nil)
	v.Resize(fyne.NewSize(60, 40))
	img := pinta(t, 60, 40, v)

	// Sobre blanco, y con la entrada ya terminada (el driver de pruebas salta a
	// `Tick(1)`), el velo resuelve a la mezcla del token con el papel.
	o := tema.Color("overlay", tema.Claro)
	a := float64(o.A) / 255
	mezcla := func(c uint8) uint8 { return uint8(255 + (float64(c)-255)*a + 0.5) }
	quiero := color.NRGBA{R: mezcla(o.R), G: mezcla(o.G), B: mezcla(o.B), A: 255}
	if got := px(img, 30*int(escala), 20*int(escala)); !cerca(got, quiero) {
		t.Errorf("el velo pinta %v y `rgba(10,21,26,.5)` sobre blanco son %v", got, quiero)
	}
}

// TestLasDuracionesYLaCurvaSonLasDeLaHoja lee `styles.css` y `theme.css`: si alguien
// cambia `sytue-modal-in` a 200 ms en la web, esto se pone rojo.
func TestLasDuracionesYLaCurvaSonLasDeLaHoja(t *testing.T) {
	hoja := webref.Read(t, "apps/frontend/packages/ui/src/styles.css")
	for _, c := range []struct {
		clase string
		ms    int
	}{
		{"sytue-fade-in", int(DuracionFundido.Milliseconds())},
		{"sytue-pop-in", int(DuracionPop.Milliseconds())},
		{"sytue-modal-in", int(DuracionDialogo.Milliseconds())},
		{"sytue-toast-in", int(DuracionAviso.Milliseconds())},
	} {
		quiero := fmt.Sprintf("animation: %s %dms var(--ease-standard) both;", c.clase, c.ms)
		if !regexp.MustCompile(regexp.QuoteMeta(quiero)).MatchString(hoja) {
			t.Errorf("styles.css no declara %q: la duración de %s ya no es la nuestra", quiero, c.clase)
		}
	}
	if !regexp.MustCompile(`sytue-toast-in\s*\{\s*from\s*\{[^}]*translateY\(8px\)`).MatchString(hoja) || DesplazamientoAviso != 8 {
		t.Error("`sytue-toast-in` entra desde translateY(8px) y el aviso tiene que decir lo mismo")
	}
	// Y el `translateY(10px)` del que entra.
	if !regexp.MustCompile(`sytue-modal-in\s*\{\s*from\s*\{[^}]*translateY\(10px\)`).MatchString(hoja) {
		t.Error("`sytue-modal-in` ya no entra desde translateY(10px)")
	}

	// `--ease-standard` resuelto es `cubic-bezier(0.2, 0, 0, 1)`.
	tokens := webref.Read(t, "apps/frontend/packages/config/src/theme.css")
	if !regexp.MustCompile(`--ease-standard:\s*cubic-bezier\(0\.2,\s*0,\s*0,\s*1\)`).MatchString(tokens) {
		t.Error("`--ease-standard` ya no es cubic-bezier(0.2, 0, 0, 1): revisar CurvaEstandar")
	}
	// La curva arranca casi recta y frena largo: a la mitad del tiempo ya ha hecho
	// más de tres cuartos del recorrido. Con una lineal o con la de Fyne, no.
	if m := CurvaEstandar(0.5); m < 0.78 || m > 0.90 {
		t.Errorf("a mitad de tiempo la curva va por %v y cubic-bezier(0.2,0,0,1) da 0,84", m)
	}
	if CurvaEstandar(0) != 0 || CurvaEstandar(1) != 1 {
		t.Error("la curva no empieza en 0 y acaba en 1")
	}
}

// TestElDialogoSeRecolocaYSuPiePuedeIrALoAncho: el pie que trae su `w-full
// justify-between` ocupa la fila entera, y `Recolocar` vuelve a medir el cuerpo cuando
// crece con el diálogo abierto. VISTO EN ROJO sin los dos: el pie medía lo suyo pegado a
// la derecha, y el panel no crecía hasta cambiar de tamaño la ventana.
func TestElDialogoSeRecolocaYSuPiePuedeIrALoAncho(t *testing.T) {
	w := test.NewTempWindow(t, fcanvas.NewRectangle(tema.Color("bg", tema.Claro)))
	w.Resize(fyne.NewSize(1280, 800))
	cuerpo := fcanvas.NewRectangle(color.NRGBA{})
	cuerpo.SetMinSize(fyne.NewSize(100, 60))
	pie := fcanvas.NewRectangle(color.NRGBA{})
	pie.SetMinSize(fyne.NewSize(120, 28))
	d := NuevoDialogo("Editar contacto", cuerpo).Con(DialogoMD, pie)
	d.PieAncho = true
	d.Mostrar(w.Canvas())
	t.Cleanup(d.Cerrar)

	if got, quiero := pie.Size().Width, float32(560-2-2*24); got != quiero {
		t.Errorf("el pie a lo ancho mide %v y en un diálogo md son %v", got, quiero)
	}
	antes := d.Size().Height
	cuerpo.SetMinSize(fyne.NewSize(100, 160))
	d.Recolocar()
	if got := d.Size().Height; got != antes+100 {
		t.Errorf("tras crecer 100 el cuerpo, el panel mide %v y medía %v", got, antes)
	}
}

// TestElTituloLargoDelDialogoPasaDeLinea: el `<h2 class="sytue-h4">` de Modal.tsx parte
// y la banda crece —20 + 3 × 26 + 8 + 1 = 107 con tres líneas—, con el aspa centrada
// en la fila. VISTO EN ROJO con el título de una línea: 61 y el texto recortado.
func TestElTituloLargoDelDialogoPasaDeLinea(t *testing.T) {
	titulo := "¿Mover a la papelera las 128 conversaciones seleccionadas de esta carpeta?"
	d := NuevoDialogo(titulo, cuerpoDe(20)).Con(DialogoSM, nil)
	if n := len(lineasTitulo(titulo, anchoUtilEn(400, fyne.Size{}))); n != 3 {
		t.Fatalf("el título va en %d líneas en un diálogo sm y son 3", n)
	}
	if got := altoCabecera(d, 400, fyne.Size{}); got != 107 {
		t.Errorf("la cabecera con tres líneas mide %v y son 107", got)
	}
	tam := MedidaDialogo(d, fyne.NewSize(1280, 800))
	d.Resize(tam)
	r := test.WidgetRenderer(d).(*dialogoRenderer)
	r.Layout(tam)
	if r.titulo.Texto != titulo || len(LineasDe(r.titulo, r.titulo.Ancho)) != 3 {
		t.Errorf("el título pintado es %q en %d líneas", r.titulo.Texto, len(LineasDe(r.titulo, r.titulo.Ancho)))
	}
	if cy, ay := r.titulo.Position().Y+r.titulo.Size().Height/2, d.aspa.Position().Y+d.aspa.Size().Height/2; cy != ay {
		t.Errorf("el aspa se centra en %v y el título en %v: `items-center`", ay, cy)
	}
}

// TestLaConfirmacionOcupadaYSinConfirmar: `busy` apaga cancelar y pone la rueda en
// confirmar; `confirmDisabled` apaga solo confirmar. VISTO EN ROJO sin los dos: la
// confirmación no tenía forma de decirlo.
func TestLaConfirmacionOcupadaYSinConfirmar(t *testing.T) {
	d := Confirmacion("¿Vaciar la papelera?", "Cancelar", "Vaciar", true, cuerpoDe(20), nil, nil)
	d.SinConfirmar(true)
	if !d.confirmar.inerte() || d.cancelar.inerte() {
		t.Error("sin confirmar: solo el botón de confirmar se apaga")
	}
	d.SinConfirmar(false)
	d.Ocupado(true)
	if !d.confirmar.Cargando || !d.cancelar.Inerte {
		t.Error("ocupada: cancelar apagado y confirmar con su rueda")
	}
}

// TestElPanelDelDialogoNoEsUnaParadaDeTab: en la web el panel lleva `tabIndex={-1}`. El
// foco se le da por código cuando no hay campos, pero el Tab recorre solo los controles.
// VISTO EN ROJO con el panel `Focusable` a secas: una parada invisible en cada vuelta.
func TestElPanelDelDialogoNoEsUnaParadaDeTab(t *testing.T) {
	w := test.NewTempWindow(t, fcanvas.NewRectangle(tema.Color("bg", tema.Claro)))
	w.Resize(fyne.NewSize(1280, 800))
	a, b := NuevoBoton("Uno", nil), NuevoBoton("Dos", nil)
	d := NuevoDialogo("Reglas", container.New(Columna{}, a, b))
	d.Mostrar(w.Canvas())
	t.Cleanup(d.Cerrar)
	if w.Canvas().Focused() != fyne.Focusable(d.aspa) {
		t.Fatalf("sin campos el foco inicial es el aspa y está en %T", w.Canvas().Focused())
	}
	for range 6 {
		w.Canvas().FocusNext()
		if w.Canvas().Focused() == fyne.Focusable(d) {
			t.Fatal("el Tab ha vuelto a caer en el panel: es una parada invisible")
		}
	}
}

// TestElCuerpoDelDialogoNoPintaSombrasDeBorde: en la web el cuerpo es un
// `overflow-y-auto` a secas, y Fyne pinta una sombra en el borde por donde queda
// contenido. VISTO EN ROJO sin el `tema.SinSombras` alrededor del desplazamiento.
func TestElCuerpoDelDialogoNoPintaSombrasDeBorde(t *testing.T) {
	d := NuevoDialogo("Evento", cuerpoDe(900))
	r := test.WidgetRenderer(d).(*dialogoRenderer)
	if r.envoltorio == nil {
		t.Fatal("el cuerpo del diálogo va sin envoltorio: Fyne le pinta sombras de borde")
	}
	if _, es := r.envoltorio.Theme.(tema.SinSombras); !es {
		t.Errorf("el cuerpo va con el tema %T y tiene que ir con tema.SinSombras", r.envoltorio.Theme)
	}
}

// TestCambiarCuerpoYPieVuelveAMedirElDialogoAbierto: el paso siguiente de un asistente
// cambia cuerpo y pie sin cerrar. Si el renderer no rehace su lista de objetos, el pie
// nuevo no se pinta y el panel se queda con la altura del paso anterior.
func TestCambiarCuerpoYPieVuelveAMedirElDialogoAbierto(t *testing.T) {
	w := test.NewTempWindow(t, fcanvas.NewRectangle(tema.Color("bg", tema.Claro)))
	w.Resize(fyne.NewSize(1280, 800))
	d := NuevoDialogo("Asistente", cuerpoDe(40))
	d.Mostrar(w.Canvas())
	antes := MedidaDialogo(d, fyne.NewSize(1280, 800))
	r := test.TempWidgetRenderer(t, d).(*dialogoRenderer)

	pie := cuerpoDe(36)
	d.CambiarCuerpo(cuerpoDe(200))
	d.CambiarPie(pie)
	despues := MedidaDialogo(d, fyne.NewSize(1280, 800))
	if despues.Height <= antes.Height+160 {
		t.Errorf("con un cuerpo 160 más alto y pie el panel mide %v; antes %v", despues.Height, antes.Height)
	}
	if !contiene(r.Objects(), pie) {
		t.Error("el pie nuevo no está entre los objetos del renderer: no se pinta")
	}
	if d.Desplazador().Content.(*fyne.Container).Objects[0] == nil {
		t.Error("el desplazador se ha quedado sin cuerpo")
	}
	d.CambiarPie(nil)
	if contiene(r.Objects(), pie) {
		t.Error("quitado el pie, el renderer lo sigue pintando")
	}
}

func contiene(objs []fyne.CanvasObject, o fyne.CanvasObject) bool {
	for _, x := range objs {
		if x == o {
			return true
		}
	}
	return false
}

// Modal.tsx reserva el aspa con shrink-0: el título parte sin comerse el cierre.
func TestElAspaConservaSuTamanoConUnTituloLargo(t *testing.T) {
	w := test.NewTempWindow(t, fcanvas.NewRectangle(tema.Color("bg", tema.Claro)))
	w.Resize(fyne.NewSize(360, 800))
	for _, titulo := range []string{"Marca de agua, numeración y encabezados", "Ajustes"} {
		d := NuevoDialogo(titulo, cuerpoDe(40))
		d.Mostrar(w.Canvas())
		r := test.WidgetRenderer(d).(*dialogoRenderer)
		if s := d.aspa.Size(); s != fyne.NewSize(32, 32) {
			t.Errorf("%q: el aspa encoge a %v; shrink-0 conserva32×32", titulo, s)
		}
		if fin := r.titulo.Position().X + r.titulo.Size().Width; fin > d.aspa.Position().X-huecoCabecera+0.01 {
			t.Errorf("%q: el título invade el cierre: termina en%v, aspa en%v", titulo, fin, d.aspa.Position())
		}
		if fin := r.titulo.Position().Y + r.titulo.Size().Height; fin > r.fileteCabecera.Position().Y {
			t.Errorf("%q: la cabecera recorta el título por abajo", titulo)
		}
		if titulo != "Ajustes" && r.titulo.Size().Height < 2*TituloDialogo.Linea {
			t.Error("el título largo no se reparte en varias líneas")
		}
		d.Cerrar()
	}
}

// Un formulario conserva un mínimo de escritorio aunque su panel mida 288 px.
// El scroll vertical de Fyne no permite encoger ese mínimo: sin el ancho fluido,
// los campos de la derecha se recortan y no hay desplazamiento horizontal para verlos.
func TestElDialogoMovilEncogeElCuerpoYEnvuelveLasAcciones(t *testing.T) {
	si := true
	antes := tactilDePrueba
	tactilDePrueba = &si
	t.Cleanup(func() { tactilDePrueba = antes })
	for _, ancho := range []float32{320, 375, 390} {
		for _, lateral := range []bool{false, true} {
			t.Run(fmt.Sprintf("%g/lateral=%t", ancho, lateral), func(t *testing.T) {
				w := test.NewTempWindow(t, fcanvas.NewRectangle(color.NRGBA{}))
				w.Resize(fyne.NewSize(ancho, 568))
				campo := NuevaEntrada("Último campo")
				cuerpo := container.New(Columna{Hueco: 8},
					cajaDePrueba(900, 1100), campo)
				cancelar := NuevoBoton("Cancelar esta operación", nil)
				guardar := NuevoBoton("Guardar todos los cambios", nil)
				pie := container.New(Fila{Hueco: 8}, cancelar, guardar)
				d := NuevoDialogo("Editar los datos de la cuenta", cuerpo).Con(DialogoMD, pie)
				if lateral {
					d.ALaDerecha()
				}
				d.Mostrar(w.Canvas())
				t.Cleanup(d.Cerrar)
				if cuerpo.Size().Width > d.Size().Width-2 || cuerpo.Size().Width <= 0 {
					t.Errorf("el cuerpo conserva el mínimo de escritorio: cuerpo=%v, panel=%v", cuerpo.Size(), d.Size())
				}
				if d.Desplazador().Content.Size().Width > d.Desplazador().Size().Width {
					t.Errorf("hay contenido a la derecha sin scroll horizontal: contenido=%v, hueco=%v", d.Desplazador().Content.Size(), d.Desplazador().Size())
				}
				if guardar.Position().Y <= cancelar.Position().Y {
					t.Error("las dos acciones largas no bajaron de fila en el móvil")
				}
				for _, b := range []*Boton{cancelar, guardar} {
					if b.Position().X < 0 || b.Position().X+b.Size().Width > pie.Size().Width+0.01 || b.Position().Y+b.Size().Height > pie.Size().Height+0.01 {
						t.Errorf("la acción %q queda cortada: %v/%v en %v", b.Rotulo, b.Position(), b.Size(), pie.Size())
					}
				}
				if d.aspa.Size() != fyne.NewSize(44, 44) {
					t.Errorf("el aspa táctil encoge a %v", d.aspa.Size())
				}
				guardarDialogoMovil(t, w.Canvas(), fmt.Sprintf("modal-%g-lateral-%t-arriba", ancho, lateral))
				d.Desplazador().ScrollToBottom()
				fin := d.margen.Position().Y + d.fluido.Position().Y + cuerpo.Position().Y + campo.Position().Y + campo.Size().Height
				if d.Desplazador().Offset.Y <= 0 || fin <= 0 || fin > d.Desplazador().Size().Height+0.01 {
					t.Errorf("el último campo no se alcanza: fin=%v, scroll=%v, hueco=%v", fin, d.Desplazador().Offset, d.Desplazador().Size())
				}
				guardarDialogoMovil(t, w.Canvas(), fmt.Sprintf("modal-%g-lateral-%t-abajo", ancho, lateral))
			})
		}
	}
}

// El teclado no hace falta para reproducirlo: una ventana baja también puede ser
// una pantalla apaisada. Si cabecera y pie no caben, antes se solapaban y el cuerpo
// quedaba con alto cero. Ahora se alcanzan ambas partes desplazando el panel entero.
func TestElDialogoConPocaAlturaNoPierdeCabeceraNiAcciones(t *testing.T) {
	si := true
	antes := tactilDePrueba
	tactilDePrueba = &si
	t.Cleanup(func() { tactilDePrueba = antes })
	w := test.NewTempWindow(t, fcanvas.NewRectangle(color.NRGBA{}))
	w.Resize(fyne.NewSize(320, 220))
	cancelar, guardar := NuevoBoton("Cancelar esta operación", nil), NuevoBoton("Guardar todos los cambios", nil)
	pie := container.New(Fila{Hueco: 8}, cancelar, guardar)
	d := NuevoDialogo("Editar los datos de la cuenta y sus permisos", cuerpoDe(900)).Con(DialogoMD, pie)
	d.Mostrar(w.Canvas())
	t.Cleanup(d.Cerrar)
	r := test.WidgetRenderer(d).(*dialogoRenderer)
	if !r.desbordando || r.desborde == nil {
		t.Fatal("la cabecera y las acciones no caben, pero el panel entero no se puede desplazar")
	}
	if contiene(r.partes.Objects, r.envoltorio) || !contiene(r.partes.Objects, d.margen) || d.Desplazador() != r.desborde {
		t.Fatal("el cuerpo conserva un scroll interior que puede quedarse el arrastre del dedo")
	}
	guardarDialogoMovil(t, w.Canvas(), "modal-320x220-arriba")
	r.desborde.ScrollToBottom()
	fin := r.partes.Position().Y + pie.Position().Y + guardar.Position().Y + guardar.Size().Height
	if r.desborde.Offset.Y <= 0 || fin <= 0 || fin > d.Size().Height+0.01 {
		t.Errorf("no se alcanza Guardar: fin=%v, panel=%v, scroll=%v", fin, d.Size(), r.desborde.Offset)
	}
	guardarDialogoMovil(t, w.Canvas(), "modal-320x220-abajo")
	r.desborde.ScrollToTop()
	if y := r.partes.Position().Y + d.aspa.Position().Y; y < 0 || y+d.aspa.Size().Height > d.Size().Height {
		t.Errorf("no se alcanza el aspa al volver arriba: y=%v, panel=%v", y, d.Size())
	}
	// El driver hace este resize al desaparecer el teclado o girar la pantalla.
	d.capa.Resize(fyne.NewSize(320, 700))
	d.capa.Refresh()
	if r.desbordando || r.envoltorio.Size().Height < 44 {
		t.Errorf("al recuperar altura el cuerpo no vuelve a tener su desplazamiento propio: %v", r.envoltorio.Size())
	}
}

type lienzoConAreaSegura struct {
	fyne.Canvas
	pos fyne.Position
	tam fyne.Size
}

func (c *lienzoConAreaSegura) InteractiveArea() (fyne.Position, fyne.Size) { return c.pos, c.tam }

// Las mismas regresiones dejan retratos reales del pintor de software en el CI;
// los datos son sintéticos y no hace falta arrancar una app ni una sesión.
func guardarDialogoMovil(t *testing.T, c fyne.Canvas, nombre string) {
	t.Helper()
	dir := os.Getenv("SYTUE_CAPTURA_DIALOGOS")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(dir, nombre+".png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, c.Capture()); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	t.Logf("pintor=software diálogo=%s lienzo=%v", nombre, c.Size())
}

// El driver ya coloca el overlay en InteractiveArea. Mostrar lo sobrescribía con el
// canvas completo al abrir; en iOS esa área también descuenta un teclado ya visible.
func TestElDialogoRespetaElAreaSeguraDesdeLaApertura(t *testing.T) {
	for _, c := range []struct {
		ventana fyne.Size
		pos     fyne.Position
		segura  fyne.Size
	}{
		{fyne.NewSize(390, 844), fyne.NewPos(0, 47), fyne.NewSize(390, 763)},
		{fyne.NewSize(844, 390), fyne.NewPos(44, 0), fyne.NewSize(756, 369)},
		{fyne.NewSize(390, 844), fyne.NewPos(0, 47), fyne.NewSize(390, 440)},
	} {
		w := test.NewTempWindow(t, fcanvas.NewRectangle(color.NRGBA{}))
		w.Resize(c.ventana)
		seguro := &lienzoConAreaSegura{Canvas: w.Canvas(), pos: c.pos, tam: c.segura}
		d := NuevoDialogo("Nuevo evento", cuerpoDe(1200))
		d.Mostrar(seguro)
		if d.capa.Position() != c.pos || d.capa.Size() != c.segura {
			t.Errorf("la capa usa el canvas completo: %v/%v; área segura %v/%v", d.capa.Position(), d.capa.Size(), c.pos, c.segura)
		}
		if d.Position().X < 0 || d.Position().Y < 0 || d.Position().X+d.Size().Width > c.segura.Width || d.Position().Y+d.Size().Height > c.segura.Height {
			t.Errorf("el panel sale del área segura: %v/%v en %v", d.Position(), d.Size(), c.segura)
		}
		d.Cerrar()
	}
}
