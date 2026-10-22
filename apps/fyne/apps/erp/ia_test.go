// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// # Sytue AI: componentes web reales, Chromium/Figtree, raíz16 (2026-10-04).
// Las alturas salen de medidas-vistas.json y medidas-vistas-extra.json.
// Las columnas de Informe se midieron con el ancestro ERP real en medidas-informe-contexto-final.json.
// Los controles y SVG conservan sus clases y atributos del JSX real.

// informeArnes es el tablero «Ventas de 2026» del arnés: una cifra, unas barras con un
// mes a cero y una tabla recortada con las cinco clases de columna.
func informeArnes() (api.ERPInforme, []api.ERPTablaInforme) {
	col := func(clave, tipo, etiqueta string) api.ERPColumnaInforme {
		return api.ERPColumnaInforme{Clave: clave, Tipo: tipo, Etiqueta: etiqueta}
	}
	inf := api.ERPInforme{Titulo: "Ventas de 2026", Partes: []api.ERPParteInforme{
		{Titulo: "Facturado este año", Vista: api.ERPVistaCifra},
		{Titulo: "Ventas por mes", Vista: api.ERPVistaBarras},
		{Titulo: "Facturas pendientes por cliente", Vista: api.ERPVistaTabla},
	}}
	datos := []api.ERPTablaInforme{
		{Columnas: []api.ERPColumnaInforme{col("total", "dinero", "Total")}, Filas: [][]any{{4406.96}}},
		{
			Columnas: []api.ERPColumnaInforme{col("mes", "texto", "Mes"), col("total", "dinero", "Total")},
			Filas:    [][]any{{"2026-06", 1200.0}, {"2026-07", 0.0}, {"2026-08", 2450.5}, {"2026-09", 4406.96}},
		},
		{
			Columnas: []api.ERPColumnaInforme{
				col("cliente", "texto", "Cliente"), col("n", "numero", "Facturas"), col("total", "dinero", "Pendiente"),
				col("vence", "fecha", "Vence"), col("pagada", "booleano", "Pagada"),
			},
			Filas: [][]any{
				{"Construcciones Almagro S.L.", 1.0, 3025.0, "2026-10-15", false},
				{"Talleres Sur", 2.0, 99.0, "2026-10-01", true},
			},
			Recortada: true,
		},
	}
	return inf, datos
}

func informeDePrueba(t *testing.T) *Informe {
	inf, datos := informeArnes()
	tr := traductor(t)
	return &Informe{tr: tr, loc: tr.Locale(), Informe: inf, Datos: datos, DosColumnas: true}
}

// TestLasCeldasDeUnInformeSonLasDeLaWeb: `valor` de SytueIA.tsx con las del arnés. Los
// importes de un informe llegan en UNIDADES.
func TestLasCeldasDeUnInformeSonLasDeLaWeb(t *testing.T) {
	_, d := informeArnes()
	c := d[2].Columnas
	for _, caso := range []struct {
		col    *api.ERPColumnaInforme
		v      any
		quiero string
	}{
		{&d[0].Columnas[0], 4406.96, "4406,96 €"},
		{&c[2], 3025.0, "3025,00 €"},
		{&c[1], 1.0, "1"},
		{&c[1], 2.345, "2,35"},
		{&c[3], "2026-10-15", "15 oct 2026"},
		{&c[4], true, "✓"},
		{&c[4], false, "—"},
		{&d[1].Columnas[0], "2026-09", "sept 2026"},
		{&d[1].Columnas[0], "2026-06", "jun 2026"},
		{&c[0], nil, ""},
		{nil, 2450.5, "2450.5"},
	} {
		if got := ValorInforme(caso.col, caso.v, "es"); got != caso.quiero {
			t.Errorf("valor(%v) = %q; la web %q", caso.v, got, caso.quiero)
		}
	}
}

