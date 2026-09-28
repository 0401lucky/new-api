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
import { afterEach, describe, expect, it } from 'vitest'

import { ModelUsageTable } from '../model-usage-table'

const data = [
  { model_name: 'alpha', created_at: 1, count: 3, token_used: 1200000 },
  { model_name: 'alpha', created_at: 2, count: 4, token_used: 34 },
  { model_name: 'beta', created_at: 1, count: 9, token_used: 200 },
  { model_name: 'image', created_at: 1, count: 2, token_used: 0 },
]

afterEach(async () => {
  await i18next.changeLanguage('en')
})

describe('model usage details', () => {
  it('updates number formatting when the interface language changes', async () => {
    render(<ModelUsageTable data={data} />)
    for (const [language, expected] of [
      ['zhCN', '1,200,034'],
      ['zhTW', '1,200,034'],
      ['en', '1,200,034'],
      ['fr', '1\u202f200\u202f034'],
      ['ja', '1,200,034'],
      ['ru', '1\u00a0200\u00a0034'],
      ['vi', '1.200.034'],
      ['invalid_locale', '1,200,034'],
    ]) {
      await act(async () => {
        await i18next.changeLanguage(language)
      })
      const row = screen.getByRole('row', { name: /^alpha / })
      expect(within(row).getAllByRole('cell')[2].textContent).toBe(expected)
    }
  })
  it('sums all time buckets per model and shows full counts including zero tokens', () => {
    render(<ModelUsageTable data={data} />)
    const rows = within(screen.getByRole('table')).getAllByRole('row')
    expect(rows).toHaveLength(4)
    expect(rows[1]).toHaveTextContent('beta')
    expect(rows[2]).toHaveTextContent('alpha')
    expect(
      within(rows[2])
        .getAllByRole('cell')
        .map((cell) => cell.textContent)
    ).toEqual(['alpha', '7', '1,200,034'])
    expect(
      within(rows[3])
        .getAllByRole('cell')
        .map((cell) => cell.textContent)
    ).toEqual(['image', '2', '0'])
  })

  it('sorts by tokens and filters model names without dropping matching counts', async () => {
    render(<ModelUsageTable data={data} />)
    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: 'Total Tokens' }))
    await user.click(screen.getByRole('menuitem', { name: 'Desc' }))
    expect(
      within(screen.getByRole('table')).getAllByRole('row')[1]
    ).toHaveTextContent('alpha')
    await user.type(
      screen.getByRole('textbox', { name: 'Search models...' }),
      'ALPHA'
    )
    expect(within(screen.getByRole('table')).getAllByRole('row')).toHaveLength(
      2
    )
    expect(screen.getByRole('cell', { name: '1,200,034' })).toBeVisible()
    await user.clear(screen.getByPlaceholderText('Search models...'))
    await user.type(screen.getByPlaceholderText('Search models...'), 'missing')
    expect(screen.getByText('No Data')).toBeVisible()
  })

  it('paginates every model and returns to the first page when filtering', async () => {
    render(
      <ModelUsageTable
        data={Array.from({ length: 11 }, (_, i) => ({
          model_name: `model-${i}`,
          created_at: 1,
          count: 11 - i,
          token_used: i,
        }))}
      />
    )
    const user = userEvent.setup()
    expect(within(screen.getByRole('table')).getAllByRole('row')).toHaveLength(
      11
    )
    await user.click(screen.getByRole('button', { name: 'Go to next page' }))
    expect(screen.getByRole('cell', { name: 'model-10' })).toBeVisible()
    await user.type(screen.getByPlaceholderText('Search models...'), 'model-0')
    expect(screen.getByRole('cell', { name: 'model-0' })).toBeVisible()
    expect(
      screen.getByRole('button', { name: 'Go to previous page' })
    ).toBeDisabled()
  })

  it('replaces stale data with loading, empty, and error states', () => {
    const { rerender } = render(<ModelUsageTable data={data} loading />)
    expect(
      screen.getByRole('region', { name: 'Model Usage Details' })
    ).toHaveAttribute('aria-busy', 'true')
    expect(
      screen.queryByRole('cell', { name: 'alpha' })
    ).not.toBeInTheDocument()
    rerender(<ModelUsageTable data={[]} />)
    expect(screen.getByText('No Data')).toBeVisible()
    rerender(<ModelUsageTable data={[]} error />)
    expect(screen.getByText('Oops! Something went wrong')).toBeVisible()
    expect(screen.queryByText('No Data')).not.toBeInTheDocument()
  })

  it('keeps long model names readable within a horizontally scrollable table', () => {
    const model = `provider/${'long-model-name-'.repeat(15)}`
    render(<ModelUsageTable data={[{ model_name: model, created_at: 1 }]} />)
    expect(screen.getByText(model)).toHaveClass(
      'break-all',
      'whitespace-normal'
    )
    expect(screen.getByRole('table').parentElement?.parentElement).toHaveClass(
      'overflow-x-auto'
    )
    expect(screen.getAllByRole('cell').map((cell) => cell.textContent)).toEqual(
      [model, '0', '0']
    )
  })
})
