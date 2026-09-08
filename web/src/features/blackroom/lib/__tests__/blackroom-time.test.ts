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
import { assert, describe, test } from 'vitest'

import { formatBlackroomTimeValue } from '../blackroom-time'

describe('blackroom time formatting', () => {
  test('always formats Unix timestamps as Beijing time', () => {
    const timestamp = Date.UTC(2026, 7, 8, 0, 0, 0) / 1000

    assert.equal(formatBlackroomTimeValue(timestamp), '2026-08-08 08:00:00')
    assert.equal(
      formatBlackroomTimeValue(String(timestamp)),
      '2026-08-08 08:00:00'
    )
  })
})
