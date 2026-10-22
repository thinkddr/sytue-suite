// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image"
	"math"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// Los números de Spinner.tsx, a mano y no leídos de la implementación: `viewBox="0 0 24
// 24"`, `r="9"`, `stroke-width="2"`, el `circle` a `opacity="0.25"` y el `path` de 270°
// que arranca en (12,3) —arriba— en el sentido del reloj.
const (
	webRuedaRejilla = 24.0
	webRuedaRadio   = 9.0
	webRuedaGrosor  = 2.0
	webRuedaBarrido = 270.0 // grados
	webRuedaAnillo  = 0.25
)

// TestLaRuedaTieneLaGeometriaDeSpinnerTsx mira los PÍXELES: que el anillo cae donde la
// web pone `r="9"` con `stroke-width="2"`, que el hueco de dentro está vacío y que el
// arco opaco cubre tres cuartos de vuelta dejando el otro cuarto al 25 %.
//
// Se mide sobre el mapa de bits y no sobre las cuentas del widget: un guardián que le
// pregunte a la implementación pasa siempre (regla 6 del paquete).
func TestLaRuedaTieneLaGeometriaDeSpinnerTsx(t *testing.T) {
	const lado = 24 // la rejilla entera, para que 1 unidad de la web sea 1 px de CSS
	r := NuevaRueda(lado, "text", tema.Claro)
	r.Resize(MedidaRueda(lado))
	img := pinta(t, lado, lado, r)

	// El centro y el radio, en píxeles de DISPOSITIVO.
	c := float64(lado) * float64(escala) / 2
	radio := webRuedaRadio / webRuedaRejilla * float64(lado) * float64(escala)
	grosor := webRuedaGrosor / webRuedaRejilla * float64(lado) * float64(escala)

	// El alfa de la tinta a una distancia y un ángulo dados del centro. El ángulo se
	// mide desde arriba y en el sentido del reloj, como el `path` de la web.
	alfa := func(rho, grados float64) float64 {
		a := grados * math.Pi / 180
		x := int(c + rho*math.Sin(a))
		y := int(c - rho*math.Cos(a))
		p := px(img, x, y)
		// Sobre blanco y con tinta oscura, lo que baja es el canal: se convierte de
		// vuelta a cobertura.
		negro := tema.Color("text", tema.Claro)
		return (255 - float64(p.R)) / (255 - float64(negro.R))
	}

	// 1. El centro del anillo está pintado y el interior, no.
	if a := alfa(radio, 45); a < 0.8 {
		t.Errorf("el arco a 45° pinta %.2f de cobertura y tendría que estar opaco", a)
	}
	if a := alfa(radio-2*grosor, 45); a > 0.1 {
		t.Errorf("el interior de la rueda pinta %.2f y `fill=\"none\"` quiere 0", a)
	}
	if a := alfa(radio+2*grosor, 45); a > 0.1 {
		t.Errorf("fuera del anillo pinta %.2f y tendría que estar limpio", a)
	}

	// 2. El cuarto que NO cubre el arco se queda en el anillo al 25 %.
	//    El arco va de 0° a 270°, así que el hueco está centrado en 315°.
	hueco := alfa(radio, 315)
	if math.Abs(hueco-webRuedaAnillo) > 0.08 {
		t.Errorf("el anillo suelto pinta %.2f y `opacity=\"0.25\"` de Spinner.tsx quiere %.2f", hueco, webRuedaAnillo)
	}

	// 3. Y el barrido son 270° y no otra cosa: se recorre el anillo de grado en grado
	//    y se cuenta cuántos salen opacos.
	opacos := 0
	for g := range 360 {
		if alfa(radio, float64(g)) > 0.6 {
			opacos++
		}
	}
	//    Los `stroke-linecap="round"` alargan el arco medio grosor por cada punta, que
	//    a este radio son 6,4° cada una: el barrido pintado son 270 + 12,7 = 282,7°, y
	//    medido salen 283. El margen es de 3°, no de «unos cuantos».
	conPuntas := webRuedaBarrido + 2*(grosor/2)/radio*180/math.Pi
	if math.Abs(float64(opacos)-conPuntas) > 3 {
		t.Errorf("el arco opaco cubre %d° y el `path` de la web con sus casquetes barre %.1f°", opacos, conPuntas)
	}
}

