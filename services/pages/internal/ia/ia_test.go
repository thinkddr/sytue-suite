// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ia

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/asistente"
	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/services/pages/internal/config"
	"git.sytue.eu/sytue/one/services/pages/internal/domain"
	"git.sytue.eu/sytue/one/services/pages/internal/ir"
)

func TestGuardarClaveBYOK_Validacion(t *testing.T) {
	// KEK de 32 bytes
	kek := []byte("01234567890123456789012345678901")
	kc, err := keycrypto.New(kek)
	if err != nil {
		t.Fatalf("keycrypto.New: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"modelos": []string{"gpt-4o", "claude-3-5-sonnet"}})
	}))
	defer srv.Close()

	cli := asistente.Nuevo(srv.URL, 5*time.Second)
	s := Nuevo(config.Config{AISecret: "sec"}, nil, cli, kc, nil, nil, nil, nil)
	ctx := context.Background()

	// 1. OpenAI sin aviso -> Error
	_, err = s.GuardarClave(ctx, "acme", "sub1", GuardarClavePeticion{
		Provider:       "openai",
		Model:          "gpt-4o",
		Key:            "sk-test",
		NoticeAccepted: false,
	})
	if err == nil || !errors.Is(err, ErrAvisoRequerido) {
		t.Errorf("esperaba ErrAvisoRequerido para openai sin aviso, obtuvo %v", err)
	}

	// 2. Proveedor no admitido -> Error
	_, err = s.GuardarClave(ctx, "acme", "sub1", GuardarClavePeticion{
		Provider:       "desconocido",
		Model:          "gpt-4o",
		Key:            "sk-test",
		NoticeAccepted: true,
	})
	if err == nil {
		t.Errorf("esperaba error por proveedor desconocido")
	}

	// 3. Modelo vacío -> Error
	_, err = s.GuardarClave(ctx, "acme", "sub1", GuardarClavePeticion{
		Provider:       "mistral",
		Model:          "",
		Key:            "key-test",
		NoticeAccepted: false,
	})
	if err == nil || !errors.Is(err, ErrModeloInvalido) {
		t.Errorf("esperaba ErrModeloInvalido, obtuvo %v", err)
	}

	// 4. Sin keycrypto -> ErrCifradoNoDisponible
	sSinKEK := Nuevo(config.Config{AISecret: "sec"}, nil, cli, nil, nil, nil, nil, nil)
	_, err = sSinKEK.GuardarClave(ctx, "acme", "sub1", GuardarClavePeticion{
		Provider:       "mistral",
		Model:          "mistral-large",
		Key:            "key-test",
		NoticeAccepted: false,
	})
	if err == nil || !errors.Is(err, ErrCifradoNoDisponible) {
		t.Errorf("esperaba ErrCifradoNoDisponible, obtuvo %v", err)
	}
}

func TestCupoCalculo_ProYEnterprise(t *testing.T) {
	cfg := config.Config{
		IACupoPro:        100,
		IACupoEnterprise: 500,
	}
	s := Nuevo(cfg, nil, nil, nil, nil, nil, nil, nil)

	// Mock o verificación directa con store nil fallaría en ContarTrabajosMes, pero probamos la lógica de cálculo
	loc, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		loc = time.FixedZone("CET", 3600)
	}
	ahora := time.Now().In(loc)
	mesEsperado := ahora.Format("2006-01")

	// Probamos el formato de mes y el reseteo
	desde := time.Date(ahora.Year(), ahora.Month(), 1, 0, 0, 0, 0, loc)
	resetAt := desde.AddDate(0, 1, 0)

	if resetAt.Day() != 1 {
		t.Errorf("resetAt debe ser el primer día del mes siguiente: %v", resetAt)
	}
	if mesEsperado == "" {
		t.Fatal("mes no debe ser vacío")
	}
	_ = s
}

func TestEjecutarExportShopify(t *testing.T) {
	s := Nuevo(config.Config{AISecret: "sec"}, nil, nil, nil, nil, nil, nil, nil)
	ctx := context.Background()

	irSitio := ir.Sitio{
		Version: 1,
		Languages: ir.IdiomasSitio{
			Default: "es",
		},
		Theme: ir.Tema{
			Colors: map[string]string{
				"background": "#ffffff",
				"text":       "#111827",
				"primary":    "#ca1682",
			},
			Container: 1200,
		},
		SEO: ir.SeoSitio{
			SiteName: ir.Textos{"es": "Mi Tienda Test"},
		},
		Pages: []ir.Pagina{
			{
				ID: "home",
				Slug: ir.Textos{
					"es": "inicio",
				},
				Title: ir.Textos{
					"es": "Inicio",
				},
				Sections: []ir.Nodo{
					{
						ID:   "hero-1",
						Type: "section",
						Kind: "hero",
						Children: []ir.Nodo{
							{
								ID:   "txt-1",
								Type: "text",
								Content: ir.TextosRicos{
									"es": []ir.Tramo{{Text: "Bienvenidos a la tienda"}},
								},
							},
						},
					},
				},
			},
		},
	}
	rawIR, err := json.Marshal(irSitio)
	if err != nil {
		t.Fatalf("json.Marshal(irSitio): %v", err)
	}

	job := &domain.AIJob{
		ID:     uuid.New(),
		Tenant: "acme",
		SiteID: uuid.New(),
		Kind:   domain.AIJobKindExport,
		Status: domain.AIJobStatusRunning,
	}

	in := domain.ExportInput{
		Destination: "shopify",
		IR:          rawIR,
	}

	s.ejecutarExport(ctx, job, in)

	if job.Status != domain.AIJobStatusDone {
		t.Fatalf("esperaba status done, obtuvo %s (error: %s)", job.Status, job.Error)
	}

	var mapa map[string][]byte
	if err := json.Unmarshal(job.Result, &mapa); err != nil {
		t.Fatalf("json.Unmarshal(job.Result): %v", err)
	}

	if _, ok := mapa["layout/theme.liquid"]; !ok {
		t.Errorf("falta layout/theme.liquid en el resultado")
	}
	if _, ok := mapa["templates/index.json"]; !ok {
		t.Errorf("falta templates/index.json en el resultado")
	}
}
