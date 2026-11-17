// SPDX-License-Identifier: Apache-2.0
package api

import (
	"bytes"
	"encoding/json"
	"unicode/utf8"

	"git.sytue.eu/sytue/one/apps/nucleo/notes/model"
)

// validarTextoEntrada prevents encoding/json from replacing invalid bytes before
// a preflight can see them. The rejected caller keeps its original input.
func (in NoteInput) validarTextoEntrada() error {
	for _, value := range []string{in.Kind, in.Title, in.Content, in.ContentCipher, in.Visibility} {
		if !utf8.ValidString(value) {
			return ErrDatosCifradosNotas
		}
	}
	if in.GroupID != nil && !utf8.ValidString(*in.GroupID) {
		return ErrDatosCifradosNotas
	}
	for _, link := range in.Links {
		for _, value := range []string{link.TargetKind, link.TargetID, link.Label, link.URL} {
			if !utf8.ValidString(value) {
				return ErrDatosCifradosNotas
			}
		}
	}
	return nil
}

// ValidarEntradaNotaJSON checks the original bytes before JSON's case folding
// and duplicate-key handling can erase evidence of an unsafe legacy input.
// It never alters the caller's journal.
func ValidarEntradaNotaJSON(raw []byte) error {
	if err := model.ValidarJSONSinClavesDuplicadas(raw, func(path []string) bool {
		return len(path) == 0 || len(path) == 1 && path[0] == "expected_crypto" || len(path) == 2 && path[0] == "links" && path[1] == "*"
	}); err != nil {
		return ErrDatosCifradosNotas
	}
	fields, ok := objetoNota(raw, map[string]bool{
		"cipher_version": true, "expected_crypto": true, "expected_updated_at": true, "kind": true, "title": true,
		"content": true, "content_cipher": true, "data": true, "x": true, "y": true,
		"col": true, "position": true, "width": true, "group_id": true, "collapsed": true,
		"checked": true, "cancelled": true, "pinned": true, "visibility": true,
		"indexable": true, "links": true,
	})
	if !ok {
		return ErrDatosCifradosNotas
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var in NoteInput
	if err := decoder.Decode(&in); err != nil {
		return ErrDatosCifradosNotas
	}
	for _, key := range []string{"cipher_version", "kind", "title", "content", "content_cipher", "visibility"} {
		if v, found := fields[key]; found && bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
			return ErrDatosCifradosNotas
		}
	}
	if err := validarExpectedCryptoNota(fields); err != nil {
		return err
	}
	if err := validarLinksNota(fields); err != nil {
		return err
	}
	return validarEscrituraCifradaNota(in)
}

func validarExpectedCryptoNota(fields map[string]json.RawMessage) error {
	crypto, found := fields["expected_crypto"]
	if !found {
		return nil
	}
	cryptoFields, ok := objetoNota(crypto, map[string]bool{"v": true, "kdf": true, "salt": true, "iterations": true, "check": true})
	if !ok {
		return ErrDatosCifradosNotas
	}
	for _, value := range cryptoFields {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return ErrDatosCifradosNotas
		}
	}
	var c NotesCrypto
	decoder := json.NewDecoder(bytes.NewReader(crypto))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&c) != nil {
		return ErrDatosCifradosNotas
	}
	return nil
}

func validarLinksNota(fields map[string]json.RawMessage) error {
	rawLinks, found := fields["links"]
	if !found || bytes.Equal(bytes.TrimSpace(rawLinks), []byte("null")) {
		return nil
	}
	var links []json.RawMessage
	if json.Unmarshal(rawLinks, &links) != nil || links == nil {
		return ErrDatosCifradosNotas
	}
	for _, link := range links {
		linkFields, ok := objetoNota(link, map[string]bool{"target_kind": true, "target_id": true, "label": true, "url": true})
		if !ok || linkFields["target_kind"] == nil || linkFields["target_id"] == nil {
			return ErrDatosCifradosNotas
		}
		for _, value := range linkFields {
			var text string
			if bytes.Equal(bytes.TrimSpace(value), []byte("null")) || json.Unmarshal(value, &text) != nil {
				return ErrDatosCifradosNotas
			}
		}
	}
	return nil
}

func validarEscrituraCifradaNota(in NoteInput) error {
	if !in.encryptedWrite() {
		return nil
	}
	if len(in.Links) != 0 || in.Title != "" || in.Content != "" {
		return ErrDatosCifradosNotas
	}
	if len(in.Data) == 0 || bytes.Equal(bytes.TrimSpace(in.Data), []byte("null")) {
		return nil
	}
	data, ok := objetoNota(in.Data, map[string]bool{"client_id": true})
	if !ok {
		return ErrDatosCifradosNotas
	}
	if client, found := data["client_id"]; found && !bytes.Equal(bytes.TrimSpace(client), []byte("null")) {
		var id string
		if json.Unmarshal(client, &id) != nil || !utf8.ValidString(id) || id == "" || len(id) > 64 {
			return ErrDatosCifradosNotas
		}
	}
	return nil
}

func objetoNota(raw []byte, allowed map[string]bool) (map[string]json.RawMessage, bool) {
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil || obj == nil {
		return nil, false
	}
	for key := range obj {
		if !allowed[key] {
			return nil, false
		}
	}
	return obj, true
}
