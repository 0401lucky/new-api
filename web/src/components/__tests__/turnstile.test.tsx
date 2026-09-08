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
import { afterEach, assert, describe, test, vi } from 'vitest'

const { act } = await import('react')
const { createRoot } = await import('react-dom/client')
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { ThemeProvider } = await import('@/context/theme-provider')
const { Turnstile } = await import('@/components/turnstile')

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: {
    en: {
      translation: {
        'Security verification': 'Security verification',
        Verifying: 'Verifying',
        Verified: 'Verified',
        'Verification failed': 'Verification failed',
        'Please complete the security check to continue.':
          'Please complete the security check to continue.',
      },
    },
  },
})

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

type TurnstileCallbacks = Record<string, (arg?: unknown) => void>
let lastRenderOptions: TurnstileCallbacks | null = null
let renderedSiteKey: string | null = null
let renderedTheme: string | null = null
let removedWidgetIds: string[] = []

function installTurnstileMock() {
  const w = window as unknown as Record<string, unknown>
  w.turnstile = {
    render: (_element: HTMLElement, options: Record<string, unknown>) => {
      renderedSiteKey = options.sitekey as string
      renderedTheme = options.theme as string
      lastRenderOptions = options as unknown as TurnstileCallbacks
      return 'widget-1'
    },
    remove: (widgetId: string) => {
      removedWidgetIds.push(widgetId)
    },
  }
}

function Harness(props: {
  siteKey: string
  onVerify: (token: string) => void
  onExpire?: () => void
  theme?: 'light' | 'dark'
}) {
  return (
    <I18nextProvider i18n={i18n}>
      <ThemeProvider defaultTheme={props.theme ?? 'light'}>
        <Turnstile
          siteKey={props.siteKey}
          onVerify={props.onVerify}
          onExpire={props.onExpire}
        />
      </ThemeProvider>
    </I18nextProvider>
  )
}

describe('Turnstile branded verification card', () => {
  afterEach(() => {
    localStorage.clear()
    vi.restoreAllMocks()
    Reflect.deleteProperty(window, 'turnstile')
  })

  test('renders the brand card with clover logo and verifying status', async () => {
    installTurnstileMock()
    renderedSiteKey = null
    lastRenderOptions = null

    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)
    const onVerify = () => {}

    await act(async () =>
      root.render(<Harness siteKey='test-site-key' onVerify={onVerify} />)
    )

    assert.equal(renderedSiteKey, 'test-site-key')
    assert.ok(container.querySelector('svg'))
    assert.equal(container.textContent?.includes('Security verification'), true)
    assert.equal(container.textContent?.includes('Verifying'), true)

    await act(async () => root.unmount())
    container.remove()
  })

  test('passes the app theme to the widget so it matches light and dark mode', async () => {
    installTurnstileMock()
    renderedTheme = null

    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)

    await act(async () =>
      root.render(<Harness siteKey='k' onVerify={() => {}} theme='dark' />)
    )
    assert.equal(renderedTheme, 'dark')

    await act(async () => root.unmount())
    container.remove()
  })

  test('switches to verified status and forwards the token on success', async () => {
    installTurnstileMock()
    lastRenderOptions = null
    let receivedToken: string | null = null

    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)

    await act(async () =>
      root.render(
        <Harness
          siteKey='k'
          onVerify={(token) => {
            receivedToken = token
          }}
        />
      )
    )

    await act(async () => {
      lastRenderOptions?.callback?.('token-123')
    })

    assert.equal(receivedToken, 'token-123')
    assert.equal(container.textContent?.includes('Verified'), true)
    assert.equal(
      container.querySelector('svg[data-clover-mood="happy"]') !== null,
      true
    )

    await act(async () => root.unmount())
    container.remove()
  })

  test('shows the failure status and calls onExpire on widget error', async () => {
    installTurnstileMock()
    lastRenderOptions = null
    let expired = false

    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)

    await act(async () =>
      root.render(
        <Harness
          siteKey='k'
          onVerify={() => {}}
          onExpire={() => {
            expired = true
          }}
        />
      )
    )

    await act(async () => {
      lastRenderOptions?.['error-callback']?.()
    })

    assert.equal(expired, true)
    assert.equal(container.textContent?.includes('Verification failed'), true)
    assert.equal(
      container.querySelector('svg[data-clover-mood="sad"]') !== null,
      true
    )

    await act(async () => root.unmount())
    container.remove()
  })

  test('re-renders the widget when the site key changes', async () => {
    installTurnstileMock()
    removedWidgetIds = []
    renderedSiteKey = null

    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)

    await act(async () =>
      root.render(<Harness siteKey='key-a' onVerify={() => {}} />)
    )
    assert.equal(renderedSiteKey, 'key-a')

    await act(async () =>
      root.render(<Harness siteKey='key-b' onVerify={() => {}} />)
    )
    assert.equal(renderedSiteKey, 'key-b')
    assert.ok(removedWidgetIds.includes('widget-1'))

    await act(async () => root.unmount())
    container.remove()
  })

  test('removes the widget on unmount so a reopened dialog gets a fresh one', async () => {
    installTurnstileMock()
    removedWidgetIds = []

    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)

    await act(async () =>
      root.render(<Harness siteKey='k' onVerify={() => {}} />)
    )
    await act(async () => root.unmount())

    assert.deepEqual(removedWidgetIds, ['widget-1'])
    container.remove()
  })
})
