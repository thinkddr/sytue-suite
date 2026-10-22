// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"time"

	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// RoomsService keeps metadata/presence on the existing API and all new message
// content on Matrix. Embedding preserves domain operations, never a plaintext
// fallback for send/edit/reply, draft, upload, search, AI or link unfurl.
type RoomsService struct {
	*api.Rooms
	Client   func(context.Context) (*Client, error)
	mu       sync.Mutex
	epoch    uint64
	files    map[attachmentKey]cachedAttachment
	messages map[string]cachedMessage
}

type attachmentKey struct {
	client     *Client
	room, file string
}

type cachedAttachment struct {
	file      MessageAttachment
	channel   string
	temporary bool
}

type cachedMessage struct {
	value  api.RoomsMessage
	client *Client
	room   string
}

func NewRoomsService(base *api.Rooms, client func(context.Context) (*Client, error)) *RoomsService {
	return &RoomsService{Rooms: base, Client: client, files: map[attachmentKey]cachedAttachment{}, messages: map[string]cachedMessage{}}
}

func (r *RoomsService) revision() uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.epoch
}

func (r *RoomsService) connection(ctx context.Context, channel string) (*Client, string, error) {
	c, err := r.Client(ctx)
	if err != nil {
		return nil, "", err
	}
	room, err := c.RoomID(channel)
	if err != nil {
		next, readErr := r.MatrixBootstrap(ctx)
		if readErr != nil {
			return nil, "", readErr
		}
		if err := c.UpdateBindings(next); err != nil {
			return nil, "", err
		}
		room, err = c.RoomID(channel)
	}
	return c, room, err
}

func (r *RoomsService) hydrate(ctx context.Context, m api.RoomsMessage) (api.RoomsMessage, error) {
	epoch := r.revision()
	if m.MatrixEvent == nil {
		if m.Encryption != "legacy" && m.Type != "call" {
			return m, ErrNotEncrypted
		}
		return m, nil
	}
	c, room, err := r.connection(ctx, m.ChannelID)
	if err != nil {
		return m, err
	}
	if m.MatrixEvent.RoomID != room {
		return m, ErrIdentity
	}
	value, err := c.Read(ctx, *m.MatrixEvent)
	if err != nil {
		return m, err
	}
	ownID, _ := c.Identity()
	host := strings.SplitN(ownID, ":", 2)
	if len(host) != 2 || value.Sender != "@sytue_"+m.AuthorSub+":"+host[1] {
		return m, ErrIdentity
	}
	content, err := DecodeMessage(value)
	if err != nil {
		return m, err
	}
	m.Body, m.Mentions, m.Card, m.Priority, m.RequiresAck = content.Body, content.Mentions, content.Card, content.Priority, content.RequiresAck
	m.AuthenticityVerified = !value.Recovered
	m.IdentityVerified = value.Trust >= id.TrustStateCrossSignedVerified
	m.Attachments = nil
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.epoch != epoch || ctx.Err() != nil || c.closed.Load() {
		return api.RoomsMessage{}, ErrIdentity
	}
	for _, file := range content.Attachments {
		descriptor, _ := json.Marshal(file.MatrixFile)
		m.Attachments = append(m.Attachments, api.RoomsAttachment{FileID: file.FileID, Name: file.Name, ContentType: file.ContentType, SizeBytes: file.SizeBytes, MatrixFile: descriptor})
		r.files[attachmentKey{c, room, file.FileID}] = cachedAttachment{file: file, channel: m.ChannelID}
	}
	r.messages[m.ID] = cachedMessage{value: m, client: c, room: room}
	return m, nil
}

func (r *RoomsService) hydrateMany(ctx context.Context, messages []api.RoomsMessage) ([]api.RoomsMessage, error) {
	for i, m := range messages {
		value, err := r.hydrate(ctx, m)
		if err != nil {
			value = m
			value.Body = ""
			value.Attachments = nil
			value.Mentions = nil
			value.Card = nil
			value.EncryptionError = true
		}
		messages[i] = value
	}
	return messages, nil
}

func (r *RoomsService) Messages(ctx context.Context, channel string, before time.Time, limit int) ([]api.RoomsMessage, error) {
	messages, err := r.Rooms.Messages(ctx, channel, before, limit)
	if err != nil {
		return nil, err
	}
	return r.hydrateMany(ctx, messages)
}

func (r *RoomsService) Post(ctx context.Context, channel, text string) (api.RoomsMessage, error) {
	return r.PostWith(ctx, channel, text, api.RoomsSend{})
}

func (r *RoomsService) content(text string, options api.RoomsSend, c *Client, room string, epoch uint64) (MessageContent, error) {
	content := MessageContent{Version: 1, Body: text, Mentions: options.Mentions, Card: options.Card, Priority: options.Priority, RequiresAck: options.RequiresAck}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.epoch != epoch || c.closed.Load() {
		return MessageContent{}, ErrIdentity
	}
	for _, fileID := range options.FileIDs {
		file, ok := r.files[attachmentKey{c, room, fileID}]
		if !ok {
			return MessageContent{}, ErrIdentity
		}
		content.Attachments = append(content.Attachments, file.file)
	}
	return content, nil
}

