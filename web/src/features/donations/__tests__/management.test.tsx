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
import { cleanup, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AxiosError, type InternalAxiosRequestConfig } from 'axios'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'
import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import { DonationCampaignForm } from '../components/campaign-form'
import { donationCampaignSchema } from '../lib/schema'
import { DonationRecords } from '../records'
import { DonationSettings } from '../settings'
import type { DonationRecord } from '../types'
import {
  admin,
  aliceSession,
  batchFixture,
  connection,
  group,
  initializeDonationSession,
  managedCampaign,
  renderDonation,
  response,
} from './fixtures'

const originalAdapter = api.defaults.adapter

it('rejects a campaign description beyond the backend UTF-8 byte boundary', () => {
  const result = donationCampaignSchema().safeParse({
    name: 'Campaign',
    description: '界'.repeat(1334),
    group_id: 7,
    reward_amount: '1',
    enabled: true,
    manual_review: false,
  })
  expect(result.success).toBe(false)
})
beforeEach(() => {
  localStorage.clear()
  initializeDonationSession(admin)
})
afterEach(() => {
  cleanup()
  api.defaults.adapter = originalAdapter
  useAuthStore.getState().auth.reset()
  useSystemConfigStore
    .getState()
    .setConfig({ currency: DEFAULT_CURRENCY_CONFIG })
  localStorage.clear()
  vi.restoreAllMocks()
})

it('keeps management pages inaccessible to a normal account without loading admin data', async () => {
  initializeDonationSession()
  const send = vi.fn(async (config: InternalAxiosRequestConfig) =>
    response(config, {})
  )
  api.defaults.adapter = send
  const view = await renderDonation(<DonationSettings />)
  await waitFor(() =>
    expect(screen.getByText('Access Forbidden')).toBeVisible()
  )
  expect(
    screen.queryByLabelText('Integration credential')
  ).not.toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Create campaign' })
  ).not.toBeInTheDocument()
  expect(send).not.toHaveBeenCalled()
  view.unmount()
  view.client.clear()
  const records = await renderDonation(<DonationRecords />)
  await waitFor(() =>
    expect(screen.getByText('Access Forbidden')).toBeVisible()
  )
  expect(send).not.toHaveBeenCalled()
  records.unmount()
  records.client.clear()
})

it('clears a saved connection credential and excludes it from the mutation cache while keeping the verified URL', async () => {
  const writes: InternalAxiosRequestConfig[] = []
  api.defaults.adapter = async (config) => {
    if (config.url === '/api/donations/admin/campaigns') {
      return response(config, [])
    }
    if (config.method === 'put') {
      writes.push(config)
      return response(config, { ...connection, version: 2 })
    }
    return response(config, connection)
  }
  const view = await renderDonation(<DonationSettings />, true)
  const user = userEvent.setup()
  const url = await screen.findByLabelText('Service URL')
  expect(url).toHaveValue(connection.base_url)
  const secret = 'synthetic-connection-private-token-00001'
  await user.type(screen.getByLabelText('Integration credential'), secret)
  await user.click(screen.getByRole('button', { name: 'Save connection' }))
  await waitFor(() =>
    expect(screen.getByLabelText('Integration credential')).toHaveValue('')
  )
  expect(screen.getByRole('status')).toHaveTextContent(
    'Connection saved and verified.'
  )
  expect(screen.getByLabelText('Service URL')).toHaveValue(connection.base_url)
  expect(writes).toHaveLength(1)
  expect(
    JSON.stringify(
      view.client
        .getMutationCache()
        .getAll()
        .map((entry) => entry.state)
    )
  ).not.toContain(secret)
  expect(JSON.stringify(localStorage)).not.toContain(secret)
  view.unmount()
  view.client.clear()
})

