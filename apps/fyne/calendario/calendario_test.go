// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"

	"git.sytue.eu/sytue/one/apps/fyne/agenda"
	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Los guardianes de Sytue Calendar.
//
// Comparan contra los números MEDIDOS en producción (`test/e2e/medidas-web.mjs
// calendar`, corporate.sytue.eu a 1280×800 con el rem en 14), no contra lo que calcula
// este mismo código: un test que compara la implementación consigo misma pasa siempre.
//
// Corren SIN CGO y SIN servidor gráfico, con el pintor de software.
func TestMain(m *testing.M) {
	test.NewApp().Settings().SetTheme(tema.Sytue{})
	tema.FormaDeLosGuardianes()
	os.Exit(m.Run())
}

// CalendarApp y CalendarGrid reales en Chromium a 1280×800, raíz de 16, medidos
// el 2026-10-04. Cambiar uno de estos números
// es decir que la web ha cambiado, y entonces hay que volver a medir.
var (
	ventana   float32 = 1280
	altoUtil  float32 = 744  // lo que queda bajo la topbar, que en nativo no existe
	anchoCont float32 = 1020 // el contenido: 1280 − 260 del menú
	altoCab   float32 = 53   // la cabecera de vista
	altoDias  float32 = 32   // la fila de nombres de día
	// altoSem es cada una de las seis semanas, y colIzq el ancho de columna que el
	// navegador da tras truncar a 1/64 (`floor(1020/7 × 64)/64`).
	altoSem float32 = 109.671875
	colIzq  float32 = 145.703
)

// Los mismos, en fila de PÍXEL, para indexar la captura. Van aparte porque Go no
// convierte a int una constante con decimales, y porque escribirlos dos veces sería
// justo la copia que estos guardianes existen para cazar.
var (
	filaFileteCab = int(altoCab) - 1
	// El `border-r` de la primera celda ocupa [404,703 · 405,703). Fyne cuantiza CADA
	// BORDE del rectángulo al píxel de dispositivo —no el rectángulo entero—, así que
	// a ×1 va de round(404,703)=405 a round(405,703)=406: se pinta el 405 y solo ese.
	// Se comprobó mirando el PNG, que es donde se vio que el 404 lleva el tinte de
	// «fuera del mes» y el 406 ya es la superficie de la celda siguiente.
	colPrimeraRaya = int(math.Round(float64(float32(tema.AnchoLateral) + colIzq - 1)))
)

// traductor carga el catálogo embebido de la app, que es lo que usa la pantalla.
func traductor(t *testing.T) *i18n.Translator {
	t.Helper()
	tr, err := i18n.New("es", i18n.Common(), catalogos.Calendar())
	if err != nil {
		t.Fatalf("no se pudo cargar el catálogo: %v", err)
	}
	return tr
}

// TestLaCabeceraDeCalendarMide53 ata la cabecera de vista al número de
// producción. No son los 48 del `min-h-12`: esta barra mete botones de 32, así que
// manda el contenido —y esa es justo la diferencia con la de Mail.
func TestLaCabeceraDeCalendarMide53(t *testing.T) {
	if got := AltoCabeceraVista(); got != altoCab {
		t.Errorf("la cabecera mide %v y producción dice %v", got, altoCab)
	}
	if AltoCabeceraVista() <= ui.MinCabeceraApp {
		t.Errorf("con un botón de %v dentro tiene que pasar del min-h-12 (%v)",
			AltoBotonCabecera, ui.MinCabeceraApp)
	}
}

