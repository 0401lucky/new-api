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

import { CloverDoodle } from './clover-doodle'
import { HandNote } from './hand-note'
import { WashiTape, type WashiTapeTone } from './washi-tape'

export type PolaroidProps = {
  rotation?: number
  tapeTone?: WashiTapeTone
  caption?: ReactNode
  className?: string
}

/** A paper botanical specimen, composed from the journal's existing vector artwork. */
export function Polaroid(props: PolaroidProps) {
  return (
    <div
      aria-hidden='true'
      className={cn(
        'journal-polaroid pointer-events-none relative inline-block max-w-full select-none',
        props.className
      )}
      style={{ transform: `rotate(${props.rotation ?? -3}deg)` }}
    >
      <WashiTape
        tone={props.tapeTone ?? 'gingham'}
        width='4.5rem'
        rotation={-5}
        className='-top-2 left-8'
      />
      <div className='journal-polaroid-frame'>
        <div className='journal-polaroid-photo relative flex aspect-[4/5] items-center justify-center overflow-hidden rounded-xs'>
          <CloverDoodle
            variant='four-leaf'
            size='lg'
            className='size-40 -rotate-15'
          />
          <CloverDoodle
            variant='sprig'
            size='lg'
            className='absolute right-6 bottom-6 size-16 rotate-12'
          />
        </div>
        {props.caption && (
          <HandNote className='mt-4 block text-center text-sm leading-relaxed'>
            {props.caption}
          </HandNote>
        )}
      </div>
    </div>
  )
}
