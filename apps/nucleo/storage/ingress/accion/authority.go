// SPDX-License-Identifier: Apache-2.0
package accion

import (
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// Authority is public account metadata from Rooms' authenticated internal
// channel. A cached copy is never a work permit: each domain revalidates the
// generation and active device around publication and acknowledgment.
type Authority struct {
	Version    int           `json:"v"`
	Scope      storage.Scope `json:"scope"`
	UserID     string        `json:"user_id"`
	RootKey    string        `json:"root_key"`
	Generation int64         `json:"generation"`
	Status     string        `json:"status"`
}

func (a Authority) Validate(scope storage.Scope) error {
	if a.Version != 1 || a.Scope != scope || !scope.Valid() || !ValidKey(a.RootKey) || a.Generation < 1 || a.Generation >= 1<<53 || !strings.HasPrefix(a.UserID, storage.IsolatedMatrixUserPrefix(scope)) || len(a.UserID) <= len(storage.IsolatedMatrixUserPrefix(scope)) || strings.ContainsAny(a.UserID, "\x00\r\n\t /?#\\") || len(a.UserID) > 255 {
		return ErrUnverified
	}
	switch a.Status {
	case "active", "revoked":
		return nil
	default:
		return ErrUnverified
	}
}

func (a Authority) RequireActive(scope storage.Scope, generation int64) error {
	if a.Validate(scope) != nil || a.Status != "active" || a.Generation != generation {
		return ErrUnverified
	}
	return nil
}

// RootEnrollment is constructed from independently queried keys by Rooms and
// from the trusted own SDK root by the client. They must agree before signing.
// The first enrollment cannot rotate or replace an existing authority.
type RootEnrollment struct {
	PreviousGeneration int64                     `json:"previous_generation,omitempty"`
	PreviousRootKey    string                    `json:"previous_root_key,omitempty"`
	Version            int                       `json:"v"`
	Scope              storage.Scope             `json:"scope"`
	UserID             string                    `json:"user_id"`
	Target             storage.PublicationTarget `json:"target"`
	RootKey            string                    `json:"root_key"`
	Generation         int64                     `json:"generation"`
}

type RootChallenge struct {
	Action  Action         `json:"action"`
	Request RootEnrollment `json:"request"`
}

// Authorization is the result of one fresh internal check. Domains must not
// cache it across publication, backup or ACK; generation is part of the signed
// action, and nonce consumption remains in the domain's transaction.
type Authorization struct {
	Version       int                       `json:"v"`
	Authority     Authority                 `json:"authority"`
	Target        storage.PublicationTarget `json:"target"`
	SigningKey    string                    `json:"signing_key"`
	Nonce         string                    `json:"nonce"`
	RequestSHA256 string                    `json:"request_sha256"`
}

func (a Authorization) Validate(expected Action) error {
	if a.Version != 1 || a.Authority.RequireActive(expected.Scope, expected.Generation) != nil || a.Authority.RootKey != expected.RootKey || a.Target != expected.Target || !ValidKey(a.SigningKey) || a.Nonce != expected.Nonce || a.RequestSHA256 != expected.RequestSHA256 {
		return ErrUnverified
	}
	return nil
}
