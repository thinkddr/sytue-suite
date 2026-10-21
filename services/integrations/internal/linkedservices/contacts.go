// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//nolint:tagliatelle // Google People and Microsoft Graph use these fixed camelCase and OData wire keys.
package linkedservices

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/htmltext"
)

const personFields = "metadata,names,emailAddresses,phoneNumbers,organizations,biographies,addresses"

func plainContent(value, kind string) string {
	if strings.EqualFold(kind, "html") {
		return htmltext.Text(value)
	}
	return value
}

func object(raw json.RawMessage) map[string]any {
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return out
}

func array(record map[string]any, key string) []any { values, _ := record[key].([]any); return values }

func field(record map[string]any, key string) string { value, _ := record[key].(string); return value }

func first(record map[string]any, key string) map[string]any {
	for _, v := range array(record, key) {
		if item, ok := v.(map[string]any); ok {
			return item
		}
	}
	return map[string]any{}
}

func googleContact(raw json.RawMessage) Contact {
	p := object(raw)
	name := first(p, "names")
	org := first(p, "organizations")
	bio := first(p, "biographies")
	contact := Contact{Resource: Resource{ID: field(p, "resourceName"), ETag: field(p, "etag"), Raw: raw}, CollectionID: "contacts", Name: field(name, "displayName"), GivenName: field(name, "givenName"), FamilyName: field(name, "familyName"), Organization: field(org, "name"), Title: field(org, "title"), Note: plainContent(field(bio, "value"), field(bio, "contentType"))}
	for _, v := range array(p, "emailAddresses") {
		if item, ok := v.(map[string]any); ok {
			contact.Emails = append(contact.Emails, field(item, "value"))
		}
	}
	for _, v := range array(p, "phoneNumbers") {
		if item, ok := v.(map[string]any); ok {
			contact.Phones = append(contact.Phones, field(item, "value"))
		}
	}
	for _, v := range array(p, "addresses") {
		if item, ok := v.(map[string]any); ok {
			contact.Addresses = append(contact.Addresses, field(item, "formattedValue"))
		}
	}
	metadata, _ := p["metadata"].(map[string]any)
	contact.ReadOnly = true
	for _, v := range array(metadata, "sources") {
		if item, ok := v.(map[string]any); ok && field(item, "type") == "CONTACT" && field(item, "etag") != "" {
			contact.ReadOnly = false
		}
	}
	return contact
}

func graphContact(raw json.RawMessage, collection string) Contact {
	p := object(raw)
	contact := Contact{Resource: Resource{ID: field(p, "id"), ETag: field(p, "@odata.etag"), Raw: raw}, CollectionID: collection, Name: field(p, "displayName"), GivenName: field(p, "givenName"), FamilyName: field(p, "surname"), Organization: field(p, "companyName"), Title: field(p, "jobTitle"), Note: field(p, "personalNotes")}
	for _, v := range array(p, "emailAddresses") {
		if item, ok := v.(map[string]any); ok {
			contact.Emails = append(contact.Emails, field(item, "address"))
		}
	}
	for _, key := range []string{"businessPhones", "homePhones"} {
		for _, v := range array(p, key) {
			if value, ok := v.(string); ok {
				contact.Phones = append(contact.Phones, value)
			}
		}
	}
	if phone := field(p, "mobilePhone"); phone != "" {
		contact.Phones = append(contact.Phones, phone)
	}
	for _, key := range []string{"businessAddress", "homeAddress", "otherAddress"} {
		if address, ok := p[key].(map[string]any); ok {
			parts := []string{}
			for _, k := range []string{"street", "city", "state", "postalCode", "countryOrRegion"} {
				if value := field(address, k); value != "" {
					parts = append(parts, value)
				}
			}
			if len(parts) > 0 {
				contact.Addresses = append(contact.Addresses, strings.Join(parts, ", "))
			}
		}
	}
	contact.ReadOnly = contact.ETag == ""
	return contact
}

func graphContactsPath(collection string) string {
	if collection == "contacts" {
		return graph + "/contacts"
	}
	return graph + "/contactFolders/" + url.PathEscape(collection) + "/contacts"
}

