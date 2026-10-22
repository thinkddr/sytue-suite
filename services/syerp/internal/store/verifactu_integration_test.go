// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/syerp/db"
	"git.sytue.eu/sytue/one/services/syerp/internal/catalogo"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
	"git.sytue.eu/sytue/one/services/syerp/internal/verifactu"
)

// organizacionVerifactu deja la organización con perfil español y, si se pide,
// VERI*FACTU activado.
func organizacionVerifactu(ctx context.Context, t *testing.T, st *store.Store, activar bool) {
	t.Helper()
	if _, err := st.GuardarOrganizacion(ctx, store.Organizacion{
		Nombre: "Panadería Ejemplo, S.L.", NIF: "B-8989000-1", Direccion: "Calle Mayor, 1", CP: "28001",
		Ciudad: "Madrid", Pais: "ES", Forma: "sl",
	}, "ada"); err != nil {
		t.Fatal(err)
	}
	if activar {
		if v, err := st.ActivarVerifactu(ctx); err != nil || !v.Activo {
			t.Fatalf("activar: %+v %v", v, err)
		}
	}
}

func facturaVF(ctx context.Context, t *testing.T, st *store.Store, c *catalogo.Catalogo, extra string) store.Registro {
	t.Helper()
	cliente := crear(ctx, t, st, modelo(t, c, "contacto"), `{"nombre":"Cafetería Cliente S.L.","tipo":"organizacion","nif":"ES B12345674","pais":"ES"}`)
	return crear(ctx, t, st, modelo(t, c, "factura_venta"),
		fmt.Sprintf(`{"cliente":%q,"fecha":"2026-09-12","base":100000,"iva":21000,"total":121000%s}`, cliente.ID, extra))
}

func xmlDeRegistros(ctx context.Context, t *testing.T, st *store.Store) []string {
	t.Helper()
	var out []string
	if err := st.ExportarVerifactu(ctx, func(x string) error { out = append(out, x); return nil }); err != nil {
		t.Fatal(err)
	}
	return out
}

