// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mailbox

import (
	"context"
	"io"
	"mime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-message/charset"
)

func (p *protocolClient) imap(ctx context.Context) (*imapclient.Client, error) {
	if p.account.Provider != "imap" && p.account.Provider != "yahoo" {
		return nil, ErrUnsupported
	}
	s := p.account.Incoming
	conn, err := p.dial(ctx, s, false)
	if err != nil {
		return nil, err
	}
	c := imapclient.New(conn, &imapclient.Options{WordDecoder: &mime.WordDecoder{CharsetReader: charset.Reader}})
	if err := c.WaitGreeting(); err != nil {
		_ = c.Close()
		return nil, protocolError(err)
	}
	switch {
	case p.account.AccessToken != "":
		auth, authErr := oauthClient(p.username(s), p.account.AccessToken, s.Host, s.Port, func(mech string) bool { return c.Caps().Has(imap.AuthCap(mech)) })
		if authErr != nil {
			_ = c.Close()
			return nil, authErr
		}
		err = c.Authenticate(auth)
	case p.account.Password != "":
		err = c.Login(p.username(s), p.account.Password).Wait()
	default:
		err = ErrAuth
	}
	if err != nil {
		_ = c.Close()
		return nil, ErrAuth
	}
	return c, nil
}

func (p *protocolClient) imapFolders(ctx context.Context) ([]Folder, error) {
	c, err := p.imap(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = c.Close() }()
	cmd := c.List("", "*", nil)
	defer func() { _ = cmd.Close() }()
	var folders []Folder
	for item := cmd.Next(); item != nil; item = cmd.Next() {
		if slices.Contains(item.Attrs, imap.MailboxAttrNoSelect) {
			continue
		}
		if len(folders) >= 500 {
			return nil, ErrTooLarge
		}
		f := Folder{ID: item.Mailbox, Name: item.Mailbox}
		if strings.EqualFold(item.Mailbox, "INBOX") {
			f.Role = "inbox"
		}
		for _, role := range []string{"Sent", "Drafts", "Trash", "Junk", "Archive"} {
			if slices.Contains(item.Attrs, imap.MailboxAttr("\\"+role)) {
				f.Role = strings.ToLower(role)
			}
		}
		folders = append(folders, f)
	}
	if err := cmd.Close(); err != nil {
		return nil, protocolError(err)
	}
	// STATUS is issued after draining LIST: a streaming LIST must not deadlock
	// its reader while another command waits for that same connection.
	for i := range folders {
		status, statusErr := c.Status(folders[i].ID, &imap.StatusOptions{NumMessages: true, NumUnseen: true}).Wait()
		if statusErr != nil {
			return nil, protocolError(statusErr)
		}
		if status.NumMessages != nil {
			folders[i].Total = int64(*status.NumMessages)
		}
		if status.NumUnseen != nil {
			folders[i].Unread = int64(*status.NumUnseen)
		}
	}
	return folders, nil
}

func imapIdentity(validity uint32, uid imap.UID) string {
	return strconv.FormatUint(uint64(validity), 10) + ":" + strconv.FormatUint(uint64(uid), 10)
}

func parseIdentity(id string) (uint32, uint32, error) {
	parts := strings.Split(id, ":")
	if len(parts) != 2 {
		return 0, 0, ErrInvalid
	}
	a, err := strconv.ParseUint(parts[0], 10, 32)
	if err != nil || a == 0 {
		return 0, 0, ErrInvalid
	}
	b, err := strconv.ParseUint(parts[1], 10, 32)
	if err != nil || b == 0 {
		return 0, 0, ErrInvalid
	}
	return uint32(a), uint32(b), nil
}

func imapAddresses(list []imap.Address) []Address {
	result := make([]Address, 0, len(list))
	for _, a := range list {
		if a.Host != "" && a.Mailbox != "" {
			result = append(result, Address{Name: a.Name, Email: a.Mailbox + "@" + a.Host})
		}
	}
	return result
}

func (p *protocolClient) imapMessages(ctx context.Context, folder, cursor string) (Page, error) {
	if folder == "" || len(folder) > 1024 {
		return Page{}, ErrInvalid
	}
	c, err := p.imap(ctx)
	if err != nil {
		return Page{}, err
	}
	defer func() { _ = c.Close() }()
	selected, err := c.Select(folder, &imap.SelectOptions{ReadOnly: true}).Wait()
	if err != nil {
		return Page{}, ErrNotFound
	}
	upper := selected.NumMessages
	if cursor != "" {
		validity, seq, parseErr := parseIdentity(cursor)
		if parseErr != nil || validity != selected.UIDValidity {
			return Page{}, ErrInvalid
		}
		upper = min(upper, seq)
	}
	page := Page{Items: []Message{}}
	if upper == 0 {
		return page, nil
	}
	lower := uint32(1)
	if upper > PageSize {
		lower = upper - PageSize + 1
	}
	set := imap.SeqSet{{Start: lower, Stop: upper}}
	items, err := c.Fetch(set, &imap.FetchOptions{UID: true, Envelope: true, Flags: true, InternalDate: true}).Collect()
	if err != nil {
		return Page{}, protocolError(err)
	}
	slices.SortFunc(items, func(a, b *imapclient.FetchMessageBuffer) int { return int(b.SeqNum) - int(a.SeqNum) })
	for _, item := range items {
		m := Message{ID: imapIdentity(selected.UIDValidity, item.UID), FolderID: folder, Seen: slices.Contains(item.Flags, imap.FlagSeen), Flagged: slices.Contains(item.Flags, imap.FlagFlagged)}
		if e := item.Envelope; e != nil {
			m.Subject, m.From, m.To, m.Cc = e.Subject, imapAddresses(e.From), imapAddresses(e.To), imapAddresses(e.Cc)
			if !e.Date.IsZero() {
				m.Date = e.Date.UTC().Format(time.RFC3339)
			}
		}
		if m.Date == "" && !item.InternalDate.IsZero() {
			m.Date = item.InternalDate.UTC().Format(time.RFC3339)
		}
		page.Items = append(page.Items, m)
	}
	if lower > 1 {
		page.NextCursor = imapIdentity(selected.UIDValidity, imap.UID(lower-1))
	}
	return page, nil
}