func (c *Client) Contacts(ctx context.Context, collection, cursor string) (Page[Contact], error) {
	result := Page[Contact]{Items: []Contact{}}
	if !validID(collection) {
		return result, ErrInvalid
	}
	if c.provider == "yahoo" {
		return c.davContacts(ctx, collection, cursor)
	}
	if c.provider == "gmail" {
		if collection != "contacts" || len(cursor) > 4096 {
			return result, ErrInvalid
		}
		var wire struct {
			Items []json.RawMessage `json:"connections"`
			Next  string            `json:"nextPageToken"`
		}
		if err := c.json(ctx, http.MethodGet, query(googlePeople+"/people/me/connections", url.Values{"personFields": {personFields}, "pageSize": {"100"}, "pageToken": {cursor}, "sortOrder": {"LAST_NAME_ASCENDING"}}), nil, &wire, nil); err != nil {
			return result, err
		}
		for _, raw := range wire.Items {
			result.Items = append(result.Items, googleContact(raw))
		}
		result.NextCursor = wire.Next
		return result, nil
	}
	prefix := graphContactsPath(collection)
	target, err := c.nextURL(cursor, query(prefix, url.Values{"$top": {"100"}}), prefix)
	if err != nil {
		return result, err
	}
	var wire struct {
		Items []json.RawMessage `json:"value"`
		Next  string            `json:"@odata.nextLink"`
	}
	if err := c.json(ctx, http.MethodGet, target, nil, &wire, nil); err != nil {
		return result, err
	}
	for _, raw := range wire.Items {
		result.Items = append(result.Items, graphContact(raw, collection))
	}
	result.NextCursor, err = c.cursor(wire.Next, prefix)
	return result, err
}

func (c *Client) Contact(ctx context.Context, collection, id string) (Contact, error) {
	if !validID(collection) || !validID(id) {
		return Contact{}, ErrInvalid
	}
	if c.provider == "yahoo" {
		return c.davContact(ctx, collection, id)
	}
	var raw json.RawMessage
	if c.provider == "gmail" {
		if collection != "contacts" || !strings.HasPrefix(id, "people/") || strings.Contains(strings.TrimPrefix(id, "people/"), "/") {
			return Contact{}, ErrInvalid
		}
		err := c.json(ctx, http.MethodGet, query(googlePeople+"/people/"+url.PathEscape(strings.TrimPrefix(id, "people/")), url.Values{"personFields": {personFields}}), nil, &raw, nil)
		return googleContact(raw), err
	}
	err := c.json(ctx, http.MethodGet, graphContactsPath(collection)+"/"+url.PathEscape(id), nil, &raw, nil)
	return graphContact(raw, collection), err
}

// mergeValues retains provider labels and unknown properties on unchanged
// addresses/numbers, instead of replacing them with untyped strings.
func mergeValues(existing []any, values []string, key string) []any {
	result := make([]any, 0, len(values))
	for _, value := range values {
		var keep map[string]any
		for _, item := range existing {
			if record, ok := item.(map[string]any); ok && field(record, key) == value {
				keep = record
				break
			}
		}
		if keep == nil {
			keep = map[string]any{key: value}
		}
		result = append(result, keep)
	}
	return result
}

func (c *Client) SaveContact(ctx context.Context, collection, id, etag string, patch ContactPatch) (Contact, error) {
	if !validID(collection) || id != "" && !validID(id) || !validContactPatch(patch, id == "") {
		return Contact{}, ErrInvalid
	}
	if c.provider == "yahoo" {
		return c.davSaveContact(ctx, collection, id, etag, patch)
	}
	current := Contact{Resource: Resource{Raw: json.RawMessage(`{}`)}}
	var headers http.Header
	if id != "" {
		var err error
		current, err = c.Contact(ctx, collection, id)
		if err != nil {
			return Contact{}, err
		}
		if current.ReadOnly {
			return Contact{}, ErrReadOnly
		}
		if current.ETag != etag {
			return Contact{}, ErrConflict
		}
		headers, err = conditional(etag)
		if err != nil {
			return Contact{}, err
		}
	}
	if c.provider == "gmail" {
		return c.saveGoogleContact(ctx, collection, id, patch, current, headers)
	}
	// Graph exposes home/business/mobile separately. A flat phone editor cannot
	// faithfully replace those categories, so that field stays view-only.
	if patch.Phones != nil {
		return Contact{}, ErrReadOnly
	}
	body := map[string]any{}
	for key, value := range map[string]*string{"displayName": patch.Name, "givenName": patch.GivenName, "surname": patch.FamilyName, "companyName": patch.Organization, "jobTitle": patch.Title, "personalNotes": patch.Note} {
		if value != nil {
			body[key] = *value
		}
	}
	if patch.Emails != nil {
		body["emailAddresses"] = mergeValues(array(object(current.Raw), "emailAddresses"), *patch.Emails, "address")
	}
	if len(body) == 0 {
		return Contact{}, ErrInvalid
	}
	target := graphContactsPath(collection)
	method := http.MethodPost
	if id != "" {
		target += "/" + url.PathEscape(id)
		method = http.MethodPatch
	}
	var raw json.RawMessage
	err := c.json(ctx, method, target, body, &raw, headers)
	return graphContact(raw, collection), err
}

