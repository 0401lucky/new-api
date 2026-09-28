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
import { afterAll, afterEach, beforeAll, expect, it, vi } from 'vitest'

import { DEFAULT_DASHBOARD_CHART_PREFERENCES } from '@/features/dashboard/constants'
import {
  buildDefaultDashboardFilters,
  getSavedChartPreferences,
  saveChartPreferences,
} from '@/features/dashboard/lib/filters'
import { computeTimeRange, getPresetDateRange } from '@/lib/time'

import { ModelsFilter } from '../models-filter-dialog'

afterEach(() => {
  vi.useRealTimers()
  localStorage.clear()
})

beforeAll(() => {
  Object.defineProperty(HTMLElement.prototype, 'getAnimations', {
    configurable: true,
    value: () => [],
  })
})
afterAll(() => {
  Reflect.deleteProperty(HTMLElement.prototype, 'getAnimations')
})

it('applies Today at local midnight through the time of applying and uses hourly buckets', async () => {
  vi.useFakeTimers({ toFake: ['Date'] })
  vi.setSystemTime(new Date(2026, 8, 28, 15, 30))
  const onFilterChange = vi.fn()
  render(
    <ModelsFilter
      preferences={DEFAULT_DASHBOARD_CHART_PREFERENCES}
      currentFilters={buildDefaultDashboardFilters()}
      onFilterChange={onFilterChange}
      onReset={vi.fn()}
    />
  )
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Filter' }))
  await user.click(screen.getByRole('button', { name: 'Today' }))
  expect(screen.getByRole('button', { name: 'Today' })).toHaveAttribute(
    'aria-pressed',
    'true'
  )
  expect(screen.getByRole('button', { name: '1 Day' })).toHaveAttribute(
    'aria-pressed',
    'false'
  )
  const now = new Date(2026, 8, 28, 15, 32, 12)
  vi.setSystemTime(now)
  await user.click(screen.getByRole('button', { name: 'Apply Filters' }))
  expect(onFilterChange).toHaveBeenCalledWith(
    expect.objectContaining({
      start_timestamp: new Date(2026, 8, 28),
      end_timestamp: now,
      time_granularity: 'hour',
      rangePreset: 'today',
    })
  )
})

it('preserves a custom midnight-to-noon range rather than treating it as Today or 1 Day', async () => {
  const filters = {
    start_timestamp: new Date(2026, 8, 28),
    end_timestamp: new Date(2026, 8, 28, 12),
  }
  const onFilterChange = vi.fn()
  render(
    <ModelsFilter
      preferences={DEFAULT_DASHBOARD_CHART_PREFERENCES}
      currentFilters={filters}
      onFilterChange={onFilterChange}
      onReset={vi.fn()}
    />
  )
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Filter' }))
  expect(screen.getByRole('button', { name: 'Today' })).toHaveAttribute(
    'aria-pressed',
    'false'
  )
  expect(screen.getByRole('button', { name: '1 Day' })).toHaveAttribute(
    'aria-pressed',
    'false'
  )
  await user.click(screen.getByRole('button', { name: 'Apply Filters' }))
  expect(onFilterChange).toHaveBeenCalledWith(filters)
})

it('keeps 1 Day as exactly 24 hours and resolves saved Today defaults on the current date', () => {
  const now = new Date(2026, 8, 28, 15, 32, 12)
  vi.useFakeTimers({ toFake: ['Date'] })
  vi.setSystemTime(now)
  expect(getPresetDateRange(1)).toEqual({
    start: new Date(2026, 8, 27, 15, 32, 12),
    end: now,
  })
  saveChartPreferences({
    ...DEFAULT_DASHBOARD_CHART_PREFERENCES,
    defaultTimeRangeDays: 'today',
  })
  expect(getSavedChartPreferences().defaultTimeRangeDays).toBe('today')
  expect(buildDefaultDashboardFilters().start_timestamp).toEqual(
    new Date(2026, 8, 28)
  )
  expect(computeTimeRange('today')).toEqual({
    start_timestamp: new Date(2026, 8, 28).getTime() / 1000,
    end_timestamp: now.getTime() / 1000,
  })
  vi.setSystemTime(new Date(2026, 8, 29, 0, 0, 0))
  expect(getPresetDateRange('today')).toEqual({
    start: new Date(2026, 8, 29),
    end: new Date(2026, 8, 29),
  })
})
