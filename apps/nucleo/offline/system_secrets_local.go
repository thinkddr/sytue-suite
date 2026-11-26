// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//go:build !js && edicionlocal

package offline

// En Sytue Office (`edicionlocal`) no hay llavero de la suite: no hay cuenta, ni sesión,
// ni nada cifrado con su clave (PLAN-EDICION-LOCAL §2). La caché cifrada de la suite
// la abren solo Open, OpenContext y RelocateJournalContext, y solo los llama la sesión
// de la suite (`apps/fyne/sesion`), que no entra en sus binarios salvo en Rooms, que la
// enlaza sin abrirla. Sin llavero, abrirla falla con ErrCacheKey, como con un llavero
// bloqueado, y sin caer nunca a guardar en claro. Así `offline`, que la edición usa por
// sus tipos y su almacén sin cifrar, no enlaza `nucleo/auth` (ni su cgo en Apple).
type systemSecrets struct{}

func (systemSecrets) Read(string) ([]byte, error) { return nil, ErrCacheKey }
func (systemSecrets) Write(string, []byte) error  { return ErrCacheKey }
