import { render, screen } from '@testing-library/preact'
import { describe, expect, it } from 'vitest'
import { App } from './app'

describe('App', () => {
  it('renders Meadowlark heading', () => {
    render(<App />)
    expect(screen.getByText('Meadowlark')).toBeInTheDocument()
  })
})
