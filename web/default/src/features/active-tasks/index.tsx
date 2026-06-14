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
import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { BarChart3, Eye, RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Empty, EmptyHeader, EmptyTitle } from '@/components/ui/empty'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { sideDrawerContentClassName } from '@/components/drawer-layout'
import { SectionPageLayout } from '@/components/layout'
import {
  getActiveTaskRank,
  getActiveTaskStats,
  getHighActiveTaskHistory,
  getUserTokenUsage24h,
} from './api'
import type { ActiveTaskRankItem } from './types'

function formatTimestamp(value?: number) {
  if (!value) return '-'
  return new Date(value * 1000).toLocaleString()
}

function formatNumber(value?: number) {
  return (value || 0).toLocaleString()
}

function StatCard(props: { title: string; value?: number; suffix?: string }) {
  return (
    <Card>
      <CardHeader className='pb-2'>
        <CardDescription>{props.title}</CardDescription>
        <CardTitle className='text-2xl'>
          {formatNumber(props.value)}
          {props.suffix ? (
            <span className='text-muted-foreground ml-1 text-sm font-normal'>
              {props.suffix}
            </span>
          ) : null}
        </CardTitle>
      </CardHeader>
    </Card>
  )
}

function TokenUsageSheet(props: {
  user: ActiveTaskRankItem | null
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const query = useQuery({
    queryKey: ['active-task-token-usage', props.user?.user_id],
    queryFn: () => getUserTokenUsage24h(props.user?.user_id || 0),
    enabled: props.open && Boolean(props.user?.user_id),
  })

  const usage = query.data?.data
  const rows = usage?.models || []

  return (
    <Sheet open={props.open} onOpenChange={props.onOpenChange}>
      <SheetContent className={sideDrawerContentClassName('sm:max-w-3xl')}>
        <SheetHeader className='border-b'>
          <SheetTitle>{t('Token usage')}</SheetTitle>
          <SheetDescription>
            {props.user
              ? `${props.user.username} · #${props.user.user_id}`
              : t('Loading')}
          </SheetDescription>
        </SheetHeader>
        <div className='flex flex-col gap-4 overflow-auto px-4 py-4'>
          <div className='grid gap-3 sm:grid-cols-2'>
            <StatCard title={t('Total tokens')} value={usage?.total_tokens} />
            <StatCard
              title={t('Total requests')}
              value={usage?.total_requests}
            />
          </div>
          <Card>
            <CardHeader className='border-b'>
              <CardTitle>{t('Models')}</CardTitle>
            </CardHeader>
            <CardContent className='p-0'>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('Model')}</TableHead>
                    <TableHead>{t('Tokens')}</TableHead>
                    <TableHead>{t('Requests')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {query.isFetching ? (
                    <TableRow>
                      <TableCell colSpan={3} className='h-24 text-center'>
                        {t('Loading')}
                      </TableCell>
                    </TableRow>
                  ) : rows.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={3}>
                        <Empty className='min-h-24 border-0'>
                          <EmptyHeader>
                            <EmptyTitle>{t('No data')}</EmptyTitle>
                          </EmptyHeader>
                        </Empty>
                      </TableCell>
                    </TableRow>
                  ) : (
                    rows.map((row) => (
                      <TableRow key={row.model_name}>
                        <TableCell className='max-w-72 truncate'>
                          {row.model_name || '-'}
                        </TableCell>
                        <TableCell>{formatNumber(row.total_tokens)}</TableCell>
                        <TableCell>
                          {formatNumber(row.request_count)}
                        </TableCell>
                      </TableRow>
                    ))
                  )}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
        </div>
      </SheetContent>
    </Sheet>
  )
}

