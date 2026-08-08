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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { minutesToTime, timeToMinutes } from '../checkin-time'

describe('minutesToTime', () => {
  test('converts minutes to HH:mm', () => {
    assert.equal(minutesToTime(0), '00:00')
    assert.equal(minutesToTime(480), '08:00')
    assert.equal(minutesToTime(1439), '23:59')
    assert.equal(minutesToTime(600), '10:00')
  })

  test('handles invalid input as 00:00', () => {
    assert.equal(minutesToTime(-1), '00:00')
    assert.equal(minutesToTime(Number.NaN), '00:00')
    assert.equal(minutesToTime(Number.POSITIVE_INFINITY), '00:00')
  })
})

describe('timeToMinutes', () => {
  test('converts valid HH:mm to minutes of day', () => {
    assert.equal(timeToMinutes('00:00'), 0)
    assert.equal(timeToMinutes('08:00'), 480)
    assert.equal(timeToMinutes('23:59'), 1439)
    assert.equal(timeToMinutes('10:30'), 630)
    assert.equal(timeToMinutes(' 09:05 '), 545)
  })

  test('rejects out-of-range and malformed values', () => {
    assert.equal(timeToMinutes('24:00'), -1)
    assert.equal(timeToMinutes('08:60'), -1)
    assert.equal(timeToMinutes('8'), -1)
    assert.equal(timeToMinutes(''), -1)
    assert.equal(timeToMinutes('abc'), -1)
  })

  test('round-trips between representations', () => {
    for (const minutes of [0, 1, 59, 60, 480, 1439]) {
      assert.equal(timeToMinutes(minutesToTime(minutes)), minutes)
    }
  })
})