// TestElInformeMideLoDeLaWeb: la rejilla, las tarjetas, las barras y la tabla del
// tablero a 1280 (contenido de 966) y en el panel (676).
//
// VISTO EN ROJO (frontal, 2026-09-23): con el filete también bajo la última fila —el
// `last:border-b-0` que `ui.Tabla` no tiene—, la tarjeta de la tabla mide 187,25 y no
// 186,25; y truncando el alto de las barras en vez de redondearlo, la de junio mide 32
// y no 33.
func TestElInformeMideLoDeLaWeb(t *testing.T) {
	x := informeDePrueba(t)
	_, cifra := x.tarjetaParte(tema.Claro, x.Informe.Partes[0], 0, 476)
	cerca(t, "la tarjeta de la cifra (sin estirar)", cifra, 106)
	_, barras := x.tarjetaParte(tema.Claro, x.Informe.Partes[1], 1, 476)
	cerca(t, "la tarjeta de las barras", barras, 184)
	_, tabla := x.tarjetaParte(tema.Claro, x.Informe.Partes[2], 2, 966)
	cerca(t, "la tarjeta de la tabla", tabla, 212)
	_, rejilla := x.Objeto(tema.Claro, 966)
	cerca(t, "la rejilla del tablero", rejilla, 412)
	_, rejilla = x.Objeto(tema.Claro, 676)
	cerca(t, "la rejilla del panel", rejilla, 412)

	puntos := []PuntoBarras{{"jun", 1200}, {"jul", 0}, {"ago", 2450.5}, {"sept", 4406.96}}
	pos, tam := geometriaBarras(puntos, 442)
	for i, quiero := range []float32{33, 1, 67, 120} {
		cerca(t, "el alto de una barra", tam[i].Height, quiero)
		cerca(t, "el pie de una barra", pos[i].Y+tam[i].Height, AltoBarras)
	}
	cerca(t, "el ancho de una barra", tam[0].Width, 109.026667)
	cerca(t, "el paso de las barras", pos[1].X, 110.5)

	anchos, _, derecha := x.medirTabla(x.Datos[2], 930)
	for i, quiero := range []float32{330.1875, 149.546875, 155.8125, 168.546875, 125.90625} {
		if d := anchos[i] - quiero; d < -.3 || d > .3 {
			t.Errorf("la columna %d mide %.2f; la web %.2f", i, anchos[i], quiero)
		}
	}
	if derecha[0] || !derecha[1] || !derecha[2] || derecha[3] {
		t.Errorf("las cifras van a la derecha y el resto a la izquierda: %v", derecha)
	}

	// En el móvil la tabla no cabe: cada columna a su mínimo, las celdas en varias
	// líneas y la tabla se desplaza de lado dentro de su caja (medido a 360: la tabla
	// mide457.96875 en una caja de282 y la tarjeta272).
	anchos, _, _ = x.medirTabla(x.Datos[2], 282)
	for i, quiero := range []float32{122.890625, 92.484375, 96.359375, 68.390625, 77.84375} {
		if d := anchos[i] - quiero; d < -.3 || d > .3 {
			t.Errorf("en el móvil la columna %d mide %.2f; la web %.2f", i, anchos[i], quiero)
		}
	}
	_, tablaMovil := x.tarjetaParte(tema.Claro, x.Informe.Partes[2], 2, 318)
	cerca(t, "la tarjeta de la tabla en el móvil", tablaMovil, 272)

	// En el móvil, una columna: cada parte en su fila.
	x.DosColumnas = false
	_, movil := x.Objeto(tema.Claro, 318)
	if movil <= rejilla {
		t.Errorf("en una columna la rejilla mide %.2f, no más que en dos (%.2f)", movil, rejilla)
	}
}

