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
import type { ReactNode } from 'react'

import { cn } from '@/lib/utils'

type StickyNoteProps = {
  children?: ReactNode
  rotation?: number
  className?: string
}

/**
 * A small paper note for decorative annotations. It is hidden from assistive
 * technology on purpose: the surrounding UI always carries the same message in
 * accessible text, and repeating it here would be noise.
 */
export function StickyNote(props: StickyNoteProps) {
  return (
    <span
      aria-hidden='true'
      className={cn(
        'journal-sticky-note relative pointer-events-none inline-block max-w-56 px-3 py-2 text-sm leading-snug select-none',
        props.className
      )}
      style={{ transform: `rotate(${props.rotation ?? -2}deg)` }}
    >
      {props.children}
    </span>
  )
}
