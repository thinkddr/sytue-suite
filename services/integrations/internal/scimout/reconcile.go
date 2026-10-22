// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package scimout

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"

	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
	"git.sytue.eu/sytue/one/services/integrations/internal/directory"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
)

// Directory es el proveedor del estado DESEADO (el directorio de Sytue ID).
type Directory interface {
	ListUsers(ctx context.Context, org string) ([]directory.User, error)
	ListGroups(ctx context.Context, org string) ([]directory.Group, error)
}

// MapStore es la persistencia del estado EMPUJADO: el mapeo Sytue↔remoto (scim_map)
// y el historial de ejecuciones (sync_runs). Lo satisface *store.Store; en tests, un
// doble en memoria (sin base de datos).
type MapStore interface {
	LoadScimMap(ctx context.Context, instanceID string) ([]domain.ScimMapEntry, error)
	UpsertScimMap(ctx context.Context, instanceID string, e domain.ScimMapEntry) error
	DeleteScimMap(ctx context.Context, instanceID string, kind domain.ScimKind, sytueID string) error
	StartSyncRun(ctx context.Context, instanceID string) (string, error)
	FinishSyncRun(ctx context.Context, runID string, c domain.SyncCounts, status string) error
}

// Deps son las dependencias de una reconciliación: de dónde leer el estado deseado
// (Dir), dónde persistir el mapeo/historial (Map) y con qué hablar SCIM (SCIM).
type Deps struct {
	Dir  Directory
	Map  MapStore
	SCIM *Client
}

// Reconcile lleva el SaaS al estado deseado del directorio de Sytue ID por DIFF —
// el ÚNICO mecanismo: cubre la sincronización inicial, la incremental y la
// reparación (es idempotente; un segundo pase sin cambios no hace nada). Abre una
// fila de sync_runs, aplica el diff y la cierra con los contadores y el estado.
//
// ponytail: reconcile por diff; delta-events de id si el volumen lo exige.
func Reconcile(ctx context.Context, d Deps, inst connector.Instance) (domain.SyncCounts, error) {
	runID, err := d.Map.StartSyncRun(ctx, inst.ID)
	if err != nil {
		return domain.SyncCounts{}, err
	}
	counts, recErr := runReconcile(ctx, d, inst)
	status := "ok"
	if recErr != nil {
		status = "error"
	}
	if ferr := d.Map.FinishSyncRun(ctx, runID, counts, status); ferr != nil {
		return counts, errors.Join(recErr, ferr)
	}
	return counts, recErr
}

