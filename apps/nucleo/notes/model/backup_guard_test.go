// SPDX-License-Identifier: Apache-2.0
package model

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestEncryptedBackupGuardPreservesLegacySources(t *testing.T) {
	book := Notebook{Encrypted: true, Crypto: &CryptoParams{V: 1, KDF: "PBKDF2-SHA256", Salt: "abcdefghijklmnop", Iterations: 600000, Check: "encv1:test"}}
	clean := Note{Kind: KindText, ContentCipher: "encv1:AAAA", Data: json.RawMessage(`{"client_id":"capture"}`)}
	if err := ValidateEncryptedBackup(book, []Note{clean}); err != nil {
		t.Fatal(err)
	}
	parent := uuid.New()
	for name, change := range map[string]func(*Notebook, *Note){
		"title": func(_ *Notebook, n *Note) { n.Title = "secret" },
		"body":  func(_ *Notebook, n *Note) { n.Content = "secret" },
		"links": func(_ *Notebook, n *Note) {
			n.Links = []NoteLink{{TargetKind: "note", TargetID: "old", Label: "secret"}}
		},
		"data":               func(_ *Notebook, n *Note) { n.Data = json.RawMessage(`{"attachments":["secret"]}`) },
		"task controls":      func(_ *Notebook, n *Note) { n.Data = json.RawMessage(`{"repeated_into":"old"}`) },
		"raw data":           func(_ *Notebook, n *Note) { n.DataJSON = `{"secret":true}` },
		"states":             func(_ *Notebook, n *Note) { n.States = []NoteState{{Name: "secret"}} },
		"group":              func(_ *Notebook, n *Note) { n.GroupID = &parent },
		"parent":             func(b *Notebook, _ *Note) { b.ParentID = &parent },
		"missing key params": func(b *Notebook, _ *Note) { b.Crypto = nil },
		"unciphered":         func(_ *Notebook, n *Note) { n.ContentCipher = "" },
	} {
		t.Run(name, func(t *testing.T) {
			b, n := book, clean
			change(&b, &n)
			before, _ := json.Marshal([]any{b, n})
			if err := ValidateEncryptedBackup(b, []Note{n}); !errors.Is(err, ErrEncryptedBackupReview) {
				t.Fatalf("not rejected: %v", err)
			}
			after, _ := json.Marshal([]any{b, n})
			if !bytes.Equal(before, after) {
				t.Fatal("validation modified source")
			}
		})
	}
	plain := Notebook{Title: "plain"}
	plainNote := Note{Title: "original", Links: []NoteLink{{TargetKind: "note", TargetID: "x"}}}
	if err := ValidateEncryptedBackup(plain, []Note{plainNote}); err != nil {
		t.Fatal("plain backup changed:", err)
	}
	plainNote.ContentCipher = "encv1:AAAA"
	if err := ValidateEncryptedBackup(plain, []Note{plainNote}); !errors.Is(err, ErrEncryptedBackupReview) {
		t.Fatal("inconsistent ciphertext accepted", err)
	}
}

func TestEncryptedTransferRejectsUnknownOrAmbiguousSource(t *testing.T) {
	const base = `{"formato":"sytue.notepads","version":1,"notebook":{"title":"book","encrypted":true,"crypto":{"v":1,"kdf":"PBKDF2-SHA256","salt":"abcdefghijklmnop","iterations":600000,"check":"encv1:test"}},"notas":[{"kind":"text","content_cipher":"encv1:AAAA"}]}`
	if _, _, err := LeerNativoTransferencia([]byte(base)); err != nil {
		t.Fatal(err)
	}
	for _, changed := range []string{
		strings.Replace(base, `"notebook":`, `"extra":"secret","notebook":`, 1),
		strings.Replace(base, `"title":"book"`, `"title":"book","extra":"secret"`, 1),
		strings.Replace(base, `"kdf":"PBKDF2-SHA256"`, `"kdf":"PBKDF2-SHA256","extra":"secret"`, 1),
		strings.Replace(base, `"kind":"text"`, `"kind":"text","extra":"secret"`, 1),
		strings.Replace(base, `"encrypted":true`, `"encrypted":true,"Encrypted":false`, 1),
		strings.Replace(base, `"notas":`, `"notaſ":[],"notas":`, 1),
	} {
		if _, _, err := LeerNativoTransferencia([]byte(changed)); err == nil {
			t.Fatalf("unsafe archive accepted: %s", changed)
		}
	}
}

func TestEncryptedReadableFormatsFailRatherThanOmitCiphertext(t *testing.T) {
	book := Notebook{Encrypted: true}
	for _, format := range []FormatoExport{FormatoMarkdown, FormatoMarkdownLargo, FormatoTextoPlano, FormatoPDF} {
		if err := ValidateEncryptedBackupFormat(book, format); !errors.Is(err, ErrEncryptedBackupReview) {
			t.Fatalf("%s: %v", format, err)
		}
	}
	for _, format := range []FormatoExport{FormatoNativo, FormatoHTML} {
		if err := ValidateEncryptedBackupFormat(book, format); err != nil {
			t.Fatalf("%s: %v", format, err)
		}
	}
	if err := ValidateEncryptedBackupFormat(Notebook{}, FormatoPDF); err != nil {
		t.Fatal("clear export changed", err)
	}
}
