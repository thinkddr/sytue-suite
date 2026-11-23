// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package storage

// PQVaultTarget es el destino de publicación de una bóveda post-cuántica (ADR
// 0086/0087): su sala es la propia bóveda y su «dispositivo», `pq-` y los 24
// primeros caracteres del identificador de su clave. Es lo que usan la web
// (`PqVault.publicationTarget`), el nativo (`pqvault.Vault.PublicationTarget`) y
// lo que Drive comprueba en un recibo del correo. Con un identificador demasiado
// corto devuelve un destino inválido.
func PQVaultTarget(vaultID, keyID string) PublicationTarget {
	if len(keyID) < 24 {
		return PublicationTarget{}
	}
	return PublicationTarget{RoomID: vaultID, DeviceID: "pq-" + keyID[:24]}
}
