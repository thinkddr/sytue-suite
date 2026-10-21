// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"fmt"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// barraDePrueba monta una barra con dos acciones y le crea el renderer, que es quien
// ata el foco rotatorio.
func barraDePrueba(alQuitar func()) *BarraDeLote {
	b := NuevaBarraDeLote("3 archivos seleccionados", "Acciones para la selección", "Quitar la selección",
		[]*Boton{
			NuevoBoton("Descargar", nil).Con(Secundario, TallaSM),
			NuevoBoton("Papelera", nil).Con(Peligro, TallaSM),
		}, alQuitar)
	b.Resize(fyne.NewSize(400, AltoBarraLote))
	b.CreateRenderer()
	return b
}

// enElRecorrido son los índices de los controles que HOY son parada de tabulación.
func enElRecorrido(b *BarraDeLote) []int {
	var dentro []int
	for i, c := range b.controles() {
		if !c.Disabled() {
			dentro = append(dentro, i)
		}
	}
	return dentro
}

// TestLaBarraDeLoteEsUnaSolaParadaDeTabulacion es el defecto que arregló
// `BulkActionBar.tsx`: las tres barras de la web prometían `role="toolbar"` y cada
// botón era su propia parada, así que salir de una barra de seis acciones costaba seis
// tabulaciones.
//
// Se mide por el gancho que USA el gestor de foco de Fyne (`Disabled()`), no por una
// variable nuestra: un test que compare la implementación consigo misma pasa siempre.
//
// VISTO EN ROJO (2026-09-22, en el piloto): quitando la llamada a `EnElRecorrido` de
// `Activar` el test dice «son parada de tabulación [0 1 2] y tiene que serlo UNO».
// Deshecho.
func TestLaBarraDeLoteEsUnaSolaParadaDeTabulacion(t *testing.T) {
	b := barraDePrueba(nil)
	if n := len(b.controles()); n != 3 {
		t.Fatalf("la barra tiene %d controles y son dos acciones y el aspa", n)
	}
	if dentro := enElRecorrido(b); len(dentro) != 1 || dentro[0] != 0 {
		t.Fatalf("son parada de tabulación %v y tiene que serlo UNO, el primero", dentro)
	}

	// Las flechas rotan y ARRASTRAN la parada: si no, el siguiente Tab saldría por
	// donde estaba antes.
	b.controles()[0].TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	if dentro := enElRecorrido(b); len(dentro) != 1 || dentro[0] != 1 {
		t.Errorf("tras la flecha derecha la parada está en %v y tenía que estar en [1]", dentro)
	}
	// Y es circular: desde el último, adelante vuelve al primero.
	b.controles()[2].TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	if dentro := enElRecorrido(b); len(dentro) != 1 || dentro[0] != 0 {
		t.Errorf("desde el último, adelante deja la parada en %v y tenía que volver a [0]", dentro)
	}
	// Fin salta al aspa, que es el último control.
	b.controles()[0].TypedKey(&fyne.KeyEvent{Name: fyne.KeyEnd})
	if dentro := enElRecorrido(b); len(dentro) != 1 || dentro[0] != 2 {
		t.Errorf("Fin deja la parada en %v y el último control es el 2", dentro)
	}
}

// TestLaBarraDeLoteVaciaConEscape: lo primero que se prueba, y lo que no hacía ninguna
// de las tres barras de la web.
func TestLaBarraDeLoteVaciaConEscape(t *testing.T) {
	vaciada := 0
	b := barraDePrueba(func() { vaciada++ })

	b.controles()[0].TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if vaciada != 1 {
		t.Errorf("Escape con el foco en una acción vació %d veces y tenía que vaciar una", vaciada)
	}
	// Y desde fuera, que es de donde llega de verdad: el foco se queda en la LISTA.
	b.Escapar()
	if vaciada != 2 {
		t.Errorf("Escapar() vació %d veces en total y tenían que ser dos", vaciada)
	}
}

// TestElBotonInerteNoEsParadaDeTabulacion: en el navegador un `<button disabled>` no
// está en el recorrido, y aquí sí lo estaba hasta que `Boton` implementó
// `fyne.Disableable`.
func TestElBotonInerteNoEsParadaDeTabulacion(t *testing.T) {
	b := NuevoBoton("Guardar", nil)
	if b.Disabled() {
		t.Error("un botón normal se ha declarado fuera del recorrido")
	}
	b.Inerte = true
	if !b.Disabled() {
		t.Error("un botón inerte sigue siendo parada de tabulación")
	}
}

