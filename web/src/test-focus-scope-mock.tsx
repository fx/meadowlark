import type { ComponentProps } from 'preact'
import { forwardRef } from 'preact/compat'

type FocusScopeProps = ComponentProps<'div'> & {
  loop?: boolean
  trapped?: boolean
  onMountAutoFocus?: (event: Event) => void
  onUnmountAutoFocus?: (event: Event) => void
  asChild?: boolean
}

const FocusScope = forwardRef<HTMLDivElement, FocusScopeProps>(function FocusScope(
  { loop: _, trapped: _t, onMountAutoFocus: _m, onUnmountAutoFocus: _u, asChild: _a, ...props },
  ref,
) {
  return <div ref={ref} {...props} />
})

export { FocusScope }
