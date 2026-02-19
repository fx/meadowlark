import { render, screen } from '@testing-library/preact'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import { Popover, PopoverAnchor, PopoverContent, PopoverTrigger } from './popover'

describe('Popover', () => {
  it('renders trigger and opens popover', async () => {
    const user = userEvent.setup()
    render(
      <Popover>
        <PopoverTrigger>Open Popover</PopoverTrigger>
        <PopoverContent>Popover Content</PopoverContent>
      </Popover>,
    )
    expect(screen.getByText('Open Popover')).toBeInTheDocument()
    expect(screen.queryByText('Popover Content')).not.toBeInTheDocument()
    await user.click(screen.getByText('Open Popover'))
    expect(screen.getByText('Popover Content')).toBeInTheDocument()
  })

  it('applies custom className to content', async () => {
    const user = userEvent.setup()
    render(
      <Popover>
        <PopoverTrigger>Open</PopoverTrigger>
        <PopoverContent className="custom-popover">Content</PopoverContent>
      </Popover>,
    )
    await user.click(screen.getByText('Open'))
    expect(screen.getByText('Content')).toHaveClass('custom-popover')
  })

  it('renders anchor element', () => {
    render(
      <Popover>
        <PopoverAnchor data-testid="anchor" />
        <PopoverTrigger>Open</PopoverTrigger>
        <PopoverContent>Content</PopoverContent>
      </Popover>,
    )
    expect(screen.getByTestId('anchor')).toBeInTheDocument()
  })
})