// TestElPieDeEventoCabeEnElTelefono cubre el pie más ancho, al editar un evento:
// Eliminar + Cancelar + Guardar. Antes la fila derecha podía empezar en X negativa
// o solaparse con Eliminar. Los botones de44 reproducen su objetivo táctil en iOS.
func TestElPieDeEventoCabeEnElTelefono(t *testing.T) {
	for _, locale := range []i18n.Locale{"es", "de", "fr"} {
		tr, err := i18n.New(locale, i18n.Common(), catalogos.Calendar())
		if err != nil {
			t.Fatal(err)
		}
		for _, ventana := range []float32{320, 375, 390, 640} {
			f := &formulario{p: &Pantalla{T: tr}}
			pie := f.pie().(*fyne.Container)
			for _, o := range pie.Objects {
				o.(*ui.Boton).Talla = ui.TallaLG
			}
			// El aire exterior y el px-6 del panel no son parte del pie útil.
			ancho := ventana - 2*ui.MargenDialogo - 2*tema.Espacio(6)
			alto := ui.AltoA(pie, ancho)
			pie.Resize(fyne.NewSize(ancho, alto))
			for i, o := range pie.Objects {
				p, s := o.Position(), o.Size()
				if p.X < 0 || p.Y < 0 || p.X+s.Width > ancho+0.01 || p.Y+s.Height > alto+0.01 {
					t.Errorf("%s a %g: botón%d ocupa %v/%v fuera del pie de%gx%g", locale, ventana, i, p, s, ancho, alto)
				}
				if i > 0 {
					anterior := pie.Objects[i-1]
					if p.Y == anterior.Position().Y && p.X < anterior.Position().X+anterior.Size().Width {
						t.Errorf("%s a %g: el botón%d se solapa con el anterior", locale, ventana, i)
					}
				}
			}
			if ventana == 320 && alto <= 44 {
				t.Errorf("%s: los tres botones no caben en una fila de%g; el pie debe crecer", locale, ancho)
			}
			if ventana == 640 && alto != 44 {
				t.Errorf("%s: con sitio el pie debe conservar una fila, mide%g", locale, alto)
			}
		}
	}
}

// TestLaColumnaDelMesEsLaDelNavegadorYNoUnaDivision es el guardián del reparto.
//
// 1020/7 son 145,714285, y producción NO mide eso: mide 145,703, que es
// `floor(145,714285 × 64)/64`. El navegador trunca a 1/64 y acumula desde ahí.
// Dividir sin truncar da 145,714 y redondear da 146: las dos están mal, y las dos se
// han escrito antes en este repositorio.
func TestLaColumnaDelMesEsLaDelNavegadorYNoUnaDivision(t *testing.T) {
	got := Cuadricula.AnchoColumna(anchoCont)
	if !cerca(got, colIzq, 0.001) {
		t.Errorf("la columna mide %v y producción dice %v", got, colIzq)
	}
	if got == anchoCont/7 {
		t.Errorf("la columna es la división exacta (%v): falta el truncado a 1/64", got)
	}
	// La séptima columna arranca donde el navegador la pone, acumulando el ancho YA
	// cuantizado y no multiplicando el exacto.
	if x, quiero := Cuadricula.X(anchoCont, 6), 6*colIzq; !cerca(x, quiero, 0.02) {
		t.Errorf("la séptima columna empieza en %v y debería en %v", x, quiero)
	}
	// Y el residuo medido: Blink reparte los 5/64 que sobran y su última columna
	// llega a 1019,984; la nuestra deja las siete iguales y se queda en 1019,922.
	// El desvío es conocido y está escrito en `mes.go`; el guardián lo FIJA para que
	// no crezca sin que nadie se entere.
	resto := anchoCont - 7*Cuadricula.AnchoColumna(anchoCont)
	if resto > 0.08 {
		t.Errorf("la rejilla deja %v sin cubrir; medido en Blink: 0,0156", resto)
	}
}

// TestLasSeisSemanasRepartenElAltoMedido: bajo la fila de días y su filete quedan
// 658, que Blink reparte en seis filas de 109.671875.
func TestLasSeisSemanasRepartenElAltoMedido(t *testing.T) {
	alto := altoUtil - altoCab
	if got := AltoDiasSemana; got != altoDias {
		t.Errorf("la fila de nombres de día mide %v y producción dice %v", got, altoDias)
	}
	if got := AltoSemana(alto); !cerca(got, altoSem, 0.01) {
		t.Errorf("cada semana mide %v y producción dice %v", got, altoSem)
	}
}

