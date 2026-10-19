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

import { AuthLayout } from '../auth-layout'

vi.mock('@/hooks/use-system-config', () => ({
  useSystemConfig: () => ({
    systemName: 'Test API',
    logo: '/logo.png',
    loading: false,
  }),
}))

vi.mock('@tanstack/react-router', () => ({
  Link: (props: { to: string; children: ReactNode }) => (
    <a href={props.to}>{props.children}</a>
  ),
}))

describe('AuthLayout journal cover', () => {
  it('keeps decoration out of the accessibility tree and renders the form', () => {
    const { container } = render(
      <AuthLayout>
        <form aria-label='Sign in form'>
          <button type='submit'>Sign in</button>
        </form>
      </AuthLayout>
    )

    expect(container.firstElementChild).toHaveAttribute(
      'data-journal-level',
      'immersive'
    )
    const cover = document.querySelector('[data-slot="auth-cover"]')
    expect(cover).toHaveAttribute('aria-hidden', 'true')
    expect(cover).toHaveClass('pointer-events-none')
    const cornerPlants = document.querySelector(
      '[data-slot="auth-corner-plants"]'
    )
    expect(cornerPlants).toHaveAttribute('aria-hidden', 'true')
    expect(cornerPlants).toHaveClass('pointer-events-none')
    expect(
      screen.getByRole('form', { name: 'Sign in form' })
    ).toBeInTheDocument()
    expect(screen.getByRole('link')).toHaveAttribute('href', '/')
  })

  it('adds a decorative handwritten welcome inside the form paper without hiding the form', () => {
    render(
      <AuthLayout>
        <form aria-label='Sign up form' />
      </AuthLayout>
    )

    const welcome = document.querySelector(
      '[data-slot="auth-welcome-note"] [aria-hidden="true"]'
    )
    expect(welcome).toBeInTheDocument()
    expect(welcome).toHaveClass('journal-hand', 'pointer-events-none')
    expect(
      screen.getByRole('form', { name: 'Sign up form' })
    ).toBeInTheDocument()
  })
})
