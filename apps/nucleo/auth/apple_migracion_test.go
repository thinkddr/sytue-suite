// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package auth

import (
	"bytes"
	"errors"
	"testing"
)

type migrationFixture struct {
	values    map[string][]byte
	moveErr   error
	afterRead func(string, string)
}

func (f *migrationFixture) Read(account, group string) ([]byte, error) {
	data := bytes.Clone(f.values[group+"/"+account])
	if f.afterRead != nil {
		f.afterRead(account, group)
	}
	return data, nil
}

func (f *migrationFixture) Move(account, from, to string) error {
	if f.moveErr != nil {
		return f.moveErr
	}
	if len(f.values[to+"/"+account]) != 0 {
		return ErrSecretoLegadoCompartido
	}
	f.values[to+"/"+account] = f.values[from+"/"+account]
	delete(f.values, from+"/"+account)
	return nil
}

func TestPrivateSecretMigrationPreservesExactMaterialAndNeverOverwritesAmbiguity(t *testing.T) {
	const account = "rooms-matrix-master-test"
	validate := func(raw []byte) error {
		if len(raw) != 32 {
			return ErrSecretoLegadoCompartido
		}
		return nil
	}
	original := bytes.Repeat([]byte{0x2a}, 32)
	for _, failure := range []error{nil, errors.New("locked keychain")} {
		fixture := &migrationFixture{values: map[string][]byte{"suite/" + account: bytes.Clone(original)}, moveErr: failure}
		value, err := migrarSecretoPrivado(fixture, account, "own", "suite", validate)
		if failure != nil {
			if !errors.Is(err, failure) || !bytes.Equal(fixture.values["suite/"+account], original) || len(value) != 0 {
				t.Fatal("failed migration lost source or returned a replacement", err)
			}
			fixture.moveErr = nil
			value, err = migrarSecretoPrivado(fixture, account, "own", "suite", validate)
		}
		if err != nil || !bytes.Equal(value, original) || !bytes.Equal(fixture.values["own/"+account], original) || len(fixture.values["suite/"+account]) != 0 {
			t.Fatal("migration did not atomically preserve exact key", err)
		}
		value, err = migrarSecretoPrivado(fixture, account, "own", "suite", validate)
		if err != nil || !bytes.Equal(value, original) {
			t.Fatal("migration could not resume", err)
		}
	}
	fixture := &migrationFixture{values: map[string][]byte{"suite/" + account: bytes.Clone(original)}}
	invalid := errors.New("wrong owner")
	if _, err := migrarSecretoPrivado(fixture, account, "own", "suite", func([]byte) error { return invalid }); !errors.Is(err, invalid) || !bytes.Equal(fixture.values["suite/"+account], original) {
		t.Fatal("wrong owner moved source", err)
	}
	fixture.values["own/"+account] = []byte("ambiguous private entry")
	if _, err := migrarSecretoPrivado(fixture, account, "own", "suite", validate); err == nil || !bytes.Equal(fixture.values["suite/"+account], original) || string(fixture.values["own/"+account]) != "ambiguous private entry" {
		t.Fatal("ambiguous target was replaced", err)
	}
}

func TestPrivateSecretMigrationDetectsConcurrentSourceChangeAndRetainsChangedMaterial(t *testing.T) {
	original := bytes.Repeat([]byte{0x33}, 32)
	updated := bytes.Repeat([]byte{0x44}, 32)
	fixture := &migrationFixture{values: map[string][]byte{"suite/key": original}}
	fixture.afterRead = func(account, group string) {
		if group == "suite" {
			fixture.values[group+"/"+account] = bytes.Clone(updated)
			fixture.afterRead = nil
		}
	}
	if _, err := migrarSecretoPrivado(fixture, "key", "own", "suite", func([]byte) error { return nil }); !errors.Is(err, ErrSecretoLegadoCompartido) {
		t.Fatal("concurrent source change was silently accepted", err)
	}
	if !bytes.Equal(fixture.values["own/key"], updated) {
		t.Fatal("atomic move lost concurrently updated material")
	}
}

func TestPrivateSecretMigrationSharedSessionPreservesCanonicalDestination(t *testing.T) {
	fixture := &migrationFixture{values: map[string][]byte{"own/sesion": []byte("existing session")}}
	validate := func(raw []byte) error {
		if len(raw) == 0 {
			return ErrSecretoLegadoCompartido
		}
		return nil
	}
	result, err := migrarSecretoPrivado(fixture, "sesion", "suite", "own", validate)
	if err != nil || string(result) != "existing session" || len(fixture.values["own/sesion"]) != 0 {
		t.Fatal("legacy session was not transferred intact", err)
	}
	fixture.values["own/sesion"] = []byte("older session")
	result, err = migrarSecretoPrivado(fixture, "sesion", "suite", "own", validate)
	if err != nil || string(result) != "existing session" || string(fixture.values["own/sesion"]) != "older session" {
		t.Fatal("canonical shared session or retained source was replaced", err)
	}
}

func TestPrivateSecretMigrationPurposeCatalog(t *testing.T) {
	for _, account := range []string{"offline-cache-A", "rooms-matrix-session-A", "rooms-matrix-master-A"} {
		if !cuentaPrivadaMigrable(account) {
			t.Fatalf("owned private purpose rejected: %s", account)
		}
	}
	for _, account := range []string{"", "sesion", "sesion-any", "other-secret", "rooms-matrix-session"} {
		if cuentaPrivadaMigrable(account) {
			t.Fatalf("unrelated purpose accepted: %s", account)
		}
	}
}
