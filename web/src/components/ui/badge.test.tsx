import { render, screen } from '@testing-library/preact'
import { describe, expect, it } from 'vitest'
import { Badge } from './badge'

describe('Badge', () => {
  it('renders with default variant', () => {
    render(<Badge>New</Badge>)
    const badge = screen.getByText('New')
    expect(badge).toBeInTheDocument()
    expect(badge.className).toContain('bg-primary')
  })

  it('applies custom className', () => {
    render(<Badge className="custom-class">Test</Badge>)
    expect(screen.getByText('Test')).toHaveClass('custom-class')
  })

  it('renders secondary variant', () => {
    render(<Badge variant="secondary">Secondary</Badge>)
    expect(screen.getByText('Secondary').className).toContain('bg-secondary')
  })

  it('renders destructive variant', () => {
    render(<Badge variant="destructive">Error</Badge>)
    expect(screen.getByText('Error').className).toContain('bg-destructive')
  })

  it('renders outline variant', () => {
    render(<Badge variant="outline">Outline</Badge>)
    expect(screen.getByText('Outline').className).toContain('text-foreground')
  })
})
