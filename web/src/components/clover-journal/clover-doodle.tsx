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

type CloverDoodleVariant = 'four-leaf' | 'sprig' | 'leaf' | 'flower'
type CloverDoodleSize = 'sm' | 'md' | 'lg'

type CloverDoodleProps = {
  variant?: CloverDoodleVariant
  size?: CloverDoodleSize
  className?: string
}

const SIZE_CLASS: Record<CloverDoodleSize, string> = {
  sm: 'size-4',
  md: 'size-6',
  lg: 'size-10',
}

/**
 * Hand-drawn botanical doodles used as pure decoration. The SVG is stroked with
 * `currentColor` so it picks up the active theme's ink, and every instance is
 * hidden from assistive technology and pointer events.
 */
export function CloverDoodle(props: CloverDoodleProps) {
  return (
    <svg
      aria-hidden='true'
      focusable='false'
      viewBox='0 0 32 32'
      fill='none'
      stroke='currentColor'
      strokeWidth='1.4'
      strokeLinecap='round'
      strokeLinejoin='round'
      className={cn(
        'journal-doodle pointer-events-none shrink-0 select-none',
        SIZE_CLASS[props.size ?? 'md'],
        props.className
      )}
    >
      {props.variant === 'sprig' && (
        <>
          <path d='M16 28V6' />
          <path d='M16 22c-4.2-.6-6.4-2.8-6.8-6.4 4.2.2 6.3 2.5 6.8 6.4Z' />
          <path d='M16 18c4.2-.6 6.4-2.8 6.8-6.4-4.2.2-6.3 2.5-6.8 6.4Z' />
          <path d='M16 12c-3.4-.4-5.2-2.2-5.6-5.2 3.4.2 5.1 2 5.6 5.2Z' />
          <path d='M16 9c3.4-.4 5.2-2.2 5.6-5.2-3.4.2-5.1 2-5.6 5.2Z' />
        </>
      )}
      {props.variant === 'leaf' && (
        <>
          <path d='M6 26C6 14 14 6 26 6c0 12-8 20-20 20Z' />
          <path d='M8 24C14 18 19 13 24 8' />
        </>
      )}
      {props.variant === 'flower' && (
        <>
          <circle cx='16' cy='9' r='3' />
          <circle cx='22' cy='13' r='3' />
          <circle cx='20' cy='20' r='3' />
          <circle cx='12' cy='20' r='3' />
          <circle cx='10' cy='13' r='3' />
          <circle cx='16' cy='15' r='1.8' />
        </>
      )}
      {(props.variant ?? 'four-leaf') === 'four-leaf' && (
        <>
          <path d='M16 16C16 16 7.5 15 6.2 10.5 5.3 7.2 7.8 4.7 10.9 5.4 14.2 6.2 16 10.7 16 16Z' />
          <path
            d='M16 16C16 16 7.5 15 6.2 10.5 5.3 7.2 7.8 4.7 10.9 5.4 14.2 6.2 16 10.7 16 16Z'
            transform='rotate(90 16 16)'
          />
          <path
            d='M16 16C16 16 7.5 15 6.2 10.5 5.3 7.2 7.8 4.7 10.9 5.4 14.2 6.2 16 10.7 16 16Z'
            transform='rotate(180 16 16)'
          />
          <path
            d='M16 16C16 16 7.5 15 6.2 10.5 5.3 7.2 7.8 4.7 10.9 5.4 14.2 6.2 16 10.7 16 16Z'
            transform='rotate(270 16 16)'
          />
          <path d='M16 16c0 4.2-1.1 6.5-3.2 8.8' />
        </>
      )}
    </svg>
  )
}
