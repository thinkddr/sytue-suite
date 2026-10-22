// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mailbox

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/mail"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Cloud clients use the gateway's authenticated egress runtime, including its
// SSRF, tenant policy, rate limit and audit trail. Never use http.DefaultClient.
type cloudClient struct {
	account Account
	http    HTTPDoer
}

func NewCloudClient(account Account, transport HTTPDoer) Client {
	return &cloudClient{account: account, http: transport}
}

func (c *cloudClient) request(ctx context.Context, method, target string, body, result any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return ErrInvalid
		}
		if len(encoded) > MaxMessageBytes*2 {
			return ErrTooLarge
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return ErrInvalid
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Prefer", `outlook.body-content-type="text"`)
	if reader != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := c.http.Do(req)
	if err != nil {
		if method == http.MethodPost && (strings.HasSuffix(target, "/send") || strings.HasSuffix(target, "/sendMail")) {
			return ErrSendUnknown
		}
		return ErrUnavailable
	}
	defer response.Body.Close()
	return decodeCloudResponse(response, method == http.MethodPost && strings.HasSuffix(target, "/send"), result)
}

func decodeCloudResponse(response *http.Response, sending bool, result any) error {
	switch response.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return ErrAuth
	case http.StatusNotFound:
		return ErrNotFound
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if response.StatusCode >= 500 && sending {
			return ErrSendUnknown
		}
		return ErrUnavailable
	}
	if result == nil || response.StatusCode == http.StatusNoContent || response.StatusCode == http.StatusAccepted {
		return nil
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, MaxMessageBytes*2+1))
	if err != nil || len(raw) > MaxMessageBytes*2 {
		if sending {
			return ErrSendUnknown
		}
		return ErrTooLarge
	}
	if json.Unmarshal(raw, result) != nil {
		if sending {
			return ErrSendUnknown
		}
		return ErrUnavailable
	}
	return nil
}

func validRemoteID(id string) bool {
	return id != "" && id != "." && id != ".." && len(id) <= 2048 && !strings.ContainsAny(id, "\r\n\x00")
}

func (c *cloudClient) Test(ctx context.Context) (Profile, error) {
	profile, err := c.profile(ctx, false)
	if err != nil {
		return profile, err
	}
	if c.account.Provider == "microsoft" {
		var folders json.RawMessage
		if err := c.request(ctx, http.MethodGet, graphBase+"/mailFolders?$top=1", nil, &folders); err != nil {
			return Profile{}, err
		}
	}
	return profile, nil
}

// OAuthProfile resolves the account identity without testing or opening a mailbox.
// Calendar, Contacts and Notes must work without consent to read or send mail.
func OAuthProfile(ctx context.Context, provider string, transport HTTPDoer) (Profile, error) {
	c := cloudClient{account: Account{Provider: provider}, http: transport}
	return c.profile(ctx, true)
}

func (c *cloudClient) profile(ctx context.Context, identityOnly bool) (Profile, error) {
	var profile Profile
	if identityOnly && (c.account.Provider == "gmail" || c.account.Provider == "yahoo") {
		target := "https://openidconnect.googleapis.com/v1/userinfo"
		if c.account.Provider == "yahoo" {
			target = "https://api.login.yahoo.com/openid/v1/userinfo"
		}
		var response struct {
			Email string `json:"email"`
			Name  string `json:"name"`
		}
		if err := c.request(ctx, http.MethodGet, target, nil, &response); err != nil {
			return profile, err
		}
		profile.Address, profile.Name = response.Email, response.Name
	} else {
		switch c.account.Provider {
		case "gmail":
			var response struct {
				Email string `json:"emailAddress"` //nolint:tagliatelle // Gmail wire format
			}
			if err := c.request(ctx, http.MethodGet, gmailBase+"/profile", nil, &response); err != nil {
				return profile, err
			}
			profile.Address = response.Email
		case "microsoft":
			var response struct {
				Mail string `json:"mail"`
				UPN  string `json:"userPrincipalName"` //nolint:tagliatelle // Graph wire format
				Name string `json:"displayName"`       //nolint:tagliatelle // Graph wire format
			}
			if err := c.request(ctx, http.MethodGet, graphBase+"?$select=mail,userPrincipalName,displayName", nil, &response); err != nil {
				return profile, err
			}
			profile.Address, profile.Name = response.Mail, response.Name
			if profile.Address == "" {
				profile.Address = response.UPN
			}
		default:
			return profile, ErrUnsupported
		}
	}
	parsed, err := mail.ParseAddress(profile.Address)
	if err != nil || parsed.Address != profile.Address {
		return Profile{}, ErrAuth
	}
	return profile, nil
}

