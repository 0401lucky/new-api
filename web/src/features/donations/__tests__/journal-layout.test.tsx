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
import type { ReactNode } from 'react'
import { I18nextProvider } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'

import en from '@/i18n/locales/en.json'

import { DonationLayout } from '../components/donation-layout'

vi.mock('@tanstack/react-router', async () => {
  const React = await import('react')
  return {
    Link: (props: { to: string; children: ReactNode }) =>
      React.createElement('a', { href: props.to }, props.children),
  }
})

const i18n = createInstance()
await i18n.init({
  lng: 'en',
  resources: { en: { translation: en } },
  initAsync: false,
})

describe('donation journal level', () => {
  it('keeps donation navigation and marks the page as medium', () => {
    render(
      <I18nextProvider i18n={i18n}>
        <DonationLayout page='donate'>
          <div>donation body</div>
        </DonationLayout>
      </I18nextProvider>
    )

    expect(screen.getByRole('main')).toHaveAttribute(
      'data-journal-level',
      'medium'
    )
    expect(screen.getByRole('heading', { name: 'Donations' })).toBeVisible()
    expect(screen.getByRole('link', { name: 'My donations' })).toBeVisible()
    expect(screen.getByText('donation body')).toBeVisible()
  })
})
