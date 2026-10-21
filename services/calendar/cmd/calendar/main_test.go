// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"strings"
	"testing"
)

func TestRunDBCheckRequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	err := runDBCheck()
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("runDBCheck() error = %v; se esperaba DATABASE_URL vacía", err)
	}
}
