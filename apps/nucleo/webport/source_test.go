// SPDX-License-Identifier: Apache-2.0
package webport

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestRetainedSourceStreamsExactOriginalAndRejectsChangedCommitment(t *testing.T) {
	identity := fixtureIdentity()
	epoch := "018f3340-7b12-7000-8000-000000000001"
	original := bytes.Repeat([]byte("{\"precise\":900719925474099312345}\r\n"), 20000)
	digest := sha256.Sum256(original)
	row := storage.StreamSourceRow{Kind: "mime_message", ID: epoch, Size: int64(len(original)), SHA256: hex.EncodeToString(digest[:])}
	for _, mutant := range []string{"exact", "changed", "short", "extra"} {
		t.Run(mutant, func(t *testing.T) {
			closed, reads := false, 0
			port := storagePort{sourcePort: sourcePort{id: strings.Repeat("a", 64), identity: identity}, handle: func(method Method, raw []byte) ([]byte, error) {
				switch method {
				case SourceOpen:
					return json.Marshal(map[string]any{"stream": strings.Repeat("b", 64), "row": row})
				case SourceRead:
					reads++
					var input struct {
						Stream string
						Offset int64
						Length int
					}
					if json.Unmarshal(raw, &input) != nil || input.Length > ChunkBytes || input.Length < 0 || input.Offset < 0 {
						return nil, ErrRejected
					}
					if input.Length == 0 {
						if mutant == "extra" {
							return []byte("x"), nil
						}
						return nil, nil
					}
					value := bytes.Clone(original[input.Offset : input.Offset+int64(input.Length)])
					if mutant == "changed" {
						value[0] ^= 1
					}
					if mutant == "short" {
						value = value[:len(value)-1]
					}
					return value, nil
				case SourceClose:
					closed = true
					return nil, nil
				default:
					return nil, ErrRejected
				}
			}}
			owner, err := OpenStorage(t.Context(), port, NewSources())
			if err != nil {
				t.Fatal(err)
			}
			source, err := NewSource(owner, false)
			if err != nil {
				t.Fatal(err)
			}
			reader, err := source.OpenPrivateStorageSource(t.Context(), identity.Scope, epoch, row)
			if err != nil {
				t.Fatal(err)
			}
			var payload bytes.Buffer
			_, err = io.Copy(&payload, reader)
			actual := payload.Bytes()
			if closeErr := reader.Close(); closeErr != nil {
				t.Fatal(closeErr)
			}
			if mutant == "exact" {
				if err != nil || !bytes.Equal(actual, original) || reads < 3 {
					t.Fatal("exact original changed or unbounded read", err)
				}
			} else if err == nil {
				t.Fatal("changed/truncated/extra original accepted")
			}
			clear(actual)
			if !closed {
				t.Fatal("retained original reader leaked")
			}
		})
	}
}

func TestRetainedSourceCancellationJoinsActualReadAndErasesLateOutput(t *testing.T) {
	identity := fixtureIdentity()
	epoch := "018f3340-7b12-7000-8000-000000000001"
	checksum := sha256.Sum256([]byte("secret"))
	row := storage.StreamSourceRow{Kind: "mime_message", ID: epoch, Size: 6, SHA256: hex.EncodeToString(checksum[:])}
	entered, finish := make(chan struct{}), make(chan struct{})
	port := storagePort{sourcePort: sourcePort{id: strings.Repeat("a", 64), identity: identity}, handle: func(method Method, _ []byte) ([]byte, error) {
		switch method {
		case SourceOpen:
			return json.Marshal(map[string]any{"stream": strings.Repeat("b", 64), "row": row})
		case SourceRead:
			close(entered)
			<-finish
			return []byte("secret"), nil
		case SourceClose:
			return nil, nil
		default:
			return nil, ErrRejected
		}
	}}
	owner, _ := OpenStorage(t.Context(), port, NewSources())
	source, _ := NewSource(owner, false)
	reader, err := source.OpenPrivateStorageSource(t.Context(), identity.Scope, epoch, row)
	if err != nil {
		t.Fatal(err)
	}
	output := make([]byte, 6)
	readDone := make(chan error, 1)
	go func() {
		n, err := reader.Read(output)
		if n != 0 {
			err = errors.New("canceled reader exposed data")
		}
		readDone <- err
	}()
	<-entered
	closeDone := make(chan error, 1)
	go func() { closeDone <- reader.Close() }()
	select {
	case <-closeDone:
		t.Fatal("close released uncooperative read before join")
	case <-time.After(20 * time.Millisecond):
	}
	close(finish)
	if err = <-readDone; !errors.Is(err, context.Canceled) {
		t.Fatal("late read reported success", err)
	}
	if err = <-closeDone; err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(output, make([]byte, 6)) {
		t.Fatal("late source output leaked")
	}
}

func TestRetainedSourcePageUsesSingleNativeCursorCodecAndPinsScope(t *testing.T) {
	identity := fixtureIdentity()
	epoch := "018f3340-7b12-7000-8000-000000000001"
	calls := 0
	cursor := storage.LegacySourceCursor{Kind: "mailbox", ID: "018f3340-7b12-7000-8000-000000000002"}
	expected, _ := storage.EncodeSourceCursor(cursor)
	port := storagePort{sourcePort: sourcePort{id: strings.Repeat("a", 64), identity: identity}, handle: func(method Method, raw []byte) ([]byte, error) {
		calls++
		if method != SourcePage {
			return nil, ErrRejected
		}
		var input struct{ Epoch, After string }
		if json.Unmarshal(raw, &input) != nil || input.Epoch != epoch || input.After != expected {
			return nil, ErrRejected
		}
		return json.Marshal(storage.StreamSourcePage{Scope: identity.Scope, Epoch: epoch, Rows: []storage.StreamSourceRow{}})
	}}
	owner, _ := OpenStorage(t.Context(), port, NewSources())
	source, _ := NewSource(owner, false)
	if _, err := source.PrivateStorageSourcePage(t.Context(), identity.Scope, epoch, cursor); err != nil {
		t.Fatal(err)
	}
	foreign := identity.Scope
	foreign.Subject = "another"
	if _, err := source.PrivateStorageSourcePage(t.Context(), foreign, epoch, cursor); err == nil || calls != 1 {
		t.Fatal("foreign source reached host")
	}
	owner.identity.Scope.Product = "notes"
	if _, err := NewSource(owner, true); err == nil {
		t.Fatal("mail provisioning granted to another product")
	}
}
