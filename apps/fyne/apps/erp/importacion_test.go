// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// dialogoArriba es el diálogo de la suite que hay encima en la ventana, si hay uno.
func dialogoArriba(w fyne.Window) *ui.Dialogo {
	var d *ui.Dialogo
	if top := w.Canvas().Overlays().Top(); top != nil {
		recorrer(top, func(o fyne.CanvasObject) {
			if x, ok := o.(*ui.Dialogo); ok && d == nil {
				d = x
			}
		})
	}
	return d
}

// pulsarDelPie pulsa el botón del pie del diálogo cuyo rótulo es `rotulo`.
func pulsarDelPie(t *testing.T, d *ui.Dialogo, rotulo string) {
	t.Helper()
	var b *ui.Boton
	recorrer(d.Pie, func(o fyne.CanvasObject) {
		if x, ok := o.(*ui.Boton); ok && b == nil && contiene(textosDe(x), rotulo) {
			b = x
		}
	})
	if b == nil {
		t.Fatalf("el pie no tiene «%s»", rotulo)
	}
	b.Tapped(nil)
}

// importarConVentana monta la pantalla con un servicio que simula 2 altas y una fila
// mala, importa el lote «lote-1» y lo revierte; elige «clientes.csv».
func importarConVentana(t *testing.T) (*Pantalla, *servicioFalso, fyne.Window) {
	t.Helper()
	p, sf := pantallaConServicio(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/importar") && r.URL.Query().Get("simular") == "1":
			_, _ = io.WriteString(w, `{"filas":3,"creados":2,"actualizados":0,"simulacion":true,`+
				`"fallos":[{"fila":4,"error":"total no es un importe"}]}`)
		case strings.HasSuffix(r.URL.Path, "/importar"):
			_, _ = io.WriteString(w, `{"id":"lote-1","filas":3,"creados":2,"actualizados":0,"fallos":[]}`)
		case r.URL.Path == "/api/v1/importaciones/lote-1/revertir":
			_, _ = io.WriteString(w, `{"borrados":2,"restaurados":0,"intactos":1}`)
		default:
			_, _ = io.WriteString(w, `{"registros":[]}`)
		}
	})
	w := test.NewWindow(p.raiz)
	w.Resize(fyne.NewSize(1280, 800))
	t.Cleanup(w.Close)
	p.lienzo = w.Canvas()
	p.Abrir(facturas())
	ruta := filepath.Join(t.TempDir(), "clientes.csv")
	if err := os.WriteFile(ruta, []byte("ref,total\nF1,1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	p.ElegirFicheros = func(listo func([]string)) { listo([]string{ruta}) }
	// Revertir pide `administrar` en contabilidad.
	p.SabePermisos = true
	p.Permisos = api.ERPPermisos{
		Modulos: map[string]string{"facturacion": api.ERPAdministrador, "contabilidad": api.ERPAdministrador},
		Verbos:  map[string][]string{"facturacion": {"administrar"}, "contabilidad": {"administrar"}},
	}
	return p, sf, w
}

func importaciones(sf *servicioFalso) []string {
	var out []string
	for _, v := range sf.vistas() {
		if strings.Contains(v, "/importar") || strings.Contains(v, "/importaciones/") {
			out = append(out, v)
		}
	}
	return out
}

// TestImportarEnseñaLaVistaPreviaYDeshaceElLote: la importación pregunta primero con
// lo que pasaría (simulado en el servicio), importa al aceptar y el aviso lleva
// «Deshacer», que revierte el lote por su id. Es el flujo de `importarFicheros`.
//
// VISTO EN ROJO: llamando a `si()` sin abrir el diálogo, no hay vista previa y el CSV
// entra de una; sin `RotuloAccion` en el aviso, no hay «Deshacer»; contando los lotes
// sin su id, el POST de revertir no sale.
func TestImportarEnseñaLaVistaPreviaYDeshaceElLote(t *testing.T) {
	p, sf, w := importarConVentana(t)
	p.importar()

	d := dialogoArriba(w)
	if d == nil {
		t.Fatalf("no se abrió la vista previa; se pidió %v", importaciones(sf))
	}
	if txt := textosDe(d); !contiene(txt, "Se crearán 2") || !contiene(txt, "Fila 4") {
		t.Errorf("la vista previa no cuenta lo que pasaría: %v", txt)
	}
	if got := importaciones(sf); len(got) != 1 || !strings.Contains(got[0], "simular=1") {
		t.Fatalf("antes de aceptar solo se simula: %v", got)
	}
	pulsarDelPie(t, d, "Importar")

	avisos := p.avisos.Avisos()
	i := slices.IndexFunc(avisos, func(a *ui.Aviso) bool { return a.RotuloAccion == "Deshacer" })
	if i < 0 {
		t.Fatalf("el aviso del resultado no lleva «Deshacer»: %d avisos", len(avisos))
	}
	avisos[i].AlPulsarAccion()

	quiere := []string{
		"POST /api/v1/factura_venta/importar?fichero=clientes.csv&simular=1",
		"POST /api/v1/factura_venta/importar?fichero=clientes.csv",
		"POST /api/v1/importaciones/lote-1/revertir?",
	}
	if got := importaciones(sf); !slices.Equal(got, quiere) {
		t.Errorf("peticiones:\n%q\nquiere\n%q", got, quiere)
	}
	ultimo := p.avisos.Avisos()[len(p.avisos.Avisos())-1]
	if ultimo.Titulo != "Importación deshecha: 2 borrados y 0 restaurados" || !strings.Contains(ultimo.Descripcion, "1 registros") {
		t.Errorf("aviso de deshacer: %q / %q", ultimo.Titulo, ultimo.Descripcion)
	}
}

// TestCancelarLaVistaPreviaNoImportaNada: cancelar deja todo como estaba.
func TestCancelarLaVistaPreviaNoImportaNada(t *testing.T) {
	p, sf, w := importarConVentana(t)
	p.importar()
	d := dialogoArriba(w)
	if d == nil {
		t.Fatal("no se abrió la vista previa")
	}
	pulsarDelPie(t, d, "Cancelar")
	if got := importaciones(sf); len(got) != 1 {
		t.Errorf("cancelar importó igual: %v", got)
	}
	if p.lista.Importando {
		t.Error("la lista se quedó importando")
	}
}

// TestSumarPreviaPoneElFicheroDelanteConVarios es el `sumarPrevia` de la web.
func TestSumarPreviaPoneElFicheroDelanteConVarios(t *testing.T) {
	fila := func(n int, e string) string { return "Fila " + string(rune('0'+n)) + ": " + e }
	uno := simulado{nombre: "a.csv"}
	uno.res.Creados = 1
	dos := simulado{nombre: "b.csv"}
	dos.res.Fallos = append(dos.res.Fallos, struct {
		Fila  int    `json:"fila"`
		Error string `json:"error"`
	}{2, "y"})
	p := sumarPrevia([]simulado{uno, dos}, fila)
	if p.creados != 1 || !slices.Equal(p.fallos, []string{"b.csv · Fila 2: y"}) || !p.merecePreguntar() {
		t.Errorf("previa = %+v", p)
	}
	if (previa{fallos: []string{"x"}}).merecePreguntar() {
		t.Error("sin nada que entraría no se pregunta")
	}
}

// TestSinAdministrarContabilidadNoSeOfreceDeshacerLaImportacion: importar pide
// `administrar` en el módulo del modelo, pero revertir lo pide en contabilidad. Sin él,
// «Deshacer» siempre contestaría «sin permiso»: no se ofrece.
//
// VISTO EN ROJO: sin mirar `PuedeAccion("contabilidad", "administrar")`, el aviso lleva
// «Deshacer».
func TestSinAdministrarContabilidadNoSeOfreceDeshacerLaImportacion(t *testing.T) {
	p, _, w := importarConVentana(t)
	delete(p.Permisos.Verbos, "contabilidad")
	p.importar()
	d := dialogoArriba(w)
	if d == nil {
		t.Fatal("no se abrió la vista previa")
	}
	pulsarDelPie(t, d, "Importar")
	for _, a := range p.avisos.Avisos() {
		if a.RotuloAccion != "" {
			t.Errorf("sin administrar contabilidad se ofrece %q en %q", a.RotuloAccion, a.Titulo)
		}
	}
}

// TestDeshacerLaImportacionNoCierraLaFichaAbierta: lo mismo que el borrado; el
// deshacer llega cuando ya puede haber otra ficha abierta.
//
// VISTO EN ROJO: con `p.escrito(m)` (lo de antes), `p.ficha` queda a nil.
func TestDeshacerLaImportacionNoCierraLaFichaAbierta(t *testing.T) {
	p, _, w := importarConVentana(t)
	p.importar()
	pulsarDelPie(t, dialogoArriba(w), "Importar")
	avisos := p.avisos.Avisos()
	i := slices.IndexFunc(avisos, func(a *ui.Aviso) bool { return a.RotuloAccion == "Deshacer" })
	if i < 0 {
		t.Fatal("sin «Deshacer»")
	}
	p.NuevaFicha(nil)
	nueva := p.ficha
	avisos[i].AlPulsarAccion()
	if nueva == nil || p.ficha != nueva {
		t.Error("deshacer la importación cerró la ficha abierta")
	}
}

// importarConSimulacion monta la pantalla con una simulación que contesta `estado` y una
// importación de verdad que entra; elige «extracto.csv».
func importarConSimulacion(t *testing.T, estado int) (*Pantalla, *servicioFalso) {
	t.Helper()
	p, sf := pantallaConServicio(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/importar") && r.URL.Query().Get("simular") == "1":
			w.WriteHeader(estado)
			_, _ = io.WriteString(w, `{"error":{"code":"x","message":"motivo del servicio"}}`)
		case strings.HasSuffix(r.URL.Path, "/importar"):
			_, _ = io.WriteString(w, `{"filas":3,"creados":3,"actualizados":0,"fallos":[]}`)
		default:
			_, _ = io.WriteString(w, `{"registros":[]}`)
		}
	})
	p.Abrir(facturas())
	ruta := filepath.Join(t.TempDir(), "extracto.csv")
	if err := os.WriteFile(ruta, []byte("11..."), 0o600); err != nil {
		t.Fatal(err)
	}
	p.ElegirFicheros = func(listo func([]string)) { listo([]string{ruta}) }
	return p, sf
}