// TestLaAyudaEnLineaCuentaSusLineas: el alto es el `p-3` de los dos lados más lo que
// ocupen titular y cuerpo repartidos, con el `mt-1` de por medio.
func TestLaAyudaEnLineaCuentaSusLineas(t *testing.T) {
	const ancho float32 = 240
	corto := NuevaAyudaEnLinea("Se borra en 30 días.")
	// Una línea sola no puede bajar del icono de 16 más el relleno: es `items-start`
	// con un glifo que ocupa sitio.
	if a := AltoAyudaEnLinea(corto, ancho); a < 2*tema.Borde+2*RellenoAyuda+GlifoAyuda {
		t.Errorf("una ayuda de una línea mide %g y el icono con su `p-3` ya son %g",
			a, 2*RellenoAyuda+GlifoAyuda)
	}
	largo := NuevaAyudaEnLinea("Lo que hay en la papelera se puede restaurar hasta que se " +
		"purgue, y a partir de ahí no lo recupera nadie, ni el administrador de la organización.")
	if AltoAyudaEnLinea(largo, ancho) <= AltoAyudaEnLinea(corto, ancho) {
		t.Error("una ayuda de varias líneas no ocupa más que una de una: no está repartiendo")
	}
	// Con titular, el `mt-1` y la línea del titular se suman.
	conTitulo := NuevaAyudaEnLinea("Se borra en 30 días.").ConTitulo("Papelera")
	quiere := AltoAyudaEnLinea(corto, ancho) + MargenTitulo + relajado.Linea
	if a := AltoAyudaEnLinea(conTitulo, ancho); a != quiere {
		t.Errorf("con titular mide %g y tenía que medir %g (una línea más y el `mt-1`)", a, quiere)
	}
	// Medido en el arnés del ERP (2026-09-23): una línea de cuerpo con su icono mide
	// 41,48, porque `rounded-md border p-3` suma el borde. VISTO EN ROJO (frontal): sin
	// él, 39,48.
	if a := AltoAyudaEnLinea(NuevaAyudaEnLinea("Falta: Vencimiento."), 653); a < 47.12 || a > 47.13 {
		t.Errorf("una ayuda de una línea mide %g; el contrato web 47,125", a)
	}
	// El tono neutro pinta borde `border` y fondo `bg-subtle`, que es `helpSurface`.
	if s := superficieTono[AyudaNeutra]; s[0] != "border" || s[1] != "bg-subtle" {
		t.Errorf("el tono neutro usa %v y la hoja pide border-border bg-bg-subtle", s)
	}
}

// TestLaPistaCampoSoloLlevaIconoEnEstado: `showIcon = !hideIcon && tone !== 'neutral'`.
func TestLaPistaCampoSoloLlevaIconoEnEstado(t *testing.T) {
	neutra := NuevaPistaCampo("Está en tu URL.")
	if neutra.conIcono() {
		t.Error("la pista neutra lleva icono y en la web no: no hay nada que señalar")
	}
	mala := NuevaPistaCampo("Ese espacio no existe.").ConTono(AyudaPeligro)
	if !mala.conIcono() {
		t.Error("la pista de error no lleva icono")
	}
	if a, b := MedidaPistaCampo(mala).Width, MedidaPistaCampo(neutra).Width; a <= b {
		t.Errorf("con icono mide %g de ancho y sin él %g: el glifo no está contando", a, b)
	}
	// Con Ancho se reparte: dos renglones ocupan dos cajas de línea y el ancho es el
	// de la caja, no el del texto.
	larga := NuevaPistaCampo("Es lo que verá la otra persona en su bandeja. Si lo dejas en blanco usaremos «Estoy de vacaciones».")
	larga.Ancho = 200
	if m := MedidaPistaCampo(larga); m.Width != 200 || m.Height < 2*ajustado.Linea {
		t.Errorf("la pista larga a 200 mide %v: tenía que repartirse en varias líneas de %g", m, ajustado.Linea)
	}
	// `text-xs leading-snug`: 1,375 sobre el tamaño de `text-xs`, no el interlineado
	// por defecto del escalón.
	if ajustado.Linea != 1.375*tema.TextoXS.Tam {
		t.Errorf("la caja de línea de la pista es %g y `leading-snug` da %g",
			ajustado.Linea, 1.375*tema.TextoXS.Tam)
	}
}

