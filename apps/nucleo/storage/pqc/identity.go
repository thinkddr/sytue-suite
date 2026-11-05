// SPDX-License-Identifier: Apache-2.0
// Package pqc defines the shared client encryption identities used by native
// applications and the browser's Go/Wasm core. The maintained age library owns
// the ML-KEM-768 + X25519 combiner, file format, randomness and authenticated
// streaming encryption; this package implements no cryptographic primitive.
package pqc

import (
	"errors"

	"filippo.io/age"
)

const (
	LegacyProfile     = "age-X25519-v1"
	HybridProfile     = "age-MLKEM768-X25519-v1"
	MaxRecipientBytes = 4096
)

var ErrIdentity = errors.New("pqc: invalid encryption identity or profile")

// Identity keeps one canonical algorithm and recipient bound to the private
// key. String is private recovery material and belongs only in sealed storage.
type Recipient interface {
	age.Recipient
	String() string
}

type Identity struct {
	key              age.Identity
	private, profile string
	recipient        Recipient
}

// GenerateIdentity always selects the maintained post-quantum hybrid profile.
func GenerateIdentity() (*Identity, error) {
	key, err := age.GenerateHybridIdentity()
	if err != nil {
		return nil, err
	}
	return &Identity{key: key, private: key.String(), profile: HybridProfile, recipient: key.Recipient()}, nil
}

// ParseIdentity retains historical classical keys for decryption and explicit
// existing enrollments. It never replaces an identity or downgrades a profile.
func ParseIdentity(raw string) (*Identity, error) {
	if key, err := age.ParseHybridIdentity(raw); err == nil && key.String() == raw {
		return &Identity{key: key, private: raw, profile: HybridProfile, recipient: key.Recipient()}, nil
	}
	if key, err := age.ParseX25519Identity(raw); err == nil && key.String() == raw {
		return &Identity{key: key, private: raw, profile: LegacyProfile, recipient: key.Recipient()}, nil
	}
	return nil, ErrIdentity
}

func (i *Identity) Unwrap(stanzas []*age.Stanza) ([]byte, error) {
	if i == nil || i.key == nil {
		return nil, ErrIdentity
	}
	return i.key.Unwrap(stanzas)
}

func (i *Identity) String() string {
	if i == nil {
		return ""
	}
	return i.private
}

func (i *Identity) Profile() string {
	if i == nil {
		return ""
	}
	return i.profile
}

func (i *Identity) Recipient() Recipient {
	if i == nil {
		return nil
	}
	return i.recipient
}

// PublicIdentity supports maintained legacy readers as well as the single new
// identity type. It returns public information only, never private key bytes.
func PublicIdentity(identity age.Identity) (profile, recipient string, err error) {
	switch key := identity.(type) {
	case *Identity:
		if key != nil && key.recipient != nil {
			return key.profile, key.recipient.String(), nil
		}
	case *age.HybridIdentity:
		if key != nil {
			return HybridProfile, key.Recipient().String(), nil
		}
	case *age.X25519Identity:
		if key != nil {
			return LegacyProfile, key.Recipient().String(), nil
		}
	}
	return "", "", ErrIdentity
}

func ValidProfile(profile string) bool { return profile == HybridProfile || profile == LegacyProfile }

// ParseRecipient binds the declared profile to its canonical maintained codec.
// A hybrid enrollment can never be silently accepted as a classical recipient.
func ParseRecipient(profile, raw string) (Recipient, error) {
	if raw == "" || len(raw) > MaxRecipientBytes {
		return nil, ErrIdentity
	}
	switch profile {
	case HybridProfile:
		key, err := age.ParseHybridRecipient(raw)
		if err == nil && key.String() == raw {
			return key, nil
		}
	case LegacyProfile:
		key, err := age.ParseX25519Recipient(raw)
		if err == nil && key.String() == raw {
			return key, nil
		}
	}
	return nil, ErrIdentity
}
