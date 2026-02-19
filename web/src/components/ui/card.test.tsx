import { render, screen } from '@testing-library/preact'
import { describe, expect, it } from 'vitest'
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from './card'

describe('Card', () => {
  it('renders card with content', () => {
    render(<Card data-testid="card">Content</Card>)
    expect(screen.getByTestId('card')).toBeInTheDocument()
    expect(screen.getByTestId('card').className).toContain('bg-card')
  })

  it('applies custom className', () => {
    render(
      <Card className="custom-class" data-testid="card">
        Test
      </Card>,
    )
    expect(screen.getByTestId('card')).toHaveClass('custom-class')
  })
})

describe('CardHeader', () => {
  it('renders with className', () => {
    render(
      <CardHeader className="custom" data-testid="header">
        Header
      </CardHeader>,
    )
    expect(screen.getByTestId('header')).toHaveClass('custom')
  })
})

describe('CardTitle', () => {
  it('renders title text', () => {
    render(<CardTitle>Title</CardTitle>)
    expect(screen.getByText('Title')).toBeInTheDocument()
    expect(screen.getByText('Title').className).toContain('font-semibold')
  })

  it('applies custom className', () => {
    render(<CardTitle className="custom">Title</CardTitle>)
    expect(screen.getByText('Title')).toHaveClass('custom')
  })
})

describe('CardDescription', () => {
  it('renders description text', () => {
    render(<CardDescription>Desc</CardDescription>)
    expect(screen.getByText('Desc')).toBeInTheDocument()
    expect(screen.getByText('Desc').className).toContain('text-muted-foreground')
  })

  it('applies custom className', () => {
    render(<CardDescription className="custom">Desc</CardDescription>)
    expect(screen.getByText('Desc')).toHaveClass('custom')
  })
})

describe('CardContent', () => {
  it('renders with className', () => {
    render(
      <CardContent className="custom" data-testid="content">
        Body
      </CardContent>,
    )
    expect(screen.getByTestId('content')).toHaveClass('custom')
  })
})

describe('CardFooter', () => {
  it('renders with className', () => {
    render(
      <CardFooter className="custom" data-testid="footer">
        Footer
      </CardFooter>,
    )
    expect(screen.getByTestId('footer')).toHaveClass('custom')
  })
})