const (
	gmailBase = "https://gmail.googleapis.com/gmail/v1/users/me"
	graphBase = "https://graph.microsoft.com/v1.0/me"
)

func (c *cloudClient) Folders(ctx context.Context) ([]Folder, error) {
	if c.account.Provider == "gmail" {
		var response struct {
			Labels []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
				Type string `json:"type"`
			} `json:"labels"`
		}
		if err := c.request(ctx, http.MethodGet, gmailBase+"/labels", nil, &response); err != nil {
			return nil, err
		}
		folders := []Folder{}
		roles := map[string]string{"INBOX": "inbox", "SENT": "sent", "DRAFT": "drafts", "TRASH": "trash", "SPAM": "junk"}
		for _, label := range response.Labels {
			if label.Type == "system" && roles[label.ID] == "" {
				continue
			}
			folders = append(folders, Folder{ID: label.ID, Name: label.Name, Role: roles[label.ID]})
		}
		return folders, nil
	}
	return c.graphFolders(ctx)
}

func (c *cloudClient) graphFolders(ctx context.Context) ([]Folder, error) {
	// Graph exposes child folders separately. Traverse the hierarchy with a fixed
	// cap; never silently present only top-level folders as the complete mailbox.
	var inbox struct {
		ID string `json:"id"`
	}
	if err := c.request(ctx, http.MethodGet, graphBase+"/mailFolders/inbox?$select=id", nil, &inbox); err != nil {
		return nil, err
	}
	folders := []Folder{}
	pending := []string{graphBase + "/mailFolders?$top=100&includeHiddenFolders=false"}
	for len(pending) > 0 {
		if len(folders) > 500 || len(pending) > 500 {
			return nil, ErrTooLarge
		}
		target := pending[0]
		pending = pending[1:]
		var response struct {
			Value []struct {
				ID     string `json:"id"`
				Name   string `json:"displayName"`      //nolint:tagliatelle // Graph wire format
				Total  int64  `json:"totalItemCount"`   //nolint:tagliatelle // Graph wire format
				Unread int64  `json:"unreadItemCount"`  //nolint:tagliatelle // Graph wire format
				Child  int    `json:"childFolderCount"` //nolint:tagliatelle // Graph wire format
			} `json:"value"`
			Next string `json:"@odata.nextLink"` //nolint:tagliatelle // Graph wire format
		}
		if err := c.request(ctx, http.MethodGet, target, nil, &response); err != nil {
			return nil, err
		}
		for _, item := range response.Value {
			role := ""
			if item.ID == inbox.ID {
				role = "inbox"
			}
			folders = append(folders, Folder{ID: item.ID, Name: item.Name, Total: item.Total, Unread: item.Unread, Role: role})
			if item.Child > 0 {
				pending = append(pending, graphBase+"/mailFolders/"+url.PathEscape(item.ID)+"/childFolders?$top=100")
			}
		}
		if response.Next != "" {
			u, err := url.Parse(response.Next)
			if err != nil || u.Scheme != "https" || u.Host != "graph.microsoft.com" || !strings.HasPrefix(u.Path, "/v1.0/me/mailFolders") || u.User != nil || u.Fragment != "" {
				return nil, ErrUnavailable
			}
			pending = append(pending, u.String())
		}
	}
	return folders, nil
}

