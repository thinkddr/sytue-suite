// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"git.sytue.eu/sytue/one/platform/pkg/callaccess"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/merge"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// Renombrar el SLUG de una organización — OPERACIÓN DE PLATAFORMA.
//
// El slug NO es una etiqueta: es la identidad del tenant en toda la suite. Es la
// columna `tenant` de cada tabla de las nueve bases de servicio, la dirección
// pública `<slug>.sytue.eu`, el namespace (`tenant-<slug>`) y el prefijo de
// almacenamiento. Por eso `admin_tenants.go` lo declara INMUTABLE desde la
// Console: un `UPDATE organizations SET slug` a secas dejaría cada correo, cada
// fichero y cada evento apuntando a un tenant que ya no existe, invisibles para
// siempre bajo RLS y sin ningún error.
//
// Esto reutiliza el motor de fusión, que ya resuelve exactamente ese problema:
// reescribe el tenant en las nueve bases dentro de una transacción por servicio y
// bajo `app.merge`, con el preflight que garantiza que ninguna tabla se quede
// muda. La diferencia con una fusión es que aquí no hay dos organizaciones: la
// fila sobrevive, solo cambia su nombre.
//
//	id rename-org <slug-actual> <slug-nuevo>
func runRenameOrg(args []string) int {
	if len(args) != 2 {
		fmt.Fprintln(os.Stderr, "uso: id rename-org <slug-actual> <slug-nuevo>")
		return 2
	}
	from, to := args[0], args[1]
	dsn := os.Getenv("SYTUE_ID_DATABASE_DSN")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "falta SYTUE_ID_DATABASE_DSN")
		return 2
	}

	ctx := context.Background()
	pool, err := pg.Connect(ctx, pg.Config{DSN: dsn, MaxConns: 2})
	if err != nil {
		fmt.Fprintf(os.Stderr, "conexión: %v\n", err)
		return 1
	}
	defer pool.Close()

	movidas, err := renameOrg(ctx, store.New(pool), dsn, from, to)
	switch {
	case errors.Is(err, errRenameYaHecho):
		fmt.Printf("nada que hacer: %q ya no existe y %q sí\n", from, to)
		return 0
	case err != nil:
		fmt.Fprintf(os.Stderr, "renombrar organización: %v\n", err)
		return 1
	}
	fmt.Printf("organización renombrada: %s → %s (%d filas de servicio reetiquetadas)\n",
		from, to, movidas)
	fmt.Printf("recuerda que su dirección pública pasa a ser https://%s.<dominio>/\n", to)
	return 0
}

// errRenameYaHecho distingue «ya estaba renombrada» de un fallo: reejecutar la
// operación tras un éxito no debe devolver error (idempotencia).
var errRenameYaHecho = errors.New("rename-org: ya aplicado")

// renameOrg reetiqueta primero los DATOS y solo después la identidad.
//
// El orden importa y es el contrario al intuitivo: mientras la fila conserve el
// slug antiguo, un fallo a mitad deja datos ya movidos al slug nuevo —invisibles
// hasta terminar— pero la organización sigue funcionando con lo que queda y la
// operación se puede repetir. Al revés, renombrar primero la fila dejaría a
// TODOS los usuarios mirando un tenant vacío mientras sus datos siguen bajo el
// nombre viejo.
func renameOrg(ctx context.Context, st *store.Store, idDSN, from, to string) (int, error) {
	if err := configureCommandCallIdentity(ctx, st); err != nil {
		return 0, err
	}
	if from == to {
		return 0, errors.New("el slug de origen y el de destino son el mismo")
	}
	if _, err := st.GetOrgBySlug(ctx, from); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			// Si el destino ya existe, el renombrado se hizo antes: no es un fallo.
			if _, e := st.GetOrgBySlug(ctx, to); e == nil {
				return 0, errRenameYaHecho
			}
			return 0, fmt.Errorf("no existe ninguna organización con el slug %q", from)
		}
		return 0, err
	}
	// El destino DEBE estar libre. Si estuviera ocupado esto no sería un
	// renombrado sino una fusión, que tiene sus propias reglas (qué rol conserva
	// cada miembro, qué pasa con las colisiones) y su propia API.
	if _, err := st.GetOrgBySlug(ctx, to); err == nil {
		return 0, fmt.Errorf("ya existe una organización con el slug %q: eso sería una fusión, no un renombrado", to)
	} else if !errors.Is(err, domain.ErrNotFound) {
		return 0, err
	}

	if err := merge.Preflight(ctx, idDSN); err != nil {
		return 0, err
	}
	movidas := 0
	for _, svc := range merge.Services {
		svcDSN, err := merge.DSNForDatabase(idDSN, svc)
		if err != nil {
			return movidas, err
		}
		svcPool, err := pg.Connect(ctx, pg.Config{DSN: svcDSN, MaxConns: 2})
		if err != nil {
			return movidas, err
		}
		res, err := merge.MoveService(ctx, svcPool, from, to, renameDocumentCallRevoker())
		svcPool.Close()
		if err != nil {
			return movidas, fmt.Errorf("%s: %w", svc, err)
		}
		movidas += res.Total
		fmt.Printf("  · %-13s %d filas\n", svc, res.Total)
	}

	const sql = `UPDATE organizations SET slug = $2, updated_at = now() WHERE slug = $1`
	if err := st.InTx(ctx, func(q pg.Querier) error { _, err := q.Exec(ctx, sql, from, to); return err }); err != nil {
		return movidas, fmt.Errorf("actualizar el slug: %w", err)
	}
	return movidas, nil
}

func renameDocumentCallRevoker() callaccess.Revoker {
	base := os.Getenv("SYTUE_ID_ROOMS_CALL_ACCESS_URL")
	if base == "" {
		base = "http://rooms:8085"
	}
	client := callaccess.New(base, os.Getenv("SYTUE_ID_ROOMS_CALL_ACCESS_SECRET"))
	if client == nil {
		return nil
	}
	return client
}
