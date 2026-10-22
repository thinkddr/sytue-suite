// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// TestERPSeguimientoPideLoQueLaWeb: cada llamada va a la ruta y con el cuerpo que usa la
// web del ERP (Seguimiento.tsx, Tareas.tsx, Libro.tsx, ErpApp.tsx).
//
// VISTO EN ROJO (frontal, 2026-09-23): con `mias=true` en vez de `mias=1` —el servicio
// compara con «1» y no filtraría—, el test lo dice en la consulta de tareas.
func TestERPSeguimientoPideLoQueLaWeb(t *testing.T) {
	var mu sync.Mutex
	var vistas []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cuerpo, _ := io.ReadAll(r.Body)
		mu.Lock()
		vistas = append(vistas, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery+" "+string(cuerpo))
		mu.Unlock()
		switch {
		case r.URL.Path == "/api/v1/actividades" && r.Method == http.MethodGet:
			_, _ = io.WriteString(w, `{"actividades":[{"id":"a1","modelo":"gasto","registro":"g1","tipo":"subir_documento","resumen":"Sube la factura","vence":"2026-09-20","estado":"pendiente"}]}`)
		case r.URL.Path == "/api/v1/adjuntos" && r.Method == http.MethodPost:
			_, _ = io.WriteString(w, `{"tareas_hechas":1}`)
		case r.URL.Path == "/api/v1/factura_venta/f1/historial" && r.Method == http.MethodGet:
			_, _ = io.WriteString(w, `{"historial":[{"id":"h1","tipo":"cambio","cambios":{"estado":{"antes":"borrador","despues":"pagado"}},"creado_en":"2026-09-10T11:00:00Z"}]}`)
		case r.URL.Path == "/api/v1/balance":
			_, _ = io.WriteString(w, `{"cuentas":[{"cuenta":"430000","debe":100,"haber":0,"saldo":100}],"total_debe":100,"total_haber":100}`)
		case r.URL.Path == "/api/v1/factura_venta/lote":
			_, _ = io.WriteString(w, `{"afectados":2}`)
		case r.URL.Path == "/api/v1/factura_compra/importar":
			_, _ = io.WriteString(w, `{"filas":2,"creados":1,"actualizados":0,"fallos":[{"fila":3,"error":"total"}],"ignoradas":["color"]}`)
		case r.URL.Path == "/api/v1/verifactu/exportar":
			w.Header().Set("Content-Disposition", `attachment; filename="verifactu-registros.xml"`)
			_, _ = io.WriteString(w, "<registros/>")
		case r.URL.Path == "/api/v1/equipo":
			_, _ = io.WriteString(w, `{"personas":[{"sub":"yo1","nombre":"Daniel"},{"sub":"luis","correo":"luis@x.es"}],"yo":"yo1"}`)
		case r.URL.Path == "/api/v1/organizacion":
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"error":{"code":"no_encontrado","message":"not found"}}`)
		default:
			_, _ = io.WriteString(w, `{}`)
		}
	}))
	defer srv.Close()
	c, ctx := NewERP(srv.URL, nil, nil, "es"), context.Background()

	as, err := c.Actividades(ctx, ERPFiltroActividades{Estado: "pendiente", Mias: true, Hasta: "2026-09-23"})
	if err != nil || len(as) != 1 || as[0].Tipo != "subir_documento" {
		t.Fatalf("tareas: %+v, %v", as, err)
	}
	if n, err := c.SubirAdjunto(ctx, "gasto", "g1", "factura.pdf", "application/pdf", []byte("%PDF")); err != nil || n != 1 {
		t.Errorf("subir: %d, %v", n, err)
	}
	h, err := c.Historial(ctx, "factura_venta", "f1")
	if err != nil || h[0].Cambios["estado"].Despues != "pagado" {
		t.Errorf("historial: %+v, %v", h, err)
	}
	b, err := c.Balance(ctx)
	if err != nil || b.TotalDebe != 100 || b.Cuentas[0].Saldo != 100 {
		t.Errorf("balance: %+v, %v", b, err)
	}
	if n, err := c.Lote(ctx, "factura_venta", "estado", []string{"f1", "f2"}, "pagado"); err != nil || n != 2 {
		t.Errorf("lote: %d, %v", n, err)
	}
	if _, err := c.Organizacion(ctx); !EsNoEncontrado(err) {
		t.Errorf("sin perfil tiene que leerse como 404, no como corte: %v", err)
	}
	// «yo» marca a quien pregunta, para que el selector lo ponga como «Yo (Daniel)».
	if ps, err := c.Equipo(ctx); err != nil || len(ps) != 2 || !ps[0].Yo || ps[1].Yo {
		t.Errorf("equipo: %+v, %v", ps, err)
	}
	_ = c.CerrarActividad(ctx, "a1", "hecha")
	// Con responsable: el servicio avisa a esa persona (erp.task.assigned).
	_ = c.CrearActividad(ctx, "gasto", "g1", "tarea", "Pedir el IBAN", "2026-10-01", ERPPersona{Sub: "luis", Correo: "luis@x.es"})
	_ = c.Anotar(ctx, "factura_venta", "f1", "Llamado")
	_ = c.AnularAsiento(ctx, "as1", "Duplicado")
	_, _ = c.CambiarModulo(ctx, "crm", false)
	if res, err := c.Importar(ctx, "factura_compra", []byte("<Facturae/>"), true); err != nil || res.Creados != 1 ||
		res.Fallos[0].Fila != 3 || res.Ignoradas[0] != "color" {
		t.Errorf("importar: %+v, %v", res, err)
	}
	if x, n, err := c.ExportarVerifactu(ctx); err != nil || n != "verifactu-registros.xml" {
		t.Errorf("exportar: %q, %v", n, err)
	} else {
		_ = x.Close()
	}
	if x, n, err := c.DeclaracionVerifactu(ctx); err != nil || n != "declaracion-responsable.html" {
		t.Errorf("declaración: %q, %v", n, err)
	} else {
		_ = x.Close()
	}

	quiero := []string{
		"GET /api/v1/actividades?estado=pendiente&hasta=2026-09-23&mias=1 ",
		"POST /api/v1/adjuntos?modelo=gasto&nombre=factura.pdf&registro=g1 %PDF",
		"GET /api/v1/factura_venta/f1/historial? ",
		"GET /api/v1/balance? ",
		`POST /api/v1/factura_venta/lote? {"accion":"estado","ids":["f1","f2"],"estado":"pagado"}`,
		"GET /api/v1/organizacion? ",
		"GET /api/v1/equipo? ",
		"POST /api/v1/actividades/a1/hecha? {}",
		`POST /api/v1/actividades? {"modelo":"gasto","registro":"g1","responsable_nombre":"luis@x.es","responsable_sub":"luis","resumen":"Pedir el IBAN","tipo":"tarea","vence":"2026-10-01"}`,
		`POST /api/v1/factura_venta/f1/historial? {"texto":"Llamado"}`,
		`POST /api/v1/libro/as1/anular? {"motivo":"Duplicado"}`,
		`POST /api/v1/modulos/crm? {"activo":false}`,
		"POST /api/v1/factura_compra/importar? <Facturae/>",
		"GET /api/v1/verifactu/exportar? ",
		"GET /api/v1/verifactu/declaracion? ",
	}
	mu.Lock()
	defer mu.Unlock()
	if strings.Join(vistas, "\n") != strings.Join(quiero, "\n") {
		t.Errorf("peticiones:\n%s\nquiero:\n%s", strings.Join(vistas, "\n"), strings.Join(quiero, "\n"))
	}
}

// TestERPSeguimientoHablaElIdiomaDelServicio lee el código del servicio: si un nombre de
// campo o una ruta cambian allí, esto se pone en rojo aquí y no en producción.
func TestERPSeguimientoHablaElIdiomaDelServicio(t *testing.T) {
	for fichero, literales := range map[string][]string{
		"services/syerp/internal/http/equipo.go": {`json:"sub"`, `json:"nombre"`, `json:"correo"`, `"personas"`, `"yo"`},
		"services/syerp/internal/store/compartido.go": {
			`json:"registro_etiqueta,omitempty"`, `json:"responsable_nombre,omitempty"`, `json:"vence"`, `json:"tamano"`,
		},
		"services/syerp/internal/store/historial.go": {`json:"cambios,omitempty"`, `json:"antes"`, `json:"despues"`},
		"services/syerp/internal/store/libro.go":     {`json:"apuntes,omitempty"`, `json:"saldo"`, `json:"haber"`},
		"services/syerp/internal/store/store.go":     {`json:"pendiente_cobro_centimos"`, `json:"proximo_vencimiento,omitempty"`, `json:"cuadra"`},
		"services/syerp/internal/store/verifactu.go": {`json:"con_errores"`, `json:"rechazadas"`, `json:"error_envio,omitempty"`},
		"services/syerp/internal/http/server.go": {
			`api.Get("/actividades"`, `"/actividades/{id}/hecha"`, `"/actividades/{id}/cancelar"`, `api.Post("/adjuntos"`,
			`"/libro/{id}/anular"`, `api.Get("/tableros"`, `api.Get("/equipo"`, `mod.Post("/lote"`, `Post("/modulos/{modulo}"`,
			`Get("/verifactu/exportar"`, `api.Get("/verifactu/declaracion"`, `Post("/importar", h.Importar)`,
		},
		"services/syerp/internal/http/importar.go":     {`json:"ignoradas"`},
		"services/syerp/internal/store/importacion.go": {`json:"creados"`, `json:"actualizados"`, `json:"fila"`},
		"services/syerp/internal/http/actividades.go":  {`q.Get("mias") == "1"`, `Get("nombre")`},
	} {
		codigo := webref.Read(t, fichero)
		for _, l := range literales {
			if !strings.Contains(codigo, l) {
				t.Errorf("%s ya no dice %s", fichero, l)
			}
		}
	}
}
