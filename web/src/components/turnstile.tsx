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
import { Check, X } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { CloverLogo, type CloverMood } from '@/components/clover-logo'
import { useTheme } from '@/context/theme-provider'
import { cn } from '@/lib/utils'

declare global {
  interface Window {
    turnstile?: {
      render: (element: HTMLElement, options: Record<string, unknown>) => string
      remove: (widgetId: string) => void
    }
  }
}

interface TurnstileProps {
  siteKey: string
  onVerify: (token: string) => void
  onExpire?: () => void
  className?: string
}

type VerifyStatus = 'loading' | 'ready' | 'success' | 'error'

const STATUS_STYLES: Record<
  VerifyStatus,
  { mood: CloverMood; text: string; textClass: string }
> = {
  loading: {
    mood: 'normal',
    text: 'Verifying',
    textClass: 'text-[#577748] dark:text-[#a7cf8c]',
  },
  ready: {
    mood: 'normal',
    text: 'Please complete the security check to continue.',
    textClass: 'text-[#577748] dark:text-[#a7cf8c]',
  },
  success: {
    mood: 'happy',
    text: 'Verified',
    textClass: 'text-[#3b7d3c] dark:text-[#9ada8f]',
  },
  error: {
    mood: 'sad',
    text: 'Verification failed',
    textClass: 'text-[#ad4f63] dark:text-[#f0a8ba]',
  },
}

/**
 * Cloudflare Turnstile wrapped in a soft clover-themed verification card.
 *
 * The widget is an iframe and cannot be restyled from the outside, so we use
 * Turnstile's own options (app theme, flexible width, visitor language) and
 * surround it with the brand mascot, which reacts to the verification state.
 */
export function Turnstile(props: TurnstileProps) {
  const { t, i18n } = useTranslation()
  const { resolvedTheme } = useTheme()
  const ref = useRef<HTMLDivElement | null>(null)
  const widgetIdRef = useRef<string | undefined>(undefined)
  const onVerifyRef = useRef(props.onVerify)
  const onExpireRef = useRef(props.onExpire)
  const [status, setStatus] = useState<VerifyStatus>('loading')

  onVerifyRef.current = props.onVerify
  onExpireRef.current = props.onExpire

  useEffect(() => {
    let script: HTMLScriptElement | undefined

    const render = () => {
      if (!ref.current || !window.turnstile) return

      try {
        if (widgetIdRef.current) {
          window.turnstile.remove(widgetIdRef.current)
          widgetIdRef.current = undefined
        }

        setStatus('loading')

        const options: Record<string, unknown> = {
          sitekey: props.siteKey,
          theme: resolvedTheme === 'dark' ? 'dark' : 'light',
          size: 'flexible',
          callback: (token: string) => {
            setStatus('success')
            onVerifyRef.current(token)
          },
          'before-interactive-callback': () => setStatus('ready'),
          'error-callback': () => {
            setStatus('error')
            onExpireRef.current?.()
          },
          'expired-callback': () => {
            setStatus('error')
            onExpireRef.current?.()
          },
        }

        // Turnstile accepts ISO 639-1 or lang-country codes; only forward a
        // plain two-letter code so an unknown value never breaks the widget.
        const rawLang = i18n?.language?.slice(0, 2).toLowerCase()
        if (rawLang && /^[a-z]{2}$/.test(rawLang)) {
          options.language = rawLang
        }

        widgetIdRef.current = window.turnstile.render(ref.current, options)
      } catch {
        /* empty */
      }
    }

    if (window.turnstile) {
      render()
    } else {
      const scriptId = 'cf-turnstile'
      script =
        document.querySelector<HTMLScriptElement>(`#${scriptId}`) ?? undefined
      if (!script) {
        script = document.createElement('script')
        script.id = scriptId
        script.src =
          'https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit'
        script.async = true
        script.defer = true
        document.head.appendChild(script)
      }
      // Every instance waits on the shared script, so a second widget still
      // renders instead of silently bailing out because the tag already exists.
      script.addEventListener('load', render)
    }

    return () => {
      script?.removeEventListener('load', render)
      if (!widgetIdRef.current) return
      try {
        window.turnstile?.remove(widgetIdRef.current)
      } catch {
        /* empty */
      }
      widgetIdRef.current = undefined
    }
  }, [i18n, props.siteKey, resolvedTheme])

  const state = STATUS_STYLES[status]

  return (
    <div
      className={cn(
        'relative overflow-hidden rounded-2xl border p-3.5',
        'border-[#dbefc6] bg-gradient-to-b from-[#f8fcf2] to-white',
        'dark:border-[#33422d] dark:from-[#1d251b] dark:to-[#171d16]',
        props.className
      )}
    >
      {/* Soft watercolor bloom behind the mascot. */}
      <div
        aria-hidden='true'
        className='pointer-events-none absolute -top-8 -left-6 h-24 w-24 rounded-full bg-[#cbe9a8]/35 blur-2xl dark:bg-[#4d7a3e]/25'
      />

      <div className='relative flex items-center gap-3'>
        <div className='relative shrink-0'>
          <CloverLogo
            mood={state.mood}
            className={cn(
              'h-11 w-11 transition-transform duration-300',
              status === 'loading' && 'animate-pulse',
              status === 'success' && 'scale-105'
            )}
          />
          {status === 'success' && (
            <span className='absolute -right-0.5 -bottom-0.5 flex h-4 w-4 items-center justify-center rounded-full bg-[#5fae57] ring-2 ring-white dark:ring-[#1d251b]'>
              <Check className='h-2.5 w-2.5 text-white' strokeWidth={3.5} />
            </span>
          )}
          {status === 'error' && (
            <span className='absolute -right-0.5 -bottom-0.5 flex h-4 w-4 items-center justify-center rounded-full bg-[#e88ca0] ring-2 ring-white dark:ring-[#1d251b]'>
              <X className='h-2.5 w-2.5 text-white' strokeWidth={3.5} />
            </span>
          )}
        </div>

        <div className='min-w-0 flex-1'>
          <p className='text-sm font-medium text-[#3f5236] dark:text-[#dfeed3]'>
            {t('Security verification')}
          </p>
          {/* Wraps rather than truncates so the instruction stays readable, and
              is announced politely as the verification state changes. */}
          <p
            role='status'
            className={cn('text-xs leading-snug', state.textClass)}
          >
            {t(state.text)}
          </p>
        </div>
      </div>

      <div className='relative mt-3 flex justify-center overflow-hidden rounded-xl'>
        <div ref={ref} className='w-full' />
      </div>
    </div>
  )
}
