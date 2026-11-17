// SPDX-License-Identifier: Apache-2.0
//go:build linux

package ajustes

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func kernelBroker(t *testing.T, confirmations *atomic.Int32) clienteTrabajoBroker {
	t.Helper()
	_, access := brokerFixture()
	return readyWorkBroker(func(_ context.Context, action, raw string) (string, error) {
		var p pedidoTrabajoBroker
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			return "", err
		}
		if action == "confirmar" {
			confirmations.Add(1)
		}
		return brokerResponse(t, action, p, access, &registroTrabajoBroker{}), nil
	})
}

func kernelRequest(exclusive bool) pedidoTrabajoBroker {
	p, _ := brokerFixture()
	p.Ambito = nil
	p.Exclusiva = &exclusive
	return p
}

func TestWorkBrokerFreshDescriptionsPreserveBothReadersAndReportWhileWriterWaits(t *testing.T) {
	file := filepath.Join(t.TempDir(), "one-inode")
	var confirmations, opens atomic.Int32
	opened := make(chan struct{}, 4)
	open := func(context.Context, string) (*os.File, error) {
		f, err := os.OpenFile(file, os.O_CREATE|os.O_RDWR, 0o600)
		if err == nil {
			opens.Add(1)
			opened <- struct{}{}
		}
		return f, err
	}
	client := kernelBroker(t, &confirmations)
	first, _, err := client.cerrojo(t.Context(), kernelRequest(false), open)
	if err != nil {
		t.Fatal(err)
	}
	defer first()
	second, _, err := client.cerrojo(t.Context(), kernelRequest(false), open)
	if err != nil {
		t.Fatal(err)
	}
	defer second()
	<-opened
	<-opened
	acquired := make(chan func(), 1)
	failed := make(chan error, 1)
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	go func() {
		release, _, err := client.cerrojo(ctx, kernelRequest(true), open)
		if err != nil {
			failed <- err
			return
		}
		acquired <- release
	}()
	<-opened
	assertWaiting := func() {
		t.Helper()
		select {
		case release := <-acquired:
			release()
			t.Fatal("writer converted another reader's description")
		case err := <-failed:
			t.Fatal(err)
		case <-time.After(35 * time.Millisecond):
		}
	}
	assertWaiting()
	if confirmations.Load() != 2 {
		t.Fatal("confirmed before kernel ownership")
	}
	// A report/refresh RPC must remain available while the EX kernel wait runs.
	p, _ := brokerFixture()
	if _, _, err := client.aplicar(t.Context(), "resumir", p); err != nil {
		t.Fatal(err)
	}
	first()
	assertWaiting()
	second()
	select {
	case release := <-acquired:
		release()
		release()
	case err := <-failed:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal("exclusive lease never acquired after both readers left")
	}
	if opens.Load() != 3 || confirmations.Load() != 3 {
		t.Fatal("not one fresh open and confirmation per lease", opens.Load(), confirmations.Load())
	}
}

func TestWorkBrokerWaitsForDifferentProcessAndCancelsWithoutConfirming(t *testing.T) {
	file := filepath.Join(t.TempDir(), "process-inode")
	command := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestWorkBrokerKernelReaderProcess$")
	command.Env = append(os.Environ(), "SYTUE_KERNEL_READER_PATH="+file)
	in, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	command.Stderr = os.Stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = in.Close(); _ = command.Wait() })
	line, err := bufio.NewReader(out).ReadString('\n')
	if err != nil || strings.TrimSpace(line) != "reader-held" {
		t.Fatal("reader process did not lock", line, err)
	}
	var confirmations atomic.Int32
	client := kernelBroker(t, &confirmations)
	var opened *os.File
	open := func(context.Context, string) (*os.File, error) {
		var err error
		opened, err = os.OpenFile(file, os.O_RDWR, 0o600)
		return opened, err
	}
	ctx, cancel := context.WithTimeout(t.Context(), 80*time.Millisecond)
	defer cancel()
	release, native, err := client.cerrojo(ctx, kernelRequest(true), open)
	if release != nil || !native || !errors.Is(err, context.DeadlineExceeded) || confirmations.Load() != 0 {
		t.Fatal("foreign process lease bypassed", native, err)
	}
	if _, err := opened.Stat(); !errors.Is(err, os.ErrClosed) {
		t.Fatal("canceled description leaked", err)
	}
}

func TestWorkBrokerKernelReaderProcess(t *testing.T) {
	file := os.Getenv("SYTUE_KERNEL_READER_PATH")
	if file == "" {
		t.Skip("subprocess helper")
	}
	f, err := os.OpenFile(file, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	release, err := CerrojoFicheroCompartido(t.Context(), f)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	_, _ = fmt.Fprintln(os.Stdout, "reader-held")
	_, _ = bufio.NewReader(os.Stdin).ReadByte()
}

func TestWorkBrokerConfirmationFailureReleasesKernelAndNeverAdmitsIO(t *testing.T) {
	for _, kind := range []string{"nonce", "capability-disappeared", "transport"} {
		t.Run(kind, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "failed-confirmation")
			var descriptor *os.File
			open := func(context.Context, string) (*os.File, error) {
				var err error
				descriptor, err = os.OpenFile(file, os.O_CREATE|os.O_RDWR, 0o600)
				return descriptor, err
			}
			calls := 0
			_, access := brokerFixture()
			client := clienteTrabajoBroker{disponible: func() (bool, error) { calls++; return kind != "capability-disappeared" || calls <= 1, nil }, pedir: func(_ context.Context, action, raw string) (string, error) {
				if kind == "transport" {
					return "", errors.New("IPC failed")
				}
				var p pedidoTrabajoBroker
				if err := json.Unmarshal([]byte(raw), &p); err != nil {
					return "", err
				}
				access.Revision = strings.Repeat("f", 32)
				return brokerResponse(t, action, p, access, nil), nil
			}}
			release, native, err := client.cerrojo(t.Context(), kernelRequest(true), open)
			if release != nil || !native || err == nil {
				t.Fatal("unconfirmed lease permitted IO", native, err)
			}
			if _, err := descriptor.Stat(); !errors.Is(err, os.ErrClosed) {
				t.Fatal("failed confirmation leaked description", err)
			}
			next, err := os.OpenFile(file, os.O_RDWR, 0o600)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			unlock, err := CerrojoFichero(ctx, next)
			if err != nil {
				t.Fatal("failed confirmation leaked lock", err)
			}
			unlock()
		})
	}
}
