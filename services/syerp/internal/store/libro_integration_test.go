// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/syerp/db"
	"git.sytue.eu/sytue/one/services/syerp/internal/catalogo"
	"git.sytue.eu/sytue/one/services/syerp/internal/domain"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

// LO QUE SE PRUEBA AQUÍ Y NO EN LOS UNITARIOS: las garantías que da POSTGRES y
// no el código de Go. El cuadre de un asiento, la inmutabilidad de lo asentado,
// el «un documento, un asiento» y el aislamiento entre organizaciones son
// triggers, restricciones e índices — y un test que llame a las funciones del
// store no demuestra ninguno, porque el store es justamente el que podría dejar
// de llamarlos.
//
// Por eso varias de estas pruebas escriben SQL a pelo: simulan el código
// equivocado que algún día se escribirá, y comprueban que la base se planta.

func tenantCtx(slug string) context.Context {
	return tenancy.Into(context.Background(), tenancy.Tenant{Slug: slug})
}

func cat(t *testing.T) *catalogo.Catalogo {
	t.Helper()
	c, err := catalogo.Cargar()
	if err != nil {
		t.Fatalf("cargar catálogo: %v", err)
	}
	return c
}

func modelo(t *testing.T, c *catalogo.Catalogo, clave string) *catalogo.Modelo {
	t.Helper()
	m, ok := c.Modelo(clave)
	if !ok {
		t.Fatalf("el catálogo no trae el modelo %q", clave)
	}
	return m
}

// crear valida y guarda un registro como lo haría el handler.
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

func TestMigracionesYRLS(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	// Sin excepciones: TODAS las tablas de SyERP llevan datos de organización.
	// Una tabla nueva sin RLS forzada falla aquí el día que se crea.
	pgtest.ExigirRLSForzada(t, h, nil)
}

// TestAislamientoEntreOrganizaciones: lo que ve una organización de la otra es
// nada. Es el invariante del que cuelga todo lo demás.
func TestAislamientoEntreOrganizaciones(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	c := cat(t)
	contacto := modelo(t, c, "contacto")

	acme := tenantCtx("acme")
	otra := tenantCtx("globex")

	crear(acme, t, st, contacto, `{"nombre":"Cliente de Acme","tipo":"organizacion"}`)

	pag, err := st.Listar(otra, store.Filtro{Modelo: "contacto"})
	if err != nil {
		t.Fatalf("Listar: %v", err)
	}
	if len(pag.Registros) != 0 {
		t.Fatalf("globex ve %d contactos de acme", len(pag.Registros))
	}
}

