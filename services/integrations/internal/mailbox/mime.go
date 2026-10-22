// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mailbox

import (
	"bytes"
	"encoding/base64"
	"errors"
	"io"
	"mime"
	"net/mail"
	"strings"
	"time"
	"unicode"

	_ "github.com/emersion/go-message/charset" // Legacy mailbox charsets.

	gomail "github.com/emersion/go-message/mail"

	"git.sytue.eu/sytue/one/apps/nucleo/htmltext"
)

// DecodeMessage reads a complete external message, rather than the indexing
// envelope used by the local Mail service. HTML is untrusted content: renderers
// must sanitize it and disable remote images before displaying it.
func DecodeMessage(raw []byte, id, folder string) (Message, error) {
	if len(raw) > MaxMessageBytes {
		return Message{}, ErrTooLarge
	}
	r, err := gomail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		return Message{}, ErrInvalid
	}
	defer func() { _ = r.Close() }()
	m := Message{ID: id, FolderID: folder}
	m.Subject, _ = r.Header.Subject()
	if date, dateErr := r.Header.Date(); dateErr == nil && !date.IsZero() {
		m.Date = date.UTC().Format(time.RFC3339)
	}
	addresses := func(key string) []Address {
		list, _ := r.Header.AddressList(key)
		out := make([]Address, 0, len(list))
		for _, a := range list {
			out = append(out, Address{Name: a.Name, Email: a.Address})
		}
		return out
	}
	m.From, m.To, m.Cc = addresses("From"), addresses("To"), addresses("Cc")
	remaining := int64(MaxMessageBytes)
	for partCount := 0; ; partCount++ {
		p, partErr := r.NextPart()
		if errors.Is(partErr, io.EOF) {
			break
		}
		if partErr != nil || partCount >= 256 {
			return Message{}, ErrInvalid
		}
		data, readErr := io.ReadAll(io.LimitReader(p.Body, remaining+1))
		if readErr != nil {
			return Message{}, ErrInvalid
		}
		remaining -= int64(len(data))
		if remaining < 0 {
			return Message{}, ErrTooLarge
		}
		var name, mediaType string
		switch h := p.Header.(type) {
		case *gomail.InlineHeader:
			var params map[string]string
			mediaType, params, _ = h.ContentType()
			name = params["name"]
			if name == "" && mediaType == "text/plain" {
				m.Text += string(data)
				continue
			}
			if name == "" && mediaType == "text/html" {
				m.HTML += string(data)
				continue
			}
		case *gomail.AttachmentHeader:
			name, _ = h.Filename()
			mediaType, _, _ = h.ContentType()
		}
		if name == "" {
			name = "attachment"
		}
		m.Attachments = append(m.Attachments, Attachment{Name: name, Type: mediaType, Size: int64(len(data)), ContentBase64: base64.StdEncoding.EncodeToString(data)})
	}
	return finishDecodedMessage(m), nil
}

func finishDecodedMessage(m Message) Message {
	if strings.TrimSpace(m.Text) == "" && m.HTML != "" {
		m.Text = htmltext.Text(m.HTML)
	}
	snippet := []rune(strings.Join(strings.Fields(m.Text), " "))
	if len(snippet) > 280 {
		snippet = snippet[:280]
	}
	m.Snippet = string(snippet)
	return m
}

func validHeader(s string) bool {
	return !strings.ContainsFunc(s, unicode.IsControl)
}

func parsedAddress(a Address) (*gomail.Address, error) {
	if !validHeader(a.Name) || !validHeader(a.Email) {
		return nil, ErrInvalid
	}
	p, err := mail.ParseAddress(a.Email)
	if err != nil || p.Address != a.Email || !strings.Contains(p.Address, "@") {
		return nil, ErrInvalid
	}
	return &gomail.Address{Name: a.Name, Address: p.Address}, nil
}

