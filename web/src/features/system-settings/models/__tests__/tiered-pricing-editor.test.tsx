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
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test, vi } from 'vitest'

import { TieredPricingEditor } from '../tiered-pricing-editor'

test('typing tier names retains focus and deleting an earlier tier keeps the remaining price editable', async () => {
  const onBillingExprChange = vi.fn()
  const user = userEvent.setup()
  render(
    <TieredPricingEditor
      modelName='editor-test'
      billingExpr='len <= 200000 ? tier("first", p * 1 + c * 2) : tier("second", p * 3 + c * 4)'
      requestRuleExpr=''
      onBillingExprChange={onBillingExprChange}
      onRequestRuleExprChange={vi.fn()}
    />
  )

  const firstName = screen.getAllByRole('textbox', { name: 'Tier name' })[0]
  await user.type(firstName, '-edited')
  expect(firstName).toHaveFocus()
  expect(firstName).toHaveValue('first-edited')
  await waitFor(() =>
    expect(onBillingExprChange).toHaveBeenLastCalledWith(
      expect.stringContaining('tier("first-edited"')
    )
  )

  // 第一档是一个分支节点，通过行操作菜单移除它。
  await user.click(screen.getByRole('button', { name: 'Branch actions 1' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Remove branch' }))

  await user.click(
    screen.getByRole('button', { name: 'Edit pricing rule second' })
  )
  const secondName = screen.getAllByRole('textbox', { name: 'Tier name' })[0]
  await user.type(secondName, '-kept')
  expect(secondName).toHaveFocus()
  expect(screen.queryByDisplayValue('first-edited')).not.toBeInTheDocument()
  await waitFor(() =>
    expect(onBillingExprChange).toHaveBeenLastCalledWith(
      'tier("second-kept", p * 3 + c * 4)'
    )
  )
})
