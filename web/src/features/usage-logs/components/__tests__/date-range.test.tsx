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
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, expect, it, vi } from 'vitest'

import { CompactDateTimeRangePicker } from '../compact-date-time-range-picker'

afterEach(() => vi.useRealTimers())

it('selects local midnight through the current instant for Today', async () => {
  const now = new Date(2026, 8, 28, 15, 42, 37)
  vi.useFakeTimers({ toFake: ['Date'] })
  vi.setSystemTime(now)
  const i18n = createInstance()
  await i18n.init({ lng: 'en', resources: { en: { translation: {} } } })
  const onChange = vi.fn()
  render(
    <I18nextProvider i18n={i18n}>
      <CompactDateTimeRangePicker onChange={onChange} />
    </I18nextProvider>
  )
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Date Range' }))
  await user.click(screen.getByRole('button', { name: 'Today' }))
  expect(onChange).toHaveBeenCalledWith({
    start: new Date(2026, 8, 28),
    end: now,
  })
})
