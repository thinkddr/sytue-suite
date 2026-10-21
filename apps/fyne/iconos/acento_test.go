// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package iconos

import (
	"image/color"
	"strings"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// fuenteGlifo es el envoltorio de los iconos de la web: de él sale el estilo.
const fuenteGlifo = "apps/frontend/packages/ui/src/components/Glifo.tsx"

// TestElEstiloEsElDeGlifo: los tres números del estilo, contra el `.tsx` que los
// decide. Si la web cambia el grosor o la opacidad del relleno y aquí no, los iconos
// nativos dejan de ser los de la web sin que ningún otro test lo diga.
func TestElEstiloEsElDeGlifo(t *testing.T) {
	web := webref.Read(t, fuenteGlifo)
	for _, trozo := range []string{
		"const REJILLA = 24;",
		"const TRAZO_PX = 1.5;",
		`fill-opacity="0.34"`,
		`fill-rule="evenodd"`,
		// El redondeo a centésimas de `grosorDeGlifo`, que `grosorTrazo` repite.
		"Math.round(((TRAZO_PX * REJILLA) / size) * 100) / 100",
	} {
		if !strings.Contains(web, trozo) {
			t.Errorf("%s ya no dice %q: el estilo de la web ha cambiado y hay que traerlo aquí", fuenteGlifo, trozo)
		}
	}
	if rejilla != 24 || grosorPx != 1.5 || opacidadAcento != 0.34 {
		t.Errorf("aquí la rejilla es %d, el trazo %g px y el relleno al %g: en la web son 24, 1,5 y 0,34",
			rejilla, grosorPx, opacidadAcento)
	}
	for lado, quiero := range map[float32]float64{14: 2.57, 16: 2.25, 18: 2, 20: 1.8, 24: 1.5, 32: 1.13} {
		if got := grosorTrazo(lado); got != quiero {
			t.Errorf("grosorTrazo(%g) = %g y `grosorDeGlifo` da %g", lado, got, quiero)
		}
	}
}

// TestLoQueSeRellenaEsLoDeLaWeb pasa por `subtrazosCerrados` y `rellenoDeCuerpo` los
// MISMOS casos que `Glifo.test.tsx`, con los mismos resultados: es la misma lógica
// escrita dos veces, y esto es lo que impide que diverja.
func TestLoQueSeRellenaEsLoDeLaWeb(t *testing.T) {
	web := webref.Read(t, "apps/frontend/packages/ui/src/components/Glifo.test.tsx")
	for _, c := range []struct{ d, quiero string }{
		// La lupa: el aro se cierra, el mango no.
		{"M11 4a7 7 0 1 0 0 14 7 7 0 0 0 0-14ZM20 20l-3.8-3.8", "M11 4a7 7 0 1 0 0 14 7 7 0 0 0 0-14Z"},
		// Línea pura: nada.
		{"M5 8.5l7 7 7-7", ""},
		{"M7 7h10v10H7Z M4 12h1", "M7 7h10v10H7Z"},
		// Pares sueltos tras la `M`: no se pegan a ella.
		{"M12 3.5 4.5 6.6v5.4L12 3.5Z", "M12 3.5 4.5 6.6v5.4L12 3.5Z"},
		// La `m` que abre el trazado es absoluta.
		{"m12 3 2.9 5.9-5.8 0Z", "M12 3l2.9 5.9-5.8 0Z"},
		// Tras un `Z` la pluma vuelve al inicio del subtrazo cerrado.
		{
			"M6 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6Zm12 7a3 3 0 1 0 0-6 3 3 0 0 0 0 6Z",
			"M6 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6Z M18 22a3 3 0 1 0 0-6 3 3 0 0 0 0 6Z",
		},
		{"M4 5h16v14H4Zm-1 2 9 6 9-6", "M4 5h16v14H4Z"},
		// El techo: una cerrada con `m` relativa tras un subtrazo abierto.
		{"M4 4h6m2 2h4v4h-4Z", ""},
	} {
		if got := subtrazosCerrados(c.d); got != c.quiero {
			t.Errorf("subtrazosCerrados(%q)\n  da     %q\n  quiero %q", c.d, got, c.quiero)
		}
		// Y que el caso sea de verdad el de la web, no uno parecido.
		if !strings.Contains(web, "'"+c.d+"'") {
			t.Errorf("el caso %q no está en Glifo.test.tsx: los dos tests tienen que probar lo mismo", c.d)
		}
	}

	for _, c := range []struct{ cuerpo, quiero string }{
		{
			// El ojo de Pages: contorno y pupila en un solo trazado.
			`<path d="M2 12s3-7 10-7 10 7 10 7-3 7-10 7-10-7-10-7Z" /><circle cx="12" cy="12" r="3" />`,
			"M2 12s3-7 10-7 10 7 10 7-3 7-10 7-10-7-10-7Z M9 12a3 3 0 1 0 6 0a3 3 0 1 0 -6 0Z",
		},
		{
			`<rect x="9" y="9" width="11" height="11" rx="2" /><polygon points="12 2 2 7 12 12 22 7 12 2" />` +
				`<polyline points="2 17 12 22 22 17" /><line x1="4" y1="4" x2="8" y2="8" />` +
				`<circle cx="19" cy="6" r="2.4" fill="currentColor" stroke="none" />`,
			"M11 9h7a2 2 0 0 1 2 2v7a2 2 0 0 1 -2 2h-7a2 2 0 0 1 -2 -2v-7a2 2 0 0 1 2 -2Z M12 2 2 7 12 12 22 7 12 2Z",
		},
		{
			// Lo de dentro de un `<g>` no se rellena: va en otro sistema de coordenadas.
			`<path d="M4 4h4v4H4Z" /><g transform="scale(0.5)"><path d="M0 0h8v8H0Z" /></g>`,
			"M4 4h4v4H4Z",
		},
	} {
		if got := rellenoDeCuerpo(c.cuerpo); got != c.quiero {
			t.Errorf("rellenoDeCuerpo(%q)\n  da     %q\n  quiero %q", c.cuerpo, got, c.quiero)
		}
	}
}

// sobreBlanco es el color que deja `c` al `alfa` sobre el blanco del lienzo de pruebas.
func sobreBlanco(c color.NRGBA, alfa float64) color.NRGBA {
	c.A = uint8(alfa*255 + 0.5)
	return tokens.SobreSRGB(c, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
}

// TestLasFormasCerradasLlevanElAcento: lo PINTADO. Dentro de una forma cerrada hay
// relleno del primario al 34 %; un glifo de línea pura no lleva nada; y una forma
// dentro de otra queda como hueco (par-impar).
//
// VISTO EN ROJO (2026-10-04, en el frontal): con `capa = ""` en `svgDe`, el escudo y
// el ojo dicen que su interior es blanco. Deshecho.
func TestLasFormasCerradasLlevanElAcento(t *testing.T) {
	const lado = 24
	blanco := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	acento := sobreBlanco(tema.Color("primary", tema.Claro), opacidadAcento)
	en := func(d *Dibujo, x, y float32) color.NRGBA {
		return px(pinta(t, lado, d), int(x*escala), int(y*escala))
	}

	// El escudo, por dentro.
	if got := en(Trazo(Escudo, lado, "text", tema.Claro), 12, 12); !cerca(got, acento) {
		t.Errorf("el interior del escudo pinta %v y el primario al 34 %% sobre blanco es %v", got, acento)
	}
	// El aspa no encierra nada: entre sus brazos, blanco.
	if got := en(Trazo(Cerrar, lado, "text", tema.Claro), 12, 7); !cerca(got, blanco) {
		t.Errorf("el aspa pinta %v entre sus brazos: un glifo de línea pura no lleva relleno", got)
	}
	// El ojo: relleno entre el párpado y la pupila, y la pupila HUECA.
	ojo := Trazo(Ojo, lado, "text", tema.Claro)
	if got := en(ojo, 7.5, 12); !cerca(got, acento) {
		t.Errorf("entre el párpado y la pupila el ojo pinta %v y debería llevar el acento %v", got, acento)
	}
	if got := en(ojo, 12, 12); !cerca(got, blanco) {
		t.Errorf("el centro de la pupila pinta %v: el par-impar no ha abierto el hueco", got)
	}
}

// TestElAcentoLoDecideQuienPoneElIcono: el `--sytue-glyph-fill` de la web. Por defecto
// el primario; `Acento` lo cambia (el carril); `AcentoTinta` lo ata a la tinta, que es
// lo que lleva un icono sobre un relleno de color, y la sigue si se tiñe.
func TestElAcentoLoDecideQuienPoneElIcono(t *testing.T) {
	const lado = 24
	centro := func(d *Dibujo) color.NRGBA { return px(pinta(t, lado, d), int(12*escala), int(12*escala)) }

	ambar := color.NRGBA{R: 0xff, G: 0xd4, B: 0x5e, A: 0xff}
	d := Trazo(Escudo, lado, "text", tema.Claro).Acento(ambar)
	if got, quiero := centro(d), sobreBlanco(ambar, opacidadAcento); !cerca(got, quiero) {
		t.Errorf("con Acento(ámbar) el escudo pinta %v por dentro y debería ser %v", got, quiero)
	}

	d = Trazo(Escudo, lado, "text", tema.Claro).AcentoTinta()
	if got, quiero := centro(d), sobreBlanco(tema.Color("text", tema.Claro), opacidadAcento); !cerca(got, quiero) {
		t.Errorf("con AcentoTinta el escudo pinta %v por dentro y la tinta al 34 %% es %v", got, quiero)
	}
	d.Tenir("danger", tema.Claro)
	if got, quiero := centro(d), sobreBlanco(tema.Color("danger", tema.Claro), opacidadAcento); !cerca(got, quiero) {
		t.Errorf("tras teñir, el relleno pinta %v y debería seguir a la tinta: %v", got, quiero)
	}

	// La forma LLENA (la estrella destacada) no lleva capa: es tinta entera.
	lleno := TrazoLleno(Escudo, lado, "text", tema.Claro)
	if got, quiero := centro(lleno), tema.Color("text", tema.Claro); !cerca(got, quiero) {
		t.Errorf("el escudo lleno pinta %v por dentro y la tinta es %v", got, quiero)
	}
}

// TestSobreUnRellenoDeColorElAcentoEsLaTinta: la regla de `styles.css`, reconocida igual
// que allí, por la tinta de quien escribe encima de un color. Sin ella el tic de un
// paso o el glifo blanco de una tesela llevarían el primario debajo: invisible sobre el
// primario y sucio sobre cualquier otro color.
func TestSobreUnRellenoDeColorElAcentoEsLaTinta(t *testing.T) {
	web := webref.Read(t, "apps/frontend/packages/ui/src/styles.css")
	for _, tinta := range []string{`.text-white`, `.text-\[var\(--sytue-on-primary\)\]`} {
		if !strings.Contains(web, tinta) {
			t.Errorf("styles.css ya no reconoce la tinta %s: la regla de la web ha cambiado", tinta)
		}
	}
	blanco := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	// `on-primary` se compara con el del tema en curso, que es el que se está pintando.
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	sobre := tema.Color("on-primary", v)
	if got := Trazo(Tic, 16, "on-primary", v).acentoDeAhora(); got != sobre {
		t.Errorf("con tinta `on-primary` el acento es %v y debería ser la propia tinta %v", got, sobre)
	}
	if got := TrazoConTinta(Escudo, 22, blanco).acentoDeAhora(); got != blanco {
		t.Errorf("con tinta blanca el acento es %v y debería ser blanco", got)
	}
	// Y la tinta neutra, que es el caso general, lleva el primario.
	if got, quiero := Trazo(Escudo, 20, "text", tema.Claro).acentoDeAhora(), tema.Color("primary", tema.Claro); got != quiero {
		t.Errorf("con tinta `text` el acento es %v y debería ser el primario %v", got, quiero)
	}
}

// TestElCuerpoSeRellenaComoLaRuta: un glifo escrito con figuras (`TrazoDeCuerpo`) lleva
// la misma capa que uno escrito con una `d`.
func TestElCuerpoSeRellenaComoLaRuta(t *testing.T) {
	const lado = 24
	negro := color.NRGBA{A: 255}
	// El «!» de prioridad de Rooms: un cuadro redondeado con el signo dentro.
	d := TrazoDeCuerpo(`<rect x="3.5" y="3.5" width="17" height="17" rx="4" /><path d="M12 8v4.5" />`, lado, negro)
	got := px(pinta(t, lado, d), int(7*escala), int(7*escala))
	if quiero := sobreBlanco(tema.Color("primary", tema.Claro), opacidadAcento); !cerca(got, quiero) {
		t.Errorf("dentro del cuadro pinta %v y debería llevar el acento %v", got, quiero)
	}
}

// TestConTintaDeColorElRellenoSigueALaTinta: la decisión del dueño (2026-10-04). Un
// icono rojo de peligro o verde de éxito sobre fondo neutro lleva el relleno de SU
// color, no el de marca. La web lo reconoce por la utilidad; aquí, por la croma.
func TestConTintaDeColorElRellenoSigueALaTinta(t *testing.T) {
	web := webref.Read(t, "apps/frontend/packages/ui/src/styles.css")
	for _, tinta := range []string{"danger", "success", "warning", "info"} {
		if !strings.Contains(web, "[class*='text-"+tinta+"']") {
			t.Errorf("styles.css ya no reconoce la tinta de %s: la regla de la web ha cambiado", tinta)
		}
	}
	for _, v := range []tema.Variante{tema.Claro, tema.Oscuro, tema.Translucido} {
		for _, token := range []string{"danger-text", "danger", "success-text", "warning-text", "product-pdf", "product-sheets"} {
			if !tintaDeColor(tema.Color(token, v)) {
				t.Errorf("tema %v: `%s` (%v) se da por neutra y es una tinta de color", v, token, tema.Color(token, v))
			}
		}
		for _, token := range []string{"text", "text-muted", "text-subtle", "sidebar-fg", "sidebar-fg-muted", "header-fg"} {
			if tintaDeColor(tema.Color(token, v)) {
				t.Errorf("tema %v: `%s` (%v) se da por tinta de color y es neutra: perdería el relleno de marca",
					v, token, tema.Color(token, v))
			}
		}
	}
	// Y lo pintado: el escudo en rojo de peligro, por dentro, es ese rojo al 34 %.
	rojo := tema.Color("danger-text", tema.Claro)
	got := px(pinta(t, 24, Trazo(Escudo, 24, "danger-text", tema.Claro)), int(12*escala), int(12*escala))
	if quiero := sobreBlanco(rojo, opacidadAcento); !cerca(got, quiero) {
		t.Errorf("el escudo en `danger-text` pinta %v por dentro y su tinta al 34 %% es %v", got, quiero)
	}
}

// TestLosPuntosVanMacizos: los de «…» y las viñetas. Con el trazo de 1,5 px un tramo de
// longitud cero es una mota; van como círculos de radio 1,25 de la rejilla.
func TestLosPuntosVanMacizos(t *testing.T) {
	web := webref.Read(t, fuenteGlifo)
	if !strings.Contains(web, "const RADIO_PUNTO = 1.25;") || radioPunto != 1.25 {
		t.Errorf("el radio del punto aquí es %g y Glifo.tsx ya no dice 1,25", radioPunto)
	}
	const quiero = "M4.75 12a1.25 1.25 0 1 0 2.5 0a1.25 1.25 0 1 0 -2.5 0Z M10.75 12a1.25 1.25 0 1 0 2.5 0a1.25 1.25 0 1 0 -2.5 0Z"
	if got := puntos("M6 12h.01M12 12h.01"); got != quiero {
		t.Errorf("puntos da\n  %q\ny quiero\n  %q", got, quiero)
	}
	if got := puntos("M4 6h16M4 12h.5"); got != "" {
		t.Errorf("un tramo de verdad no es un punto, y sale %q", got)
	}
	if got := puntosDeCuerpo(`<path d="M20 13L12 22Z" /><path d="M7.5 7.5h.01" />`); got != "M6.25 7.5a1.25 1.25 0 1 0 2.5 0a1.25 1.25 0 1 0 -2.5 0Z" {
		t.Errorf("puntosDeCuerpo da %q", got)
	}
	// Pintado: a 24 px el punto central de «…» mide 2,5 px de ancho. El píxel de
	// dispositivo que va de 0,5 a 1 px del centro es tinta entera, y con el tramo de
	// 1,5 px (0,75 de radio) se quedaría a medias.
	img := pinta(t, 24, Trazo(Puntos, 24, "text", tema.Claro))
	if got, tinta := px(img, int(12*escala)+1, int(12*escala)), tema.Color("text", tema.Claro); !cerca(got, tinta) {
		t.Errorf("entre 0,5 y 1 px del centro del punto pinta %v y debería ser tinta %v: el punto no es macizo", got, tinta)
	}
}

// TestDentroDelCarrilElRellenoEsSuAmbar: un icono del menú lateral lleva el relleno del
// carril (`sidebar-indicator`), que es el `--sytue-glyph-fill` que appearance.css
// reasigna para todo lo que cuelga de `bg-sidebar`. Lo dice el token de la tinta en
// `Trazo`, `DeToken` en los glifos propios de una app o `EnCarril` a mano; la tinta
// atenuada del carril se reconoce sola. Y fuera del carril nada cambia.
//
// VISTO EN ROJO sin la regla del carril en `acentoDeAhora`: en claro la tinta del menú
// es el blanco puro y el relleno salía blanco, y en oscuro, del primario del tema.
func TestDentroDelCarrilElRellenoEsSuAmbar(t *testing.T) {
	for _, v := range []tema.Variante{tema.Claro, tema.Oscuro, tema.Translucido} {
		nombre := map[tema.Variante]string{tema.Claro: "claro", tema.Oscuro: "oscuro", tema.Translucido: "traslucido"}[v]
		tema.Elegir(nombre)
		ambar := tema.Color("sidebar-indicator", v)
		for que, d := range map[string]*Dibujo{
			"Trazo con sidebar-fg":           Trazo(Buscar, 18, "sidebar-fg", v),
			"Trazo con sidebar-active-fg":    Trazo(Buscar, 18, "sidebar-active-fg", v),
			"TrazoLleno con sidebar-fg":      TrazoLleno(Buscar, 18, "sidebar-fg", v),
			"un glifo de app con DeToken":    TrazoDeRuta("M4 5h16v14H4Z", 18, tema.Color("sidebar-fg", v)).DeToken("sidebar-fg"),
			"la tinta atenuada, sin decirlo": TrazoConTinta(Buscar, 18, tema.Color("sidebar-fg-muted", v)),
			"EnCarril a mano":                TrazoConTinta(Buscar, 18, tema.Color("text", v)).EnCarril(),
		} {
			d.mu.Lock()
			got := d.acentoDeAhora()
			d.mu.Unlock()
			if got != ambar {
				t.Errorf("%s, %s: el relleno es %v y el ámbar del carril es %v", nombre, que, got, ambar)
			}
		}
		fuera := Trazo(Buscar, 18, "text", v)
		fuera.mu.Lock()
		got := fuera.acentoDeAhora()
		fuera.mu.Unlock()
		if got != tema.Color("primary", v) {
			t.Errorf("%s: fuera del carril el relleno es %v y sigue siendo el primario %v", nombre, got, tema.Color("primary", v))
		}
	}
	tema.Elegir("")
}
