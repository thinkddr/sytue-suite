// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ajustes

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestEscribirEnRaizNoSaleDelContenedor(t *testing.T) {
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := EscribirEnRaiz(root, "nested/data", []byte("encrypted artificial data")); err != nil {
		t.Fatal(err)
	}
	if err := EscribirEnRaiz(root, "../outside", []byte("escape")); err == nil {
		t.Fatal("write escaped root")
	}
	data, err := os.ReadFile(filepath.Join(dir, "nested", "data"))
	if err != nil || string(data) != "encrypted artificial data" {
		t.Fatal("atomic write failed", err)
	}
	entries, err := os.ReadDir(filepath.Join(dir, "nested"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "data" {
		t.Fatal("temporary retained")
	}
}

func TestEscribirEnRaizNoBorraTemporalRecreadoTrasRename(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	var temporal string
	err = escribirEnRaizConRenombre(root, "data", []byte("primero"), func(oldName, newName string) error {
		temporal = oldName
		if err := root.Rename(oldName, newName); err != nil {
			return err
		}
		return root.WriteFile(oldName, []byte("otro proceso"), 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := root.ReadFile(temporal)
	if err != nil || string(got) != "otro proceso" {
		t.Fatalf("temporal ajeno borrado o cambiado: %q, %v", got, err)
	}
}

// El openat con O_CREAT de darwin pierde la carrera contra un Rename en el mismo
// directorio y devuelve un ENOENT transitorio: fichero y directorio existen.
// Linux no lo hace. Visto con TestConcurrentAppOpenKeepsOneCacheKey en la caché
// nativa: dos apps abriendo a la vez fallaban con «no such file» sobre el cerrojo.
// Este test reproduce el churn (temporal + Rename) contra aperturas que crean.
// Sin los reintentos de AbrirCreandoEnRaiz falla en darwin en la primera tanda.
func TestAbrirCreandoEnRaizSobreviveAlChurnDeRenombres(t *testing.T) {
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	para := make(chan struct{})
	churn := &sync.WaitGroup{}
	churn.Go(func() { // el churn de EscribirEnRaiz: temporal + Rename + Remove
		for {
			select {
			case <-para:
				return
			default:
			}
			if err := EscribirEnRaiz(root, "churn.json", []byte("datos")); err != nil {
				t.Error(err)
				return
			}
			_ = root.Remove("churn.json")
		}
	})
	defer func() { close(para); churn.Wait() }()
	abridoras := &sync.WaitGroup{}
	for range 8 { // ocho abiertas concurrentes, como otras tantas apps
		abridoras.Go(func() {
			for range 25 {
				lock, err := AbrirCreandoEnRaiz(root, ".cerrojo-test", os.O_RDWR, 0o600)
				if err != nil {
					t.Error(err) // en darwin, sin reintentos: «openat .cerrojo-test: no such file or directory»
					return
				}
				lock.Close()
			}
		})
	}
	abridoras.Wait()
}
