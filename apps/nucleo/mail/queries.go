// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"slices"
	"strings"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

var _ api.MailData = (*Repository)(nil)

type MessageDocument struct {
	Kind      string `json:"kind"`
	Operation string `json:"operation,omitempty"`
	// Exact envelope JSON is retained as bytes, including imported extension
	// fields and integers that a generic JSON decoder must never round.
	Envelope []byte            `json:"envelope"`
	MIME     storage.Reference `json:"mime"`
}

const messageKind = "org.sytue.mail.message.v1"

func decodeMessage(payload []byte, id string) (api.MailMessage, MessageDocument, error) {
	var document MessageDocument
	var message api.MailMessage
	if json.Unmarshal(payload, &document) != nil || document.Kind != messageKind || !document.MIME.Valid() || json.Unmarshal(document.Envelope, &message) != nil || message.ID != id || message.MailboxID == "" || message.Size < 0 {
		return message, document, ErrRecord
	}
	return message, document, nil
}

func (r *Repository) messages(ctx context.Context, s storage.AtomicCatalogSnapshot) ([]api.MailMessage, error) {
	values, err := r.list(ctx, s, "message")
	if err != nil {
		return nil, err
	}
	defer clearDocuments(values)
	out := make([]api.MailMessage, 0, len(values))
	for id, payload := range values {
		value, doc, err := decodeMessage(payload, id)
		clear(doc.Envelope)
		if err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	slices.SortFunc(out, func(a, b api.MailMessage) int {
		if a.ReceivedAt.After(b.ReceivedAt) {
			return -1
		}
		if a.ReceivedAt.Before(b.ReceivedAt) {
			return 1
		}
		return strings.Compare(a.ID, b.ID)
	})
	counts := map[string]int{}
	for _, message := range out {
		counts[message.ThreadID]++
	}
	for i := range out {
		out[i].ThreadCount = counts[out[i].ThreadID]
	}
	return out, nil
}

func (r *Repository) Mailboxes(parent context.Context) ([]api.Mailbox, error) {
	var out []api.Mailbox
	err := r.run(parent, true, func(ctx context.Context, s storage.AtomicCatalogSnapshot) error {
		values, err := r.list(ctx, s, "mailbox")
		if err != nil {
			return err
		}
		defer clearDocuments(values)
		messages, err := r.messages(ctx, s)
		if err != nil {
			return err
		}
		for id, data := range values {
			var box api.Mailbox
			if json.Unmarshal(data, &box) != nil || box.ID != id {
				return ErrRecord
			}
			box.TotalEmails, box.UnreadEmails = 0, 0
			for _, message := range messages {
				if message.MailboxID == id {
					box.TotalEmails++
					if !message.Has(api.MailFlagSeen) {
						box.UnreadEmails++
					}
				}
			}
			out = append(out, box)
		}
		slices.SortFunc(out, func(a, b api.Mailbox) int {
			if n := strings.Compare(a.Name, b.Name); n != 0 {
				return n
			}
			return strings.Compare(a.ID, b.ID)
		})
		return nil
	})
	return out, err
}

type pageCursor struct {
	Scope   storage.Scope `json:"scope"`
	Kind    string        `json:"kind"`
	ID      string        `json:"id"`
	Time    string        `json:"time"`
	Message string        `json:"message"`
}

func (r *Repository) decodePageCursor(kind, id, cursor string) (pageCursor, time.Time, error) {
	var after pageCursor
	if cursor == "" {
		return after, time.Time{}, nil
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(cursor)
	if err != nil || len(raw) > 4096 || json.Unmarshal(raw, &after) != nil || after.Scope != r.cfg.Scope || after.Kind != kind || after.ID != id || after.Message == "" {
		return after, time.Time{}, ErrRecord
	}
	date, err := time.Parse(time.RFC3339Nano, after.Time)
	if err != nil {
		return after, date, ErrRecord
	}
	return after, date, nil
}

func (r *Repository) page(parent context.Context, kind, id, cursor string, limit int) ([]api.MailMessage, string, error) {
	var out []api.MailMessage
	next := ""
	if limit < 1 || limit > 50 {
		return nil, "", ErrRecord
	}
	after, date, err := r.decodePageCursor(kind, id, cursor)
	if err != nil {
		return nil, "", err
	}
	err = r.run(parent, true, func(ctx context.Context, s storage.AtomicCatalogSnapshot) error {
		values, err := r.messages(ctx, s)
		if err != nil {
			return err
		}
		for _, message := range values {
			if kind == "mailbox" && message.MailboxID != id || kind == "label" && !message.Has(id) {
				continue
			}
			if cursor != "" {
				if message.ReceivedAt.After(date) || message.ReceivedAt.Equal(date) && message.ID <= after.Message {
					continue
				}
			}
			if len(out) == limit {
				last := out[len(out)-1]
				data, _ := json.Marshal(pageCursor{Scope: r.cfg.Scope, Kind: kind, ID: id, Time: last.ReceivedAt.UTC().Format(time.RFC3339Nano), Message: last.ID})
				next = base64.RawURLEncoding.EncodeToString(data)
				break
			}
			out = append(out, message)
		}
		return nil
	})
	return out, next, err
}

func (r *Repository) Messages(ctx context.Context, mailbox, cursor string, limit int) ([]api.MailMessage, string, error) {
	return r.page(ctx, "mailbox", mailbox, cursor, limit)
}

func (r *Repository) LabelMessages(ctx context.Context, label, cursor string, limit int) ([]api.MailMessage, string, error) {
	return r.page(ctx, "label", label, cursor, limit)
}

func (r *Repository) Thread(parent context.Context, id string) ([]api.MailMessage, error) {
	var out []api.MailMessage
	err := r.run(parent, true, func(ctx context.Context, s storage.AtomicCatalogSnapshot) error {
		values, err := r.messages(ctx, s)
		if err != nil {
			return err
		}
		for _, message := range values {
			if message.ThreadID == id {
				out = append(out, message)
			}
		}
		return nil
	})
	return out, err
}

func (r *Repository) Search(parent context.Context, query string, offset, limit int) ([]api.MailMessage, error) {
	if offset < 0 || limit < 1 || limit > 50 || strings.Contains(query, ":") {
		return nil, api.ErrPrivateMailOperation
	}
	var out []api.MailMessage
	err := r.run(parent, true, func(ctx context.Context, s storage.AtomicCatalogSnapshot) error {
		values, err := r.messages(ctx, s)
		if err != nil {
			return err
		}
		for _, message := range values {
			text := strings.Join([]string{message.Subject, message.From, message.FromName, message.Preview, strings.Join(message.To, " "), strings.Join(message.Cc, " ")}, " ")
			if !buscar.Contiene(text, query) {
				continue
			}
			if offset > 0 {
				offset--
				continue
			}
			out = append(out, message)
			if len(out) == limit {
				break
			}
		}
		return nil
	})
	return out, err
}

func (r *Repository) Labels(parent context.Context) ([]api.MailLabel, error) {
	var out []api.MailLabel
	err := r.run(parent, true, func(ctx context.Context, s storage.AtomicCatalogSnapshot) error {
		values, err := r.list(ctx, s, "label")
		if err != nil {
			return err
		}
		defer clearDocuments(values)
		messages, err := r.messages(ctx, s)
		if err != nil {
			return err
		}
		for id, data := range values {
			var value api.MailLabel
			if json.Unmarshal(data, &value) != nil || value.ID != id {
				return ErrRecord
			}
			value.TotalEmails, value.UnreadEmails = 0, 0
			for _, message := range messages {
				if message.Has(id) {
					value.TotalEmails++
					if !message.Has(api.MailFlagSeen) {
						value.UnreadEmails++
					}
				}
			}
			out = append(out, value)
		}
		slices.SortFunc(out, func(a, b api.MailLabel) int { return strings.Compare(a.ID, b.ID) })
		return nil
	})
	return out, err
}

func (r *Repository) Signature(parent context.Context) (string, error) {
	var out string
	err := r.run(parent, true, func(ctx context.Context, s storage.AtomicCatalogSnapshot) error {
		data, err := r.document(ctx, s, "signature", "signature")
		if err != nil {
			return err
		}
		defer clear(data)
		if json.Unmarshal(data, &out) != nil {
			return ErrRecord
		}
		return nil
	})
	return out, err
}

// bodyReader clears its private MIME copy both on close and when either the
// requesting view or repository is cancelled. It is never an HTTP cache entry.
type bodyReader struct {
	mu                 sync.Mutex
	data               []byte
	reader             *bytes.Reader
	stream             io.ReadCloser
	cancel             context.CancelFunc
	closed             bool
	stopView, stopRepo func() bool
	workDone           func()
	streamDone         <-chan struct{}
}

func (b *bodyReader) Read(out []byte) (int, error) {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return 0, ErrClosed
	}
	if b.stream != nil {
		stream := b.stream
		b.mu.Unlock()
		return stream.Read(out)
	}
	n, err := b.reader.Read(out)
	b.mu.Unlock()
	return n, err
}

func (b *bodyReader) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	b.closed = true
	if b.cancel != nil {
		b.cancel()
	}
	if b.stream != nil {
		_ = b.stream.Close()
		b.stream = nil
	}
	if b.stopView != nil {
		b.stopView()
	}
	if b.stopRepo != nil {
		b.stopRepo()
	}
	clear(b.data)
	b.data = nil
	b.reader = nil
	if release := b.workDone; release != nil {
		b.workDone = nil
		if b.streamDone == nil {
			release()
		} else {
			go func() { <-b.streamDone; release() }()
		}
	}
	return nil
}

