import { render, screen } from '@testing-library/preact'
import { describe, expect, it } from 'vitest'
import { AliasesPage } from './aliases'

describe('AliasesPage', () => {
  it('renders', () => {
    render(<AliasesPage />)
    expect(screen.getByText('Aliases')).toBeInTheDocument()
  })
})
