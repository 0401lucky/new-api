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
import { render, screen } from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'

import en from '@/i18n/locales/en.json'

import { ApiKeys } from '..'

vi.mock('@/hooks/use-status', () => ({
  useStatus: () => ({ status: undefined, loading: false }),
}))
vi.mock('@/features/dashboard/hooks/use-status-data', () => ({
  useApiInfo: () => ({ items: [] }),
}))
vi.mock('../components/api-keys-table', () => ({
  ApiKeysTable: () => <div>api key table</div>,
}))
vi.mock('../components/api-keys-dialogs', () => ({
  ApiKeysDialogs: () => null,
}))

const i18n = createInstance()
await i18n.init({
  lng: 'en',
  resources: { en: { translation: en } },
  initAsync: false,
})

describe('API keys journal level', () => {
  it('keeps the page controls and marks the B-level container as medium', () => {
    render(
      <I18nextProvider i18n={i18n}>
        <ApiKeys />
      </I18nextProvider>
    )

    expect(screen.getByRole('main')).toHaveAttribute(
      'data-journal-level',
      'medium'
    )
    expect(screen.getByRole('heading', { name: 'API Keys' })).toBeVisible()
    expect(
      screen.getByRole('button', { name: 'Create API Key' })
    ).toBeVisible()
    expect(screen.getByText('api key table')).toBeVisible()
  })
})
