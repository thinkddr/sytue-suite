// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package syerphttp

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/syerp/db"
	"git.sytue.eu/sytue/one/services/syerp/internal/catalogo"
	"git.sytue.eu/sytue/one/services/syerp/internal/config"
	"git.sytue.eu/sytue/one/services/syerp/internal/domain"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

// LO QUE SE PRUEBA AQUÍ Y NO EN UN UNITARIO: la exportación solo es de verdad
// contra Postgres. El nombre de la relación sale de una consulta, el tope de
// filas solo corta si hay más filas que el tope y el recorrido por tramos con
// cursor no existe sin una tabla detrás. Un test con registros de mentira
// probaría el formateador, que es la parte que no falla.

// exportar llama al manejador como lo haría el router: con el tenant en el
// contexto y el modelo en los parámetros de ruta de chi.
func exportar(t *testing.T, h *Handlers, tenant, modelo, idioma string) [][]string {
	t.Helper()
	w := exportarCon(t, h, tenant, modelo, idioma, "")
	if w.Code != http.StatusOK {
		t.Fatalf("export %s: %d %s", modelo, w.Code, w.Body.String())
	}
	return leerCSV(t, w, modelo)
}

// exportarCon hace la petición con una consulta (`ids=…`) y devuelve la respuesta tal
// cual.
func exportarCon(t *testing.T, h *Handlers, tenant, modelo, idioma, consulta string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/"+modelo+"/export?"+consulta, http.NoBody)
	req.Header.Set("Accept-Language", idioma)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("modelo", modelo)
	ctx := tenancy.Into(req.Context(), tenancy.Tenant{Slug: tenant})
	req = req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.Exportar(w, req)
	return w
}

// leerCSV comprueba las cabeceras de la descarga y relee el CSV.
func leerCSV(t *testing.T, w *httptest.ResponseRecorder, modelo string) [][]string {
	t.Helper()
	if ct := w.Header().Get("Content-Type"); ct != "text/csv; charset=utf-8" {
		t.Errorf("Content-Type = %q", ct)
	}
	if cd := w.Header().Get("Content-Disposition"); !strings.HasPrefix(cd, `attachment; filename="`+modelo+`-`) {
		t.Errorf("Content-Disposition = %q", cd)
	}
	lector := csv.NewReader(strings.NewReader(w.Body.String()))
	// La fila de aviso del tope tiene UNA celda mientras el resto tiene las de
	// la lista; sin esto, el lector la tomaría por un fichero mal formado.
	lector.FieldsPerRecord = -1
	filas, err := lector.ReadAll()
	if err != nil {
		t.Fatalf("el CSV no se puede releer: %v\n%s", err, w.Body.String())
	}
	return filas
}

// crear valida y guarda un registro como lo haría el manejador de alta.
func crear(ctx context.Context, t *testing.T, st *store.Store, m *catalogo.Modelo, cuerpo string) store.Registro {
	t.Helper()
	data, etiqueta, err := domain.Validar(m, json.RawMessage(cuerpo))
	if err != nil {
		t.Fatalf("validar %s: %v", m.Clave, err)
	}
	reg, err := st.Crear(ctx, m, data, etiqueta, "ada")
	if err != nil {
		t.Fatalf("crear %s: %v", m.Clave, err)
	}
	return reg
}

// buscar devuelve la fila cuya primera celda es la dada: las filas salen por
// fecha descendente y dos altas seguidas pueden caer en el mismo microsegundo,
// así que el test no se juega nada al orden.
func buscar(t *testing.T, filas [][]string, primera string) []string {
	t.Helper()
	for _, f := range filas[1:] {
		if f[0] == primera {
			return f
		}
	}
	t.Fatalf("no está la fila %q en %q", primera, filas)
	return nil
}

