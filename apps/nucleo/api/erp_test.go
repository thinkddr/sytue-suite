// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

func TestERP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept-Language") != "de" {
			t.Errorf("Accept-Language = %q: el catálogo llegaría en otro idioma", r.Header.Get("Accept-Language"))
		}
		switch r.URL.Path {
		case "/api/v1/catalogo":
			_, _ = io.WriteString(w, `{"modulos":[{"clave":"facturacion","etiqueta":"Rechnungen","modelos":[{"clave":"factura_venta","modulo":"facturacion","etiqueta":"Ausgangsrechnung","campos":[{"clave":"total","tipo":"dinero","etiqueta":"Gesamt"}],"lista":["total"],"estados":[{"valor":"emitida","etiqueta":"Ausgestellt"}]}]}],"activos":["facturacion"]}`)
		case "/api/v1/factura_venta/":
			if r.URL.RawQuery != "cursor=c1&limite=50&q=acme" {
				t.Errorf("consulta de la lista: %s", r.URL.RawQuery)
			}
			_, _ = io.WriteString(w, `{"registros":[{"id":"r1","ref":"F-2026-0007","etiqueta":"ACME","estado":"emitida","data":{"total":121000,"cliente":"k9"}}],"etiquetas":{"k9":"ACME, S.L."},"siguiente":"c2"}`)
		default:
			t.Errorf("petición inesperada: %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	c, ctx := NewERP(srv.URL, nil, nil, "de"), context.Background()
	cat, err := c.Catalogo(ctx)
	if err != nil || len(cat.Modulos) != 1 || cat.Modulos[0].Modelos[0].Campos[0].Tipo != ERPDinero || cat.Activos[0] != "facturacion" {
		t.Fatalf("catálogo: %+v, %v", cat, err)
	}
	pag, err := c.Listar(ctx, "factura_venta", "acme", "c1", 50)
	if err != nil || pag.Siguiente != "c2" || len(pag.Registros) != 1 || pag.Etiquetas["k9"] != "ACME, S.L." || pag.Registros[0].Data["total"] != float64(121000) {
		t.Fatalf("página: %+v, %v", pag, err)
	}
}

// Las cuatro escrituras: qué método y qué ruta pega cada una, y que el cuerpo viaja
// como el servicio lo lee («data» envuelto, el importe en céntimos ENTEROS).
func TestERPEscribe(t *testing.T) {
	var visto []string
	var cuerpos []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		datos, _ := io.ReadAll(r.Body)
		visto, cuerpos = append(visto, r.Method+" "+r.URL.Path), append(cuerpos, string(datos))
		_, _ = io.WriteString(w, `{"id":"r1","modelo":"factura_venta","ref":"FV-2026-00042","estado":"emitida","data":{"total":121000}}`)
	}))
	defer srv.Close()
	c, ctx := NewERP(srv.URL, nil, nil, "es"), context.Background()

	if _, err := c.Crear(ctx, "factura_venta", map[string]any{"total": int64(121000)}); err != nil {
		t.Fatalf("crear: %v", err)
	}
	if _, err := c.Actualizar(ctx, "factura_venta", "r1", map[string]any{"total": int64(121000)}); err != nil {
		t.Fatalf("actualizar: %v", err)
	}
	reg, err := c.CambiarEstado(ctx, "factura_venta", "r1", "emitida")
	if err != nil || reg.Ref != "FV-2026-00042" {
		t.Fatalf("estado: %+v, %v", reg, err)
	}
	if _, err := c.Obtener(ctx, "factura_venta", "r1"); err != nil {
		t.Fatalf("obtener: %v", err)
	}
	quiere := []string{
		"POST /api/v1/factura_venta/",
		"PUT /api/v1/factura_venta/r1",
		"POST /api/v1/factura_venta/r1/estado",
		"GET /api/v1/factura_venta/r1",
	}
	if !slices.Equal(visto, quiere) {
		t.Errorf("rutas = %q; quiere %q", visto, quiere)
	}
	// El importe sale SIN decimales y envuelto en «data»: 121000 céntimos, no 1210.00.
	if cuerpos[0] != `{"data":{"total":121000}}` {
		t.Errorf("cuerpo del alta = %s", cuerpos[0])
	}
	if cuerpos[2] != `{"estado":"emitida"}` {
		t.Errorf("cuerpo del cambio de estado = %s", cuerpos[2])
	}
}

