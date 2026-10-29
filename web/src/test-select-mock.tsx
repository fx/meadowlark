// Stand-in for @/components/ui/select. Radix Select does not open in jsdom, so
// tests drive a select through the onValueChange captured per trigger id.
import type { ComponentChildren } from 'preact'

export const selectCallbacks: Record<string, (value: string) => void> = {}

let currentOnValueChange: ((value: string) => void) | undefined
let currentValue: string | undefined
let currentDisabled: boolean | undefined

export function Select({
  children,
  value,
  onValueChange,
  disabled,
}: {
  children: ComponentChildren
  value?: string
  onValueChange?: (value: string) => void
  disabled?: boolean
}) {
  currentOnValueChange = onValueChange
  currentValue = value
  currentDisabled = disabled
  return <div data-testid="mock-select">{children}</div>
}

export function SelectTrigger({ children, id }: { children: ComponentChildren; id?: string }) {
  if (id && currentOnValueChange) selectCallbacks[id] = currentOnValueChange
  return (
    <div
      data-testid={`select-trigger-${id}`}
      data-value={currentValue}
      data-disabled={currentDisabled ? 'true' : 'false'}
    >
      {children}
    </div>
  )
}

export function SelectValue({ placeholder }: { placeholder?: string }) {
  return <span>{placeholder}</span>
}

export function SelectContent({ children }: { children: ComponentChildren }) {
  return <div>{children}</div>
}

export function SelectItem({ children, value }: { children: ComponentChildren; value: string }) {
  return <option value={value}>{children}</option>
}

export function resetSelectMock() {
  for (const key of Object.keys(selectCallbacks)) delete selectCallbacks[key]
}
