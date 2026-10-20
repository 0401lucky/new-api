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
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

import { CloverDoodle } from './clover-doodle'
import { HandNote } from './hand-note'

export type CloverBotanicalCanvasProps = {
  className?: string
  showTopLeft?: boolean
  showTopRight?: boolean
  showBottomRight?: boolean
  showNotes?: boolean
}

/** Small botanical margin sketches, bounded by their page rather than the viewport. */
export function CloverBotanicalCanvas(props: CloverBotanicalCanvasProps) {
  const { t } = useTranslation()

  return (
    <div
      aria-hidden='true'
      className={cn(
        'journal-botanical-canvas pointer-events-none absolute inset-0 overflow-hidden select-none',
        props.className
      )}
    >
      {props.showTopLeft !== false && (
        <CloverDoodle
          variant='sprig'
          size='lg'
          className='absolute top-28 -left-6 hidden size-24 -rotate-12 xl:block'
        />
      )}
      {props.showTopRight !== false && (
        <CloverDoodle
          variant='leaf'
          size='lg'
          className='absolute top-24 right-10 hidden size-16 rotate-12 2xl:block'
        />
      )}
      {props.showBottomRight !== false && (
        <CloverDoodle
          variant='four-leaf'
          size='lg'
          className='absolute right-8 bottom-12 hidden size-20 -rotate-12 2xl:block'
        />
      )}
      {props.showNotes !== false && (
        <HandNote className='absolute top-44 right-10 hidden text-sm 2xl:block'>
          {t('Good things are always around you')}
        </HandNote>
      )}
    </div>
  )
}
