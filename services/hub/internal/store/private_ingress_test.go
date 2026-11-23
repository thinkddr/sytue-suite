// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
)

// El aviso cifrado lleva exactamente las claves que lee el ejecutor de los aparatos
// (apps/nucleo/hub.IngressExecutor, que rechaza las desconocidas).
func TestAvisoCifradoTieneLasClavesDelAparato(t *testing.T) {
	raw, err := json.Marshal(AvisoCifrado{Version: 1, Tenant: "acme", ID: "x", Source: "drive", Type: "t", Level: "info", Title: "a", Body: "b", URL: "https://sytue.eu", Params: map[string]string{"k": "v"}, CreatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	var campos map[string]json.RawMessage
	if err := json.Unmarshal(raw, &campos); err != nil {
		t.Fatal(err)
	}
	claves := make([]string, 0, len(campos))
	for k := range campos {
		claves = append(claves, k)
	}
	slices.Sort(claves)
	quiero := []string{"body", "created_at", "id", "level", "params", "source", "tenant", "title", "type", "url", "v"}
	if !slices.Equal(claves, quiero) {
		t.Fatalf("claves %v, quiero %v", claves, quiero)
	}
	if ingress.MaxMetadataBytes < 16<<10 {
		t.Fatal("un aviso de tamaño máximo no cabría en los metadatos de la entrega")
	}
}

func TestPuedeSerDuena(t *testing.T) {
	for subject, quiero := range map[string]bool{
		"ana": true, "": false, "email:ana@acme.test": false, string(make([]byte, 1025)): false,
	} {
		if puedeSerDuena(subject) != quiero {
			t.Errorf("puedeSerDuena(%.20q) = %v", subject, !quiero)
		}
	}
}
