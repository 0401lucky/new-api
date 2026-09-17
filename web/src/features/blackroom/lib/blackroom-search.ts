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
import { z } from 'zod'

import {
  BLACKROOM_IP_AUDIT_DEFAULT_PAGE_SIZE,
  BLACKROOM_IP_AUDIT_MAX_PAGE_SIZE,
  BLACKROOM_SOURCE_VALUES,
  BLACKROOM_STATUS_VALUES,
  BLACKROOM_TAB_VALUES,
} from '../constants'

/**
 * 小黑屋页面的 URL 查询参数。封禁记录与 IP 审计共用同一个路由，两者的
 * 分页/筛选各自独立，IP 审计用 `ip` 前缀，避免互相覆盖。
 *
 * 每个字段都是「可选 + catch 兜底」：URL 是用户可编辑的，任何非法值都
 * 必须退化成默认值，不能让手改出来的 `?ipStart=abc` 把页面打崩。
 */
export const blackroomSearchSchema = z.object({
  page: z.number().optional().catch(1),
  pageSize: z.number().optional().catch(10),
  filter: z.string().optional().catch(''),
  status: z.array(z.enum(BLACKROOM_STATUS_VALUES)).optional().catch([]),
  source: z.array(z.enum(BLACKROOM_SOURCE_VALUES)).optional().catch([]),
  tab: z.enum(BLACKROOM_TAB_VALUES).optional().catch('bans'),
  ipPage: z.number().int().min(1).optional().catch(1),
  ipPageSize: z
    .number()
    .int()
    .min(1)
    .max(BLACKROOM_IP_AUDIT_MAX_PAGE_SIZE)
    .optional()
    .catch(BLACKROOM_IP_AUDIT_DEFAULT_PAGE_SIZE),
  ipFilter: z.string().optional().catch(''),
  /** Unix 秒；0 与缺省同义，表示不设下界/上界。 */
  ipStart: z.number().int().nonnegative().optional().catch(0),
  ipEnd: z.number().int().nonnegative().optional().catch(0),
})

export type BlackroomSearch = z.infer<typeof blackroomSearchSchema>
