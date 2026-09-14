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
import { useTranslation } from 'react-i18next'

export function DonationRiskNotice() {
  const { t } = useTranslation()
  return (
    <p className='text-muted-foreground text-xs leading-relaxed break-words'>
      {t(
        'Do not use API keys from your primary account. Usage after donation may trigger platform risk controls or other restrictions, account suspension or bans, or key invalidation. Make sure you can accept these risks.'
      )}
    </p>
  )
}