func manejadores(t *testing.T, st *store.Store, tope int) *Handlers {
	t.Helper()
	c, err := catalogo.Cargar()
	if err != nil {
		t.Fatalf("cargar catálogo: %v", err)
	}
	return &Handlers{store: st, cat: c, cfg: config.Config{ExportMaxFilas: tope}}
}

func TestExportarCSV(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	hs := manejadores(t, st, 2)
	contacto, _ := hs.cat.Modelo("contacto")
	oportunidad, _ := hs.cat.Modelo("oportunidad")

	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	cliente := crear(ctx, t, st, contacto, `{"nombre":"Acme Ibérica","tipo":"organizacion"}`)
	// El nombre empieza por "=": es una fórmula en cuanto se abre el fichero.
	crear(ctx, t, st, oportunidad, `{"nombre":"=1+1","cliente":"`+cliente.ID+`","importe":123456}`)
	crear(ctx, t, st, oportunidad, `{"nombre":"Negativa","cliente":"`+cliente.ID+`","importe":-5}`)

	t.Run("cabecera traducida al idioma de la petición", func(t *testing.T) {
		filas := exportar(t, hs, "acme", "oportunidad", "es")
		quiero := []string{"Nombre", "Cliente", "Importe", "Probabilidad", "Vencimiento", "Estado"}
		if strings.Join(filas[0], "|") != strings.Join(quiero, "|") {
			t.Errorf("cabecera es = %q; se esperaba %q", filas[0], quiero)
		}
		filas = exportar(t, hs, "acme", "oportunidad", "en")
		if filas[0][0] != "Name" || filas[0][2] != "Amount" {
			t.Errorf("cabecera en = %q", filas[0])
		}
	})

	// "ref" y "estado" no son campos del modelo: su rótulo sale del diccionario
	// común y es el único sitio donde se comprueba que existe.
	t.Run("la columna ref también sale traducida", func(t *testing.T) {
		filas := exportar(t, hs, "acme", "factura_venta", "es")
		if filas[0][0] != "Referencia" || filas[0][len(filas[0])-1] != "Estado" {
			t.Errorf("cabecera de factura_venta = %q", filas[0])
		}
	})

	t.Run("valores neutralizados, relación por su nombre e importe legible", func(t *testing.T) {
		filas := exportar(t, hs, "acme", "oportunidad", "es")
		if len(filas) != 3 {
			t.Fatalf("se esperaba cabecera + 2 filas, hay %d: %q", len(filas), filas)
		}
		fila := buscar(t, filas, "'=1+1")
		if fila[0] != "'=1+1" {
			t.Errorf("la fórmula no se neutralizó: %q", fila[0])
		}
		if fila[1] != "Acme Ibérica" {
			t.Errorf("la relación salió como %q y no con su nombre", fila[1])
		}
		if strings.Contains(fila[1], cliente.ID) {
			t.Errorf("la relación salió con el UUID: %q", fila[1])
		}
		if fila[2] != "1234.56" {
			t.Errorf("importe = %q; 123456 céntimos son 1234.56", fila[2])
		}
		if fila[5] != "Nuevo" {
			t.Errorf("estado = %q; se esperaba el rótulo traducido", fila[5])
		}
		// Un importe negativo no puede salir "0.-5": la división es entera y el
		// signo se pone aparte justo para este caso.
		if neg := buscar(t, filas, "Negativa")[2]; neg != "-0.05" {
			t.Errorf("importe negativo = %q; -5 céntimos son -0.05", neg)
		}
	})

	t.Run("el tope de filas corta y lo dice", func(t *testing.T) {
		// Organización aparte: aquí hacen falta MÁS filas que el tope y el
		// subtest de arriba cuenta las suyas.
		ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "globex"})
		cliente := crear(ctx, t, st, contacto, `{"nombre":"Globex","tipo":"organizacion"}`)
		for _, n := range []string{"una", "dos", "tres"} {
			crear(ctx, t, st, oportunidad, `{"nombre":"`+n+`","cliente":"`+cliente.ID+`"}`)
		}

		filas := exportar(t, hs, "globex", "oportunidad", "es")
		// Cabecera + las 2 del tope + el aviso. Si el tope no cortara serían 5.
		if len(filas) != 4 {
			t.Fatalf("se esperaban 4 líneas (cabecera, 2 filas y aviso), hay %d: %q", len(filas), filas)
		}
		aviso := filas[3][0]
		if !strings.Contains(aviso, "cortada") || !strings.Contains(aviso, "2") {
			t.Errorf("el aviso de tope no dice qué pasó: %q", aviso)
		}
	})
}

