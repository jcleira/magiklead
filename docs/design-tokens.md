# MagikLead Design Tokens

## Fonts
- **Sans:** Geist Sans (via `next/font/google`, CSS var `--font-geist-sans`)
- **Mono:** Geist Mono (via `next/font/google`, CSS var `--font-geist-mono`)

## Colors
- **Primary:** slate-900 (`#0f172a`) — buttons, headings, emphasis
- **Text:** slate-900 for headings, slate-600 for body, slate-500 for secondary, slate-400 for muted
- **Background:** white (default), slate-50 (alternating sections)
- **Borders:** slate-200
- **Success:** emerald-600 (checkmarks), emerald-100/700 (badges)
- **Popular badge:** slate-900 bg with white text

## Spacing
- Sections: `py-20` (80px vertical padding)
- Max content width: `max-w-6xl` (1152px) for wide content, `max-w-4xl` (896px) for text-heavy
- Container padding: `px-6`

## Border Radius
- Cards: `rounded-xl` (12px)
- Buttons: `rounded-lg` (8px)
- Badges: `rounded-full`

## Shadows
- Minimal — buttons use `shadow-sm` only. Cards use borders, not shadows.

## Component Patterns
- **Card:** `border border-slate-200 rounded-xl p-6`
- **Highlighted card:** `border-slate-900 ring-1 ring-slate-900`
- **Primary button:** `bg-slate-900 text-white rounded-lg px-6 py-3 text-sm font-semibold hover:bg-slate-800`
- **Secondary button:** `border border-slate-300 text-slate-700 rounded-lg px-6 py-3 text-sm font-semibold hover:bg-slate-50`
- **Nav link:** `text-sm font-medium text-slate-600 hover:text-slate-900`
- **Section heading:** `text-3xl font-bold text-slate-900` (centered)
- **Section subtitle:** `text-slate-500` (centered, `mt-4`)

## Layout Patterns
- Marketing pages: centered content, alternating white/slate-50 sections separated by `border-t border-slate-200`
- App pages: sidebar (240px) + content area with `p-8`
- Responsive: `sm:grid-cols-3` or `sm:grid-cols-4` for card grids, stack on mobile
