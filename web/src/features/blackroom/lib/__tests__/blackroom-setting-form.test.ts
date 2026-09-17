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

import type { BlackroomSetting } from '../../types'
import {
  BLACKROOM_SETTING_FORM_DEFAULT_VALUES,
  transformFormValuesToSetting,
  transformSettingToFormDefaults,
} from '../blackroom-form'

const STORED_SETTING: BlackroomSetting = {
  enabled: true,
  auto_ban_enabled: true,
  lookback_hours: 12,
  check_interval_minutes: 5,
  min_requests: 2,
  rules: [{ ip_count: 10, duration_hours: 24, permanent: false }],
  escalation_window_days: 14,
  escalation_temporary_ban_count: 2,
  exempt_user_ids: [7],
  exempt_groups: ['vip'],
  shadow_mode: true,
  realtime_enabled: false,
  geo_enabled: true,
  geo_country_count: 4,
  geo_asn_count: 5,
  geo_min_gap_seconds: 90,
  geo_duration_hours: 48,
  country_mmdb_path: '/data/GeoLite2-Country.mmdb',
  asn_mmdb_path: '/data/GeoLite2-ASN.mmdb',
}

describe('blackroom setting form payload', () => {
  test('submits the automatic-ban and geo fields that were edited', () => {
    const values = transformSettingToFormDefaults(STORED_SETTING)
    const payload = transformFormValuesToSetting({
      ...values,
      shadow_mode: false,
      realtime_enabled: true,
      geo_enabled: false,
      geo_country_count: 6,
      geo_asn_count: 7,
      geo_min_gap_seconds: 60,
      geo_duration_hours: 12,
      country_mmdb_path: ' /data/country.mmdb ',
      asn_mmdb_path: ' /data/asn.mmdb ',
    })

    assert.equal(payload.shadow_mode, false)
    assert.equal(payload.realtime_enabled, true)
    assert.equal(payload.geo_enabled, false)
    assert.equal(payload.geo_country_count, 6)
    assert.equal(payload.geo_asn_count, 7)
    assert.equal(payload.geo_min_gap_seconds, 60)
    assert.equal(payload.geo_duration_hours, 12)
    assert.equal(payload.country_mmdb_path, '/data/country.mmdb')
    assert.equal(payload.asn_mmdb_path, '/data/asn.mmdb')
  })

  test('round-trips stored geo and shadow values through the form', () => {
    const payload = transformFormValuesToSetting(
      transformSettingToFormDefaults(STORED_SETTING)
    )

    assert.equal(payload.shadow_mode, true)
    assert.equal(payload.realtime_enabled, false)
    assert.equal(payload.geo_enabled, true)
    assert.equal(payload.geo_country_count, 4)
    assert.equal(payload.geo_asn_count, 5)
    assert.equal(payload.geo_min_gap_seconds, 90)
    assert.equal(payload.geo_duration_hours, 48)
    assert.equal(payload.country_mmdb_path, '/data/GeoLite2-Country.mmdb')
    assert.equal(payload.asn_mmdb_path, '/data/GeoLite2-ASN.mmdb')
  })

  test('falls back to backend defaults when a stored setting predates the new fields', () => {
    const legacy = { ...STORED_SETTING } as Partial<BlackroomSetting>
    delete legacy.shadow_mode
    delete legacy.geo_country_count

    const payload = transformFormValuesToSetting(
      transformSettingToFormDefaults(legacy as BlackroomSetting)
    )

    assert.equal(payload.shadow_mode, false)
    assert.equal(payload.realtime_enabled, false)
    assert.equal(payload.geo_country_count, 3)
    assert.equal(payload.geo_asn_count, 5)
  })

  test('default form values keep geo blocking off with the backend defaults', () => {
    const payload = transformFormValuesToSetting(
      BLACKROOM_SETTING_FORM_DEFAULT_VALUES
    )

    assert.equal(payload.shadow_mode, false)
    assert.equal(payload.realtime_enabled, true)
    assert.equal(payload.geo_enabled, false)
    assert.equal(payload.geo_country_count, 3)
    assert.equal(payload.geo_asn_count, 3)
    assert.equal(payload.geo_min_gap_seconds, 180)
    assert.equal(payload.geo_duration_hours, 72)
    assert.equal(payload.country_mmdb_path, '')
    assert.equal(payload.asn_mmdb_path, '')
  })
})
