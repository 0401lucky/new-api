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

type PaperGrainProps = {
  opacity?: number
  className?: string
}

/** A decorative paper-grain overlay for large surfaces. */
export function PaperGrain(props: PaperGrainProps) {
  return (
    <span
      aria-hidden='true'
      className={cn(
        'journal-grain pointer-events-none absolute inset-0',
        props.className
      )}
      style={{
        backgroundImage: 'var(--journal-grain)',
        backgroundSize: '160px 160px',
        opacity: props.opacity ?? 0.04,
      }}
    />
  )
}
