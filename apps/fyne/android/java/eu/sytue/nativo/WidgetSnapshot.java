// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import java.util.ArrayList;
import java.util.Arrays;
import java.util.Collections;
import java.util.HashSet;
import java.util.List;
import java.util.Set;

/** Contrato de apps/nucleo/widgets, sin dependencias de Android ni acceso a secretos. */
public final class WidgetSnapshot {
    public static final long MAX_AGE = 30 * 60;
    public static final int MAX_BYTES = 16 * 1024;
    private static final Set<String> KINDS = new HashSet<>(Arrays.asList("unread", "files", "events", "documents",
            "contacts", "notes", "rows", "pending", "channels", "photos", "episodes", "apps", "calendars", "address_books"));

    public static final class Counter {
        public final String kind;
        public final long value;

        public Counter(String kind, long value) {
            this.kind = kind;
            this.value = value;
        }
    }

    public final long version;
    public final String product;
    public final String generation;
    public final long updatedAt;
    public final long expiresAt;
    public final boolean active;
    public final List<Counter> counters;

    public WidgetSnapshot(long version, String product, String generation, long updatedAt,
            long expiresAt, boolean active, List<Counter> counters) {
        this.version = version;
        this.product = product;
        this.generation = generation;
        this.updatedAt = updatedAt;
        this.expiresAt = expiresAt;
        this.active = active;
        this.counters = Collections.unmodifiableList(new ArrayList<>(counters));
    }

    public boolean visible(String expectedProduct, String currentGeneration, boolean locked, long now) {
        if (locked || !active || version != 1 || product == null || !product.equals(expectedProduct)
                || generation == null || generation.isEmpty() || generation.length() > 128
                || !generation.equals(currentGeneration) || updatedAt <= 0 || updatedAt > now + 5
                || expiresAt <= now || expiresAt <= updatedAt || expiresAt - updatedAt > MAX_AGE
                || counters.size() > 12) {
            return false;
        }
        Set<String> seen = new HashSet<>();
        for (Counter c : counters) {
            if (c == null || !KINDS.contains(c.kind) || !seen.add(c.kind)
                    || c.value < 0 || c.value > 1_000_000_000_000L) {
                return false;
            }
        }
        return true;
    }
}
