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
import {
  ArrowDown01Icon,
  Copy01Icon,
  Download04Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { ButtonGroup } from '@/components/ui/button-group'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Spinner } from '@/components/ui/spinner'

type CodeCreationActionsProps = {
  formId: string
  disabled?: boolean
  isSubmitting: boolean
  onCreateAndCopy: () => void
}

export function CodeCreationActions({
  formId,
  disabled = false,
  isSubmitting,
  onCreateAndCopy,
}: CodeCreationActionsProps) {
  const { t } = useTranslation()
  const actionsDisabled = disabled || isSubmitting

  return (
    <ButtonGroup>
      <Button form={formId} type='submit' disabled={actionsDisabled}>
        {isSubmitting ? (
          <Spinner />
        ) : (
          <HugeiconsIcon icon={Download04Icon} strokeWidth={2} />
        )}
        {isSubmitting ? t('Saving...') : t('Create and download')}
      </Button>
      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <Button
              type='button'
              size='icon'
              disabled={actionsDisabled}
              aria-label={t('More creation options')}
            />
          }
        >
          <HugeiconsIcon icon={ArrowDown01Icon} strokeWidth={2} />
        </DropdownMenuTrigger>
        <DropdownMenuContent align='end'>
          <DropdownMenuGroup>
            <DropdownMenuItem onClick={onCreateAndCopy}>
              <HugeiconsIcon icon={Copy01Icon} strokeWidth={2} />
              {t('Create and copy')}
            </DropdownMenuItem>
          </DropdownMenuGroup>
        </DropdownMenuContent>
      </DropdownMenu>
    </ButtonGroup>
  )
}
