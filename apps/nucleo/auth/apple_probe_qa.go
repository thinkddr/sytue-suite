// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//go:build apple_keychain_qa && (ios || (darwin && tienda))

package auth

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
)

// Only the dedicated signed QA binaries contain this entry point. It accepts
// random synthetic names, never an arbitrary account or a production secret.
func ProbeKeychainApple(role, nonce string) error {
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(nonce) {
		return errors.New("invalid synthetic nonce")
	}
	private, shared, legacy := "qa-private-"+nonce, "sesion-qa-"+nonce, "rooms-matrix-master-qa-"+nonce
	conflict := "rooms-matrix-master-qa-conflict-" + nonce
	a, b, common := []byte("synthetic-private-a"), []byte("synthetic-private-b"), []byte("synthetic-shared")
	old := bytes.Repeat([]byte{0x2a}, 32)
	require := func(account string, want []byte) error {
		value, err := LeerSecreto(account)
		defer clear(value)
		if err != nil || !bytes.Equal(value, want) {
			return errors.New("synthetic scoped read mismatch")
		}
		return nil
	}
	validate := func(raw []byte) error {
		if !bytes.Equal(raw, old) {
			return errors.New("synthetic migration validator rejected material")
		}
		return nil
	}
	switch role {
	case "seed-a":
		if err := GuardarSecreto(private, a); err != nil {
			return err
		}
		if err := GuardarSecreto(shared, common); err != nil {
			return err
		}
		if err := guardarDatosApple(conflict, "", b); err != nil {
			return err
		}
		if err := guardarDatosApple(conflict, ajustes.GrupoSuite, old); err != nil {
			return err
		}
		return guardarDatosApple(legacy, ajustes.GrupoSuite, old)
	case "check-b":
		value, err := LeerSecreto(private)
		if err != nil || len(value) != 0 {
			clear(value)
			return errors.New("foreign private item readable")
		}
		if err = BorrarSecreto(private); err != nil {
			return err
		}
		if err = require(shared, common); err != nil {
			return err
		}
		value, err = LeerSecreto(legacy)
		if len(value) != 0 || !errors.Is(err, ErrSecretoLegadoCompartido) {
			clear(value)
			return errors.New("private query searched legacy shared group")
		}
		if _, err = MigrarSecretoPrivado(legacy, "sytue-one", validate); !errors.Is(err, ErrGrupoPrivadoApple) {
			return errors.New("foreign scheme authorized migration")
		}
		return GuardarSecreto(private, b)
	case "migrate-a":
		if err := require(private, a); err != nil {
			return err
		}
		own, err := grupoPrivadoApple()
		if err != nil {
			return err
		}
		if err = moverDatosApple(conflict, ajustes.GrupoSuite, own); err == nil {
			return errors.New("atomic transfer overwrote an existing private destination")
		}
		if err = require(conflict, b); err != nil {
			return err
		}
		retained, err := leerDatosApple(conflict, ajustes.GrupoSuite)
		defer clear(retained)
		if err != nil || !bytes.Equal(retained, old) {
			return errors.New("failed atomic transfer changed its source")
		}
		value, err := MigrarSecretoPrivado(legacy, "sytue-one", validate)
		defer clear(value)
		if err != nil || !bytes.Equal(value, old) {
			return fmt.Errorf("synthetic migration failed: %w", err)
		}
		remains, err := existeDatosApple(legacy, ajustes.GrupoSuite)
		if err != nil || remains {
			return errors.New("atomic transfer retained a shared copy")
		}
		return require(legacy, old)
	case "after-b":
		value, err := LeerSecreto(legacy)
		if err != nil || len(value) != 0 {
			clear(value)
			return errors.New("moved key became readable by foreign app")
		}
		if err = require(private, b); err != nil {
			return err
		}
		return require(shared, common)
	case "clean":
		for _, account := range []string{private, shared, legacy, conflict} {
			if err := BorrarSecreto(account); err != nil {
				return err
			}
			if err := borrarDatosApple(account, ajustes.GrupoSuite); err != nil {
				return err
			}
		}
		return nil
	default:
		return errors.New("unknown synthetic probe role")
	}
}
