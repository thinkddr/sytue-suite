// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !js

package llamada

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"os"
	"testing"
	"time"

	lksdk "github.com/livekit/server-sdk-go/v2"
	"github.com/livekit/server-sdk-go/v2/e2ee"
	"github.com/thesyncim/govpx"
)

type videoCipherFixture struct { //nolint:tagliatelle // Cross-SDK artificial fixture schema.
	Artificial bool               `json:"artificial"`
	Key        string             `json:"key_b64"`
	Frames     []videoCipherFrame `json:"frames"`
}
type videoCipherFrame struct {
	Source     string `json:"source"`
	Type       string `json:"type"`
	Plaintext  string `json:"plaintext"`
	Ciphertext string `json:"ciphertext"`
}

// The same maintained VP8 encoder is used by the actual camera and screen tracks.
// These are real encoded key/delta frames, not a hand-written codec/crypto vector.
func TestNativeVideoE2EERealCameraAndScreenFrames(t *testing.T) {
	fixture := videoCipherFixture{Artificial: true, Key: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x4b}, 32))}
	kp := e2ee.NewExternalKeyProvider()
	if err := kp.SetKeyFromPassphrase(fixture.Key, 0); err != nil {
		t.Fatal(err)
	}
	enc, err := lksdk.NewFrameEncryptor(kp, lksdk.CodecVP8)
	if err != nil {
		t.Fatal(err)
	}
	dec, err := lksdk.NewFrameDecryptor(kp, lksdk.CodecVP8, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []struct {
		name          string
		width, height int
	}{{"camera", 640, 360}, {"screen", 1280, 720}} {
		encoder, err := govpx.NewVP8Encoder(govpx.EncoderOptions{Width: source.width, Height: source.height, FPS: 15, TargetBitrateKbps: 600, RateControlMode: govpx.RateControlCBR, Threads: 2, Deadline: govpx.DeadlineRealtime, CpuUsed: 8})
		if err != nil {
			t.Fatal(err)
		}
		decoder, err := govpx.NewVP8Decoder(govpx.DecoderOptions{MaxWidth: 1280, MaxHeight: 720})
		if err != nil {
			_ = encoder.Close()
			t.Fatal(err)
		}
		for number := range 2 {
			img := image.NewYCbCr(image.Rect(0, 0, source.width, source.height), image.YCbCrSubsampleRatio420)
			for i := range img.Y {
				img.Y[i] = byte(32 + (i%source.width)/16 + number*4)
			}
			for i := range img.Cb {
				img.Cb[i] = 100
				img.Cr[i] = 140
			}
			flags := govpx.EncodeFlags(0)
			if number == 0 {
				flags = govpx.EncodeForceKeyFrame
			}
			frame, err := encoder.EncodeInto(make([]byte, 1<<20), aGovpx(img), uint64(number), 1, flags)
			if err != nil || len(frame.Data) == 0 {
				t.Fatal("real VP8 encoder", err)
			}
			frameType := "delta"
			if frame.Data[0]&1 == 0 {
				frameType = "key"
			}
			if (number == 0) != (frameType == "key") {
				t.Fatal("fixture must contain one keyframe and one delta frame")
			}
			wire, err := enc.EncryptFrame(frame.Data)
			if err != nil {
				t.Fatal(err)
			}
			plaintext, err := dec.DecryptFrame(wire)
			if err != nil || !bytes.Equal(plaintext, frame.Data) {
				t.Fatal("real frame E2EE", err)
			}
			if err = decoder.Decode(plaintext); err != nil {
				t.Fatal("decrypted frame is not decodable VP8", err)
			}
			if _, ok := decoder.NextFrame(); !ok {
				t.Fatal("decrypted video frame unavailable")
			}
			fixture.Frames = append(fixture.Frames, videoCipherFrame{Source: source.name, Type: frameType, Plaintext: base64.StdEncoding.EncodeToString(frame.Data), Ciphertext: base64.StdEncoding.EncodeToString(wire)})
			for _, offset := range []int{0, len(wire) - 3, len(wire) - 2} {
				modified := bytes.Clone(wire)
				modified[offset] ^= 0x10
				if _, err = dec.DecryptFrame(modified); err == nil {
					t.Fatal("accepted altered header, nonce or trailer")
				}
			}
		}
		_ = encoder.Close()
		_ = decoder.Close()
	}
	if output := os.Getenv("SYTUE_LIVEKIT_GO_VIDEO_FIXTURE"); output != "" {
		data, err := json.MarshalIndent(fixture, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(output, append(data, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestNativeVideoDecryptsOfficialJSWorkerFrames(t *testing.T) {
	data, err := os.ReadFile("../../../test/fixtures/rooms-livekit-js-video.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture videoCipherFixture
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if !fixture.Artificial {
		t.Fatal("only artificial interop fixtures are allowed")
	}
	kp := e2ee.NewExternalKeyProvider()
	if err = kp.SetKeyFromPassphrase(fixture.Key, 0); err != nil {
		t.Fatal(err)
	}
	dec, err := lksdk.NewFrameDecryptor(kp, lksdk.CodecVP8, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(fixture.Frames) != 4 {
		t.Fatal("expected actual camera/screen key and delta frames")
	}
	for _, frame := range fixture.Frames {
		wire, err := base64.StdEncoding.DecodeString(frame.Ciphertext)
		if err != nil {
			t.Fatal(err)
		}
		expected, err := base64.StdEncoding.DecodeString(frame.Plaintext)
		if err != nil {
			t.Fatal(err)
		}
		actual, err := dec.DecryptFrame(wire)
		if err != nil || !bytes.Equal(actual, expected) {
			t.Fatal("official JS Worker frame does not decrypt", err)
		}
	}
}

func TestRoomsMediaNeedsValidKeyBeforeContactingSFU(t *testing.T) {
	for _, key := range []*CifradoMedios{nil, {Clave: "invalid", Caduca: time.Now().Add(time.Minute)}, {Clave: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32)), Caduca: time.Now().Add(-time.Second)}} {
		if _, err := Entrar("wss://never-contact.invalid", "artificial", Opciones{RequiereCifrado: true, Cifrado: key}); !errors.Is(err, ErrSinClaveMedios) {
			t.Fatal("invalid media key did not fail before network", err)
		}
	}
}

func TestExpiredMediaKeyCannotEncryptOrDecryptOneMoreFrame(t *testing.T) {
	expired := time.Now().Add(-time.Second)
	// Nil delegates prove the exact deadline is checked before touching SDK keys.
	if _, err := (deadlineEncryptor{until: expired}).EncryptFrame([]byte{1}); !errors.Is(err, ErrSinClaveMedios) {
		t.Fatal("expired outbound frame", err)
	}
	if _, err := (deadlineDecryptor{until: expired}).DecryptFrame([]byte{1}); !errors.Is(err, ErrSinClaveMedios) {
		t.Fatal("expired inbound frame", err)
	}
}
