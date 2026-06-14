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
import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { AlertCircle, ChevronLeft, Copy, Eye, RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Empty, EmptyHeader, EmptyTitle } from '@/components/ui/empty'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { ScrollArea } from '@/components/ui/scroll-area'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
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
import { sideDrawerContentClassName } from '@/components/drawer-layout'
import { SectionPageLayout } from '@/components/layout'
import { getRecentCallById, getRecentCalls } from './api'
import type {
  RecentCallBody,
  RecentCallRecord,
  RecentCallRequest,
  RecentCallResponse,
} from './types'

const LIMIT_OPTIONS = [20, 50, 100]

function formatDate(value?: string) {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString()
}

function formatBody(body?: string, bodyType?: string) {
  if (!body) return ''
  if (bodyType === 'json') {
    try {
      return JSON.stringify(JSON.parse(body), null, 2)
    } catch {
      return body
    }
  }
  return body
}

function bodyLabel(body?: RecentCallBody) {
  if (!body || body.omitted) return '-'
  return body.body_type || '-'
}

function metadataForRequest(record?: RecentCallRecord | null) {
  if (!record) return {}
  const request = record.request || ({} as RecentCallRequest)
  return {
    id: record.id,
    created_at: record.created_at,
    user_id: record.user_id,
    channel_id: record.channel_id,
    model_name: record.model_name,
    method: record.method,
    path: record.path,
    headers: request.headers || {},
    body_type: request.body_type,
    body_truncated: Boolean(request.truncated),
    body_omitted: Boolean(request.omitted),
    omit_reason: request.omit_reason,
  }
}

function metadataForResponse(record?: RecentCallRecord | null) {
  const response = record?.response
  if (!response) return null
  return {
    status_code: response.status_code,
    headers: response.headers || {},
    body_type: response.body_type,
    body_truncated: Boolean(response.truncated),
    body_omitted: Boolean(response.omitted),
    omit_reason: response.omit_reason,
  }
}

function StatusBadge(props: { status?: number }) {
  const { status } = props
  if (!status) return <Badge variant='outline'>-</Badge>
  return (
    <Badge variant={status >= 400 ? 'destructive' : 'secondary'}>
      {status}
    </Badge>
  )
}

function CodePanel(props: {
  title: string
  content?: string | object | null
  body?: RecentCallBody
}) {
  const { t } = useTranslation()
  const text =
    typeof props.content === 'string'
      ? props.content
      : JSON.stringify(props.content ?? {}, null, 2)
  const shownText = props.body
    ? formatBody(text, props.body.body_type)
    : formatBody(text, 'json')

  const copy = async () => {
    await navigator.clipboard.writeText(shownText)
    toast.success(t('Copied to clipboard'))
  }

  return (
    <Card size='sm' className='min-h-0'>
      <CardHeader className='border-b'>
        <CardTitle>{props.title}</CardTitle>
        <CardAction>
          <Button
            variant='ghost'
            size='sm'
            onClick={copy}
            disabled={!shownText}
          >
            <Copy data-icon='inline-start' />
            {t('Copy')}
          </Button>
        </CardAction>
        {props.body && (
          <CardDescription>
            {props.body.omitted
              ? t('Body omitted: {{reason}}', {
                  reason: props.body.omit_reason || '-',
                })
              : t('Body type: {{type}}', {
                  type: bodyLabel(props.body),
                })}
          </CardDescription>
        )}
      </CardHeader>
      <CardContent className='min-h-0'>
        {props.body?.omitted ? (
          <Empty className='min-h-40 border'>
            <EmptyHeader>
              <EmptyTitle>
                {t('Body omitted: {{reason}}', {
                  reason: props.body.omit_reason || '-',
                })}
              </EmptyTitle>
            </EmptyHeader>
          </Empty>
        ) : (
          <ScrollArea className='bg-muted/30 h-72 rounded-lg border'>
            <pre className='m-0 min-w-full p-3 font-mono text-xs leading-relaxed break-words whitespace-pre-wrap'>
              {shownText || t('No data')}
            </pre>
          </ScrollArea>
        )}
      </CardContent>
    </Card>
  )
}