// TestAsientoDescuadradoLoRechazaLaBase escribe el asiento a mano y descuadrado,
// que es exactamente lo que haría un cálculo mal hecho. El trigger es DEFERRABLE,
// así que el fallo llega AL CONFIRMAR y no en el INSERT: si saltara antes, sería
// imposible escribir el primer apunte de un asiento de dos.
func TestAsientoDescuadradoLoRechazaLaBase(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := tenantCtx("acme")

	err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var moveID string
		if err := tx.QueryRow(ctx, `
			INSERT INTO moves (tenant, diario, ref, fecha, concepto, owner_sub, estado)
			VALUES (current_setting('app.tenant', true), 'ventas', 'X-1', '2026-09-12', 'prueba', 'ada', 'borrador')
			RETURNING id::text`).Scan(&moveID); err != nil {
			return err
		}
		// 1.000,00 al debe y 900,00 al haber: cien euros que no están.
		for _, l := range []struct {
			cuenta      string
			debe, haber int64
		}{{"430000", 100000, 0}, {"700000", 0, 90000}} {
			if _, err := tx.Exec(ctx, `
				INSERT INTO move_lines (move_id, tenant, cuenta, debe, haber)
				VALUES ($1::uuid, current_setting('app.tenant', true), $2, $3, $4)`,
				moveID, l.cuenta, l.debe, l.haber); err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil {
		t.Fatal("la base aceptó un asiento descuadrado")
	}
	if !strings.Contains(err.Error(), "descuadrado") {
		t.Fatalf("error inesperado: %v", err)
	}
}

// TestAsientoSinApuntesTampocoVale: un asiento vacío cuadra (0 = 0) y no es un
// asiento. Sin esta guarda, el «cuadre» dejaría pasar el caso más tonto.
func TestAsientoSinApuntesTampocoVale(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := tenantCtx("acme")

	err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var moveID string
		if err := tx.QueryRow(ctx, `
			INSERT INTO moves (tenant, diario, ref, fecha, concepto, owner_sub, estado)
			VALUES (current_setting('app.tenant', true), 'ventas', 'X-2', '2026-09-12', 'vacío', 'ada', 'borrador')
			RETURNING id::text`).Scan(&moveID); err != nil {
			return err
		}
		// Un apunte y su retirada en la misma transacción: al confirmar no queda
		// ninguno, que es el caso que el trigger tiene que ver.
		var lineID int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO move_lines (move_id, tenant, cuenta, debe, haber)
			VALUES ($1::uuid, current_setting('app.tenant', true), '430000', 100, 0)
			RETURNING id`, moveID).Scan(&lineID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `DELETE FROM move_lines WHERE id = $1`, lineID)
		return err
	})
	if err == nil {
		t.Fatal("la base aceptó un asiento sin apuntes")
	}
	// Atado al mensaje del trigger: si no, este test pasaría igual el día que
	// falle por cualquier otro motivo —una columna renombrada, la RLS— y daría
	// por vigilado un invariante que ya no vigila nadie.
	if !strings.Contains(err.Error(), "sin apuntes") {
		t.Fatalf("falló por otro motivo: %v", err)
	}
}

// TestApunteAUnSoloLado: un apunte con debe y haber a la vez no es partida
// doble, es una hoja de cálculo.
func TestApunteAUnSoloLado(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := tenantCtx("acme")

	err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var moveID string
		if err := tx.QueryRow(ctx, `
			INSERT INTO moves (tenant, diario, ref, fecha, concepto, owner_sub, estado)
			VALUES (current_setting('app.tenant', true), 'ventas', 'X-3', '2026-09-12', 'x', 'ada', 'borrador')
			RETURNING id::text`).Scan(&moveID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO move_lines (move_id, tenant, cuenta, debe, haber)
			VALUES ($1::uuid, current_setting('app.tenant', true), '430000', 100, 100)`, moveID)
		return err
	})
	if err == nil {
		t.Fatal("la base aceptó un apunte con debe y haber a la vez")
	}
	if !strings.Contains(err.Error(), "move_lines_un_solo_lado") {
		t.Fatalf("falló por otro motivo que la restricción: %v", err)
	}
}

// --- El camino completo: factura → asiento --------------------------------

// facturaDePrueba deja un cliente y una factura de 1.000 + 21 % listos.
func facturaDePrueba(ctx context.Context, t *testing.T, st *store.Store, c *catalogo.Catalogo) (*catalogo.Modelo, store.Registro) {
	t.Helper()
	contacto := modelo(t, c, "contacto")
	factura := modelo(t, c, "factura_venta")

	cliente := crear(ctx, t, st, contacto, `{"nombre":"Suministros Acme S.L.","tipo":"organizacion"}`)
	cuerpo := fmt.Sprintf(
		`{"cliente":%q,"fecha":"2026-09-12","base":100000,"iva":21000,"total":121000}`, cliente.ID)
	return factura, crear(ctx, t, st, factura, cuerpo)
}

func TestFacturaGeneraSuAsientoYCuadra(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	c := cat(t)

	factura, fv := facturaDePrueba(ctx, t, st, c)
	if fv.Ref != "FV-2026-00001" {
		t.Fatalf("ref = %q, se esperaba FV-2026-00001", fv.Ref)
	}

	asiento, err := st.Asentar(ctx, factura, fv.ID, "ada")
	if err != nil {
		t.Fatalf("Asentar: %v", err)
	}
	if asiento.Ref != "VENTAS-2026-00001" {
		t.Errorf("ref del asiento = %q", asiento.Ref)
	}
	if len(asiento.Apuntes) != 3 {
		t.Fatalf("apuntes = %d", len(asiento.Apuntes))
	}

	// El documento queda confirmado, no en borrador.
	reg, err := st.Obtener(ctx, "factura_venta", fv.ID)
	if err != nil {
		t.Fatalf("Obtener: %v", err)
	}
	if reg.Estado != "confirmado" {
		t.Errorf("estado = %q, se esperaba confirmado", reg.Estado)
	}

	// Y el balance cuadra y cuenta lo que debe.
	saldos, err := st.Balance(ctx, time.Time{})
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	var debe, haber int64
	porCuenta := map[string]int64{}
	for _, s := range saldos {
		debe += s.Debe
		haber += s.Haber
		porCuenta[s.Cuenta] = s.Saldo
	}
	if debe != haber || debe != 121000 {
		t.Fatalf("balance: debe=%d haber=%d", debe, haber)
	}
	if porCuenta["430000"] != 121000 {
		t.Errorf("el cliente debe %d, se esperaban 121000", porCuenta["430000"])
	}
	if porCuenta["700000"] != -100000 || porCuenta["477000"] != -21000 {
		t.Errorf("ingreso/IVA: %+v", porCuenta)
	}
}

