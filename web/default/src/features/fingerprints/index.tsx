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
import { Eye, RefreshCw, Search, Users } from 'lucide-react'
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
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
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
import { UserInfoDialog } from '@/features/usage-logs/components/dialogs/user-info-dialog'
import {
  findUsersByVisitorId,
  getDuplicateFingerprints,
  getFingerprints,
  searchFingerprints,
} from './api'
import type {
  DuplicateFingerprint,
  FingerprintRecord,
  PageResponse,
} from './types'

const PAGE_SIZE = 20

function formatDate(value?: string) {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString()
}

function PaginationControls(props: {
  page: number
  total: number
  isFetching?: boolean
  onPageChange: (page: number) => void
}) {
  const { t } = useTranslation()
  const pageCount = Math.max(1, Math.ceil(props.total / PAGE_SIZE))

  return (
    <div className='flex flex-wrap items-center justify-between gap-3 px-4 py-3'>
      <div className='text-muted-foreground text-sm'>
        {t('{{count}} records', { count: props.total })}
      </div>
      <div className='flex items-center gap-2'>
        <Button
          variant='outline'
          size='sm'
          disabled={props.page <= 1 || props.isFetching}
          onClick={() => props.onPageChange(props.page - 1)}
        >
          {t('Previous')}
        </Button>
        <Badge variant='outline'>
          {props.page} / {pageCount}
        </Badge>
        <Button
          variant='outline'
          size='sm'
          disabled={props.page >= pageCount || props.isFetching}
          onClick={() => props.onPageChange(props.page + 1)}
        >
          {t('Next')}
        </Button>
      </div>
    </div>
  )
}

function UserButton(props: {
  record: FingerprintRecord
  onOpenUser: (userId: number) => void
}) {
  return (
    <Button
      variant='ghost'
      size='sm'
      className='max-w-44 justify-start px-2'
      onClick={() => props.onOpenUser(props.record.id)}
    >
      <Users data-icon='inline-start' />
      <span className='truncate'>
        {props.record.username || `#${props.record.id}`}
      </span>
    </Button>
  )
}

function FingerprintRows(props: {
  data?: PageResponse<FingerprintRecord>
  loading: boolean
  colSpan?: number
  onOpenUser: (userId: number) => void
}) {
  const { t } = useTranslation()
  const rows = props.data?.items || []

  if (props.loading) {
    return (
      <TableRow>
        <TableCell colSpan={props.colSpan ?? 6} className='h-32 text-center'>
          {t('Loading')}
        </TableCell>
      </TableRow>
    )
  }

  if (rows.length === 0) {
    return (
      <TableRow>
        <TableCell colSpan={props.colSpan ?? 6}>
          <Empty className='min-h-32 border-0'>
            <EmptyHeader>
              <EmptyTitle>{t('No data')}</EmptyTitle>
            </EmptyHeader>
          </Empty>
        </TableCell>
      </TableRow>
    )
  }

  return rows.map((record) => (
    <TableRow key={`${record.id}-${record.visitor_id}-${record.ip}`}>
      <TableCell>
        <UserButton record={record} onOpenUser={props.onOpenUser} />
      </TableCell>
      <TableCell className='max-w-72 truncate font-mono text-xs'>
        {record.visitor_id}
      </TableCell>
      <TableCell className='font-mono text-xs'>{record.ip || '-'}</TableCell>
      <TableCell>{formatDate(record.record_time)}</TableCell>
      <TableCell>
        <Badge variant={record.status === 1 ? 'secondary' : 'outline'}>
          {record.status}
        </Badge>
      </TableCell>
      <TableCell className='text-right'>
        <Button
          variant='ghost'
          size='sm'
          onClick={() => props.onOpenUser(record.id)}
        >
          <Eye data-icon='inline-start' />
          {t('View')}
        </Button>
      </TableCell>
    </TableRow>
  ))
}

function FingerprintTable(props: {
  title: string
  description: string
  data?: PageResponse<FingerprintRecord>
  loading: boolean
  page: number
  onPageChange: (page: number) => void
  onOpenUser: (userId: number) => void
}) {
  const { t } = useTranslation()

  return (
    <Card>
      <CardHeader className='border-b'>
        <CardTitle>{props.title}</CardTitle>
        <CardDescription>{props.description}</CardDescription>
      </CardHeader>
      <CardContent className='p-0'>
        <div className='overflow-x-auto'>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('User')}</TableHead>
                <TableHead>{t('Visitor ID')}</TableHead>
                <TableHead>{t('IP')}</TableHead>
                <TableHead>{t('Time')}</TableHead>
                <TableHead>{t('Status')}</TableHead>
                <TableHead className='text-right'>{t('Action')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <FingerprintRows
                data={props.data}
                loading={props.loading}
                onOpenUser={props.onOpenUser}
              />
            </TableBody>
          </Table>
        </div>
        <PaginationControls
          page={props.page}
          total={props.data?.total || 0}
          isFetching={props.loading}
          onPageChange={props.onPageChange}
        />
      </CardContent>
    </Card>
  )
}

