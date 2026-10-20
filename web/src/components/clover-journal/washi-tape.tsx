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
import type { CSSProperties } from 'react'

import { cn } from '@/lib/utils'

export type WashiTapeTone = 'green' | 'gold' | 'clay' | 'gingham'

export type WashiTapeProps = {
  tone?: WashiTapeTone
  rotation?: number
  width?: string
  height?: string
  className?: string
  style?: CSSProperties
}

const TONE_CLASS: Record<WashiTapeTone, string> = {
  green: 'journal-tape-green',
  gold: 'journal-tape-gold',
  clay: 'journal-tape-clay',
  gingham: 'journal-tape-gingham',
}

/**
 * A small strip of washi tape. The preset owns its colour and texture;
 * pointer transparency keeps it clear of the surrounding controls.
 */
export function WashiTape(props: WashiTapeProps) {
  const {
    tone = 'green',
    rotation = -2,
    width = '6rem',
    height,
    className,
    style,
  } = props

  return (
    <span
      aria-hidden='true'
      className={cn(
        'journal-tape pointer-events-none absolute block select-none',
        TONE_CLASS[tone],
        'h-4',
        className
      )}
      style={{
        width,
        height,
        transform: `rotate(${rotation}deg)`,
        ...style,
      }}
    />
  )
}