// TestLaRejillaDelMesEmpiezaEnLunesYTrae42Dias comprueba el `monthGrid` de dates.ts:
// seis semanas desde el lunes de la semana del día 1.
func TestLaRejillaDelMesEmpiezaEnLunesYTrae42Dias(t *testing.T) {
	// Septiembre de 2026 empieza en martes, así que la rejilla arranca el 31/08.
	cursor := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	dias := Dias(cursor, time.UTC)
	if len(dias) != 42 {
		t.Fatalf("la rejilla trae %d días y la web pinta 42", len(dias))
	}
	if dias[0].Weekday() != time.Monday {
		t.Errorf("la rejilla empieza en %v y la semana de la suite empieza en lunes", dias[0].Weekday())
	}
	if got, quiero := dias[0].Format(time.DateOnly), "2026-08-31"; got != quiero {
		t.Errorf("la rejilla empieza el %s; en la captura de producción el primer día es %s", got, quiero)
	}
	if got, quiero := dias[41].Format(time.DateOnly), "2026-10-11"; got != quiero {
		t.Errorf("la rejilla acaba el %s y en la captura acaba el %s", got, quiero)
	}
}

// TestLaPantallaSePintaConSusMedidas pinta Calendar entero con el pintor de software
// y comprueba en los PÍXELES lo que las medidas declaran: el menú de 260, el filete de
// la cabecera a 53 y la primera línea vertical de la rejilla en la columna medida.
func TestLaPantallaSePintaConSusMedidas(t *testing.T) {
	img := pintar(t, pantallaDeMuestra(t))

	// 1. El carril mide 260 y su filete está en el píxel 259.
	if got, quiero := px(img, 100, 300), tema.Color("sidebar-bg", tema.Claro); !mismoColor(got, quiero) {
		t.Errorf("dentro del menú (100,300) se pinta %v y el token `sidebar-bg` es %v", got, quiero)
	}
	if got, quiero := px(img, 300, 300), tema.Color("surface", tema.Claro); !mismoColor(got, quiero) {
		t.Errorf("a la derecha del menú (300,300) se pinta %v y debería ser la superficie %v", got, quiero)
	}

	// 2. El filete de la cabecera de 53 ocupa el píxel 52.
	borde := tema.Color("border", tema.Claro)
	if got := px(img, 700, filaFileteCab); !mismoColor(got, borde) {
		t.Errorf("el filete de la cabecera en y=%d pinta %v y el token `border` es %v",
			filaFileteCab, got, borde)
	}

	// 3. La primera línea vertical de la rejilla cae en el borde derecho de la
	//    primera columna: 260 + 145,703 − 1.
	x := colPrimeraRaya
	y := int(altoCab+AltoDiasSemana) + 40
	if got := px(img, x, y); !mismoColor(got, borde) {
		t.Errorf("la línea de la primera columna en x=%d pinta %v y debería ser el borde %v", x, got, borde)
	}

	guardar(t, img, "calendario-mes")
}

// TestLosSeisEstadosSePintan: toda pantalla de la suite tiene que saber pintar sus
// seis estados, y ninguno puede quedarse en blanco.
func TestLosSeisEstadosSePintan(t *testing.T) {
	for _, c := range []struct {
		nombre string
		e      Estado
	}{
		{"carga", agenda.Cargando},
		{"vacio", agenda.Vacia},
		{"error", agenda.Error},
		{"sin-conexion", agenda.SinConexion},
		{"sin-permiso", agenda.SinPermiso},
		{"exito", agenda.Exito},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			p := pantallaDeMuestra(t)
			p.Estado = c.e
			p.SinConexion = c.e == agenda.SinConexion
			img := pintar(t, p)
			if !hayTinta(img, int(tema.AnchoLateral)+20, filaFileteCab+20, int(ventana)-20, int(altoUtil)-20) {
				t.Errorf("el estado %q deja el contenido en blanco", c.nombre)
			}
		})
	}
}