func (r *RoomsService) PostWith(ctx context.Context, channel, text string, options api.RoomsSend) (api.RoomsMessage, error) {
	return r.postWith(ctx, channel, text, options, "")
}

// PostWithTxn publishes through the encrypted client with a stable Matrix
// transaction. It is intentionally absent from the plaintext API client.
func (r *RoomsService) PostWithTxn(ctx context.Context, channel, text, transaction string) (api.RoomsMessage, error) {
	if transaction == "" {
		return api.RoomsMessage{}, ErrIdentity
	}
	return r.postWith(ctx, channel, text, api.RoomsSend{}, transaction)
}

func (r *RoomsService) postWith(ctx context.Context, channel, text string, options api.RoomsSend, transaction string) (api.RoomsMessage, error) {
	epoch := r.revision()
	c, room, err := r.connection(ctx, channel)
	if err != nil {
		return api.RoomsMessage{}, err
	}
	content, err := r.content(text, options, c, room, epoch)
	if err != nil {
		return api.RoomsMessage{}, err
	}
	reference, err := c.SendContentWithTxn(ctx, room, content, Relation{}, transaction)
	if err != nil {
		return api.RoomsMessage{}, err
	}
	m, err := r.MatrixSend(ctx, channel, reference)
	if err != nil {
		return m, err
	}
	return r.hydrate(ctx, m)
}

func (r *RoomsService) ReplyWith(ctx context.Context, parent, text string, options api.RoomsSend) (api.RoomsMessage, error) {
	epoch := r.revision()
	thread, err := r.Rooms.Thread(ctx, parent)
	if err != nil {
		return api.RoomsMessage{}, err
	}
	if thread.Root.MatrixEvent == nil {
		return api.RoomsMessage{}, ErrUnsupported
	}
	c, room, err := r.connection(ctx, thread.Root.ChannelID)
	if err != nil {
		return api.RoomsMessage{}, err
	}
	content, err := r.content(text, options, c, room, epoch)
	if err != nil {
		return api.RoomsMessage{}, err
	}
	reference, err := c.SendContent(ctx, room, content, Relation{Thread: thread.Root.MatrixEvent})
	if err != nil {
		return api.RoomsMessage{}, err
	}
	m, err := r.MatrixReply(ctx, parent, reference)
	if err != nil {
		return m, err
	}
	return r.hydrate(ctx, m)
}

func (r *RoomsService) Edit(ctx context.Context, message, text string) (api.RoomsMessage, error) {
	epoch := r.revision()
	r.mu.Lock()
	cached, ok := r.messages[message]
	r.mu.Unlock()
	if !ok {
		thread, err := r.Thread(ctx, message)
		if err != nil {
			return api.RoomsMessage{}, err
		}
		if thread.Root.ID != message {
			return api.RoomsMessage{}, ErrIdentity
		}
		if thread.Root.MatrixEvent == nil {
			return thread.Root, ErrUnsupported
		}
		r.mu.Lock()
		cached, ok = r.messages[message]
		r.mu.Unlock()
		if !ok {
			return api.RoomsMessage{}, ErrIdentity
		}
	}
	m := cached.value
	if m.MatrixEvent == nil {
		return m, ErrUnsupported
	}
	c, room, err := r.connection(ctx, m.ChannelID)
	if err != nil {
		return m, err
	}
	if c != cached.client || room != cached.room {
		return api.RoomsMessage{}, ErrIdentity
	}
	options := api.RoomsSend{Mentions: m.Mentions, Card: m.Card, Priority: m.Priority, RequiresAck: m.RequiresAck}
	for _, file := range m.Attachments {
		options.FileIDs = append(options.FileIDs, file.FileID)
	}
	content, err := r.content(text, options, c, room, epoch)
	if err != nil {
		return api.RoomsMessage{}, err
	}
	reference, err := c.SendContent(ctx, room, content, Relation{Replaces: m.MatrixEvent})
	if err != nil {
		return m, err
	}
	m, err = r.MatrixEdit(ctx, message, reference)
	if err != nil {
		return m, err
	}
	return r.hydrate(ctx, m)
}

func (r *RoomsService) Thread(ctx context.Context, parent string) (api.RoomsThread, error) {
	thread, err := r.Rooms.Thread(ctx, parent)
	if err != nil {
		return thread, err
	}
	thread.Root, err = r.hydrate(ctx, thread.Root)
	if err != nil {
		return thread, err
	}
	thread.Replies, err = r.hydrateMany(ctx, thread.Replies)
	return thread, err
}

func (r *RoomsService) Around(ctx context.Context, channel, message string, limit int) ([]api.RoomsMessage, string, error) {
	messages, parent, err := r.Rooms.Around(ctx, channel, message, limit)
	if err != nil {
		return nil, parent, err
	}
	messages, err = r.hydrateMany(ctx, messages)
	return messages, parent, err
}

