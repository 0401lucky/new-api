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
import { beforeEach, describe, expect, it, vi } from 'vitest'

import en from '@/i18n/locales/en.json'

import { ForbiddenError } from '../forbidden'
import { GeneralError } from '../general-error'
import { MaintenanceError } from '../maintenance-error'
import { NotFoundError } from '../not-found-error'
import { UnauthorisedError } from '../unauthorized-error'

const navigateMock = vi.fn()
const historyGoMock = vi.fn()

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => navigateMock,
  useRouter: () => ({ history: { go: historyGoMock } }),
}))

const i18n = createInstance()
await i18n.init({
  lng: 'en',
  resources: { en: { translation: en } },
  initAsync: false,
})

function renderPage(ui: ReactNode) {
  return render(<I18nextProvider i18n={i18n}>{ui}</I18nextProvider>)
}

beforeEach(() => {
  navigateMock.mockReset()
  historyGoMock.mockReset()
})

describe('error page journal contract', () => {
  it.each([
    { code: '404', ui: <NotFoundError /> },
    { code: '401', ui: <UnauthorisedError /> },
    { code: '403', ui: <ForbiddenError /> },
    { code: '503', ui: <MaintenanceError /> },
  ])('keeps the $code status and medium-level paper note', ({ code, ui }) => {
    renderPage(ui)

    expect(screen.getByRole('heading', { name: code })).toBeVisible()
    expect(
      document.querySelector("[data-slot='error-page']")
    ).toHaveAttribute('data-journal-level', 'medium')
    expect(document.querySelector("[data-slot='error-note']")).toBeVisible()
  })

  it('keeps Go Back and Back to Home navigation on the 404 page', () => {
    renderPage(<NotFoundError />)

    fireEvent.click(screen.getByRole('button', { name: 'Go Back' }))
    expect(historyGoMock).toHaveBeenCalledWith(-1)

    fireEvent.click(screen.getByRole('button', { name: 'Back to Home' }))
    expect(navigateMock).toHaveBeenCalledWith({ to: '/' })
  })

  it('keeps the 429 status and retry guidance in the general error page', () => {
    renderPage(<GeneralError error={{ response: { status: 429 } }} />)

    expect(screen.getByRole('heading', { name: '429' })).toBeVisible()
    expect(screen.getByText('Too many requests')).toBeVisible()
    expect(document.querySelector("[data-slot='error-note']")).toHaveTextContent(
      'Please wait a moment before trying again.'
    )
  })
})