// TestElDeslizadorSeMueveConSuPaso: las flechas mueven un `step` del control, no una
// centésima del recorrido. Con el recorrido, la exposición (−5 a 5) se movía cinco
// veces más deprisa que el contraste (−1 a 1) con la misma tecla.
func TestElDeslizadorSeMueveConSuPaso(t *testing.T) {
	d := NuevoDeslizador(-5, 5, 0, nil)
	d.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	if d.Valor != PasoDeslizador {
		t.Errorf("la flecha derecha dejó el valor en %v y el `step` es %v", d.Valor, PasoDeslizador)
	}
	otro := NuevoDeslizador(-1, 1, 0, nil)
	otro.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	if otro.Valor != d.Valor {
		t.Errorf("dos deslizadores con recorridos distintos se mueven %v y %v con la misma tecla",
			d.Valor, otro.Valor)
	}
	d.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEnd})
	if d.Valor != 5 {
		t.Errorf("Fin dejó el valor en %v y el tope es 5", d.Valor)
	}
	d.TypedKey(&fyne.KeyEvent{Name: fyne.KeyHome})
	if d.Valor != -5 {
		t.Errorf("Inicio dejó el valor en %v y el suelo es −5", d.Valor)
	}

	// Y se arrastra: hasta ahora el widget no era `fyne.Draggable` y solo se movía con
	// el teclado. El centro de la caja es el centro del recorrido.
	d.Resize(fyne.NewSize(100+LadoPulgar, AltoDeslizador()))
	d.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(50+LadoPulgar/2, 7)}})
	if d.Valor != 0 {
		t.Errorf("arrastrando al centro el valor es %v y tenía que ser 0", d.Valor)
	}

	// El suelo táctil NO toca la medida en escritorio: la caja es la de la web.
	if AltoDeslizador() != LadoPulgar {
		t.Errorf("en escritorio la caja mide %g y el `<input type=range>` mide %g",
			AltoDeslizador(), LadoPulgar)
	}
}

// TestLaBarraDeLoteEnvuelveSiNoCabe es el `flex-wrap` de BulkActionBar: con las cinco
// acciones de Sytue Contacts en su panel de 279 no cabe en una línea, y lo que no
// cabe baja a la siguiente, pegado a la izquierda y a `gap-1.5`; lo de la primera
// línea que va detrás del `flex-1` sigue pegado a la derecha. Con sitio, una línea y
// los 43 de siempre.
func TestLaBarraDeLoteEnvuelveSiNoCabe(t *testing.T) {
	acciones := make([]*Boton, 0, 5)
	for _, r := range []string{"Destacar", "Quitar destacado", "Añadir a etiqueta", "Quitar de etiqueta"} {
		acciones = append(acciones, NuevoBoton(r, nil).Con(Secundario, TallaSM))
	}
	acciones = append(acciones, NuevoBoton("A papelera", nil).Con(Peligro, TallaSM))
	b := NuevaBarraDeLote("2 contactos seleccionados", "Acciones para la selección", "Quitar la selección", acciones, nil)

	if got := MedidaBarraDeLoteEn(b, 1200).Height; got != AltoBarraLote {
		t.Errorf("con sitio la barra mide %v y en una línea son %v", got, AltoBarraLote)
	}
	lineas := lineasLote(b, 279)
	if len(lineas) < 2 {
		t.Fatalf("a 279 la barra cabe en %d línea y no cabe", len(lineas))
	}
	alto := MedidaBarraDeLoteEn(b, 279).Height
	if quiero := 2*tema.Espacio(2) + float32(len(lineas))*32 + float32(len(lineas)-1)*tema.Espacio(1.5) + 1; alto != quiero {
		t.Errorf("a 279, con %d líneas, la barra mide %v y son %v", len(lineas), alto, quiero)
	}
	b.Resize(fyne.NewSize(279, alto))
	_ = test.WidgetRenderer(b)
	b.Refresh()
	segunda := acciones[lineas[1][0]-1]
	if p := segunda.Position(); p.X != tema.Espacio(3) || p.Y != tema.Espacio(2)+32+tema.Espacio(1.5) {
		t.Errorf("lo primero de la segunda línea está en %v y va a (10,5, 40,25)", p)
	}
	if ult := acciones[lineas[0][len(lineas[0])-1]-1]; ult.Position().X+ult.Size().Width != 279-tema.Espacio(3) {
		t.Errorf("lo último de la primera línea acaba en %v y va pegado a la derecha (268,5)",
			ult.Position().X+ult.Size().Width)
	}
}