// Los filtros de la lista viajan con los nombres que lee `filtroDe`, y lo vacío no viaja:
// un `estado=` vacío el servicio lo validaría contra el modelo y contestaría 400.
func TestERPListaFiltradaYSusRecuentos(t *testing.T) {
	var consultas []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/factura_venta/":
			consultas = append(consultas, r.URL.RawQuery)
			_, _ = io.WriteString(w, `{"registros":[]}`)
		case "GET /api/v1/factura_venta/estadisticas":
			_, _ = io.WriteString(w, `{"modelo":"factura_venta","total":42,"por_estado":{"pagado":40,"borrador":2},"suma_dinero":1210000}`)
		case "DELETE /api/v1/factura_venta/r1", "POST /api/v1/factura_venta/r1/restaurar":
			w.WriteHeader(http.StatusNoContent)
		case "POST /api/v1/factura_venta/r1/duplicar":
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"id":"r2","estado":"borrador","data":{},"creado_en":"2026-09-22T10:00:00Z"}`)
		default:
			t.Errorf("petición inesperada: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	c, ctx := NewERP(srv.URL, nil, nil, "es"), context.Background()
	for _, f := range []ERPFiltro{
		{Q: "acme", Estado: "pagado", Orden: "total", Asc: true, Papelera: true, Cursor: "c1", Limite: 50},
		{Orden: "total", Limite: 50},
		{Asc: true, Limite: 50}, // sin orden, «asc» no significa nada y no se manda
	} {
		if _, err := c.ListarCon(ctx, "factura_venta", f); err != nil {
			t.Fatal(err)
		}
	}
	quiere := []string{
		"cursor=c1&dir=asc&estado=pagado&limite=50&orden=total&papelera=1&q=acme",
		"limite=50&orden=total",
		"limite=50",
	}
	if !slices.Equal(consultas, quiere) {
		t.Errorf("consultas = %q; quiere %q", consultas, quiere)
	}
	st, err := c.Estadisticas(ctx, "factura_venta")
	if err != nil || st.Total != 42 || st.PorEstado["borrador"] != 2 || st.SumaDinero != 1210000 {
		t.Fatalf("estadísticas: %+v, %v", st, err)
	}
	if err := c.Borrar(ctx, "factura_venta", "r1"); err != nil {
		t.Fatal(err)
	}
	if err := c.Restaurar(ctx, "factura_venta", "r1"); err != nil {
		t.Fatal(err)
	}
	copia, err := c.Duplicar(ctx, "factura_venta", "r1")
	if err != nil || copia.ID != "r2" || copia.CreadoEn.IsZero() {
		t.Fatalf("duplicar: %+v, %v", copia, err)
	}
}

func TestERPPermisosAlcanza(t *testing.T) {
	p := ERPPermisos{Modulos: map[string]string{"ventas": ERPUsuario, "contabilidad": ERPLectura}}
	casos := []struct {
		modulo, nivel string
		quiere        bool
	}{
		{"ventas", ERPLectura, true},
		{"ventas", ERPUsuario, true},
		{"ventas", ERPAdministrador, false},
		{"contabilidad", ERPUsuario, false},
		{"almacen", ERPLectura, false}, // sin nivel puesto: cerrado, no abierto
	}
	for _, c := range casos {
		if got := p.Alcanza(c.modulo, c.nivel); got != c.quiere {
			t.Errorf("Alcanza(%q, %q) = %v", c.modulo, c.nivel, got)
		}
	}
}

func TestLosCamposDelERPSonLosDelServicio(t *testing.T) {
	catalogo := webref.Read(t, "services/syerp/internal/catalogo/catalogo.go")
	for _, literal := range []string{
		`json:"modelos"`, `json:"campos"`, `json:"lista"`, `json:"estados,omitempty"`, `json:"valor"`, `json:"opciones,omitempty"`,
		`TipoDinero     = "` + ERPDinero + `"`, `TipoFecha      = "` + ERPFecha + `"`, `TipoFechaHora  = "` + ERPFechaHora + `"`,
		`TipoBooleano   = "` + ERPBooleano + `"`, `TipoSeleccion  = "` + ERPSeleccion + `"`, `TipoRelacion   = "` + ERPRelacion + `"`,
		`TipoEntero     = "` + ERPEntero + `"`, `TipoDecimal    = "` + ERPDecimal + `"`,
		`TipoTextoLargo = "` + ERPTextoLargo + `"`,
		// Lo que el formulario necesita saber de cada campo y de cada modelo.
		`json:"requerido,omitempty"`, `json:"modelo,omitempty"`, `json:"numerado"`, `json:"contable"`,
	} {
		if !strings.Contains(catalogo, literal) {
			t.Errorf("services/syerp/internal/catalogo ya no dice %s", literal)
		}
	}
	almacen := webref.Read(t, "services/syerp/internal/store/store.go")
	for _, literal := range []string{
		`json:"registros"`, `json:"etiquetas,omitempty"`, `json:"siguiente,omitempty"`, `json:"editado_en"`,
		`json:"creado_en"`, `json:"ref,omitempty"`, `json:"total"`, `json:"por_estado"`, `json:"suma_dinero,omitempty"`,
	} {
		if !strings.Contains(almacen, literal) {
			t.Errorf("services/syerp/internal/store ya no dice %s", literal)
		}
	}
	manejadores := webref.Read(t, "services/syerp/internal/http/handlers.go")
	// «limite», no «limit»: el servicio ignora en silencio el que no conoce.
	for _, literal := range []string{
		`q.Get("limite")`, `q.Get("cursor")`, `Texto:  q.Get("q")`, `json:"modulos"`, `json:"activos"`,
		`Estado: q.Get("estado")`, `q.Get("orden")`, `q.Get("dir") == "asc"`, `q.Get("papelera") == "1"`,
		"httpx.LanguageFromRequest(r)",
		// Los dos cuerpos que manda esta vista al escribir.
		"Data json.RawMessage `json:\"data\"`", "Estado string `json:\"estado\"`",
	} {
		if !strings.Contains(manejadores, literal) {
			t.Errorf("services/syerp/internal/http/handlers.go ya no dice %s", literal)
		}
	}
	niveles := webref.Read(t, "services/syerp/internal/permisos/permisos.go")
	for _, literal := range []string{
		`Ninguno       = "` + ERPNinguno + `"`, `Lectura       = "` + ERPLectura + `"`,
		`Usuario       = "` + ERPUsuario + `"`, `Administrador = "` + ERPAdministrador + `"`,
	} {
		if !strings.Contains(niveles, literal) {
			t.Errorf("services/syerp/internal/permisos ya no dice %s", literal)
		}
	}
	rutas := webref.Read(t, "services/syerp/internal/http/server.go")
	for _, ruta := range []string{
		`api.Get("/catalogo", h.Catalogo)`, `api.Route("/{modelo}"`, `mod.Get("/", h.Listar)`,
		`api.Get("/mis-permisos", h.MisPermisos)`,
		`mod.Post("/", h.Crear)`, `mod.Get("/{id}", h.Obtener)`, `mod.Put("/{id}", h.Actualizar)`,
		`mod.Post("/{id}/estado", h.CambiarEstado)`,
		`mod.Get("/estadisticas", h.Estadisticas)`, `mod.Delete("/{id}", h.Borrar)`,
		`mod.Post("/{id}/restaurar", h.Restaurar)`, `mod.Post("/{id}/duplicar", h.Duplicar)`,
		// La factura de cara afuera: lo que se imprime, lo que se manda y lo que la ley
		// obliga a registrar desde el 1-1-2027.
		`mod.Get("/{id}/ubl", h.UBL)`, `mod.Get("/{id}/facturae", h.Facturae)`,
		`mod.Get("/{id}/imprimir", h.Imprimir)`,
		`mod.Get("/{id}/verifactu", h.RegistrosFacturaVerifactu)`,
		`mod.Post("/{id}/verifactu/subsanar", h.SubsanarVerifactu)`,
	} {
		if !strings.Contains(rutas, ruta) {
			t.Errorf("services/syerp ya no monta %s", ruta)
		}
	}
	// Imprimir y VERI*FACTU son SOLO de la factura de cliente, y el servicio lo decide
	// por el nombre del modelo: si cambiara, la vista ofrecería botones que dan 422.
	imprimir := webref.Read(t, "services/syerp/internal/http/imprimir.go")
	if !strings.Contains(imprimir, `m.Clave != "`+ERPFacturaVenta+`"`) {
		t.Errorf("services/syerp ya no imprime solo %q", ERPFacturaVenta)
	}
	if !strings.Contains(imprimir, `"text/html; charset=utf-8"`) {
		t.Error("services/syerp ya no sirve la factura impresa como HTML: revisar la extensión con que se guarda")
	}
	// Facturae y UBL, solo de lo que se EMITE: el catálogo lo dice en `cobro`.
	if !strings.Contains(webref.Read(t, "services/syerp/internal/catalogo/catalogo.go"), "Cobro bool `json:\"cobro,omitempty\"`") {
		t.Error("el catálogo de services/syerp ya no publica `cobro`: revisar ERPModelo.Cobro")
	}
	estados := webref.Read(t, "apps/frontend/apps/erp/src/components/Verifactu.tsx")
	for _, e := range []string{ERPVFPendiente, ERPVFCorrecto, ERPVFConErrores, ERPVFIncorrecto} {
		if !strings.Contains(estados, e+":") {
			t.Errorf("la web ya no reconoce el estado %q de VERI*FACTU", e)
		}
	}
}