func (p *protocolClient) imapMessage(ctx context.Context, folder, id string) (Message, error) {
	validity, uid, err := parseIdentity(id)
	if err != nil || folder == "" || len(folder) > 1024 {
		return Message{}, ErrInvalid
	}
	c, err := p.imap(ctx)
	if err != nil {
		return Message{}, err
	}
	defer func() { _ = c.Close() }()
	selected, err := c.Select(folder, &imap.SelectOptions{ReadOnly: true}).Wait()
	if err != nil || selected.UIDValidity != validity {
		return Message{}, ErrNotFound
	}
	section := &imap.FetchItemBodySection{Peek: true, Partial: &imap.SectionPartial{Size: MaxMessageBytes + 1}}
	cmd := c.Fetch(imap.UIDSetNum(imap.UID(uid)), &imap.FetchOptions{UID: true, Flags: true, BodySection: []*imap.FetchItemBodySection{section}})
	defer func() { _ = cmd.Close() }()
	var raw []byte
	var flags []imap.Flag
	for item := cmd.Next(); item != nil; item = cmd.Next() {
		for data := item.Next(); data != nil; data = item.Next() {
			switch data := data.(type) {
			case imapclient.FetchItemDataFlags:
				flags = data.Flags
			case imapclient.FetchItemDataBodySection:
				if data.Literal == nil {
					continue
				}
				raw, err = io.ReadAll(io.LimitReader(data.Literal, MaxMessageBytes+1))
				if err != nil {
					return Message{}, protocolError(err)
				}
				if len(raw) > MaxMessageBytes {
					_ = c.Close()
					return Message{}, ErrTooLarge
				}
			}
		}
	}
	if err := cmd.Close(); err != nil {
		return Message{}, protocolError(err)
	}
	if raw == nil {
		return Message{}, ErrNotFound
	}
	m, err := DecodeMessage(raw, id, folder)
	m.Seen, m.Flagged = slices.Contains(flags, imap.FlagSeen), slices.Contains(flags, imap.FlagFlagged)
	return m, err
}

func (p *protocolClient) imapAction(ctx context.Context, a Action) error {
	validity, uid, err := parseIdentity(a.MessageID)
	if err != nil || a.FolderID == "" || len(a.FolderID) > 1024 || len(a.Destination) > 1024 {
		return ErrInvalid
	}
	c, err := p.imap(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	selected, err := c.Select(a.FolderID, nil).Wait()
	if err != nil || selected.UIDValidity != validity {
		return ErrNotFound
	}
	set := imap.UIDSetNum(imap.UID(uid))
	var flag imap.Flag
	op := imap.StoreFlagsAdd
	switch a.Action {
	case "read":
		flag = imap.FlagSeen
	case "unread":
		flag, op = imap.FlagSeen, imap.StoreFlagsDel
	case "flag":
		flag = imap.FlagFlagged
	case "unflag":
		flag, op = imap.FlagFlagged, imap.StoreFlagsDel
	case "move":
		if a.Destination == "" {
			return ErrInvalid
		}
		// The library otherwise falls back to global EXPUNGE, potentially
		// removing another client's deleted messages. Require UID-safe support.
		if !c.Caps().Has(imap.CapMove) && !c.Caps().Has(imap.CapUIDPlus) {
			return ErrUnsupported
		}
		_, err := c.Move(set, a.Destination).Wait()
		return protocolError(err)
	default:
		return ErrUnsupported
	}
	return protocolError(c.Store(set, &imap.StoreFlags{Op: op, Flags: []imap.Flag{flag}, Silent: true}, nil).Close())
}

func (p *protocolClient) saveSent(ctx context.Context, raw []byte) bool {
	if p.account.Provider == "pop3" || p.account.Provider == "" {
		return false
	}
	c, err := p.imap(ctx)
	if err != nil {
		return false
	}
	defer func() { _ = c.Close() }()
	options := &imap.ListOptions{}
	if c.Caps().Has(imap.CapSpecialUse) {
		options.ReturnSpecialUse = true
	}
	cmd := c.List("", "*", options)
	sent, fallback := "", ""
	for count, item := 0, cmd.Next(); item != nil; count, item = count+1, cmd.Next() {
		if count >= 500 {
			_ = c.Close()
			return false
		}
		if slices.Contains(item.Attrs, imap.MailboxAttrNoSelect) {
			continue
		}
		if slices.Contains(item.Attrs, imap.MailboxAttrSent) {
			sent = item.Mailbox
		}
		if strings.EqualFold(item.Mailbox, "Sent") || strings.EqualFold(item.Mailbox, "Sent Items") || strings.EqualFold(item.Mailbox, "Sent Messages") {
			fallback = item.Mailbox
		}
	}
	if err := cmd.Close(); err != nil {
		return false
	}
	if sent == "" {
		sent = fallback
	}
	if sent == "" {
		return false
	}
	appendCmd := c.Append(sent, int64(len(raw)), &imap.AppendOptions{Flags: []imap.Flag{imap.FlagSeen}, Time: time.Now()})
	if _, err := appendCmd.Write(raw); err != nil {
		return false
	}
	if err := appendCmd.Close(); err != nil {
		return false
	}
	_, err = appendCmd.Wait()
	return err == nil
}
