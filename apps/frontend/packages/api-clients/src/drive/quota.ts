/* SPDX-License-Identifier: Apache-2.0 */

/** Capacity reported by the connected provider; null means it is unknown. */
export interface ExternalStorageCapacity {
  totalBytes: number | null;
  usedBytes: number | null;
  availableBytes: number | null;
  /** Other connected sources have not reported a complete capacity. */
  partial?: boolean;
}

/** Wire contract shared by Drive's BFF and the direct request from My Account. */
export interface DriveQuotaResponse {
  limit_bytes: number;
  used_bytes: number;
  available_bytes: number;
  /** Original Sytue quota; optional only for compatibility with previous servers. */
  base_limit_bytes?: number;
  base_used_bytes?: number;
  external_storage?: boolean;
  /** Absent when disconnected or on older servers; individual nulls are unknown. */
  external_capacity?: {
    total_bytes: number | null;
    used_bytes: number | null;
    available_bytes: number | null;
    partial?: boolean;
  } | null;
}

export interface DriveQuota {
  /** Server totals include each external source that reports all three values. */
  limitBytes: number;
  usedBytes: number;
  availableBytes: number;
  /** Sytue's quota stays separate from the combined server totals. */
  baseLimitBytes: number;
  baseUsedBytes: number;
  externalStorage: boolean;
  externalCapacity: ExternalStorageCapacity | null;
}

const capacityBytes = (value: number | null | undefined): number | null =>
  typeof value === 'number' && Number.isFinite(value) && value >= 0 ? value : null;

export function hasCompleteExternalCapacity(
  capacity: ExternalStorageCapacity | null | undefined,
): capacity is ExternalStorageCapacity & {
  totalBytes: number;
  usedBytes: number;
  availableBytes: number;
} {
  return (
    capacity != null &&
    capacityBytes(capacity.totalBytes) !== null &&
    capacityBytes(capacity.usedBytes) !== null &&
    capacityBytes(capacity.availableBytes) !== null
  );
}

/** Preserve server totals, including zero, and accept the previous quota response. */
export function driveQuotaFromResponse(quota: DriveQuotaResponse): DriveQuota {
  const capacity = quota.external_capacity;
  return {
    limitBytes: quota.limit_bytes,
    usedBytes: quota.used_bytes,
    availableBytes: quota.available_bytes,
    baseLimitBytes: quota.base_limit_bytes ?? quota.limit_bytes,
    baseUsedBytes: quota.base_used_bytes ?? quota.used_bytes,
    externalStorage: quota.external_storage === true,
    externalCapacity: capacity
      ? {
          totalBytes: capacityBytes(capacity.total_bytes),
          usedBytes: capacityBytes(capacity.used_bytes),
          availableBytes: capacityBytes(capacity.available_bytes),
          ...(capacity.partial === true ? { partial: true } : {}),
        }
      : null,
  };
}
