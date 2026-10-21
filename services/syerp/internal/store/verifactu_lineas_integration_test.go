// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/syerp/db"
	"git.sytue.eu/sytue/one/services/syerp/internal/catalogo"
	"git.sytue.eu/sytue/one/services/syerp/internal/domain"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
	"git.sytue.eu/sytue/one/services/syerp/internal/verifactu"
)

// variablesVF son las partes de un registro que cambian en cada ejecución: la
// referencia interna, el momento de generación, el número de instalación (sale
// de la hora de activación) y las huellas, que dependen del momento.
var variablesVF = []struct {
	re    *regexp.Regexp
	marca string
}{
	{regexp.MustCompile(`<sum1:RefExterna>[^<]*<`), "<sum1:RefExterna>REF<"},
	{regexp.MustCompile(`<sum1:FechaHoraHusoGenRegistro>[^<]*<`), "<sum1:FechaHoraHusoGenRegistro>GENERADO<"},
	{regexp.MustCompile(`<sum1:NumeroInstalacion>[^<]*<`), "<sum1:NumeroInstalacion>INSTALACION<"},
	{regexp.MustCompile(`<sum1:Huella>[^<]*<`), "<sum1:Huella>HUELLA<"},
}

// compararConOro compara `obtenido` con testdata/<fichero>. Con ACTUALIZAR_ORO=1
// lo escribe en vez de compararlo.
func compararConOro(t *testing.T, fichero, obtenido string) {
	t.Helper()
	ruta := filepath.Join("testdata", fichero)
	if os.Getenv("ACTUALIZAR_ORO") != "" {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(ruta, []byte(obtenido), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	oro, err := os.ReadFile(ruta)
	if err != nil {
		t.Fatalf("falta %s (se genera con ACTUALIZAR_ORO=1): %v", ruta, err)
	}
	if string(oro) != obtenido {
		t.Fatalf("%s ha cambiado.\n--- oro\n%s\n--- ahora\n%s", ruta, oro, obtenido)
	}
}

// Una factura SIN líneas sale en VERI*FACTU byte a byte igual que antes de que
// existieran las líneas y el tipo de factura: la general, la exenta y la
// rectificativa. El oro se generó con el código anterior a las líneas.
func TestVerifactuSinLineasComoAntes(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	st.Version = "v-test"
	ctx := tenantCtx("acme")
	c := cat(t)
	organizacionVerifactu(ctx, t, st, true)
	factura := modelo(t, c, "factura_venta")
	cliente := crear(ctx, t, st, modelo(t, c, "contacto"),
		`{"nombre":"Cafetería Cliente S.L.","tipo":"organizacion","nif":"ES B12345674","pais":"ES"}`)

	general := crear(ctx, t, st, factura, fmt.Sprintf(
		`{"cliente":%q,"fecha":"2026-09-12","base":100000,"iva":21000,"total":121000,"notas":"Pan de septiembre"}`, cliente.ID))
	exenta := crear(ctx, t, st, factura, fmt.Sprintf(
		`{"cliente":%q,"fecha":"2026-09-13","base":50000,"iva":0,"total":50000,"regimen_iva":"iva_exenta"}`, cliente.ID))
	for _, f := range []store.Registro{general, exenta} {
		if _, err := st.CambiarEstado(ctx, modelo(t, cat(t), "factura_venta"), f.ID, "confirmado", "ada"); err != nil {
			t.Fatalf("emitir %s: %v", f.Ref, err)
		}
	}
	rectificativa := crear(ctx, t, st, factura, fmt.Sprintf(
		`{"cliente":%q,"fecha":"2026-09-14","base":-10000,"iva":-2100,"total":-12100,"rectifica":%q}`, cliente.ID, general.ID))
	if _, err := st.CambiarEstado(ctx, modelo(t, cat(t), "factura_venta"), rectificativa.ID, "confirmado", "ada"); err != nil {
		t.Fatalf("emitir la rectificativa: %v", err)
	}

	docs := xmlDeRegistros(ctx, t, st)
	for i := range docs {
		for _, v := range variablesVF {
			docs[i] = v.re.ReplaceAllString(docs[i], v.marca)
		}
	}
	compararConOro(t, "verifactu-sin-lineas.xml", strings.Join(docs, "\n")+"\n")
}

// lineasVF crea una factura de venta con sus líneas. `lineas` son objetos JSON sin
// el campo `factura`, que se añade aquí.
func lineasVF(ctx context.Context, t *testing.T, st *store.Store, c *catalogo.Catalogo, cabecera string, lineas ...string) store.Registro {
	t.Helper()
	f := crear(ctx, t, st, modelo(t, c, "factura_venta"), cabecera)
	for _, l := range lineas {
		crear(ctx, t, st, modelo(t, c, "linea_factura"), fmt.Sprintf(`{"factura":%q,%s`, f.ID, strings.TrimPrefix(l, "{")))
	}
	return f
}

func emitir(ctx context.Context, t *testing.T, st *store.Store, f store.Registro) {
	t.Helper()
	if _, err := st.CambiarEstado(ctx, modelo(t, cat(t), "factura_venta"), f.ID, "confirmado", "ada"); err != nil {
		t.Fatalf("emitir %s: %v", f.Ref, err)
	}
}

// Con líneas, el desglose es el de las líneas agrupado por tipo; la simplificada
// no lleva destinatario, y su rectificativa es R5. El oro es el XML que se revisó
// contra el diseño de registro de la AEAT.
func TestVerifactuConLineas(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	st.Version = "v-test"
	ctx := tenantCtx("acme")
	c := cat(t)
	organizacionVerifactu(ctx, t, st, true)
	contacto := modelo(t, c, "contacto")
	enLinea := crear(ctx, t, st, contacto, `{"nombre":"Ventas en línea — Panadería","tipo":"organizacion"}`)
	cliente := crear(ctx, t, st, contacto, `{"nombre":"Cafetería Cliente S.L.","tipo":"organizacion","nif":"B12345674","pais":"ES"}`)

	// Simplificada con vino al 21 % y menú al 10 %: 12,10 + 2 × 11,00 = 34,10 €.
	f2 := lineasVF(ctx, t, st, c,
		fmt.Sprintf(`{"cliente":%q,"fecha":"2026-09-20","tipo_factura":"F2","base":3000,"iva":410,"total":3410,"notas":"Pedido P-000123"}`, enLinea.ID),
		`{"descripcion":"Vino tinto","cantidad":"1","precio_con_iva":1210,"tipo_iva":"21"}`,
		`{"descripcion":"Menú del día","cantidad":"2","precio_con_iva":1100,"tipo_iva":"10"}`)
	emitir(ctx, t, st, f2)

	// Completa con dos productos al 21 % y al 10 % y el envío al 21 %.
	f1 := lineasVF(ctx, t, st, c,
		fmt.Sprintf(`{"cliente":%q,"fecha":"2026-09-20","base":2905,"iva":555,"total":3460}`, cliente.ID),
		`{"descripcion":"Aceite","cantidad":"2","precio_con_iva":1210,"tipo_iva":"21"}`,
		`{"descripcion":"Pan","cantidad":"1","precio_con_iva":550,"tipo_iva":"10"}`,
		`{"descripcion":"Envío","cantidad":"1","precio_con_iva":490,"tipo_iva":"21"}`)
	emitir(ctx, t, st, f1)

	// Completa con una parte exenta: la línea al 0 % va como diga el régimen.
	mixta := lineasVF(ctx, t, st, c,
		fmt.Sprintf(`{"cliente":%q,"fecha":"2026-09-21","base":6000,"iva":210,"total":6210,"regimen_iva":"iva_exenta"}`, cliente.ID),
		`{"descripcion":"Libro","cantidad":"1","precio_con_iva":1210,"tipo_iva":"21"}`,
		`{"descripcion":"Curso","cantidad":"1","precio_con_iva":5000,"tipo_iva":"0"}`)
	emitir(ctx, t, st, mixta)

	// La rectificativa de una simplificada es R5, también sin destinatario.
	r5 := lineasVF(ctx, t, st, c,
		fmt.Sprintf(`{"cliente":%q,"fecha":"2026-09-22","tipo_factura":"F2","base":-1000,"iva":-210,"total":-1210,"rectifica":%q}`, enLinea.ID, f2.ID),
		`{"descripcion":"Devolución: vino tinto","cantidad":"-1","precio_con_iva":1210,"tipo_iva":"21"}`)
	emitir(ctx, t, st, r5)

	docs := xmlDeRegistros(ctx, t, st)
	for i := range docs {
		for _, v := range variablesVF {
			docs[i] = v.re.ReplaceAllString(docs[i], v.marca)
		}
	}
	compararConOro(t, "verifactu-con-lineas.xml", strings.Join(docs, "\n")+"\n")
}

// Lo que no se emite: con o sin VERI*FACTU, una cabecera que no es la suma de las
// líneas y una simplificada que no puede serlo. Con VERI*FACTU, además, una línea
// al 0 % sin régimen y una rectificativa de otro tipo que la rectificada.
func TestLineasQueNoSeEmiten(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	st.Version = "v-test"
	c := cat(t)
	vino := `{"descripcion":"Vino","cantidad":"1","precio_con_iva":1210,"tipo_iva":"21"}`

	for _, conVF := range []bool{false, true} {
		ctx := tenantCtx(map[bool]string{false: "sinvf", true: "convf"}[conVF])
		organizacionVerifactu(ctx, t, st, conVF)
		cliente := crear(ctx, t, st, modelo(t, c, "contacto"), `{"nombre":"Cafetería Cliente S.L.","tipo":"organizacion","nif":"B12345674","pais":"ES"}`)
		casos := map[string]store.Registro{
			"cabecera que no casa": lineasVF(ctx, t, st, c,
				fmt.Sprintf(`{"cliente":%q,"fecha":"2026-09-20","base":1000,"iva":200,"total":1200}`, cliente.ID), vino),
			"simplificada de más de 3.000 €": lineasVF(ctx, t, st, c,
				fmt.Sprintf(`{"cliente":%q,"fecha":"2026-09-20","tipo_factura":"F2","base":248000,"iva":52080,"total":300080}`, cliente.ID),
				`{"descripcion":"Horno","cantidad":"1","precio_con_iva":300080,"tipo_iva":"21"}`),
			"simplificada intracomunitaria": crear(ctx, t, st, modelo(t, c, "factura_venta"),
				fmt.Sprintf(`{"cliente":%q,"fecha":"2026-09-20","tipo_factura":"F2","base":1000,"regimen_iva":"iva_entrega_ue"}`, cliente.ID)),
		}
		if conVF {
			casos["línea al 0 % sin régimen"] = lineasVF(ctx, t, st, c,
				fmt.Sprintf(`{"cliente":%q,"fecha":"2026-09-20","base":5000,"iva":0,"total":5000}`, cliente.ID),
				`{"descripcion":"Curso","cantidad":"1","precio_con_iva":5000,"tipo_iva":"0"}`)
			completa := lineasVF(ctx, t, st, c, fmt.Sprintf(`{"cliente":%q,"fecha":"2026-09-20","base":1000,"iva":210,"total":1210}`, cliente.ID), vino)
			emitir(ctx, t, st, completa)
			casos["rectificativa simplificada de una completa"] = crear(ctx, t, st, modelo(t, c, "factura_venta"),
				fmt.Sprintf(`{"cliente":%q,"fecha":"2026-09-21","tipo_factura":"F2","base":-1000,"iva":-210,"total":-1210,"rectifica":%q}`, cliente.ID, completa.ID))
		}
		for caso, f := range casos {
			_, err := st.CambiarEstado(ctx, modelo(t, cat(t), "factura_venta"), f.ID, "confirmado", "ada")
			if !errors.Is(err, domain.ErrDatos) && !errors.Is(err, verifactu.ErrDatos) {
				t.Errorf("VERI*FACTU %v, %s: se emitió (%v)", conVF, caso, err)
				continue
			}
			if reg, _ := st.Obtener(ctx, "factura_venta", f.ID); reg.Estado != "borrador" {
				t.Errorf("VERI*FACTU %v, %s: quedó en %s", conVF, caso, reg.Estado)
			}
		}
		// Justo en el tope, sí.
		tope := lineasVF(ctx, t, st, c,
			fmt.Sprintf(`{"cliente":%q,"fecha":"2026-09-20","tipo_factura":"F2","base":247934,"iva":52066,"total":300000}`, cliente.ID),
			`{"descripcion":"Horno","cantidad":"1","precio_con_iva":300000,"tipo_iva":"21"}`)
		emitir(ctx, t, st, tope)
	}
}

// Emitida la factura, sus líneas y su tipo ya no cambian: lo impide la base
// (0098), no el código. Una línea de un borrador, sí.
func TestLineasDeFacturaEmitidaNoCambian(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	st.Version = "v-test"
	ctx := tenantCtx("acme")
	c := cat(t)
	organizacionVerifactu(ctx, t, st, true)
	lineaM := modelo(t, c, "linea_factura")
	cliente := crear(ctx, t, st, modelo(t, c, "contacto"), `{"nombre":"Cafetería Cliente S.L.","tipo":"organizacion","nif":"B12345674","pais":"ES"}`)
	cabecera := fmt.Sprintf(`{"cliente":%q,"fecha":"2026-09-20","base":1000,"iva":210,"total":1210}`, cliente.ID)
	vino := `{"descripcion":"Vino","cantidad":"1","precio_con_iva":1210,"tipo_iva":"21"}`

	borrador := lineasVF(ctx, t, st, c, cabecera, vino)
	lineas, err := st.Listar(ctx, store.Filtro{Modelo: "linea_factura", Campo: "factura", Valor: borrador.ID})
	if err != nil || len(lineas.Registros) != 1 {
		t.Fatalf("líneas del borrador: %+v %v", lineas, err)
	}
	enBorrador := lineas.Registros[0]
	enBorrador.Data["cantidad"] = "2"
	if _, err := st.Actualizar(ctx, lineaM, enBorrador.ID, enBorrador.Data, "Vino"); err != nil {
		t.Fatalf("una línea de un borrador se cambia: %v", err)
	}

	emitida := lineasVF(ctx, t, st, c, cabecera, vino)
	emitir(ctx, t, st, emitida)
	lineas, _ = st.Listar(ctx, store.Filtro{Modelo: "linea_factura", Campo: "factura", Valor: emitida.ID})
	linea := lineas.Registros[0]

	nueva, _, err := domain.Validar(lineaM, json.RawMessage(fmt.Sprintf(`{"factura":%q,"descripcion":"Regalo","cantidad":"1","precio_con_iva":0,"tipo_iva":"21"}`, emitida.ID)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Crear(ctx, lineaM, nueva, "Regalo", "ada"); !errors.Is(err, store.ErrFacturaEmitida) {
		t.Errorf("añadir una línea: %v", err)
	}
	cambiada := map[string]any{"factura": emitida.ID, "descripcion": "Vino", "cantidad": "2", "precio_con_iva": int64(1210), "tipo_iva": "21"}
	if _, err := st.Actualizar(ctx, lineaM, linea.ID, cambiada, "Vino"); !errors.Is(err, store.ErrFacturaEmitida) {
		t.Errorf("cambiar la cantidad: %v", err)
	}
	cambiada["cantidad"], cambiada["factura"] = "1", borrador.ID
	if _, err := st.Actualizar(ctx, lineaM, linea.ID, cambiada, "Vino"); !errors.Is(err, store.ErrFacturaEmitida) {
		t.Errorf("pasarla a otra factura: %v", err)
	}
	if err := st.Borrar(ctx, "linea_factura", linea.ID); !errors.Is(err, store.ErrFacturaEmitida) {
		t.Errorf("a la papelera: %v", err)
	}
	datos := emitida.Data
	datos["tipo_factura"] = "F2"
	if _, err := st.Actualizar(ctx, modelo(t, c, "factura_venta"), emitida.ID, datos, ""); !errors.Is(err, store.ErrFacturaEmitida) {
		t.Errorf("pasarla a simplificada: %v", err)
	}
	// Ni a pelo: el código equivocado de mañana tampoco pasa.
	err = h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `DELETE FROM records WHERE id = $1::uuid`, linea.ID)
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "ya se ha enviado") {
		t.Errorf("borrarla con SQL: %v", err)
	}
	// Lo que reescribe una fusión (el tenant, la fecha de edición) no toca la línea.
	err = h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE records SET updated_at = now() WHERE id = $1::uuid`, linea.ID)
		return err
	})
	if err != nil {
		t.Errorf("tocar solo la fecha de edición: %v", err)
	}
}