function DetailSheet(props: {
  open: boolean
  record?: RecentCallRecord | null
  loading: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const record = props.record
  const response = record?.response as RecentCallResponse | undefined
  const request = record?.request
  const stream = record?.stream
  const error = record?.error

  return (
    <Sheet open={props.open} onOpenChange={props.onOpenChange}>
      <SheetContent className={sideDrawerContentClassName('sm:max-w-5xl')}>
        <SheetHeader className='border-b'>
          <SheetTitle>{t('Recent call details')}</SheetTitle>
          <SheetDescription>
            {record
              ? `#${record.id} · ${record.method} ${record.path}`
              : t('Loading')}
          </SheetDescription>
        </SheetHeader>

        <ScrollArea className='min-h-0 flex-1 px-4 pb-4'>
          {props.loading ? (
            <div className='text-muted-foreground flex min-h-80 items-center justify-center text-sm'>
              {t('Loading')}
            </div>
          ) : !record ? (
            <Empty className='min-h-80 border'>
              <EmptyHeader>
                <EmptyTitle>{t('No data')}</EmptyTitle>
              </EmptyHeader>
            </Empty>
          ) : (
            <div className='flex flex-col gap-4 py-4'>
              <div className='flex flex-wrap items-center gap-2'>
                <Badge variant='outline'>{formatDate(record.created_at)}</Badge>
                <Badge variant='secondary'>uid: {record.user_id}</Badge>
                {record.channel_id ? (
                  <Badge variant='outline'>ch: {record.channel_id}</Badge>
                ) : null}
                {record.model_name ? (
                  <Badge variant='outline'>{record.model_name}</Badge>
                ) : null}
                {response ? (
                  <StatusBadge status={response.status_code} />
                ) : null}
                {stream ? (
                  <Badge variant='secondary'>{t('Stream')}</Badge>
                ) : null}
                {error ? (
                  <Badge variant='destructive'>{t('Error')}</Badge>
                ) : null}
              </div>

              <div className='grid min-h-0 gap-4 xl:grid-cols-2'>
                <CodePanel
                  title={t('Request metadata')}
                  content={metadataForRequest(record)}
                />
                <CodePanel
                  title={t('Request body')}
                  content={request?.body || ''}
                  body={request}
                />
              </div>

              <div className='grid min-h-0 gap-4 xl:grid-cols-2'>
                <CodePanel
                  title={t('Response metadata')}
                  content={metadataForResponse(record) || {}}
                />
                <CodePanel
                  title={t('Response body')}
                  content={response?.body || ''}
                  body={response}
                />
              </div>

              {stream ? (
                <div className='grid min-h-0 gap-4 xl:grid-cols-2'>
                  <CodePanel
                    title={t('Aggregated stream text')}
                    content={stream.aggregated_text || ''}
                    body={{
                      body_type: 'text',
                      truncated: stream.aggregated_truncated,
                    }}
                  />
                  <CodePanel
                    title={t('Raw SSE chunks')}
                    content={(stream.chunks || []).join('\n')}
                    body={{
                      body_type: 'text',
                      truncated: stream.chunks_truncated,
                    }}
                  />
                </div>
              ) : null}

              {error ? (
                <CodePanel title={t('Error details')} content={error} />
              ) : null}
            </div>
          )}
        </ScrollArea>
      </SheetContent>
    </Sheet>
  )
}

export function RecentCalls() {
  const { t } = useTranslation()
  const [limit, setLimit] = useState(100)
  const [beforeId, setBeforeId] = useState('')
  const [activeId, setActiveId] = useState<number | null>(null)
  const [detailOpen, setDetailOpen] = useState(false)

  const listQuery = useQuery({
    queryKey: ['recent-calls', limit, beforeId],
    queryFn: () => getRecentCalls({ limit, beforeId: beforeId || undefined }),
  })

  const detailQuery = useQuery({
    queryKey: ['recent-call', activeId],
    queryFn: () => getRecentCallById(activeId || 0),
    enabled: Boolean(activeId && detailOpen),
  })

  const rows = listQuery.data?.data || []
  const nextBeforeId = useMemo(() => {
    if (rows.length === 0) return ''
    return String(Math.min(...rows.map((row) => row.id)))
  }, [rows])

  const openDetail = (record: RecentCallRecord) => {
    setActiveId(record.id)
    setDetailOpen(true)
  }

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>{t('Recent calls')}</SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <Button
            variant='outline'
            onClick={() => void listQuery.refetch()}
            disabled={listQuery.isFetching}
          >
            <RefreshCw data-icon='inline-start' />
            {t('Refresh')}
          </Button>
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <div className='flex flex-col gap-4'>
            <Card>
              <CardHeader className='border-b'>
                <CardTitle>{t('Capture window')}</CardTitle>
                <CardDescription>
                  {t('Showing in-memory recent relay calls')}
                </CardDescription>
              </CardHeader>
              <CardContent>
                <FieldGroup className='grid gap-3 md:grid-cols-[10rem_minmax(0,1fr)_auto_auto]'>
                  <Field>
                    <FieldLabel>{t('Limit')}</FieldLabel>
                    <Select
                      items={LIMIT_OPTIONS.map((value) => ({
                        value: String(value),
                        label: String(value),
                      }))}
                      value={String(limit)}
                      onValueChange={(value) => setLimit(Number(value) || 100)}
                    >
                      <SelectTrigger className='w-full'>
                        <SelectValue placeholder={String(limit)} />
                      </SelectTrigger>
                      <SelectContent alignItemWithTrigger={false}>
                        <SelectGroup>
                          {LIMIT_OPTIONS.map((value) => (
                            <SelectItem key={value} value={String(value)}>
                              {value}
                            </SelectItem>
                          ))}
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                  </Field>
                  <Field>
                    <FieldLabel>{t('Before ID')}</FieldLabel>
                    <Input
                      value={beforeId}
                      onChange={(event) => setBeforeId(event.target.value)}
                      placeholder={t('Latest')}
                    />
                  </Field>
                  <Field className='justify-end'>
                    <FieldLabel className='sr-only'>{t('Latest')}</FieldLabel>
                    <Button variant='outline' onClick={() => setBeforeId('')}>
                      {t('Latest')}
                    </Button>
                  </Field>
                  <Field className='justify-end'>
                    <FieldLabel className='sr-only'>
                      {t('Previous page')}
                    </FieldLabel>
                    <Button
                      variant='secondary'
                      disabled={!nextBeforeId}
                      onClick={() => setBeforeId(nextBeforeId)}
                    >
                      <ChevronLeft data-icon='inline-start' />
                      {t('Previous page')}
                    </Button>
                  </Field>
                </FieldGroup>
              </CardContent>
            </Card>

            <Card>
              <CardHeader className='border-b'>
                <CardTitle>{t('Recent calls')}</CardTitle>
                <CardDescription>
                  {t('{{count}} calls loaded', { count: rows.length })}
                </CardDescription>
              </CardHeader>
              <CardContent className='p-0'>
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>{t('ID')}</TableHead>
                      <TableHead>{t('Time')}</TableHead>
                      <TableHead>{t('User')}</TableHead>
                      <TableHead>{t('Channel')}</TableHead>
                      <TableHead>{t('Model')}</TableHead>
                      <TableHead>{t('Method')}</TableHead>
                      <TableHead>{t('Path')}</TableHead>
                      <TableHead>{t('Status')}</TableHead>
                      <TableHead>{t('Flags')}</TableHead>
                      <TableHead className='text-right'>
                        {t('Action')}
                      </TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {listQuery.isLoading ? (
                      <TableRow>
                        <TableCell colSpan={10} className='h-32 text-center'>
                          {t('Loading')}
                        </TableCell>
                      </TableRow>
                    ) : listQuery.isError ? (
                      <TableRow>
                        <TableCell colSpan={10} className='h-32 text-center'>
                          <span className='text-destructive inline-flex items-center gap-2'>
                            <AlertCircle data-icon='inline-start' />
                            {t('Failed to load recent calls')}
                          </span>
                        </TableCell>
                      </TableRow>
                    ) : rows.length === 0 ? (
                      <TableRow>
                        <TableCell colSpan={10}>
                          <Empty className='min-h-32 border-0'>
                            <EmptyHeader>
                              <EmptyTitle>{t('No data')}</EmptyTitle>
                            </EmptyHeader>
                          </Empty>
                        </TableCell>
                      </TableRow>
                    ) : (
                      rows.map((record) => (
                        <TableRow
                          key={record.id}
                          className='cursor-pointer'
                          onDoubleClick={() => openDetail(record)}
                        >
                          <TableCell>#{record.id}</TableCell>
                          <TableCell>{formatDate(record.created_at)}</TableCell>
                          <TableCell>{record.user_id || '-'}</TableCell>
                          <TableCell>{record.channel_id || '-'}</TableCell>
                          <TableCell className='max-w-56 truncate'>
                            {record.model_name || '-'}
                          </TableCell>
                          <TableCell>{record.method}</TableCell>
                          <TableCell className='max-w-72 truncate'>
                            {record.path}
                          </TableCell>
                          <TableCell>
                            <StatusBadge
                              status={record.response?.status_code}
                            />
                          </TableCell>
                          <TableCell>
                            <div className='flex flex-wrap gap-1'>
                              {record.request?.truncated ? (
                                <Badge variant='outline'>
                                  {t('Request truncated')}
                                </Badge>
                              ) : null}
                              {record.response?.truncated ? (
                                <Badge variant='outline'>
                                  {t('Response truncated')}
                                </Badge>
                              ) : null}
                              {record.stream ? (
                                <Badge variant='secondary'>{t('Stream')}</Badge>
                              ) : null}
                              {record.error ? (
                                <Badge variant='destructive'>
                                  {t('Error')}
                                </Badge>
                              ) : null}
                            </div>
                          </TableCell>
                          <TableCell className='text-right'>
                            <Button
                              variant='ghost'
                              size='sm'
                              onClick={() => openDetail(record)}
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
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <DetailSheet
        open={detailOpen}
        onOpenChange={setDetailOpen}
        loading={detailQuery.isFetching}
        record={detailQuery.data?.data}
      />
    </>
  )
}
