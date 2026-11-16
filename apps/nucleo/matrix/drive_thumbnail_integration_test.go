// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration && cgo

package matrix

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	privatedrive "git.sytue.eu/sytue/one/apps/nucleo/drive"
)

func exercisePrivateDriveThumbnails(ctx context.Context, t *testing.T, repository *privatedrive.Repository, open func() *privatedrive.Repository, faults *lostDriveImporterACK, source *syntheticDriveSource, gateway *fileGatewayFixture) *privatedrive.Repository {
	t.Helper()
	faults.contentLost, faults.catalogLost = true, true
	folder, err := repository.CreateFolder(ctx, source.Root, "Private thumbnail fixture")
	if err != nil {
		t.Fatal(err)
	}
	input := image.NewNRGBA(image.Rect(0, 0, 800, 400))
	for y := range 400 {
		for x := range 800 {
			input.SetNRGBA(x, y, color.NRGBA{R: 190, G: 35, B: 90, A: 128})
		}
	}
	var first api.File
	for _, format := range []string{"png", "jpeg", "gif"} {
		var encoded bytes.Buffer
		switch format {
		case "png":
			err = png.Encode(&encoded, input)
		case "jpeg":
			err = jpeg.Encode(&encoded, input, nil)
		case "gif":
			err = gif.Encode(&encoded, input, nil)
		}
		if err != nil {
			t.Fatal(err)
		}
		file, err := repository.Upload(ctx, folder.ID, "Private image."+format, "image/"+format, bytes.NewReader(encoded.Bytes()), int64(encoded.Len()))
		if err != nil {
			t.Fatal(err)
		}
		gateway.mu.Lock()
		puts := gateway.puts
		gateway.mu.Unlock()
		writes := faults.writes
		body, err := repository.Thumbnail(ctx, file.ID)
		if err != nil {
			t.Fatal(err)
		}
		preview, actual, err := image.Decode(body)
		_ = body.Close()
		want := format
		if format == "gif" {
			want = "png"
		}
		if err != nil || actual != want || preview.Bounds().Dx() != 512 || preview.Bounds().Dy() != 256 {
			t.Fatal("private derivative lost dimensions or format", format, actual, err)
		}
		if format == "png" {
			_, _, _, alpha := preview.At(10, 10).RGBA()
			if alpha < 32000 || alpha > 34000 {
				t.Fatal("thumbnail lost transparency", alpha)
			}
			first = file
		}
		gateway.mu.Lock()
		after := gateway.puts
		gateway.mu.Unlock()
		if puts != after || writes != faults.writes {
			t.Fatal("thumbnail published a server derivative")
		}
	}
	copied, err := repository.CopyFile(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Delete(ctx, api.DriveFile, first.ID); err != nil {
		t.Fatal(err)
	}
	repository.Close()
	repository = open()
	if err := repository.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	body, err := repository.Thumbnail(ctx, copied.ID)
	if err != nil {
		t.Fatal("copy lost captured content", err)
	}
	if _, err := io.Copy(io.Discard, body); err != nil {
		t.Fatal(err)
	}
	_ = body.Close()
	gateway.mu.Lock()
	gateway.tamper = true
	gateway.mu.Unlock()
	body, err = repository.Thumbnail(ctx, copied.ID)
	gateway.mu.Lock()
	gateway.tamper = false
	gateway.mu.Unlock()
	if body != nil || err == nil {
		if body != nil {
			_ = body.Close()
		}
		t.Fatal("tampered ciphertext produced a thumbnail")
	}
	if err := repository.Delete(ctx, api.DriveFolder, folder.ID); err != nil {
		t.Fatal(err)
	}
	gateway.mu.Lock()
	gets := gateway.gets
	gateway.mu.Unlock()
	body, err = repository.Thumbnail(ctx, copied.ID)
	if body != nil || !errors.Is(err, privatedrive.ErrNotFound) {
		t.Fatal("hidden descendant produced thumbnail", err)
	}
	gateway.mu.Lock()
	after := gateway.gets
	gateway.mu.Unlock()
	if gets != after {
		t.Fatal("hidden derivative read media")
	}
	if err := repository.Restore(ctx, api.DriveFolder, folder.ID); err != nil {
		t.Fatal(err)
	}
	bad, err := repository.Upload(ctx, folder.ID, "Invalid image.png", "image/png", bytes.NewBufferString("not an image"), 12)
	if err != nil {
		t.Fatal(err)
	}
	if body, err := repository.Thumbnail(ctx, bad.ID); body != nil || !errors.Is(err, privatedrive.ErrUnsupportedMedia) {
		t.Fatal("invalid private image accepted", err)
	}
	// Close with an unread derivative: the shared stream cancels and drains it.
	body, err = repository.Thumbnail(ctx, copied.ID)
	if err != nil {
		t.Fatal(err)
	}
	repository.Close()
	_ = body.Close()
	repository = open()
	if err := repository.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	t.Log("private PNG/JPEG/GIF thumbnails render on device, retain alpha and dimensions, survive copy/source trash/restart, reject tampered media and hidden descendants, publish no derivative and close without a reader")
	return repository
}
