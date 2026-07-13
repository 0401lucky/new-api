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
import { Plus, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Button } from '@/components/ui/button'

import { deleteInvalidRedemptions, deleteValidRedemptions } from '../api'
import { ERROR_MESSAGES } from '../constants'
import { useRedemptions } from './redemptions-provider'

export function RedemptionsPrimaryButtons() {
  const { t } = useTranslation()
  const { setOpen, triggerRefresh } = useRedemptions()
  const [showDeleteInvalidConfirm, setShowDeleteInvalidConfirm] =
    useState(false)
  const [showDeleteValidConfirm, setShowDeleteValidConfirm] = useState(false)
  const [isDeletingInvalid, setIsDeletingInvalid] = useState(false)
  const [isDeletingValid, setIsDeletingValid] = useState(false)

  const handleDeleteInvalid = async () => {
    setIsDeletingInvalid(true)
    try {
      const result = await deleteInvalidRedemptions()
      if (result.success) {
        const count = result.data || 0
        toast.success(
          t('Successfully deleted {{count}} invalid redemption codes', {
            count,
          })
        )
        triggerRefresh()
        setShowDeleteInvalidConfirm(false)
      } else {
        toast.error(result.message || t(ERROR_MESSAGES.DELETE_INVALID_FAILED))
      }
    } finally {
      setIsDeletingInvalid(false)
    }
  }

  const handleDeleteValid = async () => {
    setIsDeletingValid(true)
    try {
      const result = await deleteValidRedemptions()
      if (result.success) {
        const count = result.data || 0
        toast.success(
          t('Successfully deleted {{count}} valid redemption codes', {
            count,
          })
        )
        triggerRefresh()
        setShowDeleteValidConfirm(false)
      } else {
        toast.error(result.message || t(ERROR_MESSAGES.DELETE_VALID_FAILED))
      }
    } finally {
      setIsDeletingValid(false)
    }
  }

  return (
    <>
      <div className='flex flex-wrap gap-2'>
        <Button
          size='sm'
          variant='outline'
          onClick={() => setShowDeleteInvalidConfirm(true)}
          disabled={isDeletingInvalid || isDeletingValid}
        >
          <Trash2 data-icon='inline-start' />
          {t('Delete Invalid')}
        </Button>
        <Button
          size='sm'
          variant='destructive'
          onClick={() => setShowDeleteValidConfirm(true)}
          disabled={isDeletingInvalid || isDeletingValid}
        >
          <Trash2 data-icon='inline-start' />
          {t('Delete Valid')}
        </Button>
        <Button size='sm' onClick={() => setOpen('create')}>
          <Plus data-icon='inline-start' />
          {t('Create Code')}
        </Button>
      </div>

      <ConfirmDialog
        destructive
        open={showDeleteInvalidConfirm}
        onOpenChange={setShowDeleteInvalidConfirm}
        handleConfirm={handleDeleteInvalid}
        isLoading={isDeletingInvalid}
        className='max-w-md'
        title={t('Delete Invalid Redemption Codes?')}
        desc={
          <>
            {t('This will delete all')} <strong>{t('used')}</strong>,{' '}
            <strong>{t('disabled')}</strong>
            {t(', and')} <strong>{t('expired')}</strong>{' '}
            {t('redemption codes.')}
            <br />
            {t('This action cannot be undone.')}
          </>
        }
        confirmText={t('Delete Invalid')}
      />

      <ConfirmDialog
        destructive
        open={showDeleteValidConfirm}
        onOpenChange={setShowDeleteValidConfirm}
        handleConfirm={handleDeleteValid}
        isLoading={isDeletingValid}
        className='max-w-md'
        title={t('Delete Valid Redemption Codes?')}
        desc={
          <>
            {t(
              'This will delete all enabled redemption codes that have not expired, including codes that never expire.'
            )}
            <br />
            {t(
              'This applies to all matching redemption codes and is not limited by the current search or page.'
            )}
            <br />
            {t('This action cannot be undone.')}
          </>
        }
        confirmText={t('Delete Valid')}
      />
    </>
  )
}
