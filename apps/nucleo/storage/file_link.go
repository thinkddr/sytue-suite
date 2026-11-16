// SPDX-License-Identifier: Apache-2.0
package storage

import "encoding/json"

const FileLinkKind = "org.sytue.file-link.v1"

// FileLink retains a verified file outside an application's index catalog.
// Its record uses role content and the same object/revision as the file. It
// contains no content key and remains inside maintained SDK encryption.
type FileLink struct {
	Kind    string         `json:"kind"`
	Content ContentPointer `json:"content"`
}

func (l FileLink) Validate() error {
	if l.Kind != FileLinkKind || l.Content.Kind != "file" || !l.Content.Valid() {
		return ErrRecord
	}
	return nil
}

func DecodeFileLink(record Record, scope Scope) (FileLink, error) {
	var link FileLink
	if record.Validate(scope) != nil || record.Role != "content" || len(record.Previous) != 0 || json.Unmarshal(record.Payload, &link) != nil || link.Validate() != nil || link.Content.RevisionID != record.RevisionID {
		return FileLink{}, ErrRecord
	}
	return link, nil
}
