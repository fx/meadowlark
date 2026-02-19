import { render, screen } from '@testing-library/preact'
import { describe, expect, it } from 'vitest'
import { Button } from './button'

describe('Button', () => {
  it('renders with default variant and size', () => {
    render(<Button>Click me</Button>)
    const button = screen.getByRole('button', { name: 'Click me' })
    expect(button).toBeInTheDocument()
    expect(button.className).toContain('bg-primary')
    expect(button.className).toContain('h-9')
  })

  it('applies custom className', () => {
    render(<Button className="custom-class">Test</Button>)
    expect(screen.getByRole('button')).toHaveClass('custom-class')
  })

  it('renders destructive variant', () => {
    render(<Button variant="destructive">Delete</Button>)
    expect(screen.getByRole('button').className).toContain('bg-destructive')
  })

  it('renders outline variant', () => {
    render(<Button variant="outline">Outline</Button>)
    expect(screen.getByRole('button').className).toContain('border')
  })

  it('renders secondary variant', () => {
    render(<Button variant="secondary">Secondary</Button>)
    expect(screen.getByRole('button').className).toContain('bg-secondary')
  })

  it('renders ghost variant', () => {
    render(<Button variant="ghost">Ghost</Button>)
    expect(screen.getByRole('button').className).toContain('hover:bg-accent')
  })

  it('renders link variant', () => {
    render(<Button variant="link">Link</Button>)
    expect(screen.getByRole('button').className).toContain('underline-offset-4')
  })

  it('renders sm size', () => {
    render(<Button size="sm">Small</Button>)
    expect(screen.getByRole('button').className).toContain('h-8')
  })

  it('renders xs size', () => {
    render(<Button size="xs">XSmall</Button>)
    expect(screen.getByRole('button').className).toContain('h-6')
  })

  it('renders lg size', () => {
    render(<Button size="lg">Large</Button>)
    expect(screen.getByRole('button').className).toContain('h-10')
  })

  it('renders icon size', () => {
    render(<Button size="icon">I</Button>)
    expect(screen.getByRole('button').className).toContain('w-9')
  })

  it('renders as child when asChild is true', () => {
    render(
      <Button asChild>
        <a href="/test">Link Button</a>
      </Button>,
    )
    const link = screen.getByRole('link', { name: 'Link Button' })
    expect(link).toBeInTheDocument()
    expect(link.className).toContain('bg-primary')
  })

  it('passes through HTML attributes', () => {
    render(<Button disabled>Disabled</Button>)
    expect(screen.getByRole('button')).toBeDisabled()
  })
})
