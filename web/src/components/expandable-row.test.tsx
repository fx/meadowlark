import { render, screen } from '@testing-library/preact'
import userEvent from '@testing-library/user-event'
import { useState } from 'preact/hooks'
import { describe, expect, it, vi } from 'vitest'
import { ExpandableRow } from './expandable-row'

function TestHarness({ initialExpanded = null }: { initialExpanded?: string | null }) {
  const [expandedId, setExpandedId] = useState<string | null>(initialExpanded)
  return (
    <div>
      <ExpandableRow
        id="row-1"
        expandedId={expandedId}
        onToggle={setExpandedId}
        collapsed={<span>Row 1 Summary</span>}
        expanded={<span>Row 1 Details</span>}
      />
      <ExpandableRow
        id="row-2"
        expandedId={expandedId}
        onToggle={setExpandedId}
        collapsed={<span>Row 2 Summary</span>}
        expanded={<span>Row 2 Details</span>}
      />
    </div>
  )
}

describe('ExpandableRow', () => {
  it('renders collapsed content', () => {
    render(<TestHarness />)
    expect(screen.getByText('Row 1 Summary')).toBeInTheDocument()
    expect(screen.getByText('Row 2 Summary')).toBeInTheDocument()
  })

  it('does not show expanded content by default', () => {
    render(<TestHarness />)
    expect(screen.queryByText('Row 1 Details')).not.toBeInTheDocument()
    expect(screen.queryByText('Row 2 Details')).not.toBeInTheDocument()
  })

  it('expands on click', async () => {
    const user = userEvent.setup()
    render(<TestHarness />)
    await user.click(screen.getByText('Row 1 Summary'))
    expect(screen.getByText('Row 1 Details')).toBeInTheDocument()
  })

  it('collapses on second click', async () => {
    const user = userEvent.setup()
    render(<TestHarness />)
    await user.click(screen.getByText('Row 1 Summary'))
    expect(screen.getByText('Row 1 Details')).toBeInTheDocument()
    await user.click(screen.getByText('Row 1 Summary'))
    expect(screen.queryByText('Row 1 Details')).not.toBeInTheDocument()
  })

  it('only one row expanded at a time', async () => {
    const user = userEvent.setup()
    render(<TestHarness />)
    await user.click(screen.getByText('Row 1 Summary'))
    expect(screen.getByText('Row 1 Details')).toBeInTheDocument()
    await user.click(screen.getByText('Row 2 Summary'))
    expect(screen.queryByText('Row 1 Details')).not.toBeInTheDocument()
    expect(screen.getByText('Row 2 Details')).toBeInTheDocument()
  })

  it('sets aria-expanded correctly', async () => {
    const user = userEvent.setup()
    render(<TestHarness />)
    const row1 = screen.getByRole('button', { name: /Row 1 Summary/ })
    expect(row1).toHaveAttribute('aria-expanded', 'false')
    await user.click(row1)
    expect(row1).toHaveAttribute('aria-expanded', 'true')
  })

  it('supports create mode with new id', () => {
    render(<TestHarness initialExpanded="new" />)
    // Neither row-1 nor row-2 are expanded when 'new' is the expandedId
    expect(screen.queryByText('Row 1 Details')).not.toBeInTheDocument()
    expect(screen.queryByText('Row 2 Details')).not.toBeInTheDocument()
  })

  it('calls onToggle with null when collapsing', async () => {
    const user = userEvent.setup()
    const onToggle = vi.fn()
    render(
      <ExpandableRow
        id="test"
        expandedId="test"
        onToggle={onToggle}
        collapsed={<span>Summary</span>}
        expanded={<span>Details</span>}
      />,
    )
    await user.click(screen.getByRole('button', { name: /Summary/ }))
    expect(onToggle).toHaveBeenCalledWith(null)
  })

  it('calls onToggle with id when expanding', async () => {
    const user = userEvent.setup()
    const onToggle = vi.fn()
    render(
      <ExpandableRow
        id="test"
        expandedId={null}
        onToggle={onToggle}
        collapsed={<span>Summary</span>}
        expanded={<span>Details</span>}
      />,
    )
    await user.click(screen.getByRole('button', { name: /Summary/ }))
    expect(onToggle).toHaveBeenCalledWith('test')
  })
})
