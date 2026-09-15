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
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  StaticDataTable,
  type StaticDataTableColumn,
} from '@/components/data-table'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { formatQuota } from '@/lib/format'
import { useAuthStore } from '@/stores/auth-store'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { donationApi, donationQueryKey } from './api'
import { DonationCampaignForm } from './components/campaign-form'
import { DonationConnectionForm } from './components/connection-form'
import { DonationLayout } from './components/donation-layout'
import { useDonationSession } from './hooks/use-donation-session'
import { donationPermissions } from './lib/access'
import { validationModeLabel } from './lib/labels'
import type { DonationSession, ManagedCampaign } from './types'

export function DonationSettings() {
  const { t } = useTranslation()
  const session = useDonationSession()
  const user = useAuthStore((state) => state.auth.user)
  const permissions = donationPermissions(user)
  return (
    <DonationLayout page='settings'>
      {permissions.configRead && session.sid ? (
        <SettingsWorkspace
          key={session.key}
          session={session}
          canWrite={permissions.configWrite}
        />
      ) : (
        <ErrorState
          title={t('Access Forbidden')}
          description={t('You do not have permission to perform this action.')}
        />
      )}
    </DonationLayout>
  )
}

function SettingsWorkspace(props: {
  session: DonationSession
  canWrite: boolean
}) {
  const { t } = useTranslation()
  useSystemConfigStore((state) => state.config.currency)
  const [editor, setEditor] = useState<ManagedCampaign | 'new' | null>(null)
  const connection = useQuery({
    queryKey: donationQueryKey(props.session, 'connection'),
    queryFn: ({ signal }) => donationApi.connection(props.session, signal),
    retry: false,
    gcTime: 0,
    meta: { errorToast: false },
  })
  const campaigns = useQuery({
    queryKey: donationQueryKey(props.session, 'managed-campaigns'),
    queryFn: ({ signal }) =>
      donationApi.managedCampaigns(props.session, signal),
    retry: false,
    gcTime: 0,
    meta: { errorToast: false },
  })
  const columns: StaticDataTableColumn<ManagedCampaign>[] = [
    {
      id: 'name',
      header: t('Campaign'),
      cell: (row) => (
        <div className='flex flex-col gap-1'>
          <span className='font-medium'>{row.name}</span>
          <span className='text-muted-foreground text-xs'>
            {row.description}
          </span>
          <span className='text-muted-foreground text-xs'>
            {validationModeLabel(row.validation_mode, t)}
          </span>
        </div>
      ),
    },
    {
      id: 'group',
      header: t('Receiving group'),
      cell: (row) => row.group_name,
    },
    {
      id: 'reward',
      header: t('Permanent reward per key'),
      cell: (row) => formatQuota(row.reward_quota),
    },
    {
      id: 'enabled',
      header: t('Status'),
      cell: (row) => (
        <StatusBadge
          label={row.enabled ? t('Enabled') : t('Closed')}
          variant={row.enabled ? 'success' : 'neutral'}
          copyable={false}
        />
      ),
    },
    ...(props.canWrite
      ? [
          {
            id: 'edit',
            header: t('Actions'),
            cell: (row: ManagedCampaign) => (
              <Button variant='ghost' size='sm' onClick={() => setEditor(row)}>
                {t('Edit')}
              </Button>
            ),
          },
        ]
      : []),
  ]
  return (
    <div className='mx-auto flex w-full max-w-5xl flex-col gap-6 pb-4'>
      {connection.isPending && <LoadingState />}
      {connection.isError && (
        <ErrorState
          title={t('Unable to load connection settings')}
          description={t(connection.error.message)}
          onRetry={() => void connection.refetch()}
        />
      )}
      {connection.data && (
        <DonationConnectionForm
          session={props.session}
          connection={connection.data}
          canWrite={props.canWrite}
        />
      )}
      <section
        aria-label={t('Donation campaigns')}
        className='flex flex-col gap-3'
      >
        <div className='flex flex-wrap items-center justify-between gap-2'>
          <h3 className='text-base font-semibold'>{t('Donation campaigns')}</h3>
          {props.canWrite && (
            <Button
              disabled={!connection.data?.configured}
              onClick={() => setEditor('new')}
            >
              {t('Create campaign')}
            </Button>
          )}
        </div>
        {campaigns.isPending && <LoadingState />}
        {campaigns.isError && (
          <ErrorState
            title={t('Unable to load campaigns')}
            description={t(campaigns.error.message)}
            onRetry={() => void campaigns.refetch()}
          />
        )}
        {campaigns.data && (
          <StaticDataTable
            columns={columns}
            data={campaigns.data}
            getRowKey={(row) => row.id}
            emptyContent={
              <EmptyState
                title={t('No donation campaigns')}
                description={t(
                  'Connect gpt-load and create a campaign to start accepting donations.'
                )}
                className='min-h-0 py-6'
              />
            }
          />
        )}
      </section>
      {editor !== null && props.canWrite && (
        <DonationCampaignForm
          key={editor === 'new' ? 'new' : editor.id}
          session={props.session}
          campaign={editor === 'new' ? null : editor}
          onClose={() => setEditor(null)}
        />
      )}
    </div>
  )
}