// runReconcile hace el trabajo del diff. Es best-effort por recurso: un fallo
// aislado incrementa Errors y no aborta el resto; se devuelve el primer error para
// que el outbox reintente el trabajo entero (idempotente).
//
//nolint:gocognit,gocyclo // reconciliación SCIM saliente: diff de altas/bajas/cambios en una pasada, con el estado del ciclo compartido.
func runReconcile(ctx context.Context, d Deps, inst connector.Instance) (domain.SyncCounts, error) {
	var counts domain.SyncCounts
	org := inst.OrgID

	// Estado deseado (lecturas del directorio de Sytue ID).
	users, err := d.Dir.ListUsers(ctx, org)
	if err != nil {
		return counts, err
	}
	groups, err := d.Dir.ListGroups(ctx, org)
	if err != nil {
		return counts, err
	}

	// Estado empujado: el mapeo local EN BLOQUE (sin N+1 en el diff).
	entries, err := d.Map.LoadScimMap(ctx, inst.ID)
	if err != nil {
		return counts, err
	}
	userMap := make(map[string]domain.ScimMapEntry)
	groupMap := make(map[string]domain.ScimMapEntry)
	for _, e := range entries {
		switch e.Kind {
		case domain.ScimUser:
			userMap[e.SytueID] = e
		case domain.ScimGroup:
			groupMap[e.SytueID] = e
		}
	}

	var firstErr error
	fail := func(e error) {
		counts.Errors++
		if firstErr == nil {
			firstErr = e
		}
	}

	// --- Usuarios: crear/adoptar/actualizar ---
	desiredUsers := make(map[string]struct{}, len(users))
	remoteByUser := make(map[string]string, len(users)) // sytueUserID → remoteID (para membresías)
	for _, u := range users {
		desiredUsers[u.ID] = struct{}{}
		input := toUserInput(u)
		h := hashUser(input)

		if cur, ok := userMap[u.ID]; ok {
			remoteByUser[u.ID] = cur.RemoteID
			if cur.Hash == h {
				continue // idempotencia: sin cambios, sin operación.
			}
			if err := d.SCIM.ReplaceUser(ctx, cur.RemoteID, input); err != nil {
				fail(err)
				continue
			}
			if err := d.Map.UpsertScimMap(ctx, inst.ID, domain.ScimMapEntry{Kind: domain.ScimUser, SytueID: u.ID, RemoteID: cur.RemoteID, Hash: h}); err != nil {
				fail(err)
				continue
			}
			counts.Updated++
			continue
		}

		// No mapeado: adoptar por filtro `userName eq` si ya existe; si no, crear.
		remoteID, adopted, err := d.adoptOrCreateUser(ctx, input)
		if err != nil {
			fail(err)
			continue
		}
		remoteByUser[u.ID] = remoteID
		if err := d.Map.UpsertScimMap(ctx, inst.ID, domain.ScimMapEntry{Kind: domain.ScimUser, SytueID: u.ID, RemoteID: remoteID, Hash: h}); err != nil {
			fail(err)
			continue
		}
		if adopted {
			counts.Updated++ // alineamos un recurso preexistente (no duplicamos).
		} else {
			counts.Created++
		}
	}

	// --- Usuarios: desactivar los borrados en Sytue (deactivate, NUNCA delete) ---
	for sytueID, e := range userMap {
		if _, ok := desiredUsers[sytueID]; ok {
			continue
		}
		if err := d.SCIM.SetUserActive(ctx, e.RemoteID, false); err != nil {
			fail(err)
			continue
		}
		if err := d.Map.DeleteScimMap(ctx, inst.ID, domain.ScimUser, sytueID); err != nil {
			fail(err)
			continue
		}
		counts.Deactivated++
	}

	// --- Grupos: crear/adoptar/actualizar (tras usuarios: ya hay ids remotos) ---
	desiredGroups := make(map[string]struct{}, len(groups))
	for _, g := range groups {
		desiredGroups[g.ID] = struct{}{}
		memberIDs := resolveMembers(g.Members, remoteByUser)
		input := GroupInput{DisplayName: g.Name, ExternalID: g.ID, MemberIDs: memberIDs}
		h := hashGroup(input)

		if cur, ok := groupMap[g.ID]; ok {
			if cur.Hash == h {
				continue
			}
			if err := d.SCIM.ReplaceGroup(ctx, cur.RemoteID, input); err != nil {
				fail(err)
				continue
			}
			if err := d.Map.UpsertScimMap(ctx, inst.ID, domain.ScimMapEntry{Kind: domain.ScimGroup, SytueID: g.ID, RemoteID: cur.RemoteID, Hash: h}); err != nil {
				fail(err)
				continue
			}
			counts.Updated++
			continue
		}

		remoteID, adopted, err := d.adoptOrCreateGroup(ctx, input)
		if err != nil {
			fail(err)
			continue
		}
		if err := d.Map.UpsertScimMap(ctx, inst.ID, domain.ScimMapEntry{Kind: domain.ScimGroup, SytueID: g.ID, RemoteID: remoteID, Hash: h}); err != nil {
			fail(err)
			continue
		}
		if adopted {
			counts.Updated++
		} else {
			counts.Created++
		}
	}

	// --- Grupos: borrar los eliminados en Sytue (un grupo es estructura, no identidad) ---
	for sytueID, e := range groupMap {
		if _, ok := desiredGroups[sytueID]; ok {
			continue
		}
		if err := d.SCIM.DeleteGroup(ctx, e.RemoteID); err != nil {
			fail(err)
			continue
		}
		if err := d.Map.DeleteScimMap(ctx, inst.ID, domain.ScimGroup, sytueID); err != nil {
			fail(err)
			continue
		}
		counts.Deactivated++
	}

	return counts, firstErr
}

