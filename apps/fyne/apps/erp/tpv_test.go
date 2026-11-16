// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// productoTPV es un producto del catálogo como lo lista el servicio: el precio con IVA en
// céntimos llega como número JSON (float64) y el tipo, como cadena.
func productoTPV(id, nombre, categoria string, precio float64, tipo string) Registro {
	return Registro{ID: id, Modelo: "producto", Etiqueta: nombre, Data: map[string]any{
		"nombre": nombre, "categoria": categoria, "precio_con_iva": precio, "tipo_iva": tipo,
	}}
}

// productosTPV son los del arnés: dos categorías y uno sin precio, que no se vende.
func productosTPV() []Registro {
	return []Registro{
		productoTPV("11111111-1111-4111-8111-111111111111", "Café con leche", "Bebidas", 605, "21"),
		productoTPV("22222222-2222-4222-8222-222222222222", "Barra de pan", "Panadería", 110, "10"),
		productoTPV("33333333-3333-4333-8333-333333333333", "Tostada con tomate y aceite de oliva virgen extra", "Panadería", 333, "21"),
		{ID: "44444444-4444-4444-8444-444444444444", Modelo: "producto", Etiqueta: "Bandeja sin precio", Data: map[string]any{"categoria": "Bebidas", "tipo_iva": "21"}},
	}
}

const sesionTPVArnes = "55555555-5555-4555-8555-555555555555"

func sesionesTPV() []Registro {
	return []Registro{{ID: sesionTPVArnes, Modelo: api.ERPModeloSesionTPV, Ref: "TPV-2026-00003", Data: map[string]any{"caja": "Mostrador"}}}
}

// TestTPVCuadraLaCestaAlCentimo: se agrupa por tipo ANTES de desglosar, como el servicio,
// y bases y cuotas suman el total sin perder un céntimo.
//
// VISTO EN ROJO: desglosando línea a línea, dos productos de 1,00 € al 21 % dan 34 de
// IVA (17 + 17) y no los 35 del grupo, que es lo que asienta el servicio.
func TestTPVCuadraLaCestaAlCentimo(t *testing.T) {
	tpv := NuevoTPV(traductor(t))
	ps := productosTPV()
	for range 3 {
		tpv.Modificar(ps[0], 1)
	}
	tpv.Modificar(ps[1], 1)
	tpv.Modificar(ps[1], 1)
	tpv.Modificar(ps[2], 1)
	total, cuotas := tpv.Totales()
	if total != 3*605+2*110+333 {
		t.Fatalf("total %d", total)
	}
	quiere := []CuotaTPV{{Tipo: 21, Base: 1775, IVA: 373}, {Tipo: 10, Base: 200, IVA: 20}}
	if !slices.Equal(cuotas, quiere) {
		t.Errorf("cuotas %+v; quiere %+v", cuotas, quiere)
	}
	var suma int64
	for _, c := range cuotas {
		suma += c.Base + c.IVA
	}
	if suma != total {
		t.Errorf("bases y cuotas suman %d; el total es %d", suma, total)
	}
	// Quitar la última unidad quita la línea; el tope por línea es el del servicio.
	tpv.Modificar(ps[2], -1)
	if tpv.Cantidad(ps[2].ID) != 0 || len(tpv.Lineas) != 2 {
		t.Errorf("líneas %+v", tpv.Lineas)
	}
	for range MaxUnidadesTPV + 5 {
		tpv.Modificar(ps[1], 1)
	}
	if tpv.Cantidad(ps[1].ID) != MaxUnidadesTPV {
		t.Errorf("unidades %d; el tope es %d", tpv.Cantidad(ps[1].ID), MaxUnidadesTPV)
	}
	dos := NuevoTPV(traductor(t))
	dos.Modificar(productoTPV("a", "A", "", 100, "21"), 1)
	dos.Modificar(productoTPV("b", "B", "", 100, "21"), 1)
	if _, c := dos.Totales(); !slices.Equal(c, []CuotaTPV{{Tipo: 21, Base: 165, IVA: 35}}) {
		t.Errorf("dos de 1,00 € al 21 %%: %+v; el grupo da 165 + 35", c)
	}
	if ListoTPV(ps[3]) {
		t.Error("un producto sin precio no se vende")
	}
}

