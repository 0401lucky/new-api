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
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, test, vi } from 'vitest'

vi.mock('@/hooks', () => ({ useMediaQuery: () => false }))
vi.mock('@/components/layout', () => ({ SectionPageLayout: () => null }))

const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { Toaster, toast } = await import('sonner')
const { api } = await import('@/lib/api')
const { InviteCodeMutateDrawer } = await import('../index')

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { translation: {} } },
})

type ApiMethod = (url: string, data?: unknown) => Promise<{ data: unknown }>
type MockableApi = {
  post: ApiMethod
}

const apiClient = api as unknown as MockableApi
const originalPost = apiClient.post
const originalClipboardDescriptor = Object.getOwnPropertyDescriptor(
  navigator,
  'clipboard'
)
const originalCreateObjectUrlDescriptor = Object.getOwnPropertyDescriptor(
  URL,
  'createObjectURL'
)

function renderCreateDrawer() {
  render(
    <I18nextProvider i18n={i18n}>
      <InviteCodeMutateDrawer
        open
        currentRow={null}
        onOpenChange={() => undefined}
        onRefresh={() => undefined}
      />
      <Toaster duration={60_000} />
    </I18nextProvider>
  )
}

afterEach(() => {
  apiClient.post = originalPost
  toast.dismiss()

  if (originalClipboardDescriptor) {
    Object.defineProperty(navigator, 'clipboard', originalClipboardDescriptor)
  } else {
    Reflect.deleteProperty(navigator, 'clipboard')
  }

  if (originalCreateObjectUrlDescriptor) {
    Object.defineProperty(
      URL,
      'createObjectURL',
      originalCreateObjectUrlDescriptor
    )
  } else {
    Reflect.deleteProperty(URL, 'createObjectURL')
  }
})

describe('invite code output actions', () => {
  test('creates through the dropdown and copies multiple codes without downloading', async () => {
    const createdPayloads: Array<Record<string, unknown>> = []
    apiClient.post = async (url, data) => {
      expect(url).toBe('/api/invitation/')
      expect(data && typeof data === 'object').toBeTruthy()
      createdPayloads.push(data as Record<string, unknown>)
      return {
        data: {
          success: true,
          data: ['invite-code-1', 'invite-code-2'],
        },
      }
    }

    const writeText = vi.fn(async () => undefined)
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText },
    })
    const createObjectURL = vi.fn(() => 'blob:invite-codes')
    Object.defineProperty(URL, 'createObjectURL', {
      configurable: true,
      value: createObjectURL,
    })

    renderCreateDrawer()

    fireEvent.input(screen.getByLabelText('Name'), {
      target: { value: 'campaign' },
    })
    fireEvent.click(
      screen.getByRole('button', { name: 'More creation options' })
    )
    fireEvent.click(
      await screen.findByRole('menuitem', { name: 'Create and copy' })
    )

    await waitFor(() => expect(createdPayloads).toHaveLength(1))
    await waitFor(() =>
      expect(writeText).toHaveBeenCalledWith('invite-code-1\ninvite-code-2')
    )

    expect(createdPayloads[0]?.name).toBe('campaign')
    expect(createObjectURL).not.toHaveBeenCalled()
    expect(await screen.findByText('Copied to clipboard')).toBeVisible()
  })
})