// La ficha de Drive mostraba Papelera como una raya roja debajo de la lista:
// Pila reservaba una fila antes de dar ancho a la barra, que necesitaba dos.
// Se comprueba el primer reparto y los dos sentidos del resize, sin corregir el
// alto a mano ni refrescar la pila una segunda vez para que llegue a cuadrar.
func TestLaPilaReservaLaBarraDeLoteAlAnchoActual(t *testing.T) {
	for _, tactil := range []bool{false, true} {
		for _, estrecho := range []float32{320, 375, 390} {
			t.Run(fmt.Sprintf("tactil=%v/%g", tactil, estrecho), func(t *testing.T) {
				antes := tactilDePrueba
				tactilDePrueba = &tactil
				t.Cleanup(func() { tactilDePrueba = antes })
				b := NuevaBarraDeLote("3 archivos seleccionados", "Acciones para la selección", "Quitar la selección",
					[]*Boton{
						NuevoBoton("Mover", nil).Con(Secundario, TallaSM),
						NuevoBoton("Descargar", nil).Con(Secundario, TallaSM),
						NuevoBoton("Papelera", nil).Con(Peligro, TallaSM),
					}, nil).Con(tema.Espacio(4))
				cabecera := canvas.NewRectangle(tema.Color("bg-subtle", tema.Claro))
				cabecera.SetMinSize(fyne.NewSize(0, 40))
				cuerpo := canvas.NewRectangle(tema.Color("surface", tema.Claro))
				pie := canvas.NewRectangle(tema.Color("bg-subtle", tema.Claro))
				pie.SetMinSize(fyne.NewSize(0, 24))
				pila := container.New(Pila{Elastico: 2}, cabecera, b, cuerpo, pie)
				if b.Size().Width != 0 {
					t.Fatal("el fixture ya dio ancho a la barra antes del primer reparto")
				}
				for _, ancho := range []float32{estrecho, 1200, estrecho, 1200} {
					pila.Resize(fyne.NewSize(ancho, 640))
					r := test.WidgetRenderer(b).(*barraLoteRenderer)
					if b.Size().Height != b.AltoPara(ancho) {
						t.Fatalf("a %g la pila reserva %g para una barra de %g", ancho, b.Size().Height, b.AltoPara(ancho))
					}
					if cuerpo.Position().Y != cabecera.Size().Height+b.Size().Height || cuerpo.Position().Y+cuerpo.Size().Height != pie.Position().Y || pie.Position().Y+pie.Size().Height != 640 {
						t.Fatalf("a %g el cuerpo y los fijos se pisan: barra=%v/%v cuerpo=%v/%v pie=%v/%v", ancho, b.Position(), b.Size(), cuerpo.Position(), cuerpo.Size(), pie.Position(), pie.Size())
					}
					for _, accion := range b.controles() {
						caja := fyne.NewSize(b.Size().Width, b.Size().Height-tema.Borde)
						if tactil {
							if r.tira == nil || accion.Size().Height < 44 || r.tira.Position().Y+r.tira.Size().Height > b.Size().Height-tema.Borde {
								t.Fatal("la barra perdió su tira y su alto táctil")
							}
							r.tira.Mostrar(accion)
							caja = r.tira.Size()
						}
						p, m := accion.Position(), accion.Size()
						if p.X < -0.01 || p.Y < 0 || p.X+m.Width > caja.Width+0.01 || p.Y+m.Height > caja.Height+0.01 {
							t.Fatalf("a %g se recorta %q: %v/%v en %v", ancho, accion.Texto, p, m, caja)
						}
					}
				}
			})
		}
	}
}

