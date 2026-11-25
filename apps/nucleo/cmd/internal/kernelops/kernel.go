// SPDX-License-Identifier: Apache-2.0
//go:build js && wasm

// Package kernelops son las operaciones del núcleo criptográfico que la bóveda usa
// en el navegador: identidad híbrida, sobres, registros cifrados por fragmentos,
// la clave de firma de la bóveda y la firma de acciones de entrada. Las sirven dos
// módulos: `vault` (cmd/vault-wasm), el núcleo ligero que toda app con bóveda
// descarga al arrancar, y `core` (cmd/pqc-wasm), que además lleva las operaciones
// de Matrix, de recursos y de búsqueda de Rooms. Un solo código para los dos: el
// mismo sobre se abre igual en cualquiera de ellos (docs/decisions.md#0086).
//
// Nada de aquí importa webport ni el SDK de Matrix: es lo que mantiene ligero el
// módulo de la bóveda. TestElNucleoDeBovedaNoArrastraMatrix lo vigila.
package kernelops

import (
	"encoding/json"
	"syscall/js"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
	"git.sytue.eu/sytue/one/apps/nucleo/webport/rpc"
)

// Operations se rellena aquí y en el init de cada fichero del paquete; no cambia
// después de arrancar.
var Operations = map[string]func(js.Value) (any, error){
	"identity.create": func(js.Value) (any, error) {
		identity, err := pqc.GenerateIdentity()
		if err != nil {
			return nil, err
		}
		return publicIdentity(identity, true)
	},
	"identity.public": func(req js.Value) (any, error) {
		identity, err := identityArg(req)
		if err != nil {
			return nil, err
		}
		return publicIdentity(identity, false)
	},
	"seal": seal,
	"open": open,
	"storage.context": func(req js.Value) (any, error) {
		raw, err := pqc.StorageContext(scopeArg(req), req.Get("room").String(), req.Get("domain").String())
		if err != nil {
			return nil, err
		}
		return bytesValue(raw), nil
	},
	"storage.seal": sealRecord,
	"storage.open": openRecord,
}

// Execute es el Execute del módulo de la bóveda: una operación desconocida se
// rechaza igual que en el núcleo completo.
func Execute(operation string, req js.Value) (any, error) {
	if handler, ok := Operations[operation]; ok {
		return handler(req)
	}
	return nil, pqc.ErrEnvelope
}

func bytesArg(v js.Value, maximum int) ([]byte, error) { return rpc.BytesArg(v, maximum) }
func bytesValue(raw []byte) js.Value                   { return rpc.BytesValue(raw) }
func scopeArg(req js.Value) storage.Scope              { return rpc.ScopeArg(req) }

// IdentityArg e IntegerArg los usan también las operaciones de Matrix y de recursos
// del núcleo completo.
func IdentityArg(req js.Value) (*pqc.Identity, error)     { return identityArg(req) }
func IntegerArg(value js.Value, maximum int) (int, error) { return integerArg(value, maximum) }

func identityArg(req js.Value) (*pqc.Identity, error) {
	raw, err := bytesArg(req.Get("identity"), 4096)
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	return pqc.ParseIdentity(string(raw))
}

func recipientArg(req js.Value) (pqc.Recipient, error) {
	if req.Get("profile").String() != pqc.HybridProfile {
		return nil, pqc.ErrIdentity
	}
	return pqc.ParseRecipient(pqc.HybridProfile, req.Get("recipient").String())
}

func envelopeArg(value js.Value) (pqc.Envelope, error) {
	if value.Type() != js.TypeObject || value.Get("v").Type() != js.TypeNumber || value.Get("v").Float() != float64(pqc.EnvelopeVersion) {
		return pqc.Envelope{}, pqc.ErrEnvelope
	}
	ciphertext, err := bytesArg(value.Get("ciphertext"), pqc.MaxEnvelopeCiphertext)
	if err != nil {
		return pqc.Envelope{}, err
	}
	return pqc.Envelope{Version: pqc.EnvelopeVersion, Profile: value.Get("profile").String(), KeyID: value.Get("key_id").String(), Ciphertext: ciphertext}, nil
}

func envelopeValue(envelope pqc.Envelope) map[string]any {
	defer clear(envelope.Ciphertext)
	return map[string]any{"v": envelope.Version, "profile": envelope.Profile, "key_id": envelope.KeyID, "ciphertext": bytesValue(envelope.Ciphertext)}
}

func publicIdentity(identity *pqc.Identity, private bool) (any, error) {
	keyID, err := pqc.RecipientID(identity.Recipient())
	if err != nil {
		return nil, err
	}
	out := map[string]any{"profile": identity.Profile(), "recipient": identity.Recipient().String(), "key_id": keyID}
	if private {
		raw := []byte(identity.String())
		defer clear(raw)
		out["identity"] = bytesValue(raw)
	}
	return out, nil
}

func seal(req js.Value) (any, error) {
	recipient, err := recipientArg(req)
	if err != nil {
		return nil, err
	}
	context, err := bytesArg(req.Get("context"), pqc.MaxEnvelopeContext)
	if err != nil {
		return nil, err
	}
	defer clear(context)
	payload, err := bytesArg(req.Get("payload"), pqc.MaxEnvelopePayload)
	if err != nil {
		return nil, err
	}
	defer clear(payload)
	envelope, err := pqc.Seal(recipient, context, payload)
	if err != nil {
		return nil, err
	}
	return envelopeValue(envelope), nil
}

func open(req js.Value) (any, error) {
	identity, err := identityArg(req)
	if err != nil {
		return nil, err
	}
	context, err := bytesArg(req.Get("context"), pqc.MaxEnvelopeContext)
	if err != nil {
		return nil, err
	}
	defer clear(context)
	envelope, err := envelopeArg(req.Get("envelope"))
	if err != nil {
		return nil, err
	}
	defer clear(envelope.Ciphertext)
	raw, err := pqc.Open(identity, context, envelope)
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	return bytesValue(raw), nil
}

func sealRecord(req js.Value) (any, error) {
	recipient, err := recipientArg(req)
	if err != nil {
		return nil, err
	}
	raw, err := bytesArg(req.Get("record"), 96<<10)
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	var record storage.Record
	if json.Unmarshal(raw, &record) != nil {
		return nil, pqc.ErrEnvelope
	}
	defer clear(record.Payload)
	proof, err := bytesArg(req.Get("proof"), 4096)
	if err != nil {
		return nil, err
	}
	defer clear(proof)
	envelope, err := pqc.SealStorageRecord(recipient, scopeArg(req), req.Get("room").String(), record, proof)
	if err != nil {
		return nil, err
	}
	return envelopeValue(envelope), nil
}

func openRecord(req js.Value) (any, error) {
	identity, err := identityArg(req)
	if err != nil {
		return nil, err
	}
	envelope, err := envelopeArg(req.Get("envelope"))
	if err != nil {
		return nil, err
	}
	defer clear(envelope.Ciphertext)
	record, proof, err := pqc.OpenStorageRecord(identity, scopeArg(req), req.Get("room").String(), envelope)
	if err != nil {
		return nil, err
	}
	defer clear(record.Payload)
	defer clear(proof)
	raw, err := json.Marshal(record)
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	return map[string]any{"record": bytesValue(raw), "proof": bytesValue(proof)}, nil
}
