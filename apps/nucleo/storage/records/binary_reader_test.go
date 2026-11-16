// SPDX-License-Identifier: Apache-2.0
package records

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math"
	"sync/atomic"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type rangeReadProbe struct {
	*commitmentReadProbe
	ranges int
	read   func(context.Context, int64, int64) ([]byte, error)
}

func (p *rangeReadProbe) ReadStorageFileRange(ctx context.Context, _ *api.Drive, ref storage.Reference, object, revision string, offset, length int64) ([]byte, error) {
	p.ranges++
	if ref != (storage.Reference{RoomID: "!photos:test", EventID: "$file"}) || object != p.info.ObjectID || revision != p.info.RevisionID || offset < 0 || length < 0 || offset+length > int64(len(p.body)) {
		return nil, ErrSource
	}
	if p.read != nil {
		return p.read(ctx, offset, length)
	}
	return bytes.Clone(p.body[offset : offset+length]), nil
}

func rangeReaderFixture(t *testing.T) (*Repository, *rangeReadProbe, *atomic.Int32) {
	t.Helper()
	r, base, version, row := binaryReaderFixture(t)
	bindCommitmentFixture(r, &version)
	probe := &rangeReadProbe{commitmentReadProbe: &commitmentReadProbe{binaryReadProbe: base, digest: row.SHA256}}
	r.cfg.Crypto = probe
	r.ready = true
	r.heads = map[string][]storage.ObjectVersion{version.ObjectID: {version}}
	leases := &atomic.Int32{}
	r.gate = offline.NewWorkGateWithLease(t.Context(), func(ctx context.Context) (func(), error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		leases.Add(1)
		return func() { leases.Add(-1) }, nil
	})
	t.Cleanup(r.Close)
	return r, probe, leases
}

func TestBinaryReaderSeeksWithoutDownloadAndPinsAuthenticatedRevision(t *testing.T) {
	r, probe, leases := rangeReaderFixture(t)
	reader, err := r.OpenBinary(t.Context(), "original", "asset")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	if probe.downloads != 0 || probe.ranges != 0 || leases.Load() != 1 || reader.Size() != int64(len(probe.body)) {
		t.Fatal("open fetched body or lost lease")
	}
	out := make([]byte, 3)
	if n, err := reader.Read(out); err != nil || n != 3 || !bytes.Equal(out, probe.body[:3]) {
		t.Fatal("initial read", n, err)
	}
	if probe.ranges != 1 {
		t.Fatal("range was not bounded and buffered")
	}
	if n, err := reader.Read(out); err != nil || n != 3 || !bytes.Equal(out, probe.body[3:6]) || probe.ranges != 1 {
		t.Fatal("sequential read fetched again", n, err)
	}
	if _, err := reader.Seek(-3, io.SeekEnd); err != nil {
		t.Fatal(err)
	}
	tail, err := io.ReadAll(reader)
	if err != nil || !bytes.Equal(tail, probe.body[len(probe.body)-3:]) || probe.ranges != 2 {
		t.Fatalf("seek err=%v got=%x want=%x range calls=%d", err, tail, probe.body[len(probe.body)-3:], probe.ranges)
	}
	if _, err := reader.Seek(-1, io.SeekStart); err == nil {
		t.Fatal("negative seek accepted")
	}
	if _, err := reader.Seek(math.MaxInt64, io.SeekEnd); err == nil {
		t.Fatal("overflow accepted")
	}
	if _, err := reader.Seek(0, 99); err == nil {
		t.Fatal("invalid origin accepted")
	}
	if err := reader.Close(); err != nil || leases.Load() != 0 {
		t.Fatal("close retained lease", err)
	}
	if _, err := reader.Read(out); !errors.Is(err, ErrClosed) {
		t.Fatal("closed reader returned data", err)
	}
}

func TestBinaryReaderRejectsForeignHeaderAndFailedAuthenticatedChunk(t *testing.T) {
	r, probe, _ := rangeReaderFixture(t)
	saved := probe.info.ObjectID
	probe.info.ObjectID = "foreign"
	if _, err := r.OpenBinary(t.Context(), "original", "asset"); !errors.Is(err, ErrSource) || probe.ranges != 0 {
		t.Fatal("foreign header passed", err)
	}
	probe.info.ObjectID = saved
	reader, err := r.OpenBinary(t.Context(), "original", "asset")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	probe.read = func(context.Context, int64, int64) ([]byte, error) { return []byte("unauthenticated"), ErrSource }
	out := []byte{0, 0, 0}
	if n, err := reader.Read(out); n != 0 || !errors.Is(err, ErrSource) || !bytes.Equal(out, []byte{0, 0, 0}) {
		t.Fatal("failed SDK bytes exposed", n, err)
	}
	probe.read = func(context.Context, int64, int64) ([]byte, error) { return []byte{1}, nil }
	if n, err := reader.Read(out); n != 0 || !errors.Is(err, ErrSource) {
		t.Fatal("truncated range exposed", n, err)
	}
}

func TestBinaryReaderRetirementErasesIdleBufferAndReleasesLease(t *testing.T) {
	r, probe, leases := rangeReaderFixture(t)
	reader, err := r.OpenBinary(t.Context(), "original", "asset")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reader.Read(make([]byte, 1)); err != nil {
		t.Fatal(err)
	}
	reader.mu.Lock()
	retained := reader.buffer
	reader.mu.Unlock()
	if len(retained) == 0 || bytes.Equal(retained, make([]byte, len(retained))) {
		t.Fatal("fixture has no retained private bytes")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := r.Drain(ctx); err != nil {
		t.Fatal("idle reader blocked retirement", err)
	}
	if leases.Load() != 0 || !bytes.Equal(retained, make([]byte, len(retained))) {
		t.Fatal("retirement left plaintext or lease")
	}
	if n, err := reader.Read(make([]byte, 1)); n != 0 || err == nil {
		t.Fatal("retired reader exposed data", err)
	}
	if probe.downloads != 0 {
		t.Fatal("retirement downloaded whole media")
	}
}

func TestBinaryReaderRetirementJoinsCanceledSDKBeforeReleasingLease(t *testing.T) {
	r, probe, leases := rangeReaderFixture(t)
	entered, canceled, finish := make(chan struct{}), make(chan struct{}), make(chan struct{})
	probe.read = func(ctx context.Context, _, _ int64) ([]byte, error) {
		close(entered)
		<-ctx.Done()
		close(canceled)
		<-finish
		return nil, ctx.Err()
	}
	reader, err := r.OpenBinary(t.Context(), "original", "asset")
	if err != nil {
		t.Fatal(err)
	}
	readDone := make(chan error, 1)
	go func() { _, err := reader.Read(make([]byte, 1)); readDone <- err }()
	<-entered
	drainDone := make(chan error, 1)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	go func() { drainDone <- r.Drain(ctx) }()
	<-canceled
	if leases.Load() != 1 {
		t.Fatal("lease released while SDK still owns work")
	}
	select {
	case err := <-drainDone:
		t.Fatal("retirement skipped SDK cleanup", err)
	default:
	}
	close(finish)
	if err := <-readDone; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := <-drainDone; err != nil || leases.Load() != 0 {
		t.Fatal("retirement did not join SDK", err)
	}
}
