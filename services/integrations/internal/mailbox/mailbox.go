// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package mailbox connects a person's external mailbox. The integration gateway
// owns its encrypted credentials and actor authorization; clients receive only
// mail content and connection metadata. No remote message is imported implicitly.
package mailbox

import (
	"context"
	"errors"
	"net/http"
)

const (
	MaxMessageBytes = 25 << 20
	PageSize        = 50
)

// Errors are stable categories. Provider replies and credentials must never be
// returned in API error bodies or logs.
var (
	ErrInvalid     = errors.New("mailbox: invalid request")
	ErrAuth        = errors.New("mailbox: reconnect account")
	ErrTLS         = errors.New("mailbox: TLS verification failed")
	ErrBlocked     = errors.New("mailbox: destination blocked")
	ErrUnavailable = errors.New("mailbox: provider unavailable")
	ErrUnsupported = errors.New("mailbox: operation unavailable")
	ErrTooLarge    = errors.New("mailbox: message too large")
	ErrNotFound    = errors.New("mailbox: message not found")
	// ErrSendUnknown means the connection failed after submission started. The UI
	// must ask the person to inspect Sent before sending again; never auto-retry.
	ErrSendUnknown = errors.New("mailbox: sending outcome unknown")
)

type Server struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	// Security is tls (implicit TLS) or starttls (mandatory upgrade, SMTP only).
	Security string `json:"security"`
}

// Account is server-only. Never serialize an Account or persist its passwords in
// instance Config: the gateway stores Password/SMTPPassword/AccessToken in vault.
type Account struct {
	Provider     string
	Address      string
	Incoming     Server
	Outgoing     Server
	Password     string
	SMTPPassword string
	AccessToken  string
}

type Address struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email"`
}

type Profile struct {
	Address string `json:"address"`
	Name    string `json:"name,omitempty"`
}

type Folder struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Role   string `json:"role,omitempty"`
	Total  int64  `json:"total,omitempty"`
	Unread int64  `json:"unread,omitempty"`
}

type Attachment struct {
	ID            string `json:"id,omitempty"`
	Name          string `json:"name"`
	Type          string `json:"type"`
	Size          int64  `json:"size,omitempty"`
	ContentBase64 string `json:"content_base64,omitempty"`
}

type Message struct {
	ID          string       `json:"id"`
	FolderID    string       `json:"folder_id,omitempty"`
	Subject     string       `json:"subject"`
	From        []Address    `json:"from"`
	To          []Address    `json:"to"`
	Cc          []Address    `json:"cc,omitempty"`
	Date        string       `json:"date"`
	Snippet     string       `json:"snippet,omitempty"`
	Text        string       `json:"text,omitempty"`
	HTML        string       `json:"html,omitempty"`
	Seen        bool         `json:"seen"`
	Flagged     bool         `json:"flagged"`
	Attachments []Attachment `json:"attachments,omitempty"`
}

type Page struct {
	Items      []Message `json:"items"`
	NextCursor string    `json:"next_cursor,omitempty"`
}

// Outgoing never accepts a From address. The authenticated provider account is
// the only sender, including when an arbitrary SMTP username differs from email.
type Outgoing struct {
	To          []Address    `json:"to"`
	Cc          []Address    `json:"cc,omitempty"`
	Bcc         []Address    `json:"bcc,omitempty"`
	Subject     string       `json:"subject"`
	Text        string       `json:"text"`
	HTML        string       `json:"html,omitempty"`
	Attachments []Attachment `json:"attachments,omitempty"`
	InReplyTo   string       `json:"in_reply_to,omitempty"`
}

type SendResult struct {
	ID string `json:"id,omitempty"`
	// False still means sent: saving the local Sent copy failed or is unavailable.
	SentCopySaved *bool `json:"sent_copy_saved,omitempty"`
}

type Action struct {
	FolderID    string `json:"folder_id"`
	MessageID   string `json:"message_id"`
	Action      string `json:"action"`
	Destination string `json:"destination,omitempty"`
}

// Client is the single contract used by the gateway, web and Fyne adapters.
// Test authenticates incoming and outgoing services without sending a message.
// POP3 is read-only: clients must expose that capability instead of offering
// move/flag/delete controls that the protocol cannot mirror safely.
type Client interface {
	Test(context.Context) (Profile, error)
	Folders(context.Context) ([]Folder, error)
	Messages(context.Context, string, string) (Page, error)
	Message(context.Context, string, string) (Message, error)
	Send(context.Context, Outgoing) (SendResult, error)
	Action(context.Context, Action) error
}

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}
