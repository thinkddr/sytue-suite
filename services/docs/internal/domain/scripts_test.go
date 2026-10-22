// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestManifiestoNormalizaYAcota(t *testing.T) {
	m, err := ParseManifiesto([]byte(`{"v":1,"commands":["italic","bold","bold"],"read":["text","cells"],"write":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(m.Commands, []string{"bold", "italic"}) || !slices.Equal(m.Read, []string{"cells", "text"}) {
		t.Fatalf("no normaliza: %+v", m)
	}
	if m.MaxOps != OpsPorDefecto || m.MaxMs != MsPorDefecto {
		t.Fatalf("sin cotas por defecto: %+v", m)
	}
	// El JSON canónico es estable: normalizar dos veces da lo mismo.
	a, _ := json.Marshal(m)
	m2, err := ParseManifiesto(a)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(m2)
	if !bytes.Equal(a, b) {
		t.Fatalf("no es idempotente:\n%s\n%s", a, b)
	}
	for nombre, malo := range map[string]string{
		"vacío":      ``,
		"sobrante":   `{"v":1,"commands":[],"read":[],"write":false} {}`,
		"ops":        `{"v":1,"commands":[],"read":[],"write":false,"max_ops":10001}`,
		"ms mínimo":  `{"v":1,"commands":[],"read":[],"write":false,"max_ms":5}`,
		"hoja vacía": `{"v":1,"commands":[],"read":[],"write":false,"scope":{"sheets":[""]}}`,
	} {
		if _, err := ParseManifiesto([]byte(malo)); !errors.Is(err, ErrScriptInvalido) {
			t.Errorf("%s: aceptado (%v)", nombre, err)
		}
	}
}

// Los scripts tienen una lista de comandos más corta que las macros: lo que el registro no
// marca con `script: true` no se puede declarar, exista o no.
func TestManifiestoSoloAdmiteComandosDeScript(t *testing.T) {
	for _, c := range []string{
		"aifill", "iafill", "aiPanel", "share", "scriptsManage", "macroRecord", "protect", "unprotectbook",
		"undo", "redo", "presentar", "nameset", "extdata", "bloque", "versionSave", "mergeFinish", "noExiste",
	} {
		_, err := ParseManifiesto([]byte(`{"v":1,"commands":["bold","` + c + `"],"read":[],"write":true}`))
		if !errors.Is(err, ErrScriptInvalido) || !strings.Contains(err.Error(), c) {
			t.Errorf("%s: aceptado o sin nombrarlo en el error (%v)", c, err)
		}
	}
	for _, c := range []string{"bold", "paste", "insertLink", "sort", "insrow", "align"} {
		if _, err := ParseManifiesto([]byte(`{"v":1,"commands":["` + c + `"],"read":[],"write":true}`)); err != nil {
			t.Errorf("%s: rechazado (%v)", c, err)
		}
	}
	// El registro se lee de verdad: si la copia embebida perdiera las marcas, esto lo dice.
	if n := len(comandosDeScript()); n < 50 || n > 120 {
		t.Fatalf("comandos de script = %d: la lista ha cambiado de orden de magnitud", n)
	}
}

func TestCuotaDecide(t *testing.T) {
	if err := DecidirCuota(UsoCuota{}, 5000); err != nil {
		t.Fatal(err)
	}
	for nombre, uso := range map[string]UsoCuota{
		"usuario":  {EjecucionesUsuario: CuotaEjecucionesUsuarioHora},
		"org":      {EjecucionesOrg: CuotaEjecucionesOrgHora},
		"tiempo":   {MsUsuario: CuotaMsUsuarioHora - 4999},
		"de sobra": {EjecucionesUsuario: CuotaEjecucionesUsuarioHora + 9},
	} {
		if err := DecidirCuota(uso, 5000); !errors.Is(err, ErrCuotaScripts) {
			t.Errorf("%s: no se frena (%v)", nombre, err)
		}
	}
	if err := DecidirCuota(UsoCuota{MsUsuario: CuotaMsUsuarioHora - 5000}, 5000); err != nil {
		t.Errorf("el último hueco debe caber: %v", err)
	}
}

func TestPuedeEjecutarVersion(t *testing.T) {
	org := Script{AuthorSub: "ana", Scope: AmbitoOrg}
	priv := Script{AuthorSub: "ana", Scope: AmbitoPrivado}
	borrador := VersionScript{Status: EstadoBorrador}
	aprobada := VersionScript{Status: EstadoAprobada}
	if PuedeEjecutarVersion(org, borrador, "ana") != nil {
		t.Error("la autora debe poder probar su borrador")
	}
	if !errors.Is(PuedeEjecutarVersion(org, borrador, "beto"), ErrScriptSinAprobar) {
		t.Error("beto ejecuta un borrador")
	}
	if !errors.Is(PuedeEjecutarVersion(org, VersionScript{Status: EstadoPendiente}, "beto"), ErrScriptSinAprobar) {
		t.Error("beto ejecuta una pendiente")
	}
	if PuedeEjecutarVersion(org, aprobada, "beto") != nil {
		t.Error("beto no puede con una aprobada")
	}
	if !errors.Is(PuedeEjecutarVersion(priv, aprobada, "beto"), ErrScriptSinAprobar) {
		t.Error("un privado, aunque figure aprobado, no es de beto")
	}
}

func TestRolParaEjecutar(t *testing.T) {
	if RolParaEjecutar(ManifiestoScript{Write: true}) != RoleEditor || RolParaEjecutar(ManifiestoScript{}) != RoleViewer {
		t.Fatal("el rol mínimo no sigue al manifiesto")
	}
}

func TestHashDelCodigoEsSHA256(t *testing.T) {
	// SHA-256("abc")
	if got := HashCodigoScript("abc"); got != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Fatal(got)
	}
}
