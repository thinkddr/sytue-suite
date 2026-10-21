// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"os"

	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/drive/internal/config"
	"git.sytue.eu/sytue/one/services/drive/internal/service"
)

// runAvisosHistoricos es `drive avisos-historicos <organización>…`: deja en la campana,
// ya leídos y con su fecha, los avisos de «compartido contigo» que el Hub no llegó a guardar. Se
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
	return hubnotify.ReconstruirOrganizaciones(ctx, hubnotify.New(cfg.HubBaseURL, cfg.HubInternalSecret), os.Stdout, orgs,
		func(ctx context.Context) ([]hubnotify.Historico, error) {
			return service.AvisosHistoricos(ctx, pool, cfg.PublicURL)
		})
}
