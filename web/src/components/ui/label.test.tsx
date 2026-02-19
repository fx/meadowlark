import { render, screen } from '@testing-library/preact'
import { describe, expect, it } from 'vitest'
import { Label } from './label'

describe('Label', () => {
  it('renders a label element', () => {
    render(<Label>Username</Label>)
    expect(screen.getByText('Username')).toBeInTheDocument()
  })

  it('applies custom className', () => {
    render(<Label className="custom-class">Test</Label>)
    expect(screen.getByText('Test')).toHaveClass('custom-class')
  })

  it('passes through htmlFor attribute', () => {
    render(<Label htmlFor="email">Email</Label>)
    expect(screen.getByText('Email')).toHaveAttribute('for', 'email')
  })
})
