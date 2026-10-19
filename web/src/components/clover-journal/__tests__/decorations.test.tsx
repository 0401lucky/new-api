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
import { cleanup, render } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'

import {
  CloverDoodle,
  HandNote,
  JournalPage,
  JournalSection,
  PaperClip,
  PaperGrain,
  StickyNote,
  WashiTape,
} from '../index'

afterEach(cleanup)

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
