// SPDX-License-Identifier: Apache-2.0
package offline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strings"
)

type stagedReservation struct {
	Version   int        `json:"v"`
	Owner     string     `json:"owner"`
	Namespace string     `json:"namespace"`
	File      StagedFile `json:"file"`
}

func stagedReservationID(file StagedFile) string { return file.ID + "_reservation" }

// StageFileReserved binds the caller's already-durable ID/size/digest BEFORE
// the first source read. Reserved IDs exclude the internal underscore suffixes
// used by block, reservation and application journal rows. It shares the original encrypted block capture and
// cleanup. A completed capture can recover a lost caller ACK without reopening
// the original input. An incomplete capture requires the same original source;
// missing input, changed intent or corrupt completed bytes fail closed.
// The authenticated caller supplies owner/namespace; a descriptor grants no access.
func (s *Store) StageFileReserved(ctx context.Context, owner, namespace string, file StagedFile, input io.Reader) (StagedFile, error) {
	if !s.Encrypted() {
		return StagedFile{}, ErrEncryptedStore
	}
	sum, err := hex.DecodeString(file.SHA256)
	if owner == "" || namespace == "" || !file.valid() || strings.ContainsRune(file.ID, '_') || err != nil || hex.EncodeToString(sum) != file.SHA256 {
		return StagedFile{}, ErrStagedFile
	}
	var result StagedFile
	err = s.Exclusive(ctx, owner, namespace, stagedReservationID(file), func() error {
		var err error
		result, err = s.stageFileReservedLocked(ctx, owner, namespace, file, input)
		return err
	})
	if err != nil {
		return StagedFile{}, err
	}
	return result, nil
}

func (s *Store) stageFileReservedLocked(ctx context.Context, owner, namespace string, file StagedFile, input io.Reader) (StagedFile, error) {
	if err := ctx.Err(); err != nil {
		return StagedFile{}, err
	}
	expected := stagedReservation{1, owner, namespace, file}
	var reservation stagedReservation
	found, err := s.Read(owner, namespace, stagedReservationID(file), &reservation)
	if err != nil {
		return StagedFile{}, err
	}
	if found && reservation != expected {
		return StagedFile{}, ErrStagedFile
	}
	var complete StagedFile
	completed, err := s.Read(owner, namespace, file.ID, &complete)
	if err != nil {
		return StagedFile{}, err
	}
	if !found {
		// An existing unreserved completion cannot be claimed by a new reservation.
		if completed {
			return StagedFile{}, ErrStagedFile
		}
		if err := s.Write(owner, namespace, stagedReservationID(file), expected); err != nil {
			return StagedFile{}, err
		}
	}
	if completed {
		if complete != file {
			return StagedFile{}, ErrStagedFile
		}
		if err := s.verifyReservedStagedFile(ctx, owner, namespace, file); err != nil {
			return StagedFile{}, err
		}
		return file, nil
	}
	if input == nil {
		return StagedFile{}, ErrStagedFile
	}
	if err := s.captureFile(ctx, owner, namespace, file, input, true); err != nil {
		return StagedFile{}, err
	}
	if err := s.verifyReservedStagedFile(ctx, owner, namespace, file); err != nil {
		return StagedFile{}, err
	}
	return file, nil
}

func (s *Store) verifyReservedStagedFile(ctx context.Context, owner, namespace string, file StagedFile) error {
	reader, err := s.OpenStagedFile(ctx, owner, namespace, file)
	if err != nil {
		return err
	}
	defer func() { _ = reader.Close() }()
	digest := sha256.New()
	n, err := io.Copy(digest, io.NewSectionReader(reader, 0, file.SizeBytes))
	if err != nil {
		return err
	}
	if n != file.SizeBytes || hex.EncodeToString(digest.Sum(nil)) != file.SHA256 {
		return ErrStagedFile
	}
	return ctx.Err()
}
