// SPDX-License-Identifier: Apache-2.0
//go:build js && wasm

package main

import (
	"syscall/js"

	"git.sytue.eu/sytue/one/apps/nucleo/recoverywrap"
)

// La clave de firma híbrida de una bóveda (ADR 0087): se crea, se abre y firma
// aquí, en el núcleo, como la identidad de cifrado. El navegador nunca implementa
// Ed25519 ni ML-DSA: solo guarda la clave dentro del secreto de la bóveda.
func init() {
	domainOperations["vault.signing.create"] = vaultSigningCreate
	domainOperations["vault.signing.public"] = vaultSigningPublic
	domainOperations["vault.signing.sign"] = vaultSigningSign
	domainOperations["vault.signing.enroll"] = vaultSigningEnroll
}

const (
	maxVaultSigningKey     = 128
	maxVaultSigningMessage = 1 << 20
)

func vaultSignerArg(req js.Value) (*recoverywrap.PQVaultSigner, error) {
	raw, err := bytesArg(req.Get("signing_key"), maxVaultSigningKey)
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	return recoverywrap.ParsePQVaultSigningKey(string(raw))
}

func vaultVerifyKeyValue(signer *recoverywrap.PQVaultSigner) (map[string]any, error) {
	public := signer.Public()
	keyID, err := public.ID()
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"profile": public.Profile,
		"key_id":  keyID,
		"ed25519": bytesValue(public.Ed25519),
		"mldsa65": bytesValue(public.MLDSA65),
	}, nil
}

func vaultSigningCreate(js.Value) (any, error) {
	encoded, err := recoverywrap.NewPQVaultSigningKey()
	if err != nil {
		return nil, err
	}
	signer, err := recoverywrap.ParsePQVaultSigningKey(encoded)
	if err != nil {
		return nil, err
	}
	out, err := vaultVerifyKeyValue(signer)
	if err != nil {
		return nil, err
	}
	raw := []byte(encoded)
	defer clear(raw)
	out["signing_key"] = bytesValue(raw)
	return out, nil
}

func vaultSigningPublic(req js.Value) (any, error) {
	signer, err := vaultSignerArg(req)
	if err != nil {
		return nil, err
	}
	return vaultVerifyKeyValue(signer)
}

func vaultSigningSign(req js.Value) (any, error) {
	signer, err := vaultSignerArg(req)
	if err != nil {
		return nil, err
	}
	message, err := bytesArg(req.Get("message"), maxVaultSigningMessage)
	if err != nil {
		return nil, err
	}
	signature, err := signer.Sign(req.Get("context").String(), message)
	if err != nil {
		return nil, err
	}
	return map[string]any{"signature": bytesValue(signature)}, nil
}

// vaultSigningEnroll prepara el registro de una generación de la clave en Sytue
// ID: la parte pública, la prueba de la clave nueva y, si se pasa la anterior
// (`previous_signing_key`), su aprobación. La declaración la compone el núcleo.
func vaultSigningEnroll(req js.Value) (any, error) {
	signer, err := vaultSignerArg(req)
	if err != nil {
		return nil, err
	}
	out, err := vaultVerifyKeyValue(signer)
	if err != nil {
		return nil, err
	}
	keyID, _ := out["key_id"].(string)
	generation := int64(req.Get("generation").Int())
	previousKeyID := ""
	var previous *recoverywrap.PQVaultSigner
	if v := req.Get("previous_signing_key"); v.Type() != js.TypeUndefined && v.Type() != js.TypeNull {
		raw, err := bytesArg(v, maxVaultSigningKey)
		if err != nil {
			return nil, err
		}
		previous, err = recoverywrap.ParsePQVaultSigningKey(string(raw))
		clear(raw)
		if err != nil {
			return nil, err
		}
		if previousKeyID, err = previous.Public().ID(); err != nil {
			return nil, err
		}
	} else if v := req.Get("previous_key_id"); v.Type() == js.TypeString {
		// Un vaciado: la nueva sustituye a una que ya no se puede abrir.
		previousKeyID = v.String()
	}
	statement, err := recoverywrap.PQVaultKeyStatement(req.Get("subject").String(), req.Get("tenant").String(),
		req.Get("product").String(), generation, keyID, previousKeyID)
	if err != nil {
		return nil, err
	}
	proof, err := signer.Sign(recoverywrap.VaultKeyEnrollContext, statement)
	if err != nil {
		return nil, err
	}
	out["generation"] = generation
	out["previous_key_id"] = previousKeyID
	out["proof"] = bytesValue(proof)
	if previous != nil {
		continuity, err := previous.Sign(recoverywrap.VaultKeyRotateContext, statement)
		if err != nil {
			return nil, err
		}
		out["continuity"] = bytesValue(continuity)
	}
	return out, nil
}