// TestTPVPideLoQueFalta: sin caja, con efectivo de menos o con 1.000 € en efectivo no
// se envía nada, y lo dice.
func TestTPVPideLoQueFalta(t *testing.T) {
	tpv := NuevoTPV(traductor(t))
	var enviados int
	tpv.AlCobrar = func(api.ERPCobroTPV) { enviados++ }
	tpv.Modificar(productosTPV()[0], 1)
	tpv.Cobrar()
	if enviados != 0 || tpv.Fallo != tpv.t("pos.review") {
		t.Errorf("sin caja: %d envíos, fallo %q", enviados, tpv.Fallo)
	}
	tpv.Sesion = sesionTPVArnes
	tpv.PonerMedio(api.ERPMedioEfectivo)
	tpv.Recibido = "6"
	tpv.Cobrar()
	if enviados != 0 {
		t.Error("6 € no pagan 6,05 €")
	}
	tpv.Recibido = "6,10"
	tpv.Cobrar()
	c, _ := tpv.PrepararCobro()
	if enviados != 1 || c.Recibido != 610 || c.Medio != api.ERPMedioEfectivo || c.Lineas[0].Precio != 605 {
		t.Errorf("efectivo: %d envíos, cobro %+v", enviados, c)
	}
}

// cajaTPVFalsa es el servicio del TPV: lista productos y cajas, y emite UNA factura por
// operación. Con `cortar`, la primera respuesta del cobro se pierde con la venta ya hecha.
type cajaTPVFalsa struct {
	mu       sync.Mutex
	facturas map[string]string
	cuerpos  []string
	cortar   bool
}

func (c *cajaTPVFalsa) responder(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/v1/producto/":
		datos, _ := json.Marshal(api.ERPPagina{Registros: productosTPV()})
		_, _ = w.Write(datos)
	case "/api/v1/sesion_tpv/":
		datos, _ := json.Marshal(api.ERPPagina{Registros: sesionesTPV()})
		_, _ = w.Write(datos)
	case "/api/v1/tpv/cobrar":
		cuerpo, _ := io.ReadAll(r.Body)
		var cobro api.ERPCobroTPV
		_ = json.Unmarshal(cuerpo, &cobro)
		c.mu.Lock()
		c.cuerpos = append(c.cuerpos, string(cuerpo))
		id, ok := c.facturas[cobro.Operacion]
		if !ok {
			id = "fv-" + strconv.Itoa(len(c.facturas)+1)
			c.facturas[cobro.Operacion] = id
		}
		cortar := c.cortar
		c.cortar = false
		c.mu.Unlock()
		if cortar {
			if conn, _, err := w.(http.Hijacker).Hijack(); err == nil {
				_ = conn.Close()
			}
			return
		}
		_, _ = io.WriteString(w, `{"factura":{"id":"`+id+`","modelo":"factura_venta","ref":"FS-2026-00042","data":{"total":1815}},"cambio":185}`)
	default:
		_, _ = io.WriteString(w, `{"registros":[]}`)
	}
}

// permisosTPV son los de quien cobra: crear en el TPV y crear, asentar y cobrar facturas.
func permisosTPV() api.ERPPermisos {
	return api.ERPPermisos{
		Modulos: map[string]string{"tpv": api.ERPUsuario, "facturacion": api.ERPUsuario, "productos": api.ERPLectura, "contactos": api.ERPLectura},
		Verbos:  map[string][]string{"tpv": {"crear"}, "facturacion": {"crear", "asentar", "pagar"}},
	}
}

