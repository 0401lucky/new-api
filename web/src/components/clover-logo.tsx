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
import { useId } from 'react'

export type CloverMood = 'normal' | 'happy' | 'sad'

/** Heart-shaped leaf, tip at the origin, body growing along -Y. */
const PETAL =
  'M0 0C-3-10-23-17-23-30-23-40-11-45 0-36 11-45 23-40 23-30 23-17 3-10 0 0Z'

/** The four leaves point their tips at the flower center. */
const PETAL_ANGLES = [-45, 45, 135, 225]

/** Small heart used for the floating decorations and a leaf highlight. */
const HEART =
  'M0 7C-4.4 3.6-6 1.4-6-1 -6-4-3-5 0-2.4 3-5 6-4 6-1 6 1.4 4.4 3.6 0 7Z'

/** Soft four-point sparkle. */
const SPARKLE =
  'M0-7C.7-2.7 2.7-.7 7 0 2.7.7.7 2.7 0 7-.7 2.7-2.7.7-7 0-2.7-.7-.7-2.7 0-7Z'

const FACES = {
  normal: { winking: false, mouth: 'M78.9 41.4Q83 45.8 87.1 41.4' },
  happy: { winking: true, mouth: 'M77.9 41.2Q83 47.4 88.1 41.2' },
  sad: { winking: false, mouth: 'M79.5 44.4Q83 40.8 86.5 44.4' },
} as const

const INK = '#3b4147'
const BLUSH = '#f6a8ba'

/**
 * Brand four-leaf clover: four heart leaves in soft watercolor greens with a
 * hand-drawn ring, floating hearts and sparkles. `mood` lets the mascot react
 * to state (idle / verified / failed).
 */
export function CloverLogo(props: { className?: string; mood?: CloverMood }) {
  const uid = useId()
  const petalId = `clover-petal-${uid}`
  const face = FACES[props.mood ?? 'normal']

  return (
    <svg
      viewBox='0 0 128 128'
      className={props.className}
      data-clover-mood={props.mood ?? 'normal'}
      aria-hidden='true'
      focusable='false'
    >
      <defs>
        <linearGradient id={petalId} x1='0.1' y1='0' x2='0.9' y2='1'>
          <stop offset='0%' stopColor='#e0f2c4' />
          <stop offset='52%' stopColor='#b8df95' />
          <stop offset='100%' stopColor='#8bc86c' />
        </linearGradient>
      </defs>

      {/* Hand-drawn ring: a full circle plus a lighter inner sweep. */}
      <circle
        cx='64'
        cy='64'
        r='58'
        fill='none'
        stroke='#c9e6a2'
        strokeWidth='3'
      />
      <path
        d='M23 93A49 49 0 0 1 37 25'
        fill='none'
        stroke='#dcefc6'
        strokeWidth='2.4'
        strokeLinecap='round'
      />

      {/* Stem, drawn under the leaves so they overlap its top. */}
      <path
        d='M64 88C67 98 65 107 60 111 55.5 115 50 110.5 54 106.5'
        fill='none'
        stroke='#7ab961'
        strokeWidth='5'
        strokeLinecap='round'
      />

      {/* Leaves, highlights and face share one transform so they stay aligned
          while sitting above the stem inside the ring. */}
      <g transform='translate(8.45 1.93) scale(0.8679)'>
        <g
          transform='translate(64 60) scale(1.06)'
          fill={`url(#${petalId})`}
          stroke='#57a052'
          strokeWidth='3'
          strokeLinejoin='round'
        >
          {PETAL_ANGLES.map((angle) => (
            <path
              key={angle}
              d={PETAL}
              transform={`rotate(${angle}) translate(0 -3)`}
            />
          ))}
        </g>

        {/* Glossy highlights. */}
        <g fill='#ffffff'>
          <ellipse
            cx='41'
            cy='33'
            rx='6.6'
            ry='3.5'
            opacity='0.85'
            transform='rotate(-38 41 33)'
          />
          <circle cx='34' cy='44' r='1.8' opacity='0.7' />
          <circle cx='38' cy='48' r='1.1' opacity='0.6' />
          <ellipse
            cx='78'
            cy='25'
            rx='5.2'
            ry='2.8'
            opacity='0.85'
            transform='rotate(32 78 25)'
          />
          <path
            d={HEART}
            opacity='0.9'
            transform='translate(41 84) scale(0.62)'
          />
          <ellipse
            cx='90'
            cy='88'
            rx='4.6'
            ry='2.5'
            opacity='0.8'
            transform='rotate(-32 90 88)'
          />
        </g>

        {/* Kawaii face on the upper-right leaf. */}
        <g>
          <ellipse
            cx='70'
            cy='43'
            rx='4.3'
            ry='2.9'
            fill={BLUSH}
            opacity='0.9'
          />
          <ellipse
            cx='96'
            cy='43'
            rx='4.3'
            ry='2.9'
            fill={BLUSH}
            opacity='0.9'
          />
          {face.winking ? (
            <g fill='none' stroke={INK} strokeWidth='2.4' strokeLinecap='round'>
              <path d='M72.9 38.4Q76.1 34.2 79.3 38.4' />
              <path d='M86.7 38.4Q89.9 34.2 93.1 38.4' />
            </g>
          ) : (
            <g fill={INK}>
              <ellipse cx='76.1' cy='37' rx='2.9' ry='3.7' />
              <ellipse cx='89.9' cy='37' rx='2.9' ry='3.7' />
              <circle cx='75.1' cy='35.4' r='0.95' fill='#ffffff' />
              <circle cx='88.9' cy='35.4' r='0.95' fill='#ffffff' />
            </g>
          )}
          <path
            d={face.mouth}
            fill='none'
            stroke={INK}
            strokeWidth='2.2'
            strokeLinecap='round'
          />
        </g>
      </g>

      {/* Floating decorations in the white gaps. */}
      <g>
        <path
          d={HEART}
          fill='#f9a3b2'
          transform='translate(99 28) scale(0.66)'
        />
        <path
          d={HEART}
          fill='#f9a3b2'
          transform='translate(29 92) scale(0.66)'
        />
        <path
          d={SPARKLE}
          fill='#bde198'
          transform='translate(22 60) scale(0.72)'
        />
        <path
          d={SPARKLE}
          fill='#bde198'
          transform='translate(106 60) scale(0.72)'
        />
        <circle cx='104' cy='40' r='3' fill='#bde198' />
        <circle cx='44' cy='103' r='3' fill='#bde198' />
        <circle cx='76' cy='106' r='3' fill='#bde198' />
      </g>
    </svg>
  )
}
