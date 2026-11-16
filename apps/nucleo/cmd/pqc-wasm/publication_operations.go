//go:build js && wasm

// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/json"
	"strconv"
	"syscall/js"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/publication"
)

func init() {
	domainOperations["publication.wire.digest"] = publicationWireDigest
	domainOperations["publication.wire.attach"] = publicationWireAttach
	domainOperations["publication.request.build"] = publicationRequestBuild
	domainOperations["publication.request.encode"] = publicationRequestEncode
	domainOperations["publication.authority.decode"] = publicationAuthorityDecode
	domainOperations["publication.receipt.decode"] = publicationReceiptDecode
	domainOperations["publication.recovery.decode"] = publicationRecoveryDecode
}

func publicationWireDigest(req js.Value) (any, error) {
	raw, err := bytesArg(req.Get("wire"), 65536)
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	required := req.Get("require_marker")
	if required.Type() != js.TypeBoolean {
		return nil, publication.ErrInvalid
	}
	digest, marker, err := publication.WireDigest(raw, required.Bool())
	if err != nil {
		return nil, err
	}
	return map[string]any{"sha256": digest, "marker": map[string]any{"v": marker.Version, "id": marker.ID}}, nil
}

func publicationRequestEncode(req js.Value) (any, error) {
	raw, err := bytesArg(req.Get("request"), 16384)
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	var value publication.Request
	if json.Unmarshal(raw, &value) != nil || !value.Valid() || value.Scope != scopeArg(req) {
		return nil, publication.ErrInvalid
	}
	payload, err := matrixproof.CanonicalBytes(value)
	if err != nil {
		return nil, err
	}
	defer clear(payload)
	digest, err := matrixproof.RequestDigest(value)
	if err != nil {
		return nil, err
	}
	return map[string]any{"sha256": digest, "payload": bytesValue(payload)}, nil
}

func publicationAuthorityDecode(req js.Value) (any, error) {
	raw, err := bytesArg(req.Get("authority"), 16384)
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	authority, err := publication.DecodeActiveAuthority(raw, scopeArg(req))
	if err != nil {
		return nil, err
	}
	return map[string]any{"v": authority.Version, "scope": map[string]any{"tenant": authority.Scope.Tenant, "subject": authority.Scope.Subject, "product": authority.Scope.Product}, "user_id": authority.UserID, "root_key": authority.RootKey, "generation": strconv.FormatInt(authority.Generation, 10), "status": authority.Status}, nil
}

func publicationReceiptDecode(req js.Value) (any, error) {
	requestRaw, err := bytesArg(req.Get("request"), 16384)
	if err != nil {
		return nil, err
	}
	defer clear(requestRaw)
	raw, err := bytesArg(req.Get("receipt"), 16384)
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	var request publication.Request
	if json.Unmarshal(requestRaw, &request) != nil || request.Scope != scopeArg(req) {
		return nil, publication.ErrInvalid
	}
	receipt, err := publication.DecodeReceipt(raw, request)
	if err != nil {
		return nil, err
	}
	marker := map[string]any{"v": receipt.Marker.Version, "id": receipt.Marker.ID}
	value := map[string]any{"v": receipt.Version, "marker": marker, "status": receipt.Status, "request_sha256": receipt.RequestSHA256}
	if receipt.Reference != nil {
		value["reference"] = map[string]any{"room_id": receipt.Reference.RoomID, "event_id": receipt.Reference.EventID}
	}
	if reserved, err := receipt.ReservedFor(request); err == nil {
		value["reserved_marker"] = map[string]any{"v": reserved.Version, "id": reserved.ID}
	}
	return value, nil
}

func publicationRequestBuild(req js.Value) (any, error) {
	wire, err := bytesArg(req.Get("wire"), 65536)
	if err != nil {
		return nil, err
	}
	defer clear(wire)
	raw, err := bytesArg(req.Get("authority"), 16384)
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	scope := scopeArg(req)
	authority, err := publication.DecodeActiveAuthority(raw, scope)
	if err != nil {
		return nil, err
	}
	targetValue := req.Get("target")
	if targetValue.Type() != js.TypeObject || targetValue.IsNull() || targetValue.Get("room_id").Type() != js.TypeString || targetValue.Get("device_id").Type() != js.TypeString {
		return nil, publication.ErrInvalid
	}
	target := storage.PublicationTarget{RoomID: targetValue.Get("room_id").String(), DeviceID: targetValue.Get("device_id").String()}
	transaction := req.Get("transaction")
	if transaction.Type() != js.TypeString {
		return nil, publication.ErrInvalid
	}
	prepared, err := publication.BuildStorageRequest(scope, target, transaction.String(), wire, authority)
	if err != nil {
		return nil, err
	}
	defer clear(prepared.Payload)
	return map[string]any{"payload": bytesValue(prepared.Payload), "request_sha256": prepared.SHA256, "root_key": authority.RootKey, "user_id": authority.UserID, "generation": strconv.FormatInt(authority.Generation, 10)}, nil
}

func publicationWireAttach(req js.Value) (any, error) {
	wire, err := bytesArg(req.Get("wire"), 65536)
	if err != nil {
		return nil, err
	}
	defer clear(wire)
	value := req.Get("marker")
	if value.Type() != js.TypeObject || value.IsNull() || value.Get("id").Type() != js.TypeString {
		return nil, publication.ErrInvalid
	}
	version, err := integerArg(value.Get("v"), 1)
	if err != nil || version != 1 {
		return nil, publication.ErrInvalid
	}
	marker := publication.Marker{Version: version, ID: value.Get("id").String()}
	marked, err := publication.AttachMarker(wire, marker)
	if err != nil {
		return nil, err
	}
	defer clear(marked)
	return map[string]any{"wire": bytesValue(marked)}, nil
}

func publicationRecoveryDecode(req js.Value) (any, error) {
	raw, err := bytesArg(req.Get("recovery"), 32768)
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	room, txn := req.Get("room_id"), req.Get("transaction_id")
	if room.Type() != js.TypeString || txn.Type() != js.TypeString {
		return nil, publication.ErrInvalid
	}
	selection := publication.RecoverySelection{Scope: scopeArg(req), RoomID: room.String(), TransactionID: txn.String()}
	hint, err := publication.DecodeRecoveryHint(raw, selection)
	if err != nil {
		return nil, err
	}
	original := hint.Original()
	value := map[string]any{"status": hint.Status(), "request_sha256": hint.RequestSHA256(), "cipher_sha256": original.CipherSHA256, "device_id": original.Target.DeviceID, "generation": strconv.FormatInt(original.AuthorityGeneration, 10)}
	if ref, ok := hint.AcceptedReference(); ok {
		value["reference"] = map[string]any{"room_id": ref.RoomID, "event_id": ref.EventID}
	}
	return value, nil
}
