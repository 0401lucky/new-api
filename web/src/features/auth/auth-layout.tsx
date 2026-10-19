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
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import {
  CloverDoodle,
  HandNote,
  PaperClip,
  StickyNote,
  WashiTape,
} from '@/components/clover-journal'
import { Skeleton } from '@/components/ui/skeleton'
import { useSystemConfig } from '@/hooks/use-system-config'

type AuthLayoutProps = {
  children: React.ReactNode
}

export function AuthLayout({ children }: AuthLayoutProps) {
  const { t } = useTranslation()
  const { systemName, logo, loading } = useSystemConfig()

  return (
    <div
      data-slot='auth-layout'
      data-journal-level='immersive'
      className='relative min-h-svh overflow-hidden'
    >
      <Link
        to='/'
        className='absolute top-4 left-4 z-20 flex items-center gap-2 transition-opacity hover:opacity-80 sm:top-8 sm:left-8'
      >
        <div data-slot='system-brand-logo' className='relative h-8 w-8'>
          {loading ? (
            <Skeleton className='absolute inset-0 rounded-full' />
          ) : (
            <img
              src={logo}
              alt={t('Logo')}
              className='h-8 w-8 rounded-full object-cover'
            />
          )}
        </div>
        {loading ? (
          <Skeleton className='h-6 w-24' />
        ) : (
          <h1 className='text-xl font-medium'>{systemName}</h1>
        )}
      </Link>

      {/* Desktop journal cover. Purely decorative, so it is hidden from
       * assistive technology and never intercepts pointer events. */}
      <div
        aria-hidden='true'
        data-slot='auth-cover'
        className='pointer-events-none absolute inset-y-0 left-0 hidden w-[46%] overflow-hidden lg:block'
      >
        <CloverDoodle
          variant='sprig'
          size='lg'
          className='absolute top-[10%] left-[7%] size-24 opacity-60'
        />
        <CloverDoodle
          variant='four-leaf'
          size='lg'
          className='absolute right-[5%] bottom-[12%] size-32 opacity-50'
        />
        <CloverDoodle
          variant='flower'
          className='absolute top-[18%] right-[16%] opacity-60'
        />

        <div className='bg-card absolute top-[25%] left-[9%] w-[64%] -rotate-3 p-2 shadow-[var(--journal-paper-shadow)]'>
          <WashiTape
            className='-top-3 left-8'
            tone='green'
            width='5.5rem'
            rotation={-3}
          />
          <div className='aspect-[4/3] overflow-hidden rounded-[2px]'>
            <svg
              viewBox='0 0 160 120'
              role='presentation'
              className='size-full'
            >
              <defs>
                <linearGradient id='auth-cover-sky' x1='0' y1='0' x2='0' y2='1'>
                  <stop offset='0%' stopColor='#dceee6' />
                  <stop offset='100%' stopColor='#f5edd2' />
                </linearGradient>
                <linearGradient
                  id='auth-cover-field'
                  x1='0'
                  y1='0'
                  x2='0'
                  y2='1'
                >
                  <stop offset='0%' stopColor='#a9cf98' />
                  <stop offset='100%' stopColor='#cfe5b6' />
                </linearGradient>
              </defs>
              <rect width='160' height='120' fill='url(#auth-cover-sky)' />
              <circle cx='128' cy='26' r='9' fill='#f2d58a' />
              <path
                d='M0 78 28 52 48 70 70 44 96 74 118 58 160 82V120H0Z'
                fill='#9fc596'
              />
              <path
                d='M0 91 34 76 60 88 92 70 124 86 160 74V120H0Z'
                fill='url(#auth-cover-field)'
              />
              <path
                d='M18 112c2-8 6-12 12-14M42 116c1-6 4-10 9-12M118 114c2-7 6-11 11-13'
                fill='none'
                stroke='#6f9d63'
                strokeWidth='1.2'
                strokeLinecap='round'
              />
              <path
                d='M24 104c-4-1-6-3-6-6 4 0 6 2 6 6Zm0 0c4-1 6-3 6-6-4 0-6 2-6 6Z'
                fill='#6f9d63'
              />
            </svg>
          </div>
          <p className='journal-hand text-foreground/70 mt-2 text-center text-sm'>
            {t('Good things are always around you')}
          </p>
        </div>

        <StickyNote
          className='absolute top-[57%] right-[8%] w-40'
          rotation={-3}
        >
          {t('Record every valuable call')}
        </StickyNote>
      </div>

      {/* Botanical corners for stacked layouts, where the cover is hidden.
       * Decorative only, so the whole layer stays out of the a11y tree. */}
      <div
        aria-hidden='true'
        data-slot='auth-corner-plants'
        className='pointer-events-none absolute inset-0 lg:hidden'
      >
        <CloverDoodle
          variant='sprig'
          size='md'
          className='absolute top-20 right-3 opacity-50 sm:top-24 sm:right-8'
        />
        <CloverDoodle
          variant='leaf'
          size='sm'
          className='absolute bottom-10 left-3 opacity-40'
        />
      </div>

      <div
        data-slot='auth-form-shell'
        className='relative flex min-h-svh items-center justify-center px-4 pt-20 pb-10'
      >
        <div className='relative w-full max-w-[440px]'>
          <div
            data-slot='auth-form-card'
            className='journal-auth-card relative p-6 sm:p-8'
          >
            <WashiTape
              className='-top-3 left-8'
              tone='gold'
              width='6rem'
              rotation={-2}
            />
            <PaperClip className='absolute -top-2 right-6 rotate-12' />
            <CloverDoodle
              variant='sprig'
              size='sm'
              className='absolute -right-2 -bottom-3 hidden rotate-12 sm:block'
            />
            <div data-slot='auth-welcome-note' className='mb-3 flex justify-end'>
              <HandNote className='text-sm' rotation={-2}>
                {t('A little luck for your journey')}
              </HandNote>
            </div>
            {children}
          </div>
          <p className='journal-hand text-muted-foreground mt-4 text-center text-sm'>
            {t('Good things are always around you')}
          </p>
        </div>
      </div>
    </div>
  )
}
