// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/erp/vectores"
)

// TestVectoresDorados recorre los escenarios compartidos con el almacén de Postgres
// (services/syerp/internal/store/vectores_integration_test.go) por el cliente de
// siempre y el transporte en proceso: misma numeración, mismos asientos, mismos saldos.
func TestVectoresDorados(t *testing.T) {
	escenarios, err := vectores.Cargar()
	if err != nil || len(escenarios) == 0 {
		t.Fatalf("vectores: %v %d", err, len(escenarios))
	}
	for _, esc := range escenarios {
		t.Run(esc.Nombre, func(t *testing.T) {
			ctx := context.Background()
			e, alm := abrir(t, t.TempDir())
			defer func() { _ = alm.Cerrar() }()
			ids := map[string]string{}
			modelos := map[string]string{}
			for i, p := range esc.Pasos {
				switch {
				case p.Crear != "":
					r, err := e.Crear(ctx, p.Crear, vectores.Resolver(p.Data, ids))
					if err != nil {
						t.Fatalf("paso %d: crear %s: %v", i, p.Crear, err)
					}
					if p.Ref != "" && r.Ref != p.Ref {
						t.Fatalf("paso %d: referencia %q, se esperaba %q", i, r.Ref, p.Ref)
					}
					if p.Como != "" {
						ids[p.Como], modelos[p.Como] = r.ID, p.Crear
					}
				case p.Asentar != "":
					modelo, id := modelos[p.Asentar], ids[p.Asentar]
					_, err := e.Asentar(ctx, modelo, id)
					if p.Error != "" {
						var ae *api.Error
						quiere := map[string]int{"ya_asentado": http.StatusConflict, "datos": http.StatusUnprocessableEntity}[p.Error]
						if !errors.As(err, &ae) || ae.Status != quiere {
							t.Fatalf("paso %d: asentar %s: %v, se esperaba %s", i, p.Asentar, err, p.Error)
						}
						continue
					}
					if err != nil {
						t.Fatalf("paso %d: asentar %s: %v", i, p.Asentar, err)
					}
					as, err := alm.AsientosDe(ctx, modelo, id)
					if err != nil || len(as) != 1 {
						t.Fatalf("paso %d: asientos de %s: %v %d", i, p.Asentar, err, len(as))
					}
					if err := alm.conApuntes(ctx, as); err != nil {
						t.Fatal(err)
					}
					a := as[0]
					if a.Ref != p.Ref || a.Diario != p.Diario || len(a.Apuntes) != len(p.Apuntes) {
						t.Fatalf("paso %d: asiento %+v, se esperaba %s %s con %d apuntes", i, a, p.Diario, p.Ref, len(p.Apuntes))
					}
					for j, q := range p.Apuntes {
						ap := a.Apuntes[j]
						if ap.Cuenta != q.Cuenta || ap.Debe != q.Debe || ap.Haber != q.Haber || ap.Tercero != q.TerceroID(ids) {
							t.Fatalf("paso %d: apunte %d = %+v, se esperaba %+v", i, j, ap, q)
						}
					}
					if r, err := e.Obtener(ctx, modelo, id); err != nil || r.Estado != p.Estado {
						t.Fatalf("paso %d: estado de %s: %v %q", i, p.Asentar, err, r.Estado)
					}
				}
			}
			b, err := e.Balance(ctx)
			if err != nil || len(b.Cuentas) != len(esc.Balance) {
				t.Fatalf("balance: %v %+v", err, b)
			}
			for i, q := range esc.Balance {
				c := b.Cuentas[i]
				if c.Cuenta != q.Cuenta || c.Debe != q.Debe || c.Haber != q.Haber || c.Saldo != q.Saldo {
					t.Fatalf("balance %d = %+v, se esperaba %+v", i, c, q)
				}
			}
			if b.TotalDebe != b.TotalHaber {
				t.Fatalf("el libro no cuadra: %d / %d", b.TotalDebe, b.TotalHaber)
			}
		})
	}
}