func (r *Repository) Raw(parent context.Context, id string) (io.ReadCloser, error) {
	readerCtx, release, err := r.start(parent)
	if err != nil {
		return nil, err
	}
	adopted := false
	defer func() {
		if !adopted {
			release()
		}
	}()
	var out io.ReadCloser
	err = r.runAdmitted(readerCtx, true, func(ctx context.Context, s storage.AtomicCatalogSnapshot) error {
		data, err := r.document(ctx, s, "message", id)
		if err != nil {
			return err
		}
		defer clear(data)
		envelope, message, err := decodeMessage(data, id)
		defer clear(message.Envelope)
		if err != nil {
			return err
		}
		version, err := r.readMIMEVersion(s, message.MIME, id)
		if err != nil {
			return err
		}
		b, err := r.openMIMEReader(ctx, readerCtx, version, envelope.Size)
		if err != nil {
			return err
		}
		r.retainBodyReader(readerCtx, b, release)
		out = b
		adopted = true
		return nil
	})
	return out, err
}

func (r *Repository) openMIMEReader(ctx, parent context.Context, version storage.ObjectVersion, size int64) (*bodyReader, error) {
	pointer := version.Commit.Content
	if version.Commit.State != "live" || pointer == nil || pointer.Kind != "file" {
		return nil, ErrRecord
	}
	info, err := r.cfg.Files.Info(ctx, pointer.Reference, version.ObjectID, pointer.RevisionID)
	if err != nil {
		return nil, err
	}
	if info.Validate(r.cfg.Scope) != nil || info.ObjectID != version.ObjectID || info.RevisionID != pointer.RevisionID || info.SizeBytes != size {
		return nil, ErrRecord
	}
	if size <= 1<<20 {
		raw, err := r.readDocument(ctx, version)
		if err != nil {
			return nil, err
		}
		if int64(len(raw)) != size {
			clear(raw)
			return nil, ErrRecord
		}
		return newBodyReader(raw), nil
	}
	// The run context ends when Raw returns; a stream instead inherits its caller
	// and is cancelled synchronously by the registered view/repository lifetime.
	streamCtx, cancel := context.WithCancel(parent)
	reader, writer := io.Pipe()
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		defer cancel()
		bounded := &sourceHashWriter{hash: writer, size: size}
		actual, downloadErr := r.cfg.Files.Download(streamCtx, pointer.Reference, version.ObjectID, pointer.RevisionID, bounded)
		before, _ := json.Marshal(info)
		after, _ := json.Marshal(actual)
		if downloadErr == nil && (bounded.n != size || !bytes.Equal(before, after)) {
			downloadErr = ErrRecord
		}
		_ = writer.CloseWithError(downloadErr)
	}()
	return &bodyReader{stream: reader, cancel: cancel, streamDone: finished}, nil
}

