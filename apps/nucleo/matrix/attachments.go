// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"context"
	"errors"
	"io"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto/attachment"
	"maunium.net/go/mautrix/event"
)

// MaxAttachmentBytes bounds native memory for authenticated downloads. Encrypted
// uploads stream; neither filename nor MIME metadata is sent to the media server.
const MaxAttachmentBytes int64 = 50 << 20

type Attachment struct {
	Reader io.Reader
	Name   string
	MIME   string
	Size   int64
}

// UploadAttachment uploads only ciphertext and puts its key/IV/hash descriptor
// inside the Megolm-encrypted message. The returned receipt contains no keys.
func (c *Client) UploadAttachment(ctx context.Context, roomID string, file Attachment) (event.EncryptedFileInfo, error) {
	c.stateMu.RLock()
	roomErr := c.requireEncrypted(ctx, roomID)
	c.stateMu.RUnlock()
	if err := roomErr; err != nil {
		return event.EncryptedFileInfo{}, err
	}
	if file.Reader == nil || file.Name == "" || file.Size < 0 || file.Size > MaxAttachmentBytes {
		return event.EncryptedFileInfo{}, errors.New("matrix: invalid attachment")
	}
	encryptedFile := attachment.NewEncryptedFile()
	stream := encryptedFile.EncryptStream(io.LimitReader(file.Reader, file.Size))
	uploaded, err := c.cli.UploadMedia(ctx, mautrix.ReqUploadMedia{Content: stream, ContentLength: file.Size, ContentType: "application/octet-stream"})
	closeErr := stream.Close() // finalizes the authenticated SHA-256 descriptor
	if err != nil || closeErr != nil {
		return event.EncryptedFileInfo{}, errors.Join(err, closeErr)
	}
	if uploaded == nil || uploaded.ContentURI.IsEmpty() {
		return event.EncryptedFileInfo{}, errors.New("matrix: missing encrypted media receipt")
	}
	return event.EncryptedFileInfo{EncryptedFile: *encryptedFile, URL: uploaded.ContentURI.CUString()}, nil
}

func (c *Client) SendAttachment(ctx context.Context, roomID string, file Attachment) (EventRef, error) {
	descriptor, err := c.UploadAttachment(ctx, roomID, file)
	if err != nil {
		return EventRef{}, err
	}
	return c.send(ctx, roomID, event.MessageEventContent{MsgType: event.MsgFile, Body: file.Name, FileName: file.Name, Info: &event.FileInfo{MimeType: file.MIME, Size: int(file.Size)}, File: &descriptor})
}

// DownloadAttachment verifies the ciphertext hash before returning any plaintext.
// Streaming directly into a viewer would expose unauthenticated data before Close.
func (c *Client) DownloadAttachment(ctx context.Context, file event.EncryptedFileInfo) ([]byte, error) {
	if c.closed.Load() {
		return nil, ErrClosed
	}
	uri, err := file.URL.Parse()
	if err != nil {
		return nil, err
	}
	response, err := c.cli.Download(ctx, uri)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, MaxAttachmentBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > MaxAttachmentBytes {
		return nil, errors.New("matrix: encrypted attachment is too large")
	}
	if err = file.DecryptInPlace(data); err != nil {
		clear(data)
		return nil, err
	}
	return data, nil
}