// TestTPVReintentarTrasCorteEmiteUnaSolaFactura: se pierde la respuesta del cobro con la
// venta YA registrada. La cesta se congela, el botón pasa a «Reintentar» y el reintento
// manda el mismo cuerpo: la caja acaba con UNA factura y la pantalla enseña la venta.
//
// VISTO EN ROJO (2026-11-12): liberando la cesta ante cualquier fallo, como si un corte
// fuera un 422, el corte deja la cesta editable y el siguiente cobro lleva OTRA
// operación, que el servicio cobraría dos veces.
func TestTPVReintentarTrasCorteEmiteUnaSolaFactura(t *testing.T) {
	caja := &cajaTPVFalsa{facturas: map[string]string{}, cortar: true}
	p, _ := pantallaConServicio(t, caja.responder)
	p.Catalogo.Activos = append(p.Catalogo.Activos, "tpv")
	p.SabePermisos, p.Permisos = true, permisosTPV()
	p.IrA(VistaTPV)
	tpv := p.tpv
	if len(tpv.Productos) != 4 || tpv.Sesion != sesionTPVArnes {
		t.Fatalf("catálogo %d productos, caja %q", len(tpv.Productos), tpv.Sesion)
	}
	cafe := tpv.Productos[0]
	for range 3 {
		tpv.Modificar(cafe, 1)
	}
	tpv.Cobrar()
	if !tpv.Incierto || tpv.Venta != nil || tpv.Fallo == "" || tpv.Ocupado {
		t.Fatalf("tras el corte: incierto %v, venta %v, fallo %q", tpv.Incierto, tpv.Venta, tpv.Fallo)
	}
	// Con el cobro incierto la cesta no se toca: otra cesta sería otra venta.
	tpv.Modificar(cafe, 1)
	tpv.PonerMedio(api.ERPMedioEfectivo)
	if tpv.Cantidad(cafe.ID) != 3 || tpv.Medio != api.ERPMedioTarjeta {
		t.Errorf("la cesta cambió con un cobro incierto: %d unidades, %s", tpv.Cantidad(cafe.ID), tpv.Medio)
	}
	if textos := textosDe(p.vista(tema.Claro)); !contiene(textos, "Reintentar") {
		t.Errorf("el botón debe ofrecer reintentar; se pintó %v", textos)
	}
	tpv.Cobrar()
	if tpv.Venta == nil || tpv.Venta.Factura.ID != "fv-1" || tpv.Incierto {
		t.Fatalf("reintento: venta %+v, incierto %v, fallo %q", tpv.Venta, tpv.Incierto, tpv.Fallo)
	}
	if len(caja.facturas) != 1 || len(caja.cuerpos) != 2 || caja.cuerpos[0] != caja.cuerpos[1] {
		t.Errorf("%d facturas; cuerpos %q", len(caja.facturas), caja.cuerpos)
	}
	textos := textosDe(p.vista(tema.Claro))
	for _, s := range []string{"Venta registrada", "FS-2026-00042", "Nueva venta"} {
		if !contiene(textos, s) {
			t.Errorf("la venta hecha no dice «%s»; se pintó %v", s, textos)
		}
	}
	// La siguiente venta es otra operación.
	tpv.NuevaVenta()
	tpv.Modificar(cafe, 1)
	tpv.Cobrar()
	if len(caja.facturas) != 2 {
		t.Errorf("una venta nueva debe emitir otra factura; hay %d", len(caja.facturas))
	}
}

