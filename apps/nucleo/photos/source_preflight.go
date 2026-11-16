// SPDX-License-Identifier: Apache-2.0
package photos

import (
	"context"
	"encoding/hex"
	"errors"
	"hash"
	"io"

	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type PagedSourcePlanControl interface {
	StartPrivateSourcePreflight(context.Context, storage.Scope, string) (model.SourcePreflightInfo, error)
	PrivateSourcePreflightPage(context.Context, model.SourcePreflightInfo, storage.LegacySourceCursor) (storage.StreamSourcePage, error)
	OpenPrivateSourcePreflightJSON(context.Context, model.SourcePreflightInfo, storage.StreamSourceRow) (io.ReadCloser, error)
}

// PrepareSourcePlanPaged reads at most one complete legacy JSON entity at a
// time. Raw camera strings, vectors and unknown fields do not accumulate in
// the graph used to validate pins. Their exact descriptors remain in the plan
// and ImportLegacySnapshot still imports and verifies the complete original
// bytes. This v1 plan format retains its existing descriptor/pin size guard;
// it no longer applies that guard to aggregate source JSON size.
func PrepareSourcePlanPaged(ctx context.Context, scope storage.Scope, epoch string, control PagedSourcePlanControl, authority OriginalPinAuthority) (SourcePlan, error) {
	if control == nil {
		return SourcePlan{}, ErrSourcePlan
	}
	info, err := control.StartPrivateSourcePreflight(ctx, scope, epoch)
	if err != nil {
		return SourcePlan{}, err
	}
	if err := info.Validate(scope, epoch); err != nil {
		return SourcePlan{}, err
	}
	rows, backend, err := readPreflightValidationGraph(ctx, info, control)
	if err != nil {
		return SourcePlan{}, err
	}
	defer func() {
		for _, row := range rows {
			clear(row.JSON)
		}
	}()
	snapshot, err := model.NewSnapshot(scope, rows)
	if err != nil {
		return SourcePlan{}, err
	}
	return prepareSourcePlanPins(ctx, scope, epoch, info.CAS, snapshot, backend, info.Blobs, authority)
}

type preflightValidationGraph struct {
	sum              hash.Hash
	rows             []model.Row
	backend          []storage.StreamSourceRow
	blobs, originals int64
	budget           int
}

func (g *preflightValidationGraph) clear() {
	for _, row := range g.rows {
		clear(row.JSON)
	}
}

func (g *preflightValidationGraph) append(ctx context.Context, info model.SourcePreflightInfo, control PagedSourcePlanControl, row storage.StreamSourceRow) error {
	g.budget += len(row.ID) + len(row.Kind) + 256
	if g.budget > maxSourcePlanBytes/2 {
		return ErrSourcePlan
	}
	if err := model.AppendSourceInventoryHash(g.sum, row); err != nil {
		return err
	}
	g.backend = append(g.backend, row)
	if row.Kind == "variant" {
		g.blobs++
		return nil
	}
	value, err := readPreflightValidationRow(ctx, info, control, row)
	if err != nil {
		return err
	}
	g.budget += len(value.JSON)
	if g.budget > maxSourcePlanBytes/2 {
		clear(value.JSON)
		return ErrSourcePlan
	}
	g.rows = append(g.rows, value)
	if row.Kind == "asset" {
		g.originals++
	}
	return nil
}

func (g *preflightValidationGraph) validate(info model.SourcePreflightInfo) error {
	if int64(len(g.backend)) != info.Rows || g.blobs != info.Blobs || g.originals != info.Originals || hex.EncodeToString(g.sum.Sum(nil)) != info.SHA256 {
		return ErrSourcePlan
	}
	return nil
}

func readPreflightValidationGraph(ctx context.Context, info model.SourcePreflightInfo, control PagedSourcePlanControl) ([]model.Row, []storage.StreamSourceRow, error) {
	sum, err := model.SourceInventoryHash(info.Scope, info.Epoch)
	if err != nil {
		return nil, nil, err
	}
	graph := preflightValidationGraph{sum: sum}
	keep := false
	defer func() {
		if !keep {
			graph.clear()
		}
	}()
	cursor := storage.LegacySourceCursor{}
	for {
		page, err := control.PrivateSourcePreflightPage(ctx, info, cursor)
		if err != nil {
			return nil, nil, err
		}
		if page.Validate(info.Scope, info.Epoch, cursor) != nil {
			return nil, nil, ErrSourcePlan
		}
		for _, row := range page.Rows {
			if err := graph.append(ctx, info, control, row); err != nil {
				return nil, nil, err
			}
		}
		if page.Next == nil {
			break
		}
		cursor = *page.Next
	}
	if err := graph.validate(info); err != nil {
		return nil, nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	keep = true
	return graph.rows, graph.backend, nil
}

func readPreflightValidationRow(ctx context.Context, info model.SourcePreflightInfo, control PagedSourcePlanControl, row storage.StreamSourceRow) (model.Row, error) {
	if !model.SourceKind(row.Kind) || row.Size > model.MaxRowBytes {
		return model.Row{}, ErrSourcePlan
	}
	reader, err := control.OpenPrivateSourcePreflightJSON(ctx, info, row)
	if err != nil {
		return model.Row{}, err
	}
	if reader == nil {
		return model.Row{}, ErrSourcePlan
	}
	buffer := make([]byte, row.Size+1)
	defer clear(buffer)
	n, readErr := io.ReadFull(reader, buffer)
	closeErr := reader.Close()
	raw := buffer[:n]
	if readErr != nil && !errors.Is(readErr, io.EOF) && !errors.Is(readErr, io.ErrUnexpectedEOF) {
		return model.Row{}, readErr
	}
	if closeErr != nil {
		return model.Row{}, closeErr
	}
	if err := ctx.Err(); err != nil {
		return model.Row{}, err
	}
	if int64(len(raw)) != row.Size || planBytesHash(raw) != row.SHA256 {
		return model.Row{}, ErrSourcePlan
	}
	return model.SourceValidationRow(info.Scope, model.Row{Kind: row.Kind, ID: row.ID, JSON: raw})
}
