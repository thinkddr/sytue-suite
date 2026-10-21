/** Skeleton — placeholder de carga (aria-hidden; el contenedor usa aria-busy). */
import type { JSX } from 'preact';
import { cx } from '../lib/cx.js';

export interface SkeletonProps extends JSX.HTMLAttributes<HTMLDivElement> {
  width?: string;
  height?: string;
  circle?: boolean;
}

export function Skeleton({
  width,
  height = '1rem',
  circle = false,
  class: cls,
  style,
  ...rest
}: SkeletonProps): JSX.Element {
  return (
    <div
      aria-hidden="true"
      class={cx('sytue-skeleton', circle ? 'rounded-full' : 'rounded-md', cls)}
      style={{ width: width ?? '100%', height, ...(style as Record<string, unknown>) }}
      {...rest}
    />
  );
}
