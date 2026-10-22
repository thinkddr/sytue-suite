// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// # La lista con filas y la ficha, contra el arnés del frontal
//
// El barrido de producción no puede medir filas ni fichas —la cuenta de auditoría no
// tiene permisos de módulo y todo le da 403—, así que estas cifras salen del ARNÉS: la
// web de verdad (ErpApp.tsx, compilada del main de hoy) con un servicio de mentira que
// contesta los mismos cuatro registros que `facturasArnes`. Chromium con Figtree,
// raíz 16 px y fuente CSS real, 2026-10-04; viewport 1280×710 y 390×710.

// facturaArnes es `factura_venta` tal y como la publica el catálogo en español.
func facturaArnes() Modelo {
	return Modelo{
		Clave: "factura_venta", Modulo: "facturacion", Etiqueta: "Facturas de cliente",
		Numerado: true, Contable: true, Cobro: true,
		Lista: []string{"ref", "cliente", "fecha", "vencimiento", "total", "estado"},
		Campos: []Campo{
			{Clave: "cliente", Tipo: api.ERPRelacion, Etiqueta: "Cliente", Requerido: true, Modelo: "contacto"},
			{Clave: "pedido", Tipo: api.ERPRelacion, Etiqueta: "Pedido", Modelo: "pedido_venta"},
			{Clave: "fecha", Tipo: api.ERPFecha, Etiqueta: "Fecha", Requerido: true},
			{Clave: "vencimiento", Tipo: api.ERPFecha, Etiqueta: "Vencimiento"},
			{Clave: "base", Tipo: api.ERPDinero, Etiqueta: "Base imponible", Requerido: true},
			{Clave: "iva", Tipo: api.ERPDinero, Etiqueta: "IVA"},
			{Clave: "total", Tipo: api.ERPDinero, Etiqueta: "Total"},
			{Clave: "regimen_iva", Tipo: api.ERPSeleccion, Etiqueta: "Régimen de IVA", Opciones: []api.ERPOpcion{
				{Valor: "iva_general", Etiqueta: "General (con IVA)"}, {Valor: "iva_exenta", Etiqueta: "Exenta"},
			}},
			{Clave: "rectifica", Tipo: api.ERPRelacion, Etiqueta: "Rectifica a", Modelo: "factura_venta"},
			{Clave: "notas", Tipo: api.ERPTextoLargo, Etiqueta: "Notas"},
			{Clave: "externo_id", Tipo: "texto", Etiqueta: "ID externo"},
		},
		Estados: []api.ERPOpcion{
			{Valor: "borrador", Etiqueta: "Borrador"},
			{Valor: "confirmado", Etiqueta: "Confirmado"},
			{Valor: "pagado", Etiqueta: "Pagado"},
			{Valor: "anulado", Etiqueta: "Anulado"},
		},
	}
}

func facturasArnes() Pagina {
	reg := func(id, ref, et, est string, data map[string]any) Registro {
		creado, _ := time.Parse(time.RFC3339, "2026-09-01T09:12:00Z")
		editado, _ := time.Parse(time.RFC3339, "2026-09-18T16:40:00Z")
		return Registro{
			ID: id, Modelo: "factura_venta", Ref: ref, Etiqueta: et, Estado: est, Data: data,
			CreadoEn: creado, EditadoEn: editado,
		}
	}
	return Pagina{
		Registros: []Registro{
			reg("f1", "FV-2026-00041", "Ferretería Lobo", "pagado", map[string]any{
				"cliente": "c1", "fecha": "2026-09-01",
				"vencimiento": "2026-10-01", "base": float64(102030), "iva": float64(21426), "total": float64(123456),
				"regimen_iva": "iva_general", "notas": "Pedido de septiembre",
			}),
			reg("f2", "FV-2026-00042", "Talleres Sur", "borrador", map[string]any{
				"cliente": "c2", "fecha": "2026-09-10",
				"base": float64(8182), "iva": float64(1718), "total": float64(9900),
			}),
			reg("f3", "FV-2026-00043", "Construcciones Almagro S.L.", "confirmado", map[string]any{
				"cliente": "c3",
				"fecha":   "2026-09-15", "vencimiento": "2026-10-15", "base": float64(250000), "iva": float64(52500),
				"total": float64(302500),
			}),
			reg("f4", "FV-2026-00044", "Panadería La Espiga", "anulado", map[string]any{
				"cliente": "c4",
				"fecha":   "2026-09-18", "base": float64(4000), "iva": float64(840), "total": float64(4840),
			}),
		},
		Etiquetas: map[string]string{
			"c1": "Ferretería Lobo", "c2": "Talleres Sur", "c3": "Construcciones Almagro S.L.",
			"c4": "Panadería La Espiga",
		},
	}
}