func (c *cloudClient) Messages(ctx context.Context, folder, cursor string) (Page, error) {
	if !validRemoteID(folder) || len(cursor) > 8192 {
		return Page{}, ErrInvalid
	}
	if c.account.Provider == "microsoft" {
		return c.graphMessages(ctx, folder, cursor)
	}

	q := url.Values{"labelIds": {folder}, "maxResults": {strconv.Itoa(PageSize)}, "includeSpamTrash": {"true"}}
	if cursor != "" {
		q.Set("pageToken", cursor)
	}
	var response struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
		Next string `json:"nextPageToken"` //nolint:tagliatelle // Gmail wire format
	}
	if err := c.request(ctx, http.MethodGet, gmailBase+"/messages?"+q.Encode(), nil, &response); err != nil {
		return Page{}, err
	}
	if len(response.Messages) > PageSize {
		return Page{}, ErrUnavailable
	}
	// Gmail returns IDs only. Its official batch endpoint avoids fifty egress
	// connections (and exhausting the gateway's per-instance burst) per page.
	ids := make([]string, len(response.Messages))
	for index, item := range response.Messages {
		ids[index] = item.ID
	}
	items, err := c.gmailMetadataBatch(ctx, folder, ids)
	return Page{Items: items, NextCursor: response.Next}, err
}

func (c *cloudClient) graphMessages(ctx context.Context, folder, cursor string) (Page, error) {
	q := url.Values{"$top": {strconv.Itoa(PageSize)}, "$orderby": {"receivedDateTime desc"}, "$select": {"id,subject,from,toRecipients,ccRecipients,receivedDateTime,bodyPreview,isRead,flag"}}
	if cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil {
			return Page{}, ErrInvalid
		}
		next, err := url.ParseQuery(string(raw))
		if err != nil || next.Get("$skip") == "" && next.Get("$skiptoken") == "" {
			return Page{}, ErrInvalid
		}
		for _, key := range []string{"$skip", "$skiptoken"} {
			if value := next.Get(key); value != "" {
				q.Set(key, value)
			}
		}
	}
	var response struct {
		Value []graphMessage `json:"value"`
		Next  string         `json:"@odata.nextLink"` //nolint:tagliatelle // Graph wire format
	}
	if err := c.request(ctx, http.MethodGet, graphBase+"/mailFolders/"+url.PathEscape(folder)+"/messages?"+q.Encode(), nil, &response); err != nil {
		return Page{}, err
	}
	page := Page{Items: []Message{}}
	for _, item := range response.Value {
		page.Items = append(page.Items, item.message(folder))
	}
	if response.Next != "" {
		next, err := url.Parse(response.Next)
		if err != nil || next.Scheme != "https" || next.Host != "graph.microsoft.com" || next.User != nil || next.Fragment != "" || next.Path != "/v1.0/me/mailFolders/"+folder+"/messages" {
			return Page{}, ErrUnavailable
		}
		if next.Query().Get("$skiptoken") == "" && next.Query().Get("$skip") == "" {
			return Page{}, ErrUnavailable
		}
		page.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(next.RawQuery))
	}
	return page, nil
}

