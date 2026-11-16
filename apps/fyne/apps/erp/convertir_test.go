// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"io"
	"net/http"
	"slices"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// # «Crear pedido» y «Crear factura»
//
// La web convierte el presupuesto aceptado en pedido y el pedido confirmado o entregado
// en factura (`convertir` y su botón en la cabecera de la ficha, ErpApp.tsx). El ERP
// nativo no tenía ni el botón ni la llamada: quien vendía desde el escritorio tenía que
// volver a teclear el pedido entero.

// catalogoVentas es el trozo de la venta del catálogo: presupuestos y pedidos en
// «ventas», facturas en «facturacion».
func catalogoVentas() Catalogo {
	estados := func(vs ...string) []api.ERPOpcion {
		out := make([]api.ERPOpcion, len(vs))
		for i, v := range vs {
			out[i] = api.ERPOpcion{Valor: v, Etiqueta: v}
		}
		return out
	}
	campos := []Campo{{Clave: "cliente", Tipo: api.ERPRelacion, Etiqueta: "Cliente", Modelo: "contacto"}}
	return Catalogo{
		Activos: []string{"ventas", "facturacion"},
		Modulos: []Modulo{
			{Clave: "ventas", Etiqueta: "Ventas", Modelos: []Modelo{
				{
					Clave: ModeloPresupuesto, Modulo: "ventas", Etiqueta: "Presupuestos", Numerado: true,
					Lista: []string{"ref", "cliente", "estado"}, Campos: campos,
					Estados: estados("borrador", "enviado", "aceptado", "rechazado"),
				},
				{
					Clave: ModeloPedidoVenta, Modulo: "ventas", Etiqueta: "Pedidos de venta", Numerado: true,
					Lista: []string{"ref", "cliente", "estado"}, Campos: campos,
					Estados: estados("borrador", "confirmado", "entregado", "facturado"),
				},
			}},
			{Clave: "facturacion", Etiqueta: "Facturación", Modelos: []Modelo{{
				Clave: api.ERPFacturaVenta, Modulo: "facturacion", Etiqueta: "Facturas de cliente", Numerado: true,
				Lista: []string{"ref", "cliente", "estado"}, Campos: campos,
				Estados: estados("borrador", "emitida", "pagado"),
			}}},
		},
	}
}

// permisosDeVenta concede escribir y crear en la venta y en la facturación.
func permisosDeVenta() api.ERPPermisos {
	return api.ERPPermisos{
		Modulos: map[string]string{"ventas": api.ERPUsuario, "facturacion": api.ERPUsuario},
		Verbos:  map[string][]string{"ventas": {"crear", "editar"}, "facturacion": {"crear", "editar"}},
	}
}

// botonConvertir es el botón de la cabecera cuyo rótulo es `texto`, o nil.
func botonConvertir(f *Ficha, texto string) *ui.Boton {
	for _, o := range f.botonesCabecera() {
		if b, ok := o.(*ui.Boton); ok && b.Texto == texto {
			return b
		}
	}
	return nil
}

func modeloVentas(t *testing.T, clave string) Modelo {
	t.Helper()
	for _, m := range catalogoVentas().Modulos {
		for _, x := range m.Modelos {
			if x.Clave == clave {
				return x
			}
		}
	}
	t.Fatalf("sin modelo %q", clave)
	return Modelo{}
}

