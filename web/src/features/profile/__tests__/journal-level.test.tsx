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
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { Profile } from '..'

vi.mock('../hooks', () => ({
  useProfile: () => ({
    profile: null,
    loading: false,
    refreshProfile: vi.fn(),
  }),
}))
vi.mock('@/hooks/use-status', () => ({
  useStatus: () => ({ status: undefined }),
}))
vi.mock('../components/profile-header', () => ({
  ProfileHeader: () => <div>profile header</div>,
}))
vi.mock('../components/profile-settings-card', () => ({
  ProfileSettingsCard: () => <div>profile settings</div>,
}))
vi.mock('../components/language-preferences-card', () => ({
  LanguagePreferencesCard: () => <div>language preferences</div>,
}))
vi.mock('../components/checkin-calendar-card', () => ({
  CheckinCalendarCard: () => <div>checkin calendar</div>,
}))
vi.mock('../components/sidebar-modules-card', () => ({
  SidebarModulesCard: () => <div>sidebar modules</div>,
}))

describe('profile journal level', () => {
  it('marks the custom main container as medium while keeping the cards', () => {
    render(<Profile />)

    expect(screen.getByRole('main')).toHaveAttribute(
      'data-journal-level',
      'medium'
    )
    expect(screen.getByText('profile header')).toBeVisible()
    expect(screen.getByText('profile settings')).toBeVisible()
  })
})