func (c *cloudClient) gmailMetadataBatch(ctx context.Context, folder string, ids []string) ([]Message, error) {
	items := make([]Message, 0, len(ids))
	if len(ids) == 0 {
		return items, nil
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for index, id := range ids {
		if !validRemoteID(id) {
			return nil, ErrUnavailable
		}
		header := textproto.MIMEHeader{"Content-Type": {"application/http"}, "Content-ID": {fmt.Sprintf("<item-%d>", index)}}
		part, err := writer.CreatePart(header)
		if err != nil {
			return nil, ErrUnavailable
		}
		if _, err := fmt.Fprintf(part, "GET /gmail/v1/users/me/messages/%s?format=metadata HTTP/1.1\r\nAccept: application/json\r\n\r\n", url.PathEscape(id)); err != nil {
			return nil, ErrUnavailable
		}
	}
	if err := writer.Close(); err != nil {
		return nil, ErrUnavailable
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://gmail.googleapis.com/batch/gmail/v1", &body)
	if err != nil {
		return nil, ErrInvalid
	}
	req.Header.Set("Content-Type", "multipart/mixed; boundary="+writer.Boundary())
	response, err := c.http.Do(req)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return nil, ErrAuth
	}
	if response.StatusCode != http.StatusOK {
		return nil, ErrUnavailable
	}
	mediaType, params, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/mixed" || params["boundary"] == "" {
		return nil, ErrUnavailable
	}
	// Bound the complete untrusted batch, including MIME headers, before parsing.
	raw, err := io.ReadAll(io.LimitReader(response.Body, 8<<20+1))
	if err != nil || len(raw) > 8<<20 {
		return nil, ErrTooLarge
	}
	reader := multipart.NewReader(bytes.NewReader(raw), params["boundary"])
	for index, id := range ids {
		wire, err := readGmailBatchPart(reader, index, id)
		if err != nil {
			return nil, err
		}
		items = append(items, wire.message(folder))
	}
	if _, err := reader.NextPart(); err != io.EOF {
		return nil, ErrUnavailable
	}
	return items, nil
}

func readGmailBatchPart(reader *multipart.Reader, index int, id string) (gmailMessage, error) {
	var wire gmailMessage
	part, err := reader.NextPart()
	if err != nil {
		return wire, ErrUnavailable
	}
	if contentID := part.Header.Get("Content-ID"); contentID != "" && contentID != fmt.Sprintf("<response-item-%d>", index) {
		return wire, ErrUnavailable
	}
	nested, err := http.ReadResponse(bufio.NewReader(part), nil)
	if err != nil {
		return wire, ErrUnavailable
	}
	data, readErr := io.ReadAll(io.LimitReader(nested.Body, 1<<20+1))
	_ = nested.Body.Close()
	_ = part.Close()
	if nested.StatusCode == http.StatusUnauthorized || nested.StatusCode == http.StatusForbidden {
		return wire, ErrAuth
	}
	if nested.StatusCode != http.StatusOK || readErr != nil || len(data) > 1<<20 {
		return wire, ErrUnavailable
	}
	if json.Unmarshal(data, &wire) != nil || wire.ID != id {
		return wire, ErrUnavailable
	}
	return wire, nil
}

func (c *cloudClient) Message(ctx context.Context, folder, id string) (Message, error) {
	if !validRemoteID(id) {
		return Message{}, ErrInvalid
	}
	if c.account.Provider == "microsoft" {
		var response graphMessage
		if err := c.request(ctx, http.MethodGet, graphBase+"/messages/"+url.PathEscape(id), nil, &response); err != nil {
			return Message{}, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, graphBase+"/messages/"+url.PathEscape(id)+"/$value", http.NoBody)
		if err != nil {
			return Message{}, ErrInvalid
		}
		reply, err := c.http.Do(req)
		if err != nil {
			return Message{}, ErrUnavailable
		}
		defer reply.Body.Close()
		if reply.StatusCode == http.StatusUnauthorized || reply.StatusCode == http.StatusForbidden {
			return Message{}, ErrAuth
		}
		if reply.StatusCode != http.StatusOK {
			return Message{}, ErrUnavailable
		}
		raw, err := io.ReadAll(io.LimitReader(reply.Body, MaxMessageBytes+1))
		if err != nil || len(raw) > MaxMessageBytes {
			return Message{}, ErrTooLarge
		}
		message, err := DecodeMessage(raw, id, folder)
		message.Seen, message.Flagged = response.Seen, response.Flag.Status == "flagged"
		return message, err
	}
	var response struct {
		Raw    string   `json:"raw"`
		Labels []string `json:"labelIds"` //nolint:tagliatelle // Gmail wire format
	}
	if err := c.request(ctx, http.MethodGet, gmailBase+"/messages/"+url.PathEscape(id)+"?format=raw", nil, &response); err != nil {
		return Message{}, err
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(response.Raw, "="))
	if err != nil || len(raw) > MaxMessageBytes {
		return Message{}, ErrTooLarge
	}
	message, err := DecodeMessage(raw, id, folder)
	message.Seen, message.Flagged = true, false
	for _, label := range response.Labels {
		if label == "UNREAD" {
			message.Seen = false
		}
		if label == "STARRED" {
			message.Flagged = true
		}
	}
	return message, err
}

