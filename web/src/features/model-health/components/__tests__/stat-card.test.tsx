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
import { describe, expect, test } from 'vitest'

import { StatCard, StatCardSkeleton } from '../stat-card'

describe('model health stat card', () => {
  test('shows title, value and subtitle as visible text with a decorative icon', () => {
    render(
      <StatCard
        title='Overall success rate'
        icon={<svg data-testid='stat-icon' />}
        value='91.79%'
        subtitle='Past 24 hours'
      />
    )

    expect(screen.getByText('Overall success rate')).toBeInTheDocument()
    expect(screen.getByText('91.79%')).toBeInTheDocument()
    expect(screen.getByText('Past 24 hours')).toBeInTheDocument()
    expect(screen.getByTestId('stat-icon').parentElement).toHaveAttribute(
      'aria-hidden',
      'true'
    )
  })

  test('omits the subtitle row when no subtitle is provided', () => {
    const rendered = render(
      <StatCard title='Total tokens' icon={<svg />} value='214.8M' />
    )

    expect(screen.getByText('214.8M')).toBeInTheDocument()
    const valueNode = screen.getByText('214.8M')
    expect(valueNode.parentElement?.childElementCount).toBe(1)
    expect(rendered.container.textContent).toBe('Total tokens214.8M')
  })

  test('loading skeleton keeps the same visible title so the tile stays identifiable', () => {
    render(
      <StatCardSkeleton
        title='Monitored models'
        icon={<svg data-testid='skeleton-icon' />}
        valueWidth={72}
      />
    )

    expect(screen.getByText('Monitored models')).toBeInTheDocument()
    expect(screen.getByTestId('skeleton-icon').parentElement).toHaveAttribute(
      'aria-hidden',
      'true'
    )
  })
})
