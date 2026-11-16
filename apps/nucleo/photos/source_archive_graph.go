// SPDX-License-Identifier: Apache-2.0
package photos

import (
	"bytes"
	"context"
	"encoding/json"

	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type (
	sourceGraphLookup func(context.Context, string, string) (storage.StreamSourceRow, *model.Row, error)
	sourceGraphSeen   func(string, string, string) error
)

func sourceGraphRow(ctx context.Context, scope storage.Scope, kind, id string, lookup sourceGraphLookup) (storage.StreamSourceRow, *model.Row, error) {
	descriptor, row, err := lookup(ctx, kind, id)
	if err != nil {
		return descriptor, nil, err
	}
	if descriptor.Kind != kind || descriptor.ID != id || descriptor.Validate() != nil {
		return descriptor, nil, ErrSourcePlan
	}
	if kind == "variant" {
		if row != nil {
			return descriptor, nil, ErrSourcePlan
		}
		return descriptor, nil, nil
	}
	if row == nil || row.Kind != kind || row.ID != id || model.ValidateRow(scope, *row) != nil {
		return descriptor, nil, ErrSourcePlan
	}
	cloned := *row
	cloned.JSON = bytes.Clone(row.JSON)
	return descriptor, &cloned, nil
}

func sourceGraphOwned(ctx context.Context, scope storage.Scope, kind, id string, lookup sourceGraphLookup) error {
	_, row, err := sourceGraphRow(ctx, scope, kind, id, lookup)
	if err != nil {
		return err
	}
	defer clear(row.JSON)
	var header struct {
		Owner string `json:"owner_sub"`
	}
	if json.Unmarshal(row.JSON, &header) != nil || header.Owner != scope.Subject {
		return ErrSourcePlan
	}
	return nil
}

// The original bytes remain identified by descriptor, not by graph.JSON.
// This validates only relationships in a bounded encrypted staging lookup; it
// does not replace retained rows or prove availability of any original/variant.
func validateSourceGraphEntry(ctx context.Context, scope storage.Scope, descriptor storage.StreamSourceRow, graph *model.Row, lookup sourceGraphLookup, seen sourceGraphSeen) error {
	if lookup == nil || seen == nil || !scope.Valid() || scope.Product != "photos" || !backendPlanKind(descriptor.Kind) || descriptor.Validate() != nil {
		return ErrSourcePlan
	}
	if descriptor.Kind == "variant" {
		return validateGraphVariant(ctx, scope, descriptor, graph, lookup)
	}
	if graph == nil || graph.Kind != descriptor.Kind || graph.ID != descriptor.ID || model.ValidateRow(scope, *graph) != nil {
		return ErrSourcePlan
	}
	return validateGraphRelationships(ctx, scope, descriptor.Kind, graph, lookup, seen)
}

func validateGraphVariant(ctx context.Context, scope storage.Scope, descriptor storage.StreamSourceRow, graph *model.Row, lookup sourceGraphLookup) error {
	if graph != nil {
		return ErrSourcePlan
	}
	_, row, err := sourceGraphRow(ctx, scope, "media", descriptor.ID, lookup)
	if err != nil {
		return err
	}
	defer clear(row.JSON)
	var media model.MediaRow
	if json.Unmarshal(row.JSON, &media) != nil || media.Owner != scope.Subject || media.Role != "variant" || media.Descriptor() != descriptor {
		return ErrSourcePlan
	}
	return sourceGraphOwned(ctx, scope, "asset", media.AssetID, lookup)
}

func validateGraphRelationships(ctx context.Context, scope storage.Scope, kind string, graph *model.Row, lookup sourceGraphLookup, seen sourceGraphSeen) error {
	switch kind {
	case "asset":
		var asset model.AssetRow
		if json.Unmarshal(graph.JSON, &asset) != nil || asset.Owner != scope.Subject || len(asset.DriveID) > 1024 {
			return ErrSourcePlan
		}
		return seen("drive", asset.DriveID, asset.ID)
	case "album":
		var album model.AlbumRow
		if json.Unmarshal(graph.JSON, &album) != nil || album.Owner != scope.Subject {
			return ErrSourcePlan
		}
		if album.PublicToken != "" || album.PublicTokenHash != "" {
			return model.ErrShared
		}
		if album.CoverAssetID != nil {
			return sourceGraphOwned(ctx, scope, "asset", *album.CoverAssetID, lookup)
		}
		return nil
	case "edit":
		var edit model.EditRow
		if json.Unmarshal(graph.JSON, &edit) != nil || edit.Owner != scope.Subject {
			return ErrSourcePlan
		}
		if err := sourceGraphOwned(ctx, scope, "asset", edit.AssetID, lookup); err != nil {
			return err
		}
		return seen("edit", edit.AssetID+"\x00"+edit.Owner, edit.ID)
	case "album_asset":
		var member model.MembershipRow
		if json.Unmarshal(graph.JSON, &member) != nil {
			return ErrSourcePlan
		}
		if err := sourceGraphOwned(ctx, scope, "album", member.AlbumID, lookup); err != nil {
			return err
		}
		return sourceGraphOwned(ctx, scope, "asset", member.AssetID, lookup)
	case "album_person":
		return model.ErrShared
	case "media":
		return validateGraphMedia(ctx, scope, graph, lookup)
	default:
		return ErrSourcePlan
	}
}

func validateGraphMedia(ctx context.Context, scope storage.Scope, graph *model.Row, lookup sourceGraphLookup) error {
	var media model.MediaRow
	if json.Unmarshal(graph.JSON, &media) != nil || media.Owner != scope.Subject || media.Role != "variant" {
		return ErrSourcePlan
	}
	if err := sourceGraphOwned(ctx, scope, "asset", media.AssetID, lookup); err != nil {
		return err
	}
	body, _, err := sourceGraphRow(ctx, scope, "variant", media.ID, lookup)
	if err != nil {
		return err
	}
	if body != media.Descriptor() {
		return ErrSourcePlan
	}
	return nil
}