// TestElDobleClicDelDeslizador: dos clics seguidos llaman a AlDobleClic (el
// `onDblClick` del editor de Photos, que vuelve el ajuste a 0); uno suelto, no.
func TestElDobleClicDelDeslizador(t *testing.T) {
	veces := 0
	d := NuevoDeslizador(-1, 1, 0, nil)
	d.AlDobleClic = func() { veces++ }
	d.Resize(fyne.NewSize(100+LadoPulgar, AltoDeslizador()))
	en := &fyne.PointEvent{Position: fyne.NewPos(80, 7)}
	d.Tapped(en)
	if veces != 0 {
		t.Fatal("un clic suelto ya contó como doble")
	}
	d.Tapped(en)
	if veces != 1 {
		t.Fatalf("dos clics seguidos llamaron %d veces a AlDobleClic", veces)
	}
	d.Tapped(en)
	if veces != 1 {
		t.Error("el tercer clic contó como otro doble: el par ya se había gastado")
	}
}

// TestLaBarraDeLoteSeDeslizaEnTactil: en un teléfono la barra NO se reparte en
// líneas —en el panel de Contactos, a 279, sus cinco acciones ocupaban tres y se
// comían la lista que hay debajo—. Es UNA fila que se desplaza de lado, la misma
// respuesta que la web en móvil. VISTO EN ROJO quitando la rama de `esTactil` de
// `MedidaBarraDeLoteEn`: la barra volvía a medir tres líneas.
func TestLaBarraDeLoteSeDeslizaEnTactil(t *testing.T) {
	si := true
	tactilDePrueba = &si
	t.Cleanup(func() { tactilDePrueba = nil })

	b := NuevaBarraDeLote("3 archivos seleccionados", "Acciones para la selección", "Quitar la selección",
		[]*Boton{
			NuevoBoton("Descargar", nil).Con(Secundario, TallaSM),
			NuevoBoton("Papelera", nil).Con(Peligro, TallaSM),
			NuevoBoton("Etiquetar", nil).Con(Secundario, TallaSM),
			NuevoBoton("Spam", nil).Con(Peligro, TallaSM),
		}, nil)
	// El ancho del panel de lista de Contacts, donde la envoltura hacía tres líneas.
	const ancho float32 = 279
	alto := MedidaBarraDeLote().Height
	if got := MedidaBarraDeLoteEn(b, ancho).Height; got != alto {
		t.Errorf("en táctil la barra mide %g a %g de ancho y una línea son %g",
			got, ancho, alto)
	}

	b.Resize(fyne.NewSize(ancho, alto))
	r := b.CreateRenderer()
	defer r.Destroy()
	var tira *TiraDeslizable
	for _, o := range r.Objects() {
		if v, ok := o.(*TiraDeslizable); ok {
			tira = v
		}
	}
	if tira == nil {
		t.Fatal("en táctil la barra no monta la tira deslizable")
	}
	// Recuento + cuatro acciones + el aspa: todo dentro de la tira, ninguno
	// suelto fuera.
	if len(tira.Objetos) != 6 {
		t.Errorf("la tira lleva %d objetos y son el recuento, cuatro acciones y el aspa", len(tira.Objetos))
	}
	// Y su contenido NO cabe: por eso se desplaza, que es el punto.
	if tira.AnchoContenido() <= ancho-2*rellenoLoteX {
		t.Error("el contenido de la tira cabe entero: la prueba ya no prueba deslizamiento")
	}
	for _, ancho := range []float32{279, 320, 375, 390} {
		b.Resize(MedidaBarraDeLoteEn(b, ancho))
		r.Layout(b.Size())
		if fin := tira.Position().Y + tira.Size().Height; fin > b.Size().Height-tema.Borde {
			t.Errorf("a %g la tira acaba en %g fuera de la barra de %g", ancho, fin, b.Size().Height)
		}
		for _, accion := range b.controles() {
			if accion.Size().Height < 44 || accion.Position().Y < 0 || accion.Position().Y+accion.Size().Height > tira.Size().Height {
				t.Errorf("a %g la acción %q queda recortada: %v/%v en una tira de %v", ancho,
					accion.Texto, accion.Position(), accion.Size(), tira.Size())
			}
		}
	}
}
