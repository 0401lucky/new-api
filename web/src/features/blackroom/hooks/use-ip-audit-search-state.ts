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
import type { OnChangeFn, PaginationState } from '@tanstack/react-table'
import { useCallback, useMemo } from 'react'

import type { NavigateFn } from '@/hooks/use-table-url-state'

import { BLACKROOM_IP_AUDIT_DEFAULT_PAGE_SIZE } from '../constants'
import type { BlackroomIPAuditSearchParams } from '../types'

/** 0、负数与非有限数都视为「没有设置边界」。 */
function normalizeUnixSeconds(value?: number): number | undefined {
  if (typeof value !== 'number' || !Number.isFinite(value) || value <= 0) {
    return undefined
  }
  return Math.floor(value)
}

function fromUnixSeconds(value?: number): Date | undefined {
  const seconds = normalizeUnixSeconds(value)
  return seconds === undefined ? undefined : new Date(seconds * 1000)
}

function toUnixSeconds(date?: Date): number | undefined {
  if (!date) return undefined
  const milliseconds = date.getTime()
  if (!Number.isFinite(milliseconds)) return undefined
  return Math.floor(milliseconds / 1000)
}

/**
 * 把 IP 审计的筛选与分页状态放进 URL：刷新、分享链接都能还原。
 *
 * 历史记录取舍：翻页是「浏览」动作，会压入历史条目，用户可以直接按返回
 * 键回上一页；改关键词、改时间范围、改每页条数都是「换一个视图」，用
 * replace 覆盖当前条目，否则边打字边筛选会把历史刷满。
 */
export function useBlackroomIPAuditSearchState(params: {
  search: BlackroomIPAuditSearchParams
  navigate: NavigateFn
}) {
  const { search, navigate } = params

  const page = search.ipPage && search.ipPage > 0 ? search.ipPage : 1
  const pageSize =
    search.ipPageSize && search.ipPageSize > 0
      ? search.ipPageSize
      : BLACKROOM_IP_AUDIT_DEFAULT_PAGE_SIZE

  const pagination = useMemo<PaginationState>(
    () => ({ pageIndex: Math.max(0, page - 1), pageSize }),
    [page, pageSize]
  )

  const globalFilter = search.ipFilter ?? ''

  const rangeSeconds = useMemo(
    () => ({
      start: normalizeUnixSeconds(search.ipStart),
      end: normalizeUnixSeconds(search.ipEnd),
    }),
    [search.ipStart, search.ipEnd]
  )

  const range = useMemo(
    () => ({
      start: fromUnixSeconds(rangeSeconds.start),
      end: fromUnixSeconds(rangeSeconds.end),
    }),
    [rangeSeconds]
  )

  const onGlobalFilterChange: OnChangeFn<string> = (updater) => {
    const next = typeof updater === 'function' ? updater(globalFilter) : updater
    const value = next.trim()
    if (value === globalFilter) return

    navigate({
      replace: true,
      search: (previous) => ({
        ...previous,
        ipPage: undefined,
        ipFilter: value ? value : undefined,
      }),
    })
  }

  const onRangeChange = (next: { start?: Date; end?: Date }) => {
    const start = toUnixSeconds(next.start)
    const end = toUnixSeconds(next.end)
    if (start === rangeSeconds.start && end === rangeSeconds.end) return

    navigate({
      replace: true,
      search: (previous) => ({
        ...previous,
        ipPage: undefined,
        ipStart: start,
        ipEnd: end,
      }),
    })
  }

  const onPaginationChange: OnChangeFn<PaginationState> = (updater) => {
    const next = typeof updater === 'function' ? updater(pagination) : updater
    const nextPage = next.pageIndex + 1
    const sizeChanged = next.pageSize !== pagination.pageSize
    if (!sizeChanged && nextPage === page) return

    navigate({
      replace: sizeChanged,
      search: (previous) => ({
        ...previous,
        ipPage: nextPage <= 1 ? undefined : nextPage,
        ipPageSize:
          next.pageSize === BLACKROOM_IP_AUDIT_DEFAULT_PAGE_SIZE
            ? undefined
            : next.pageSize,
      }),
    })
  }

  const ensurePageInRange = useCallback(
    (pageCount: number) => {
      if (pageCount > 0 && page > pageCount) {
        navigate({
          replace: true,
          search: (previous) => ({ ...previous, ipPage: undefined }),
        })
      }
    },
    [navigate, page]
  )

  return {
    pagination,
    globalFilter,
    range,
    onGlobalFilterChange,
    onRangeChange,
    onPaginationChange,
    ensurePageInRange,
  }
}