func (r *RoomsService) Pins(ctx context.Context, channel string) ([]api.RoomsMessage, error) {
	messages, err := r.Rooms.Pins(ctx, channel)
	if err != nil {
		return nil, err
	}
	return r.hydrateMany(ctx, messages)
}

func (r *RoomsService) Saved(ctx context.Context) ([]api.RoomsMessage, error) {
	messages, err := r.Rooms.Saved(ctx)
	if err != nil {
		return nil, err
	}
	return r.hydrateMany(ctx, messages)
}

// Fyne's existing attachment interface now transports ciphertext, retaining the
// decrypted descriptor only in this process until the encrypted message is sent.
func (r *RoomsService) UploadToChannel(ctx context.Context, channel, name, mime string, reader io.Reader, size int64) (api.File, error) {
	epoch := r.revision()
	c, room, err := r.connection(ctx, channel)
	if err != nil {
		return api.File{}, err
	}
	descriptor, err := c.UploadAttachment(ctx, room, Attachment{Reader: reader, Name: name, MIME: mime, Size: size})
	if err != nil {
		return api.File{}, err
	}
	file := MessageAttachment{FileID: string(descriptor.URL), Name: name, ContentType: mime, SizeBytes: size, MatrixFile: descriptor}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.epoch != epoch || ctx.Err() != nil || c.closed.Load() {
		return api.File{}, ErrIdentity
	}
	r.files[attachmentKey{c, room, file.FileID}] = cachedAttachment{file: file, channel: channel, temporary: true}
	return api.File{ID: file.FileID, Name: name, ContentType: mime, SizeBytes: size}, nil
}

func (r *RoomsService) ChannelContent(ctx context.Context, channel, fileID string, _ bool) (io.ReadCloser, error) {
	epoch := r.revision()
	c, room, err := r.connection(ctx, channel)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	file, ok := r.files[attachmentKey{c, room, fileID}]
	current := r.epoch == epoch
	r.mu.Unlock()
	if !ok || !current {
		return nil, ErrIdentity
	}
	data, err := c.DownloadAttachment(ctx, file.file.MatrixFile)
	if err != nil {
		return nil, err
	}
	if r.revision() != epoch || ctx.Err() != nil || c.closed.Load() {
		clear(data)
		return nil, ErrIdentity
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

// ForgetUpload removes only an uncommitted upload's process-local descriptor.
// Received message attachments remain scoped to their authenticated room.
func (r *RoomsService) ForgetUpload(channel, fileID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, value := range r.files {
		if key.file == fileID && value.channel == channel && value.temporary {
			delete(r.files, key)
		}
	}
}

// ForgetUploads clears abandoned draft uploads, never received attachments.
func (r *RoomsService) ForgetUploads(channel string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, value := range r.files {
		if value.channel == channel && value.temporary {
			delete(r.files, key)
		}
	}
}

func (*RoomsService) SaveChannelFileToMyDrive(context.Context, string, string) (api.File, error) {
	return api.File{}, ErrUnsupported
}

func (*RoomsService) Search(context.Context, string, int) ([]api.RoomsMessage, error) {
	return nil, ErrUnsupported
}

func (*RoomsService) Schedule(context.Context, string, string, []string, time.Time) (api.RoomsScheduled, error) {
	return api.RoomsScheduled{}, ErrUnsupported
}

func (*RoomsService) Reschedule(context.Context, string, time.Time) (api.RoomsScheduled, error) {
	return api.RoomsScheduled{}, ErrUnsupported
}

func (*RoomsService) ExecuteCommand(context.Context, string, string) (api.RoomsCommandResult, error) {
	return api.RoomsCommandResult{}, ErrUnsupported
}

func (*RoomsService) VoteCard(context.Context, string, string, bool) ([]api.RoomsCardVote, error) {
	return nil, ErrUnsupported
}

func (*RoomsService) Unfurl(context.Context, string) (api.RoomsUnfurl, error) {
	return api.RoomsUnfurl{}, ErrUnsupported
}

func (r *RoomsService) Reply(ctx context.Context, parent, text string) (api.RoomsMessage, error) {
	return r.ReplyWith(ctx, parent, text, api.RoomsSend{})
}

func (r *RoomsService) MyThreads(ctx context.Context, before string) ([]api.RoomsThreadSummary, string, error) {
	threads, next, err := r.Rooms.MyThreads(ctx, before)
	if err != nil {
		return nil, next, err
	}
	for i := range threads {
		threads[i].Root, err = r.hydrate(ctx, threads[i].Root)
		if err != nil {
			return nil, next, err
		}
	}
	return threads, next, nil
}

// Forget drops all plaintext descriptors when this identity/tenant is closed.
func (r *RoomsService) Forget() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.epoch++
	clear(r.files)
	clear(r.messages)
}