function RelatedUsersSheet(props: {
  duplicate: DuplicateFingerprint | null
  open: boolean
  onOpenChange: (open: boolean) => void
  onOpenUser: (userId: number) => void
}) {
  const { t } = useTranslation()
  const query = useQuery({
    queryKey: [
      'fingerprint-related-users',
      props.duplicate?.visitor_id,
      props.duplicate?.ip,
    ],
    queryFn: () =>
      findUsersByVisitorId({
        visitorId: props.duplicate?.visitor_id || '',
        ip: props.duplicate?.ip || undefined,
        page_size: 100,
      }),
    enabled: props.open && Boolean(props.duplicate?.visitor_id),
  })

  const data = query.data?.data

  return (
    <Sheet open={props.open} onOpenChange={props.onOpenChange}>
      <SheetContent className={sideDrawerContentClassName('sm:max-w-4xl')}>
        <SheetHeader className='border-b'>
          <SheetTitle>{t('Related users')}</SheetTitle>
          <SheetDescription>
            {props.duplicate?.visitor_id || t('Loading')}
          </SheetDescription>
        </SheetHeader>
        <div className='min-h-0 flex-1 overflow-auto px-4 py-4'>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('User')}</TableHead>
                <TableHead>{t('Visitor ID')}</TableHead>
                <TableHead>{t('IP')}</TableHead>
                <TableHead>{t('Time')}</TableHead>
                <TableHead>{t('Status')}</TableHead>
                <TableHead className='text-right'>{t('Action')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <FingerprintRows
                data={data}
                loading={query.isFetching}
                onOpenUser={props.onOpenUser}
              />
            </TableBody>
          </Table>
        </div>
      </SheetContent>
    </Sheet>
  )
}

