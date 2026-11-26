// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package android.media;

import java.util.HashSet;
import java.util.Set;

/**
 * Un MediaCodecList de MENTIRA para VideoSistemaPrueba: el teléfono «sabe» descodificar los
 * tipos MIME de {@link #SABE}.
 */
public final class MediaCodecList {
    public static final int REGULAR_CODECS = 0;
    public static final int ALL_CODECS = 1;
    public static final Set<String> SABE = new HashSet<>();

    public MediaCodecList(int clase) {}

    public String findDecoderForFormat(MediaFormat f) {
        String mime = f.getString(MediaFormat.KEY_MIME);
        return SABE.contains(mime) ? "falso." + mime : null;
    }
}
