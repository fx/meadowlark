import type { ComponentChildren } from 'preact'
import { useRef } from 'preact/hooks'

type PresenceProps = {
  present: boolean
  children: ComponentChildren | ((props: { present: boolean }) => ComponentChildren)
}

function Presence({ present, children }: PresenceProps) {
  const ref = useRef({ present, animationName: 'none' })
  ref.current.present = present

  if (!present) return null

  if (typeof children === 'function') {
    return <>{children({ present })}</>
  }

  return <>{children}</>
}

function usePresence(present: boolean) {
  return {
    isPresent: present,
    ref: useRef(null),
  }
}

export { Presence, usePresence }
