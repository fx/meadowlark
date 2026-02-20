import { CaretDown, Check } from '@phosphor-icons/react'
import { useCallback, useMemo, useRef, useState } from 'preact/hooks'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { cn } from '@/lib/utils'

type ComboboxOption = {
  value: string
  label: string
}

type ComboboxProps = {
  value: string
  onChange: (value: string) => void
  options: ComboboxOption[]
  placeholder?: string
  disabled?: boolean
  id?: string
  required?: boolean
  loading?: boolean
  className?: string
}

function Combobox({
  value,
  onChange,
  options,
  placeholder,
  disabled,
  id,
  required,
  loading,
  className,
}: ComboboxProps) {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const inputRef = useRef<HTMLInputElement>(null)

  const filtered = useMemo(() => {
    if (!query) return options
    const q = query.toLowerCase()
    return options.filter(
      (o) => o.value.toLowerCase().includes(q) || o.label.toLowerCase().includes(q),
    )
  }, [options, query])

  const handleSelect = useCallback(
    (optionValue: string) => {
      onChange(optionValue)
      setQuery('')
      setOpen(false)
    },
    [onChange],
  )

  const handleInputChange = (e: Event) => {
    const v = (e.target as HTMLInputElement).value
    setQuery(v)
    onChange(v)
    if (!open) setOpen(true)
  }

  const handleKeyDown = (e: KeyboardEvent) => {
    if (e.key === 'Escape') {
      setOpen(false)
    }
  }

  const showDropdown = options.length > 0

  return (
    <Popover open={open && showDropdown} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <div
          className={cn(
            'border-input dark:bg-input/30 flex h-9 w-full items-center rounded-md border bg-transparent shadow-xs transition-[color,box-shadow]',
            'focus-within:border-ring focus-within:ring-ring/50 focus-within:ring-[3px]',
            disabled && 'pointer-events-none cursor-not-allowed opacity-50',
            className,
          )}
        >
          <input
            ref={inputRef}
            id={id}
            value={value}
            onInput={handleInputChange}
            onFocus={() => showDropdown && setOpen(true)}
            onKeyDown={handleKeyDown}
            placeholder={loading ? 'Loading...' : placeholder}
            disabled={disabled}
            required={required}
            autoComplete="off"
            className="h-full w-full min-w-0 bg-transparent px-3 py-1 text-base outline-none placeholder:text-muted-foreground md:text-sm"
          />
          {showDropdown && (
            <button
              type="button"
              tabIndex={-1}
              className="flex h-full shrink-0 items-center px-2 text-muted-foreground"
              onClick={() => {
                setOpen(!open)
                inputRef.current?.focus()
              }}
              aria-label="Toggle options"
            >
              <CaretDown className="size-4" />
            </button>
          )}
          {loading && (
            <div className="flex h-full shrink-0 items-center pr-2">
              <div className="h-4 w-4 animate-spin border-2 border-muted-foreground border-t-transparent rounded-full" />
            </div>
          )}
        </div>
      </PopoverTrigger>
      <PopoverContent
        className="w-[var(--radix-popover-trigger-width)] p-1"
        align="start"
        onOpenAutoFocus={(e: Event) => e.preventDefault()}
      >
        <div className="max-h-60 overflow-y-auto">
          {filtered.length === 0 ? (
            <div className="px-2 py-1.5 text-sm text-muted-foreground">No matches</div>
          ) : (
            filtered.map((opt) => (
              <button
                key={opt.value}
                type="button"
                className={cn(
                  'relative flex w-full cursor-default items-center gap-2 rounded-sm py-1.5 pr-8 pl-2 text-sm outline-hidden select-none',
                  'hover:bg-accent hover:text-accent-foreground',
                  opt.value === value && 'bg-accent text-accent-foreground',
                )}
                onClick={() => handleSelect(opt.value)}
              >
                {opt.label}
                {opt.value === value && (
                  <span className="absolute right-2 flex size-3.5 items-center justify-center">
                    <Check className="size-4" />
                  </span>
                )}
              </button>
            ))
          )}
        </div>
      </PopoverContent>
    </Popover>
  )
}

export { Combobox }
export type { ComboboxOption, ComboboxProps }
