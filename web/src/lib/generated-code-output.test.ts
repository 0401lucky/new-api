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
import { afterEach, describe, expect, test, vi } from 'vitest'

import { outputGeneratedCodes } from './generated-code-output'

const { copyToClipboard } = vi.hoisted(() => ({
  copyToClipboard: vi.fn(),
}))

vi.mock('@/lib/copy-to-clipboard', () => ({ copyToClipboard }))

afterEach(() => {
  vi.restoreAllMocks()
})

describe('outputGeneratedCodes', () => {
  test('copies all generated codes as newline-delimited text', async () => {
    copyToClipboard.mockResolvedValue(true)

    const success = await outputGeneratedCodes(
      ['first-code', 'second-code'],
      'unused.txt',
      'copy'
    )

    expect(success).toBe(true)
    expect(copyToClipboard).toHaveBeenCalledWith('first-code\nsecond-code')
  })

  test('downloads all generated codes as a text file', async () => {
    const createObjectURL = vi.fn(() => 'blob:generated-codes')
    const revokeObjectURL = vi.fn()
    Object.defineProperty(URL, 'createObjectURL', {
      configurable: true,
      value: createObjectURL,
    })
    Object.defineProperty(URL, 'revokeObjectURL', {
      configurable: true,
      value: revokeObjectURL,
    })
    const click = vi
      .spyOn(HTMLAnchorElement.prototype, 'click')
      .mockImplementation(() => undefined)

    const success = await outputGeneratedCodes(
      ['first-code', 'second-code'],
      'generated.txt',
      'download'
    )

    expect(success).toBe(true)
    expect(createObjectURL).toHaveBeenCalledOnce()
    expect(click).toHaveBeenCalledOnce()
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:generated-codes')
  })
})
