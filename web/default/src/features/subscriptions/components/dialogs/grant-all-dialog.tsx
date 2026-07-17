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

import { ConfirmDialog } from '@/components/confirm-dialog'

import { grantPlanToAllUsers } from '../../api'
import { useSubscriptions } from '../subscriptions-provider'

export function GrantAllDialog() {
  const { t } = useTranslation()
  const { open, setOpen, currentRow, triggerRefresh } = useSubscriptions()
  const [granting, setGranting] = useState(false)
  const isOpen = open === 'grant-all'
  const plan = currentRow?.plan
  const planLabel = plan?.title || (plan?.id ? `#${plan.id}` : '-')

  const handleConfirm = async () => {
    if (!plan?.id) return
    setGranting(true)
    try {
      const res = await grantPlanToAllUsers(plan.id)
      if (res.success) {
        toast.success(
          t('Granted to {{granted}} users, skipped {{skipped}}', {
            granted: res.data?.granted_count || 0,
            skipped: res.data?.skipped_count || 0,
          })
        )
        if ((res.data?.failed_count || 0) > 0) {
          toast.warning(
            t('{{count}} users failed to receive the plan', {
              count: res.data?.failed_count || 0,
            })
          )
        }
        triggerRefresh()
        setOpen(null)
      }
    } catch {
      toast.error(t('Operation failed'))
    } finally {
      setGranting(false)
    }
  }

  return (
    <ConfirmDialog
      open={isOpen}
      onOpenChange={(nextOpen) => !nextOpen && setOpen(null)}
      title={t('Grant to all users')}
      desc={t(
        'Grant plan {{plan}} to all enabled users? Users who already have this plan will be skipped.',
        { plan: planLabel }
      )}
      confirmText={t('Grant to all users')}
      handleConfirm={handleConfirm}
      disabled={!plan?.id}
      isLoading={granting}
    />
  )
}
