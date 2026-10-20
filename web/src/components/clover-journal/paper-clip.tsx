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

export type PaperClipProps = {
  tone?: 'silver' | 'brass'
  size?: 'sm' | 'md' | 'lg'
  rotation?: number
  className?: string
  style?: CSSProperties
}

/**
 * A realistic metallic paper clip with highlight and drop shadow.
 * Clips neatly onto card corners and sticky notes.
 */
export function PaperClip(props: PaperClipProps) {
  const {
    tone = 'silver',
    size = 'md',
    rotation = -6,
    className,
    style,
  } = props

  const sizeClass = {
    sm: 'w-4 h-7',
    md: 'w-5 h-9',
    lg: 'w-6 h-12',
  }[size]

  const isBrass = tone === 'brass'

  return (
    <svg
      aria-hidden='true'
      focusable='false'
      viewBox='0 0 24 44'
      className={cn(
        'journal-paper-clip pointer-events-none shrink-0 select-none drop-shadow-[0_2px_2px_rgba(0,0,0,0.18)]',
        sizeClass,
        className
      )}
      style={{
        transform: rotation ? `rotate(${rotation}deg)` : undefined,
        ...style,
      }}
    >
      <defs>
        <linearGradient id='metal-silver' x1='0%' y1='0%' x2='100%' y2='0%'>
          <stop offset='0%' stopColor='#9ca3af' />
          <stop offset='30%' stopColor='#f3f4f6' />
          <stop offset='55%' stopColor='#d1d5db' />
          <stop offset='85%' stopColor='#e5e7eb' />
          <stop offset='100%' stopColor='#6b7280' />
        </linearGradient>
        <linearGradient id='metal-brass' x1='0%' y1='0%' x2='100%' y2='0%'>
          <stop offset='0%' stopColor='#b48b3e' />
          <stop offset='30%' stopColor='#fde68a' />
          <stop offset='55%' stopColor='#d97706' />
          <stop offset='85%' stopColor='#fef3c7' />
          <stop offset='100%' stopColor='#92400e' />
        </linearGradient>
      </defs>
      {/* Outer loop */}
      <path
        d='M7 11V34C7 37.3 9.7 40 13 40C16.3 40 19 37.3 19 34V8C19 4.7 16.3 2 13 2C9.7 2 7 4.7 7 8V28C7 30.2 8.8 32 11 32C13.2 32 15 30.2 15 28V11'
        fill='none'
        stroke={isBrass ? 'url(#metal-brass)' : 'url(#metal-silver)'}
        strokeWidth='2.2'
        strokeLinecap='round'
        strokeLinejoin='round'
      />
    </svg>
  )
}