it('preserves the selected group on refresh failure and permits closing a campaign whose group disappeared', async () => {
  let mode: 'ok' | 'error' | 'empty' = 'ok'
  api.defaults.adapter = async (config) => {
    if (mode === 'error') {
      throw new AxiosError(
        'group unavailable',
        'ERR_BAD_RESPONSE',
        config,
        undefined,
        { ...response(config, null), status: 503 }
      )
    }
    return response(config, mode === 'empty' ? [] : [group])
  }
  const view = await renderDonation(
    <DonationCampaignForm
      session={aliceSession}
      campaign={managedCampaign}
      onClose={() => {}}
    />
  )
  const user = userEvent.setup()
  const selector = await screen.findByRole('combobox', {
    name: 'Receiving group',
  })
  await waitFor(() => expect(selector).toBeEnabled())
  expect(selector).toHaveValue(group.name)
  mode = 'error'
  await user.click(screen.getByRole('button', { name: 'Refresh groups' }))
  await screen.findByText('Unable to load groups')
  expect(selector).toHaveValue(group.name)
  expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled()
  mode = 'empty'
  await user.click(screen.getByRole('button', { name: 'Refresh groups' }))
  await screen.findByText('No receiving groups')
  expect(selector).toHaveValue(group.name)
  expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled()
  await user.click(screen.getByRole('switch', { name: 'Accept donations' }))
  expect(screen.getByRole('button', { name: 'Save' })).toBeEnabled()
  view.unmount()
  view.client.clear()
})

it('preserves the original exact integer reward when an existing campaign is renamed', async () => {
  useSystemConfigStore.getState().setConfig({
    currency: { ...DEFAULT_CURRENCY_CONFIG, quotaPerUnit: 500000 },
  })
  const existing = { ...managedCampaign, reward_quota: Number.MAX_SAFE_INTEGER }
  let write: InternalAxiosRequestConfig | undefined
  const closed = vi.fn()
  api.defaults.adapter = async (config) => {
    if (config.method === 'patch') {
      write = config
      return response(config, existing)
    }
    return response(config, [group])
  }
  const view = await renderDonation(
    <DonationCampaignForm
      session={aliceSession}
      campaign={existing}
      onClose={closed}
    />
  )
  const user = userEvent.setup()
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Save' })).toBeEnabled()
  )
  await user.clear(screen.getByLabelText('Campaign name'))
  await user.type(
    screen.getByLabelText('Campaign name'),
    'A different display name'
  )
  await user.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(closed).toHaveBeenCalledOnce())
  const payload = JSON.parse(String(write?.data))
  expect(payload).toMatchObject({
    name: 'A different display name',
    group_id: existing.group_id,
    reward_quota: Number.MAX_SAFE_INTEGER,
  })
  expect(payload).not.toHaveProperty('platform')
  expect(payload).not.toHaveProperty('expires_at')
  view.unmount()
  view.client.clear()
})

it('persists manual review mode and states when the reward is credited', async () => {
  const manual = {
    ...managedCampaign,
    validation_mode: 'manual_review' as const,
  }
  let write: InternalAxiosRequestConfig | undefined
  const closed = vi.fn()
  api.defaults.adapter = async (config) => {
    if (config.method === 'patch') {
      write = config
      return response(config, manual)
    }
    return response(config, [
      { ...group, can_probe: false, unavailable_reason: 'probe_unavailable' },
    ])
  }
  const view = await renderDonation(
    <DonationCampaignForm
      session={aliceSession}
      campaign={manual}
      onClose={closed}
    />
  )
  const mode = screen.getByRole('switch', {
    name: 'Skip model testing and review manually',
  })
  expect(mode).toBeChecked()
  expect(
    screen.getByText(
      'Keys stay in temporary storage until an administrator reviews them. The reward is credited only after the review passes and the key is received.'
    )
  ).toBeVisible()
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Save' })).toBeEnabled()
  )
  await userEvent.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(closed).toHaveBeenCalledOnce())
  expect(JSON.parse(String(write?.data))).toMatchObject({
    validation_mode: 'manual_review',
    group_id: manual.group_id,
    reward_quota: manual.reward_quota,
  })
  view.unmount()
  view.client.clear()
})

it('does not use the offline close exception to change an existing manual campaign to automatic mode', async () => {
  let offline = false
  api.defaults.adapter = async (config) => {
    if (offline) {
      throw new AxiosError('offline', 'ERR_NETWORK', config)
    }
    return response(config, [group])
  }
  const view = await renderDonation(
    <DonationCampaignForm
      session={aliceSession}
      campaign={{ ...managedCampaign, validation_mode: 'manual_review' }}
      onClose={() => {}}
    />
  )
  const user = userEvent.setup()
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Save' })).toBeEnabled()
  )
  offline = true
  await user.click(screen.getByRole('button', { name: 'Refresh groups' }))
  await screen.findByText('Unable to load groups')
  await user.click(screen.getByRole('switch', { name: 'Accept donations' }))
  expect(screen.getByRole('button', { name: 'Save' })).toBeEnabled()
  await user.click(
    screen.getByRole('switch', {
      name: 'Skip model testing and review manually',
    })
  )
  expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled()
  view.unmount()
  view.client.clear()
})