export function Fingerprints() {
  const { t } = useTranslation()
  const [duplicatePage, setDuplicatePage] = useState(1)
  const [allPage, setAllPage] = useState(1)
  const [searchPage, setSearchPage] = useState(1)
  const [keyword, setKeyword] = useState('')
  const [submittedKeyword, setSubmittedKeyword] = useState('')
  const [activeDuplicate, setActiveDuplicate] =
    useState<DuplicateFingerprint | null>(null)
  const [relatedOpen, setRelatedOpen] = useState(false)
  const [selectedUserId, setSelectedUserId] = useState<number | null>(null)
  const [userInfoOpen, setUserInfoOpen] = useState(false)

  const duplicateQuery = useQuery({
    queryKey: ['fingerprint-duplicates', duplicatePage],
    queryFn: () =>
      getDuplicateFingerprints({
        p: duplicatePage,
        page_size: PAGE_SIZE,
      }),
  })

  const allQuery = useQuery({
    queryKey: ['fingerprints', allPage],
    queryFn: () =>
      getFingerprints({
        p: allPage,
        page_size: PAGE_SIZE,
      }),
  })

  const searchQuery = useQuery({
    queryKey: ['fingerprint-search', submittedKeyword, searchPage],
    queryFn: () =>
      searchFingerprints({
        keyword: submittedKeyword,
        p: searchPage,
        page_size: PAGE_SIZE,
      }),
    enabled: submittedKeyword.trim() !== '',
  })

  const duplicateData = duplicateQuery.data?.data
  const allData = allQuery.data?.data
  const searchData = searchQuery.data?.data
  const duplicateRows = duplicateData?.items || []

  const isFetching = useMemo(
    () =>
      duplicateQuery.isFetching || allQuery.isFetching || searchQuery.isFetching,
    [duplicateQuery.isFetching, allQuery.isFetching, searchQuery.isFetching]
  )

  const openRelatedUsers = (duplicate: DuplicateFingerprint) => {
    setActiveDuplicate(duplicate)
    setRelatedOpen(true)
  }

  const openUser = (userId: number) => {
    setSelectedUserId(userId)
    setUserInfoOpen(true)
  }

  const refresh = () => {
    void duplicateQuery.refetch()
    void allQuery.refetch()
    if (submittedKeyword) {
      void searchQuery.refetch()
    }
  }

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>{t('Fingerprints')}</SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <Button variant='outline' disabled={isFetching} onClick={refresh}>
            <RefreshCw data-icon='inline-start' />
            {t('Refresh')}
          </Button>
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <Tabs defaultValue='duplicates' className='flex flex-col gap-4'>
            <TabsList>
              <TabsTrigger value='duplicates'>
                {t('Duplicate fingerprints')}
              </TabsTrigger>
              <TabsTrigger value='all'>{t('All records')}</TabsTrigger>
              <TabsTrigger value='search'>{t('Search')}</TabsTrigger>
            </TabsList>

            <TabsContent value='duplicates'>
              <Card>
                <CardHeader className='border-b'>
                  <CardTitle>{t('Duplicate fingerprints')}</CardTitle>
                  <CardDescription>
                    {t('{{count}} records', {
                      count: duplicateData?.total || 0,
                    })}
                  </CardDescription>
                </CardHeader>
                <CardContent className='p-0'>
                  <div className='overflow-x-auto'>
                    <Table>
                      <TableHeader>
                        <TableRow>
                          <TableHead>{t('Visitor ID')}</TableHead>
                          <TableHead>{t('IP')}</TableHead>
                          <TableHead>{t('Users')}</TableHead>
                          <TableHead>{t('Last seen')}</TableHead>
                          <TableHead className='text-right'>
                            {t('Action')}
                          </TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {duplicateQuery.isLoading ? (
                          <TableRow>
                            <TableCell colSpan={5} className='h-32 text-center'>
                              {t('Loading')}
                            </TableCell>
                          </TableRow>
                        ) : duplicateRows.length === 0 ? (
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
                          duplicateRows.map((row) => (
                            <TableRow key={`${row.visitor_id}-${row.ip}`}>
                              <TableCell className='max-w-96 truncate font-mono text-xs'>
                                {row.visitor_id}
                              </TableCell>
                              <TableCell className='font-mono text-xs'>
                                {row.ip || '-'}
                              </TableCell>
                              <TableCell>
                                <Badge variant='secondary'>
                                  {row.user_count}
                                </Badge>
                              </TableCell>
                              <TableCell>{formatDate(row.last_seen)}</TableCell>
                              <TableCell className='text-right'>
                                <Button
                                  variant='ghost'
                                  size='sm'
                                  onClick={() => openRelatedUsers(row)}
                                >
                                  <Users data-icon='inline-start' />
                                  {t('View users')}
                                </Button>
                              </TableCell>
                            </TableRow>
                          ))
                        )}
                      </TableBody>
                    </Table>
                  </div>
                  <PaginationControls
                    page={duplicatePage}
                    total={duplicateData?.total || 0}
                    isFetching={duplicateQuery.isFetching}
                    onPageChange={setDuplicatePage}
                  />
                </CardContent>
              </Card>
            </TabsContent>

            <TabsContent value='all'>
              <FingerprintTable
                title={t('All records')}
                description={t('{{count}} records', {
                  count: allData?.total || 0,
                })}
                data={allData}
                loading={allQuery.isLoading}
                page={allPage}
                onPageChange={setAllPage}
                onOpenUser={openUser}
              />
            </TabsContent>

            <TabsContent value='search'>
              <div className='flex flex-col gap-4'>
                <Card>
                  <CardHeader className='border-b'>
                    <CardTitle>{t('Search')}</CardTitle>
                  </CardHeader>
                  <CardContent>
                    <FieldGroup className='grid gap-3 md:grid-cols-[minmax(0,1fr)_auto]'>
                      <Field>
                        <FieldLabel>{t('Keyword')}</FieldLabel>
                        <Input
                          value={keyword}
                          onChange={(event) => setKeyword(event.target.value)}
                          placeholder={t('Visitor ID, username, or email')}
                          onKeyDown={(event) => {
                            if (event.key === 'Enter') {
                              setSearchPage(1)
                              setSubmittedKeyword(keyword.trim())
                            }
                          }}
                        />
                      </Field>
                      <Field className='justify-end'>
                        <FieldLabel className='sr-only'>
                          {t('Search')}
                        </FieldLabel>
                        <Button
                          disabled={!keyword.trim()}
                          onClick={() => {
                            setSearchPage(1)
                            setSubmittedKeyword(keyword.trim())
                          }}
                        >
                          <Search data-icon='inline-start' />
                          {t('Search')}
                        </Button>
                      </Field>
                    </FieldGroup>
                  </CardContent>
                </Card>

                <FingerprintTable
                  title={t('Search results')}
                  description={t('{{count}} records', {
                    count: searchData?.total || 0,
                  })}
                  data={searchData}
                  loading={searchQuery.isFetching}
                  page={searchPage}
                  onPageChange={setSearchPage}
                  onOpenUser={openUser}
                />
              </div>
            </TabsContent>
          </Tabs>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <RelatedUsersSheet
        duplicate={activeDuplicate}
        open={relatedOpen}
        onOpenChange={setRelatedOpen}
        onOpenUser={openUser}
      />
      <UserInfoDialog
        userId={selectedUserId}
        open={userInfoOpen}
        onOpenChange={setUserInfoOpen}
      />
    </>
  )
}