// TestUnAsientoPorDocumento es la restricción que impide duplicar ingresos. Se
// prueba en serie (el store lo ve) y EN PARALELO (solo lo ve el índice único).
func TestUnAsientoPorDocumento(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	c := cat(t)

	factura, fv := facturaDePrueba(ctx, t, st, c)
	if _, err := st.Asentar(ctx, factura, fv.ID, "ada"); err != nil {
		t.Fatalf("primer asiento: %v", err)
	}
	if _, err := st.Asentar(ctx, factura, fv.ID, "ada"); !errors.Is(err, store.ErrYaAsentado) {
		t.Fatalf("segundo asiento: err = %v, se esperaba ErrYaAsentado", err)
	}

	// Y la carrera: dos peticiones a la vez sobre OTRA factura. El FOR UPDATE del
	// store las serializa y el índice único es la red de abajo; lo que no puede
	// pasar es que salgan dos asientos.
	_, otra := facturaDePrueba(ctx, t, st, c)
	var wg sync.WaitGroup
	exitos := make(chan struct{}, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := st.Asentar(ctx, factura, otra.ID, "ada"); err == nil {
				exitos <- struct{}{}
			}
		}()
	}
	wg.Wait()
	close(exitos)
	if n := len(exitos); n != 1 {
		t.Fatalf("%d asientos para el mismo documento; debía ser 1", n)
	}
}

// TestAsientoAsentadoEsInmutable: corregir contabilidad se hace con otro
// asiento. Es lo que separa un libro de una hoja de cálculo, y es lo que
// pregunta cualquier auditoría.
func TestAsientoAsentadoEsInmutable(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	c := cat(t)

	factura, fv := facturaDePrueba(ctx, t, st, c)
	asiento, err := st.Asentar(ctx, factura, fv.ID, "ada")
	if err != nil {
		t.Fatalf("Asentar: %v", err)
	}

	casos := map[string]string{
		"cambiar la fecha":  `UPDATE moves SET fecha = '2025-01-01' WHERE id = $1::uuid`,
		"cambiar el diario": `UPDATE moves SET diario = 'compras' WHERE id = $1::uuid`,
		"borrar el asiento": `DELETE FROM moves WHERE id = $1::uuid`,
		"tocar un importe":  `UPDATE move_lines SET debe = 1 WHERE move_id = $1::uuid`,
		"borrar un apunte":  `DELETE FROM move_lines WHERE move_id = $1::uuid`,
		"colar otro apunte": `INSERT INTO move_lines (move_id, tenant, cuenta, debe, haber) VALUES ($1::uuid, current_setting('app.tenant', true), '999999', 1, 0)`,
	}

	// ESTE es el caso que se colaba, y por eso va aparte: un PAR EQUILIBRADO
	// dentro de un asiento ya asentado. El de arriba —una línea suelta— lo paraba
	// el cuadre de casualidad, así que el trigger podía no cubrir INSERT y nadie
	// se enteraba. Dinero nuevo dentro de un asiento cerrado, con su fecha y su
	// número ya emitidos, es la peor forma de tocar un libro.
	casos["colar un par equilibrado"] = `
		INSERT INTO move_lines (move_id, tenant, cuenta, debe, haber) VALUES
			($1::uuid, current_setting('app.tenant', true), '999999', 5000, 0),
			($1::uuid, current_setting('app.tenant', true), '888888', 0, 5000)`
	for nombre, sql := range casos {
		t.Run(nombre, func(t *testing.T) {
			err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
				_, err := tx.Exec(ctx, sql, asiento.ID)
				return err
			})
			if err == nil {
				t.Fatalf("la base permitió %s sobre un asiento asentado", nombre)
			}
			// Que falle NO basta: tiene que fallar por el trigger de
			// inmutabilidad y no por la RLS, un nombre de columna o cualquier
			// otra cosa que un día cambie y deje este test verde sin vigilar nada.
			if !strings.Contains(err.Error(), "ya asentado") {
				t.Fatalf("%s falló por otro motivo: %v", nombre, err)
			}
		})
	}

	// La ÚNICA transición admitida: anularlo, que deja la fila y sus apuntes.
	if err := st.Anular(ctx, asiento.ID, "error de facturación"); err != nil {
		t.Fatalf("Anular: %v", err)
	}
	saldos, err := st.Balance(ctx, time.Time{})
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if len(saldos) != 0 {
		t.Fatalf("un asiento anulado sigue contando en el balance: %+v", saldos)
	}
}