func (c *cloudClient) Send(ctx context.Context, out Outgoing) (SendResult, error) {
	raw, err := EncodeMessage(c.account.Address, out)
	if err != nil {
		return SendResult{}, err
	}
	// SMTP's wire MIME omits Bcc: those recipients belong only in its envelope.
	// Gmail/Graph MIME APIs instead derive the envelope from these headers, so
	// include validated Bcc only in the authenticated API request to the provider.
	if len(out.Bcc) > 0 {
		addresses := []string{}
		for _, recipient := range out.Bcc {
			address, err := mail.ParseAddress(recipient.Email)
			if err != nil || address.Address != recipient.Email || strings.ContainsAny(recipient.Name, "\r\n") {
				return SendResult{}, ErrInvalid
			}
			addresses = append(addresses, (&mail.Address{Name: recipient.Name, Address: recipient.Email}).String())
		}
		raw = append([]byte("Bcc: "+strings.Join(addresses, ", ")+"\r\n"), raw...)
	}
	if len(raw) > MaxMessageBytes {
		return SendResult{}, ErrTooLarge
	}
	if c.account.Provider == "gmail" {
		var result SendResult
		err := c.request(ctx, http.MethodPost, gmailBase+"/messages/send", map[string]string{"raw": base64.RawURLEncoding.EncodeToString(raw)}, &result)
		return result, err
	}
	// MIME preserves the same validated sender, MIME attachments and reply headers
	// on both providers. Graph accepts a base64 MIME body for sendMail.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, graphBase+"/sendMail", strings.NewReader(base64.StdEncoding.EncodeToString(raw)))
	if err != nil {
		return SendResult{}, ErrInvalid
	}
	req.Header.Set("Content-Type", "text/plain")
	response, err := c.http.Do(req)
	if err != nil {
		return SendResult{}, ErrSendUnknown
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return SendResult{}, ErrAuth
	}
	if response.StatusCode != http.StatusAccepted {
		if response.StatusCode >= 500 {
			return SendResult{}, ErrSendUnknown
		}
		return SendResult{}, ErrUnavailable
	}
	return SendResult{}, nil
}

func (c *cloudClient) Action(ctx context.Context, action Action) error {
	if !validRemoteID(action.MessageID) {
		return ErrInvalid
	}
	if c.account.Provider == "microsoft" {
		target := graphBase + "/messages/" + url.PathEscape(action.MessageID)
		switch action.Action {
		case "read", "unread":
			return c.request(ctx, http.MethodPatch, target, map[string]bool{"isRead": action.Action == "read"}, nil)
		case "flag", "unflag":
			status := "notFlagged"
			if action.Action == "flag" {
				status = "flagged"
			}
			return c.request(ctx, http.MethodPatch, target, map[string]any{"flag": map[string]string{"flagStatus": status}}, nil)
		case "trash":
			return c.request(ctx, http.MethodPost, target+"/move", map[string]string{"destinationId": "deleteditems"}, nil)
		case "move":
			if !validRemoteID(action.Destination) {
				return ErrInvalid
			}
			return c.request(ctx, http.MethodPost, target+"/move", map[string]string{"destinationId": action.Destination}, nil)
		}
		return ErrUnsupported
	}
	target := gmailBase + "/messages/" + url.PathEscape(action.MessageID)
	if action.Action == "trash" {
		return c.request(ctx, http.MethodPost, target+"/trash", map[string]any{}, nil)
	}
	add, remove := []string{}, []string{}
	switch action.Action {
	case "read":
		remove = append(remove, "UNREAD")
	case "unread":
		add = append(add, "UNREAD")
	case "flag":
		add = append(add, "STARRED")
	case "unflag":
		remove = append(remove, "STARRED")
	case "move":
		if !validRemoteID(action.Destination) || !validRemoteID(action.FolderID) {
			return ErrInvalid
		}
		add, remove = append(add, action.Destination), append(remove, action.FolderID)
	default:
		return ErrUnsupported
	}
	return c.request(ctx, http.MethodPost, target+"/modify", map[string]any{"addLabelIds": add, "removeLabelIds": remove}, nil)
}

