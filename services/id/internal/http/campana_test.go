// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// Toda superficie de la suite que se despliega con su client OIDC pinta la campana, y
// tiene que recibir el claim `mailboxes` (ADR 0053): una app nueva que no se añada a
// clientesConCampana vería solo los avisos de la organización de la sesión, sin que
// nada fallara. Y nada que no sea de la suite se cuela: la forja no está.
func TestClientesConCampanaSonLosDeLaSuite(t *testing.T) {
	re := regexp.MustCompile(`OIDC_CLIENT_ID:\s*"?(sytue-[a-z-]+)`)
	vistos := 0
	err := filepath.WalkDir("../../../../deploy", func(ruta string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(ruta)
		if err != nil {
			return err
		}
		for _, m := range re.FindAllStringSubmatch(string(b), -1) {
			vistos++
			if !clientesConCampana[m[1]] {
				t.Errorf("%s despliega %s, que no está en clientesConCampana", ruta, m[1])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if vistos == 0 {
		t.Fatal("no se encontró ningún OIDC_CLIENT_ID en deploy/: ¿se movió?")
	}
	if clientesConCampana["sytue-git"] || clientesConCampana["sytue-gateway"] {
		t.Fatal("un client que no es una superficie de la suite recibe mailboxes")
	}
}
