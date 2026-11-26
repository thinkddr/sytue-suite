// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package android.media;

/** Las constantes de android.media.MediaCodecInfo que usa VideoSistema, con sus valores de verdad. */
public final class MediaCodecInfo {
    private MediaCodecInfo() {}

    public static final class CodecCapabilities {
        private CodecCapabilities() {}

        public static final int COLOR_FormatYUV420Planar = 19;
        public static final int COLOR_FormatYUV420SemiPlanar = 21;
        public static final int COLOR_FormatYUV420Flexible = 0x7F420888;
    }
}
