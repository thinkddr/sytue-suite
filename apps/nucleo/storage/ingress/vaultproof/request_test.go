// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package vaultproof

import (
	"encoding/json"
	"errors"
	"testing"
)

// El cuerpo que manda el módulo de correo (webport.Storage.SignIngressAction) se
// lee tal cual y lo firmado verifica.
func TestVaultproofCuerpoDelPuerto(t *testing.T) {
	s, reg := firmante(t, 1)
	a := accion(t, reg, "key.enroll")
	i := a.Intent()
	cuerpo, err := json.Marshal(SignRequest{Expected: IntentWire{i.Generation, i.Scope, i.Epoch, i.Kind, i.Target, i.RequestSHA256}, Action: a})
	if err != nil {
		t.Fatal(err)
	}
	req, err := DecodeSignRequest(cuerpo)
	if err != nil {
		t.Fatal(err)
	}
	prueba, err := SignIntent(s, req.Expected.Intent(), req.Action, ahora)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(a, prueba, reg, ahora); err != nil {
		t.Fatal(err)
	}
	for nombre, malo := range map[string]string{
		"campo de más":   `{"expected":{},"action":{},"otro":1}`,
		"basura detrás":  string(cuerpo) + `{}`,
		"no es un JSON":  `x`,
		"campo anidado ": `{"expected":{"raro":1},"action":{}}`,
	} {
		if _, err := DecodeSignRequest([]byte(malo)); !errors.Is(err, ErrUnverified) {
			t.Errorf("%s: %v", nombre, err)
		}
	}
}
