// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/callaccess"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/merge"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// Renombrar el SLUG de una organización — OPERACIÓN DE PLATAFORMA.
//
// El slug NO es una etiqueta: es la identidad del tenant en toda la suite. Es la
// columna `tenant` de las tablas de las bases de servicio, la dirección
// pública `<slug>.sytue.eu`, el namespace (`tenant-<slug>`) y el prefijo de
// almacenamiento. Por eso `admin_tenants.go` lo declara INMUTABLE desde la
// Console: un `UPDATE organizations SET slug` a secas dejaría cada correo, cada
// fichero y cada evento apuntando a un tenant que ya no existe, invisibles para
// siempre bajo RLS y sin ningún error.
//
// Esto reutiliza el motor de fusión, que ya resuelve exactamente ese problema:
// reescribe el tenant dentro de una transacción por servicio y
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
// La intención y ambos slugs se conservan antes de mover datos. La admisión
// queda cerrada hasta resolver la identidad; todos los pasos usan las mismas
// sesiones físicas que mantienen la exclusión con las fusiones de plataforma.
func renameOrg(ctx context.Context, st *store.Store, idDSN, from, to string) (moved int, resultErr error) {
	if !tenancy.ValidSlug(from) || !tenancy.ValidSlug(to) || from == to {
		return 0, errors.New("el origen y el destino deben ser slugs válidos distintos")
	}
	intent, err := st.FindOrgRename(ctx, from, to)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return 0, err
	}
	jobID := intent.ID
	if jobID == "" {
		jobID = uuid.NewString()
	}
	owner, err := merge.AcquireOwnership(ctx, idDSN, jobID)
	if err != nil {
		return 0, err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		closeErr := owner.Close(cleanup)
		if closeErr != nil && errors.Is(resultErr, errRenameYaHecho) {
			resultErr = closeErr
		} else {
			resultErr = errors.Join(resultErr, closeErr)
		}
	}()
	return renameOrgOwned(ctx, st.WithDatabase(owner.Identity()), owner, from, to)
}

func renameOrgOwned(ctx context.Context, owned *store.Store, owner *merge.Ownership, from, to string) (moved int, resultErr error) {
	if err := configureCommandCallIdentity(ctx, owned); err != nil {
		return 0, err
	}
	intent, err := owned.PrepareOrgRename(ctx, owner.JobID(), from, to)
	if err != nil {
		return 0, err
	}
	committed, err := owned.OrgRenameIdentityOutcome(ctx, intent.ID)
	if err != nil {
		return 0, err
	}
	if committed {
		if err := merge.CompleteOwnedAdmission(ctx, owner, from, to); err != nil {
			return 0, err
		}
		if err := owned.CompleteOrgRename(ctx, intent.ID); err != nil {
			return 0, err
		}
		return 0, errRenameYaHecho
	}
	admission, err := merge.PrepareOwnedAdmission(ctx, owner, from, to)
	if err != nil {
		return 0, err
	}
	defer func() {
		if resultErr == nil {
			return
		}
		resultErr = settleFailedRename(ctx, owned, admission, intent.ID, resultErr)
	}()
	for _, svc := range merge.Services {
		res, err := merge.MoveService(ctx, owner.Service(svc), from, to, renameDocumentCallRevoker())
		if err != nil {
			return moved, fmt.Errorf("%s: %w", svc, err)
		}
		moved += res.Total
		fmt.Printf("  · %-13s %d filas\n", svc, res.Total)
	}
	leftovers, err := merge.VerifyOwned(ctx, owner, from)
	if err != nil {
		return moved, err
	}
	if len(leftovers) > 0 {
		return moved, merge.LeftoversError(leftovers)
	}
	if err := owned.CommitOrgRename(ctx, intent); err != nil {
		return moved, err
	}
	if err := admission.Complete(ctx); err != nil {
		return moved, err
	}
	return moved, owned.CompleteOrgRename(ctx, intent.ID)
}

// Observe the retained identity on the SAME owned connection. If it died, no
// replacement pool may reopen the source while old sessions still live.
func settleFailedRename(ctx context.Context, owned *store.Store, admission *merge.Admission, id string, cause error) error {
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	committed, err := owned.OrgRenameIdentityOutcome(cleanup, id)
	if err != nil {
		return errors.Join(cause, fmt.Errorf("rename: identity outcome unknown; admission remains closed: %w", err))
	}
	if committed {
		err = admission.Complete(cleanup)
	} else {
		err = admission.Abort(cleanup)
	}
	return errors.Join(cause, err)
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
