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
import { Link } from '@tanstack/react-router'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { useAuthStore } from '@/stores/auth-store'

import { donationPermissions } from '../lib/access'

export function DonationLayout(props: {
  page: 'donate' | 'settings' | 'records'
  children: ReactNode
}) {
  const { t } = useTranslation()
  const user = useAuthStore((state) => state.auth.user)
  const permissions = donationPermissions(user)
  return (
    <SectionPageLayout stackActionsOnMobile>
      <SectionPageLayout.Title>{t('Donations')}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <nav
          aria-label={t('Donation navigation')}
          className='flex flex-wrap items-center gap-1'
        >
          <Button
            variant={props.page === 'donate' ? 'secondary' : 'ghost'}
            size='sm'
            nativeButton={false}
            render={<Link to='/donations' />}
            aria-current={props.page === 'donate' ? 'page' : undefined}
          >
            {t('My donations')}
          </Button>
          {permissions.configRead && (
            <Button
              variant={props.page === 'settings' ? 'secondary' : 'ghost'}
              size='sm'
              nativeButton={false}
              render={<Link to='/donations/settings' />}
              aria-current={props.page === 'settings' ? 'page' : undefined}
            >
              {t('Donation settings')}
            </Button>
          )}
          {permissions.recordsRead && (
            <Button
              variant={props.page === 'records' ? 'secondary' : 'ghost'}
              size='sm'
              nativeButton={false}
              render={<Link to='/donations/records' />}
              aria-current={props.page === 'records' ? 'page' : undefined}
            >
              {t('Donation records')}
            </Button>
          )}
        </nav>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>{props.children}</SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
