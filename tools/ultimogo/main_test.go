// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import "testing"

func TestUltimaEstableIgnoraRcBetasYOrdenaPorNumero(t *testing.T) {
	lista := "v0.0.1-go1.9rc2.linux-amd64 v0.0.1-go1.27.4.linux-amd64 v0.0.1-go1.27.10.darwin-arm64 " +
		"v0.0.1-go1.28rc1.linux-amd64 v0.0.1-go1.28beta1.linux-arm64 v0.0.1-go1.27.9.linux-amd64 otra-cosa"
	got, ok := ultimaEstable(lista)
	// 1.27.10 > 1.27.9 (por número, no por texto) y la rc de 1.28 no cuenta.
	if !ok || got != (version{1, 27, 10}) {
		t.Fatalf("última = %v, %v; quiero 1.27.10", got, ok)
	}
	if _, ok := ultimaEstable("v0.0.1-go1.28rc1.linux-amd64"); ok {
		t.Fatal("una lista solo con rc no tiene última estable")
	}
}

func TestVersionDeGoModYComparacion(t *testing.T) {
	v, ok := versionDeGoMod("module x\n\ngo 1.27.4\n\nrequire y v1.0.0\n")
	if !ok || v != (version{1, 27, 4}) {
		t.Fatalf("go.mod = %v, %v", v, ok)
	}
	if _, ok := versionDeGoMod("module x\n\ngo 1.27\n"); ok {
		t.Fatal("una versión sin parche no es una versión fijada")
	}
	if !v.menor(version{1, 28, 0}) || !v.menor(version{1, 27, 5}) || v.menor(v) || v.menor(version{1, 26, 9}) {
		t.Fatal("comparación de versiones")
	}
}