// TestTPVUnRechazoDelServicioLiberaLaCesta: un 422 al primer envío prueba que no se
// registró nada (el servicio deshizo la transacción): la cesta se puede corregir.
func TestTPVUnRechazoDelServicioLiberaLaCesta(t *testing.T) {
	tpv := NuevoTPV(traductor(t))
	tpv.Sesion = sesionTPVArnes
	var cobros []api.ERPCobroTPV
	tpv.AlCobrar = func(c api.ERPCobroTPV) { cobros = append(cobros, c) }
	cafe := productosTPV()[0]
	tpv.Modificar(cafe, 1)
	tpv.Cobrar()
	tpv.Cobrado(api.ERPVentaTPV{}, &api.Error{Status: http.StatusUnprocessableEntity}, "precio cambiado")
	if tpv.Incierto || tpv.Fallo != "precio cambiado" {
		t.Fatalf("incierto %v, fallo %q", tpv.Incierto, tpv.Fallo)
	}
	tpv.Modificar(cafe, 1)
	if tpv.Cantidad(cafe.ID) != 2 {
		t.Error("tras un rechazo la cesta debe poder corregirse")
	}
	// Un 5xx no prueba nada: se queda incierto con la misma operación.
	tpv.Cobrar()
	tpv.Cobrado(api.ERPVentaTPV{}, &api.Error{Status: http.StatusBadGateway}, "caído")
	tpv.Cobrar()
	if !tpv.Incierto || len(cobros) != 3 || cobros[1].Operacion != cobros[2].Operacion || cobros[0].Operacion == cobros[1].Operacion {
		t.Errorf("operaciones %v", []string{cobros[0].Operacion, cobros[1].Operacion, cobros[len(cobros)-1].Operacion})
	}
}

// TestElCarrilLlevaElTPVEntreTareasYBanco: con el módulo encendido, el punto de venta va
// en el menú y en el carril entre Tareas y Banco, como en la web; sin él, no va.
func TestElCarrilLlevaElTPVEntreTareasYBanco(t *testing.T) {
	p := pantallaDePrueba(t)
	p.Fase = Exito
	titulos := func() []string {
		p.rellenarMenu()
		arriba := p.carril().Arriba
		out := make([]string, 0, len(arriba))
		for _, a := range arriba {
			out = append(out, a.Titulo)
		}
		return out
	}
	if slices.Contains(titulos(), "Punto de venta") {
		t.Error("sin el módulo tpv encendido no hay punto de venta")
	}
	p.Catalogo.Activos = append(p.Catalogo.Activos, "tpv")
	got := titulos()
	i, j, k := slices.Index(got, "Tareas"), slices.Index(got, "Punto de venta"), slices.Index(got, "Banco")
	if i < 0 || j != i+1 || k != j+1 {
		t.Errorf("carril %v: el TPV debe ir entre Tareas y Banco", got)
	}
}

// TestElTPVSeVeComoLaWeb pinta el TPV en ancho y en el teléfono con una venta a medias y
// la venta hecha, y guarda los PNG (SYTUE_PNG) para mirarlos.
func TestElTPVSeVeComoLaWeb(t *testing.T) {
	for _, caso := range []struct {
		nombre string
		tam    fyne.Size
		hecha  bool
	}{
		{"tpv-ancho", fyne.NewSize(1440, 1000), false},
		{"tpv-movil", fyne.NewSize(400, 1600), false},
		{"tpv-hecha", fyne.NewSize(1440, 900), true},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			p, w := pantallaArnes(t, caso.tam)
			t.Cleanup(w.Close)
			p.Catalogo.Activos = append(p.Catalogo.Activos, "tpv")
			p.SabePermisos, p.Permisos = true, permisosTPV()
			p.IrA(VistaTPV)
			tpv := p.tpv
			tpv.Productos, tpv.Sesiones, tpv.Sesion = productosTPV(), sesionesTPV(), sesionTPVArnes
			tpv.Modificar(tpv.Productos[0], 1)
			tpv.Modificar(tpv.Productos[0], 1)
			tpv.Modificar(tpv.Productos[2], 1)
			if caso.hecha {
				tpv.Venta = &api.ERPVentaTPV{Factura: Registro{ID: "fv-1", Ref: "FS-2026-00042", Data: map[string]any{"total": float64(1543)}}, Cambio: 57}
			}
			p.Repintar()
			textos := textosDe(w.Canvas().Content())
			quiere := []string{"Punto de venta", "Venta actual", "Café con leche", "Registrar cobro", "Total", "Configura", "Todos", "Panadería"}
			if caso.hecha {
				quiere = []string{"Venta registrada", "FS-2026-00042", "Imprimir factura", "Abrir", "Nueva venta"}
			}
			for _, s := range quiere {
				if !contiene(textos, s) {
					t.Errorf("no se pinta «%s»; se pintó %v", s, textos)
				}
			}
			if !caso.hecha && !contiene(textos, "15,43") {
				t.Errorf("el total de 2 × 6,05 + 3,33 es 15,43 €; se pintó %v", textos)
			}
			guardarPNG(t, w.Canvas().Capture(), caso.nombre+".png")
		})
	}
}

