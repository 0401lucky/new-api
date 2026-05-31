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
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Textarea } from '@/components/ui/textarea'
import { ConfirmDialog } from '@/components/confirm-dialog'
import { releaseBlackroomEntry } from '../api'
import { useBlackroom } from './blackroom-provider'

export function ReleaseDialog() {
  const { t } = useTranslation()
  const { open, setOpen, currentRow, triggerRefresh } = useBlackroom()
  const [reason, setReason] = useState('')
  const [isSubmitting, setIsSubmitting] = useState(false)
  const isOpen = open === 'release'

  const handleConfirm = async () => {
    if (!currentRow) return
    setIsSubmitting(true)
    try {
      const result = await releaseBlackroomEntry(currentRow.id, {
        reason: reason.trim(),
      })
      if (result.success) {
        toast.success(t('User released successfully'))
        setOpen(null)
        setReason('')
        triggerRefresh()
      } else {
        toast.error(result.message || t('Failed to release user'))
      }
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <ConfirmDialog
      open={isOpen}
      onOpenChange={(value) => {
        if (!value) {
          setOpen(null)
          setReason('')
        }
      }}
      title={t('Release user')}
      desc={t('This user will be removed from the blackroom immediately.')}
      confirmText={isSubmitting ? t('Saving...') : t('Release')}
      disabled={isSubmitting}
      isLoading={isSubmitting}
      handleConfirm={handleConfirm}
    >
      <Textarea
        value={reason}
        onChange={(event) => setReason(event.target.value)}
        rows={3}
        placeholder={t('Enter release reason')}
      />
    </ConfirmDialog>
  )
}