// TestUnaTablaQueNoCabeSaltaDeFila: la colocación de la rejilla es la de CSS por
// defecto, que no vuelve atrás a rellenar el hueco que deja una tabla.
func TestUnaTablaQueNoCabeSaltaDeFila(t *testing.T) {
	x := informeDePrueba(t)
	p := x.Informe.Partes
	x.Informe.Partes = []api.ERPParteInforme{p[0], p[2], p[1]}
	x.Datos = []api.ERPTablaInforme{x.Datos[0], x.Datos[2], x.Datos[1]}
	_, alto := x.Objeto(tema.Claro, 966)
	// Chromium mide106,212 y184, con dos gap-4 de16.
	cerca(t, "cifra, tabla y barras en tres filas", alto, 106+16+212+16+184)
}

// planArnes es el plan del arnés: un contacto que ya existía y una factura nueva a la
// que le falta el vencimiento, con un aviso y la tarea que se creará.
func planArnes() *api.ERPPlan {
	return &api.ERPPlan{
		Resumen: "Voy a crear la factura de Claude Code para Anthropic, que ya está en tus contactos.",
		Avisos:  []string{"El IVA de un servicio digital de fuera de la UE va por inversión del sujeto pasivo."},
		Pasos: []api.ERPPasoPlan{
			{Ref: "p1", Modelo: "contacto", Etiqueta: "Contacto", Existente: &api.ERPRegistroCorto{ID: "c9", Etiqueta: "Anthropic PBC"}, Data: map[string]any{"nombre": "Anthropic PBC"}},
			{
				Ref: "p2", Modelo: "factura_venta",
				Data:    map[string]any{"fecha": "2026-09-23", "total": 18000.0, "notas": "Claude Code, suscripción de septiembre"},
				Enlaces: map[string]string{"cliente": "p1"}, Faltan: []string{"vencimiento"},
				Avisos: []string{"Comprueba el tipo de IVA antes de confirmarla."}, Tarea: "Sube la factura en PDF",
			},
		},
	}
}

// TestElPanelDeSytueAIMideLoDeLaWeb: las pestañas, la petición y el plan del arnés.
//
// VISTO EN ROJO (frontal, 2026-09-23): sin el hueco que el `<textarea>` en línea deja
// debajo, el panel mide 287 y no 292,25.
func TestElPanelDeSytueAIMideLoDeLaWeb(t *testing.T) {
	tr := traductor(t)
	crear := NuevaPestanaIA(tr.T("ai.tabCreate", nil), true, nil).MinSize()
	inf := NuevaPestanaIA(tr.T("ai.tabReport", nil), false, nil).MinSize()
	cerca(t, "«Crear registros»", crear.Width, 112.328125)
	cerca(t, "«Informe»", inf.Width, 69.640625)
	cerca(t, "una pestaña (alto)", crear.Height, 30)

	x := NuevoPanelIA(tr, nil, []Modelo{facturaArnes()})
	x.Ventana = 1280
	_, grupo := x.pestanas(tema.Claro)
	cerca(t, "las pestañas", grupo, 32)
	d := ui.NuevoDialogo("Sytue AI", x.Cuerpo(tema.Claro, anchoCuerpoDialogo(ui.DialogoLG, 1280))).Con(ui.DialogoLG, nil)
	// 342 hasta el diálogo del 2026-10-15: cabecera 61 (no 73) y `pt-3` (no `pt-5`).
	cerca(t, "el panel vacío", ui.MedidaDialogo(d, fyne.NewSize(1280, 800)).Height, 322)
	cerca(t, "el ancho del cuerpo", anchoCuerpoDialogo(ui.DialogoLG, 1280), 670)

	x.Plan = planArnes()
	_, existente := x.paso(tema.Claro, x.Plan.Pasos[0], 676)
	cerca(t, "el paso que ya existe", existente, 46)
	_, nuevo := x.paso(tema.Claro, x.Plan.Pasos[1], 676)
	cerca(t, "el paso nuevo", nuevo, 193.125)
	_, avisos := avisosPlan(tema.Claro, x.Plan.Avisos, 676)
	cerca(t, "los avisos del plan", avisos, 47.125)
	_, plan := x.plan(tema.Claro, 676)
	cerca(t, "el plan", plan, 384.25)
	if !x.Plan.Incompleto() {
		t.Error("con un campo que falta, el plan no se puede crear")
	}
	textos := textosDe(x.Cuerpo(tema.Claro, 676))
	for _, s := range []string{"Facturas de cliente", "Anthropic PBC", "23 sept 2026", "180,00", "Después: Sube la factura en PDF"} {
		if !contiene(textos, s) {
			t.Errorf("el plan no dice «%s»", s)
		}
	}

	// La pestaña del informe, con el informe propuesto: la sección mide 422.
	x.Elegir(PestanaInforme)
	if x.Plan != nil {
		t.Error("cambiar de pestaña tiene que vaciar la otra, como al desmontarla en la web")
	}
	x.Informe = informeDePrueba(t)
	_, seccion := x.informe(tema.Claro, 676)
	cerca(t, "el informe del panel", seccion, 464)
}