// TestLasColumnasDelTPVSonLasDeLaRejilla: `auto-fill, minmax(10.5rem, 1fr)` con `gap:
// 0.75rem` (168 y 12), y dos fijas en el teléfono.
func TestLasColumnasDelTPVSonLasDeLaRejilla(t *testing.T) {
	for _, c := range []struct {
		ancho float32
		movil bool
		n     int
	}{{800, false, 4}, {168, false, 1}, {347, false, 1}, {348, false, 2}, {320, true, 2}} {
		n, w := ColumnasTPV(c.ancho, c.movil)
		if n != c.n {
			t.Errorf("%v px: %d columnas; la web %d", c.ancho, n, c.n)
		}
		gap := TeselaTPVGap
		if c.movil {
			gap = 0.625 * tema.Rem
		}
		if total := float32(n)*w + float32(n-1)*gap; total < c.ancho-0.01 || total > c.ancho+0.01 {
			t.Errorf("%v px: las columnas ocupan %v", c.ancho, total)
		}
	}
}

// TestElTPVSeRecuperaDeUnFalloDeCarga: si el catálogo no llega, sale el aviso; al volver
// a pedirlo con la red de vuelta, el aviso se va, como en la web, que limpia el error al pedir.
//
// VISTO EN ROJO: sin limpiar el fallo al recargar, el catálogo cargado seguía con el
// aviso del error encima.
func TestElTPVSeRecuperaDeUnFalloDeCarga(t *testing.T) {
	caja := &cajaTPVFalsa{facturas: map[string]string{}}
	caida := true
	p, _ := pantallaConServicio(t, func(w http.ResponseWriter, r *http.Request) {
		if caida {
			http.Error(w, `{"code":"unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		caja.responder(w, r)
	})
	p.Catalogo.Activos = append(p.Catalogo.Activos, "tpv")
	p.SabePermisos, p.Permisos = true, permisosTPV()
	p.IrA(VistaTPV)
	if p.tpv.Fallo == "" || len(p.tpv.Productos) != 0 {
		t.Fatalf("con el servicio caído: fallo %q, %d productos", p.tpv.Fallo, len(p.tpv.Productos))
	}
	caida = false
	p.IrA(VistaTPV)
	if p.tpv.Fallo != "" || len(p.tpv.Productos) != 4 {
		t.Errorf("con el servicio de vuelta: fallo %q, %d productos", p.tpv.Fallo, len(p.tpv.Productos))
	}
}

// TestElNombreDeLaCajaSeRecorta: los blancos no son un nombre y el tope es el de la web.
func TestElNombreDeLaCajaSeRecorta(t *testing.T) {
	if got := NombreCajaTPV("  Mostrador \t"); got != "Mostrador" {
		t.Errorf("recortado: %q", got)
	}
	if got := NombreCajaTPV("   "); got != "" {
		t.Errorf("solo blancos: %q", got)
	}
	largo := strings.Repeat("ñ", MaxNombreCajaTPV+20)
	if got := NombreCajaTPV(largo); len([]rune(got)) != MaxNombreCajaTPV {
		t.Errorf("%d caracteres; el tope es %d", len([]rune(got)), MaxNombreCajaTPV)
	}
}
