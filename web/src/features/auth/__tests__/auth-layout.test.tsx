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
import { readFileSync } from 'node:fs'

import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

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

const presetStyles = readFileSync('src/styles/theme-presets.css', 'utf8')
let style: HTMLStyleElement

beforeEach(() => {
  document.body.removeAttribute('data-theme-preset')
  style = document.createElement('style')
  style.textContent = presetStyles
  document.head.append(style)
})

afterEach(() => {
  style.remove()
  document.body.removeAttribute('data-theme-preset')
})

describe('AuthLayout journal cover', () => {
  it('keeps form fields keyboard accessible and submits through the original form', async () => {
    document.body.dataset.themePreset = 'clover-journal'
    const onSubmit = vi.fn((event: React.FormEvent) => event.preventDefault())
    const user = userEvent.setup()
    render(
      <AuthLayout>
        <form aria-label='Sign in form' onSubmit={onSubmit}>
          <label htmlFor='username'>Username</label>
          <input id='username' autoComplete='username' />
          <label htmlFor='password'>Password</label>
          <input
            id='password'
            type='password'
            autoComplete='current-password'
          />
          <button type='submit'>Sign in</button>
        </form>
      </AuthLayout>
    )

    await user.tab()
    await user.tab()
    expect(screen.getByLabelText('Username')).toHaveFocus()
    await user.keyboard('journal-user{Tab}')
    expect(screen.getByLabelText('Password')).toHaveFocus()
    await user.keyboard('example-password{Tab}{Enter}')
    expect(onSubmit).toHaveBeenCalledOnce()
  })
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
      '[data-slot="auth-welcome-note"] .journal-hand'
    )
    expect(welcome).toBeInTheDocument()
    expect(welcome).toHaveClass('journal-hand', 'pointer-events-none')
    expect(
      screen.getByRole('form', { name: 'Sign up form' })
    ).toBeInTheDocument()
  })

  it('loads the decorative artwork only for the journal preset and keeps the form available when presets change', () => {
    const { container } = render(
      <AuthLayout>
        <form aria-label='Sign in form' />
      </AuthLayout>
    )
    const artwork = container.querySelector<HTMLElement>(
      '[data-slot="auth-illustration"]'
    )
    const marker = container.querySelector<HTMLElement>(
      '[data-slot="auth-paper-marker"]'
    )
    expect(artwork).toBeInTheDocument()
    expect(marker).toBeInTheDocument()
    if (!artwork || !marker) throw new Error('Journal decorations are missing')

    expect(getComputedStyle(artwork).backgroundImage).not.toContain('url(')
    expect(getComputedStyle(marker).display).toBe('none')

    document.body.dataset.themePreset = 'clover-journal'
    expect(getComputedStyle(artwork).backgroundImage).toContain('url(')
    expect(getComputedStyle(artwork).aspectRatio).toBe('1 / 1')
    expect(getComputedStyle(marker).display).not.toBe('none')
    expect(screen.getByRole('form', { name: 'Sign in form' })).toBeVisible()
    expect(screen.getAllByRole('img')).toHaveLength(1)

    document.body.dataset.themePreset = 'lucky-clover'
    expect(getComputedStyle(artwork).backgroundImage).not.toContain('url(')
    expect(getComputedStyle(marker).display).toBe('none')
    expect(screen.getByRole('form', { name: 'Sign in form' })).toBeVisible()
  })
})
