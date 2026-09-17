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
import { getRouteApi } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

import { BlackroomDialogs } from './components/blackroom-dialogs'
import { BlackroomIPAuditTable } from './components/blackroom-ip-audit-table'
import { BlackroomPrimaryButtons } from './components/blackroom-primary-buttons'
import { BlackroomProvider } from './components/blackroom-provider'
import { BlackroomStatusBanner } from './components/blackroom-status-banner'
import { BlackroomTable } from './components/blackroom-table'
import { BLACKROOM_DEFAULT_TAB, BLACKROOM_TAB_IP_AUDIT } from './constants'

const route = getRouteApi('/_authenticated/blackroom/')

export function Blackroom() {
  const { t } = useTranslation()
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const tab = search.tab ?? BLACKROOM_DEFAULT_TAB

  const handleTabChange = (value: string) => {
    navigate({
      search: (previous) => ({
        ...previous,
        tab:
          value === BLACKROOM_TAB_IP_AUDIT ? BLACKROOM_TAB_IP_AUDIT : undefined,
      }),
    })
  }

  // IP 审计侧点用户后回到封禁记录，复用列表既有的关键词筛选（数字会被
  // 后端当作 user_id 精确匹配）。
  const showUserBans = (userId: number) => {
    navigate({
      search: (previous) => ({
        ...previous,
        tab: undefined,
        filter: String(userId),
        page: undefined,
      }),
    })
  }

  return (
    <BlackroomProvider>
      <SectionPageLayout>
        <SectionPageLayout.Title>{t('Blackroom')}</SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <BlackroomPrimaryButtons />
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <Tabs
            value={tab}
            onValueChange={handleTabChange}
            className='flex h-full min-h-0 flex-col gap-3'
          >
            <TabsList className='max-w-full flex-wrap justify-start group-data-horizontal/tabs:h-auto'>
              <TabsTrigger value={BLACKROOM_DEFAULT_TAB}>
                {t('Ban records')}
              </TabsTrigger>
              <TabsTrigger value={BLACKROOM_TAB_IP_AUDIT}>
                {t('IP audit')}
              </TabsTrigger>
            </TabsList>
            <TabsContent
              value={BLACKROOM_DEFAULT_TAB}
              className='flex min-h-0 flex-1 flex-col gap-3'
            >
              <BlackroomStatusBanner />
              <div className='min-h-0 flex-1'>
                <BlackroomTable />
              </div>
            </TabsContent>
            <TabsContent
              value={BLACKROOM_TAB_IP_AUDIT}
              className='min-h-0 flex-1'
            >
              <BlackroomIPAuditTable
                search={search}
                navigate={navigate}
                onSelectUser={showUserBans}
              />
            </TabsContent>
          </Tabs>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <BlackroomDialogs />
    </BlackroomProvider>
  )
}
