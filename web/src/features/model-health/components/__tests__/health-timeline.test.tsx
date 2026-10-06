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
import { act, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { expect, test } from 'vitest'

import { TooltipProvider } from '@/components/ui/tooltip'

import { HealthTimeline } from '../health-timeline'

test.each([
  ['zhCN', '1,234'],
  ['zhTW', '1,234'],
  ['en', '1,234'],
  ['fr', '1\u202f234'],
  ['ru', '1\u00a0234'],
  ['ja', '1,234'],
  ['vi', '1.234'],
  ['invalid_locale', '1,234'],
])(
  'request counts support interface locale %s and update when the language changes',
  async (language, expected) => {
    const instance = i18next.createInstance()
    const languages = [
      'zhCN',
      'zhTW',
      'en',
      'fr',
      'ru',
      'ja',
      'vi',
      'invalid_locale',
    ]
    await instance.init({
      lng: language,
      fallbackLng: 'en',
      nsSeparator: false,
      resources: Object.fromEntries(
        languages.map((code) => [
          code,
          { translation: { 'Total requests': 'Total requests' } },
        ])
      ),
    })
    const user = userEvent.setup()
    render(
      <I18nextProvider i18n={instance}>
        <TooltipProvider delay={0}>
          <HealthTimeline
            items={[
              {
                hour_start_ts: 3600,
                total_requests: 1234,
                success_requests: 1234,
                error_requests: 0,
                qualified_success_requests: 1234,
                success_rate: 1,
                success_tokens: 1234,
              },
            ]}
          />
        </TooltipProvider>
      </I18nextProvider>
    )
    await user.tab()
    const tooltip = await screen.findByRole('tooltip')
    expect(within(tooltip).getByText(/Total requests:/).textContent).toContain(
      expected
    )
    await act(async () => {
      await instance.changeLanguage('vi')
    })
    expect(within(tooltip).getByText(/Total requests:/).textContent).toContain(
      '1.234'
    )
  }
)

test('keyboard focus exposes successful, failed and threshold counts separately for short replies', async () => {
  const user = userEvent.setup()
  render(
    <TooltipProvider delay={0}>
      <HealthTimeline
        items={[
          {
            hour_start_ts: 3600,
            total_requests: 22,
            success_requests: 22,
            error_requests: 0,
            qualified_success_requests: 19,
            success_rate: 1,
            success_tokens: 409007,
          },
        ]}
      />
    </TooltipProvider>
  )
  await user.tab()
  expect(screen.getByRole('button', { name: /100.00%/ })).toHaveFocus()
  const tooltip = await screen.findByRole('tooltip')
  expect(within(tooltip).getByText(/Successful requests:/)).toHaveTextContent(
    '22'
  )
  expect(within(tooltip).getByText(/Error requests:/)).toHaveTextContent('0')
  expect(within(tooltip).getByText(/Content threshold met:/)).toHaveTextContent(
    '19'
  )
  expect(within(tooltip).getByText(/Valid short replies count/)).toBeVisible()
})

test('an hour without samples displays no data rather than a zero success rate', async () => {
  const user = userEvent.setup()
  render(
    <TooltipProvider delay={0}>
      <HealthTimeline
        items={[
          {
            hour_start_ts: 3600,
            total_requests: 0,
            success_requests: 0,
            error_requests: 0,
            qualified_success_requests: 0,
            success_rate: 0,
            success_tokens: 0,
          },
        ]}
      />
    </TooltipProvider>
  )
  await user.tab()
  const tooltip = await screen.findByRole('tooltip')
  expect(within(tooltip).getByText('No data')).toBeVisible()
  expect(within(tooltip).queryByText('0.00%')).not.toBeInTheDocument()
})
