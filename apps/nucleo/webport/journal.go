// SPDX-License-Identifier: Apache-2.0
package webport

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
)

type journal struct {
	port     Port
	identity Identity
}

func OpenJournal(ctx context.Context, port Port) (*offline.Store, Identity, error) {
	return OpenJournalForLifetime(ctx, ctx, port)
}

// OpenJournalForLifetime authenticates construction with ctx while retaining only
// the verified owner lifetime for future operations and paused pending inventory.
func OpenJournalForLifetime(ctx, lifetime context.Context, port Port) (*offline.Store, Identity, error) {
	if ctx == nil || lifetime == nil || port == nil {
		return nil, Identity{}, ErrRejected
	}
	current, err := port.Current(ctx)
	if err != nil || current.Validate() != nil {
		return nil, current, ErrRejected
	}
	owner := strings.Join([]string{current.Scope.Tenant, current.Scope.Subject, current.Scope.Product}, "\x00")
	store, err := offline.OpenSealedPort(lifetime, owner, &journal{port: port, identity: current})
	if err == nil {
		err = ctx.Err()
	}
	return store, current, err
}

func (j *journal) Revalidate(ctx context.Context) error {
	current, err := j.port.Current(ctx)
	if err != nil {
		return err
	}
	if current != j.identity {
		return ErrRejected
	}
	return nil
}

func (j *journal) call(ctx context.Context, method Method, input any) ([]byte, error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	return j.port.Call(ctx, j.identity.Scope, method, raw)
}

func (j *journal) Read(ctx context.Context, key string) ([]byte, bool, error) {
	raw, err := j.call(ctx, JournalRead, struct {
		Key string `json:"key"`
	}{key})
	if err != nil {
		return nil, false, err
	}
	if len(raw) == 0 || raw[0] > 1 || raw[0] == 0 && len(raw) != 1 {
		clear(raw)
		return nil, false, ErrRejected
	}
	found := raw[0] == 1
	if !found {
		clear(raw)
		return nil, false, nil
	}
	out := append([]byte(nil), raw[1:]...)
	clear(raw)
	return out, true, nil
}

func (j *journal) Write(ctx context.Context, key string, raw []byte) error {
	metadata, err := json.Marshal(struct {
		Key string `json:"key"`
	}{key})
	if err != nil {
		return err
	}
	defer clear(metadata)
	if len(metadata) > 8192 || len(raw) > 8<<20 {
		return ErrRejected
	}
	frame := make([]byte, 4+len(metadata)+len(raw))
	defer clear(frame)
	binary.BigEndian.PutUint32(frame, uint32(len(metadata))) //nolint:gosec // G115: len(metadata) is bounded to 8192 above
	copy(frame[4:], metadata)
	copy(frame[4+len(metadata):], raw)
	response, err := j.port.Call(ctx, j.identity.Scope, JournalWrite, frame)
	clear(response)
	return err
}

func (j *journal) Delete(ctx context.Context, key string) error {
	out, err := j.call(ctx, JournalDelete, struct {
		Key string `json:"key"`
	}{key})
	clear(out)
	return err
}

func (j *journal) Keys(ctx context.Context, prefix, after string, limit int) ([]string, string, error) {
	raw, err := j.call(ctx, JournalKeys, struct {
		Prefix string `json:"prefix"`
		After  string `json:"after"`
		Limit  int    `json:"limit"`
	}{prefix, after, limit})
	if err != nil {
		return nil, "", err
	}
	defer clear(raw)
	var result struct {
		Keys []string `json:"keys"`
		Next string   `json:"next"`
	}
	if json.Unmarshal(raw, &result) != nil {
		return nil, "", ErrRejected
	}
	return result.Keys, result.Next, nil
}

func (j *journal) Lock(ctx context.Context, key string) (func(), error) {
	raw, err := j.call(ctx, JournalLock, struct {
		Key string `json:"key"`
	}{key})
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	var result struct {
		Lock string `json:"lock"`
	}
	if json.Unmarshal(raw, &result) != nil || !ValidID(result.Lock) {
		return nil, ErrRejected
	}
	return func() {
		raw, _ := j.call(context.WithoutCancel(ctx), JournalUnlock, result)
		clear(raw)
	}, nil
}