// TestRellenarConSytueAIMideLoDeLaWeb: el diálogo de la ficha de una factura a 1280,
// vacío (431) y con el aviso del límite (490,125). Con el diálogo del 2026-10-15 mide 32
// menos que antes: cabecera 61 (no 73), cuerpo `py-3` (no `py-5`) y pie 65 (no 69).
func TestRellenarConSytueAIMideLoDeLaWeb(t *testing.T) {
	x := NuevoRellenarIA(traductor(t), facturaArnes())
	x.AlElegirPDF = func() {}
	ancho := anchoCuerpoDialogo(ui.DialogoMD, 1280)
	cerca(t, "el cuerpo del diálogo", ancho, 510)
	d := ui.NuevoDialogo("Rellenar con Sytue AI", x.Cuerpo(tema.Claro, ancho)).Con(ui.DialogoMD, x.Pie(func() {}))
	cerca(t, "«Rellenar con Sytue AI»", ui.MedidaDialogo(d, fyne.NewSize(1280, 800)).Height, 431)
	x.Fallo = traductor(t).T("ai.limit", nil)
	d.Cuerpo = x.Cuerpo(tema.Claro, ancho)
	cerca(t, "«Rellenar con Sytue AI» con el aviso", ui.MedidaDialogo(d, fyne.NewSize(1280, 800)).Height, 490.125)
}

// TestUnaParteSinDatosLoDice: sin permiso, sin poder leerla y sin filas, cada parte
// avisa en su sitio (medido: 97,73 las dos primeras y 180 la tabla vacía).
func TestUnaParteSinDatosLoDice(t *testing.T) {
	x := informeDePrueba(t)
	x.Datos = []api.ERPTablaInforme{
		{Error: "sin_permiso"},
		{Error: "no_disponible"},
		{Columnas: []api.ERPColumnaInforme{{Clave: "cliente", Tipo: "texto", Etiqueta: "Cliente"}}},
	}
	o, h := x.tarjetaParte(tema.Claro, x.Informe.Partes[0], 0, 331)
	cerca(t, "la parte sin permiso", h, 111.125)
	if !contiene(textosDe(o), "No tienes permiso") {
		t.Error("la parte sin permiso no lo dice")
	}
	_, h = x.tarjetaParte(tema.Claro, x.Informe.Partes[1], 1, 331)
	cerca(t, "la parte que no se pudo leer", h, 111.125)
	_, h = x.tarjetaParte(tema.Claro, x.Informe.Partes[2], 2, 676)
	cerca(t, "la tabla vacía", h, 205)
	x.Datos = nil
	if _, h = x.tarjetaParte(tema.Claro, x.Informe.Partes[0], 0, 331); h >= 111.125 {
		t.Errorf("cargando, la parte lleva solo la rueda: mide %.2f", h)
	}
}