export function ActiveTasks() {
  const { t } = useTranslation()
  const [selectedUser, setSelectedUser] = useState<ActiveTaskRankItem | null>(
    null
  )
  const [usageOpen, setUsageOpen] = useState(false)

  const statsQuery = useQuery({
    queryKey: ['active-task-stats'],
    queryFn: getActiveTaskStats,
    refetchInterval: 10000,
  })
  const rankQuery = useQuery({
    queryKey: ['active-task-rank'],
    queryFn: () => getActiveTaskRank({ window: 30, limit: 50 }),
    refetchInterval: 10000,
  })
  const historyQuery = useQuery({
    queryKey: ['active-task-history'],
    queryFn: () => getHighActiveTaskHistory({ limit: 100 }),
    refetchInterval: 60000,
  })

  const stats = statsQuery.data?.data
  const rank = rankQuery.data?.data?.rank || []
  const history = historyQuery.data?.data?.records || []
  const isFetching =
    statsQuery.isFetching || rankQuery.isFetching || historyQuery.isFetching

  const openUsage = (user: ActiveTaskRankItem) => {
    setSelectedUser(user)
    setUsageOpen(true)
  }

  const refresh = () => {
    void statsQuery.refetch()
    void rankQuery.refetch()
    void historyQuery.refetch()
  }

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>{t('Active tasks')}</SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <Button variant='outline' disabled={isFetching} onClick={refresh}>
            <RefreshCw data-icon='inline-start' />
            {t('Refresh')}
          </Button>
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <div className='flex flex-col gap-4'>
            <div className='grid gap-3 md:grid-cols-4'>
              <StatCard title={t('Active slots')} value={stats?.active_slots} />
              <StatCard title={t('Total slots')} value={stats?.total_slots} />
              <StatCard title={t('Active users')} value={stats?.active_users} />
              <StatCard
                title={t('Window')}
                value={stats?.window_seconds}
                suffix='s'
              />
            </div>

            <Tabs defaultValue='rank' className='flex flex-col gap-4'>
              <TabsList>
                <TabsTrigger value='rank'>{t('Current rank')}</TabsTrigger>
                <TabsTrigger value='history'>{t('History')}</TabsTrigger>
              </TabsList>

              <TabsContent value='rank'>
                <Card>
                  <CardHeader className='border-b'>
                    <CardTitle>{t('Current rank')}</CardTitle>
                    <CardDescription>
                      {t('{{count}} records', { count: rank.length })}
                    </CardDescription>
                  </CardHeader>
                  <CardContent className='p-0'>
                    <Table>
                      <TableHeader>
                        <TableRow>
                          <TableHead>{t('User')}</TableHead>
                          <TableHead>{t('Active slots')}</TableHead>
                          <TableHead className='text-right'>
                            {t('Action')}
                          </TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {rankQuery.isLoading ? (
                          <TableRow>
                            <TableCell colSpan={3} className='h-32 text-center'>
                              {t('Loading')}
                            </TableCell>
                          </TableRow>
                        ) : rank.length === 0 ? (
                          <TableRow>
                            <TableCell colSpan={3}>
                              <Empty className='min-h-32 border-0'>
                                <EmptyHeader>
                                  <EmptyTitle>{t('No data')}</EmptyTitle>
                                </EmptyHeader>
                              </Empty>
                            </TableCell>
                          </TableRow>
                        ) : (
                          rank.map((row) => (
                            <TableRow key={row.user_id}>
                              <TableCell>
                                {row.username || `#${row.user_id}`}
                              </TableCell>
                              <TableCell>
                                <Badge variant='secondary'>
                                  {row.active_slots}
                                </Badge>
                              </TableCell>
                              <TableCell className='text-right'>
                                <Button
                                  variant='ghost'
                                  size='sm'
                                  onClick={() => openUsage(row)}
                                >
                                  <BarChart3 data-icon='inline-start' />
                                  {t('Usage')}
                                </Button>
                              </TableCell>
                            </TableRow>
                          ))
                        )}
                      </TableBody>
                    </Table>
                  </CardContent>
                </Card>
              </TabsContent>

              <TabsContent value='history'>
                <Card>
                  <CardHeader className='border-b'>
                    <CardTitle>{t('History')}</CardTitle>
                    <CardDescription>
                      {t('{{count}} records', { count: history.length })}
                    </CardDescription>
                  </CardHeader>
                  <CardContent className='p-0'>
                    <Table>
                      <TableHeader>
                        <TableRow>
                          <TableHead>{t('Time')}</TableHead>
                          <TableHead>{t('User')}</TableHead>
                          <TableHead>{t('Active slots')}</TableHead>
                          <TableHead>{t('Window')}</TableHead>
                          <TableHead className='text-right'>
                            {t('Action')}
                          </TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {historyQuery.isLoading ? (
                          <TableRow>
                            <TableCell colSpan={5} className='h-32 text-center'>
                              {t('Loading')}
                            </TableCell>
                          </TableRow>
                        ) : history.length === 0 ? (
                          <TableRow>
                            <TableCell colSpan={5}>
                              <Empty className='min-h-32 border-0'>
                                <EmptyHeader>
                                  <EmptyTitle>{t('No data')}</EmptyTitle>
                                </EmptyHeader>
                              </Empty>
                            </TableCell>
                          </TableRow>
                        ) : (
                          history.map((row) => (
                            <TableRow key={row.id}>
                              <TableCell>
                                {formatTimestamp(row.created_at)}
                              </TableCell>
                              <TableCell>
                                {row.username || `#${row.user_id}`}
                              </TableCell>
                              <TableCell>
                                <Badge variant='secondary'>
                                  {row.active_slots}
                                </Badge>
                              </TableCell>
                              <TableCell>{row.window_secs}s</TableCell>
                              <TableCell className='text-right'>
                                <Button
                                  variant='ghost'
                                  size='sm'
                                  onClick={() =>
                                    openUsage({
                                      user_id: row.user_id,
                                      username: row.username,
                                      active_slots: row.active_slots,
                                    })
                                  }
                                >
                                  <Eye data-icon='inline-start' />
                                  {t('View')}
                                </Button>
                              </TableCell>
                            </TableRow>
                          ))
                        )}
                      </TableBody>
                    </Table>
                  </CardContent>
                </Card>
              </TabsContent>
            </Tabs>
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <TokenUsageSheet
        user={selectedUser}
        open={usageOpen}
        onOpenChange={setUsageOpen}
      />
    </>
  )
}