func (r *Repository) readMIMEVersion(s storage.AtomicCatalogSnapshot, ref storage.Reference, id string) (storage.ObjectVersion, error) {
	version, found, err := storage.AcceptedAtomicObject(s, r.cfg.Scope, objectID("mime", id), "", &ref)
	if err != nil || !found {
		return storage.ObjectVersion{}, ErrRecord
	}
	header, err := decodeHeader(version)
	if err != nil || header.Type != "mime" || header.ID != id {
		return version, ErrRecord
	}
	return version, nil
}

func newBodyReader(data []byte) *bodyReader {
	return &bodyReader{data: data, reader: bytes.NewReader(data)}
}

// retainBodyReader is called while the repository admission/mutex is held.
// The actual stream keeps that admission until cleanup has joined.
func (r *Repository) retainBodyReader(readerCtx context.Context, b *bodyReader, release func()) {
	b.mu.Lock()
	b.workDone = release
	b.stopView = context.AfterFunc(readerCtx, func() { _ = b.Close() })
	b.stopRepo = context.AfterFunc(r.ctx, func() { _ = b.Close() }) //nolint:contextcheck // The repository lifetime closes every open body.
	b.mu.Unlock()
	for reader := range r.readers {
		reader.mu.Lock()
		closed := reader.closed
		reader.mu.Unlock()
		if closed {
			delete(r.readers, reader)
		}
	}
	r.readers[b] = struct{}{}
}
