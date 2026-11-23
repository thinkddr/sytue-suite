// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

// La marca «copia en curso» de la bóveda (ADR 0086, «pasa Matrix a ella»).
//
// Antes de escribir nada, la copia de Matrix a la bóveda pone en el servidor una
// marca con su identificador, la época del servidor y una caducidad corta, y la
// renueva mientras copia. Con ella puesta:
//
//   - nadie más publica en la bóveda ni la vacía (el servidor lo rechaza): la web
//     no importa encima la fuente del servidor mientras se copia, y enseña que
//     otro dispositivo está pasando el Drive a la bóveda;
//   - cualquiera ve que lo que hay en la bóveda sin recibo es una copia de Matrix,
//     aunque el diario de este dispositivo se pierda: Matrix sigue entero y la
//     copia se puede repetir vaciando la bóveda sin perder nada.
//
// Y vaciar va con lo que la copia vio con la marca ya puesta (vault_id y versión):
// si alguien publicó algo después, el servidor no vacía.

import (
	"context"
	"crypto/rand"
	"errors"
	"runtime"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

const (
	// vaultCopyRecord guarda en el diario de la bóveda (con el plan de la copia)
	// el identificador de la marca de este dispositivo.
	vaultCopyRecord = "copy"
	// VaultCopyTTL es cuánto vale la marca sin renovar. La copia la renueva cada
	// vaultCopyRenew; si se para, caduca pronto y deja de bloquear.
	VaultCopyTTL   = 5 * time.Minute
	vaultCopyRenew = time.Minute
)

// ErrVaultCopyInProgress: otro dispositivo está pasando su Drive de Matrix a la
// bóveda (tiene la marca activa), o la marca de esta copia se canceló. No se
// escribió nada; Matrix sigue entero.
var ErrVaultCopyInProgress = errors.New("drive: another device is copying into the vault")

// VaultNotEmptyError es ErrVaultNotEmpty con lo que hace falta para vaciar sin
// carreras: lo que la copia vio con su marca puesta (el servidor solo vacía si
// sigue así) y la marca con la que vaciar.
type VaultNotEmptyError struct {
	Expect api.PQVaultExpect
	CopyID string
	// Copy dice que lo que hay en la bóveda es, seguro, una copia de Matrix sin
	// recibo (el plan está en el diario, o la bóveda ya tenía la marca de una
	// copia de Matrix): vaciarla no pierde nada. Si no, la bóveda tiene a medias
	// otra cosa (una importación de la web) y no se vacía sola.
	Copy bool
}

func (e *VaultNotEmptyError) Error() string { return ErrVaultNotEmpty.Error() }

// Is hace que errors.Is(err, ErrVaultNotEmpty) siga valiendo.
func (e *VaultNotEmptyError) Is(target error) bool { return target == ErrVaultNotEmpty }

type vaultCopyMark struct {
	V     int           `json:"v"`
	Scope storage.Scope `json:"scope"`
	ID    string        `json:"id"`
}

// vaultCopy es la marca puesta por esta copia.
type vaultCopy struct {
	id string
	// prior es la marca que había antes de ponerla (nil si ninguna).
	prior *api.PQVaultCopy
	// seen es la bóveda tal como estaba con la marca ya puesta.
	seen api.PQVaultExpect
}

func vaultCopyHolder() string { return "native-" + runtime.GOOS }

// claimVaultCopy pone (o renueva) la marca de esta copia antes de leer la bóveda.
// El identificador vive en el diario: un corte reanuda con la misma marca.
func (r *Repository) claimVaultCopy(ctx context.Context, epoch string) (vaultCopy, error) {
	var mark vaultCopyMark
	found, err := r.cfg.Journal.Read(r.owner(), vaultMigrationNamespace, vaultCopyRecord, &mark)
	if err != nil {
		return vaultCopy{}, err
	}
	if found && (mark.V != 1 || mark.Scope != r.cfg.Scope || mark.ID == "") {
		return vaultCopy{}, ErrSource
	}
	before, err := r.cfg.Gateway.PQVaultKeys(ctx, r.cfg.Scope)
	if err != nil {
		return vaultCopy{}, err
	}
	if !found {
		mark = vaultCopyMark{V: 1, Scope: r.cfg.Scope, ID: rand.Text()}
	}
	state, err := r.cfg.Gateway.ClaimPQVaultCopy(ctx, r.cfg.Scope, api.PQVaultCopyClaim{ID: mark.ID, Holder: vaultCopyHolder(), Epoch: epoch, TTL: VaultCopyTTL})
	if api.IsPQVaultCode(err, api.CodePQVaultCopyInProgress) {
		return vaultCopy{}, ErrVaultCopyInProgress
	}
	if err != nil {
		return vaultCopy{}, err
	}
	if state.Copy == nil || state.Copy.ID != mark.ID {
		return vaultCopy{}, ErrSource
	}
	if !found {
		if err := r.cfg.Journal.Write(r.owner(), vaultMigrationNamespace, vaultCopyRecord, mark); err != nil {
			return vaultCopy{}, err
		}
	}
	return vaultCopy{id: mark.ID, prior: before.Copy, seen: api.PQVaultExpect{VaultID: state.VaultID, Version: state.Version}}, nil
}

// keepVaultCopy renueva la marca mientras dura la copia. Si no se puede (se
// canceló, o la red), la copia lo nota en su siguiente escritura, que el servidor
// rechaza sin la marca.
func (r *Repository) keepVaultCopy(ctx context.Context, mark vaultCopy, epoch string) func() {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(vaultCopyRenew)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_, _ = r.cfg.Gateway.ClaimPQVaultCopy(ctx, r.cfg.Scope, api.PQVaultCopyClaim{ID: mark.id, Holder: vaultCopyHolder(), Epoch: epoch, TTL: VaultCopyTTL})
			}
		}
	}()
	return func() { cancel(); <-done }
}

// releaseVaultCopy retira la marca (si sigue siendo la suya) y la olvida. Si el
// servidor no contesta, la deja en el diario: la cuenta sigue en Matrix y el
// siguiente intento la retira.
func (r *Repository) releaseVaultCopy(ctx context.Context, mark vaultCopy) error {
	_, err := r.cfg.Gateway.ReleasePQVaultCopy(ctx, r.cfg.Scope, mark.id)
	if err != nil && !api.IsPQVaultCode(err, api.CodePQVaultChanged) {
		return err
	}
	return r.cfg.Journal.Delete(r.owner(), vaultMigrationNamespace, vaultCopyRecord)
}

func (r *Repository) vaultNotEmpty(mark vaultCopy, isCopy bool) error {
	return &VaultNotEmptyError{Expect: mark.seen, CopyID: mark.id, Copy: isCopy}
}
