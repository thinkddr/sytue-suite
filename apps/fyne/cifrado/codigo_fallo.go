// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cifrado

// El código corto de por qué el Drive (o la bóveda) no abrió, el mismo que la web
// enseña en «Más información» (`vaultFailureCode`): va debajo del aviso genérico y
// al registro, para saber en producción qué comprobación lo rechazó.
//
// Solo lleva lo que es fijo en el código: el error centinela, la comprobación de la
// importación (drive.ImportCheck), el estado HTTP y el código de error del
// servicio. Nunca mensajes, URLs, nombres ni identificadores de la persona.

import (
	"context"
	"errors"
	"log/slog"
	"regexp"
	"strconv"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	privateDrive "git.sytue.eu/sytue/one/apps/nucleo/drive"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqvault"
)

var codigoServicio = regexp.MustCompile(`^[a-z0-9_]{1,64}$`)

var centinelas = []struct {
	err    error
	codigo string
}{
	{privateDrive.ErrNotReady, "drive.not_ready"},
	{privateDrive.ErrConflict, "drive.conflict"},
	{privateDrive.ErrSource, "drive.source"},
	{privateDrive.ErrClosed, "drive.closed"},
	{privateDrive.ErrVaultCopyInProgress, "drive.copy_in_progress"},
	{privateDrive.ErrVaultNotEmpty, "drive.vault_not_empty"},
	{pqvault.ErrSignin, "vault.signin"},
	{pqvault.ErrWrong, "vault.wrong"},
	{pqvault.ErrIdentity, "vault.identity"},
	{pqvault.ErrClosed, "vault.closed"},
	{pqvault.ErrChanged, "vault.changed"},
	{pqvault.ErrCopyInProgress, "vault.copy"},
	{auth.ErrAuthorizationChanged, "auth.changed"},
	{auth.ErrLoginRequired, "auth.login"},
	{offline.ErrWorkPaused, "offline.paused"},
	{offline.ErrEncryptedStore, "offline.store"},
	{mx.ErrRecoveryRequired, "matrix.recovery"},
	{ErrPrivateResources, "resources"},
	{storage.ErrRecord, "record"},
	{context.Canceled, "canceled"},
	{context.DeadlineExceeded, "timeout"},
}

// CodigoFallo es el código corto de `err` (vacío sin error).
func CodigoFallo(err error) string {
	if err == nil {
		return ""
	}
	if check := privateDrive.ImportCheck(err); check != "" {
		return "drive.import." + check
	}
	for _, c := range centinelas {
		if errors.Is(err, c.err) {
			return c.codigo
		}
	}
	var http *api.Error
	if errors.As(err, &http) {
		codigo := "http." + strconv.Itoa(http.Status)
		if code := http.Code(); codigoServicio.MatchString(code) {
			codigo += "." + code
		}
		return codigo
	}
	return "other"
}

// avisarFallo deja en el registro el código de un fallo que la pantalla enseña.
func avisarFallo(donde string, err error) string {
	codigo := CodigoFallo(err)
	if codigo != "" {
		slog.Warn("cifrado: "+donde+" no abrió", "codigo", codigo)
	}
	return codigo
}
