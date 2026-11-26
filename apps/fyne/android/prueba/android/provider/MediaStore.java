// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package android.provider;

import android.net.Uri;

/**
 * Un MediaStore de MENTIRA para CarretePrueba: las tablas de fotos y de vídeos con los
 * nombres de columna y las uris de Android.
 */
public final class MediaStore {
    private MediaStore() {}

    public static final String VOLUME_EXTERNAL = "external";
    public static final String VOLUME_EXTERNAL_PRIMARY = "external_primary";

    /** Como en Android: la misma uri con «requireOriginal=1». */
    public static Uri setRequireOriginal(Uri uri) {
        return Uri.parse(uri + "?requireOriginal=1");
    }

    public interface MediaColumns {
        String _ID = "_id";
        String DISPLAY_NAME = "_display_name";
        String MIME_TYPE = "mime_type";
        String DATE_ADDED = "date_added";
        String SIZE = "_size";
        String RELATIVE_PATH = "relative_path";
        String IS_PENDING = "is_pending";
        String WIDTH = "width";
        String HEIGHT = "height";
        String DURATION = "duration";
    }

    public static final class Images {
        private Images() {}

        public static final class Media implements MediaColumns {
            private Media() {}

            public static final String DATE_TAKEN = "datetaken";
            public static final Uri EXTERNAL_CONTENT_URI = getContentUri(VOLUME_EXTERNAL);

            public static Uri getContentUri(String volumen) {
                return Uri.parse("content://media/" + volumen + "/images/media");
            }
        }
    }

    public static final class Video {
        private Video() {}

        public static final class Media implements MediaColumns {
            private Media() {}

            public static final String DATE_TAKEN = "datetaken";
            public static final Uri EXTERNAL_CONTENT_URI = getContentUri(VOLUME_EXTERNAL);

            public static Uri getContentUri(String volumen) {
                return Uri.parse("content://media/" + volumen + "/video/media");
            }
        }
    }
}
