import { useRef } from 'preact/hooks'

function useFloating(_options?: unknown) {
  const reference = useRef<Element | null>(null)
  const floating = useRef<HTMLElement | null>(null)
  return {
    x: 0,
    y: 0,
    strategy: 'absolute' as const,
    placement: 'bottom' as const,
    middlewareData: {},
    isPositioned: true,
    floatingStyles: { position: 'absolute' as const, top: 0, left: 0 },
    refs: {
      reference,
      floating,
      setReference: (node: Element | null) => {
        reference.current = node
      },
      setFloating: (node: HTMLElement | null) => {
        floating.current = node
      },
    },
    elements: {
      reference: reference.current,
      floating: floating.current,
    },
    update: () => {},
  }
}

function arrow(_options?: unknown) {
  return {
    name: 'arrow',
    fn: () => ({}),
  }
}

function offset(_value?: unknown) {
  return {
    name: 'offset',
    fn: () => ({}),
  }
}

function shift(_options?: unknown) {
  return {
    name: 'shift',
    fn: () => ({}),
  }
}

function flip(_options?: unknown) {
  return {
    name: 'flip',
    fn: () => ({}),
  }
}

function size(_options?: unknown) {
  return {
    name: 'size',
    fn: () => ({}),
  }
}

function hide(_options?: unknown) {
  return {
    name: 'hide',
    fn: () => ({}),
  }
}

function limitShift(_options?: unknown) {
  return () => ({})
}

function autoUpdate(_reference: unknown, _floating: unknown, update: () => void) {
  update()
  return () => {}
}

export { arrow, autoUpdate, flip, hide, limitShift, offset, shift, size, useFloating }
