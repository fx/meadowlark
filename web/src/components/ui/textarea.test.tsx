import { render, screen } from '@testing-library/preact'
import { describe, expect, it } from 'vitest'
import { Textarea } from './textarea'

describe('Textarea', () => {
  it('renders a textarea element', () => {
    render(<Textarea placeholder="Enter text" />)
    expect(screen.getByPlaceholderText('Enter text')).toBeInTheDocument()
  })

  it('applies custom className', () => {
    render(<Textarea className="custom-class" data-testid="textarea" />)
    expect(screen.getByTestId('textarea')).toHaveClass('custom-class')
  })

  it('passes through disabled attribute', () => {
    render(<Textarea disabled data-testid="textarea" />)
    expect(screen.getByTestId('textarea')).toBeDisabled()
  })
})
