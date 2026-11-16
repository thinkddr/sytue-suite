// SPDX-License-Identifier: Apache-2.0
package domain

import card "git.sytue.eu/sytue/one/apps/nucleo/contacts/card"

const MaxPhotoBytes = card.MaxPhotoBytes

var ErrInvalidPhoto = card.ErrInvalidPhoto

func ValidatePhotoDataURI(data string) (string, error) { return card.ValidatePhotoDataURI(data) }
func PhotoBytes(data string) ([]byte, string, error)   { return card.PhotoBytes(data) }
func SetCardPhoto(data, photo string) (string, error)  { return card.SetCardPhoto(data, photo) }
func RemoveCardPhoto(data string) (string, error)      { return card.RemoveCardPhoto(data) }
func CardHasEmbeddedPhoto(data string) bool            { return card.HasEmbeddedPhoto(data) }
func CardPhotoDataURI(data string) (string, error)     { return card.PhotoDataURI(data) }