// EncodeMessage fixes From to the connected account. Bcc is exclusively an
// envelope recipient and is never disclosed in the MIME header.
func EncodeMessage(from string, out Outgoing) ([]byte, error) {
	h, err := outgoingHeader(from, out)
	if err != nil {
		return nil, err
	}
	if len(out.Text)+len(out.HTML) > MaxMessageBytes {
		return nil, ErrTooLarge
	}
	var b bytes.Buffer
	w, err := gomail.CreateWriter(&b, h)
	if err != nil {
		return nil, ErrInvalid
	}
	if err := writeInline(w, out); err != nil {
		return nil, err
	}
	if err := writeAttachments(w, &b, out.Attachments); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, ErrInvalid
	}
	if b.Len() > MaxMessageBytes {
		return nil, ErrTooLarge
	}
	return b.Bytes(), nil
}

func outgoingHeader(from string, out Outgoing) (gomail.Header, error) {
	sender, err := parsedAddress(Address{Email: from})
	if err != nil || !validHeader(out.Subject) || len(out.Subject) > 998 || len(out.To)+len(out.Cc)+len(out.Bcc) == 0 || len(out.To)+len(out.Cc)+len(out.Bcc) > 100 || len(out.Attachments) > 100 {
		return gomail.Header{}, ErrInvalid
	}
	var h gomail.Header
	h.SetAddressList("From", []*gomail.Address{sender})
	for _, field := range []struct {
		key  string
		list []Address
	}{{"To", out.To}, {"Cc", out.Cc}, {"Bcc", out.Bcc}} {
		list := make([]*gomail.Address, 0, len(field.list))
		for _, a := range field.list {
			p, parseErr := parsedAddress(a)
			if parseErr != nil {
				return gomail.Header{}, parseErr
			}
			list = append(list, p)
		}
		if field.key != "Bcc" {
			h.SetAddressList(field.key, list)
		}
	}
	h.SetSubject(out.Subject)
	h.SetDate(time.Now())
	if err := h.GenerateMessageIDWithHostname(strings.SplitN(sender.Address, "@", 2)[1]); err != nil {
		return gomail.Header{}, ErrUnavailable
	}
	if out.InReplyTo != "" {
		if !validHeader(out.InReplyTo) {
			return gomail.Header{}, ErrInvalid
		}
		h.Set("In-Reply-To", "<"+strings.Trim(out.InReplyTo, "<>")+">")
		if ids, parseErr := h.MsgIDList("In-Reply-To"); parseErr != nil || len(ids) != 1 {
			return gomail.Header{}, ErrInvalid
		}
	}
	return h, nil
}

func writeInline(w *gomail.Writer, out Outgoing) error {
	inline, err := w.CreateInline()
	if err != nil {
		return ErrInvalid
	}
	for _, part := range []struct{ kind, body string }{{"text/plain", out.Text}, {"text/html", out.HTML}} {
		if part.kind == "text/html" && part.body == "" {
			continue
		}
		var ph gomail.InlineHeader
		ph.SetContentType(part.kind, map[string]string{"charset": "utf-8"})
		pw, partErr := inline.CreatePart(ph)
		if partErr != nil {
			return ErrInvalid
		}
		if _, partErr = io.WriteString(pw, part.body); partErr != nil {
			return ErrInvalid
		}
		if partErr = pw.Close(); partErr != nil {
			return ErrInvalid
		}
	}
	return inline.Close()
}

func writeAttachments(w *gomail.Writer, b *bytes.Buffer, attachments []Attachment) error {
	for _, a := range attachments {
		if !validHeader(a.Name) || len(a.Name) > 255 || len(a.ContentBase64) > MaxMessageBytes*4/3+4 {
			return ErrInvalid
		}
		mediaType, params, typeErr := mime.ParseMediaType(a.Type)
		if typeErr != nil || strings.HasPrefix(mediaType, "multipart/") {
			return ErrInvalid
		}
		data, decodeErr := base64.StdEncoding.DecodeString(a.ContentBase64)
		if decodeErr != nil {
			return ErrInvalid
		}
		if len(data)+b.Len() > MaxMessageBytes {
			return ErrTooLarge
		}
		var ah gomail.AttachmentHeader
		ah.SetContentType(mediaType, params)
		ah.SetFilename(a.Name)
		aw, partErr := w.CreateAttachment(ah)
		if partErr != nil {
			return ErrInvalid
		}
		if _, partErr = aw.Write(data); partErr != nil {
			return ErrInvalid
		}
		if partErr = aw.Close(); partErr != nil {
			return ErrInvalid
		}
	}
	return nil
}