// TestElTableroMideLoDeLaWeb: la pantalla entera de un tablero a 1280.
func TestElTableroMideLoDeLaWeb(t *testing.T) {
	inf, datos := informeArnes()
	x := NuevoTablero(traductor(t), api.ERPTablero{ID: "t1", Titulo: "Ventas de 2026", Informe: inf})
	x.Datos, x.Ventana = datos, 1280
	_, cab := x.cabecera(tema.Claro, 966)
	cerca(t, "la cabecera del tablero", cab, 36)
	cerca(t, "la página del tablero", x.Objeto(tema.Claro).MinSize().Height, 516)
}

// TestElTableroSePinta monta la pantalla con un tablero guardado y el panel de Sytue AI
// abierto encima, guarda los PNG para MIRARLOS y comprueba que el menú lista el tablero y
// que las barras llevan la tinta del acento.
func TestElTableroSePinta(t *testing.T) {
	p, w := pantallaArnes(t, fyne.NewSize(1280, 800))
	defer w.Close()
	inf, datos := informeArnes()
	tb := api.ERPTablero{ID: "t1", Titulo: "Ventas de 2026", Informe: inf}
	p.Tableros = []api.ERPTablero{tb, {ID: "t2", Titulo: "Cobros pendientes", Informe: inf}}
	p.AbrirTablero(tb)
	p.tablero.Datos = datos
	p.Repintar()
	img := w.Canvas().Capture()
	guardarPNG(t, img, "erp-tablero.png")
	if !contiene(textosDe(p.menu.Objeto(tema.Claro)), "Cobros pendientes") {
		t.Error("el menú no lista los tableros guardados")
	}
	// Una barra de septiembre, la más alta: dentro, la tinta del acento.
	r, g, b, _ := img.At(1180, 200).RGBA()
	a := tema.Color("accent", tema.Claro)
	if dif(r>>8, uint32(a.R))+dif(g>>8, uint32(a.G))+dif(b>>8, uint32(a.B)) > 6 {
		t.Errorf("la barra pinta (%d,%d,%d) y el acento es (%d,%d,%d)", r>>8, g>>8, b>>8, a.R, a.G, a.B)
	}

	// .erp-main de la fuente web actual usa --sytue-bg (#f1f6f6 en claro, el lienzo),
	// sin el degradado antiguo. Estos cuatro puntos deben conservar el lienzo.
	for _, c := range []struct {
		x, y    int
		r, g, b uint32
	}{{1270, 5, 241, 246, 246}, {1100, 60, 241, 246, 246}, {800, 10, 241, 246, 246}, {270, 700, 241, 246, 246}} {
		r, g, b, _ := img.At(c.x, c.y).RGBA()
		if dif(r>>8, c.r)+dif(g>>8, c.g)+dif(b>>8, c.b) > 4 {
			t.Errorf("en (%d, %d) el fondo de main es (%d,%d,%d) y la web pinta (%d,%d,%d)", c.x, c.y, r>>8, g>>8, b>>8, c.r, c.g, c.b)
		}
	}

	p.Atajos(w.Canvas(), nil)
	p.AbrirIA()
	guardarPNG(t, w.Canvas().Capture(), "erp-ia-panel.png")
	if !contiene(textosDe(w.Canvas().Overlays().Top()), "Crear registros") {
		t.Error("el botón de Sytue AI no abre el panel")
	}
	// El plan y el informe propuestos, para mirarlos contra la web.
	x := NuevoPanelIA(p.tr, nil, []Modelo{facturaArnes()})
	x.Ventana, x.Plan = 1280, planArnes()
	d := ui.NuevoDialogo("Sytue AI", x.Cuerpo(tema.Claro, 676)).Con(ui.DialogoLG, nil)
	d.Mostrar(w.Canvas())
	guardarPNG(t, w.Canvas().Capture(), "erp-ia-plan.png")
	d.Cerrar()
	x.Elegir(PestanaInforme)
	x.Informe = informeDePrueba(t)
	d = ui.NuevoDialogo("Sytue AI", x.Cuerpo(tema.Claro, 676)).Con(ui.DialogoLG, nil)
	d.Mostrar(w.Canvas())
	guardarPNG(t, w.Canvas().Capture(), "erp-ia-informe.png")
	d.Cerrar()

	w.Resize(fyne.NewSize(360, 740))
	p.Ventana = fyne.NewSize(360, 740)
	p.Repintar()
	guardarPNG(t, w.Canvas().Capture(), "erp-tablero-movil.png")
}

