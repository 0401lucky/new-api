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
import { Skeleton } from '@/components/ui/skeleton'

export function StatCard(props: {
  title: string
  icon: ReactNode
  value: ReactNode
  subtitle?: string
}) {
  return (
    <div className='bg-card hover:border-foreground/20 flex flex-col justify-between gap-4 rounded-xl border p-4 transition-colors sm:p-5'>
      <div className='text-muted-foreground flex items-center gap-2'>
        <span aria-hidden='true'>{props.icon}</span>
        <span className='text-[10px] font-semibold tracking-wider uppercase'>
          {props.title}
        </span>
      </div>
      <div>
        <div className='text-foreground font-mono text-2xl leading-none font-semibold tracking-tight sm:text-3xl'>
          {props.value}
        </div>
        {props.subtitle && (
          <div className='text-muted-foreground mt-1.5 text-xs'>
            {props.subtitle}
          </div>
        )}
      </div>
    </div>
  )
}

export function StatCardSkeleton(props: {
  title: string
  icon: ReactNode
  valueWidth?: number
}) {
  return (
    <div className='bg-card flex flex-col justify-between gap-4 rounded-xl border p-4 sm:p-5'>
      <div className='text-muted-foreground flex items-center gap-2'>
        <span aria-hidden='true'>{props.icon}</span>
        <span className='text-[10px] font-semibold tracking-wider uppercase'>
          {props.title}
        </span>
      </div>
      <div>
        <Skeleton
          className='h-6 rounded-md sm:h-[30px]'
          style={{ width: props.valueWidth ?? 96 }}
        />
        <Skeleton className='mt-1.5 h-4 w-24 rounded' />
      </div>
    </div>
  )
}