type gmailMessage struct {
	ID      string   `json:"id"`
	Snippet string   `json:"snippet"`
	Labels  []string `json:"labelIds"` //nolint:tagliatelle // Gmail wire format
	Payload struct {
		Headers []struct{ Name, Value string } `json:"headers"`
	} `json:"payload"`
}

func (wire gmailMessage) message(folder string) Message {
	message := Message{ID: wire.ID, FolderID: folder, Snippet: wire.Snippet, Seen: true, From: []Address{}, To: []Address{}}
	for _, header := range wire.Payload.Headers {
		switch strings.ToLower(header.Name) {
		case "subject":
			message.Subject = header.Value
			if decoded, err := new(mime.WordDecoder).DecodeHeader(header.Value); err == nil {
				message.Subject = decoded
			}
		case "date":
			message.Date = header.Value
			if date, err := mail.ParseDate(header.Value); err == nil {
				message.Date = date.Format(time.RFC3339)
			}
		case "from":
			message.From = parseAddresses(header.Value)
		case "to":
			message.To = parseAddresses(header.Value)
		case "cc":
			message.Cc = parseAddresses(header.Value)
		}
	}
	for _, label := range wire.Labels {
		if label == "UNREAD" {
			message.Seen = false
		}
		if label == "STARRED" {
			message.Flagged = true
		}
	}
	return message
}

func parseAddresses(raw string) []Address {
	addresses, _ := mail.ParseAddressList(raw)
	result := make([]Address, 0, len(addresses))
	for _, address := range addresses {
		result = append(result, Address{Name: address.Name, Email: address.Address})
	}
	return result
}

type graphAddress struct {
	Email struct {
		Name  string `json:"name"`
		Email string `json:"address"`
	} `json:"emailAddress"` //nolint:tagliatelle // Graph wire format
}

func (address graphAddress) dto() Address {
	return Address{Name: address.Email.Name, Email: address.Email.Email}
}

type graphMessage struct {
	ID      string         `json:"id"`
	Subject string         `json:"subject"`
	From    graphAddress   `json:"from"`
	To      []graphAddress `json:"toRecipients"`     //nolint:tagliatelle // Graph wire format
	Cc      []graphAddress `json:"ccRecipients"`     //nolint:tagliatelle // Graph wire format
	Date    string         `json:"receivedDateTime"` //nolint:tagliatelle // Graph wire format
	Snippet string         `json:"bodyPreview"`      //nolint:tagliatelle // Graph wire format
	Seen    bool           `json:"isRead"`           //nolint:tagliatelle // Graph wire format
	Flag    struct {
		Status string `json:"flagStatus"` //nolint:tagliatelle // Graph wire format
	} `json:"flag"`
	Body struct {
		Content string `json:"content"`
	} `json:"body"`
}

func (wire graphMessage) message(folder string) Message {
	message := Message{ID: wire.ID, FolderID: folder, Subject: wire.Subject, Date: wire.Date, Snippet: wire.Snippet, Text: wire.Body.Content, Seen: wire.Seen, Flagged: wire.Flag.Status == "flagged", From: []Address{wire.From.dto()}, To: []Address{}}
	for _, address := range wire.To {
		message.To = append(message.To, address.dto())
	}
	for _, address := range wire.Cc {
		message.Cc = append(message.Cc, address.dto())
	}
	return message
}
