// SPDX-License-Identifier: Apache-2.0
package webport

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/keyring"
)

type ingressSecrets struct{ storage *Storage }

func validIngressSecretName(name string) bool {
	const prefix = "org.sytue.ingress.key.v1."
	return strings.HasPrefix(name, prefix) && ValidID(strings.TrimPrefix(name, prefix))
}

func (s *Storage) IngressSecrets() (keyring.Secrets, error) {
	if s.StorageIdentityVersion() != storage.IsolatedIdentity {
		return nil, ErrRejected
	}
	return ingressSecrets{storage: s}, nil
}

func (s ingressSecrets) Read(ctx context.Context, name string) ([]byte, error) {
	if !validIngressSecretName(name) {
		return nil, ErrRejected
	}
	raw, err := s.storage.callOwned(ctx, IngressSecretRead, struct {
		Name string `json:"name"`
	}{name})
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	if len(raw) == 0 || raw[0] > 1 || raw[0] == 0 && len(raw) != 1 {
		return nil, ErrRejected
	}
	if raw[0] == 0 {
		return nil, keyring.ErrRecoveryRequired
	}
	return bytes.Clone(raw[1:]), nil
}

func (s ingressSecrets) Write(ctx context.Context, name string, value []byte) error {
	if !validIngressSecretName(name) || len(value) > 2048 || !json.Valid(value) {
		return ErrRejected
	}
	return s.storage.call(ctx, IngressSecretWrite, struct {
		Name  string `json:"name"`
		Value []byte `json:"value"`
	}{name, value}, nil)
}
