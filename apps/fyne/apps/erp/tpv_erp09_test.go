// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"bytes"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
)

// almacenDePrueba es el almacén sin conexión de la suite en un directorio de la prueba.
func almacenDePrueba(t *testing.T) *offline.Store {
	t.Helper()
	st, err := offline.OpenWithSecrets(t.TempDir(), new(authtest.Secrets))
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// TestElCobroInciertoSobreviveACerrarLaApp es el de la web (`PuntoVenta.tsx`,
// `readOfflineValue(cuenta, 'erp-tpv', 'cobro')`): se pierde la respuesta del cobro con
// la venta YA registrada y la app se cierra. Al volver a abrirla, el punto de venta
// enseña la misma cesta congelada y el reintento lleva la MISMA operación: la caja acaba
// con una factura. Otra cuenta del mismo equipo no ve ese cobro.
//
// VISTO EN ROJO (2026-11-25): sin guardar el cobro pendiente, la app reabierta enseña la
// cesta vacía y el siguiente cobro es otra operación, otra factura.
func TestElCobroInciertoSobreviveACerrarLaApp(t *testing.T) {
	st := almacenDePrueba(t)
	caja := &cajaTPVFalsa{facturas: map[string]string{}, cortar: true}
	abrir := func(cuenta string) *TPV {
		p, _ := pantallaConServicio(t, caja.responder)
		p.Catalogo.Activos = append(p.Catalogo.Activos, "tpv")
		p.SabePermisos, p.Permisos = true, permisosTPV()
		p.CobrosTPV = AlmacenCobroTPVDe(st, func() string { return cuenta })
		p.IrA(VistaTPV)
		return p.tpv
	}
	tpv := abrir("ana@acme")
	cafe := tpv.Productos[0]
	tpv.Modificar(cafe, 1)
	tpv.Modificar(cafe, 1)
	tpv.PonerMedio(api.ERPMedioEfectivo)
	tpv.Recibido = "20"
	tpv.Cobrar()
	if !tpv.Incierto || tpv.Venta != nil {
		t.Fatalf("tras el corte: incierto %v, venta %v", tpv.Incierto, tpv.Venta)
	}
	operacion := tpv.pendiente.Operacion

	// Otra cuenta en el mismo equipo empieza con la cesta vacía.
	if otra := abrir("beto@acme"); len(otra.Lineas) != 0 || otra.Incierto {
		t.Fatalf("otra cuenta hereda el cobro: %d líneas, incierto %v", len(otra.Lineas), otra.Incierto)
	}

	// Se cierra la app y se vuelve a abrir: la misma cesta, incierta, con su operación.
	vuelta := abrir("ana@acme")
	if !vuelta.Incierto || vuelta.Cantidad(cafe.ID) != 2 || vuelta.Medio != api.ERPMedioEfectivo ||
		vuelta.Recibido != "20" || vuelta.pendiente == nil || vuelta.pendiente.Operacion != operacion {
		t.Fatalf("al volver: incierto %v, %d unidades, medio %s, recibido %q, pendiente %+v",
			vuelta.Incierto, vuelta.Cantidad(cafe.ID), vuelta.Medio, vuelta.Recibido, vuelta.pendiente)
	}
	vuelta.Modificar(cafe, 1)
	if vuelta.Cantidad(cafe.ID) != 2 {
		t.Error("la cesta recuperada se pudo cambiar con el cobro incierto")
	}
	vuelta.Cobrar()
	if vuelta.Venta == nil || len(caja.facturas) != 1 || caja.cuerpos[0] != caja.cuerpos[1] {
		t.Fatalf("reintento: venta %+v, %d facturas, cuerpos %q", vuelta.Venta, len(caja.facturas), caja.cuerpos)
	}
	// Confirmado, ya no hay nada guardado.
	if c, err := AlmacenCobroTPVDe(st, func() string { return "ana@acme" }).Leer(); err != nil || c != nil {
		t.Errorf("el cobro confirmado sigue guardado: %+v, %v", c, err)
	}
}

// Un cobro guardado de OTRA operación no se pisa: se recupera y se reintenta ese.
func TestOtroCobroSinConfirmarNoSePisa(t *testing.T) {
	st := almacenDePrueba(t)
	almacen := AlmacenCobroTPVDe(st, func() string { return "ana@acme" })
	cafe := productosTPV()[0]
	previo := CobroPendienteTPV{
		Cuerpo: api.ERPCobroTPV{
			Operacion: "op-previa", Sesion: sesionTPVArnes, Medio: api.ERPMedioTarjeta, Recibido: 605,
			Lineas: []api.ERPLineaTPV{{Producto: cafe.ID, Cantidad: 1, Precio: 605, TipoIVA: "21"}},
		},
		Lineas: []LineaTPV{{Producto: cafe, Cantidad: 1}},
	}
	if err := almacen.Guardar(previo); err != nil {
		t.Fatal(err)
	}
	if err := almacen.Guardar(CobroPendienteTPV{Cuerpo: api.ERPCobroTPV{Operacion: "otra"}}); !errors.Is(err, ErrOtroCobroTPV) {
		t.Fatalf("guardar otro cobro encima: %v", err)
	}
	// Quitar otra operación no lo toca.
	if err := almacen.Quitar("otra"); err != nil {
		t.Fatal(err)
	}
	tpv := NuevoTPV(traductor(t))
	tpv.Pendientes = almacen
	enviados := 0
	tpv.AlCobrar = func(api.ERPCobroTPV) { enviados++ }
	tpv.Sesion = sesionTPVArnes
	tpv.Modificar(productosTPV()[1], 1)
	tpv.Cobrar()
	if enviados != 0 || tpv.Fallo == "" || tpv.pendiente == nil || tpv.pendiente.Operacion != "op-previa" || tpv.Cantidad(cafe.ID) != 1 {
		t.Fatalf("se envió %d, fallo %q, pendiente %+v", enviados, tpv.Fallo, tpv.pendiente)
	}
	// «Nueva venta» lo descarta.
	tpv.NuevaVenta()
	if c, _ := almacen.Leer(); c != nil {
		t.Errorf("tras Nueva venta sigue guardado %+v", c)
	}
}

// pngDePrueba es una imagen de 800×600, más grande que lo que se guarda.
func pngDePrueba(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 800, 600))
	for y := range 600 {
		for x := range 800 {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: 90, A: 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// TestElTPVLlevaImagenesYPresupuesto: la pieza ERP-09. El producto con imagen la enseña
// (reducida a LadoImagenTPV), «Crear presupuesto» pide cliente y manda la cesta tal como
// se vio, y con permiso de escritura en productos sale «Imágenes».
func TestElTPVLlevaImagenesYPresupuesto(t *testing.T) {
	foto := pngDePrueba(t)
	caja := &cajaTPVFalsa{facturas: map[string]string{}}
	var presupuesto api.ERPPresupuestoTPV
	cafe := productosTPV()[0]
	p, sf := pantallaConServicio(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/productos/imagenes":
			if !strings.Contains(r.URL.Query().Get("registros"), cafe.ID) {
				t.Errorf("no se piden las imágenes del catálogo: %q", r.URL.RawQuery)
			}
			_, _ = io.WriteString(w, `{"imagenes":{"`+cafe.ID+`":"adj-cafe"}}`)
		case "/api/v1/adjuntos/adj-cafe":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(foto)
		case "/api/v1/tpv/presupuesto":
			_ = json.NewDecoder(r.Body).Decode(&presupuesto)
			_, _ = io.WriteString(w, `{"id":"pr-1","modelo":"presupuesto","ref":"P-2026-00001","data":{}}`)
		default:
			caja.responder(w, r)
		}
	})
	p.Catalogo.Activos = append(p.Catalogo.Activos, "tpv", "ventas")
	p.Catalogo.Modulos = append(p.Catalogo.Modulos, catalogoVentas().Modulos[0])
	permisos := permisosTPV()
	permisos.Modulos["productos"], permisos.Modulos["ventas"] = api.ERPUsuario, api.ERPUsuario
	permisos.Verbos["productos"] = []string{"escribir"}
	permisos.Verbos["ventas"] = []string{"crear"}
	p.SabePermisos, p.Permisos = true, permisos
	p.ElegirFicheros = func(func([]string)) {}
	p.IrA(VistaTPV)
	tpv := p.tpv

	img := tpv.Imagenes[cafe.ID]
	if img == nil || max(img.Bounds().Dx(), img.Bounds().Dy()) != LadoImagenTPV {
		t.Fatalf("la imagen del café: %v", img)
	}
	textos := textosDe(p.vista(tema.Claro))
	if !contiene(textos, "Imágenes") {
		t.Errorf("con permiso en productos no se ofrece «Imágenes»: %v", textos)
	}

	tpv.Modificar(cafe, 1)
	tpv.Modificar(cafe, 1)
	if textos := textosDe(p.vista(tema.Claro)); !contiene(textos, "Crear presupuesto") {
		t.Fatalf("con la cesta y permiso no sale «Crear presupuesto»: %v", textos)
	}
	tpv.CrearPresupuesto()
	if tpv.Fallo == "" || presupuesto.Operacion != "" {
		t.Fatalf("sin cliente no hay presupuesto: fallo %q, enviado %+v", tpv.Fallo, presupuesto)
	}
	tpv.PonerCliente("cli-1", "Bar Manolo")
	tpv.CrearPresupuesto()
	if presupuesto.Cliente != "cli-1" || presupuesto.Operacion == "" || len(presupuesto.Lineas) != 1 ||
		presupuesto.Lineas[0].Cantidad != 2 || presupuesto.Lineas[0].Precio != 605 || tpv.Ocupado || tpv.Fallo != "" {
		t.Fatalf("presupuesto %+v, ocupado %v, fallo %q", presupuesto, tpv.Ocupado, tpv.Fallo)
	}
	if !strings.Contains(strings.Join(sf.pedidas, "\n"), "POST /api/v1/tpv/presupuesto") {
		t.Error("no se pidió el presupuesto")
	}
}
