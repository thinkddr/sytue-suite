// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// El aislamiento del ERP NO puede depender solo de la RLS.
//
// El resto de tests de este paquete conectan con el rol de la aplicación
// (NOSUPERUSER, NOBYPASSRLS), así que lo que demuestran es que las políticas
// aíslan. Este conecta con el rol DUEÑO, que se las salta todas —como el
// despliegue que entrega a los servicios el superusuario de arranque de
// PostgreSQL—, y exige lo mismo: lo de otra organización no existe. Lo único que
// puede sostenerlo aquí es el SQL del store.
package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/peticiones"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/syerp/db"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

// montarSinRLS da un store sobre una conexión que se salta la RLS.
func montarSinRLS(t *testing.T) *store.Store {
	t.Helper()
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool := h.PoolSinRLS(t)
	// La premisa: si este rol no se saltara la RLS, un verde aquí no diría nada
	// del SQL.
	var super, bypass bool
	if err := pool.QueryRow(ctx, `SELECT rolsuper, rolbypassrls FROM pg_roles WHERE rolname = current_user`).Scan(&super, &bypass); err != nil {
		t.Fatal(err)
	}
	if !super && !bypass {
		t.Fatal("el rol dueño del harness no se salta la RLS: este test no probaría el SQL")
	}
	return store.New(pool)
}