func (c *Client) saveGoogleContact(ctx context.Context, collection, id string, p ContactPatch, current Contact, headers http.Header) (Contact, error) {
	if collection != "contacts" {
		return Contact{}, ErrInvalid
	}
	existing := object(current.Raw)
	body := map[string]any{}
	masks := []string{}
	if p.Name != nil || p.GivenName != nil || p.FamilyName != nil {
		name := first(existing, "names")
		delete(name, "metadata")
		delete(name, "displayName")
		delete(name, "displayNameLastFirst")
		delete(name, "unstructuredName")
		if p.GivenName != nil {
			name["givenName"] = *p.GivenName
		}
		if p.FamilyName != nil {
			name["familyName"] = *p.FamilyName
		}
		if p.Name != nil && p.GivenName == nil && p.FamilyName == nil {
			// This input replaces the formatted full name. Keeping its previous
			// prefix/middle/suffix would make People generate a different value
			// (for example "Dr Dr Alice Smith Henry"). Structured-name patches
			// above still retain every component they did not explicitly edit.
			delete(name, "honorificPrefix")
			delete(name, "honorificSuffix")
			delete(name, "middleName")
			name["givenName"] = *p.Name
			name["familyName"] = ""
		}
		body["names"] = []any{name}
		masks = append(masks, "names")
	}
	if p.Organization != nil || p.Title != nil {
		orgs := array(existing, "organizations")
		org := first(existing, "organizations")
		delete(org, "metadata")
		if p.Organization != nil {
			org["name"] = *p.Organization
		}
		if p.Title != nil {
			org["title"] = *p.Title
		}
		if len(orgs) == 0 {
			orgs = []any{org}
		} else {
			orgs[0] = org
		}
		body["organizations"] = orgs
		masks = append(masks, "organizations")
	}
	if p.Note != nil {
		body["biographies"] = []any{map[string]string{"value": *p.Note, "contentType": "TEXT_PLAIN"}}
		masks = append(masks, "biographies")
	}
	if p.Emails != nil {
		body["emailAddresses"] = mergeValues(array(existing, "emailAddresses"), *p.Emails, "value")
		masks = append(masks, "emailAddresses")
	}
	if p.Phones != nil {
		body["phoneNumbers"] = mergeValues(array(existing, "phoneNumbers"), *p.Phones, "value")
		masks = append(masks, "phoneNumbers")
	}
	if len(masks) == 0 {
		return Contact{}, ErrInvalid
	}
	target := query(googlePeople+"/people:createContact", url.Values{"personFields": {personFields}})
	method := http.MethodPost
	if id != "" {
		// Google People uses the CONTACT source ETag in the body, in addition
		// to the HTTP precondition. Omitting it can overwrite a concurrent edit.
		body["metadata"] = existing["metadata"]
		body["etag"] = current.ETag
		target = query(googlePeople+"/people/"+url.PathEscape(strings.TrimPrefix(id, "people/"))+":updateContact", url.Values{"updatePersonFields": {strings.Join(masks, ",")}, "personFields": {personFields}})
		method = http.MethodPatch
	}
	var raw json.RawMessage
	err := c.json(ctx, method, target, body, &raw, headers)
	return googleContact(raw), err
}

func validContactPatch(p ContactPatch, create bool) bool {
	if create && p.Name == nil && p.GivenName == nil && p.FamilyName == nil {
		return false
	}
	for _, v := range []*string{p.Name, p.GivenName, p.FamilyName, p.Organization, p.Title, p.Note} {
		if v != nil && (len(*v) > 1<<20 || strings.ContainsRune(*v, 0)) {
			return false
		}
	}
	for _, list := range []*[]string{p.Emails, p.Phones} {
		if list != nil {
			if len(*list) > 100 {
				return false
			}
			for _, v := range *list {
				if len(v) > 4096 || strings.ContainsAny(v, "\r\n\x00") {
					return false
				}
			}
		}
	}
	return true
}