// pantallaArnes monta la pantalla con la lista de facturas del arnés.
func pantallaArnes(t *testing.T, ventana fyne.Size) (*Pantalla, fyne.Window) {
	t.Helper()
	p := pantallaDePrueba(t)
	p.Fase = Exito
	p.Catalogo.Modulos[0].Modelos[0] = facturaArnes()
	p.lista.Modelo = facturaArnes()
	p.lista.Pagina = facturasArnes()
	p.lista.Fase = Exito
	p.lista.Total, p.lista.SumaDinero = 4, 440696
	p.lista.PorEstado = map[string]int{"borrador": 1, "confirmado": 1, "pagado": 1, "anulado": 1}
	p.Organizacion, p.SabeOrg = &api.ERPOrganizacion{Pais: "ES"}, true
	w := test.NewWindow(p.Objeto())
	w.SetPadded(false)
	w.Resize(ventana)
	p.Ventana = ventana
	p.Repintar()
	return p, w
}

// TestLasColumnasSeRepartenComoElNavegador: `table-layout: auto` con las cuatro facturas
// del arnés. Tabla desktop: 970 px dentro del filete, en un main con carril 260.
//
// VISTO EN ROJO (frontal, 2026-09-23): repartiendo lo que sobra a partes iguales, que es
// lo que hace `ui.Tabla`, las columnas se salen del ±2.
func TestLasColumnasSeRepartenComoElNavegador(t *testing.T) {
	l := NuevaLista(traductor(t))
	l.Modelo, l.Pagina, l.Fase = facturaArnes(), facturasArnes(), Exito
	l.AlDuplicar = func(Registro) {}
	res := l.resolver(970)
	medido := []float32{40, 138.1875, 224.78125, 123.109375, 132.4375, 110.671875, 123.53125, 77.28125}
	if len(res.anchos) != len(medido) {
		t.Fatalf("%d columnas; la web pinta %d", len(res.anchos), len(medido))
	}
	for i, m := range medido {
		// El texto sale a ≤0,17 del navegador; la mono es otra letra (DejaVu en los
		// dos, pero el navegador la redondea distinto): ±2.
		if d := res.anchos[i] - m; d < -2 || d > 2 {
			t.Errorf("columna %d mide %.2f; la web %.2f", i, res.anchos[i], m)
		}
	}
	for _, a := range res.altos {
		if a != AltoFilaERP {
			t.Errorf("una fila mide %v; en la web, con todo en una línea, %v", a, AltoFilaERP)
		}
	}
}

