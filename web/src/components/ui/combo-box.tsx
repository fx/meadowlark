import { Input } from '@/components/ui/input'

type ComboBoxOption = {
  value: string
  label: string
}

type ComboBoxProps = {
  value: string
  onChange: (value: string) => void
  options: ComboBoxOption[]
  loading?: boolean
  placeholder?: string
  disabled?: boolean
  id: string
  required?: boolean
}

function ComboBox({
  value,
  onChange,
  options,
  loading = false,
  placeholder,
  disabled,
  id,
  required,
}: ComboBoxProps) {
  const listId = `${id}-list`

  return (
    <div className="relative">
      <Input
        id={id}
        list={listId}
        value={value}
        onInput={(e) => onChange((e.target as HTMLInputElement).value)}
        placeholder={loading ? 'Loading...' : placeholder}
        disabled={disabled}
        required={required}
        autoComplete="off"
      />
      <datalist id={listId}>
        {options.map((opt) => (
          <option key={opt.value} value={opt.value}>
            {opt.label}
          </option>
        ))}
      </datalist>
      {loading && (
        <output className="absolute right-2 top-1/2 -translate-y-1/2" aria-label="Loading options">
          <div className="h-4 w-4 animate-spin border-2 border-muted-foreground border-t-transparent" />
        </output>
      )}
    </div>
  )
}

export { ComboBox }
export type { ComboBoxOption, ComboBoxProps }
