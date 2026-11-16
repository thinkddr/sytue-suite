// SPDX-License-Identifier: Apache-2.0
// Package storagetest contains opaque storage fixtures shared by client proofs.
package storagetest

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type CipherState struct {
	Data             map[string][]byte
	Puts, Gets       int
	LoseNext, Tamper bool
}

// ServeCipherBlob preserves the maintained gateway fixture protocol. The caller
// owns its mutex, allowing older sensitivity-label fixtures to share the state.
func ServeCipherBlob(w http.ResponseWriter, r *http.Request, product string, state *CipherState) {
	prefix := "/api/v1/cipher/blobs/" + product + "/"
	if !storage.Product(product) || !strings.HasPrefix(r.URL.Path, prefix) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, prefix), "/content")
	if r.Method == http.MethodPut {
		state.Puts++
		data, _ := io.ReadAll(r.Body)
		if previous, exists := state.Data[id]; exists && !bytes.Equal(previous, data) {
			w.WriteHeader(http.StatusConflict)
			return
		}
		if state.Data == nil {
			state.Data = map[string][]byte{}
		}
		state.Data[id] = data
		if state.LoseNext {
			state.LoseNext = false
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		sum := sha256.Sum256(data)
		_ = json.NewEncoder(w).Encode(storage.BlobReceipt{ID: id, Product: product, SizeBytes: int64(len(data)), SHA256: hex.EncodeToString(sum[:])})
		return
	}
	state.Gets++
	data, ok := state.Data[id]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if state.Tamper && len(data) > 0 {
		data = bytes.Clone(data)
		data[0] ^= 1
	}
	_, _ = w.Write(data)
}

type CipherGateway struct {
	Product string
	mu      sync.Mutex
	state   CipherState
}

func (g *CipherGateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	defer g.mu.Unlock()
	ServeCipherBlob(w, r, g.Product, &g.state)
}

func (g *CipherGateway) Snapshot() CipherState {
	g.mu.Lock()
	defer g.mu.Unlock()
	state := g.state
	state.Data = make(map[string][]byte, len(g.state.Data))
	for key, data := range g.state.Data {
		state.Data[key] = bytes.Clone(data)
	}
	return state
}

// CipherCounters contains no object IDs, ciphertext or original payload bytes.
type CipherCounters struct{ Puts, Gets, Blobs int }

func (g *CipherGateway) TryCounters() (CipherCounters, bool) {
	if !g.mu.TryLock() {
		return CipherCounters{}, false
	}
	defer g.mu.Unlock()
	return CipherCounters{g.state.Puts, g.state.Gets, len(g.state.Data)}, true
}