it('blocks manual review when the connected gpt-load does not advertise the capability', async () => {
  api.defaults.adapter = async (config) =>
    response(config, [
      {
        ...group,
        can_manual_review: undefined,
        manual_target_revision: undefined,
        manual_unavailable_reason: undefined,
      },
    ])
  const view = await renderDonation(
    <DonationCampaignForm
      session={aliceSession}
      campaign={null}
      onClose={() => {}}
    />
  )
  const mode = await screen.findByRole('switch', {
    name: 'Skip model testing and review manually',
  })
  expect(mode).toHaveAttribute('aria-disabled', 'true')
  expect(
    await screen.findByText(
      'Manual review is unavailable: the connected gpt-load does not support it, or no group qualifies for it.'
    )
  ).toBeVisible()
  view.unmount()
  view.client.clear()
})

it('refuses to save an existing manual campaign against a receiver without the capability', async () => {
  const manual = {
    ...managedCampaign,
    validation_mode: 'manual_review' as const,
  }
  const writes: InternalAxiosRequestConfig[] = []
  api.defaults.adapter = async (config) => {
    if (config.method === 'patch') {
      writes.push(config)
      return response(config, manual)
    }
    return response(config, [
      {
        ...group,
        can_manual_review: undefined,
        manual_target_revision: undefined,
        manual_unavailable_reason: undefined,
      },
    ])
  }
  const view = await renderDonation(
    <DonationCampaignForm
      session={aliceSession}
      campaign={manual}
      onClose={() => {}}
    />
  )
  expect(
    screen.getByRole('switch', {
      name: 'Skip model testing and review manually',
    })
  ).toBeChecked()
  expect(
    await screen.findByText(
      'This campaign uses manual review, but no group currently qualifies for it. Upgrade gpt-load or choose another mode before saving.'
    )
  ).toBeVisible()
  expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled()
  expect(writes).toHaveLength(0)
  view.unmount()
  view.client.clear()
})

it('applies record filters and shows the original owner, credential and permanent reward in details', async () => {
  const batch = batchFixture()
  const item = batch.items[0]
  if (!item) throw new Error('Expected fixture item')
  const record: DonationRecord = {
    item,
    batch,
    reward: {
      id: 'reward-record',
      item_id: item.id,
      user_id: batch.user_id,
      quota: 25,
      credited_at_ms: item.rewarded_at_ms ?? 0,
    },
    events: [
      {
        id: 1,
        item_id: item.id,
        state: 'rewarded',
        reason_code: '',
        created_at_ms: item.rewarded_at_ms ?? 0,
      },
    ],
  }
  const filters: InternalAxiosRequestConfig[] = []
  api.defaults.adapter = async (config) => {
    if (config.url === '/api/donations/admin/records') {
      filters.push(config)
      return response(config, {
        page: 1,
        page_size: 20,
        total: 1,
        items: [record],
      })
    }
    return response(config, record)
  }
  const view = await renderDonation(<DonationRecords />)
  const user = userEvent.setup()
  await user.type(await screen.findByLabelText('User ID'), '1')
  await user.click(screen.getByRole('button', { name: 'Search' }))
  await waitFor(() => expect(filters.at(-1)?.params.user_id).toBe(1))
  await user.click(await screen.findByRole('button', { name: 'Details' }))
  const dialog = await screen.findByRole('dialog', { name: 'Donation details' })
  await waitFor(() =>
    expect(within(dialog).getByText('alice · 1')).toBeVisible()
  )
  expect(within(dialog).getByText('100')).toBeVisible()
  expect(within(dialog).getByText('Permanent quota: $0.25')).toBeVisible()
  expect(within(dialog).getByText('reward-record')).toBeVisible()
  expect(
    within(dialog).getByRole('region', { name: 'Processing history' })
  ).toBeVisible()
  view.unmount()
  view.client.clear()
})
