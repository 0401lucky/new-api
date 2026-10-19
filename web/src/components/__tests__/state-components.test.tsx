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
import { fireEvent, render, screen } from '@testing-library/react'
import { createInstance } from 'i18next'
import type { ReactNode } from 'react'
import { I18nextProvider } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'

import en from '@/i18n/locales/en.json'

import { EmptyState } from '../empty-state'
import { ErrorState } from '../error-state'
import { LoadingState } from '../loading-state'

const i18n = createInstance()
await i18n.init({
  lng: 'en',
  resources: { en: { translation: en } },
  initAsync: false,
})

function renderState(ui: ReactNode) {
  return render(<I18nextProvider i18n={i18n}>{ui}</I18nextProvider>)
}

describe('shared state components in the journal theme', () => {
  it('renders the empty state title, description and action', () => {
    renderState(
      <div data-journal-level='medium'>
        <EmptyState
          title='No keys yet'
          description='Create your first key.'
          action={<button type='button'>Create</button>}
        />
      </div>
    )

    expect(screen.getByText('No keys yet')).toBeVisible()
    expect(screen.getByText('Create your first key.')).toBeVisible()
    expect(screen.getByRole('button', { name: 'Create' })).toBeVisible()
  })

  it('keeps the error retry callback connected', () => {
    const onRetry = vi.fn()
    renderState(
      <div data-journal-level='medium'>
        <ErrorState title='Load failed' onRetry={onRetry} />
      </div>
    )

    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(onRetry).toHaveBeenCalledTimes(1)
  })

  it('exposes the loading status and keeps the message', () => {
    renderState(<LoadingState message='Loading ledger...' />)

    expect(screen.getByRole('status', { name: 'Loading' })).toBeVisible()
    expect(screen.getByText('Loading ledger...')).toBeVisible()
  })
})
