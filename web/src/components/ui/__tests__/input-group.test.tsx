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
import { compile } from 'tailwindcss'
import { afterEach, expect, test } from 'vitest'

import {
  InputGroup,
  InputGroupButton,
  InputGroupTextarea,
} from '../input-group'

let style: HTMLStyleElement | undefined

afterEach(() => style?.remove())

test.each([false, true])(
  'a disabled send button dims the group only when its text control is disabled=%s',
  async (disabled) => {
    render(
      <InputGroup>
        <InputGroupTextarea aria-label='Message' disabled={disabled} />
        <InputGroupButton disabled>Send</InputGroupButton>
      </InputGroup>
    )
    const group = screen.getByRole('group')
    const compiler = await compile('@tailwind utilities;')
    style = document.createElement('style')
    style.textContent = compiler.build(Array.from(group.classList))
    document.head.append(style)

    const rawOpacity = getComputedStyle(group).opacity
    const opacity =
      Number.parseFloat(rawOpacity) / (rawOpacity.endsWith('%') ? 100 : 1)
    expect(opacity).toBe(disabled ? 0.5 : 1)
    expect(screen.getByRole('button', { name: 'Send' })).toBeDisabled()
    expect(
      screen.getByRole('textbox', { name: 'Message' }).hasAttribute('disabled')
    ).toBe(disabled)
  }
)
