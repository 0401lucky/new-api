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
import { useMutation } from '@tanstack/react-query'
import type { Table } from '@tanstack/react-table'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { DataTableBulkActions } from '@/components/data-table'
import { Button } from '@/components/ui/button'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { AuthOperationError } from '@/lib/secure-verification'

import { batchDeleteInactiveUsers } from '../api'
import type { User } from '../types'
import { useUsers } from './users-provider'

export function UserActivityBulkActions(props: {
  table: Table<User>
  disabled: boolean
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const users = useUsers()
  const [pendingUsers, setPendingUsers] = useState<User[]>([])
  const selected = props.table
    .getSelectedRowModel()
    .rows.map((row) => row.original)
  const mutation = useMutation({
    mutationFn: async () => {
      const ids = pendingUsers.map((user) => user.id).sort((a, b) => a - b)
      const proof = await users.requestVerification({
        scope: 'admin.user.batch_delete',
        context: { user_ids: ids },
        title: t('Verify to delete selected users'),
        description: t(
          'Confirm your identity before permanently deleting the {{count}} selected accounts.',
          { count: ids.length }
        ),
      })
      if (!proof) return null
      return batchDeleteInactiveUsers(ids, proof.proof_token)
    },
    onSuccess: (result) => {
      if (!result) return
      toast.success(
        t('Deleted {{count}} users', { count: result.data?.deleted ?? 0 })
      )
      setPendingUsers([])
      props.table.resetRowSelection()
      users.triggerRefresh()
    },
    onError: (error) => {
      handleServerError(AuthOperationError.from(error))
      setPendingUsers([])
      props.table.resetRowSelection()
      users.triggerRefresh()
    },
  })

  return (
    <>
      <DataTableBulkActions
        table={props.table}
        entityName='user'
        placement='inline'
      >
        <Button
          variant='destructive'
          disabled={
            props.disabled ||
            selected.length === 0 ||
            selected.length > 100 ||
            selected.some((user) => !user.cleanup_eligible)
          }
          onClick={() => setPendingUsers(selected)}
        >
          {t('Delete selected users')}
        </Button>
      </DataTableBulkActions>
      <ConfirmDialog
        open={pendingUsers.length > 0 && !users.verificationActive}
        onOpenChange={(open) => {
          if (!open && !mutation.isPending) setPendingUsers([])
        }}
        title={t('Delete {{count}} inactive users?', {
          count: pendingUsers.length,
        })}
        desc={t(
          'These accounts will be permanently deleted and their sessions and API keys revoked. Activity is checked again before deletion. This action cannot be undone.'
        )}
        destructive
        isLoading={mutation.isPending}
        confirmText={
          mutation.isPending ? t('Deleting...') : t('Delete selected users')
        }
        handleConfirm={() => mutation.mutate()}
      >
        <ul
          className='max-h-48 overflow-y-auto rounded-md border p-3 text-sm'
          aria-label={t('Selected users')}
        >
          {pendingUsers.map((user) => (
            <li key={user.id} className='break-all'>
              #{formatNumber(user.id, locale)} · {user.username}
            </li>
          ))}
        </ul>
      </ConfirmDialog>
    </>
  )
}
