import type { ComponentChildren } from 'preact'
import { cn } from '@/lib/utils'

function ExpandableRow({
  id,
  expandedId,
  onToggle,
  collapsed,
  expanded,
}: {
  id: string
  expandedId: string | null
  onToggle: (id: string | null) => void
  collapsed: ComponentChildren
  expanded: ComponentChildren
}) {
  const isExpanded = expandedId === id

  return (
    <div className={cn('border-b', isExpanded && 'bg-muted/50')}>
      <button
        type="button"
        className="flex w-full cursor-pointer items-center gap-3 bg-transparent px-4 py-3 text-left"
        aria-expanded={isExpanded}
        onClick={() => onToggle(isExpanded ? null : id)}
      >
        {collapsed}
      </button>
      {isExpanded && <div className="px-4 pb-4">{expanded}</div>}
    </div>
  )
}

export { ExpandableRow }
