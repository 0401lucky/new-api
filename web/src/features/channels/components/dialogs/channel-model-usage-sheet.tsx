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
import { Loader2 } from 'lucide-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { sideDrawerContentClassName } from '@/components/drawer-layout'
import { Badge } from '@/components/ui/badge'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Empty, EmptyHeader, EmptyTitle } from '@/components/ui/empty'
import { Progress } from '@/components/ui/progress'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatNumber } from '@/lib/format'

import { getChannelModelUsage } from '../../api'
import { channelsQueryKeys } from '../../lib'

interface ChannelModelUsageSheetProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  channelId: number
  channelName: string
}

/**
 * 渠道侧滑面板：展示该渠道下各模型被成功计费的调用次数。
 */
export function ChannelModelUsageSheet(props: ChannelModelUsageSheetProps) {
  const { t } = useTranslation()
  const query = useQuery({
    queryKey: channelsQueryKeys.modelUsage(props.channelId),
    queryFn: () => getChannelModelUsage(props.channelId),
    enabled: props.open && props.channelId > 0,
    staleTime: 30_000,
  })

  const models = query.data?.models ?? []
  const totalRequests = query.data?.total_requests ?? 0
  const maxRequests = models.reduce(
    (max, model) => Math.max(max, Number(model.request_count) || 0),
    0
  )

  let tableRows: ReactNode
  if (query.isFetching && models.length === 0) {
    tableRows = [
      'model-usage-loading-1',
      'model-usage-loading-2',
      'model-usage-loading-3',
      'model-usage-loading-4',
      'model-usage-loading-5',
    ].map((key) => (
      <TableRow key={key}>
        <TableCell>
          <Skeleton className='h-4 w-56' />
          <Skeleton className='mt-2 h-1.5 w-full' />
        </TableCell>
        <TableCell>
          <Skeleton className='ml-auto h-4 w-16' />
        </TableCell>
      </TableRow>
    ))
  } else if (models.length === 0) {
    tableRows = (
      <TableRow>
        <TableCell colSpan={2}>
          <Empty className='min-h-56 border-0'>
            <EmptyHeader>
              <EmptyTitle>{t('No data')}</EmptyTitle>
            </EmptyHeader>
          </Empty>
        </TableCell>
      </TableRow>
    )
  } else {
    tableRows = models.map((model) => {
      const requests = Number(model.request_count) || 0
      const percent =
        maxRequests > 0
          ? Math.max(4, Math.round((requests / maxRequests) * 100))
          : 0
      return (
        <TableRow key={model.model_name || 'unknown'}>
          <TableCell className='max-w-[28rem]'>
            <div className='flex min-w-0 flex-col gap-2'>
              <div className='flex min-w-0 items-center gap-2'>
                <span className='truncate font-medium'>
                  {model.model_name || '-'}
                </span>
                {requests === maxRequests && maxRequests > 0 ? (
                  <Badge variant='secondary'>{t('Top')}</Badge>
                ) : null}
              </div>
              <Progress value={percent} />
            </div>
          </TableCell>
          <TableCell className='text-right font-medium'>
            {formatNumber(requests)}
          </TableCell>
        </TableRow>
      )
    })
  }

  return (
    <Sheet open={props.open} onOpenChange={props.onOpenChange}>
      <SheetContent className={sideDrawerContentClassName('sm:max-w-3xl')}>
        <SheetHeader className='border-b'>
          <SheetTitle>{t('Channel Model Usage')}</SheetTitle>
          <SheetDescription>
            {`${props.channelName} · #${props.channelId}`}
          </SheetDescription>
        </SheetHeader>

        <div className='flex min-h-0 flex-1 flex-col gap-4 overflow-hidden px-4 py-4 sm:px-6'>
          <Card>
            <CardHeader className='pb-2'>
              <CardDescription>{t('Total requests')}</CardDescription>
              <CardTitle className='text-2xl'>
                {formatNumber(totalRequests)}
              </CardTitle>
            </CardHeader>
          </Card>

          <Card className='min-h-0 flex-1 gap-0 overflow-hidden py-0'>
            <CardHeader className='border-b py-3'>
              <div className='flex items-center justify-between gap-3'>
                <CardTitle className='text-base'>
                  {t('Model call count')}
                </CardTitle>
                {query.isFetching && (
                  <Loader2 className='text-muted-foreground size-4 animate-spin' />
                )}
              </div>
            </CardHeader>
            <CardContent className='min-h-0 flex-1 overflow-auto p-0'>
              <Table className='min-w-[420px]'>
                <TableHeader className='bg-background sticky top-0 z-10'>
                  <TableRow>
                    <TableHead>{t('Model')}</TableHead>
                    <TableHead className='w-32 text-right'>
                      {t('Requests')}
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>{tableRows}</TableBody>
              </Table>
            </CardContent>
          </Card>
        </div>
      </SheetContent>
    </Sheet>
  )
}
