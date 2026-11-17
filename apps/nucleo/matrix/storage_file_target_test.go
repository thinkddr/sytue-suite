// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type countedTargetSource struct {
	io.ReaderAt
	calls int
}

func (s *countedTargetSource) ReadAt(out []byte, offset int64) (int, error) {
	s.calls++
	return s.ReaderAt.ReadAt(out, offset)
}

func TestStorageFileExplicitTargetPinsFirstPublicationAcrossRotation(t *testing.T) {
	f, gateway, records, _ := fileFixture(t)
	wanted := records.target
	records.target.RoomID = "!rotated:matrix.test"
	f.records = storageFileTarget{storageRecordIO: records, storagePublicationIO: records, target: wanted}
	records.loseRoot = true
	data := []byte("private immutable body follows the original intent")
	request := fileRequest(int64(len(data)))
	if _, err := f.upload(t.Context(), "explicit", request, bytes.NewReader(data)); err == nil {
		t.Fatal("lost root acknowledgment accepted")
	}
	ref, err := f.upload(t.Context(), "explicit", request, bytes.NewReader(data))
	if err != nil || ref.RoomID != wanted.RoomID || len(records.records) != 1 || gateway.puts != 1 {
		t.Fatal("first publication followed the rotated default or retry duplicated it", ref, err, gateway.puts)
	}
	var pending storageFileUpload
	found, err := f.journal.Read(f.journalOwner(), storage.PendingUploadsNamespace, "explicit", &pending)
	if err != nil || !found || pending.Target != wanted || !pending.Verified {
		t.Fatal("intent endpoint was not durably retained", err)
	}
}

func TestStorageFileExplicitTargetRejectsAnotherJobEndpointBeforeSourceOrProvider(t *testing.T) {
	f, gateway, records, _ := fileFixture(t)
	wanted := records.target
	data := []byte("retained pending body")
	request := fileRequest(int64(len(data)))
	gateway.loseNext = true
	if _, err := f.upload(t.Context(), "pending", request, bytes.NewReader(data)); err == nil {
		t.Fatal("fixture did not interrupt")
	}
	wrong := wanted
	wrong.RoomID = "!rotated:matrix.test"
	f.records = storageFileTarget{storageRecordIO: records, storagePublicationIO: records, target: wrong}
	probe := &countedTargetSource{ReaderAt: bytes.NewReader(data)}
	before := gateway.puts
	if _, err := f.upload(t.Context(), "pending", request, probe); !errors.Is(err, ErrIdentity) || probe.calls != 0 || gateway.puts != before {
		t.Fatal("another endpoint read or published pending work", err, probe.calls, gateway.puts)
	}
}

func TestStorageFileExplicitTargetRejectsInvalidFirstEndpointBeforeSource(t *testing.T) {
	for _, fault := range []string{"device", "room"} {
		t.Run(fault, func(t *testing.T) {
			f, gateway, records, _ := fileFixture(t)
			target := records.target
			if fault == "device" {
				target.DeviceID = "FOREIGN"
			} else {
				target.RoomID = "!foreign:matrix.test"
			}
			f.records = storageFileTarget{storageRecordIO: records, storagePublicationIO: records, target: target}
			data := []byte("private source")
			probe := &countedTargetSource{ReaderAt: bytes.NewReader(data)}
			if _, err := f.upload(t.Context(), "first", fileRequest(int64(len(data))), probe); !errors.Is(err, ErrIdentity) || probe.calls != 0 || gateway.puts != 0 || len(records.records) != 0 {
				t.Fatal("invalid target reached source or provider", err, probe.calls, gateway.puts)
			}
		})
	}
}

func TestStorageFileAtChecksTargetBeforeEncryptedStateAndKeys(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	client, err := newClient(storageConfig(t, server, "drive"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()
	client.session.DeviceID = "DEVICE"
	for _, target := range []storage.PublicationTarget{{RoomID: "!drive:matrix.test", DeviceID: "FOREIGN"}, {RoomID: "!foreign:matrix.test", DeviceID: "DEVICE"}} {
		probe := &countedTargetSource{ReaderAt: bytes.NewReader([]byte("private"))}
		if _, err := client.UploadStorageFileAt(t.Context(), nil, nil, "first", target, fileRequest(7), probe); !errors.Is(err, ErrIdentity) || calls.Load() != 0 || probe.calls != 0 {
			t.Fatal("target was checked after network, keys or source", err, calls.Load(), probe.calls)
		}
	}
}

var _ storage.RecordStore = (*Client)(nil)
