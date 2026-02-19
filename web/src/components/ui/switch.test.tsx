import { render, screen } from '@testing-library/preact'
import { describe, expect, it } from 'vitest'
import { Switch } from './switch'

describe('Switch', () => {
  it('renders a switch', () => {
    render(<Switch aria-label="Toggle" />)
    expect(screen.getByRole('switch')).toBeInTheDocument()
  })

  it('applies custom className', () => {
    render(<Switch className="custom-class" aria-label="Toggle" />)
    expect(screen.getByRole('switch')).toHaveClass('custom-class')
  })

  it('passes through disabled attribute', () => {
    render(<Switch disabled aria-label="Toggle" />)
    expect(screen.getByRole('switch')).toBeDisabled()
  })
})