// TestExportarSoloLasMarcadas: con `ids`, solo esas filas —las marcadas en la lista—;
// hasta el 2026-09-23 el servicio ignoraba el parámetro y la exportación en lote de la
// web y de la app nativa bajaba la lista entera.
//
// VISTO EN ROJO (agentes, 2026-09-23): sin la condición de `ids` en la consulta, salen
// las tres filas y no dos.
func TestExportarSoloLasMarcadas(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	hs := manejadores(t, st, 100)
	contacto, _ := hs.cat.Modelo("contacto")
	oportunidad, _ := hs.cat.Modelo("oportunidad")
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "initech"})
	cliente := crear(ctx, t, st, contacto, `{"nombre":"Initech","tipo":"organizacion"}`)
	una := crear(ctx, t, st, oportunidad, `{"nombre":"una","cliente":"`+cliente.ID+`"}`)
	crear(ctx, t, st, oportunidad, `{"nombre":"dos","cliente":"`+cliente.ID+`"}`)
	tres := crear(ctx, t, st, oportunidad, `{"nombre":"tres","cliente":"`+cliente.ID+`"}`)

	w := exportarCon(t, hs, "initech", "oportunidad", "es", "ids="+una.ID+","+tres.ID)
	if w.Code != http.StatusOK {
		t.Fatalf("export con ids: %d %s", w.Code, w.Body.String())
	}
	filas := leerCSV(t, w, "oportunidad")
	if len(filas) != 3 {
		t.Fatalf("se esperaba cabecera + las 2 marcadas, hay %d: %q", len(filas), filas)
	}
	buscar(t, filas, "una")
	buscar(t, filas, "tres")

	for _, mal := range []string{"ids=no-es-un-id", "ids=" + una.ID + ",DROP"} {
		if w := exportarCon(t, hs, "initech", "oportunidad", "es", mal); w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d; un identificador que no es UUID es un 400", mal, w.Code)
		}
	}
}

// TestExportarNoSeConfundeConUnID es la comprobación de enrutado: el router
// tiene /{modelo}/{id}, así que "export" podría entrar por ahí y acabar
// buscando un registro con ese identificador. chi recorre los hijos estáticos
// antes que los de parámetro, pero eso es un detalle de su implementación y
// esto lo fija contra el router DE VERDAD.
func TestExportarNoSeConfundeConUnID(t *testing.T) {
	c, err := catalogo.Cargar()
	if err != nil {
		t.Fatalf("cargar catálogo: %v", err)
	}
	// Sin base ni verificador: `Find` resuelve el árbol de rutas sin ejecutar
	// ni los middlewares ni el manejador.
	rutas, ok := NewRouter(Deps{Catalogo: c, Store: store.New(nil)}).(chi.Routes)
	if !ok {
		t.Fatal("NewRouter debería devolver un chi.Routes")
	}
	casos := map[string]string{
		"/api/v1/contacto/export":                               "/api/v1/{modelo}/export",
		"/api/v1/contacto/0b6f2c1e-0000-4000-8000-000000000001": "/api/v1/{modelo}/{id}",
	}
	for ruta, patron := range casos {
		if got := rutas.Find(chi.NewRouteContext(), http.MethodGet, ruta); got != patron {
			t.Errorf("GET %s casa con %q y no con %q", ruta, got, patron)
		}
	}
}