// TestLaFichaColocaDondeLaWeb abre FV-2026-00041 con todo lo que cuelga de ella y ata
// las alturas de los bloques a las del arnés actual. La cabecera incluye el aviso
// de Facturae, además del título, referencia y acciones. Guarda los PNG para mirarlos.
//
// VISTO EN ROJO (frontal, 2026-09-23): sin el hueco que deja el `<textarea>` en línea,
// las notas miden 80,75.
func TestLaFichaColocaDondeLaWeb(t *testing.T) {
	p, w := pantallaArnes(t, fyne.NewSize(1280, 710))
	defer w.Close()
	r := facturasArnes().Registros[0]
	p.AbrirFicha(r)
	f := p.ficha
	f.Asientos = []api.ERPAsiento{{
		ID: "as1", Ref: "AS-2026-00017", Fecha: "2026-09-01", Concepto: "Factura FV-2026-00041",
		Estado: "asentado", Apuntes: []api.ERPApunte{
			{Cuenta: "430000", Concepto: "Ferretería Lobo", Debe: 123456},
			{Cuenta: "700000", Concepto: "Ventas", Haber: 102030},
			{Cuenta: "477000", Concepto: "IVA repercutido", Haber: 21426},
		},
	}}
	hoy := time.Now()
	f.Tareas = []api.ERPActividad{
		{ID: "a1", Tipo: "subir_documento", Resumen: "Sube la factura firmada en PDF", Vence: hoy.AddDate(0, 0, -2).Format(time.DateOnly), ResponsableNombre: "Ada Lovelace"},
		{ID: "a2", Tipo: "llamada", Resumen: "Llamar para confirmar el cobro", Vence: hoy.Format(time.DateOnly), ResponsableNombre: "Ada Lovelace"},
	}
	creado, _ := time.Parse(time.RFC3339, "2026-09-02T10:00:00Z")
	f.Adjuntos = []api.ERPAdjunto{{ID: "d1", Nombre: "factura-FV-2026-00041.pdf", Tamano: 184320, CreadoEn: creado}}
	cod := 1110
	f.Verifactu = &api.ERPFacturaVerifactu{QR: "https://prewww2.aeat.es/", Registros: []api.ERPRegistroVerifactu{
		{Tipo: "alta", Estado: api.ERPVFConErrores, CSV: "A-7KQ2P9XW", Codigo: &cod, Descripcion: "El NIF del destinatario no está identificado."},
	}}
	f.Historial = []api.ERPEntradaHistorial{
		{ID: "h3", Tipo: "nota", Nombre: "Ada Lovelace", Texto: "El cliente pide el duplicado por correo.", CreadoEn: editadoArnes()},
		{ID: "h2", Tipo: "cambio", Nombre: "Ada Lovelace", Cambios: map[string]api.ERPCambio{"estado": {Antes: "confirmado", Despues: "pagado"}}, CreadoEn: editadoArnes()},
		{ID: "h1", Tipo: "creado", Nombre: "Grace Hopper", CreadoEn: editadoArnes()},
	}
	p.Repintar()
	guardarPNG(t, w.Canvas().Capture(), "erp-ficha.png")

	f.ancho = AnchoFicha
	_, altoCab := f.cabecera(tema.Claro)
	_, altoBarra := f.barraEstado(tema.Claro)
	_, altoAsientos := f.seccionAsientos(tema.Claro)
	_, altoTareas := f.tareas(tema.Claro)
	_, altoDocs := f.documentos(tema.Claro)
	_, altoVF := f.verifactu(tema.Claro)
	_, altoHist := f.historial(tema.Claro)
	_, altoTraza := f.trazabilidad(tema.Claro)
	var altoCliente, altoNotas float32
	for _, k := range f.controles {
		_, a := f.bloqueCampo(tema.Claro, k)
		switch k.Campo.Clave {
		case "cliente":
			altoCliente = a
		case "notas":
			altoNotas = a
		}
	}
	for _, c := range []struct {
		que         string
		got, quiero float32
	}{
		{"cabecera", altoCab, 122},
		{"barra de estado", altoBarra, 84},
		{"campo", altoCliente, 56},
		{"notas", altoNotas, 98},
		{"asientos", altoAsientos, 239},
		{"tareas", altoTareas, 108},
		{"documentos", altoDocs, 74},
		{"VERI*FACTU", altoVF, 132},
		{"historial", altoHist, 196},
		{"trazabilidad", altoTraza, 73},
	} {
		if d := c.got - c.quiero; d < -0.3 || d > 0.3 {
			t.Errorf("%s mide %.2f; la web %.2f", c.que, c.got, c.quiero)
		}
	}

	// Y en el móvil, a pantalla completa.
	w.Resize(fyne.NewSize(390, 710))
	p.Ventana = fyne.NewSize(390, 710)
	p.Repintar()
	guardarPNG(t, w.Canvas().Capture(), "erp-ficha-movil.png")
}

// El JSX real de2026-10-04 reserva una fila completa al título y otra a la ayuda.
// Medidas DOM a1280/panel448 y móvil390:399/341×122, título42 y ayuda32.
func TestLaCabeceraFacturaeMantieneTituloAccionesYAyuda(t *testing.T) {
	r := facturasArnes().Registros[0]
	for _, caso := range []struct{ panel, util float32 }{{448, 399}, {390, 341}} {
		f := NuevaFicha(traductor(t), facturaArnes(), &r, nil, nil)
		f.Facturae, f.ancho = true, caso.panel
		obj, alto := f.cabecera(tema.Claro)
		c := obj.(*fyne.Container).Layout.(*cabeceraFicha)
		obj.Resize(fyne.NewSize(caso.util, alto))
		cerca(t, "cabecera legible", alto, 122)
		cerca(t, "ancho reservado al título", c.anchoTitulo, caso.util)
		cerca(t, "título y referencia", c.altoTitulo, 42)
		cerca(t, "ayuda completa", c.altoAviso, 32)
		if c.titulo.Text != "Ferretería Lobo" || len(c.botones) != 5 {
			t.Fatalf("título o acciones perdidos: %q, %d", c.titulo.Text, len(c.botones))
		}
		for i, y := range []float32{50, 53, 50, 50, 50} {
			cerca(t, "fila de acciones", c.botones[i].Position().Y, y)
			if c.botones[i].Position().X+c.botones[i].Size().Width > caso.util+0.3 {
				t.Fatal("una acción se sale del panel")
			}
		}
		cerca(t, "ayuda después de las acciones", c.aviso.Position().Y, 90)
		if c.aviso.Size().Width != caso.util {
			t.Fatal("la ayuda debe usar toda su fila")
		}
	}
}

