type BrandMarkProps = {
  className?: string
  label?: string
}

export function BrandMark({ className = '', label }: BrandMarkProps) {
  return (
    <svg
      className={`brand-mark ${className}`.trim()}
      viewBox="0 0 128 128"
      role={label ? 'img' : undefined}
      aria-label={label}
      aria-hidden={label ? undefined : true}
    >
      <defs>
        <linearGradient id="brand-background" x1="12" y1="8" x2="116" y2="120" gradientUnits="userSpaceOnUse">
          <stop stopColor="#174579" />
          <stop offset="1" stopColor="#071a34" />
        </linearGradient>
        <linearGradient id="brand-sync" x1="28" y1="25" x2="101" y2="101" gradientUnits="userSpaceOnUse">
          <stop stopColor="#63e7ff" />
          <stop offset="1" stopColor="#1689e6" />
        </linearGradient>
      </defs>
      <rect x="4" y="4" width="120" height="120" rx="28" fill="url(#brand-background)" />
      <path d="M35 37a39 39 0 0 1 58 5" fill="none" stroke="url(#brand-sync)" strokeWidth="8" strokeLinecap="round" />
      <path d="m91 29 5 16-17-1" fill="none" stroke="#63e7ff" strokeWidth="7" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M93 91a39 39 0 0 1-58-5" fill="none" stroke="url(#brand-sync)" strokeWidth="8" strokeLinecap="round" />
      <path d="m37 99-5-16 17 1" fill="none" stroke="#1689e6" strokeWidth="7" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M64 40 85 52v24L64 88 43 76V52Z" fill="#0b2745" stroke="#83ecff" strokeWidth="4" />
      <circle cx="64" cy="64" r="8" fill="#32d8ff" />
      <circle cx="52" cy="57" r="3" fill="#dffaff" />
      <circle cx="76" cy="57" r="3" fill="#dffaff" />
      <path d="M52 73h24" fill="none" stroke="#dffaff" strokeWidth="4" strokeLinecap="round" />
    </svg>
  )
}
