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

import { Home } from '../index'

vi.mock('../hooks', () => ({
  useHomePageContent: () => ({
    content: '',
    isLoaded: true,
    isUrl: false,
  }),
}))

vi.mock('@/components/layout', () => ({
  PublicLayout: (props: { children?: ReactNode }) => (
    <div data-testid='public-layout'>{props.children}</div>
  ),
}))

vi.mock('@/components/layout/components/footer', () => ({
  Footer: () => <footer data-testid='footer' />,
}))

vi.mock('@/context/theme-provider', () => ({
  useTheme: () => ({ resolvedTheme: 'light' }),
}))

vi.mock('@/hooks/use-status', () => ({
  useStatus: () => ({
    status: { docs_link: 'https://docs.example.com' },
    loading: false,
    error: null,
  }),
}))

vi.mock('@tanstack/react-router', () => ({
  Link: (props: { to: string; children: ReactNode }) => (
    <a href={props.to}>{props.children}</a>
  ),
}))

describe('home journal composition', () => {
  it('marks the default home as immersive and keeps both primary CTAs', () => {
    const { container } = render(<Home />)

    const page = container.querySelector('[data-slot="home-journal"]')
    expect(page).toHaveAttribute('data-journal-level', 'immersive')
    const getStarted = screen.getAllByRole('link', { name: /Get Started/ })
    expect(getStarted.length).toBeGreaterThan(1)
    for (const link of getStarted) {
      expect(link).toHaveAttribute('href', '/sign-up')
    }
    const pricingLinks = screen.getAllByRole('link', { name: 'View Pricing' })
    expect(pricingLinks.length).toBeGreaterThan(1)
    for (const link of pricingLinks) {
      expect(link).toHaveAttribute('href', '/pricing')
    }
    expect(screen.getByTestId('footer')).toBeInTheDocument()
  })

  it('keeps the hero collage decorative, accessible and inside its section', () => {
    const { container } = render(<Home />)

    const hero = container.querySelector('[data-slot="home-hero"]')
    expect(hero).toHaveClass('overflow-hidden')
    expect(
      container.querySelector('[data-slot="home-hero-polaroid"]')
    ).toBeInTheDocument()
    const frame = container.querySelector('[data-slot="home-polaroid-frame"]')
    expect(frame).toBeInTheDocument()
    expect(
      frame?.querySelector('[data-slot="home-terminal"]')
    ).toBeInTheDocument()

    const decorations = container.querySelectorAll(
      '.journal-hand, .journal-tape, .journal-doodle'
    )
    expect(decorations.length).toBeGreaterThan(0)
    decorations.forEach((node) => {
      expect(node).toHaveAttribute('aria-hidden', 'true')
      expect(node).toHaveClass('pointer-events-none')
    })
  })

  it('keeps the stats and section anchors available for the paper skins', () => {
    const { container } = render(<Home />)

    expect(container.querySelector('[data-slot="home-stats"]')).toBeVisible()
    expect(
      container.querySelectorAll('[data-slot="home-stat"]')
    ).toHaveLength(4)
    expect(
      container.querySelector('[data-slot="home-features-grid"]')
    ).toBeInTheDocument()
    expect(
      container.querySelector('[data-slot="home-how-it-works"]')
    ).toBeInTheDocument()
    expect(container.querySelector('[data-slot="home-cta-card"]')).toBeVisible()
    expect(
      screen.getByRole('heading', { name: /Built for developers/ })
    ).toBeVisible()
  })
})
