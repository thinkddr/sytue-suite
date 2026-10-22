// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package config

import (
	"strings"
	"testing"
	"time"
)

// entornoMinimo deja lo obligatorio para que Load arranque en desarrollo.
func entornoMinimo(t *testing.T) {
	t.Helper()
	t.Setenv("ENV", "dev")
	t.Setenv("DATABASE_URL", "postgres://x@127.0.0.1/cast")
	t.Setenv("S3_ACCESS_KEY", "a")
	t.Setenv("S3_SECRET_KEY", "b")
	for _, v := range []string{"CAST_SWEEP_INTERVAL", "CAST_SWEEP_DELETED_GRACE", "CAST_SWEEP_STALE_UPLOAD"} {
		t.Setenv(v, "")
	}
}

func TestBarridoPorDefecto(t *testing.T) {
	entornoMinimo(t)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SweepInterval != time.Hour || cfg.SweepDeletedGrace != 7*24*time.Hour || cfg.SweepStaleUpload != 6*time.Hour {
		t.Fatalf("barrido = cada %s, gracia %s, subida %s", cfg.SweepInterval, cfg.SweepDeletedGrace, cfg.SweepStaleUpload)
	}
}

func TestBarridoConfigurable(t *testing.T) {
	entornoMinimo(t)
	t.Setenv("CAST_SWEEP_INTERVAL", "15m")
	t.Setenv("CAST_SWEEP_DELETED_GRACE", "48h")
	t.Setenv("CAST_SWEEP_STALE_UPLOAD", "12h")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SweepInterval != 15*time.Minute || cfg.SweepDeletedGrace != 48*time.Hour || cfg.SweepStaleUpload != 12*time.Hour {
		t.Fatalf("barrido = cada %s, gracia %s, subida %s", cfg.SweepInterval, cfg.SweepDeletedGrace, cfg.SweepStaleUpload)
	}
}

// Un plazo de subida por debajo de la copia desde Drive daría por fallida una
// copia en marcha: no arranca. Con el barrido apagado da igual.
func TestBarridoRechazaUnPlazoDeSubidaCorto(t *testing.T) {
	entornoMinimo(t)
	t.Setenv("CAST_SWEEP_STALE_UPLOAD", "1h")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "CAST_SWEEP_STALE_UPLOAD") {
		t.Fatalf("Load = %v; quiero el error del plazo de subida", err)
	}
	t.Setenv("CAST_SWEEP_INTERVAL", "0")
	if _, err := Load(); err != nil {
		t.Fatalf("con el barrido apagado: %v", err)
	}
}

func TestBarridoDuracionIlegible(t *testing.T) {
	entornoMinimo(t)
	t.Setenv("CAST_SWEEP_DELETED_GRACE", "7 días")
	if _, err := Load(); err == nil {
		t.Fatal("una duración ilegible tiene que impedir el arranque")
	}
}
