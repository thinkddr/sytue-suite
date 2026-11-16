// SPDX-License-Identifier: Apache-2.0
package webport

import (
	"context"
	"encoding/json"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func hostReplyHandle(method Method) (Method, string, bool) {
	switch method {
	case LeaseAcquire:
		return LeaseRelease, "lease", true
	case JournalLock:
		return JournalUnlock, "lock", true
	case StorageFileDownload:
		return StorageFileClose, "stream", true
	case SourceOpen:
		return SourceClose, "stream", true
	case ServiceRequest:
		return ServiceClose, "stream", true
	default:
		return "", "", false
	}
}

// closeHostReplyHandle disposes only the original scoped capability. It never
// substitutes Current or retries an operation using a replacement owner.
func closeHostReplyHandle(ctx context.Context, port Port, scope storage.Scope, method Method, id string) error {
	closeMethod, field, ok := hostReplyHandle(method)
	if !ok || ctx == nil || port == nil || !scope.Valid() || !ValidID(id) {
		return ErrRejected
	}
	request, err := json.Marshal(map[string]string{field: id})
	if err != nil {
		return err
	}
	defer clear(request)
	response, err := port.Call(context.WithoutCancel(ctx), scope, closeMethod, request)
	clear(response)
	return err
}

// discardHostReply joins disposal of an unadopted host capability before its
// original operation releases its witness. Only fixed protocol reply fields
// select a matching close method; response data cannot choose another route.
func discardHostReply(ctx context.Context, port Port, scope storage.Scope, method Method, raw []byte) error {
	defer clear(raw)
	_, field, ok := hostReplyHandle(method)
	if !ok || len(raw) == 0 {
		return nil
	}
	limit, ok := Limits(method)
	if !ok || len(raw) > limit.Response {
		return ErrRejected
	}
	var value map[string]json.RawMessage
	if json.Unmarshal(raw, &value) != nil {
		return ErrRejected
	}
	var id string
	if json.Unmarshal(value[field], &id) != nil {
		return ErrRejected
	}
	return closeHostReplyHandle(ctx, port, scope, method, id)
}
