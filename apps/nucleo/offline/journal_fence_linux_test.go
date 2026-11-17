// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//go:build linux

package offline

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
)

// Test-only, same-app private key fixture across two executable processes. This
// proves POSIX descriptor fencing, not Apple Keychain entitlements or a broker.
type journalFixtureKeys string

func (file journalFixtureKeys) values() (map[string][]byte, error) {
	raw, err := os.ReadFile(string(file))
	if errors.Is(err, os.ErrNotExist) {
		return map[string][]byte{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	var values map[string][]byte
	err = json.Unmarshal(raw, &values)
	return values, err
}

func (file journalFixtureKeys) Read(key string) ([]byte, error) {
	values, err := file.values()
	return values[key], err
}

func (file journalFixtureKeys) Write(key string, value []byte) error {
	values, err := file.values()
	if err != nil {
		return err
	}
	values[key] = bytes.Clone(value)
	raw, err := json.Marshal(values)
	if err != nil {
		return err
	}
	defer clear(raw)
	return os.WriteFile(string(file), raw, 0o600)
}

func TestJournalRetirementProcessHelper(t *testing.T) {
	role := os.Getenv("SYTUE_QA_JOURNAL_ROLE")
	if role == "" {
		t.Skip("subprocess helper")
	}
	root := os.Getenv("SYTUE_QA_JOURNAL_FIXTURE")
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	const account = "sesion-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const identity = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	const owner = "acme\x00alice\x00mail"
	const other = "acme\x00bob\x00mail"
	const namespace = "mail-private-writes-v1"
	sourceDir := filepath.Join(root, "source")
	keys := journalFixtureKeys(filepath.Join(root, "source.keys"))
	if role == "writer" {
		if err := ajustes.PonerAccesoSesionCon(ctx, account, identity, true); err != nil {
			t.Fatal(err)
		}
	}
	access, err := ajustes.LeerAccesoSesion(account)
	if err != nil {
		t.Fatal(err)
	}
	witness := auth.TestigoTrabajo{Almacen: account, Acceso: access}
	source, err := OpenWithSecrets(sourceDir, keys)
	if err != nil {
		t.Fatal(err)
	}
	if role == "writer" {
		release, err := witness.Compartido(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, actor := range []string{owner, other} {
			if err = source.Write(actor, namespace, "pending", actor); err != nil {
				t.Fatal(err)
			}
		}
		_, _ = fmt.Fprintln(os.Stdout, "READY")
		input := bufio.NewReader(os.Stdin)
		if line, err := input.ReadString('\n'); err != nil || line != "DRAIN\n" {
			t.Fatal("missing drain request", err)
		}
		release()
		_, _ = fmt.Fprintln(os.Stdout, "DRAINED")
		if line, err := input.ReadString('\n'); err != nil || line != "PROBE\n" {
			t.Fatal("missing post-retirement probe", err)
		}
		var value string
		if _, err = source.Read(owner, namespace, "pending", &value); err == nil {
			t.Fatal("already-open reader bypassed source retirement")
		}
		if err = source.Write(owner, namespace, "resurrected", "hidden late work"); err == nil {
			t.Fatal("already-open writer resurrected retired work")
		}
		if err = source.Delete(owner, namespace, "pending"); err == nil {
			t.Fatal("already-open delete changed retained source")
		}
		if err = source.Clear(); err == nil {
			t.Fatal("already-open clear removed retirement fence")
		}
		if err = source.Write(owner, namespace, "after-clear", "hidden work"); err == nil {
			t.Fatal("old clear allowed source recreation")
		}
		if _, err = OpenWithSecrets(sourceDir, keys); err == nil {
			t.Fatal("cold reader bypassed source retirement")
		}
		_, _ = fmt.Fprintln(os.Stdout, "FENCED")
		return
	}
	if role != "retire" {
		t.Fatal("invalid helper role")
	}
	release, err := witness.Exclusivo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	targetDir := filepath.Join(root, "group", "mail", "offline")
	target, err := relocateJournal(ctx, sourceDir, targetDir, keys, nil)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := os.OpenRoot(filepath.Dir(targetDir))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = metadata.Close() }()
	state, _, err := readJournalRelocation(metadata)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := OpenWithSecrets(filepath.Join(filepath.Dir(targetDir), "retained", state.Source.CacheID), keys)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate an acknowledged current-owner upload. Retained source must neither
	// revive it nor erase another account's still-pending destination record.
	if err = target.Delete(owner, namespace, "pending"); err != nil {
		t.Fatal(err)
	}
	for _, store := range []*Store{target, archive} {
		var value string
		if found, err := store.Read(other, namespace, "pending", &value); err != nil || !found || value != other {
			t.Fatal("cutover lost another account's exact data", err)
		}
	}
	var value string
	if found, err := archive.Read(owner, namespace, "pending", &value); err != nil || !found || value != owner {
		t.Fatal("ack removed retained original", err)
	}
}

func TestJournalRetirementFencesAlreadyOpenReaderWriterAndClearAcrossProcesses(t *testing.T) {
	root, err := os.MkdirTemp("/tmp", "sytue-journal-retire-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(root, "source"), 0o700); _ = os.RemoveAll(root) })
	uid, gid := os.Geteuid(), os.Getegid()
	if uid == 0 {
		uid, gid = 65534, 65534
		if err = os.Chown(root, uid, gid); err != nil {
			t.Fatal(err)
		}
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(root, "probe")
	source, err := os.Open(executable)
	if err != nil {
		t.Fatal(err)
	}
	destination, err := os.OpenFile(binary, os.O_CREATE|os.O_WRONLY, 0o755)
	if err != nil {
		_ = source.Close()
		t.Fatal(err)
	}
	_, copyErr := io.Copy(destination, source)
	closeErr := destination.Close()
	_ = source.Close()
	if copyErr != nil || closeErr != nil {
		t.Fatal("copy process probe", copyErr, closeErr)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	command := func(role string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, binary, "-test.run=^TestJournalRetirementProcessHelper$")
		cmd.Env = append(os.Environ(), "SYTUE_QA_JOURNAL_ROLE="+role, "SYTUE_QA_JOURNAL_FIXTURE="+root, "XDG_CONFIG_HOME="+filepath.Join(root, "config"))
		if os.Geteuid() == 0 {
			cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid), NoSetGroups: true}}
		}
		return cmd
	}
	old := command("writer")
	stdin, err := old.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := old.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stderr, err := os.CreateTemp(root, "stderr-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stderr.Close() }()
	old.Stderr = stderr
	if err = old.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = old.Process.Kill(); _ = old.Wait() })
	reader := bufio.NewReader(stdout)
	expect := func(want string) {
		line, err := reader.ReadString('\n')
		if err != nil || strings.TrimSpace(line) != want {
			_ = old.Process.Kill()
			_ = old.Wait()
			rest, _ := io.ReadAll(reader)
			diagnostics, _ := os.ReadFile(stderr.Name())
			t.Fatal("process handshake", want, line, err, string(rest), string(diagnostics))
		}
	}
	expect("READY")
	_, _ = io.WriteString(stdin, "DRAIN\n")
	expect("DRAINED")
	retired := command("retire")
	if output, err := retired.CombinedOutput(); err != nil {
		t.Fatal("exclusive cutover process", err, string(output))
	}
	_, _ = io.WriteString(stdin, "PROBE\n")
	expect("FENCED")
	_ = stdin.Close()
	if err = old.Wait(); err != nil {
		diagnostics, _ := os.ReadFile(stderr.Name())
		t.Fatal("old process bypassed retirement", err, string(diagnostics))
	}
}