// TestLaRuedaQuietaSiguePintandose es la decisión de `SinMovimiento`: sin animación la
// rueda NO desaparece, se queda parada. Una rueda que se va no dice que algo carga.
func TestLaRuedaQuietaSiguePintandose(t *testing.T) {
	antes := SinMovimiento
	SinMovimiento = true
	defer func() { SinMovimiento = antes }()

	r := NuevaRueda(24, "text", tema.Claro)
	r.Resize(MedidaRueda(24))
	img := pinta(t, 24, 24, r)
	if pixelesPintados(img) == 0 {
		t.Error("con SinMovimiento la rueda no ha pintado nada: un indicador que se apaga no indica")
	}
}

// pixelesPintados cuenta los píxeles que no son el blanco del lienzo.
func pixelesPintados(img image.Image) int {
	b := img.Bounds()
	n := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if p := px(img, x, y); p.R < 250 || p.G < 250 || p.B < 250 {
				n++
			}
		}
	}
	return n
}

// TestElBotonDeSoloIconoEsCuadrado son los `iconSizes` de Button.tsx: `h-N w-N p-0`, con
// el suelo de 24 de `xs` en los DOS ejes (`min-h-[24px] min-w-[24px]`, WCAG 2.5.8).
func TestElBotonDeSoloIconoEsCuadrado(t *testing.T) {
	// Los números salen de la hoja con el rem de 14, escritos a mano.
	casos := []struct {
		clase string
		talla Talla
		lado  float32
	}{
		{"xs → h-6 w-6 con min-*-[24px]", TallaXS, 24},
		{"sm → h-8 w-8", TallaSM, 2 * tema.Rem},
		{"md → h-9 w-9", TallaMD, 2.25 * tema.Rem},
		{"lg → h-11 w-11", TallaLG, 2.75 * tema.Rem},
	}
	for _, c := range casos {
		b := &Boton{
			Variante: Fantasma, Talla: c.talla, SoloIcono: true,
			Icono: iconos.Cerrar, Rotulo: "Cerrar",
		}
		b.ExtendBaseWidget(b)
		m := MedidaBotonCon(b.Texto, b.Talla, b.Ancho, b.glifos(), b.SoloIcono, b.TamIcono)
		if m.Width != c.lado || m.Height != c.lado {
			t.Errorf("%s: el botón de solo icono mide %v y la hoja dice %v×%v", c.clase, m, c.lado, c.lado)
		}
		if got := b.MinSize(); got != m {
			t.Errorf("%s: MinSize() = %v y MedidaBotonCon = %v: dos cuentas donde tiene que haber una", c.clase, got, m)
		}
	}
}

// TestElIconoDelanteOcupaSuHueco es `leadingIcon` con `gap-2`: el botón se ensancha
// exactamente el glifo más el hueco, ni más ni menos.
func TestElIconoDelanteOcupaSuHueco(t *testing.T) {
	mide := func(b *Boton) fyne.Size {
		b.ExtendBaseWidget(b)
		return MedidaBotonCon(b.Texto, b.Talla, b.Ancho, b.glifos(), b.SoloIcono, b.TamIcono)
	}
	solo := mide(&Boton{Texto: "Redactar"})
	con := mide(&Boton{Texto: "Redactar", Icono: iconos.Mas})

	// `md` es `gap-2` (0.5rem = 7) y el glifo por defecto de Icon.tsx, 20.
	quiero := solo.Width + 2*0.25*tema.Rem + TamGlifoBoton
	if !aproximado(con.Width, quiero) {
		t.Errorf("con `leadingIcon` el botón mide %v de ancho y el glifo más el `gap-2` piden %v", con.Width, quiero)
	}
	if con.Height != solo.Height {
		t.Error("el icono ha cambiado el alto del botón, y `h-9` no depende del contenido")
	}
}