// TestNombreDelExcel: el título sin lo que un sistema de ficheros no admite.
func TestNombreDelExcel(t *testing.T) {
	for titulo, quiero := range map[string]string{
		"Ventas 2026/T3": "Ventas 2026-T3.xlsx", "  ": "Sytue AI.xlsx", `a:b*c?"d"`: "a-b-c-d-.xlsx",
	} {
		if got := NombreExcel(titulo); got != quiero {
			t.Errorf("NombreExcel(%q) = %q; quiero %q", titulo, got, quiero)
		}
	}
}

// TestLosRequisitosDeFacturaSeDespliegan: el `<details>` de la ficha de un gasto y del
// diálogo del banco, plegado (30) y abierto con los requisitos del arnés (213,75), y el
// rótulo lo abre y lo cierra.
//
// VISTO EN ROJO (frontal, 2026-09-23): la ficha lo pintaba siempre plegado y con 30,
// pulsara quien pulsara.
func TestLosRequisitosDeFacturaSeDespliegan(t *testing.T) {
	r := &api.ERPRequisitosFactura{
		Fuente:       "Real Decreto 1619/2012",
		Completa:     []string{"Número y serie", "Fecha de expedición", "NIF del emisor y del destinatario"},
		Simplificada: []string{"Número y serie", "Fecha"},
		Extranjera:   []string{"NIF-IVA del destinatario"},
	}
	f := NuevaFicha(traductor(t), facturaArnes(), nil, nil, nil)
	f.Requisitos = r
	f.AlRequisitos = func() { f.VerRequisitos = !f.VerRequisitos }
	f.ancho = 516 + tema.Borde + 2*FichaPX
	_, cerrado := f.requisitos(tema.Claro)
	cerca(t, "los requisitos plegados", cerrado, 34)
	o, _ := f.requisitos(tema.Claro)
	recorrer(o, func(x fyne.CanvasObject) {
		if p, ok := x.(*Pulsador); ok {
			p.Tapped(nil)
		}
	})
	if !f.VerRequisitos {
		t.Fatal("pulsar el rótulo no despliega los requisitos")
	}
	o, abierto := f.requisitos(tema.Claro)
	cerca(t, "los requisitos abiertos", abierto, 244)
	if !contiene(textosDe(o), "Real Decreto 1619/2012") {
		t.Error("abiertos, no dicen su fuente")
	}
}

// TestElPanelSeRehaceAlLlegarElPlan: con el panel ABIERTO, lo que llega del servicio se
// ve. `ui.Dialogo` monta su cuerpo una vez, así que el panel lo rehace dentro de un
// contenedor que no cambia (`vivo`).
//
// VISTO EN ROJO (frontal, 2026-09-24): asignando otro `d.Cuerpo`, como se hacía, el plan
// llega a `x.Plan` pero la ventana sigue enseñando el panel vacío.
func TestElPanelSeRehaceAlLlegarElPlan(t *testing.T) {
	p, _ := pantallaConServicio(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/ia/acciones" {
			_, _ = io.WriteString(w, `{"resumen":"Voy a crear la factura de Claude Code.","pasos":[{"ref":"p1","modelo":"factura_venta","data":{"total":18000}}]}`)
			return
		}
		_, _ = io.WriteString(w, `{}`)
	})
	w := test.NewWindow(p.raiz)
	defer w.Close()
	w.Resize(fyne.NewSize(1280, 800))
	p.Ventana = fyne.NewSize(1280, 800)
	p.Atajos(w.Canvas(), nil)
	p.AbrirIA()
	x := p.panelIA
	x.AlPreparar("un gasto de Claude Code")
	esperar(t, func() bool { return x.Plan != nil })
	if !contiene(textosDe(w.Canvas().Overlays().Top()), "Voy a crear la factura de Claude Code.") {
		t.Error("el plan llegó y el panel abierto no lo enseña")
	}
}

