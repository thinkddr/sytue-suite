// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package android.content;

import android.net.Uri;

/** De MENTIRA para ArbolDocumentosPrueba. */
public final class UriPermission {
    private final Uri uri;
    private final int banderas;

    public UriPermission(Uri uri, int banderas) {
        this.uri = uri;
        this.banderas = banderas;
    }

    public Uri getUri() { return uri; }

    public boolean isReadPermission() { return (banderas & Intent.FLAG_GRANT_READ_URI_PERMISSION) != 0; }

    public boolean isWritePermission() { return (banderas & Intent.FLAG_GRANT_WRITE_URI_PERMISSION) != 0; }
}
