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

import { UserActivityPanel } from './components/user-activity-panel'
import { UsersDeleteDialog } from './components/users-delete-dialog'
import { UsersMutateDrawer } from './components/users-mutate-drawer'
import { UsersPrimaryButtons } from './components/users-primary-buttons'
import { UsersProvider, useUsers } from './components/users-provider'
import { UsersTable } from './components/users-table'

const route = getRouteApi('/_authenticated/users/')

function UsersContent() {
  const { t } = useTranslation()
  const { open, setOpen, currentRow } = useUsers()
  const tab = route.useSearch().tab ?? 'users'
  const navigate = route.useNavigate()

  return (
    <>
      <SectionPageLayout fixedContent>
        <SectionPageLayout.Title>{t('Users')}</SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          {tab === 'users' ? <UsersPrimaryButtons /> : null}
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <Tabs
            value={tab}
            onValueChange={(value) =>
              void navigate({
                search: (previous) => ({
                  ...previous,
                  tab: value === 'statistics' ? 'statistics' : 'users',
                  page: 1,
                  status: previous.status?.filter((status) => status !== '-1'),
                }),
              })
            }
            className='flex h-full min-h-0 flex-col gap-3'
          >
            <TabsList className='w-fit shrink-0'>
              <TabsTrigger value='users'>{t('Users')}</TabsTrigger>
              <TabsTrigger value='statistics'>
                {t('User statistics')}
              </TabsTrigger>
            </TabsList>
            <TabsContent
              value='users'
              className='mt-0 min-h-0 flex-1 outline-none'
            >
              <UsersTable />
            </TabsContent>
            <TabsContent
              value='statistics'
              className='mt-0 min-h-0 flex-1 outline-none'
            >
              <UserActivityPanel />
            </TabsContent>
          </Tabs>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <UsersMutateDrawer
        open={open === 'create' || open === 'update'}
        onOpenChange={(isOpen) => !isOpen && setOpen(null)}
        currentRow={open === 'update' ? currentRow || undefined : undefined}
      />
      <UsersDeleteDialog />
    </>
  )
}

export function Users() {
  return (
    <UsersProvider>
      <UsersContent />
    </UsersProvider>
  )
}