func editadoArnes() time.Time {
	t, _ := time.Parse(time.RFC3339, "2026-09-18T16:40:00Z")
	return t
}

// TestLaListaEnSusTresFormas guarda la tabla, las tarjetas y las filas del móvil.
func TestLaListaEnSusTresFormas(t *testing.T) {
	p, w := pantallaArnes(t, fyne.NewSize(1280, 710))
	defer w.Close()
	guardarPNG(t, w.Canvas().Capture(), "erp-lista-tabla.png")
	p.lista.Tarjetas = true
	p.Repintar()
	guardarPNG(t, w.Canvas().Capture(), "erp-lista-tarjetas.png")
	if got := altoTarjeta(3); got < 196.7 || got > 197.3 {
		t.Errorf("una tarjeta con tres campos mide %.2f; la web 197", got)
	}
	p.lista.Tarjetas = false
	p.lista.Marcadas = map[string]bool{"f1": true, "f3": true}
	p.Repintar()
	guardarPNG(t, w.Canvas().Capture(), "erp-lista-lote.png")

	p.lista.Marcadas = map[string]bool{}
	w.Resize(fyne.NewSize(390, 710))
	p.Ventana = fyne.NewSize(390, 710)
	p.Repintar()
	guardarPNG(t, w.Canvas().Capture(), "erp-lista-movil.png")
}

// TestElDineroViajaEnCentimos: `aCentimos` y `aCuerpo` de la web. Es lo único de la
// ficha que puede equivocarse en silencio y costar dinero.
//
// VISTO EN ROJO (frontal, 2026-09-23): sin el redondeo comercial del tercer decimal,
// «10.005» sale 1000.
func TestElDineroViajaEnCentimos(t *testing.T) {
	for entrada, quiero := range map[string]int64{
		"1234.56": 123456, "1234,56": 123456, "0.1": 10, "-3,5": -350, "10.005": 1001, "1 000,00": 100000,
	} {
		if got, ok := ACentimos(entrada); !ok || got != quiero {
			t.Errorf("ACentimos(%q) = %d, %v; quiero %d", entrada, got, ok, quiero)
		}
	}
	for _, mal := range []string{"", "-", ".", "12a", "1.2.3"} {
		if _, ok := ACentimos(mal); ok {
			t.Errorf("ACentimos(%q) se lee como importe", mal)
		}
	}
	m := facturaArnes()
	v := ValoresIniciales(m, &facturasArnes().Registros[0])
	if v["base"] != "1020.30" || v["fecha"] != "01/09/2026" {
		t.Errorf("valores iniciales: base %q, fecha %q", v["base"], v["fecha"])
	}
	cuerpo := ACuerpo(m, v, time.UTC)
	if cuerpo["base"] != int64(102030) || cuerpo["fecha"] != "2026-09-01" || cuerpo["cliente"] != "c1" {
		t.Errorf("cuerpo: %#v", cuerpo)
	}
	if _, hay := cuerpo["pedido"]; hay {
		t.Error("un campo vacío viaja al servicio")
	}
}

// TestLaRelacionObligatoriaSeCompruebaAntesDeEnviar: la web lo hace a mano porque el
// control es un botón y el formulario no lo valida.
//
// VISTO EN ROJO (frontal, 2026-09-23): saltándose la comprobación, se deja guardar.
func TestLaRelacionObligatoriaSeCompruebaAntesDeEnviar(t *testing.T) {
	f := NuevaFicha(traductor(t), facturaArnes(), nil, nil, nil)
	if f.Comprobar() {
		t.Fatal("sin cliente se deja guardar")
	}
	if f.Errores["cliente"] == "" {
		t.Error("el error no va debajo del cliente")
	}
	f.fijar("cliente", "c1")
	if f.Errores["cliente"] != "" || !f.Comprobar() {
		t.Error("elegido el cliente sigue sin dejar guardar")
	}
}
