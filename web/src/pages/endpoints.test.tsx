import { render, screen } from '@testing-library/preact'
import { describe, expect, it } from 'vitest'
import { EndpointsPage } from './endpoints'

describe('EndpointsPage', () => {
  it('renders', () => {
    render(<EndpointsPage />)
    expect(screen.getByText('Endpoints')).toBeInTheDocument()
  })
})
