import { render, screen } from '@testing-library/preact'
import { describe, expect, it } from 'vitest'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from './tooltip'

describe('Tooltip', () => {
  it('renders trigger', () => {
    render(
      <TooltipProvider>
        <Tooltip>
          <TooltipTrigger>Hover me</TooltipTrigger>
          <TooltipContent>Tooltip text</TooltipContent>
        </Tooltip>
      </TooltipProvider>,
    )
    expect(screen.getByText('Hover me')).toBeInTheDocument()
  })

  it('applies custom className to trigger', () => {
    render(
      <TooltipProvider>
        <Tooltip>
          <TooltipTrigger className="custom">Hover</TooltipTrigger>
          <TooltipContent className="content-class">Text</TooltipContent>
        </Tooltip>
      </TooltipProvider>,
    )
    expect(screen.getByText('Hover')).toHaveClass('custom')
  })
})
