//go:build js && wasm

// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/json"
	"syscall/js"

	roomsearch "git.sytue.eu/sytue/one/apps/nucleo/rooms/search"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

func init() {
	domainOperations["rooms.search.parse"] = roomsSearchParse
	domainOperations["rooms.search.metadata"] = roomsSearchMetadata
	domainOperations["rooms.search.match"] = roomsSearchMatch
}

func roomsSearchParse(req js.Value) (any, error) {
	raw, err := bytesArg(req.Get("query"), 64<<10)
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	query := roomsearch.Parse(string(raw))
	encoded, err := json.Marshal(query)
	if err != nil {
		return nil, err
	}
	defer clear(encoded)
	return bytesValue(encoded), nil
}

func roomsSearchDocuments(req js.Value) (roomsearch.Query, []roomsearch.Document, error) {
	var query roomsearch.Query
	var documents []roomsearch.Document
	ast, err := bytesArg(req.Get("query"), 128<<10)
	if err != nil {
		return query, nil, err
	}
	defer clear(ast)
	data, err := bytesArg(req.Get("documents"), 16<<20)
	if err != nil {
		return query, nil, err
	}
	defer clear(data)
	if json.Unmarshal(ast, &query) != nil || json.Unmarshal(data, &documents) != nil || len(documents) > 200 {
		return query, nil, pqc.ErrEnvelope
	}
	return query, documents, nil
}
func roomsSearchMetadata(req js.Value) (any, error) { return roomsSearchEvaluate(req, true) }
func roomsSearchMatch(req js.Value) (any, error)    { return roomsSearchEvaluate(req, false) }
func roomsSearchEvaluate(req js.Value, metadata bool) (any, error) {
	query, documents, err := roomsSearchDocuments(req)
	if err != nil {
		return nil, err
	}
	matches := make([]bool, len(documents))
	for i, document := range documents {
		if metadata {
			matches[i] = query.MetadataMatches(document)
		} else {
			matches[i] = query.Matches(document)
		}
	}
	encoded, err := json.Marshal(matches)
	if err != nil {
		return nil, err
	}
	defer clear(encoded)
	return bytesValue(encoded), nil
}
