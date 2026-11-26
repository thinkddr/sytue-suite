// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package android.content;

import android.net.Uri;

/** Un ContentUris de MENTIRA para CarretePrueba: «uri/id» y de vuelta. */
public final class ContentUris {
    private ContentUris() {}

    public static Uri withAppendedId(Uri uri, long id) {
        return Uri.parse(uri.sinConsulta() + "/" + id);
    }

    public static long parseId(Uri uri) {
        return Long.parseLong(uri.getLastPathSegment());
    }
}