// TestElBotonDeConvertirSaleDondeEnLaWeb: el botón sale exactamente en los casos de la
// web —registro guardado, con escritura, fuera de la papelera, con «crear» en el destino
// y en el estado que se convierte— y dice a qué se convierte.
//
// VISTO EN ROJO: sin el botón, ningún caso lo pinta.
func TestElBotonDeConvertirSaleDondeEnLaWeb(t *testing.T) {
	tr := traductor(t)
	pedido, factura := tr.T("sales.createOrder", nil), tr.T("sales.createInvoice", nil)
	casos := []struct {
		nombre, modelo, estado string
		nueva, soloLectura     bool
		papelera, sinCrear     bool
		quiere                 string
	}{
		{nombre: "presupuesto aceptado", modelo: ModeloPresupuesto, estado: "aceptado", quiere: pedido},
		{nombre: "presupuesto enviado", modelo: ModeloPresupuesto, estado: "enviado"},
		{nombre: "pedido confirmado", modelo: ModeloPedidoVenta, estado: "confirmado", quiere: factura},
		{nombre: "pedido entregado", modelo: ModeloPedidoVenta, estado: "entregado", quiere: factura},
		{nombre: "pedido facturado", modelo: ModeloPedidoVenta, estado: "facturado"},
		{nombre: "pedido borrador", modelo: ModeloPedidoVenta, estado: "borrador"},
		{nombre: "factura emitida", modelo: api.ERPFacturaVenta, estado: "emitida"},
		{nombre: "presupuesto nuevo", modelo: ModeloPresupuesto, estado: "aceptado", nueva: true},
		{nombre: "solo lectura", modelo: ModeloPresupuesto, estado: "aceptado", soloLectura: true},
		{nombre: "en la papelera", modelo: ModeloPedidoVenta, estado: "confirmado", papelera: true},
		{nombre: "sin crear en el destino", modelo: ModeloPresupuesto, estado: "aceptado", sinCrear: true},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			r := &Registro{ID: "x1", Modelo: c.modelo, Estado: c.estado, Data: map[string]any{}}
			if c.nueva {
				r = nil
			}
			f := NuevaFicha(tr, modeloVentas(t, c.modelo), r, nil, nil)
			f.EstadoActual = c.estado
			f.SoloLectura, f.EnPapelera = c.soloLectura, c.papelera
			pedidos := []string{}
			f.PuedeCrear = func(modelo string) bool {
				pedidos = append(pedidos, modelo)
				return !c.sinCrear
			}
			f.AlConvertir = func() {}
			var hay []string
			for _, texto := range []string{pedido, factura} {
				if botonConvertir(f, texto) != nil {
					hay = append(hay, texto)
				}
			}
			switch {
			case c.quiere == "" && len(hay) > 0:
				t.Fatalf("no debía ofrecer convertir y sale %q", hay)
			case c.quiere != "" && !slices.Equal(hay, []string{c.quiere}):
				t.Fatalf("botones = %q; quiere solo %q", hay, c.quiere)
			}
			// Lo que se pregunta es si se puede crear en el DESTINO, no en el origen.
			if c.sinCrear && !slices.Equal(pedidos[:1], []string{ModeloPedidoVenta}) {
				t.Errorf("PuedeCrear preguntado por %q; quiere %q", pedidos, ModeloPedidoVenta)
			}
		})
	}
}

// TestElBotonDeConvertirSigueLaEtapaYEsperaAlGuardar: la condición mira el estado de
// la barra de etapas, como `estadoActual` en la web, y mientras se guarda el botón no
// responde.
func TestElBotonDeConvertirSigueLaEtapaYEsperaAlGuardar(t *testing.T) {
	tr := traductor(t)
	r := &Registro{ID: "p1", Modelo: ModeloPresupuesto, Estado: "enviado", Data: map[string]any{}}
	f := NuevaFicha(tr, modeloVentas(t, ModeloPresupuesto), r, nil, nil)
	f.PuedeCrear = func(string) bool { return true }
	f.AlConvertir = func() {}
	if botonConvertir(f, tr.T("sales.createOrder", nil)) != nil {
		t.Fatal("un presupuesto enviado no se convierte")
	}
	f.EstadoActual = "aceptado"
	b := botonConvertir(f, tr.T("sales.createOrder", nil))
	if b == nil || b.Inerte {
		t.Fatalf("aceptado en la barra: el botón debe salir activo (%v)", b)
	}
	f.Guardando = true
	if b := botonConvertir(f, tr.T("sales.createOrder", nil)); b == nil || !b.Inerte {
		t.Fatal("mientras se guarda, el botón no responde")
	}
}

// TestElPermisoDeCrearEsElDeLaWeb: `puedeCrear` pide el módulo activo y el verbo
// «crear» concedido; sin saber los permisos, no se ofrece.
func TestElPermisoDeCrearEsElDeLaWeb(t *testing.T) {
	p := pantallaDePrueba(t)
	p.Catalogo = catalogoVentas()
	if p.PuedeCrear(api.ERPFacturaVenta) {
		t.Fatal("sin saber los permisos no se ofrece crear en otro modelo")
	}
	p.Permisos, p.SabePermisos = permisosDeVenta(), true
	if !p.PuedeCrear(api.ERPFacturaVenta) || !p.PuedeCrear(ModeloPedidoVenta) {
		t.Fatal("con «crear» concedido, se ofrece")
	}
	p.Permisos.Verbos["facturacion"] = []string{"editar"}
	if p.PuedeCrear(api.ERPFacturaVenta) {
		t.Fatal("sin el verbo «crear» en facturación no se ofrece la factura")
	}
	p.Catalogo.Activos = []string{"facturacion"}
	if p.PuedeCrear(ModeloPedidoVenta) {
		t.Fatal("con «ventas» apagado no se ofrece el pedido")
	}
	if p.PuedeCrear("no_existe") {
		t.Fatal("un modelo que no está en el catálogo no se crea")
	}
}