// TestSiNoSePuedeSimularPorElServicioNoSeImportaACiegas: con la red caída o un 5xx no
// hay vista previa, y antes se importaba directamente sin preguntar. Ahora se dice y no
// se importa nada. Un 4xx sí es del fichero (un Norma 43 con extensión .csv, que el
// servicio no simula): ese entra sin vista previa, UNA vez.
//
// VISTO EN ROJO: descartando el error de la simulación (lo de antes), con el 503 sale
// el POST de verdad y no hay aviso de peligro.
func TestSiNoSePuedeSimularPorElServicioNoSeImportaACiegas(t *testing.T) {
	p, sf := importarConSimulacion(t, http.StatusServiceUnavailable)
	p.importar()
	if got := importaciones(sf); len(got) != 1 {
		t.Errorf("con la simulación caída se importó igual: %v", got)
	}
	if av := ultimoAviso(t, p); av.Tono != ui.AvisoPeligro || av.Titulo != p.t("list.importPreviewFailed") {
		t.Errorf("aviso: %q (%v)", av.Titulo, av.Tono)
	}
	if p.lista.Importando {
		t.Error("la lista se quedó importando")
	}

	// Un 401 de sesión caducada, un 408 o un 429 de límite tampoco son del fichero.
	// VISTO EN ROJO con «cualquier 4xx es del fichero»: salía el POST de verdad.
	for _, estado := range []int{http.StatusUnauthorized, http.StatusRequestTimeout, http.StatusTooManyRequests} {
		p, sf = importarConSimulacion(t, estado)
		p.importar()
		if got := importaciones(sf); len(got) != 1 {
			t.Errorf("con un %d en la simulación se importó igual: %v", estado, got)
		}
	}

	p, sf = importarConSimulacion(t, http.StatusUnprocessableEntity)
	p.importar()
	quiere := []string{
		"POST /api/v1/factura_venta/importar?fichero=extracto.csv&simular=1",
		"POST /api/v1/factura_venta/importar?fichero=extracto.csv",
	}
	if got := importaciones(sf); !slices.Equal(got, quiere) {
		t.Errorf("con un 422 del fichero:\n%q\nquiere\n%q", got, quiere)
	}
}
