// SPDX-License-Identifier: Apache-2.0
// Package sessionproof identifies immutable SDK session attestations stored with
// maintained Matrix SSSS. It is a codec, not a cipher or a source of trust.
package sessionproof

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"unicode/utf8"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

const Kind = "org.sytue.matrix.session.v2"

var ErrProof = errors.New("matrix: invalid session provenance")

type Session struct {
	RoomID     string `json:"room_id"`
	SessionID  string `json:"session_id"`
	SenderKey  string `json:"sender_key"`
	SenderUser string `json:"sender_user"`
	SigningKey string `json:"signing_key"`
}
type (
	proof struct {
		Kind    string        `json:"kind"`
		Scope   storage.Scope `json:"scope"`
		Session Session       `json:"session"`
	}
	Encoded struct {
		ID      string
		Payload []byte
	}
)

func key(value string) bool {
	decoded, err := base64.RawStdEncoding.Strict().DecodeString(value)
	return err == nil && len(decoded) == 32 && base64.RawStdEncoding.EncodeToString(decoded) == value
}

func identifier(value string, prefix byte) bool {
	return len(value) > 1 && len(value) <= 255 && value[0] == prefix && utf8.ValidString(value) && !strings.ContainsAny(value, "\x00\r\n\t ")
}

func (s Session) Valid() bool {
	return identifier(s.RoomID, '!') && identifier(s.SenderUser, '@') && strings.IndexByte(s.SenderUser, ':') > 1 && key(s.SessionID) && key(s.SenderKey) && key(s.SigningKey)
}

func Encode(scope storage.Scope, session Session) (Encoded, error) {
	if !scope.Valid() || !session.Valid() {
		return Encoded{}, ErrProof
	}
	payload, err := json.Marshal(proof{Kind: Kind, Scope: scope, Session: session})
	if err != nil {
		return Encoded{}, ErrProof
	}
	digest := sha256.Sum256(payload)
	return Encoded{ID: "session2_" + hex.EncodeToString(digest[:]), Payload: payload}, nil
}

func DecodeSession(raw []byte) (Session, error) {
	var session Session
	if !decode(raw, &session) || !session.Valid() {
		return Session{}, ErrProof
	}
	return session, nil
}

func decode(raw []byte, out any) bool {
	if len(raw) == 0 || len(raw) > 4096 {
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(out) != nil {
		return false
	}
	var extra any
	return decoder.Decode(&extra) == io.EOF
}

// Matches accepts only the canonical bytes emitted by this shared Go codec.
// The SSSS envelope separately authenticates the ID, owner scope and revision.
func Matches(scope storage.Scope, session Session, payload []byte) bool {
	var parsed proof
	if !decode(payload, &parsed) || parsed.Kind != Kind || parsed.Scope != scope || parsed.Session != session {
		return false
	}
	expected, err := Encode(scope, session)
	return err == nil && bytes.Equal(payload, expected.Payload)
}
