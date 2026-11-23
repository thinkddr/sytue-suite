// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package vaultproof

import (
	"bytes"
	"encoding/json"
	"io"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

// IntentWire es la intención tal como la manda el módulo de correo por el puerto
// `ingress.action.sign` (el mismo formato que webport.controlIntentWire).
type IntentWire struct {
	Generation    int64                     `json:"authority_generation"`
	Scope         storage.Scope             `json:"scope"`
	Epoch         string                    `json:"epoch"`
	Kind          string                    `json:"kind"`
	Target        storage.PublicationTarget `json:"target"`
	RequestSHA256 string                    `json:"request_sha256"`
}

// Intent es la intención que se compara con la acción.
func (w IntentWire) Intent() matrixproof.Intent {
	return matrixproof.Intent{Generation: w.Generation, Scope: w.Scope, Epoch: w.Epoch, Kind: w.Kind, Target: w.Target, RequestSHA256: w.RequestSHA256}
}

// SignRequest es el cuerpo entero de `ingress.action.sign`.
type SignRequest struct {
	Expected IntentWire         `json:"expected"`
	Action   matrixproof.Action `json:"action"`
}

// DecodeSignRequest lee un SignRequest estricto: sin campos de más ni basura detrás.
func DecodeSignRequest(raw []byte) (SignRequest, error) {
	var out SignRequest
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&out) != nil || decoder.Decode(new(any)) != io.EOF {
		return SignRequest{}, ErrUnverified
	}
	return out, nil
}
