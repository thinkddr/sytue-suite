// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !js

package llamada

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/livekit/protocol/livekit"
	"github.com/livekit/server-sdk-go/v2/e2ee"
	"google.golang.org/protobuf/proto"
)

type dataCipherFixture struct { //nolint:tagliatelle // Shared official SDK wire fixture.
	Artificial bool   `json:"artificial"`
	Generator  string `json:"generator"`
	Key        string `json:"key_b64"`
	Wire       string `json:"wire_packet"`
	Plaintext  string `json:"plaintext"`
}

func TestNativeDataE2EEOfficialJSWire(t *testing.T) {
	raw, err := os.ReadFile("../../../test/fixtures/rooms-livekit-js.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture dataCipherFixture
	if err = json.Unmarshal(raw, &fixture); err != nil || !fixture.Artificial {
		t.Fatal("artificial SDK fixture", err)
	}
	kp := e2ee.NewExternalKeyProvider()
	if err = kp.SetKeyFromPassphrase(fixture.Key, 0); err != nil {
		t.Fatal(err)
	}
	wire, err := base64.StdEncoding.DecodeString(fixture.Wire)
	if err != nil {
		t.Fatal(err)
	}
	var packet livekit.DataPacket
	if err = proto.Unmarshal(wire, &packet); err != nil {
		t.Fatal(err)
	}
	ep := packet.GetEncryptedPacket()
	if ep == nil || ep.GetEncryptionType() != livekit.Encryption_GCM {
		t.Fatal("fixture is not encrypted GCM")
	}
	dc := e2ee.NewDataCryptor(kp)
	actual, err := dc.Decrypt(ep)
	expected, _ := base64.StdEncoding.DecodeString(fixture.Plaintext)
	if err != nil || !bytes.Equal(actual.GetUser().GetPayload(), expected) {
		t.Fatal("official JS wire did not decrypt", err)
	}
	none := proto.Clone(ep).(*livekit.EncryptedPacket)
	none.EncryptionType = livekit.Encryption_NONE
	if _, err = dc.Decrypt(none); err == nil {
		t.Fatal("accepted NONE downgrade")
	}
	tampered := proto.Clone(ep).(*livekit.EncryptedPacket)
	tampered.EncryptedValue[0] ^= 1
	if _, err = dc.Decrypt(tampered); err == nil {
		t.Fatal("accepted tampered cipher")
	}
	encrypted, err := dc.Encrypt(&livekit.DataPacket{Value: &livekit.DataPacket_User{User: &livekit.UserPacket{Payload: expected}}})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := proto.Marshal(encrypted)
	if err != nil {
		t.Fatal(err)
	}
	out := dataCipherFixture{Artificial: true, Generator: "LiveKit server-sdk-go v2.19.0 DataCryptor", Key: fixture.Key, Wire: base64.StdEncoding.EncodeToString(encoded), Plaintext: fixture.Plaintext}
	if destination := os.Getenv("SYTUE_LIVEKIT_DATA_FIXTURE_OUT"); destination != "" {
		raw, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(destination, append(raw, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestEncryptedSignalsExpireBeforeHandler(t *testing.T) {
	l := &Llamada{o: Opciones{Cifrado: &CifradoMedios{Caduca: time.Now().Add(-time.Millisecond)}}}
	if l.claveVigente() {
		t.Fatal("expired call accepts data")
	}
	if err := l.Senal(Mano(true)); !errors.Is(err, ErrSinClaveMedios) {
		t.Fatal("expired call published data", err)
	}
}