// Los documentos legales de una factura y lo que contestó la Agencia Tributaria.
func TestERPFacturaHaciaFuera(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/factura_venta/f1/imprimir":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = io.WriteString(w, "<!doctype html><p>FV-2026-00042")
		case "GET /api/v1/factura_venta/f1/facturae":
			w.Header().Set("Content-Disposition", `attachment; filename="facturae-FV-2026-00042.xml"`)
			_, _ = io.WriteString(w, "<Facturae/>")
		case "GET /api/v1/factura_venta/f1/ubl":
			// Sin cabecera: se usa el nombre de respaldo en vez de quedarse sin nombre.
			_, _ = io.WriteString(w, "<Invoice/>")
		case "GET /api/v1/factura_venta/f1/verifactu":
			_, _ = io.WriteString(w, `{"registros":[{"id":"r1","tipo":"alta","estado":"incorrecto","codigo_error":1103,"descripcion_error":"NIF no identificado","num_serie":"FV-2026-00042"}],"qr":"https://prewww2.aeat.es/QR"}`)
		case "POST /api/v1/factura_venta/f1/verifactu/subsanar":
			_, _ = io.WriteString(w, `{"registros":[{"id":"r1","tipo":"alta","estado":"incorrecto"},{"id":"r2","tipo":"alta","estado":"correcto","csv":"ABC123"}],"qr":"https://prewww2.aeat.es/QR"}`)
		default:
			t.Errorf("petición inesperada: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	e, ctx := NewERP(srv.URL, nil, nil, "es"), context.Background()

	leer := func(c io.ReadCloser, err error) string {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = c.Close() }()
		datos, err := io.ReadAll(c)
		if err != nil {
			t.Fatal(err)
		}
		return string(datos)
	}
	if html := leer(e.Imprimir(ctx, ERPFacturaVenta, "f1")); !strings.Contains(html, "FV-2026-00042") {
		t.Errorf("la factura impresa: %q", html)
	}
	cuerpo, nombre, err := e.Facturae(ctx, ERPFacturaVenta, "f1")
	if xml := leer(cuerpo, err); xml != "<Facturae/>" || nombre != "facturae-FV-2026-00042.xml" {
		t.Errorf("Facturae: %q en %q", xml, nombre)
	}
	cuerpo, nombre, err = e.UBL(ctx, ERPFacturaVenta, "f1")
	if xml := leer(cuerpo, err); xml != "<Invoice/>" || nombre != "factura.xml" {
		t.Errorf("UBL: %q en %q", xml, nombre)
	}

	f, err := e.RegistrosVerifactu(ctx, ERPFacturaVenta, "f1")
	if err != nil {
		t.Fatal(err)
	}
	ultimo, hay := f.Ultimo()
	if !hay || !ultimo.Corregible() || ultimo.Codigo == nil || *ultimo.Codigo != 1103 || f.QR == "" {
		t.Fatalf("registros: %+v, qr %q", f.Registros, f.QR)
	}
	f, err = e.SubsanarVerifactu(ctx, ERPFacturaVenta, "f1")
	if err != nil {
		t.Fatal(err)
	}
	// El que manda es el ÚLTIMO: tras subsanar, la factura está aceptada aunque el
	// rechazo siga en la lista.
	if ultimo, _ = f.Ultimo(); ultimo.Estado != ERPVFCorrecto || ultimo.CSV != "ABC123" || ultimo.Corregible() {
		t.Errorf("tras subsanar manda %+v", ultimo)
	}
}

