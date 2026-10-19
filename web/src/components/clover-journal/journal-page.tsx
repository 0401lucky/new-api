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
import type { ComponentProps } from 'react'

import { cn } from '@/lib/utils'

export type JournalLevel = 'immersive' | 'medium' | 'calm'

type JournalPageProps = ComponentProps<'div'> & {
  level?: JournalLevel
}

/**
 * Marks a page subtree with a decoration level. The theme CSS uses the
 * attribute to suppress tape/notes on dense admin screens; unmarked pages
 * behave as `calm`.
 */
export function JournalPage(props: JournalPageProps) {
  const { level = 'calm', className, ...rest } = props
  return (
    <div
      data-journal-level={level}
      className={cn('min-h-0', className)}
      {...rest}
    />
  )
}

export function JournalSection(
  props: ComponentProps<'section'> & { level?: JournalLevel }
) {
  const { level = 'calm', className, ...rest } = props
  return (
    <section
      data-journal-level={level}
      className={cn('min-h-0', className)}
      {...rest}
    />
  )
}
