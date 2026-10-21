// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/docs/internal/config"
	"git.sytue.eu/sytue/one/services/docs/internal/drive"
	dochttp "git.sytue.eu/sytue/one/services/docs/internal/http"
	"git.sytue.eu/sytue/one/services/docs/internal/store"
)

// runReconciliarPapelera es `docs reconciliar-papelera [--aplicar] <organización>…`:
// los documentos que se borraron en Docs antes de que las dos papeleras fueran una
// dejaron su .odt/.ods/.odp vivo en Drive. Sin `--aplicar` solo CUENTA (Drive dice
// qué haría sin hacerlo); con él, manda esos ficheros a la papelera de Drive. Nunca
// purga nada y solo mueve ficheros del dueño del documento. Repetirlo no hace nada
// nuevo: lo ya movido sale como «ya estaban». Runbook: docs/operations.md
// #reconciliar-papelera.
func runReconciliarPapelera(args []string) error {
	aplicar := false
	var orgs []string
	for _, a := range args {
		if a == "--aplicar" {
			aplicar = true
			continue
		}
		if strings.HasPrefix(a, "-") {
			return fmt.Errorf("opción desconocida %q (solo --aplicar)", a)
		}
		orgs = append(orgs, a)
	}
	if len(orgs) == 0 {
		return errors.New("uso: docs reconciliar-papelera [--aplicar] <organización>…")
	}
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
	// El aislamiento por organización se decide igual que al servir: sin él, no se
	// toca nada.
	st := store.New(pool)
	if _, err := st.ResolveAppRole(ctx); err != nil {
		return fmt.Errorf("aislamiento por organización: %w", err)
	}
	h := dochttp.New(dochttp.Deps{
		Store: st,
		Drive: drive.Nuevo(cfg.DriveURL, cfg.DriveTimeout).ConSecretoInterno(cfg.DriveInternalSecret),
	})
	modo := "EN SECO (no se mueve nada)"
	if aplicar {
		modo = "APLICANDO (a la papelera de Drive, sin purgar)"
	}
	fmt.Fprintln(os.Stdout, "reconciliar-papelera:", modo)
	fmt.Fprintln(os.Stdout, "organización\tcandidatos\ta_papelera\tya_estaban\tmodificados\tno_aplica\terrores")
	var fallo error
	for _, org := range orgs {
		r, err := h.ReconciliarPapelera(ctx, org, aplicar)
		if err != nil {
			fmt.Fprintf(os.Stdout, "%s\tERROR: %v\n", org, err)
			fallo = errors.New("alguna organización no se pudo reconciliar")
			continue
		}
		nota := ""
		if r.Candidatos == 0 {
			// Docs no tiene lista de organizaciones: una con erratas sale a cero.
			nota = "\t(ninguno: ¿está bien escrita la organización?)"
		}
		fmt.Fprintf(os.Stdout, "%s\t%d\t%d\t%d\t%d\t%d\t%d%s\n", org, r.Candidatos, r.APapelera, r.YaEstaban,
			r.Modificados, r.NoAplica, r.Errores, nota)
		if r.Errores > 0 {
			fallo = errors.New("hubo errores: relánzalo (lo ya movido sale como «ya estaban»)")
		}
	}
	return fallo
}
