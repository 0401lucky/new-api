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
import type { ReactNode } from 'react'
import { describe, expect, it, vi } from 'vitest'

import { Pricing } from '../index'

vi.mock('@visactor/react-vchart', () => ({ VChart: () => null }))

vi.mock('../hooks/use-pricing-data', () => ({
  usePricingData: () => ({
    models: [],
    vendors: [],
    groupRatio: {},
    usableGroup: {},
    endpointMap: {},
    autoGroups: [],
    isLoading: false,
    priceRate: 1,
    usdExchangeRate: 1,
  }),
}))

vi.mock('../hooks/use-filters', () => ({
  useFilters: () => ({
    searchInput: '',
    sortBy: 'name',
    vendorFilter: 'all',
    groupFilter: 'all',
    quotaTypeFilter: 'all',
    endpointTypeFilter: 'all',
    tagFilter: 'all',
    tokenUnit: 'M',
    viewMode: 'card',
    showRechargePrice: false,
    setSearchInput: vi.fn(),
    setSortBy: vi.fn(),
    setVendorFilter: vi.fn(),
    setGroupFilter: vi.fn(),
    setQuotaTypeFilter: vi.fn(),
    setEndpointTypeFilter: vi.fn(),
    setTagFilter: vi.fn(),
    setTokenUnit: vi.fn(),
    setViewMode: vi.fn(),
    setShowRechargePrice: vi.fn(),
    filteredModels: [],
    hasActiveFilters: false,
    activeFilterCount: 0,
    availableTags: [],
    clearFilters: vi.fn(),
    clearSearch: vi.fn(),
  }),
}))

vi.mock('@/components/layout', () => ({
  PublicLayout: (props: { children?: ReactNode }) => <div>{props.children}</div>,
}))

vi.mock('@/components/page-transition', () => ({
  PageTransition: (props: { children?: ReactNode; className?: string }) => (
    <div className={props.className}>{props.children}</div>
  ),
}))

describe('pricing journal composition', () => {
  it('marks the model square as immersive and keeps search, filters and empty state', () => {
    const { container } = render(<Pricing />)

    expect(container.querySelector('[data-slot="pricing-page"]')).toHaveAttribute(
      'data-journal-level',
      'immersive'
    )
    expect(
      screen.getByRole('textbox', { name: 'Search models' })
    ).toBeInTheDocument()
    expect(
      container.querySelector('[data-slot="pricing-sidebar"]')
    ).toBeInTheDocument()
    expect(
      container.querySelector('[data-slot="pricing-toolbar"]')
    ).toBeInTheDocument()
    expect(
      container.querySelector('[data-slot="pricing-empty"]')
    ).toBeInTheDocument()
    expect(
      screen.getByRole('heading', { name: 'Model Square' })
    ).toBeInTheDocument()
  })

  it('keeps the handwritten annotation decorative', () => {
    const { container } = render(<Pricing />)

    const notes = container.querySelectorAll('.journal-hand')
    expect(notes.length).toBeGreaterThan(0)
    notes.forEach((note) => {
      expect(note).toHaveAttribute('aria-hidden', 'true')
      expect(note).toHaveClass('pointer-events-none')
    })
  })
})
