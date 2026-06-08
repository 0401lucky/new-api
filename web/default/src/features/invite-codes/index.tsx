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
import { useEffect, useMemo, useState } from 'react'
import { z } from 'zod'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery } from '@tanstack/react-query'
import { getRouteApi } from '@tanstack/react-router'
import {
  type ColumnDef,
  type Row,
  type SortingState,
  type Table,
  type VisibilityState,
  getCoreRowModel,
  getFacetedRowModel,
  getFacetedUniqueValues,
  getFilteredRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
} from '@tanstack/react-table'
import { useMediaQuery } from '@/hooks'
import type { TFunction } from 'i18next'
import {
  Download,
  Edit,
  MoreHorizontal as DotsHorizontalIcon,
  Plus,
  Power,
  PowerOff,
  Trash2,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { formatTimestampToDate } from '@/lib/format'
import { addTimeToDate } from '@/lib/time'
import { useTableUrlState } from '@/hooks/use-table-url-state'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuShortcut,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { ConfirmDialog } from '@/components/confirm-dialog'
import { CopyButton } from '@/components/copy-button'
import {
  DISABLED_ROW_DESKTOP,
  DISABLED_ROW_MOBILE,
  DataTableBulkActions as BulkActionsToolbar,
  DataTableColumnHeader,
  DataTablePage,
} from '@/components/data-table'
import { DateTimePicker } from '@/components/datetime-picker'
import {
  SideDrawerSection,
  sideDrawerContentClassName,
  sideDrawerFooterClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
} from '@/components/drawer-layout'
import { SectionPageLayout } from '@/components/layout'
import { MaskedValueDisplay } from '@/components/masked-value-display'
import { StatusBadge } from '@/components/status-badge'
import { TableId } from '@/components/table-id'
import {
  createInviteCode,
  deleteInvalidInviteCodes,
  deleteInviteCode,
  getInviteCode,
  getInviteCodes,
  searchInviteCodes,
  updateInviteCode,
  updateInviteCodeStatus,
} from './api'
import {
  INVITE_CODE_FILTER_EXPIRED,
  INVITE_CODE_STATUSES,
  INVITE_CODE_STATUS,
  INVITE_CODE_SUCCESS_MESSAGES,
  INVITE_CODE_VALIDATION,
  getInviteCodeStatusOptions,
} from './constants'
import { type InviteCode, inviteCodeSchema } from './types'

const route = getRouteApi('/_authenticated/invite-codes/')

type InviteCodesDialogType = 'create' | 'update' | 'delete' | null

type InviteCodeFormValues = {
  name: string
  expired_time?: Date
  count?: number
  key_prefix?: string
}

const INVITE_CODE_FORM_DEFAULT_VALUES: InviteCodeFormValues = {
  name: '',
  expired_time: undefined,
  count: 1,
  key_prefix: '',
}

function isTimestampExpired(timestamp: number): boolean {
  return timestamp !== 0 && timestamp < Math.floor(Date.now() / 1000)
}

function isInviteCodeExpired(expiredTime: number, status: number): boolean {
  return (
    status === INVITE_CODE_STATUS.ENABLED && isTimestampExpired(expiredTime)
  )
}

function getInviteCodeFormSchema(t: TFunction) {
  return z.object({
    name: z
      .string()
      .min(
        INVITE_CODE_VALIDATION.NAME_MIN_LENGTH,
        t('Name must be between {{min}} and {{max}} characters', {
          min: INVITE_CODE_VALIDATION.NAME_MIN_LENGTH,
          max: INVITE_CODE_VALIDATION.NAME_MAX_LENGTH,
        })
      )
      .max(
        INVITE_CODE_VALIDATION.NAME_MAX_LENGTH,
        t('Name must be between {{min}} and {{max}} characters', {
          min: INVITE_CODE_VALIDATION.NAME_MIN_LENGTH,
          max: INVITE_CODE_VALIDATION.NAME_MAX_LENGTH,
        })
      ),
    expired_time: z.date().optional(),
    count: z
      .number()
      .min(
        INVITE_CODE_VALIDATION.COUNT_MIN,
        t('Count must be between {{min}} and {{max}}', {
          min: INVITE_CODE_VALIDATION.COUNT_MIN,
          max: INVITE_CODE_VALIDATION.COUNT_MAX,
        })
      )
      .max(
        INVITE_CODE_VALIDATION.COUNT_MAX,
        t('Count must be between {{min}} and {{max}}', {
          min: INVITE_CODE_VALIDATION.COUNT_MIN,
          max: INVITE_CODE_VALIDATION.COUNT_MAX,
        })
      )
      .optional(),
    key_prefix: z
      .string()
      .max(
        INVITE_CODE_VALIDATION.KEY_PREFIX_MAX_LENGTH,
        t('Key prefix must be at most {{max}} characters', {
          max: INVITE_CODE_VALIDATION.KEY_PREFIX_MAX_LENGTH,
        })
      )
      .optional(),
  })
}

function transformFormDataToPayload(data: InviteCodeFormValues) {
  return {
    name: data.name,
    expired_time: data.expired_time
      ? Math.floor(data.expired_time.getTime() / 1000)
      : 0,
    count: data.count || 1,
    ...(data.key_prefix?.trim() ? { key_prefix: data.key_prefix.trim() } : {}),
  }
}

function transformInviteCodeToFormDefaults(
  inviteCode: InviteCode
): InviteCodeFormValues {
  return {
    name: inviteCode.name,
    expired_time:
      inviteCode.expired_time > 0
        ? new Date(inviteCode.expired_time * 1000)
        : undefined,
    count: 1,
    key_prefix: '',
  }
}

function downloadTextAsFile(text: string, filename: string) {
  const blob = new Blob([text], { type: 'text/plain;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  URL.revokeObjectURL(url)
}

type InviteCodesColumnsOptions = {
  onEdit: (inviteCode: InviteCode) => void
  onDelete: (inviteCode: InviteCode) => void
  onRefresh: () => void
}

function useInviteCodesColumns({
  onEdit,
  onDelete,
  onRefresh,
}: InviteCodesColumnsOptions): ColumnDef<InviteCode>[] {
  const { t } = useTranslation()

  return [
    {
      id: 'select',
      meta: { label: t('Select') },
      header: ({ table }) => (
        <Checkbox
          checked={table.getIsAllPageRowsSelected()}
          indeterminate={table.getIsSomePageRowsSelected()}
          onCheckedChange={(value) => table.toggleAllPageRowsSelected(!!value)}
          aria-label={t('Select all')}
          className='translate-y-[2px]'
        />
      ),
      cell: ({ row }) => (
        <Checkbox
          checked={row.getIsSelected()}
          onCheckedChange={(value) => row.toggleSelected(!!value)}
          aria-label={t('Select row')}
          className='translate-y-[2px]'
        />
      ),
      enableSorting: false,
      enableHiding: false,
    },
    {
      accessorKey: 'id',
      meta: { label: t('ID'), mobileHidden: true },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('ID')} />
      ),
      cell: ({ row }) => (
        <TableId value={row.getValue('id') as number} className='w-[60px]' />
      ),
    },
    {
      accessorKey: 'name',
      meta: { label: t('Name'), mobileTitle: true },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Name')} />
      ),
      cell: ({ row }) => (
        <div className='max-w-[150px] truncate font-medium'>
          {row.getValue('name')}
        </div>
      ),
    },
    {
      accessorKey: 'status',
      meta: { label: t('Status'), mobileBadge: true },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Status')} />
      ),
      cell: ({ row }) => {
        const inviteCode = row.original
        const statusValue = row.getValue('status') as number

        if (isInviteCodeExpired(inviteCode.expired_time, statusValue)) {
          return (
            <StatusBadge
              label={t('Expired')}
              variant='warning'
              copyable={false}
            />
          )
        }

        const statusConfig = INVITE_CODE_STATUSES[statusValue]
        if (!statusConfig) return null

        return (
          <StatusBadge
            label={t(statusConfig.labelKey)}
            variant={statusConfig.variant}
            copyable={false}
          />
        )
      },
      filterFn: (row, id, value) => {
        const inviteCode = row.original
        const statusValue = row.getValue(id) as number
        if (value.includes(INVITE_CODE_FILTER_EXPIRED)) {
          if (isInviteCodeExpired(inviteCode.expired_time, statusValue)) {
            return true
          }
        }
        return value.includes(String(statusValue))
      },
    },
    {
      id: 'code',
      accessorKey: 'key',
      meta: { label: t('Code') },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Code')} />
      ),
      cell: ({ row }) => {
        const key = row.original.key
        const maskedKey = `${key.slice(0, 8)}${'*'.repeat(16)}${key.slice(-8)}`
        return (
          <MaskedValueDisplay
            label={t('Full Code')}
            fullValue={key}
            maskedValue={maskedKey}
            copyTooltip={t('Copy code')}
            copyAriaLabel={t('Copy invite code')}
          />
        )
      },
      enableSorting: false,
    },
    {
      accessorKey: 'created_time',
      meta: { label: t('Created'), mobileHidden: true },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Created')} />
      ),
      cell: ({ row }) => (
        <div className='min-w-[140px] font-mono text-sm'>
          {formatTimestampToDate(row.getValue('created_time'))}
        </div>
      ),
    },
    {
      accessorKey: 'expired_time',
      meta: { label: t('Expires'), mobileHidden: true },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Expires')} />
      ),
      cell: ({ row }) => {
        const expiredTime = row.getValue('expired_time') as number
        if (expiredTime === 0) {
          return (
            <StatusBadge
              label={t('Never')}
              variant='neutral'
              copyable={false}
            />
          )
        }
        const expired = isTimestampExpired(expiredTime)
        return (
          <div
            className={`min-w-[140px] font-mono text-sm ${expired ? 'text-destructive' : ''}`}
          >
            {formatTimestampToDate(expiredTime)}
          </div>
        )
      },
    },
    {
      accessorKey: 'used_user_id',
      meta: { label: t('Used By'), mobileHidden: true },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Used By')} />
      ),
      cell: ({ row }) => {
        const userId = row.getValue('used_user_id') as number
        const inviteCode = row.original
        if (userId === 0) {
          return <span className='text-muted-foreground text-sm'>-</span>
        }
        return (
          <Tooltip>
            <TooltipTrigger
              render={
                <StatusBadge
                  label={t('User {{id}}', { id: userId })}
                  variant='neutral'
                  copyable={false}
                  className='cursor-help'
                />
              }
            />
            <TooltipContent>
              <div className='space-y-1 text-xs'>
                <div>
                  {t('User ID:')} {userId}
                </div>
                {inviteCode.used_time > 0 && (
                  <div>
                    {t('Used:')} {formatTimestampToDate(inviteCode.used_time)}
                  </div>
                )}
              </div>
            </TooltipContent>
          </Tooltip>
        )
      },
    },
    {
      id: 'actions',
      cell: ({ row }) => (
        <InviteCodeRowActions
          row={row}
          onEdit={onEdit}
          onDelete={onDelete}
          onRefresh={onRefresh}
        />
      ),
    },
  ]
}

