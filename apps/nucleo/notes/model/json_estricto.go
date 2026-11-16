// SPDX-License-Identifier: Apache-2.0
package model

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ErrJSONAmbiguo does not include source keys or values, which may be private.
var ErrJSONAmbiguo = errors.New("notes: invalid or ambiguous JSON")

const (
	maxStrictJSONBytes    = 32 << 20
	maxStrictJSONDepth    = 256
	maxStrictJSONTokens   = 1_000_000
	maxStrictJSONKeyBytes = 64 << 10
)

// ValidarJSONSinClavesDuplicadas reads a single JSON value without modifying it
// or limiting numbers. It rejects inputs exceeding bounded scan resources. It rejects duplicate keys; esStruct selects paths where
// Go's Unicode case-insensitive struct matching also makes aliases ambiguous.
// A path contains decoded property names and "*" for each array element; the
// root path is empty. Nil esStruct applies exact key comparison everywhere.
// It caps input at 32 MiB; callers may apply a smaller size limit and must
// validate schema/unknown fields separately. Invalid UTF-8 is rejected.
func ValidarJSONSinClavesDuplicadas(raw []byte, esStruct func([]string) bool) error {
	if len(raw) > maxStrictJSONBytes || !utf8.Valid(raw) || !jsonUnicodeEscapesValid(raw) {
		return ErrJSONAmbiguo
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	state := strictJSONState{decoder: d, esStruct: esStruct}
	if err := state.read(nil); err != nil {
		return ErrJSONAmbiguo
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return ErrJSONAmbiguo
	}
	return nil
}

type strictJSONState struct {
	decoder  *json.Decoder
	esStruct func([]string) bool
	tokens   int
}

func (s *strictJSONState) read(path []string) error {
	if len(path) > maxStrictJSONDepth {
		return ErrJSONAmbiguo
	}
	s.tokens++
	if s.tokens > maxStrictJSONTokens {
		return ErrJSONAmbiguo
	}
	token, err := s.decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		return s.readObject(path)
	case '[':
		return s.readArray(path)
	default:
		return ErrJSONAmbiguo
	}
}

func (s *strictJSONState) readObject(path []string) error {
	seen := map[string]bool{}
	for s.decoder.More() {
		s.tokens++
		if s.tokens > maxStrictJSONTokens {
			return ErrJSONAmbiguo
		}
		keyToken, err := s.decoder.Token()
		if err != nil {
			return err
		}
		key, ok := keyToken.(string)
		if !ok || len(key) > maxStrictJSONKeyBytes {
			return ErrJSONAmbiguo
		}
		identity := key
		if s.esStruct != nil && s.esStruct(path) {
			identity = foldJSONKey(key)
		}
		if seen[identity] {
			return ErrJSONAmbiguo
		}
		seen[identity] = true
		if err := s.read(append(path, key)); err != nil {
			return err
		}
	}
	end, err := s.decoder.Token()
	if err != nil || end != json.Delim('}') {
		return ErrJSONAmbiguo
	}
	return nil
}

func (s *strictJSONState) readArray(path []string) error {
	for s.decoder.More() {
		if err := s.read(append(path, "*")); err != nil {
			return err
		}
	}
	end, err := s.decoder.Token()
	if err != nil || end != json.Delim(']') {
		return ErrJSONAmbiguo
	}
	return nil
}

// Equivalent to encoding/json's Unicode simple-fold for struct field names.
func foldJSONKey(key string) string {
	var out strings.Builder
	for _, r := range key {
		lowest := r
		for next := unicode.SimpleFold(r); next != r; next = unicode.SimpleFold(next) {
			if next < lowest {
				lowest = next
			}
		}
		out.WriteRune(lowest)
	}
	return out.String()
}

// encoding/json replaces lone surrogate escapes with U+FFFD instead of
// rejecting them. Check string escapes first so a later decode cannot silently
// alter an archived or journaled value. Syntax is still checked by the decoder.
func jsonUnicodeEscapesValid(raw []byte) bool {
	inString := false
	for i := 0; i < len(raw); i++ {
		if !inString {
			if raw[i] == '"' {
				inString = true
			}
			continue
		}
		if raw[i] == '"' {
			inString = false
			continue
		}
		if raw[i] != '\\' {
			continue
		}
		i++
		if i >= len(raw) {
			return false
		}
		if raw[i] != 'u' {
			continue
		}
		first, ok := jsonHex16(raw, i+1)
		if !ok {
			return false
		}
		i += 4
		if first >= 0xdc00 && first <= 0xdfff {
			return false
		}
		if first >= 0xd800 && first <= 0xdbff {
			if i+6 >= len(raw) || raw[i+1] != '\\' || raw[i+2] != 'u' {
				return false
			}
			second, ok := jsonHex16(raw, i+3)
			if !ok || second < 0xdc00 || second > 0xdfff {
				return false
			}
			i += 6
		}
	}
	return !inString
}

func jsonHex16(raw []byte, start int) (uint16, bool) {
	if start+4 > len(raw) {
		return 0, false
	}
	var value uint16
	for _, b := range raw[start : start+4] {
		value <<= 4
		switch {
		case b >= '0' && b <= '9':
			value |= uint16(b - '0')
		case b >= 'a' && b <= 'f':
			value |= uint16(b - 'a' + 10)
		case b >= 'A' && b <= 'F':
			value |= uint16(b - 'A' + 10)
		default:
			return 0, false
		}
	}
	return value, true
}