// esperar da vueltas a la cola de `fyne.Do` —el driver de pruebas la corre en la rutina
// del test al sincronizar— hasta que se cumpla `listo`.
func esperar(t *testing.T, listo func() bool) {
	t.Helper()
	for range 400 {
		fyne.DoAndWait(func() {})
		if listo() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("no llegó a tiempo")
}

// TestElInformeSeVeLlegarConSusRuedas: el informe se enseña en cuanto llega, con una
// rueda en cada parte mientras se piden sus datos, como la web (`setInforme` y después
// `setDatos`); «Crear tablero» sigue cargando y las acciones, apagadas. Medido en el
// arnés con los datos retenidos (2026-09-24): cada parte 80, la sección 228 a 1280 y
// 354 a 360, y «Crear tablero» con su rueda, 139,984375.
//
// VISTO EN ROJO (frontal, 2026-09-24): pidiendo informe y datos seguidos y pintando al
// final, como antes, el informe no aparece hasta que llegan los datos.
func TestElInformeSeVeLlegarConSusRuedas(t *testing.T) {
	suelta := make(chan struct{})
	inf, datos := informeArnes()
	p, _ := pantallaConServicio(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/ia/informe":
			_ = json.NewEncoder(w).Encode(map[string]any{"informe": inf})
		case "/api/v1/informes/datos":
			<-suelta
			_ = json.NewEncoder(w).Encode(map[string]any{"datos": datos})
		default:
			_, _ = io.WriteString(w, `{}`)
		}
	})
	p.fondo = func(f func()) { go f() }
	// Se suelta al acabar aunque el test falle antes: el servidor espera a sus peticiones
	// al cerrarse (esta limpieza va ANTES que la suya, que se apuntó primero).
	var soltar sync.Once
	suelto := func() { soltar.Do(func() { close(suelta) }) }
	t.Cleanup(suelto)
	w := test.NewWindow(p.raiz)
	defer w.Close()
	w.Resize(fyne.NewSize(1280, 800))
	p.Ventana = fyne.NewSize(1280, 800)
	p.Atajos(w.Canvas(), nil)
	p.AbrirIA()
	x := p.panelIA
	x.Elegir(PestanaInforme)
	x.AlProponer("ventas", api.ERPParaTablero)
	esperar(t, func() bool { return x.Informe != nil })
	if x.Informe.Datos != nil || x.Ocupado != OcupadoProponer {
		t.Fatalf("con el informe recién llegado: datos %v, ocupado %q", x.Informe.Datos, x.Ocupado)
	}
	capa := w.Canvas().Overlays().Top()
	if !contiene(textosDe(capa), "Ventas de 2026") || !contieneTipo[*ui.Rueda](capa) || contiene(textosDe(capa), "4406,96") {
		t.Error("el panel no enseña el informe con sus ruedas mientras llegan los datos")
	}
	cerca(t, "una parte cargando", func() float32 { _, h := x.Informe.tarjetaParte(tema.Claro, inf.Partes[0], 0, 331); return h }(), 80)
	_, seccion := x.informe(tema.Claro, 676)
	cerca(t, "el informe cargando a 1280", seccion, 228)
	x.Ventana = 360
	_, seccion = x.informe(tema.Claro, 278)
	cerca(t, "el informe cargando a 360", seccion, 354)
	x.Ventana = 1280
	tablero := ui.NuevoBoton(p.t("ai.dashboard"), nil).Con(ui.Primario, ui.TallaMD)
	tablero.Cargando = true
	cerca(t, "«Crear tablero» cargando", tablero.MinSize().Width, 139.984375)

	suelto()
	esperar(t, func() bool { return x.Ocupado == "" })
	if x.Informe.Datos == nil || !contiene(textosDe(w.Canvas().Overlays().Top()), "4406,96") {
		t.Error("llegados los datos, el panel no los enseña")
	}
}

