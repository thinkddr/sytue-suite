// SPDX-License-Identifier: Apache-2.0
package photos

import (
	"context"
	"encoding/json"
	"io"

	"git.sytue.eu/sytue/one/apps/nucleo/drive"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type driveOriginalPins struct {
	repository func(context.Context) (*drive.Repository, error)
}

// NewDriveOriginalPins retains access in the already authorized Drive device.
// Photos never receives its keys, and a legacy HTTP original is never opened.
func NewDriveOriginalPins(repository *drive.Repository) (OriginalPinAuthority, error) {
	if repository == nil {
		return nil, ErrSourcePlan
	}
	return NewDriveOriginalPinsProvider(func(context.Context) (*drive.Repository, error) { return repository, nil })
}

// NewDriveOriginalPinsProvider resolves an already verified repository when an
// original is actually needed. A recovered Photos catalog can therefore open
// without an online Drive device; initial capture still requires complete pins.
func NewDriveOriginalPinsProvider(resolve func(context.Context) (*drive.Repository, error)) (OriginalPinAuthority, error) {
	if resolve == nil {
		return nil, ErrSourcePlan
	}
	return driveOriginalPins{repository: resolve}, nil
}

func (a driveOriginalPins) Capture(ctx context.Context, scope storage.Scope, original model.Original) (PlannedOriginal, error) {
	repository, err := a.repository(ctx)
	if err != nil {
		return PlannedOriginal{}, err
	}
	if repository == nil {
		return PlannedOriginal{}, ErrNotReady
	}
	pin, err := repository.PinRetainedOriginal(ctx, "file", original.DriveID)
	if err != nil {
		return PlannedOriginal{}, err
	}
	raw, err := json.Marshal(pin)
	if err != nil {
		return PlannedOriginal{}, err
	}
	planned := PlannedOriginal{Original: original, Descriptor: storage.StreamSourceRow{Kind: "original", ID: original.AssetID, Size: pin.SizeBytes, SHA256: pin.SHA256}, Pin: raw}
	if _, err := originalDrivePin(scope, planned); err != nil {
		return PlannedOriginal{}, err
	}
	return planned, nil
}

func originalDrivePin(scope storage.Scope, planned PlannedOriginal) (drive.FilePin, error) {
	if !scope.Valid() || scope.Product != "photos" || validatePlannedOriginal(planned) != nil {
		return drive.FilePin{}, ErrSourcePlan
	}
	var pin drive.FilePin
	if json.Unmarshal(planned.Pin, &pin) != nil || !pin.Valid() || pin.Scope.Tenant != scope.Tenant || pin.Scope.Subject != scope.Subject || pin.SourceKind != "file" || pin.SourceID != planned.Original.DriveID || pin.SizeBytes != planned.Descriptor.Size || pin.SHA256 != planned.Descriptor.SHA256 {
		return drive.FilePin{}, ErrSourcePlan
	}
	return pin, nil
}

func (a driveOriginalPins) Validate(ctx context.Context, scope storage.Scope, planned PlannedOriginal) error {
	pin, err := originalDrivePin(scope, planned)
	if err != nil {
		return err
	}
	repository, err := a.repository(ctx)
	if err != nil {
		return err
	}
	if repository == nil {
		return ErrNotReady
	}
	return repository.ValidatePinnedOriginal(ctx, pin)
}

func (a driveOriginalPins) Write(ctx context.Context, scope storage.Scope, planned PlannedOriginal, out io.Writer) error {
	pin, err := originalDrivePin(scope, planned)
	if err != nil {
		return err
	}
	repository, err := a.repository(ctx)
	if err != nil {
		return err
	}
	if repository == nil {
		return ErrNotReady
	}
	return repository.WritePinnedOriginal(ctx, pin, out)
}
