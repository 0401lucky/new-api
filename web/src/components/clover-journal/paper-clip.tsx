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

type PaperClipProps = {
  className?: string
  size?: 'sm' | 'md'
}

/** A decorative paper clip. The caller positions it over a card corner. */
export function PaperClip(props: PaperClipProps) {
  return (
    <svg
      aria-hidden='true'
      focusable='false'
      viewBox='0 0 24 24'
      fill='none'
      stroke='currentColor'
      strokeWidth='1.6'
      strokeLinecap='round'
      strokeLinejoin='round'
      className={cn(
        'journal-paper-clip pointer-events-none shrink-0 select-none',
        props.size === 'sm' ? 'size-5' : 'size-6',
        props.className
      )}
    >
      <path d='M8.5 12.5 14.8 6.2a3.2 3.2 0 0 1 4.5 4.5l-7.6 7.6a5 5 0 0 1-7.1-7.1l7.2-7.2' />
    </svg>
  )
}
