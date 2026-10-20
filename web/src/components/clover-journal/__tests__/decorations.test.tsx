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

import { cleanup, render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'

import {
  CloverBotanicalCanvas,
  CloverDoodle,
  HandNote,
  JournalPage,
  JournalSection,
  PaperClip,
  PaperGrain,
  Polaroid,
  StickyNote,
  WashiTape,
} from '../index'

const presetStyles = readFileSync('src/styles/theme-presets.css', 'utf8')
let style: HTMLStyleElement

beforeEach(() => {
  document.body.removeAttribute('data-theme-preset')
  style = document.createElement('style')
  style.textContent = presetStyles
  document.head.append(style)
})

afterEach(() => {
  cleanup()
  style.remove()
  document.body.removeAttribute('data-theme-preset')
})

describe('clover journal decorations', () => {
  it('hides decorative primitives from assistive technology and pointer events', () => {
    const { container } = render(
      <>
        <CloverDoodle />
        <WashiTape />
        <PaperClip />
        <StickyNote>note</StickyNote>
        <HandNote>hand</HandNote>
        <PaperGrain />
      </>
    )

    const decorations = container.querySelectorAll('[aria-hidden="true"]')
    expect(decorations).toHaveLength(6)
    decorations.forEach((node) => {
      expect(node).toHaveClass('pointer-events-none')
    })
  })

  it('keeps the caller className and rotation on the tape', () => {
    const { container } = render(
      <WashiTape className='custom-tape' rotation={3} />
    )
    const tape = container.firstElementChild as HTMLElement

    expect(tape).toHaveClass('custom-tape')
    expect(tape.style.transform).toBe('rotate(3deg)')
  })

  it('honors explicit tape dimensions so a narrow strip does not cover content', () => {
    const { container } = render(<WashiTape width='5rem' height='0.75rem' />)
    const tape = container.firstElementChild as HTMLElement

    expect(tape.style.width).toBe('5rem')
    expect(tape.style.height).toBe('0.75rem')
  })

  it.each([undefined, 'default', 'lucky-clover'])(
    'hides botanical margins and the photo when the active preset is %s',
    (preset) => {
      if (preset) document.body.dataset.themePreset = preset
      const { container } = render(
        <>
          <CloverBotanicalCanvas />
          <Polaroid />
        </>
      )

      const decorations = container.querySelectorAll(
        '.journal-botanical-canvas, .journal-polaroid'
      )
      expect(decorations).toHaveLength(2)
      for (const decoration of decorations) {
        expect(getComputedStyle(decoration).display).toBe('none')
      }
    }
  )

  it('hides botanical margins inside calm data pages even with the journal preset', () => {
    document.body.dataset.themePreset = 'clover-journal'
    const { container } = render(
      <JournalPage level='calm'>
        <CloverBotanicalCanvas />
      </JournalPage>
    )

    const canvas = container.querySelector('.journal-botanical-canvas')
    expect(canvas).not.toBeNull()
    expect(getComputedStyle(canvas as Element).display).toBe('none')
  })

  it('restores the botanical cover when switching back to the journal preset', () => {
    document.body.dataset.themePreset = 'clover-journal'
    const { container } = render(<Polaroid caption='A little luck' />)
    const photo = container.querySelector('.journal-polaroid') as Element

    expect(getComputedStyle(photo).display).not.toBe('none')
    document.body.dataset.themePreset = 'lucky-clover'
    expect(getComputedStyle(photo).display).toBe('none')
    document.body.dataset.themePreset = 'clover-journal'
    expect(getComputedStyle(photo).display).not.toBe('none')
    expect(photo).toHaveAttribute('aria-hidden', 'true')
    expect(photo).toHaveClass('pointer-events-none')
  })

  it('renders note text inside a hidden note', () => {
    const { container } = render(<StickyNote>记录每一次调用</StickyNote>)
    const note = container.firstElementChild as HTMLElement

    expect(note).toHaveTextContent('记录每一次调用')
    expect(note).toHaveAttribute('aria-hidden', 'true')
  })

  it('marks page containers with the requested decoration level', () => {
    const { container } = render(
      <>
        <JournalPage level='immersive' data-testid='page' />
        <JournalSection level='medium' data-testid='section' />
      </>
    )

    expect(container.querySelector('[data-testid="page"]')).toHaveAttribute(
      'data-journal-level',
      'immersive'
    )
    expect(container.querySelector('[data-testid="section"]')).toHaveAttribute(
      'data-journal-level',
      'medium'
    )
  })
})
