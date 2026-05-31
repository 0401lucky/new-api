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
import { Ban, Loader2, ScanLine, Settings } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { scanBlackroom } from '../api'
import { useBlackroom } from './blackroom-provider'

export function BlackroomPrimaryButtons() {
  const { t } = useTranslation()
  const { setOpen, triggerRefresh } = useBlackroom()
  const [isScanning, setIsScanning] = useState(false)

  const handleScan = async () => {
    setIsScanning(true)
    try {
      const result = await scanBlackroom()
      if (result.success) {
        toast.success(t('Blackroom scan completed'))
        triggerRefresh()
      } else {
        toast.error(result.message || t('Failed to run blackroom scan'))
      }
    } catch (error) {
      toast.error(
        error instanceof Error
          ? error.message
          : t('Failed to run blackroom scan')
      )
    } finally {
      setIsScanning(false)
    }
  }

  return (
    <div className='flex flex-wrap gap-2'>
      <Button
        size='sm'
        variant='outline'
        onClick={handleScan}
        disabled={isScanning}
      >
        {isScanning ? (
          <Loader2 className='h-4 w-4 animate-spin' />
        ) : (
          <ScanLine className='h-4 w-4' />
        )}
        {isScanning ? t('Scanning...') : t('Scan now')}
      </Button>
      <Button size='sm' variant='outline' onClick={() => setOpen('setting')}>
        <Settings className='h-4 w-4' />
        {t('Blackroom settings')}
      </Button>
      <Button size='sm' onClick={() => setOpen('manual-ban')}>
        <Ban className='h-4 w-4' />
        {t('Manual ban')}
      </Button>
    </div>
  )
}
