// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package android;

/** Un android.Manifest de MENTIRA para CarretePrueba: los permisos que pide el carrete. */
public final class Manifest {
    private Manifest() {}

    public static final class permission {
        private permission() {}

        public static final String READ_EXTERNAL_STORAGE = "android.permission.READ_EXTERNAL_STORAGE";
        public static final String READ_MEDIA_IMAGES = "android.permission.READ_MEDIA_IMAGES";
        public static final String READ_MEDIA_VIDEO = "android.permission.READ_MEDIA_VIDEO";
        public static final String READ_MEDIA_VISUAL_USER_SELECTED = "android.permission.READ_MEDIA_VISUAL_USER_SELECTED";
        public static final String ACCESS_MEDIA_LOCATION = "android.permission.ACCESS_MEDIA_LOCATION";
    }
}
