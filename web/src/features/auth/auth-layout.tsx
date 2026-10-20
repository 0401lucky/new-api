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
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import {
  CloverBotanicalCanvas,
  CloverDoodle,
  HandNote,
  PaperClip,
  StickyNote,
  WashiTape,
} from '@/components/clover-journal'
import { Skeleton } from '@/components/ui/skeleton'
import { useSystemConfig } from '@/hooks/use-system-config'

type AuthLayoutProps = {
  children: ReactNode
}

export function AuthLayout(props: AuthLayoutProps) {
  const { t } = useTranslation()
  const { systemName, logo, loading } = useSystemConfig()

  return (
    <div
      data-slot='auth-layout'
      data-journal-level='immersive'
      className='bg-background relative min-h-svh overflow-hidden'
    >
      <CloverBotanicalCanvas showNotes={false} showTopLeft={false} />
      <Link
        to='/'
        className='absolute top-5 left-5 z-20 flex items-center gap-2.5 transition-opacity hover:opacity-80 sm:top-8 sm:left-8'
      >
        <div data-slot='system-brand-logo' className='relative size-8'>
          {loading ? (
            <Skeleton className='absolute inset-0 rounded-full' />
          ) : (
            <img
              src={logo}
              alt={t('Logo')}
              className='size-8 rounded-full object-cover'
            />
          )}
        </div>
        {loading ? (
          <Skeleton className='h-6 w-24' />
        ) : (
          <span
            data-slot='system-brand'
            className='text-base font-semibold tracking-tight'
          >
            {systemName}
          </span>
        )}
      </Link>

      <div
        aria-hidden='true'
        data-slot='auth-corner-plants'
        className='pointer-events-none absolute inset-0 lg:hidden'
      >
        <CloverDoodle
          variant='sprig'
          size='md'
          className='absolute top-20 right-5 opacity-30'
        />
        <CloverDoodle
          variant='leaf'
          size='sm'
          className='absolute bottom-8 left-5 opacity-30'
        />
      </div>

      <div data-slot='auth-content' className='relative'>
        <div
          aria-hidden='true'
          data-slot='auth-cover'
          className='pointer-events-none relative hidden min-w-0 flex-col items-center justify-center'
        >
          <div data-slot='auth-illustration' className='w-full' />
          <div
            data-slot='auth-cover-caption'
            className='relative flex items-center justify-center gap-3 px-5'
          >
            <CloverDoodle variant='four-leaf' size='md' />
            <HandNote className='text-center text-xl' rotation={-2}>
              {t('Record every valuable call')}
            </HandNote>
          </div>
        </div>

        <div
          data-slot='auth-form-shell'
          className='relative flex min-h-svh items-center justify-center px-4 pt-24 pb-10'
        >
          <div
            data-slot='auth-form-column'
            className='relative w-full max-w-[420px]'
          >
            <div data-slot='auth-form-card' className='relative w-full'>
              <WashiTape
                className='-top-2 left-7'
                tone='gingham'
                width='6rem'
                height='1.25rem'
                rotation={-12}
              />
              <div
                data-slot='auth-welcome-note'
                className='pointer-events-none absolute -top-5 right-5 max-w-[60%]'
              >
                <StickyNote rotation={4}>
                  <PaperClip
                    tone='brass'
                    size='sm'
                    rotation={12}
                    className='absolute -top-4 right-1'
                  />
                  <HandNote className='text-sm'>
                    {t('A little luck for your journey')}
                  </HandNote>
                </StickyNote>
              </div>
              <div
                aria-hidden='true'
                data-slot='auth-paper-marker'
                className='pointer-events-none mb-5 flex items-center gap-3'
              >
                <CloverDoodle variant='four-leaf' size='md' />
                <span className='bg-border h-px w-12' />
              </div>
              {props.children}
              <div
                aria-hidden='true'
                data-slot='auth-paper-botanical'
                className='pointer-events-none absolute -right-7 -bottom-6 flex items-end'
              >
                <CloverDoodle
                  variant='sprig'
                  size='lg'
                  className='size-16 -rotate-20'
                />
                <CloverDoodle
                  variant='four-leaf'
                  size='lg'
                  className='-ml-5 size-12 rotate-15'
                />
              </div>
            </div>
            <HandNote className='mt-7 block text-center text-base'>
              {t('Good things are always around you')}
            </HandNote>
          </div>
        </div>
      </div>
    </div>
  )
}