// TestElEstadoSinPermisoNoOfreceReintentar: un 403 no se arregla reintentando, así que
// ese estado no lleva botón. El guardián existe porque la diferencia entre «error» y
// «sin permiso» es justo esa y es fácil de perder al refactorizar.
func TestElEstadoSinPermisoNoOfreceReintentar(t *testing.T) {
	p := pantallaDeMuestra(t)
	p.Estado = agenda.SinPermiso
	if got := p.textosDeEstado(); got.RotuloAccion != "" {
		t.Errorf("«sin permiso» ofrece la acción %q y no debería ofrecer ninguna", got.RotuloAccion)
	}
	p.Estado = agenda.Error
	if got := p.textosDeEstado(); got.RotuloAccion == "" {
		t.Error("«error» tiene que ofrecer «Reintentar»")
	}
}

// TestSinConexionElReintentoEstaApagado: sin red la app pasa a consulta, y el botón
// está pero inerte (`disabled={offline}`), no ausente.
func TestSinConexionElReintentoEstaApagado(t *testing.T) {
	p := pantallaDeMuestra(t)
	p.Estado = agenda.SinConexion
	got := p.textosDeEstado()
	if got.RotuloAccion == "" {
		t.Fatal("sin conexión el botón de reintentar sigue estando, solo que apagado")
	}
	if got.Accion != nil {
		t.Error("sin conexión el botón de reintentar no puede hacer nada")
	}
}

// TestLosTextosSalenDelCatalogoYNoDeLaClave comprueba que las claves que la pantalla
// pide EXISTEN en el catálogo generado desde la web. `Translator.T` devuelve la propia
// clave cuando no la encuentra, así que una clave inventada se ve en pantalla como
// «nav.createEvent» — y eso es lo que caza esto.
func TestLosTextosSalenDelCatalogoYNoDeLaClave(t *testing.T) {
	tr := traductor(t)
	for _, clave := range []string{
		"nav.createEvent", "nav.myCalendars", "nav.noCalendars", "nav.prev", "nav.next",
		"nav.prevMonth", "nav.nextMonth", "nav.showCalendar", "nav.calendarOptions",
		"nav.permReader", "nav.permWriter",
		"header.today", "header.invitations",
		"view.month", "view.week", "view.day", "view.agenda",
		"grid.more", "event.untitled", "cal.create", "cal.sharedWithMe", "cal.noUpcoming",
		"cal.error.loadEvents", "cal.error.loadCalendars", "load.error", "common.retry",
		"search.events", "subscribe.byUrl", "resources.manage", "agenda.empty", "agenda.noMatches",
	} {
		if got := tr.T(clave, i18n.Vars{"count": 1, "name": "x", "label": "x"}); got == clave {
			t.Errorf("la clave %q no está en el catálogo de la app: saldría cruda en pantalla", clave)
		}
	}
}

// ---- utilidades de los guardianes ----

// pantallaDeMuestra es la misma escena que la captura de producción: un calendario
// «Personal» y los eventos de septiembre de 2026.
func pantallaDeMuestra(t *testing.T) *Pantalla {
	t.Helper()
	cursor := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	cal := api.Calendar{ID: "personal", DisplayName: "Personal", Color: "#1a6fbb"}
	evento := func(dia int, titulo string) Ocurrencia {
		inicio := time.Date(2026, 9, dia, 10, 0, 0, 0, time.UTC)
		return Ocurrencia{
			Instance: api.Instance{
				Event: api.Event{ID: titulo, Summary: titulo, DTStart: inicio},
				Start: inicio, End: inicio.Add(time.Hour),
			},
			Color: AcentoPorDefecto, CalendarioID: cal.ID,
		}
	}
	return &Pantalla{
		T: traductor(t), Cursor: cursor, Zona: time.UTC, Vista: VistaMes,
		Calendarios: []api.Calendar{cal},
		Eventos: []Ocurrencia{
			evento(11, "Reunión de producto"), evento(11, "Revisión de infraestructura"),
			evento(12, "Comité de seguridad"), evento(14, "Alta de cliente nuevo"),
		},
	}
}

