// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mailbox

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net/textproto"
	"slices"
	"strconv"
	"strings"
)

// POP3 sessions never issue DELE. Reading mail leaves the server copy intact;
// POP3 cannot mirror folders or read/flag/move state across devices.
func (p *protocolClient) pop(ctx context.Context) (*textproto.Conn, error) {
	conn, err := p.dial(ctx, p.account.Incoming, false)
	if err != nil {
		return nil, err
	}
	c := textproto.NewConn(conn)
	fail := func(err error) (*textproto.Conn, error) { _ = c.Close(); return nil, err }
	if _, err := popReply(c); err != nil {
		return fail(protocolError(err))
	}
	username := p.username(p.account.Incoming)
	if username == "" || p.account.Password == "" || !validHeader(username) || !validHeader(p.account.Password) {
		return fail(ErrAuth)
	}
	if _, err := popCommand(c, "USER %s", username); err != nil {
		return fail(ErrAuth)
	}
	if _, err := popCommand(c, "PASS %s", p.account.Password); err != nil {
		return fail(ErrAuth)
	}
	return c, nil
}

func popReply(c *textproto.Conn) (string, error) {
	line, err := c.ReadLine()
	if err != nil {
		return "", err
	}
	if line != "+OK" && !strings.HasPrefix(line, "+OK ") {
		return "", ErrUnavailable
	}
	return strings.TrimSpace(strings.TrimPrefix(line, "+OK")), nil
}

func popCommand(c *textproto.Conn, format string, args ...any) (string, error) {
	if err := c.PrintfLine(format, args...); err != nil {
		return "", err
	}
	return popReply(c)
}

type popItem struct {
	sequence int
	uid      string
}

func popUIDs(c *textproto.Conn) ([]popItem, error) {
	if _, err := popCommand(c, "UIDL"); err != nil {
		return nil, protocolError(err)
	}
	var items []popItem
	seen := make(map[string]bool)
	r := c.DotReader()
	// Textproto's DotReader is streaming. Its enclosing bounded connection
	// limits wire data; UID count and line lengths are separately restricted.
	reader := textproto.NewReader(bufio.NewReader(r))
	for {
		line, err := reader.ReadLine()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, protocolError(err)
		}
		parts := strings.Fields(line)
		if len(parts) != 2 || len(parts[1]) > 70 || len(items) >= 100000 {
			return nil, ErrTooLarge
		}
		seq, parseErr := strconv.Atoi(parts[0])
		if parseErr != nil || seq <= 0 || seen[parts[1]] {
			return nil, ErrInvalid
		}
		seen[parts[1]] = true
		items = append(items, popItem{sequence: seq, uid: parts[1]})
	}
	slices.SortFunc(items, func(a, b popItem) int { return b.sequence - a.sequence })
	return items, nil
}

func (p *protocolClient) popFolders(ctx context.Context) ([]Folder, error) {
	c, err := p.pop(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = c.Close() }()
	stat, err := popCommand(c, "STAT")
	if err != nil {
		return nil, protocolError(err)
	}
	fields := strings.Fields(stat)
	if len(fields) != 2 {
		return nil, ErrInvalid
	}
	total, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil || total < 0 {
		return nil, ErrInvalid
	}
	return []Folder{{ID: "INBOX", Name: "INBOX", Role: "inbox", Total: total}}, nil
}

func popRetrieve(c *textproto.Conn, item popItem, headersOnly bool) ([]byte, error) {
	var err error
	if headersOnly {
		_, err = popCommand(c, "TOP %d 0", item.sequence)
		// TOP is optional. A server rejection is consumed; RETR is then safe.
		if err != nil && !errors.Is(err, ErrUnavailable) {
			return nil, protocolError(err)
		}
	}
	if !headersOnly || err != nil {
		if _, err := popCommand(c, "RETR %d", item.sequence); err != nil {
			return nil, protocolError(err)
		}
	}
	raw, err := io.ReadAll(io.LimitReader(c.DotReader(), MaxMessageBytes+1))
	if err != nil {
		return nil, protocolError(err)
	}
	if len(raw) > MaxMessageBytes {
		return nil, ErrTooLarge
	}
	return raw, nil
}

func (p *protocolClient) popMessages(ctx context.Context, folder, cursor string) (Page, error) {
	if folder != "INBOX" {
		return Page{}, ErrNotFound
	}
	c, err := p.pop(ctx)
	if err != nil {
		return Page{}, err
	}
	defer func() { _ = c.Close() }()
	items, err := popUIDs(c)
	if err != nil {
		return Page{}, err
	}
	start := 0
	if cursor != "" {
		index := slices.IndexFunc(items, func(item popItem) bool { return item.uid == cursor })
		if index < 0 {
			return Page{}, ErrInvalid
		}
		start = index + 1
	}
	end := min(start+PageSize, len(items))
	page := Page{Items: []Message{}}
	for _, item := range items[start:end] {
		raw, err := popRetrieve(c, item, true)
		if err != nil {
			return Page{}, err
		}
		m, err := DecodeMessage(raw, item.uid, folder)
		if err != nil {
			return Page{}, err
		}
		// Lists contain summaries only, including when TOP falls back to RETR.
		m.Text, m.HTML, m.Attachments = "", "", nil
		page.Items = append(page.Items, m)
	}
	if end < len(items) && end > start {
		page.NextCursor = items[end-1].uid
	}
	return page, nil
}

func (p *protocolClient) popMessage(ctx context.Context, folder, id string) (Message, error) {
	if folder != "INBOX" || id == "" || len(id) > 70 {
		return Message{}, ErrNotFound
	}
	c, err := p.pop(ctx)
	if err != nil {
		return Message{}, err
	}
	defer func() { _ = c.Close() }()
	items, err := popUIDs(c)
	if err != nil {
		return Message{}, err
	}
	for _, item := range items {
		if item.uid != id {
			continue
		}
		raw, err := popRetrieve(c, item, false)
		if err != nil {
			return Message{}, err
		}
		return DecodeMessage(raw, id, folder)
	}
	return Message{}, ErrNotFound
}
