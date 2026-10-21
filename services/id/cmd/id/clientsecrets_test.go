// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// Un Secret de Kubernetes montado NO es un directorio plano de ficheros: kubelet
// crea `..data` (enlace al directorio de la versión vigente), un directorio
// `..2026_08_03_...` con el contenido real y un enlace por clave. Si la lectura
// no ignora las entradas que empiezan por punto, `..data` se toma por un
// client_id y la reconciliación aborta con «clients inexistentes».
func TestClientSecretsFromDirIgnoraLaEstructuraDeKubernetes(t *testing.T) {
	dir := t.TempDir()
	versionado := filepath.Join(dir, "..2026_08_03_00_00_00.123456789")
	if err := os.Mkdir(versionado, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, secret := range map[string]string{
		"sytue-console": "secreto-console\n",
		"sytue-chat":    "secreto-rooms",
	} {
		if err := os.WriteFile(filepath.Join(versionado, name), []byte(secret), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join("..data", name), filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(versionado, filepath.Join(dir, "..data")); err != nil {
		t.Fatal(err)
	}

	secrets, err := clientSecretsFromDir(dir)
	if err != nil {
		t.Fatalf("lectura: %v", err)
	}
	if len(secrets) != 2 {
		t.Fatalf("se esperaban 2 clients, se leyeron %d: %v", len(secrets), secrets)
	}
	// El salto de línea final NO forma parte del secreto: el BFF envía el valor
	// tal cual lo recibe de Kubernetes, sin él, y compararlo con el hash de un
	// valor con "\n" fallaría con invalid_client.
	if secrets["sytue-console"] != "secreto-console" {
		t.Errorf("no se recortó el salto de línea: %q", secrets["sytue-console"])
	}
	if secrets["sytue-chat"] != "secreto-rooms" {
		t.Errorf("secreto inesperado: %q", secrets["sytue-chat"])
	}
}

func TestClientSecretsFromDirRechazaEntradasInservibles(t *testing.T) {
	t.Run("secreto vacío", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "sytue-console"), []byte("\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		// Un secreto vacío dejaría el client deshabilitado en silencio, que es
		// exactamente el fallo que este subcomando existe para impedir.
		if _, err := clientSecretsFromDir(dir); err == nil {
			t.Fatal("se aceptó un secreto vacío")
		}
	})

	t.Run("directorio sin secretos", func(t *testing.T) {
		if _, err := clientSecretsFromDir(t.TempDir()); err == nil {
			t.Fatal("se aceptó un directorio sin ningún secreto")
		}
	})

	t.Run("directorio inexistente", func(t *testing.T) {
		if _, err := clientSecretsFromDir(filepath.Join(t.TempDir(), "no-existe")); err == nil {
			t.Fatal("se aceptó un directorio inexistente")
		}
	})
}