// pintar captura la pantalla con el pintor de software, a la ventana de producción.
func pintar(t *testing.T, p *Pantalla) image.Image {
	t.Helper()
	w := test.NewWindow(p.Construir())
	t.Cleanup(w.Close)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(ventana, altoUtil))
	return w.Canvas().Capture()
}

func px(img image.Image, x, y int) color.NRGBA {
	r, g, b, a := img.At(x, y).RGBA()
	return color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}
}

// mismoColor admite ±1 nivel de 255: la cuantización del alfa deja esa holgura y
// ningún gate de este módulo puede exigir byte a byte (ADR 0048).
func mismoColor(a, b color.NRGBA) bool {
	d := func(x, y uint8) bool { return int(x)-int(y) > 1 || int(y)-int(x) > 1 }
	return !d(a.R, b.R) && !d(a.G, b.G) && !d(a.B, b.B)
}

func cerca(a, b, tol float32) bool {
	if a > b {
		a, b = b, a
	}
	return b-a <= tol
}

// hayTinta dice si en ese rectángulo se ha pintado ALGO que no sean los dos fondos de
// la suite. Es lo que distingue «el estado se pinta» de «el estado deja la pantalla en
// blanco».
//
// La referencia son los TOKENS y no el píxel de la esquina, que es como este guardián
// dio un falso verde: si la esquina cae encima de un bloque del esqueleto, todo lo
// demás «se parece al fondo» y el test dice que no hay nada pintado.
func hayTinta(img image.Image, x0, y0, x1, y1 int) bool {
	fondos := []color.NRGBA{
		tema.Color("surface", tema.Claro),
		tema.Color("bg", tema.Claro),
	}
	for y := y0; y < y1; y += 3 {
		for x := x0; x < x1; x += 3 {
			c := px(img, x, y)
			if !mismoColor(c, fondos[0]) && !mismoColor(c, fondos[1]) {
				return true
			}
		}
	}
	return false
}

// guardar deja el PNG cuando se pide con SYTUE_CAPTURAS, que es como se saca la imagen
// que acompaña al PR y la que se mira al lado de la captura de la web.
func guardar(t *testing.T, img image.Image, nombre string) {
	t.Helper()
	dir := os.Getenv("SYTUE_CAPTURAS")
	if dir == "" {
		return
	}
	f, err := os.Create(dir + "/" + nombre + ".png")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	t.Logf("captura en %s/%s.png", dir, nombre)
}

// TestLosIconosDeLaCabeceraSiguenAlModo: la campana y el botón de invitaciones crean su
// icono con la tinta del modo en que nacen, y al cambiar de modo CON LA APP ABIERTA
// tienen que reteñirlo. No lo hacían: en oscuro quedaba el glifo oscuro sobre la
// superficie oscura (visto en ventana real el 2026-10-05).
//
// VISTO EN ROJO quitando el `Tenir` del Refresh de cada uno: la tinta se queda en la del
// modo claro.
func TestLosIconosDeLaCabeceraSiguenAlModo(t *testing.T) {
	t.Cleanup(func() { tema.Elegir("") })
	tema.Elegir("claro")
	inv := nuevoBotonInvitaciones("Invitaciones", 0, "", nil)
	ri := test.WidgetRenderer(inv).(*invitacionesRenderer)
	cam := &botonCampana{nombre: "Recordatorios"}
	cam.ExtendBaseWidget(cam)
	rc := test.WidgetRenderer(cam).(*campanaRenderer)

	tema.Elegir("oscuro")
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	if v != tema.Oscuro {
		t.Fatalf("el modo en vigor es %v y acabo de elegir el oscuro", v)
	}
	inv.Refresh()
	cam.Refresh()
	if got, quiero := ri.icono.Tinta(), tema.Color("text", v); got != quiero {
		t.Errorf("el icono de invitaciones sigue en %v; en oscuro es %v", got, quiero)
	}
	if got, quiero := rc.icono.Tinta(), tema.Color("header-fg", v); got != quiero {
		t.Errorf("el icono de la campana sigue en %v; en oscuro es %v", got, quiero)
	}
}
