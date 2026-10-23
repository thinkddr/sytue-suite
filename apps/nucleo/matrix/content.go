// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"context"
	"encoding/json"
	"errors"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

var ErrUnsupported = errors.New("matrix: this action is unavailable for encrypted content")

//nolint:tagliatelle // The shared web/Fyne encrypted envelope uses camelCase.
type MessageAttachment struct {
	FileID      string                  `json:"fileId"`
	Name        string                  `json:"name"`
	ContentType string                  `json:"contentType"`
	SizeBytes   int64                   `json:"sizeBytes"`
	MatrixFile  event.EncryptedFileInfo `json:"matrixFile"`
}

//nolint:tagliatelle // The shared web/Fyne encrypted envelope uses camelCase.
type MessageContent struct {
	Version     int                 `json:"version"`
	Body        string              `json:"body"`
	Mentions    []string            `json:"mentions,omitempty"`
	Attachments []MessageAttachment `json:"attachments,omitempty"`
	Card        *api.RoomsCard      `json:"card,omitempty"`
	Priority    string              `json:"priority,omitempty"`
	RequiresAck bool                `json:"requiresAck,omitempty"`
}
type Relation struct {
	Thread   *EventRef
	Replaces *EventRef
}

func (c *Client) SendContent(ctx context.Context, roomID string, content MessageContent, relation Relation) (EventRef, error) {
	return c.SendContentWithTxn(ctx, roomID, content, relation, "")
}

// SendContentWithTxn retains the Matrix transaction across an explicit retry, so
// an accepted event whose receipt was lost cannot become a second message.
func (c *Client) SendContentWithTxn(ctx context.Context, roomID string, content MessageContent, relation Relation, transaction string) (EventRef, error) {
	content.Version = 1
	if err := validateContent(content); err != nil {
		return EventRef{}, err
	}
	payload := map[string]any{"msgtype": "m.text", "body": content.Body, "org.sytue.rooms": content}
	for kind, reference := range map[string]*EventRef{"m.thread": relation.Thread, "m.replace": relation.Replaces} {
		if reference == nil {
			continue
		}
		if reference.RoomID != roomID || reference.EventID == "" {
			return EventRef{}, ErrIdentity
		}
		if kind == "m.replace" {
			payload["m.new_content"] = map[string]any{"msgtype": "m.text", "body": content.Body, "org.sytue.rooms": content}
		}
		relationContent := map[string]any{"rel_type": kind, "event_id": reference.EventID}
		if kind == "m.thread" {
			relationContent["is_falling_back"] = false
		}
		payload["m.relates_to"] = relationContent
	}
	return c.sendEventWithTxn(ctx, roomID, event.EventMessage, payload, transaction)
}

func validateContent(content MessageContent) error {
	if content.Version != 1 || len(content.Attachments) > 10 || len(content.Body) > 1<<20 {
		return ErrIdentity
	}
	var total int64
	for _, file := range content.Attachments {
		if file.FileID != string(file.MatrixFile.URL) || file.Name == "" || file.ContentType == "" || file.SizeBytes < 0 || file.SizeBytes > MaxAttachmentBytes || file.MatrixFile.URL == "" {
			return ErrIdentity
		}
		total += file.SizeBytes
	}
	if total > MaxAttachmentBytes {
		return ErrIdentity
	}
	return nil
}

func DecodeMessage(value DecryptedEvent) (MessageContent, error) {
	if value.Type != event.EventMessage.Type {
		return MessageContent{}, ErrIdentity
	}
	var payload struct {
		Body     string                   `json:"body"`
		Own      *MessageContent          `json:"org.sytue.rooms"` //nolint:tagliatelle // Matrix namespaced extension.
		File     *event.EncryptedFileInfo `json:"file"`
		FileName string                   `json:"filename"`
		Info     *event.FileInfo          `json:"info"`
	}
	if err := json.Unmarshal(value.Content, &payload); err != nil {
		return MessageContent{}, err
	}
	content := MessageContent{Version: 1, Body: payload.Body}
	if payload.Own != nil {
		content = *payload.Own
		content.Body = payload.Body
	}
	if payload.File != nil && payload.Info != nil {
		content.Attachments = append(content.Attachments, MessageAttachment{FileID: string(payload.File.URL), Name: payload.FileName, ContentType: payload.Info.MimeType, SizeBytes: int64(payload.Info.Size), MatrixFile: *payload.File})
	}
	return content, validateContent(content)
}

// Read fetches one opaque receipt and authenticates/decrypts it locally.
func (c *Client) Read(ctx context.Context, reference EventRef) (DecryptedEvent, error) {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	if err := c.requireEncrypted(ctx, reference.RoomID); err != nil {
		return DecryptedEvent{}, err
	}
	return c.readEncryptedEvent(ctx, reference)
}

// ReadResult preserves the receipt order and isolates an unavailable or invalid
// event from the rest of a page. An error never carries decrypted content.
type ReadResult struct {
	Event DecryptedEvent
	Err   error
}

// ReadMany authenticates one bounded page in the same room. Like History, the
// room's encryption state is fetched once while sync/binding changes are locked;
// every receipt is still fetched, decrypted and device-verified independently.
// No encryption state or trust decision is retained between calls.
func (c *Client) ReadMany(ctx context.Context, roomID string, references []EventRef) ([]ReadResult, error) {
	if len(references) < 1 || len(references) > 100 {
		return nil, errors.New("matrix: invalid receipt page size")
	}
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	if err := c.requireEncrypted(ctx, roomID); err != nil {
		return nil, err
	}
	results := make([]ReadResult, len(references))
	for i, reference := range references {
		if reference.RoomID != roomID {
			results[i].Err = ErrIdentity
			continue
		}
		results[i].Event, results[i].Err = c.readEncryptedEvent(ctx, reference)
	}
	return results, nil
}

// The caller holds stateMu and has authenticated this room's encryption state.
func (c *Client) readEncryptedEvent(ctx context.Context, reference EventRef) (DecryptedEvent, error) {
	if reference.EventID == "" {
		return DecryptedEvent{}, ErrIdentity
	}
	if err := c.readReady(ctx); err != nil {
		return DecryptedEvent{}, err
	}
	encrypted, err := c.cli.GetEvent(ctx, id.RoomID(reference.RoomID), id.EventID(reference.EventID))
	if err != nil {
		return DecryptedEvent{}, err
	}
	if err := c.readReady(ctx); err != nil {
		return DecryptedEvent{}, err
	}
	if encrypted.Type != event.EventEncrypted || encrypted.ID.String() != reference.EventID || encrypted.RoomID != "" && encrypted.RoomID.String() != reference.RoomID {
		return DecryptedEvent{}, ErrIdentity
	}
	encrypted.RoomID = id.RoomID(reference.RoomID)
	if err := encrypted.Content.ParseRaw(encrypted.Type); err != nil {
		return DecryptedEvent{}, err
	}
	clearEvent, err := c.crypto.Decrypt(ctx, encrypted)
	if err != nil {
		return DecryptedEvent{}, err
	}
	return c.decryptedEvent(ctx, clearEvent)
}

func (c *Client) readReady(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if c.closed.Load() {
		return ErrClosed
	}
	if !c.ready.Load() {
		return ErrNotReady
	}
	return nil
}
