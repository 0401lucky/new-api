/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { cn } from '@/lib/utils'

type WashiTapeTone = 'green' | 'gold' | 'clay'

type WashiTapeProps = {
  tone?: WashiTapeTone
  rotation?: number
  width?: string
  className?: string
}

const TONE_CLASS: Record<WashiTapeTone, string> = {
  green: 'bg-[var(--journal-tape-green)]',
  gold: 'bg-[var(--journal-tape-gold)]',
  clay: 'bg-[var(--journal-tape-clay)]',
}

/**
 * A strip of washi tape. Position it inside a `relative` parent; it is
 * pointer-transparent so it never blocks the control underneath.
 */
export function WashiTape(props: WashiTapeProps) {
  return (
    <span
      aria-hidden='true'
      className={cn(
        'journal-tape pointer-events-none absolute block h-5',
        TONE_CLASS[props.tone ?? 'green'],
        props.className
      )}
      style={{
        width: props.width ?? '6rem',
        transform: `rotate(${props.rotation ?? -2}deg)`,
      }}
    />
  )
}
