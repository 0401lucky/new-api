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

const CHUNK_RELOAD_KEY = 'app:chunk-reload'

const CHUNK_ERROR_PATTERNS = [
  'ChunkLoadError',
  'Loading chunk',
  'Loading CSS chunk',
  'dynamically imported module',
  'error loading dynamically imported module',
  'Importing a module script failed',
  'Unable to preload CSS',
]

function stringifyError(error: unknown): string {
  if (error instanceof Error) {
    return [error.name, error.message, error.stack].filter(Boolean).join('\n')
  }

  if (typeof error === 'string') return error

  if (error && typeof error === 'object') {
    try {
      return JSON.stringify(error)
    } catch {
      return String(error)
    }
  }

  return String(error)
}

function getBuildRevision(): string {
  try {
    return window.__APP_BUILD__?.rev || 'unknown'
  } catch {
    return 'unknown'
  }
}

export function isChunkLoadError(error: unknown): boolean {
  const text = stringifyError(error)
  return CHUNK_ERROR_PATTERNS.some((pattern) =>
    text.toLowerCase().includes(pattern.toLowerCase())
  )
}

export function recoverFromChunkLoadError(error: unknown): boolean {
  if (typeof window === 'undefined') return false
  if (!isChunkLoadError(error)) return false

  const marker = `${getBuildRevision()}:${window.location.pathname}`

  try {
    if (window.sessionStorage.getItem(CHUNK_RELOAD_KEY) === marker) {
      return false
    }
    window.sessionStorage.setItem(CHUNK_RELOAD_KEY, marker)
  } catch {
    return false
  }

  window.location.reload()
  return true
}

let installed = false

export function installChunkLoadRecovery(): void {
  if (installed || typeof window === 'undefined') return
  installed = true

  window.addEventListener('error', (event) => {
    if (recoverFromChunkLoadError(event.error || event.message)) {
      event.preventDefault()
    }
  })

  window.addEventListener('unhandledrejection', (event) => {
    if (recoverFromChunkLoadError(event.reason)) {
      event.preventDefault()
    }
  })
}
