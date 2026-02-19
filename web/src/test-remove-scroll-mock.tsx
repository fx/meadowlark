import type { ComponentChildren } from 'preact'
import { forwardRef } from 'preact/compat'

type RemoveScrollProps = {
  children?: ComponentChildren
  allowPinchZoom?: boolean
  shards?: unknown[]
  as?: unknown
  forwardProps?: boolean
  enabled?: boolean
  removeScrollBar?: boolean
  inert?: boolean
  noRelative?: boolean
  noIsolation?: boolean
  gapMode?: string
  className?: string
  [key: string]: unknown
}

const RemoveScroll = forwardRef<HTMLDivElement, RemoveScrollProps>(function RemoveScroll(
  { children, className, as: _as, forwardProps: _fp, shards: _s, ...rest },
  ref,
) {
  const {
    allowPinchZoom: _,
    enabled: _e,
    removeScrollBar: _r,
    inert: _i,
    noRelative: _nr,
    noIsolation: _ni,
    gapMode: _g,
    sideCar: _sc,
    ...domProps
  } = rest
  return (
    <div ref={ref} className={className} {...domProps}>
      {children}
    </div>
  )
})

RemoveScroll.classNames = {
  fullWidth: 'remove-scroll-full-width',
  zeroRight: 'remove-scroll-zero-right',
}

export { RemoveScroll }
export default RemoveScroll
