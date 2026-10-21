// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package catalogo

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	maxFotosBiblioteca = 120
	maxBytesBiblioteca = 24 << 20
	maxBytesFoto       = 350_000
)

type catalogoBiblioteca struct {
	Fotos []fotoBiblioteca `json:"fotos"`
}

type fotoBiblioteca struct {
	Fichero  string `json:"fichero"`
	SHA256   string `json:"sha256"`
	Licencia string `json:"licencia"`
	Fuente   string `json:"fuente"`
	Prueba   string `json:"prueba"`
}

func TestBibliotecaCC0TienePruebaYNoSuperaTopes(t *testing.T) {
	data, err := os.ReadFile("biblioteca.json")
	if err != nil {
		t.Fatal(err)
	}
	var catalogo catalogoBiblioteca
	if err := json.Unmarshal(data, &catalogo); err != nil {
		t.Fatal(err)
	}
	if len(catalogo.Fotos) < 40 || len(catalogo.Fotos) > maxFotosBiblioteca {
		t.Fatalf("fotos = %d, esperado entre 40 y %d", len(catalogo.Fotos), maxFotosBiblioteca)
	}
	var total int64
	for _, foto := range catalogo.Fotos {
		if !strings.HasSuffix(foto.Fichero, ".jpg") || (foto.Licencia != "CC0-1.0" && foto.Licencia != "PDM-1.0") || foto.Fuente == "" || foto.Prueba == "" {
			t.Fatalf("entrada de biblioteca inválida: %#v", foto)
		}
		data, err := os.ReadFile(filepath.Join("biblioteca", foto.Fichero))
		if err != nil {
			t.Fatal(err)
		}
		if len(data) > maxBytesFoto {
			t.Fatalf("%s ocupa %d bytes; máximo %d", foto.Fichero, len(data), maxBytesFoto)
		}
		suma := sha256.Sum256(data)
		if got := hex.EncodeToString(suma[:]); got != foto.SHA256 {
			t.Fatalf("%s: sha256 = %s, want %s", foto.Fichero, got, foto.SHA256)
		}
		total += int64(len(data))
	}
	if total > maxBytesBiblioteca {
		t.Fatalf("biblioteca ocupa %d bytes; máximo %d", total, maxBytesBiblioteca)
	}
}

func TestBibliotecaCubreTodasLasVerticales(t *testing.T) {
	cat, err := CargarPorDefecto()
	if err != nil {
		t.Fatalf("CargarPorDefecto() error = %v", err)
	}
	verticalesEsperadas := []string{
		"tienda", "fabrica", "producto", "restaurante", "servicios",
		"reformas", "clinica", "alojamiento", "academia", "portfolio",
	}
	conteos := make(map[string]int)
	for _, f := range cat.Fotos {
		if len(f.Verticales) == 0 {
			t.Errorf("foto %s no tiene verticales asignadas", f.ID)
		}
		for _, v := range f.Verticales {
			valida := false
			for _, esperada := range verticalesEsperadas {
				if v == esperada {
					valida = true
					break
				}
			}
			if !valida {
				t.Errorf("foto %s tiene vertical desconocida %q", f.ID, v)
			}
			conteos[v]++
		}
	}
	for _, v := range verticalesEsperadas {
		if c := conteos[v]; c < 3 {
			t.Errorf("vertical %q tiene %d fotos; want >= 3", v, c)
		}
	}
}
