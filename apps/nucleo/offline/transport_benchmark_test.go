// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !js

package offline

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// These inputs are artificial bytes, never a user response or decrypted E2E data.
// Run remotely with fixed iterations, e.g. -benchtime=3x -benchmem. The memory source
// deliberately excludes network latency; slow-put adds a declared backend delay.
func BenchmarkTransportCache(b *testing.B) {
	for _, size := range []int{64 << 10, 1 << 20, 16 << 20, 32 << 20} {
		for _, mode := range []string{"direct", "cold-write", "rewrite", "offline-read"} {
			b.Run(fmt.Sprintf("%dKiB/%s", size>>10, mode), func(b *testing.B) { benchmarkTransportCache(b, size, mode) })
		}
	}
	for _, mode := range []string{"slow-put", "failed-put"} {
		b.Run("1024KiB/"+mode, func(b *testing.B) { benchmarkTransportCache(b, 1<<20, mode) })
	}
}

type benchmarkSource struct {
	data    []byte
	read    int64
	offline bool
}

func (s *benchmarkSource) RoundTrip(r *http.Request) (*http.Response, error) {
	s.read = 0
	if s.offline {
		return nil, fs.ErrNotExist
	}
	s.read = 0
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), ContentLength: int64(len(s.data)), Body: io.NopCloser(&benchmarkBody{Reader: bytes.NewReader(s.data), source: s}), Request: r}, nil
}

type benchmarkBody struct {
	*bytes.Reader
	source *benchmarkSource
}

func (r *benchmarkBody) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.source.read += int64(n)
	return n, err
}

// Hide bytes.Reader.WriteTo so complete-read also exercises Read in the baseline.
func (r *benchmarkBody) WriteTo(w io.Writer) (int64, error) {
	return io.Copy(w, struct{ io.Reader }{r})
}

type benchmarkBackend struct {
	backend
	delay time.Duration
	fail  bool
}

func (d benchmarkBackend) lockHTTP(ctx context.Context) (func(), error) {
	planner, ok := d.backend.(httpBudgetPlanner)
	if !ok {
		return nil, fs.ErrInvalid
	}
	return planner.lockHTTP(ctx)
}

func (d benchmarkBackend) pruneHTTP(namespaces []string, keep string, size int64) error {
	planner, ok := d.backend.(httpBudgetPlanner)
	if !ok {
		return fs.ErrInvalid
	}
	return planner.pruneHTTP(namespaces, keep, size)
}

func (d benchmarkBackend) put(key string, body []byte) error {
	if d.delay > 0 {
		time.Sleep(d.delay)
	}
	if d.fail {
		return fs.ErrPermission
	}
	return d.backend.put(key, body)
}

func benchmarkTransportCache(b *testing.B, size int, mode string) {
	b.Helper()
	b.Setenv("XDG_CONFIG_HOME", b.TempDir())
	const owner = "artificial/benchmark-tenant"
	const product = "benchmark"
	data := bytes.Repeat([]byte{0xa5}, size)
	source := &benchmarkSource{data: data}
	dir := b.TempDir()
	store, err := Open(dir)
	if err != nil {
		b.Fatal(err)
	}
	if f, ok := store.b.(ficheros); ok {
		b.Cleanup(func() { _ = f.raiz.Close() })
	}
	if mode == "slow-put" {
		store.b = benchmarkBackend{backend: store.b, delay: 25 * time.Millisecond}
	}
	if mode == "failed-put" {
		store.b = benchmarkBackend{backend: store.b, fail: true}
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://benchmark.invalid/api/list", http.NoBody)
	if err != nil {
		b.Fatal(err)
	}
	var transport http.RoundTripper = source
	if mode != "direct" {
		transport = &Transport{Base: source, Store: store, Owner: func() string { return owner }, Product: product}
	}
	if mode == "rewrite" || mode == "offline-read" {
		response, err := transport.RoundTrip(req)
		if err != nil {
			b.Fatal(err)
		}
		_, err = io.Copy(io.Discard, response.Body)
		if err != nil {
			b.Fatal(err)
		}
		if err := response.Body.Close(); err != nil {
			b.Fatal(err)
		}
	}
	source.offline = mode == "offline-read"
	var roundtrip, firstByte, complete time.Duration
	var atReturn int64
	var first [1]byte
	b.SetBytes(int64(size))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if mode == "cold-write" {
			b.StopTimer()
			if err := store.Delete(owner, espacioHTTP(product), req.URL.String()); err != nil {
				b.Fatal(err)
			}
			b.StartTimer()
		}
		started := time.Now()
		response, err := transport.RoundTrip(req)
		if err != nil {
			b.Fatal(err)
		}
		returned := time.Since(started)
		atReturn += source.read
		n, err := response.Body.Read(first[:])
		if n != 1 || err != nil || first[0] != 0xa5 {
			b.Fatal("missing first byte", n, err)
		}
		firstByte += time.Since(started)
		rest, err := io.Copy(io.Discard, response.Body)
		if err != nil || rest != int64(size-1) {
			b.Fatal("incomplete body", rest, err)
		}
		if err := response.Body.Close(); err != nil {
			b.Fatal(err)
		}
		complete += time.Since(started)
		roundtrip += returned
	}
	b.StopTimer()
	persisted := int64(0)
	info, err := os.Stat(filepath.Join(dir, filepath.FromSlash(clave(owner, espacioHTTP(product), req.URL.String())+".json")))
	if err == nil {
		persisted = info.Size()
	} else if !os.IsNotExist(err) {
		b.Fatal(err)
	}
	b.ReportMetric(float64(roundtrip)/float64(b.N)/float64(time.Millisecond), "roundtrip-ms/op")
	b.ReportMetric(float64(firstByte)/float64(b.N)/float64(time.Millisecond), "first-byte-ms/op")
	b.ReportMetric(float64(complete)/float64(b.N)/float64(time.Millisecond), "complete-ms/op")
	b.ReportMetric(float64(atReturn)/float64(b.N), "source-B-at-return/op")
	b.ReportMetric(float64(persisted), "persisted-B")
}
