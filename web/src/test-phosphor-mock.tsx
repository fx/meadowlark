import type { ComponentProps } from 'preact'

function createIcon(name: string) {
  return function Icon(props: ComponentProps<'svg'>) {
    return <svg data-testid={`icon-${name}`} {...props} />
  }
}

export const X = createIcon('x')
export const Check = createIcon('check')
export const Circle = createIcon('circle')
export const CaretDown = createIcon('caret-down')
export const CaretUp = createIcon('caret-up')
export const Sun = createIcon('sun')
export const Moon = createIcon('moon')
export const Monitor = createIcon('monitor')
export const PlugsConnected = createIcon('plugs-connected')
export const SpeakerHigh = createIcon('speaker-high')
export const TagSimple = createIcon('tag-simple')
export const GearSix = createIcon('gear-six')
export const List = createIcon('list')
export const Lightning = createIcon('lightning')
export const MagnifyingGlass = createIcon('magnifying-glass')
export const Trash = createIcon('trash')
export const Play = createIcon('play')
export const Eye = createIcon('eye')
export const EyeSlash = createIcon('eye-slash')