// adoptOrCreateUser adopta por `userName eq` si el usuario ya existe en el SaaS
// (alineándolo con un PUT) o lo crea. adopted indica cuál fue el camino.
func (d Deps) adoptOrCreateUser(ctx context.Context, input UserInput) (remoteID string, adopted bool, err error) {
	if rid, found, ferr := d.SCIM.FindUser(ctx, input.UserName); ferr != nil {
		return "", false, ferr
	} else if found {
		if err := d.SCIM.ReplaceUser(ctx, rid, input); err != nil {
			return "", false, err
		}
		return rid, true, nil
	}
	rid, cerr := d.SCIM.CreateUser(ctx, input)
	if cerr != nil {
		// Carrera: otro proceso lo creó entre el filtro y el POST ⇒ adoptar.
		if errors.Is(cerr, ErrConflict) {
			rid, found, ferr := d.SCIM.FindUser(ctx, input.UserName)
			if ferr != nil {
				return "", false, ferr
			}
			if !found {
				return "", false, cerr
			}
			if err := d.SCIM.ReplaceUser(ctx, rid, input); err != nil {
				return "", false, err
			}
			return rid, true, nil
		}
		return "", false, cerr
	}
	return rid, false, nil
}

// adoptOrCreateGroup es el análogo de adoptOrCreateUser para grupos (filtro
// `displayName eq`).
func (d Deps) adoptOrCreateGroup(ctx context.Context, input GroupInput) (remoteID string, adopted bool, err error) {
	if rid, found, ferr := d.SCIM.FindGroup(ctx, input.DisplayName); ferr != nil {
		return "", false, ferr
	} else if found {
		if err := d.SCIM.ReplaceGroup(ctx, rid, input); err != nil {
			return "", false, err
		}
		return rid, true, nil
	}
	rid, cerr := d.SCIM.CreateGroup(ctx, input)
	if cerr != nil {
		if errors.Is(cerr, ErrConflict) {
			rid, found, ferr := d.SCIM.FindGroup(ctx, input.DisplayName)
			if ferr != nil {
				return "", false, ferr
			}
			if !found {
				return "", false, cerr
			}
			if err := d.SCIM.ReplaceGroup(ctx, rid, input); err != nil {
				return "", false, err
			}
			return rid, true, nil
		}
		return "", false, cerr
	}
	return rid, false, nil
}

// resolveMembers traduce ids de usuario de Sytue a ids remotos (solo los usuarios
// activos aprovisionados en este pase). Los no resolubles (p. ej. dados de baja) se
// omiten: un grupo no referencia a quien no está en el SaaS.
func resolveMembers(sytueIDs []string, remoteByUser map[string]string) []string {
	out := make([]string, 0, len(sytueIDs))
	for _, id := range sytueIDs {
		if rid, ok := remoteByUser[id]; ok {
			out = append(out, rid)
		}
	}
	return out
}

// toUserInput proyecta un usuario del directorio al estado deseado SCIM. userName y
// email se toman del correo (convención de Sytue ID: userName == email).
func toUserInput(u directory.User) UserInput {
	return UserInput{
		UserName:   u.Email,
		ExternalID: u.ID,
		Formatted:  u.Name,
		Email:      u.Email,
		Active:     u.Active,
	}
}

// hashUser/hashGroup calculan un hash estable del estado deseado empujado. Si
// coincide con el guardado en scim_map, el diff no toca el recurso (idempotencia).
func hashUser(u UserInput) string {
	b, _ := json.Marshal(u) // struct: orden de campos determinista.
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func hashGroup(g GroupInput) string {
	members := append([]string(nil), g.MemberIDs...)
	sort.Strings(members) // orden estable: la membresía es un conjunto.
	b, _ := json.Marshal(struct {
		DisplayName string
		ExternalID  string
		Members     []string
	}{g.DisplayName, g.ExternalID, members})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
