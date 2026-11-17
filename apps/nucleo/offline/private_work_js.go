// SPDX-License-Identifier: Apache-2.0
package offline

import "context"

// Fyne's js target is type checking only. It must not substitute localStorage for
// native encrypted journals, cross-app leases or permission to delete work.
func CountDirectoryOwner(string, string, ...string) (int, error) { return 0, ErrEncryptedStore }

func CountDirectory(string, ...string) (int, error) { return 0, ErrEncryptedStore }

func OpenContext(context.Context, string) (*Store, error) { return nil, ErrEncryptedStore }

func ClearDirectoryKeepingContext(context.Context, string, ...string) error { return ErrEncryptedStore }

func DiscardDirectoryOwnerContext(context.Context, string, string, ...string) error {
	return ErrEncryptedStore
}

func JournalRelocationRetired(context.Context, string, string) (bool, error) {
	return false, ErrEncryptedStore
}

func JournalRelocationVisible(context.Context, string) (bool, error) { return false, ErrEncryptedStore }