// TestCargandoRelevaAlIconoYAtenuaMenos son las dos mitades del `loading` de Button.tsx:
// la rueda RELEVA al icono de delante (el ternario del JSX) y el botón se queda al 75 %,
// no al 50 % del inerte — «cargando ≠ deshabilitado», dice el componente.
func TestCargandoRelevaAlIconoYAtenuaMenos(t *testing.T) {
	pulsado := false
	b := &Boton{Texto: "Guardar", Icono: iconos.Mas, AlPulsar: func() { pulsado = true }}
	b.ExtendBaseWidget(b)
	mide := func() fyne.Size {
		return MedidaBotonCon(b.Texto, b.Talla, b.Ancho, b.glifos(), b.SoloIcono, b.TamIcono)
	}
	reposo := mide()

	// La rueda RELEVA al icono con el lado de su talla (`<Spinner size={16}>` en `md`):
	// cargando, el botón mide lo mismo con el glifo de 20 cambiado por la rueda de 16.
	// VISTO EN ROJO (frontal, 2026-09-24): con la rueda del lado del glifo, 4 px de más.
	b.Cargando = true
	if got, quiero := b.MinSize().Width, reposo.Width-TamGlifoBoton+LadoRuedaBoton(TallaMD); !aproximado(got, quiero) {
		t.Errorf("cargando el botón mide %v de ancho y la web %v: la rueda releva al icono con su lado", got, quiero)
	}
	sinIcono := &Boton{Texto: "Crear tablero", Cargando: true}
	sinIcono.ExtendBaseWidget(sinIcono)
	if got := sinIcono.MinSize().Width; got < 139.7 || got > 140.3 {
		t.Errorf("«Crear tablero» cargando mide %v; la web 124,48", got)
	}
	if b.opacidad() != 0.75 {
		t.Errorf("cargando la opacidad es %v y Button.tsx pone `opacity-75`", b.opacidad())
	}
	b.Inerte = true
	if b.opacidad() != 0.5 {
		t.Errorf("inerte la opacidad es %v y la hoja pone `disabled:opacity-50`", b.opacidad())
	}

	b.Inerte = false
	b.Resize(reposo)
	// Se pinta para que el renderer exista y monte la rueda, y se comprueba que un
	// botón cargando no responde (`const inerte = disabled || loading`).
	pinta(t, reposo.Width+10, reposo.Height+10, b)
	b.Tapped(nil)
	if pulsado {
		t.Error("un botón cargando ha llamado a AlPulsar")
	}
}

// TestLaRuedaDeUnBotonQueEmpiezaACargarQuedaColocada es el fallo que este paquete ya ha
// tenido dos veces: una pieza que aparece DESPUÉS de montar el widget y que nadie
// coloca, porque `Layout` solo corre al redimensionar. Se pinta en la esquina.
func TestLaRuedaDeUnBotonQueEmpiezaACargarQuedaColocada(t *testing.T) {
	b := &Boton{Texto: "Guardar"}
	b.ExtendBaseWidget(b)
	m := b.MinSize()
	b.Resize(m)
	pinta(t, m.Width+20, m.Height+20, b) // monta el renderer

	b.Cargando = true
	b.Refresh()
	pinta(t, m.Width+20, m.Height+20, b)

	rueda := ruedaDe(t, b)
	if rueda.Position().X <= 0 || rueda.Position().Y <= 0 {
		t.Errorf("la rueda se ha quedado en %v: nadie la ha colocado al aparecer", rueda.Position())
	}
	lado := LadoRuedaBoton(b.Talla) // el `<Spinner size={16}>` de `md`
	// Y centrada en vertical, que es el `items-center`.
	if quiero := (m.Height - lado) / 2; !aproximado(rueda.Position().Y, quiero) {
		t.Errorf("la rueda está a y=%v y centrada en la caja de %v cae en %v", rueda.Position().Y, m.Height, quiero)
	}
}

// ruedaDe busca la rueda entre los objetos del renderer vivo del botón.
func ruedaDe(t *testing.T, b *Boton) fyne.CanvasObject {
	t.Helper()
	for _, o := range test.WidgetRenderer(b).Objects() {
		if _, ok := o.(*Rueda); ok {
			return o
		}
	}
	t.Fatal("el botón cargando no tiene rueda entre sus objetos: no se va a pintar nada")
	return nil
}

func aproximado(a, b float32) bool { return math.Abs(float64(a-b)) < 0.01 }
