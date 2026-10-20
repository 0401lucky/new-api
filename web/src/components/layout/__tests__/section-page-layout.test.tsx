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
import { afterEach, beforeEach, describe, expect, it } from 'vitest'

import { SectionPageLayout } from '../components/section-page-layout'

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

describe('SectionPageLayout journal level', () => {
  it('keeps the heading and keyboard actions accessible when immersive decorations are enabled', async () => {
    document.body.dataset.themePreset = 'clover-journal'
    const user = userEvent.setup()
    render(
      <SectionPageLayout journalLevel='immersive'>
        <SectionPageLayout.Title>Models</SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <button type='button'>Refresh</button>
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>model table</SectionPageLayout.Content>
      </SectionPageLayout>
    )

    expect(screen.getByRole('main')).toHaveAttribute(
      'data-journal-level',
      'immersive'
    )
    expect(screen.getByRole('heading', { name: 'Models' })).toBeVisible()
    const decoration = screen
      .getByRole('main')
      .querySelector('[data-slot="section-page-decoration"]')
    expect(decoration).toHaveAttribute('aria-hidden', 'true')
    expect(decoration).toHaveClass('pointer-events-none')
    if (!decoration) throw new Error('Heading decoration is missing')
    expect(getComputedStyle(decoration).display).not.toBe('none')
    await user.tab()
    expect(screen.getByRole('button', { name: 'Refresh' })).toHaveFocus()

    document.body.dataset.themePreset = 'lucky-clover'
    expect(getComputedStyle(decoration).display).toBe('none')
    expect(screen.getByRole('heading', { name: 'Models' })).toBeVisible()
  })

  it('leaves unmarked pages to the calm default', () => {
    render(
      <SectionPageLayout>
        <SectionPageLayout.Content>admin table</SectionPageLayout.Content>
      </SectionPageLayout>
    )

    expect(screen.getByRole('main')).not.toHaveAttribute('data-journal-level')
    expect(
      screen
        .getByRole('main')
        .querySelector('[data-slot="section-page-decoration"]')
    ).toBeNull()
  })

  it('marks the action group so wide header actions can wrap', () => {
    render(
      <SectionPageLayout>
        <SectionPageLayout.Title>Channels</SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <button type='button'>Create</button>
        </SectionPageLayout.Actions>
      </SectionPageLayout>
    )

    expect(
      screen
        .getByRole('button', { name: 'Create' })
        .closest("[data-slot='section-page-actions']")
    ).not.toBeNull()
  })
})
