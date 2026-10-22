// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"errors"
	"os"

	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/peticiones"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/docs/internal/config"
	dochttp "git.sytue.eu/sytue/one/services/docs/internal/http"
)

// runAvisosHistoricos es `docs avisos-historicos <organización>…`: deja en la campana,
// ya leídos y con su fecha, los avisos de compartidos, comentarios y menciones que el Hub no llegó a guardar. Se
// lanza a mano (kubectl exec en el pod, que ya tiene la configuración); repetirlo no
// duplica nada. Ver hubnotify.Historico.
func runAvisosHistoricos(orgs []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx := context.Background()
	pool, err := pg.Connect(ctx, pg.Config{DSN: cfg.DatabaseURL, MaxConns: 2})
	if err != nil {
		return err
	}
	defer pool.Close()
	if cfg.IDDirectorioSecret == "" {
		// Sin Sytue ID no se sabe quién sigue en la organización: nada se reconstruye.
		return errors.New("falta SYTUE_DOCS_ID_DIRECTORIO_SECRET (Secret sytue-id-directorio): sin él no se sabe quién puede abrir hoy cada documento")
	}
	return hubnotify.ReconstruirOrganizaciones(ctx, hubnotify.New(cfg.HubBaseURL, cfg.HubInternalSecret), os.Stdout, orgs,
		func(ctx context.Context) ([]hubnotify.Historico, error) {
			t := tenancy.MustFrom(ctx)
			resolver := func(ctx context.Context, c peticiones.ConsultaDirectorio) (peticiones.Resueltas, error) {
				return peticiones.PersonasDeOrg(ctx, cfg.IDBaseURL, cfg.IDDirectorioSecret, t.Slug, c)
			}
			return dochttp.AvisosHistoricos(ctx, pool, cfg.PublicURL, resolver)
		})
}