// Emitir deja el alta encadenada en la misma transacción; la base no deja tocar
// ni la factura emitida ni sus registros; anular deja su registro de anulación.
func TestVerifactuEmiteEncadenaYProtege(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	st.Version = "v-test"
	ctx := tenantCtx("acme")
	c := cat(t)
	factura := modelo(t, c, "factura_venta")

	// Sin VERI*FACTU, confirmar no registra nada.
	organizacionVerifactu(ctx, t, st, false)
	antes := facturaVF(ctx, t, st, c, "")
	if _, err := st.CambiarEstado(ctx, modelo(t, cat(t), "factura_venta"), antes.ID, "confirmado", "ada"); err != nil {
		t.Fatal(err)
	}
	if r, _ := st.RegistrosVerifactu(ctx, antes.ID); len(r) != 0 {
		t.Fatalf("sin VERI*FACTU no hay registros: %+v", r)
	}
	if v, _ := st.ActivarVerifactu(ctx); !v.Activo {
		t.Fatal("no se activó")
	}

	uno := facturaVF(ctx, t, st, c, `,"notas":"Pan de septiembre"`)
	dos := facturaVF(ctx, t, st, c, "")
	// La importada entra por donde entran las importadas: `externo_id` ya no se
	// escribe en un alta normal (NEG-14).
	sinMarca := facturaVF(ctx, t, st, c, "")
	datosImportada := map[string]any{"cliente": sinMarca.Data["cliente"], "fecha": "2026-09-12", "base": int64(100000), "iva": int64(21000), "total": int64(121000), "externo_id": "holded:1"}
	if _, err := st.UpsertPorExterno(ctx, factura, "holded:1", datosImportada, "", "borrador", "ada"); err != nil {
		t.Fatal(err)
	}
	var importada store.Registro
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id::text FROM records WHERE model = 'factura_venta' AND data->>'externo_id' = 'holded:1'`).Scan(&importada.ID)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CambiarEstado(ctx, modelo(t, cat(t), "factura_venta"), uno.ID, "confirmado", "ada"); err != nil {
		t.Fatal(err)
	}
	if n, err := st.LoteEstado(ctx, modelo(t, cat(t), "factura_venta"), []string{dos.ID, importada.ID}, "confirmado", "ada"); err != nil || n != 2 {
		t.Fatalf("lote: %d %v", n, err)
	}
	r1, _ := st.RegistrosVerifactu(ctx, uno.ID)
	r2, _ := st.RegistrosVerifactu(ctx, dos.ID)
	if ri, _ := st.RegistrosVerifactu(ctx, importada.ID); len(ri) != 0 {
		t.Fatalf("una factura de otro sistema no se registra aquí: %+v", ri)
	}
	if len(r1) != 1 || len(r2) != 1 || r1[0].Tipo != "alta" || r1[0].Estado != "pendiente" || r1[0].NIF != "B89890001" {
		t.Fatalf("altas: %+v %+v", r1, r2)
	}
	docs := xmlDeRegistros(ctx, t, st)
	if len(docs) != 2 || !strings.Contains(docs[0], "<sum1:PrimerRegistro>S</sum1:PrimerRegistro>") ||
		!strings.Contains(docs[1], "<sum1:Huella>"+r1[0].Huella+"</sum1:Huella></sum1:RegistroAnterior>") ||
		!strings.Contains(docs[0], "<sum1:DescripcionOperacion>Pan de septiembre</sum1:DescripcionOperacion>") ||
		!strings.Contains(docs[0], "<sum1:NIF>B12345674</sum1:NIF>") || !strings.Contains(docs[0], "<sum1:Version>v-test</sum1:Version>") {
		t.Fatalf("cadena:\n%s", strings.Join(docs, "\n"))
	}

	// La factura emitida no se toca: ni importes, ni papelera, ni vuelta a borrador.
	datos := map[string]any{"cliente": uno.Data["cliente"], "fecha": "2026-09-12", "base": int64(90000), "iva": int64(18900), "total": int64(108900)}
	if _, err := st.Actualizar(ctx, factura, uno.ID, datos, ""); !errors.Is(err, store.ErrFacturaEmitida) {
		t.Fatalf("cambiar el importe: %v", err)
	}
	if err := st.Borrar(ctx, "factura_venta", uno.ID); !errors.Is(err, store.ErrFacturaEmitida) {
		t.Fatalf("a la papelera: %v", err)
	}
	if _, err := st.CambiarEstado(ctx, modelo(t, cat(t), "factura_venta"), uno.ID, "borrador", "ada"); !errors.Is(err, store.ErrFacturaEmitida) {
		t.Fatalf("vuelta a borrador: %v", err)
	}
	// Las notas y el vencimiento, sí.
	datos = map[string]any{"cliente": uno.Data["cliente"], "fecha": "2026-09-12", "base": int64(100000), "iva": int64(21000), "total": int64(121000), "vencimiento": "2026-10-12"}
	if _, err := st.Actualizar(ctx, factura, uno.ID, datos, ""); err != nil {
		t.Fatalf("el vencimiento se puede cambiar: %v", err)
	}
	// Los registros tampoco, ni borrarlos.
	err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE verifactu_registros SET huella = 'X'`)
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "no se modifica") {
		t.Fatalf("un registro no se modifica: %v", err)
	}
	err = h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `DELETE FROM verifactu_registros`)
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "no se borra") {
		t.Fatalf("un registro no se borra: %v", err)
	}
	// Ni el NIF de la organización (escrito de otra forma, sí).
	org := store.Organizacion{Nombre: "Panadería Ejemplo, S.L.", NIF: "B12345674", CP: "28001", Pais: "ES", Forma: "sl"}
	if _, err := st.GuardarOrganizacion(ctx, org, "ada"); !errors.Is(err, store.ErrNIFConVerifactu) {
		t.Fatalf("cambiar el NIF: %v", err)
	}
	org.NIF = "b89890001"
	if _, err := st.GuardarOrganizacion(ctx, org, "ada"); err != nil {
		t.Fatalf("el mismo NIF escrito distinto: %v", err)
	}

	// Anular deja la anulación encadenada; el alta sigue pendiente, así que la AEAT
	// la tendrá antes: no es «sin registro previo».
	if _, err := st.CambiarEstado(ctx, modelo(t, cat(t), "factura_venta"), dos.ID, "anulado", "ada"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CambiarEstado(ctx, modelo(t, cat(t), "factura_venta"), dos.ID, "confirmado", "ada"); !errors.Is(err, store.ErrFacturaEmitida) {
		t.Fatalf("una factura anulada no revive: %v", err)
	}
	docs = xmlDeRegistros(ctx, t, st)
	r2, _ = st.RegistrosVerifactu(ctx, dos.ID)
	if len(docs) != 3 || len(r2) != 2 || r2[1].Tipo != "anulacion" ||
		!strings.Contains(docs[2], "<sum1:NumSerieFacturaAnulada>"+dos.Ref+"</sum1:NumSerieFacturaAnulada>") ||
		strings.Contains(docs[2], "SinRegistroPrevio") {
		t.Fatalf("anulación:\n%s", docs[2])
	}

	// Una rectificativa de la primera, por diferencias.
	rect := facturaVF(ctx, t, st, c, fmt.Sprintf(`,"rectifica":%q`, uno.ID))
	datos = map[string]any{"cliente": rect.Data["cliente"], "fecha": "2026-09-12", "base": int64(-10000), "iva": int64(-2100), "total": int64(-12100), "rectifica": uno.ID}
	if _, err := st.Actualizar(ctx, factura, rect.ID, datos, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CambiarEstado(ctx, modelo(t, cat(t), "factura_venta"), rect.ID, "confirmado", "ada"); err != nil {
		t.Fatal(err)
	}
	docs = xmlDeRegistros(ctx, t, st)
	if !strings.Contains(docs[3], "<sum1:TipoFactura>R1</sum1:TipoFactura><sum1:TipoRectificativa>I</sum1:TipoRectificativa><sum1:FacturasRectificadas><sum1:IDFacturaRectificada><sum1:IDEmisorFactura>B89890001</sum1:IDEmisorFactura><sum1:NumSerieFactura>"+uno.Ref+"</sum1:NumSerieFactura>") ||
		!strings.Contains(docs[3], "<sum1:ImporteTotal>-121.00</sum1:ImporteTotal>") {
		t.Fatalf("rectificativa:\n%s", docs[3])
	}

	// Un cliente con NIF-IVA alemán y sin país sale como de Alemania.
	aleman := crear(ctx, t, st, modelo(t, c, "contacto"), `{"nombre":"Kunde GmbH","tipo":"organizacion","nif":"DE 123456789"}`)
	ue := crear(ctx, t, st, factura, fmt.Sprintf(`{"cliente":%q,"fecha":"2026-09-12","base":1000,"regimen_iva":"iva_no_sujeta"}`, aleman.ID))
	if _, err := st.CambiarEstado(ctx, modelo(t, cat(t), "factura_venta"), ue.ID, "confirmado", "ada"); err != nil {
		t.Fatal(err)
	}
	if docs = xmlDeRegistros(ctx, t, st); !strings.Contains(docs[len(docs)-1], "<sum1:IDOtro><sum1:CodigoPais>DE</sum1:CodigoPais><sum1:IDType>02</sum1:IDType><sum1:ID>DE123456789</sum1:ID></sum1:IDOtro>") ||
		!strings.Contains(docs[len(docs)-1], "<sum1:CalificacionOperacion>N2</sum1:CalificacionOperacion><sum1:BaseImponibleOimporteNoSujeto>10.00</sum1:BaseImponibleOimporteNoSujeto></sum1:DetalleDesglose>") {
		t.Fatalf("cliente de la UE:\n%s", docs[len(docs)-1])
	}

	// Sin NIF del cliente no se emite, y la factura sigue en borrador.
	sinNIF := crear(ctx, t, st, modelo(t, c, "contacto"), `{"nombre":"Cliente sin NIF","tipo":"persona"}`)
	mala := crear(ctx, t, st, factura, fmt.Sprintf(`{"cliente":%q,"fecha":"2026-09-12","base":1000,"iva":210,"total":1210}`, sinNIF.ID))
	if _, err := st.CambiarEstado(ctx, modelo(t, cat(t), "factura_venta"), mala.ID, "confirmado", "ada"); !errors.Is(err, verifactu.ErrDatos) {
		t.Fatalf("sin NIF: %v", err)
	}
	if reg, _ := st.Obtener(ctx, "factura_venta", mala.ID); reg.Estado != "borrador" {
		t.Fatalf("la factura que no se pudo registrar sigue en borrador: %s", reg.Estado)
	}
	// Sin IVA hay que decir por qué; exenta, sale como E1.
	exenta := facturaVF(ctx, t, st, c, "")
	datos = map[string]any{"cliente": exenta.Data["cliente"], "fecha": "2026-09-12", "base": int64(5000), "iva": int64(0), "total": int64(5000)}
	if _, err := st.Actualizar(ctx, factura, exenta.ID, datos, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CambiarEstado(ctx, modelo(t, cat(t), "factura_venta"), exenta.ID, "confirmado", "ada"); !errors.Is(err, verifactu.ErrDatos) {
		t.Fatalf("sin IVA ni régimen: %v", err)
	}
	datos["regimen_iva"] = "iva_exenta"
	if _, err := st.Actualizar(ctx, factura, exenta.ID, datos, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CambiarEstado(ctx, modelo(t, cat(t), "factura_venta"), exenta.ID, "confirmado", "ada"); err != nil {
		t.Fatal(err)
	}
	if docs = xmlDeRegistros(ctx, t, st); !strings.Contains(docs[len(docs)-1], "<sum1:OperacionExenta>E1</sum1:OperacionExenta>") {
		t.Fatalf("exenta:\n%s", docs[len(docs)-1])
	}
}

// El envío: se reclama un lote por turno, se anota lo que dice la AEAT, lo
// rechazado se convierte en una tarea y se corrige con un alta por rechazo.
func TestVerifactuEnvioRespuestaYSubsanacion(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	st.Version = "v-test"
	ctx := tenantCtx("acme")
	c := cat(t)
	organizacionVerifactu(ctx, t, st, true)

	uno := facturaVF(ctx, t, st, c, "")
	dos := facturaVF(ctx, t, st, c, "")
	if _, err := st.LoteEstado(ctx, modelo(t, cat(t), "factura_venta"), []string{uno.ID, dos.ID}, "confirmado", "ada"); err != nil {
		t.Fatal(err)
	}
	if tenants, err := st.TenantsConVerifactuPendiente(context.Background()); err != nil || len(tenants) != 1 || tenants[0] != "acme" {
		t.Fatalf("pendientes: %v %v", tenants, err)
	}
	lote, err := st.ReclamarLoteVerifactu(ctx)
	if err != nil || len(lote.IDs) != 2 || lote.Obligado.NIF != "B89890001" || lote.Obligado.Nombre == "" {
		t.Fatalf("lote: %+v %v", lote, err)
	}
	if otro, err := st.ReclamarLoteVerifactu(ctx); err != nil || len(otro.IDs) != 0 {
		t.Fatalf("mientras se espera la respuesta no se reclama otro lote: %+v %v", otro, err)
	}

	resp, err := verifactu.LeerRespuesta([]byte(fmt.Sprintf(`<env:Envelope xmlns:env="http://schemas.xmlsoap.org/soap/envelope/"><env:Body>
<r:RespuestaRegFactuSistemaFacturacion xmlns:r="urn:r"><r:CSV>CSV1</r:CSV><r:TiempoEsperaEnvio>60</r:TiempoEsperaEnvio><r:EstadoEnvio>ParcialmenteCorrecto</r:EstadoEnvio>
<r:RespuestaLinea><r:RefExterna>%s</r:RefExterna><r:EstadoRegistro>Correcto</r:EstadoRegistro></r:RespuestaLinea>
<r:RespuestaLinea><r:RefExterna>%s</r:RefExterna><r:EstadoRegistro>Incorrecto</r:EstadoRegistro><r:CodigoErrorRegistro>1105</r:CodigoErrorRegistro><r:DescripcionErrorRegistro>El NIF del destinatario no está identificado.</r:DescripcionErrorRegistro></r:RespuestaLinea>
</r:RespuestaRegFactuSistemaFacturacion></env:Body></env:Envelope>`, lote.IDs[0], lote.IDs[1])), 200)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.GuardarRespuestaVerifactu(ctx, lote, resp, nil); err != nil {
		t.Fatal(err)
	}
	r1, _ := st.RegistrosVerifactu(ctx, uno.ID)
	r2, _ := st.RegistrosVerifactu(ctx, dos.ID)
	if r1[0].Estado != "correcto" || r1[0].CSV != "CSV1" || r2[0].Estado != "incorrecto" || r2[0].Codigo == nil || *r2[0].Codigo != 1105 {
		t.Fatalf("respuesta: %+v %+v", r1, r2)
	}
	tareas, err := st.Actividades(ctx, store.FiltroActividades{Estado: "pendiente", Modelo: "factura_venta", Registro: dos.ID})
	if err != nil || len(tareas) != 1 || !strings.Contains(tareas[0].Resumen, dos.Ref) {
		t.Fatalf("la factura rechazada deja una tarea: %+v %v", tareas, err)
	}
	if v, _ := st.EstadoVerifactu(ctx); v.Rechazadas != 1 || v.Pendientes != 0 {
		t.Fatalf("estado: %+v", v)
	}
	if err := st.SubsanarVerifactu(ctx, uno.ID, "ada"); !errors.Is(err, store.ErrNadaQueSubsanar) {
		t.Fatalf("una factura correcta no se subsana: %v", err)
	}

	// Rechazada, la factura se puede corregir y se vuelve a enviar como alta por rechazo.
	datos := map[string]any{"cliente": dos.Data["cliente"], "fecha": "2026-09-13", "base": int64(100000), "iva": int64(21000), "total": int64(121000)}
	if _, err := st.Actualizar(ctx, modelo(t, c, "factura_venta"), dos.ID, datos, ""); err != nil {
		t.Fatalf("una factura rechazada se puede corregir: %v", err)
	}
	if err := st.SubsanarVerifactu(ctx, dos.ID, "ada"); err != nil {
		t.Fatal(err)
	}
	if err := st.SubsanarVerifactu(ctx, dos.ID, "ada"); !errors.Is(err, store.ErrVerifactuPendiente) {
		t.Fatalf("mientras la AEAT no conteste no se vuelve a subsanar: %v", err)
	}
	docs := xmlDeRegistros(ctx, t, st)
	if len(docs) != 3 || !strings.Contains(docs[2], "<sum1:Subsanacion>S</sum1:Subsanacion><sum1:RechazoPrevio>X</sum1:RechazoPrevio>") ||
		!strings.Contains(docs[2], "<sum1:FechaExpedicionFactura>13-09-2026</sum1:FechaExpedicionFactura>") {
		t.Fatalf("alta por rechazo:\n%s", docs[2])
	}

	// Un envío que falla deja lo pendiente con el motivo y aplaza el siguiente.
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE organizacion SET verifactu_proximo_envio = now()`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	lote, err = st.ReclamarLoteVerifactu(ctx)
	if err != nil || len(lote.IDs) != 1 {
		t.Fatalf("lote de la subsanación: %+v %v", lote, err)
	}
	if err := st.GuardarRespuestaVerifactu(ctx, lote, verifactu.Respuesta{}, &verifactu.ErrorEnvio{Detalle: "Codigo[4104]. El NIF del titular no está identificado."}); err != nil {
		t.Fatal(err)
	}
	if v, _ := st.EstadoVerifactu(ctx); v.Pendientes != 1 || !strings.Contains(v.ErrorEnvio, "4104") {
		t.Fatalf("envío fallido: %+v", v)
	}
	if otro, _ := st.ReclamarLoteVerifactu(ctx); len(otro.IDs) != 0 {
		t.Fatal("tras un fallo se espera antes de volver a enviar")
	}
}

// TestUnEnvioLlenoQueFallaEspera (NEG-21): con mil registros pendientes el envío
// no espera al plazo de la AEAT, pero si falla, el siguiente sí espera.
func TestUnEnvioLlenoQueFallaEspera(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	organizacionVerifactu(ctx, t, st, true)
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO verifactu_registros (id, tenant, instalacion, factura, tipo, nif, num_serie, fecha_expedicion,
			                                 huella, huella_anterior, xml, generado_en, generado_por)
			SELECT gen_random_uuid(), 'acme', 'i', gen_random_uuid(), 'alta', 'B89890001', 'S-' || g, '2026-09-12',
			       md5(g::text), 'h' || g, '<x/>', now(), 'ada'
			  FROM generate_series(1, $1::int) g`, verifactu.MaxRegistros)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	lote, err := st.ReclamarLoteVerifactu(ctx)
	if err != nil || len(lote.IDs) != verifactu.MaxRegistros {
		t.Fatalf("lote lleno: %d %v", len(lote.IDs), err)
	}
	if err := st.GuardarRespuestaVerifactu(ctx, lote, verifactu.Respuesta{}, &verifactu.ErrorEnvio{Detalle: "503"}); err != nil {
		t.Fatal(err)
	}
	if otro, err := st.ReclamarLoteVerifactu(ctx); err != nil || len(otro.IDs) != 0 {
		t.Fatalf("tras fallar, un lote lleno espera como los demás: %d %v", len(otro.IDs), err)
	}
}

// TestNingunaFacturaEmitidaQuedaFueraDeLaCadena (NEG-14): ni escribiendo a mano
// el `externo_id` de una factura importada, ni por la tienda, que la creaba ya
// pagada y `registroQueToca` la daba por emitida.
func TestNingunaFacturaEmitidaQuedaFueraDeLaCadena(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	st.Version = "v-test"
	ctx := tenantCtx("acme")
	c := cat(t)
	factura := modelo(t, c, "factura_venta")
	organizacionVerifactu(ctx, t, st, true)

	cliente := crear(ctx, t, st, modelo(t, c, "contacto"), `{"nombre":"Cafetería Cliente S.L.","tipo":"organizacion","nif":"ES B12345674","pais":"ES"}`)
	datos := map[string]any{"cliente": cliente.ID, "fecha": "2026-09-12", "base": int64(100000), "iva": int64(21000), "total": int64(121000), "externo_id": "odoo:1"}
	if _, err := st.Crear(ctx, factura, datos, "", "ada"); err == nil {
		t.Fatal("un alta a mano no se marca como importada")
	}
	if _, _, err := st.CrearVarios(ctx, []store.Nuevo{{Modelo: factura, Data: datos}}, nil, "ada"); err == nil {
		t.Fatal("un plan de IA no se marca como importado")
	}
	fv := facturaVF(ctx, t, st, c, "")
	if _, err := st.Actualizar(ctx, factura, fv.ID, datos, ""); err == nil {
		t.Fatal("una edición no marca la factura como importada")
	}
	delete(datos, "externo_id")
	if _, err := st.Actualizar(ctx, factura, fv.ID, datos, ""); err != nil {
		t.Fatalf("editar sin tocarlo: %v", err)
	}
	if _, err := st.CambiarEstado(ctx, factura, fv.ID, "confirmado", "ada"); err != nil {
		t.Fatal(err)
	}
	if r, _ := st.RegistrosVerifactu(ctx, fv.ID); len(r) != 1 {
		t.Fatalf("la factura emitida está en la cadena: %+v", r)
	}

	for _, m := range []string{"ventas", "facturacion"} {
		if err := st.ActivarModulo(ctx, m, true); err != nil {
			t.Fatal(err)
		}
	}
	var p store.PedidoTiendaInput
	p.Tenant, p.Origen, p.PedidoID, p.Numero, p.Fecha, p.Divisa, p.Factura = "acme", "pages", "00000000-0000-0000-0000-0000000000a1", "W-1", "2026-09-12", "EUR", "simplificada"
	p.Comprador.Nombre, p.Comprador.Correo = "Ana", "ana@example.com"
	p.Lineas = append(p.Lineas, struct {
		SKU                    string
		Nombre                 string
		Cantidad               int64
		PrecioUnitarioCentimos int64
		TipoIVA                int
	}{SKU: "taza", Nombre: "Taza", Cantidad: 1, PrecioUnitarioCentimos: 1210, TipoIVA: 21})
	p.TotalCentimos = 1210
	p.Cobro.Pasarela, p.Cobro.Referencia, p.Cobro.Fecha, p.Cobro.ImporteCentimos = "stripe", "pi_1", "2026-09-12", 1210
	res, err := st.ProcesarPedidoTienda(ctx, p, "pages")
	if err != nil {
		t.Fatalf("pedido de la tienda: %v", err)
	}
	var id, estado string
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id::text, estado FROM records WHERE model = 'factura_venta' AND ref = $1`, res.FacturaRef).Scan(&id, &estado)
	}); err != nil {
		t.Fatal(err)
	}
	if r, _ := st.RegistrosVerifactu(ctx, id); len(r) != 1 || estado != "pagado" {
		t.Fatalf("la factura de la tienda (%s) está en la cadena: %+v", estado, r)
	}
}

// TestLasFacturasDeLaTiendaEntranEnLaCadena (decisión del dueño, 17/10): la
// simplificada sin NIF y la completa con NIF que SyERP hace de un pedido cobrado
// de Sytue Web dejan su alta encadenada, con su huella, y salen en el envío como
// las demás. Un NIF español identifica al cliente aunque el pedido vaya a otro
// país: con la dirección de envío en Portugal, el alta lo mandaba como «PT» + NIF.
func TestLasFacturasDeLaTiendaEntranEnLaCadena(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	st.Version = "v-test"
	ctx := tenantCtx("acme")
	organizacionVerifactu(ctx, t, st, true)
	for _, m := range []string{"ventas", "facturacion"} {
		if err := st.ActivarModulo(ctx, m, true); err != nil {
			t.Fatal(err)
		}
	}
	pedido := func(id, numero, factura, nif, pais string) store.ResultadoPedidoTienda {
		t.Helper()
		var p store.PedidoTiendaInput
		p.Tenant, p.Origen, p.PedidoID, p.Numero, p.Fecha, p.Divisa, p.Factura = "acme", "pages", id, numero, "2026-09-12", "EUR", factura
		p.Comprador.Nombre, p.Comprador.Correo, p.Comprador.NIF = "Cafetería Cliente S.L.", "ana@example.com", nif
		p.Comprador.Direccion.Pais = pais
		p.Lineas = append(p.Lineas, struct {
			SKU                    string
			Nombre                 string
			Cantidad               int64
			PrecioUnitarioCentimos int64
			TipoIVA                int
		}{SKU: "taza", Nombre: "Taza", Cantidad: 2, PrecioUnitarioCentimos: 1210, TipoIVA: 21})
		p.TotalCentimos = 2420
		p.Cobro.Pasarela, p.Cobro.Referencia, p.Cobro.Fecha, p.Cobro.ImporteCentimos = "stripe", "pi_"+numero, "2026-09-12T10:00:00Z", 2420
		res, err := st.ProcesarPedidoTienda(ctx, p, "pages")
		if err != nil {
			t.Fatalf("pedido %s: %v", numero, err)
		}
		return res
	}
	registros := func(ref string) []store.RegistroVerifactu {
		t.Helper()
		var id string
		if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT id::text FROM records WHERE model = 'factura_venta' AND ref = $1`, ref).Scan(&id)
		}); err != nil {
			t.Fatal(err)
		}
		r, err := st.RegistrosVerifactu(ctx, id)
		if err != nil || len(r) != 1 || r[0].Tipo != "alta" || r[0].Estado != "pendiente" || r[0].NumSerie != ref {
			t.Fatalf("la factura %s tiene su alta: %+v %v", ref, r, err)
		}
		return r
	}

	f2 := pedido("00000000-0000-0000-0000-0000000000b1", "W-1", "simplificada", "", "ES")
	f1 := pedido("00000000-0000-0000-0000-0000000000b2", "W-2", "completa", "B12345674", "PT")
	r2, r1 := registros(f2.FacturaRef), registros(f1.FacturaRef)
	if f2.Verifactu != "pendiente" || f1.Verifactu != "pendiente" {
		t.Fatalf("Pages recibe el estado de VERI*FACTU: %+v %+v", f2, f1)
	}

	docs := xmlDeRegistros(ctx, t, st)
	if len(docs) != 2 {
		t.Fatalf("dos altas en la cadena:\n%s", strings.Join(docs, "\n"))
	}
	if d := docs[0]; !strings.Contains(d, "<sum1:TipoFactura>F2</sum1:TipoFactura>") || strings.Contains(d, "Destinatarios") ||
		!strings.Contains(d, "<sum1:PrimerRegistro>S</sum1:PrimerRegistro>") || !strings.Contains(d, "<sum1:Huella>"+r2[0].Huella+"</sum1:Huella>") {
		t.Fatalf("simplificada:\n%s", d)
	}
	if d := docs[1]; !strings.Contains(d, "<sum1:TipoFactura>F1</sum1:TipoFactura>") ||
		!strings.Contains(d, "<sum1:Destinatarios><sum1:IDDestinatario><sum1:NombreRazon>Cafetería Cliente S.L.</sum1:NombreRazon><sum1:NIF>B12345674</sum1:NIF>") ||
		strings.Contains(d, "IDOtro") ||
		!strings.Contains(d, "<sum1:Huella>"+r2[0].Huella+"</sum1:Huella></sum1:RegistroAnterior>") ||
		!strings.Contains(d, "<sum1:Huella>"+r1[0].Huella+"</sum1:Huella>") {
		t.Fatalf("completa, encadenada tras la simplificada:\n%s", d)
	}

	lote, err := st.ReclamarLoteVerifactu(ctx)
	if err != nil || len(lote.IDs) != 2 || lote.IDs[0] != r2[0].ID || lote.IDs[1] != r1[0].ID {
		t.Fatalf("las dos salen en el envío: %+v %v", lote.IDs, err)
	}
}

