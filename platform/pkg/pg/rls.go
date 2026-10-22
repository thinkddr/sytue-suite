// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pg

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// TenantSettingKey es la variable de sesión que transporta el tenant activo a
// las políticas RLS. Debe coincidir con las políticas generadas por
// TenantRLSPolicySQL y con SetTenant.
const TenantSettingKey = "app.tenant"

// tenantSQL devuelve la expresión SQL que las políticas RLS usan para leer el
// tenant activo de la sesión. `true` en current_setting evita error si no está
// fijado (deny-by-default: sin tenant, no hay filas visibles).
const tenantSQL = "current_setting('" + TenantSettingKey + "', true)"

// TenantRLSPolicySQL genera el SQL idempotente que habilita RLS multi-tenant
// sobre una tabla con columna `tenant_slug`. Aplica deny-by-default:
//   - ENABLE + FORCE ROW LEVEL SECURITY (afecta también al dueño de la tabla).
//   - Política USING que solo deja ver filas del tenant de la sesión.
//   - Política WITH CHECK que impide insertar/actualizar filas de otro tenant.
//
// El identificador de tabla se valida (identSafe) para evitar SQL dinámico
// peligroso; el valor del tenant NUNCA se interpola: llega por set_config
// parametrizado (ver SetTenant).
func TenantRLSPolicySQL(table string) (string, error) {
	if !identSafe(table) {
		return "", fmt.Errorf("pg: nombre de tabla no seguro para RLS: %q", table)
	}
	policy := "rls_tenant_isolation_" + table
	sql := fmt.Sprintf(
		"ALTER TABLE %[1]s ENABLE ROW LEVEL SECURITY;\n"+
			"ALTER TABLE %[1]s FORCE ROW LEVEL SECURITY;\n"+
			"DROP POLICY IF EXISTS %[2]s ON %[1]s;\n"+
			"CREATE POLICY %[2]s ON %[1]s\n"+
			"    USING (tenant_slug = %[3]s)\n"+
			"    WITH CHECK (tenant_slug = %[3]s);",
		table, policy, tenantSQL,
	)
	return sql, nil
}

// WithTenant fija el tenant del contexto en la conexión/transacción actual para
// que las políticas RLS lo apliquen. Debe llamarse dentro de una transacción
// (SET LOCAL) o al inicio de una conexión dedicada. Devuelve ErrNoTenant si no
// hay tenant en el contexto (deny-by-default explícito).
func WithTenant(ctx context.Context, q Querier) error {
	t, err := tenancy.From(ctx)
	if err != nil {
		return err
	}
	return SetTenant(ctx, q, t.Slug)
}

// InRLSTx abre una transacción, fija el tenant del contexto (SET LOCAL) y ejecuta
// fn. Es el camino recomendado para cualquier acceso a tablas con RLS: garantiza
// que ninguna consulta se ejecute sin tenant fijado.
func (p *Pool) InRLSTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	return p.InTenantTx(ctx, fn)
}

// MaintenanceSettingKey es la variable de sesión que habilita la política
// PERMISSIVE de mantenimiento (operaciones cross-tenant controladas: cola de
// correo, purga, entrega del MX antes de conocer el tenant).
const MaintenanceSettingKey = "app.maintenance"

// SetMaintenance fija `app.maintenance='on'` LOCAL a la transacción actual, para
// que las políticas PERMISSIVE de mantenimiento habiliten operaciones
// cross-tenant SIN necesidad de un rol BYPASSRLS. Las transacciones de negocio
// nunca fijan este flag, así que su aislamiento por tenant no se ve afectado.
func SetMaintenance(ctx context.Context, q Querier) error {
	if _, err := q.Exec(ctx, "SELECT set_config('"+MaintenanceSettingKey+"', 'on', true)"); err != nil {
		return fmt.Errorf("pg: set_config app.maintenance: %w", err)
	}
	return nil
}

// InMaintenanceTx abre una transacción con `app.maintenance='on'` fijado (SET
// LOCAL) y ejecuta fn. Es el camino para tareas cross-tenant legítimas (cola de
// salida, resolución de dirección entrante del MX) sin recurrir a BYPASSRLS.
func (p *Pool) InMaintenanceTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	return p.InTx(ctx, func(tx pgx.Tx) error {
		if err := SetMaintenance(ctx, tx); err != nil {
			return err
		}
		return fn(tx)
	})
}

// identSafe valida que un identificador SQL contenga solo caracteres seguros
// (minúsculas, dígitos, guion bajo; sin empezar por dígito). Suficiente para los
// nombres de tabla generados por las migraciones de la suite.
func identSafe(s string) bool {
	if s == "" || len(s) > 63 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z':
		case c == '_':
		case c >= '0' && c <= '9':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// MergeSettingKey habilita las políticas PERMISSIVE de FUSIÓN de organizaciones.
//
// Es una variable propia y no `app.maintenance` a propósito: las políticas de
// mantenimiento existen en unas pocas tablas para tareas de negocio concretas
// (cola de salida, entrega del MX). La fusión necesita reescribir el tenant de
// TODAS las tablas, y concederle ese alcance a `app.maintenance` ampliaría en
// silencio lo que puede hacer cualquier transacción de mantenimiento ya escrita.
const MergeSettingKey = "app.merge"

// SetMerge fija `app.merge='on'` LOCAL a la transacción actual.
func SetMerge(ctx context.Context, q Querier) error {
	if _, err := q.Exec(ctx, "SELECT set_config('"+MergeSettingKey+"', 'on', true)"); err != nil {
		return fmt.Errorf("pg: set_config app.merge: %w", err)
	}
	return nil
}

// InMergeTx abre una transacción con `app.merge='on'` fijado y ejecuta fn. Lo usa
// EXCLUSIVAMENTE la fusión de organizaciones de la administración de plataforma.
func (p *Pool) InMergeTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	return p.InTx(ctx, func(tx pgx.Tx) error {
		if err := SetMerge(ctx, tx); err != nil {
			return err
		}
		return fn(tx)
	})
}

// MergePolicySQL genera el SQL idempotente que autoriza a una transacción de
// fusión a leer y escribir filas de cualquier tenant en `table`. Se aplica desde
// una migración por servicio; sin ella, la fusión vería cero filas y declararía
// «movidas 0» sin que nada fallara, que es el modo de fallo más peligroso de RLS.
func MergePolicySQL(table string) (string, error) {
	if !identSafe(table) {
		return "", fmt.Errorf("pg: nombre de tabla no seguro para fusión: %q", table)
	}
	policy := table + "_platform_merge"
	return fmt.Sprintf(
		"DROP POLICY IF EXISTS %[2]s ON %[1]s;\n"+
			"CREATE POLICY %[2]s ON %[1]s\n"+
			"    USING (current_setting('%[3]s', true) = 'on')\n"+
			"    WITH CHECK (current_setting('%[3]s', true) = 'on');",
		table, policy, MergeSettingKey,
	), nil
}