// TestSinRLSOtraOrganizacionNoExiste: acme tiene un contacto con su adjunto, una
// factura con la aprobación pedida, un lote importado, una conexión, permisos,
// un umbral, un cierre, una tarea y una petición de edición. Para globex nada de
// eso existe: ni lo lee, ni lo cambia, ni lo borra. Y al acabar, acme lo tiene
// todo como estaba.
func TestSinRLSOtraOrganizacionNoExiste(t *testing.T) {
	st := montarSinRLS(t)
	c := cat(t)
	contacto, factura := modelo(t, c, "contacto"), modelo(t, c, "factura_venta")
	acme, globex := tenantCtx("acme"), tenantCtx("globex")

	// --- Lo de acme -----------------------------------------------------------
	cliente := crear(acme, t, st, contacto, `{"nombre":"Cliente de Acme","tipo":"organizacion"}`)
	adjunto, _, err := st.GuardarAdjunto(acme, "contacto", cliente.ID, "contrato.pdf", "application/pdf", []byte("%PDF-1.7 de acme"), "ana")
	if err != nil {
		t.Fatalf("GuardarAdjunto: %v", err)
	}
	doc := crear(acme, t, st, factura, `{"cliente":"`+cliente.ID+`","fecha":"2026-09-12","base":200000,"iva":42000,"total":242000}`)
	solicitud, err := st.PedirAprobacion(acme, factura, doc.ID, 242000, "EUR", "ana")
	if err != nil {
		t.Fatalf("PedirAprobacion: %v", err)
	}
	fila := store.FilaImportada{
		Linea: 2, Etiqueta: "Proveedor de Acme", Externo: "ext-1",
		Data: map[string]any{"nombre": "Proveedor de Acme", "tipo": "organizacion", "externo_id": "ext-1"},
	}
	lote, err := st.Importar(acme, contacto, []store.FilaImportada{fila}, "contactos.csv", "ana")
	if err != nil || lote.Creados != 1 {
		t.Fatalf("Importar en acme: %+v %v", lote, err)
	}
	importado, err := st.BuscarPorExterno(acme, "contacto", "ext-1")
	if err != nil {
		t.Fatalf("BuscarPorExterno en acme: %v", err)
	}
	if err := st.GuardarConexion(acme, store.Conexion{Proveedor: "odoo", Instancia: "inst-acme", Config: map[string]any{}, Activo: true}, nil, "ana"); err != nil {
		t.Fatalf("GuardarConexion: %v", err)
	}
	if err := st.GuardarPermisos(acme, "bea", "bea@acme.test", "Bea", map[string]string{factura.Modulo: "lectura"}, nil, "ana"); err != nil {
		t.Fatalf("GuardarPermisos: %v", err)
	}
	if err := st.GuardarUmbral(acme, factura.Modulo, 100000, "EUR", "ana"); err != nil {
		t.Fatalf("GuardarUmbral: %v", err)
	}
	corte := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	if err := st.Cerrar(acme, corte, "cierre de junio", "ana"); err != nil {
		t.Fatalf("Cerrar: %v", err)
	}
	tarea, err := st.CrearActividad(acme, store.Actividad{Modelo: "contacto", Registro: cliente.ID, Tipo: "llamada", Resumen: "Llamar", Vence: "2026-10-01"}, "ana")
	if err != nil {
		t.Fatalf("CrearActividad: %v", err)
	}
	if _, _, err := st.PedirEdicion(acme, factura.Modulo, peticiones.Peticion{Sujeto: "bea", Nombre: "Bea", Correo: "bea@acme.test"}); err != nil {
		t.Fatalf("PedirEdicion: %v", err)
	}
	if _, err := st.Anotar(acme, "contacto", cliente.ID, "ana", "nota de acme"); err != nil {
		t.Fatalf("Anotar en acme: %v", err)
	}

	noExiste := func(que string, err error) {
		t.Helper()
		if !errors.Is(err, store.ErrNoEncontrado) {
			t.Errorf("%s desde otra organización: %v, esperaba que no existiera", que, err)
		}
	}
	vacio := func(que string, n int, err error) {
		t.Helper()
		if err != nil || n != 0 {
			t.Errorf("globex ve %d %s de acme (%v)", n, que, err)
		}
	}

	// --- Leer -----------------------------------------------------------------
	_, err = st.Obtener(globex, "contacto", cliente.ID)
	noExiste("Obtener un registro", err)
	_, _, err = st.AdjuntoConContenido(globex, adjunto.ID)
	noExiste("AdjuntoConContenido", err)
	_, err = st.BuscarPorExterno(globex, "contacto", "ext-1")
	noExiste("BuscarPorExterno", err)
	_, err = st.BuscarPorRef(globex, "factura_venta", doc.Ref)
	noExiste("BuscarPorRef", err)
	_, err = st.Actividad(globex, tarea.ID)
	noExiste("Actividad", err)
	aprobaciones, err := st.Aprobaciones(globex, "")
	vacio("aprobaciones", len(aprobaciones), err)
	lotes, err := st.Importaciones(globex, 50)
	vacio("lotes importados", len(lotes), err)
	conexiones, err := st.ListarConexiones(globex)
	vacio("conexiones", len(conexiones), err)
	if _, err := st.Conexion(globex, "odoo", nil); !errors.Is(err, store.ErrNoEncontrado) {
		t.Errorf("globex lee la conexión de acme: %v", err)
	}
	umbrales, err := st.Umbrales(globex)
	vacio("umbrales", len(umbrales), err)
	cierres, err := st.HistorialDeCierres(globex)
	vacio("cierres", len(cierres), err)
	if _, cerrado, err := st.CierreVigente(globex); err != nil || cerrado {
		t.Errorf("el cierre de acme cierra el libro de globex (cerrado=%v, %v)", cerrado, err)
	}
	pedidas, err := st.PeticionesEdicion(globex, factura.Modulo)
	vacio("peticiones de edición", len(pedidas), err)
	modulos, err := st.ModulosPedidos(globex, "bea")
	vacio("módulos pedidos", len(modulos), err)

	// --- Cambiar --------------------------------------------------------------
	// La clave natural de una importación es de la organización: el mismo
	// `externo_id` en globex CREA un contacto suyo, no pisa el de acme.
	fila.Etiqueta, fila.Data = "Pisado por globex", map[string]any{"nombre": "Pisado por globex", "tipo": "organizacion", "externo_id": "ext-1"}
	loteGlobex, err := st.Importar(globex, contacto, []store.FilaImportada{fila}, "contactos.csv", "zoe")
	if err != nil || loteGlobex.Creados != 1 || loteGlobex.Actualizados != 0 {
		t.Errorf("la importación de globex tenía que crear, no actualizar: %+v %v", loteGlobex, err)
	}
	_, err = st.Actualizar(globex, contacto, cliente.ID, map[string]any{"nombre": "Robado", "tipo": "organizacion"}, "Robado")
	noExiste("Actualizar", err)
	_, err = st.CambiarEstado(globex, modelo(t, cat(t), "factura_venta"), doc.ID, "anulado", "zoe")
	noExiste("CambiarEstado", err)
	_, err = st.ResolverAprobacion(globex, solicitud.ID, "aprobada", "", "zoe")
	noExiste("ResolverAprobacion", err)
	_, err = st.PedirAprobacion(globex, factura, doc.ID, 1, "EUR", "zoe")
	noExiste("PedirAprobacion", err)
	_, err = st.Revertir(globex, lote.ID, "zoe")
	noExiste("Revertir una importación", err)
	_, err = st.Anotar(globex, "contacto", cliente.ID, "zoe", "nota de globex")
	noExiste("Anotar", err)
	_, err = st.CerrarActividad(globex, tarea.ID, "cancelada")
	noExiste("CerrarActividad", err)
	if n, err := st.LoteEstado(globex, modelo(t, cat(t), "factura_venta"), []string{doc.ID}, "anulado", "zoe"); err != nil || n != 0 {
		t.Errorf("LoteEstado de globex toca %d facturas de acme (%v)", n, err)
	}
	// Lo que borra y vuelve a escribir «lo de una persona» o «lo de un módulo».
	if err := st.GuardarPermisos(globex, "bea", "", "", map[string]string{}, nil, "zoe"); err != nil {
		t.Fatalf("GuardarPermisos en globex: %v", err)
	}
	if err := st.GuardarUmbral(globex, factura.Modulo, -1, "EUR", "zoe"); err != nil {
		t.Fatalf("GuardarUmbral en globex: %v", err)
	}
	if err := st.ResolverPeticionEdicion(globex, factura.Modulo, "bea", false, "zoe"); err == nil {
		t.Error("globex rechaza una petición de edición de acme")
	}

	// --- Borrar ---------------------------------------------------------------
	noExiste("Borrar un registro", st.Borrar(globex, "contacto", cliente.ID))
	noExiste("BorrarAdjunto", st.BorrarAdjunto(globex, adjunto.ID))
	if n, err := st.LoteBorrar(globex, "contacto", []string{cliente.ID, importado.ID}); err != nil || n != 0 {
		t.Errorf("LoteBorrar de globex borra %d contactos de acme (%v)", n, err)
	}

	// --- Y acme lo tiene todo como estaba -------------------------------------
	if r, err := st.Obtener(acme, "contacto", cliente.ID); err != nil || r.Etiqueta != cliente.Etiqueta {
		t.Errorf("el contacto de acme: %+v %v", r, err)
	}
	if r, err := st.Obtener(acme, "contacto", importado.ID); err != nil || r.Etiqueta != "Proveedor de Acme" {
		t.Errorf("el contacto importado de acme: %+v %v", r, err)
	}
	if r, err := st.Obtener(acme, "factura_venta", doc.ID); err != nil || r.Estado != doc.Estado {
		t.Errorf("la factura de acme: %+v %v", r, err)
	}
	if _, contenido, err := st.AdjuntoConContenido(acme, adjunto.ID); err != nil || string(contenido) != "%PDF-1.7 de acme" {
		t.Errorf("el adjunto de acme: %q %v", contenido, err)
	}
	if as, err := st.Aprobaciones(acme, "pendiente"); err != nil || len(as) != 1 || as[0].ID != solicitud.ID {
		t.Errorf("la aprobación de acme tenía que seguir pendiente: %+v %v", as, err)
	}
	if ls, err := st.Importaciones(acme, 50); err != nil || len(ls) != 1 || ls[0].RevertidaEn != nil {
		t.Errorf("el lote de acme: %+v %v", ls, err)
	}
	if niveles, _, err := st.PermisosDe(acme, "bea"); err != nil || niveles[factura.Modulo] != "lectura" {
		t.Errorf("los permisos de bea en acme: %v %v", niveles, err)
	}
	if us, err := st.Umbrales(acme); err != nil || len(us) != 1 {
		t.Errorf("el umbral de acme: %+v %v", us, err)
	}
	if hasta, cerrado, err := st.CierreVigente(acme); err != nil || !cerrado || !hasta.Equal(corte) {
		t.Errorf("el cierre de acme: %v %v %v", hasta, cerrado, err)
	}
	if ps, err := st.PeticionesEdicion(acme, factura.Modulo); err != nil || len(ps) != 1 {
		t.Errorf("la petición de edición de acme: %+v %v", ps, err)
	}
	if a, err := st.Actividad(acme, tarea.ID); err != nil || a.Estado != "pendiente" {
		t.Errorf("la tarea de acme: %+v %v", a, err)
	}
	if hs, err := st.Historial(acme, "contacto", cliente.ID, 50); err != nil {
		t.Errorf("Historial de acme: %v", err)
	} else {
		for _, h := range hs {
			if h.Texto == "nota de globex" {
				t.Error("la nota de globex está en el historial del contacto de acme")
			}
		}
	}
}