func emitirCertStorePrueba(t *testing.T, orgID string) (certPEM, keyPEM []byte) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sujeto := pkix.Name{CommonName: "Certificado Organización"}
	if orgID != "" {
		sujeto.ExtraNames = append(sujeto.ExtraNames, pkix.AttributeTypeAndValue{
			Type:  asn1.ObjectIdentifier{2, 5, 4, 97},
			Value: orgID,
		})
	}
	ahora := time.Now().UTC()
	plantilla := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               sujeto,
		NotBefore:             ahora.Add(-time.Hour),
		NotAfter:              ahora.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, plantilla, plantilla, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
}

// TestVerifactuProduccionExigeCertificadoPropioConMismoNIF (NEG-08): en
// producción no vale el certificado de plataforma: cada organización necesita
// su propio certificado cualificado cuyo NIF coincida con el NIF emisor.
func TestVerifactuProduccionExigeCertificadoPropioConMismoNIF(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	cifra, err := keycrypto.New(bytes.Repeat([]byte("k"), keycrypto.KeyLen))
	if err != nil {
		t.Fatal(err)
	}
	ctx := tenantCtx("acme")
	organizacionVerifactu(ctx, t, st, false)

	respaldo := &verifactu.Cliente{}
	// En pruebas sirve el de respaldo; en producción, sin certificado propio, se rechaza.
	if cli, err := st.ClienteVerifactu(ctx, cifra, false, respaldo); err != nil || cli != respaldo {
		t.Fatalf("en pruebas sin certificado propio usa el de respaldo: %v %v", cli, err)
	}
	if _, err := st.ClienteVerifactu(ctx, cifra, true, respaldo); !errors.Is(err, store.ErrCertificadoInvalido) {
		t.Fatalf("en producción sin certificado propio debe fallar: %v", err)
	}

	// Un certificado con otro NIF o sin NIF no se guarda.
	certAjeno, keyAjeno := emitirCertStorePrueba(t, "VATES-B12345674")
	if _, err := st.GuardarCertificadoVerifactu(ctx, certAjeno, keyAjeno, false, cifra); !errors.Is(err, store.ErrCertificadoNIFNoCoincide) {
		t.Fatalf("certificado con NIF ajeno: %v", err)
	}
	certSinNIF, keySinNIF := emitirCertStorePrueba(t, "")
	if _, err := st.GuardarCertificadoVerifactu(ctx, certSinNIF, keySinNIF, false, cifra); !errors.Is(err, store.ErrCertificadoInvalido) {
		t.Fatalf("certificado sin NIF: %v", err)
	}

	// Con el certificado del mismo NIF de la organización (B89890001), se guarda y habilita producción.
	certPropio, keyPropio := emitirCertStorePrueba(t, "VATES-B89890001")
	if nif, err := st.GuardarCertificadoVerifactu(ctx, certPropio, keyPropio, false, cifra); err != nil || nif != "B89890001" {
		t.Fatalf("GuardarCertificadoVerifactu: nif=%q err=%v", nif, err)
	}
	if v, err := st.EstadoVerifactu(ctx); err != nil || !v.TieneCertificado || v.CertificadoNIF != "B89890001" {
		t.Fatalf("EstadoVerifactu tras guardar certificado: %+v %v", v, err)
	}
	cliProd, err := st.ClienteVerifactu(ctx, cifra, true, respaldo)
	if err != nil || cliProd == nil || cliProd == respaldo || cliProd.NIF() != "B89890001" {
		t.Fatalf("ClienteVerifactu en producción usa el propio: cli=%v nif=%q err=%v", cliProd, cliProd.NIF(), err)
	}

	// Cambiar el NIF de la organización antes de activar VERI*FACTU invalida el certificado anterior.
	if _, err := st.GuardarOrganizacion(ctx, store.Organizacion{
		Nombre: "Panadería Ejemplo, S.L.", NIF: "B12345674", Direccion: "Calle Mayor, 1", CP: "28001",
		Ciudad: "Madrid", Pais: "ES", Forma: "sl",
	}, "ada"); err != nil {
		t.Fatal(err)
	}
	if v, err := st.EstadoVerifactu(ctx); err != nil || v.TieneCertificado {
		t.Fatalf("cambiar el NIF debe limpiar el certificado anterior: %+v %v", v, err)
	}
	if _, err := st.ClienteVerifactu(ctx, cifra, true, respaldo); !errors.Is(err, store.ErrCertificadoInvalido) {
		t.Fatalf("tras cambiar el NIF no queda certificado para producción: %v", err)
	}
}
