// SPDX-License-Identifier: Apache-2.0
package storage

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"unicode/utf8"
)

// EncodeSourceCursor is shared by retained-source clients and servers. The
// opaque cursor does not authorize access; the authenticated scope and epoch do.
func EncodeSourceCursor(cursor LegacySourceCursor) (string, error) {
	if cursor == (LegacySourceCursor{}) {
		return "", nil
	}
	if !SourceCursorValid(cursor) {
		return "", ErrRecord
	}
	raw, err := json.Marshal(cursor)
	if err != nil {
		return "", ErrRecord
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func DecodeSourceCursor(encoded string) (LegacySourceCursor, error) {
	if encoded == "" {
		return LegacySourceCursor{}, nil
	}
	if len(encoded) > 8192 {
		return LegacySourceCursor{}, ErrRecord
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(encoded)
	if err != nil || !utf8.Valid(raw) || base64.RawURLEncoding.EncodeToString(raw) != encoded {
		return LegacySourceCursor{}, ErrRecord
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var cursor LegacySourceCursor
	if decoder.Decode(&cursor) != nil || decoder.Decode(new(any)) != io.EOF || !SourceCursorValid(cursor) {
		return LegacySourceCursor{}, ErrRecord
	}
	return cursor, nil
}
