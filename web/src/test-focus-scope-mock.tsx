import type { ComponentProps } from 'preact'
import { forwardRef } from 'preact/compat'
import { useEffect } from 'preact/hooks'

type FocusScopeProps = ComponentProps<'div'> & {
  loop?: boolean
  trapped?: boolean
  onMountAutoFocus?: (event: Event) => void
  onUnmountAutoFocus?: (event: Event) => void
  asChild?: boolean
}

const FocusScope = forwardRef<HTMLDivElement, FocusScopeProps>(function FocusScope(
  { loop: _, trapped: _t, onMountAutoFocus, onUnmountAutoFocus: _u, asChild: _a, ...props },
  ref,
) {
  useEffect(() => {
    if (onMountAutoFocus) {
      onMountAutoFocus(new Event('focusScope.autoFocus', { cancelable: true }))
    }
  }, [onMountAutoFocus])

  return <div ref={ref} {...props} />
})

export { FocusScope }
