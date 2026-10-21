// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pg

import (
	"strings"
	"testing"
)

func TestTenantRLSPolicySQL(t *testing.T) {
	sql, err := TenantRLSPolicySQL("drive_files")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	for _, want := range []string{
		"ENABLE ROW LEVEL SECURITY",
		"FORCE ROW LEVEL SECURITY",
		"CREATE POLICY rls_tenant_isolation_drive_files",
		"tenant_slug = current_setting('app.tenant', true)",
		"WITH CHECK",
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("SQL no contiene %q:\n%s", want, sql)
		}
	}
}

func TestTenantRLSPolicyRejectsUnsafeTable(t *testing.T) {
	for _, bad := range []string{"", "drop table x;--", "Files", "1abc", "a b"} {
		if _, err := TenantRLSPolicySQL(bad); err == nil {
			t.Errorf("nombre inseguro aceptado: %q", bad)
		}
	}
}

func TestIdentSafe(t *testing.T) {
	ok := []string{"a", "table1", "drive_files", "x_y_z"}
	bad := []string{"", "1x", "Foo", "a-b", "a;b", "sel ect", strings.Repeat("a", 64)}
	for _, s := range ok {
		if !identSafe(s) {
			t.Errorf("%q debería ser seguro", s)
		}
	}
	for _, s := range bad {
		if identSafe(s) {
			t.Errorf("%q no debería ser seguro", s)
		}
	}
}

func TestTenantSettingKeyConstant(t *testing.T) {
	if TenantSettingKey != "app.tenant" {
		t.Fatalf("clave de tenant inesperada: %q", TenantSettingKey)
	}
}
