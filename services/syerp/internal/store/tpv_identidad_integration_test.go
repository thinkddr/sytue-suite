// SPDX-License-Identifier: Apache-2.0
//go:build integration

package store_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/syerp/db"
	"git.sytue.eu/sytue/one/services/syerp/internal/domain"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

func TestIdentidadTPVNoSeFalsificaNiSePierde(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	c := cat(t)
	ctx := tenantCtx("acme")
	m := modelo(t, c, "presupuesto")
	cliente := crear(ctx, t, st, modelo(t, c, "contacto"), `{"nombre":"Cliente","tipo":"organizacion"}`)
	datos := map[string]any{"cliente": cliente.ID, "fecha": "2026-10-03", "base": 100, "iva": 21, "total": 121, "tpv_operacion": uuid.NewString(), "tpv_huella": "huella"}
	if _, err := st.Crear(ctx, m, datos, "Oferta", "ada"); !errors.Is(err, domain.ErrDatos) {
		t.Fatalf("alta falsificada: %v", err)
	}
	if _, _, err := st.CrearVarios(ctx, []store.Nuevo{{Modelo: m, Data: datos, Etiqueta: "Oferta"}}, nil, "ada"); !errors.Is(err, domain.ErrDatos) {
		t.Fatalf("alta por plan falsificada: %v", err)
	}
	r := crear(ctx, t, st, m, fmt.Sprintf(`{"cliente":%q,"fecha":"2026-10-03","base":100,"iva":21,"total":121}`, cliente.ID))
	id := uuid.NewString()
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE records SET data=data||jsonb_build_object('tpv_operacion',$2::text,'tpv_huella','original') WHERE id=$1::uuid`, r.ID, id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	normal := map[string]any{"cliente": cliente.ID, "fecha": "2026-10-03", "base": 100, "iva": 21, "total": 121, "notas": "Revisado"}
	actualizado, err := st.Actualizar(ctx, m, r.ID, normal, "")
	if err != nil || actualizado.Data["tpv_operacion"] != id || actualizado.Data["tpv_huella"] != "original" {
		t.Fatalf("identidad perdida: %+v %v", actualizado, err)
	}
	normal["tpv_operacion"] = uuid.NewString()
	if _, err := st.Actualizar(ctx, m, r.ID, normal, ""); !errors.Is(err, domain.ErrDatos) {
		t.Fatalf("identidad reemplazada: %v", err)
	}
	copia, err := st.Duplicar(ctx, m, r.ID, "ada")
	if err != nil || copia.Data["tpv_operacion"] != nil || copia.Data["tpv_huella"] != nil {
		t.Fatalf("duplicado arrastra cobro: %+v %v", copia, err)
	}
}