function InviteCodeRowActions({
  row,
  onEdit,
  onDelete,
  onRefresh,
}: {
  row: Row<InviteCode>
  onEdit: (inviteCode: InviteCode) => void
  onDelete: (inviteCode: InviteCode) => void
  onRefresh: () => void
}) {
  const { t } = useTranslation()
  const inviteCode = inviteCodeSchema.parse(row.original)
  const isEnabled = inviteCode.status === INVITE_CODE_STATUS.ENABLED
  const isUsed = inviteCode.status === INVITE_CODE_STATUS.USED
  const isExpired = isInviteCodeExpired(
    inviteCode.expired_time,
    inviteCode.status
  )

  const handleToggleStatus = async () => {
    const nextStatus = isEnabled
      ? INVITE_CODE_STATUS.DISABLED
      : INVITE_CODE_STATUS.ENABLED
    const result = await updateInviteCodeStatus(inviteCode.id, nextStatus)
    if (result.success) {
      toast.success(
        isEnabled
          ? t(INVITE_CODE_SUCCESS_MESSAGES.DISABLED)
          : t(INVITE_CODE_SUCCESS_MESSAGES.ENABLED)
      )
      onRefresh()
    }
  }

  const canEdit = isEnabled && !isExpired
  const canToggle = !isUsed && !isExpired

  return (
    <DropdownMenu modal={false}>
      <DropdownMenuTrigger
        render={
          <Button
            variant='ghost'
            className='data-popup-open:bg-muted flex h-8 w-8 p-0'
          />
        }
      >
        <DotsHorizontalIcon className='h-4 w-4' />
        <span className='sr-only'>{t('Open menu')}</span>
      </DropdownMenuTrigger>
      <DropdownMenuContent align='end' className='w-[160px]'>
        <DropdownMenuItem
          onClick={() => onEdit(inviteCode)}
          disabled={!canEdit}
        >
          {t('Edit')}
          <DropdownMenuShortcut>
            <Edit size={16} />
          </DropdownMenuShortcut>
        </DropdownMenuItem>
        {canToggle && (
          <DropdownMenuItem onClick={handleToggleStatus}>
            {isEnabled ? (
              <>
                {t('Disable')}
                <DropdownMenuShortcut>
                  <PowerOff size={16} />
                </DropdownMenuShortcut>
              </>
            ) : (
              <>
                {t('Enable')}
                <DropdownMenuShortcut>
                  <Power size={16} />
                </DropdownMenuShortcut>
              </>
            )}
          </DropdownMenuItem>
        )}
        <DropdownMenuSeparator />
        <DropdownMenuItem
          onClick={() => onDelete(inviteCode)}
          className='text-destructive focus:text-destructive'
        >
          {t('Delete')}
          <DropdownMenuShortcut>
            <Trash2 size={16} />
          </DropdownMenuShortcut>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function InviteCodesBulkActions<TData>({
  table,
  onRefresh,
}: {
  table: Table<TData>
  onRefresh: () => void
}) {
  const { t } = useTranslation()
  const [showDeleteInvalidConfirm, setShowDeleteInvalidConfirm] =
    useState(false)
  const [isDeleting, setIsDeleting] = useState(false)
  const selectedRows = table.getFilteredSelectedRowModel().rows

  const contentToCopy = useMemo(() => {
    const selectedCodes = selectedRows.map((row) => {
      const inviteCode = row.original as InviteCode
      return `${inviteCode.name}\t${inviteCode.key}`
    })
    return selectedCodes.join('\n')
  }, [selectedRows])

  const handleDeleteInvalid = async () => {
    setIsDeleting(true)
    try {
      const result = await deleteInvalidInviteCodes()
      if (result.success) {
        const count = result.data || 0
        toast.success(
          t('Successfully deleted {{count}} invalid invite codes', { count })
        )
        table.resetRowSelection()
        onRefresh()
        setShowDeleteInvalidConfirm(false)
      }
    } finally {
      setIsDeleting(false)
    }
  }

  return (
    <>
      <BulkActionsToolbar table={table} entityName={t('invite code')}>
        <CopyButton
          value={contentToCopy}
          variant='outline'
          size='icon'
          className='size-8'
          tooltip={t('Copy selected codes')}
          successTooltip={t('Codes copied!')}
          aria-label={t('Copy selected codes')}
        />
        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                variant='destructive'
                size='icon'
                onClick={() => setShowDeleteInvalidConfirm(true)}
                className='size-8'
                aria-label={t('Delete invalid invite codes')}
                title={t('Delete invalid invite codes')}
              />
            }
          >
            <Trash2 />
            <span className='sr-only'>{t('Delete invalid codes')}</span>
          </TooltipTrigger>
          <TooltipContent>
            <p>{t('Delete invalid codes (used/disabled/expired)')}</p>
          </TooltipContent>
        </Tooltip>
      </BulkActionsToolbar>

      <ConfirmDialog
        destructive
        open={showDeleteInvalidConfirm}
        onOpenChange={setShowDeleteInvalidConfirm}
        handleConfirm={handleDeleteInvalid}
        isLoading={isDeleting}
        className='max-w-md'
        title={t('Delete Invalid Invite Codes?')}
        desc={
          <>
            {t('This will delete all')} <strong>{t('used')}</strong>,{' '}
            <strong>{t('disabled')}</strong>
            {t(', and')} <strong>{t('expired')}</strong> {t('invite codes.')}
            <br />
            {t('This action cannot be undone.')}
          </>
        }
        confirmText={t('Delete Invalid')}
      />
    </>
  )
}

function InviteCodesTable({
  refreshTrigger,
  onEdit,
  onDelete,
  onRefresh,
}: {
  refreshTrigger: number
  onEdit: (inviteCode: InviteCode) => void
  onDelete: (inviteCode: InviteCode) => void
  onRefresh: () => void
}) {
  const { t } = useTranslation()
  const columns = useInviteCodesColumns({ onEdit, onDelete, onRefresh })
  const isMobile = useMediaQuery('(max-width: 640px)')
  const [rowSelection, setRowSelection] = useState({})
  const [sorting, setSorting] = useState<SortingState>([])
  const [columnVisibility, setColumnVisibility] = useState<VisibilityState>({})

  const {
    globalFilter,
    onGlobalFilterChange,
    columnFilters,
    onColumnFiltersChange,
    pagination,
    onPaginationChange,
    ensurePageInRange,
  } = useTableUrlState({
    search: route.useSearch(),
    navigate: route.useNavigate(),
    pagination: { defaultPage: 1, defaultPageSize: isMobile ? 10 : 20 },
    globalFilter: { enabled: true, key: 'filter' },
    columnFilters: [{ columnId: 'status', searchKey: 'status', type: 'array' }],
  })

  const { data, isLoading, isFetching } = useQuery({
    queryKey: [
      'invite-codes',
      pagination.pageIndex + 1,
      pagination.pageSize,
      globalFilter,
      refreshTrigger,
    ],
    queryFn: async () => {
      const hasFilter = globalFilter?.trim()
      const params = {
        p: pagination.pageIndex + 1,
        page_size: pagination.pageSize,
      }
      const result = hasFilter
        ? await searchInviteCodes({ ...params, keyword: globalFilter })
        : await getInviteCodes(params)
      return {
        items: result.data?.items || [],
        total: result.data?.total || 0,
      }
    },
    placeholderData: (previousData) => previousData,
  })

  const inviteCodes = data?.items || []

  const table = useReactTable({
    data: inviteCodes,
    columns,
    state: {
      sorting,
      columnVisibility,
      rowSelection,
      columnFilters,
      globalFilter,
      pagination,
    },
    enableRowSelection: true,
    onRowSelectionChange: setRowSelection,
    onSortingChange: setSorting,
    onColumnVisibilityChange: setColumnVisibility,
    globalFilterFn: (row, _columnId, filterValue) => {
      const name = String(row.getValue('name')).toLowerCase()
      const id = String(row.getValue('id'))
      const code = String(row.getValue('code')).toLowerCase()
      const searchValue = String(filterValue).toLowerCase()
      return (
        name.includes(searchValue) ||
        id.includes(searchValue) ||
        code.includes(searchValue)
      )
    },
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getFacetedRowModel: getFacetedRowModel(),
    getFacetedUniqueValues: getFacetedUniqueValues(),
    onPaginationChange,
    onGlobalFilterChange,
    onColumnFiltersChange,
    manualPagination: !globalFilter,
    pageCount: Math.ceil((data?.total || 0) / pagination.pageSize),
  })

  const pageCount = table.getPageCount()
  useEffect(() => {
    ensurePageInRange(pageCount)
  }, [pageCount, ensurePageInRange])

  const statusOptions = useMemo(() => getInviteCodeStatusOptions(t), [t])

  return (
    <DataTablePage
      table={table}
      columns={columns}
      isLoading={isLoading}
      isFetching={isFetching}
      emptyTitle={t('No Invite Codes Found')}
      emptyDescription={t(
        'No invite codes available. Create your first invite code to get started.'
      )}
      skeletonKeyPrefix='invite-codes-skeleton'
      toolbarProps={{
        searchPlaceholder: t('Filter by name, code or ID...'),
        filters: [
          {
            columnId: 'status',
            title: t('Status'),
            options: statusOptions,
            singleSelect: true,
          },
        ],
      }}
      getRowClassName={(row, { isMobile }) =>
        row.original.status !== INVITE_CODE_STATUS.ENABLED ||
        isInviteCodeExpired(row.original.expired_time, row.original.status)
          ? isMobile
            ? DISABLED_ROW_MOBILE
            : DISABLED_ROW_DESKTOP
          : undefined
      }
      bulkActions={
        <InviteCodesBulkActions table={table} onRefresh={onRefresh} />
      }
    />
  )
}

function InviteCodeMutateDrawer({
  open,
  onOpenChange,
  currentRow,
  onRefresh,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  currentRow: InviteCode | null
  onRefresh: () => void
}) {
  const { t } = useTranslation()
  const isUpdate = !!currentRow
  const [isSubmitting, setIsSubmitting] = useState(false)

  const form = useForm<InviteCodeFormValues>({
    resolver: zodResolver(getInviteCodeFormSchema(t)),
    defaultValues: INVITE_CODE_FORM_DEFAULT_VALUES,
  })

  useEffect(() => {
    if (open && isUpdate && currentRow) {
      getInviteCode(currentRow.id).then((result) => {
        if (result.success && result.data) {
          form.reset(transformInviteCodeToFormDefaults(result.data))
        }
      })
    } else if (open && !isUpdate) {
      form.reset(INVITE_CODE_FORM_DEFAULT_VALUES)
    }
  }, [open, isUpdate, currentRow, form])

  const onSubmit = async (data: InviteCodeFormValues) => {
    setIsSubmitting(true)
    try {
      const basePayload = transformFormDataToPayload(data)
      if (isUpdate && currentRow) {
        const result = await updateInviteCode({
          ...basePayload,
          id: currentRow.id,
        })
        if (result.success) {
          toast.success(t(INVITE_CODE_SUCCESS_MESSAGES.UPDATED))
          onOpenChange(false)
          onRefresh()
        }
      } else {
        const result = await createInviteCode(basePayload)
        if (result.success) {
          const keys = result.keys || result.data || []
          const count = keys.length
          toast.success(
            count > 1
              ? t('Successfully created {{count}} invite codes', { count })
              : t(INVITE_CODE_SUCCESS_MESSAGES.CREATED)
          )
          if (keys.length > 0) {
            downloadTextAsFile(keys.join('\n'), `${data.name}.txt`)
          }
          onOpenChange(false)
          onRefresh()
        }
      }
    } finally {
      setIsSubmitting(false)
    }
  }

  const handleSetExpiry = (months: number, days: number, hours: number) => {
    form.setValue('expired_time', addTimeToDate(months, days, hours))
  }

  return (
    <Sheet
      open={open}
      onOpenChange={(value) => {
        onOpenChange(value)
        if (!value) {
          form.reset()
        }
      }}
    >
      <SheetContent className={sideDrawerContentClassName('sm:max-w-[560px]')}>
        <SheetHeader className={sideDrawerHeaderClassName()}>
          <SheetTitle>
            {isUpdate ? t('Update Invite Code') : t('Create Invite Code')}
          </SheetTitle>
          <SheetDescription>
            {isUpdate
              ? t('Update the invite code by providing necessary info.')
              : t('Add new invite code(s) by providing necessary info.')}{' '}
            {t('Click save when you&apos;re done.')}
          </SheetDescription>
        </SheetHeader>
        <Form {...form}>
          <form
            id='invite-code-form'
            onSubmit={form.handleSubmit(onSubmit)}
            className={sideDrawerFormClassName()}
          >
            <SideDrawerSection>
              <FormField
                control={form.control}
                name='name'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Name')}</FormLabel>
                    <FormControl>
                      <Input {...field} placeholder={t('Enter a name')} />
                    </FormControl>
                    <FormDescription>
                      {t('Name for this invite code (1-20 characters)')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='expired_time'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Expiration Time')}</FormLabel>
                    <div className='flex flex-col gap-2'>
                      <FormControl>
                        <DateTimePicker
                          value={field.value}
                          onChange={field.onChange}
                          placeholder={t('Never expires')}
                        />
                      </FormControl>
                      <div className='grid grid-cols-4 gap-1.5 sm:flex sm:gap-2'>
                        <Button
                          type='button'
                          variant='outline'
                          size='sm'
                          onClick={() => handleSetExpiry(0, 0, 0)}
                        >
                          {t('Never')}
                        </Button>
                        <Button
                          type='button'
                          variant='outline'
                          size='sm'
                          onClick={() => handleSetExpiry(1, 0, 0)}
                        >
                          {t('1M')}
                        </Button>
                        <Button
                          type='button'
                          variant='outline'
                          size='sm'
                          onClick={() => handleSetExpiry(0, 7, 0)}
                        >
                          {t('1W')}
                        </Button>
                        <Button
                          type='button'
                          variant='outline'
                          size='sm'
                          onClick={() => handleSetExpiry(0, 1, 0)}
                        >
                          {t('1 Day')}
                        </Button>
                      </div>
                    </div>
                    <FormDescription>
                      {t('Leave empty for never expires')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {!isUpdate && (
                <>
                  <FormField
                    control={form.control}
                    name='count'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Quantity')}</FormLabel>
                        <FormControl>
                          <Input
                            {...field}
                            type='number'
                            min='1'
                            max='100000'
                            placeholder={t('Number of codes to create')}
                            onChange={(event) =>
                              field.onChange(
                                parseInt(event.target.value, 10) || 1
                              )
                            }
                          />
                        </FormControl>
                        <FormDescription>
                          {t('Create multiple invite codes at once (1-100000)')}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <FormField
                    control={form.control}
                    name='key_prefix'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Key prefix')}</FormLabel>
                        <FormControl>
                          <Input
                            {...field}
                            placeholder={t('Optional invite code prefix')}
                          />
                        </FormControl>
                        <FormDescription>
                          {t(
                            'Generated codes keep 32 characters total and reserve at least 8 random characters.'
                          )}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </>
              )}
            </SideDrawerSection>
          </form>
        </Form>
        <SheetFooter className={sideDrawerFooterClassName()}>
          <SheetClose render={<Button variant='outline' />}>
            {t('Close')}
          </SheetClose>
          <Button form='invite-code-form' type='submit' disabled={isSubmitting}>
            {isSubmitting ? (
              t('Saving...')
            ) : !isUpdate ? (
              <>
                <Download className='mr-2 h-4 w-4' />
                {t('Create and download')}
              </>
            ) : (
              t('Save changes')
            )}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}

function InviteCodeDeleteDialog({
  open,
  onOpenChange,
  currentRow,
  onRefresh,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  currentRow: InviteCode | null
  onRefresh: () => void
}) {
  const { t } = useTranslation()
  const [isDeleting, setIsDeleting] = useState(false)

  const handleDelete = async () => {
    if (!currentRow) return
    setIsDeleting(true)
    try {
      const result = await deleteInviteCode(currentRow.id)
      if (result.success) {
        toast.success(t(INVITE_CODE_SUCCESS_MESSAGES.DELETED))
        onOpenChange(false)
        onRefresh()
      }
    } finally {
      setIsDeleting(false)
    }
  }

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t('Are you sure?')}</AlertDialogTitle>
          <AlertDialogDescription>
            {t('This will permanently delete invite code')}{' '}
            <span className='font-semibold'>{currentRow?.name}</span>
            {t('. This action cannot be undone.')}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={isDeleting}>
            {t('Cancel')}
          </AlertDialogCancel>
          <AlertDialogAction
            onClick={handleDelete}
            disabled={isDeleting}
            className='bg-destructive text-destructive-foreground hover:bg-destructive/90'
          >
            {isDeleting ? t('Deleting...') : t('Delete')}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}

export function InviteCodes() {
  const { t } = useTranslation()
  const [open, setOpen] = useState<InviteCodesDialogType>(null)
  const [currentRow, setCurrentRow] = useState<InviteCode | null>(null)
  const [refreshTrigger, setRefreshTrigger] = useState(0)

  const triggerRefresh = () => setRefreshTrigger((prev) => prev + 1)

  const handleEdit = (inviteCode: InviteCode) => {
    setCurrentRow(inviteCode)
    setOpen('update')
  }

  const handleDelete = (inviteCode: InviteCode) => {
    setCurrentRow(inviteCode)
    setOpen('delete')
  }

  const handleOpenChange = (nextOpen: InviteCodesDialogType) => {
    setOpen(nextOpen)
    if (!nextOpen) {
      setCurrentRow(null)
    }
  }

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>{t('Invite Codes')}</SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <Button size='sm' onClick={() => handleOpenChange('create')}>
            <Plus className='h-4 w-4' />
            {t('Create Code')}
          </Button>
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <InviteCodesTable
            refreshTrigger={refreshTrigger}
            onEdit={handleEdit}
            onDelete={handleDelete}
            onRefresh={triggerRefresh}
          />
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <InviteCodeMutateDrawer
        open={open === 'create' || open === 'update'}
        onOpenChange={(value) => handleOpenChange(value ? open : null)}
        currentRow={open === 'update' ? currentRow : null}
        onRefresh={triggerRefresh}
      />
      <InviteCodeDeleteDialog
        open={open === 'delete'}
        onOpenChange={(value) => handleOpenChange(value ? 'delete' : null)}
        currentRow={currentRow}
        onRefresh={triggerRefresh}
      />
    </>
  )
}
