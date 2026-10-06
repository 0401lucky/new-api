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
import { expect, test } from 'vitest'

import type { ModelHealthHourlyStat } from '../types'
import { summarizeModelHealth } from '../utils'

const shortReplies: ModelHealthHourlyStat = {
  model_name: 'sample',
  hour_start_ts: 3600,
  success_slices: 4,
  total_slices: 5,
  success_rate: 1,
  total_requests: 22,
  error_requests: 0,
  success_requests: 22,
  qualified_success_requests: 19,
  success_tokens: 409007,
}

test('the hourly summary counts valid short replies as successes', () => {
  expect(summarizeModelHealth([shortReplies])).toMatchObject({
    avgRate: 1,
    totalRequests: 22,
    successRequests: 22,
    errorRequests: 0,
  })
})

test('the summary weights hours by final request counts', () => {
  expect(
    summarizeModelHealth([
      shortReplies,
      {
        ...shortReplies,
        hour_start_ts: 7200,
        total_requests: 2,
        success_requests: 0,
        qualified_success_requests: 0,
        error_requests: 2,
        success_rate: 0,
      },
    ])
  ).toMatchObject({ avgRate: 22 / 24, minRate: 0, maxRate: 1 })
})

test('an empty window has zero counts and finite summary values', () => {
  expect(summarizeModelHealth([])).toEqual({
    avgRate: 0,
    totalSuccess: 0,
    totalSlices: 0,
    minRate: 0,
    maxRate: 0,
    totalRequests: 0,
    errorRequests: 0,
    successRequests: 0,
  })
})