// El nivel de usuario no implica un gesto cuando el servidor ha resuelto una
// lista de verbos. Incluso un administrador puede tener funciones segregadas.
func TestERPPermisosConservaVerbosEfectivos(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/mis-permisos" {
			t.Errorf("ruta inesperada: %s", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"administrador":true,"modulos":{"facturacion":"usuario","contabilidad":"usuario","ventas":"lectura","tpv":"administrador"},"verbos":{"facturacion":["crear"],"contabilidad":["conciliar"],"ventas":["asentar"],"tpv":[]}}`)
	}))
	defer srv.Close()
	p, err := NewERP(srv.URL, nil, nil, "es").MisPermisos(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		modulo, verbo string
		permitido     bool
	}{
		{"facturacion", "crear", true},
		{"facturacion", "asentar", false},
		{"contabilidad", "conciliar", true},
		{"contabilidad", "asentar", false},
		{"ventas", "asentar", false},       // El verbo tampoco sustituye la escritura.
		{"tpv", "crear", false},            // Una lista vacía es una denegación explícita.
		{"obligaciones", "asentar", false}, // Campo ausente: cerrado.
	} {
		if got := p.PuedeAccion(c.modulo, c.verbo); got != c.permitido {
			t.Errorf("PuedeAccion(%q, %q) = %v; quiere %v", c.modulo, c.verbo, got, c.permitido)
		}
	}
}