// paginaFalsa es un desplazador de página que cuenta lo que le llega.
type paginaFalsa struct{ rueda int }

func (f *paginaFalsa) Scrolled(*fyne.ScrollEvent) { f.rueda++ }

// TestUnaTablaAnchaSoloSeDesplazaDeLado: la tabla de un informe que no cabe (el móvil)
// se desplaza de lado dentro de su caja, y lo VERTICAL —la rueda, el trackpad y el dedo,
// que el driver del móvil entrega como desplazamiento— sigue siendo de la página, que es
// lo que hace `overflow-x-auto` en la web.
//
// VISTO EN ROJO (frontal, 2026-09-24): sin pasarle lo vertical a la página, como hacía
// `container.NewHScroll`, la rueda encima de la tabla la movía de lado.
func TestUnaTablaAnchaSoloSeDesplazaDeLado(t *testing.T) {
	x := informeDePrueba(t)
	pagina := &paginaFalsa{}
	x.Afuera = func() Desplazador { return pagina }
	o, _ := x.tabla(tema.Claro, x.Datos[2], 286)
	var d *desplazaLado
	recorrer(o, func(c fyne.CanvasObject) {
		if l, ok := c.(*desplazaLado); ok {
			d = l
		}
	})
	if d == nil {
		t.Fatal("una tabla que no cabe no se desplaza de lado")
	}
	d.Resize(fyne.NewSize(284, 100))
	d.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.NewDelta(0, -30)})
	d.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.NewDelta(3, -20)})
	if pagina.rueda != 2 || d.X() != 0 {
		t.Errorf("lo vertical no llegó a la página (%d) o movió la tabla (%.2f)", pagina.rueda, d.X())
	}
	d.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.NewDelta(-50, 4)})
	if d.X() != 50 {
		t.Errorf("lo lateral mueve la tabla a %.2f y no a 50", d.X())
	}
	d.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.NewDelta(-1000, 0)})
	if tope := d.Ancho - 284; d.X() != tope {
		t.Errorf("la tabla se sale: %.2f, el tope es %.2f", d.X(), tope)
	}
	d.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.NewDelta(2000, 0)})
	if d.X() != 0 {
		t.Errorf("la tabla se sale por la izquierda: %.2f", d.X())
	}
	if o, _ := x.tabla(tema.Claro, x.Datos[2], 934); contieneTipo[*desplazaLado](o) {
		t.Error("una tabla que cabe no lleva desplazador")
	}
}

func contieneTipo[T any](o fyne.CanvasObject) bool {
	hay := false
	recorrer(o, func(c fyne.CanvasObject) {
		if _, ok := c.(T); ok {
			hay = true
		}
	})
	return hay
}

// .erp-workspace table activa tnum también en las fechas:80.234375px para
// «15 oct2026». Pintarlo proporcionalmente reduce el ancho a74.296875px.
func TestElInformePintaLasCifrasTabularesDelWorkspace(t *testing.T) {
	x := informeDePrueba(t)
	o, h := x.tabla(tema.Claro, x.Datos[2], 932)
	o.Resize(fyne.NewSize(932, h))
	quiero := []float32{8.734375, 68.234375, 80.234375, 8.734375, 50.78125, 71.515625}
	var anchos []float32
	recorrer(o, func(obj fyne.CanvasObject) {
		if c, ok := obj.(*fyne.Container); ok {
			if _, tabular := c.Layout.(*ui.TextoTabular); tabular {
				anchos = append(anchos, c.Size().Width)
			}
		}
	})
	if len(anchos) != len(quiero) {
		t.Fatalf("el informe tiene%d grupos tabulares; Chromium tiene6 celdas con cifras", len(anchos))
	}
	for i, w := range anchos {
		cerca(t, "cifra/fecha del informe", w, quiero[i])
	}
}