// TestSecuenciaNoRepiteNumero: dos documentos no pueden compartir referencia, ni
// aunque se creen a la vez. El bloqueo de fila de `sequences` es lo que lo
// garantiza, y el índice único de `records` la red de abajo.
func TestSecuenciaNoRepiteNumero(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	c := cat(t)
	pedido := modelo(t, c, "pedido_venta")
	contacto := modelo(t, c, "contacto")

	cliente := crear(ctx, t, st, contacto, `{"nombre":"Cliente","tipo":"organizacion"}`)
	cuerpo := fmt.Sprintf(`{"cliente":%q,"fecha":"2026-09-12","base":1000}`, cliente.ID)

	const n = 12
	refs := make(chan string, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			data, etiqueta, err := domain.Validar(pedido, json.RawMessage(cuerpo))
			if err != nil {
				return
			}
			if reg, err := st.Crear(ctx, pedido, data, etiqueta, "ada"); err == nil {
				refs <- reg.Ref
			}
		}()
	}
	wg.Wait()
	close(refs)

	vistas := map[string]bool{}
	for r := range refs {
		if vistas[r] {
			t.Fatalf("referencia repetida: %s", r)
		}
		vistas[r] = true
	}
	if len(vistas) != n {
		t.Fatalf("se crearon %d pedidos de %d", len(vistas), n)
	}
}

// TestRelacionInexistente: guardar un pedido que apunta a un cliente que no
// existe deja un documento que no se puede leer entero. Se comprueba DENTRO de
// la transacción, que es lo único que lo hace cierto.
func TestRelacionInexistente(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	c := cat(t)
	pedido := modelo(t, c, "pedido_venta")

	cuerpo := `{"cliente":"3f6c0f6e-0000-4000-8000-000000000001","fecha":"2026-09-12","base":1000}`
	data, etiqueta, err := domain.Validar(pedido, json.RawMessage(cuerpo))
	if err != nil {
		t.Fatalf("validar: %v", err)
	}
	if _, err := st.Crear(ctx, pedido, data, etiqueta, "ada"); !errors.Is(err, domain.ErrDatos) {
		t.Fatalf("err = %v, se esperaba ErrDatos", err)
	}
}

// TestRelacionDeOtroModelo: un pedido cuyo «cliente» es en realidad un producto.
// El identificador existe y es válido, así que solo lo caza comprobar el MODELO.
func TestRelacionDeOtroModelo(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	c := cat(t)

	producto := crear(ctx, t, st, modelo(t, c, "producto"),
		`{"nombre":"Silla","tipo":"bien","precio":9900}`)

	pedido := modelo(t, c, "pedido_venta")
	cuerpo := fmt.Sprintf(`{"cliente":%q,"fecha":"2026-09-12","base":1000}`, producto.ID)
	data, etiqueta, err := domain.Validar(pedido, json.RawMessage(cuerpo))
	if err != nil {
		t.Fatalf("validar: %v", err)
	}
	if _, err := st.Crear(ctx, pedido, data, etiqueta, "ada"); !errors.Is(err, domain.ErrDatos) {
		t.Fatalf("err = %v, se esperaba ErrDatos", err)
	}
}