// pantallaDeVentas monta la pantalla con la venta y un presupuesto aceptado abierto.
func pantallaDeVentas(t *testing.T, responder func(w http.ResponseWriter, r *http.Request)) (*Pantalla, *servicioFalso) {
	t.Helper()
	p, sf := pantallaConServicio(t, responder)
	p.Catalogo = catalogoVentas()
	p.paleta.Modelos = p.Modelos()
	p.Permisos, p.SabePermisos = permisosDeVenta(), true
	var abierto bool
	fyne.DoAndWait(func() {
		p.Abrir(modeloVentas(t, ModeloPresupuesto))
		p.AbrirFicha(Registro{ID: "p1", Modelo: ModeloPresupuesto, Ref: "PR-2026-00003", Estado: "aceptado", Data: map[string]any{}})
		abierto = p.ficha != nil
	})
	if !abierto {
		t.Fatal("la ficha del presupuesto no se abrió")
	}
	return p, sf
}

// TestCrearPedidoAbreElDocumentoNuevo: pulsar «Crear pedido de venta» llama a
// `convertir` y abre el PEDIDO —su lista y su ficha—, como `onConvertido`.
//
// VISTO EN ROJO: sin AlConvertir conectado, el botón no sale y no se pide nada.
func TestCrearPedidoAbreElDocumentoNuevo(t *testing.T) {
	p, sf := pantallaDeVentas(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "POST /api/v1/presupuesto/p1/convertir":
			_, _ = io.ReadAll(r.Body)
			_, _ = io.WriteString(w, `{"id":"pv7","modelo":"pedido_venta","ref":"PV-2026-00007","estado":"borrador","data":{}}`)
		case "GET /api/v1/pedido_venta/pv7":
			_, _ = io.WriteString(w, `{"id":"pv7","modelo":"pedido_venta","ref":"PV-2026-00007","estado":"borrador","data":{}}`)
		default:
			_, _ = io.WriteString(w, `{"registros":[]}`)
		}
	})
	fyne.DoAndWait(func() {
		b := botonConvertir(p.ficha, p.t("sales.createOrder"))
		if b == nil {
			t.Fatal("un presupuesto aceptado ofrece «Crear pedido de venta»")
		}
		b.Tapped(nil)
	})
	if !slices.Contains(sf.vistas(), "POST /api/v1/presupuesto/p1/convertir?") {
		t.Fatalf("no se pidió convertir: %q", sf.vistas())
	}
	var modelo, id string
	fyne.DoAndWait(func() {
		modelo = p.lista.Modelo.Clave
		if p.ficha != nil && p.ficha.Registro != nil {
			id = p.ficha.Registro.ID
		}
	})
	if modelo != ModeloPedidoVenta || id != "pv7" {
		t.Fatalf("abierto %q/%q; quiere el pedido nuevo pedido_venta/pv7", modelo, id)
	}
}

// TestUnFalloAlConvertirSeQuedaEnLaFicha: si el servicio lo rechaza, la ficha sigue
// abierta con el motivo de `MensajeDeError` en su banda y el botón vuelve a responder.
func TestUnFalloAlConvertirSeQuedaEnLaFicha(t *testing.T) {
	const cuerpo = `{"error":{"code":"sin_permiso","message":"no"}}`
	p, _ := pantallaDeVentas(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, cuerpo)
			return
		}
		_, _ = io.WriteString(w, `{"registros":[]}`)
	})
	f := p.ficha
	fyne.DoAndWait(func() { botonConvertir(f, p.t("sales.createOrder")).Tapped(nil) })
	quiere := MensajeDeError(errorDelServicio(http.StatusForbidden, cuerpo), p.tr)
	fyne.DoAndWait(func() {
		if p.ficha != f || p.lista.Modelo.Clave != ModeloPresupuesto {
			t.Fatal("un fallo no debe cerrar la ficha ni cambiar de lista")
		}
		if f.Fallo == "" || f.Fallo != quiere {
			t.Fatalf("fallo = %q; quiere %q", f.Fallo, quiere)
		}
		if f.Guardando || botonConvertir(f, p.t("sales.createOrder")).Inerte {
			t.Fatal("tras el fallo, el botón vuelve a responder")
		}
	})
}
