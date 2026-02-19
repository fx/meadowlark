import { render, screen } from '@testing-library/preact'
import { describe, expect, it } from 'vitest'
import { VoicesPage } from './voices'

describe('VoicesPage', () => {
  it('renders', () => {
    render(<VoicesPage />)
    expect(screen.getByText('Voices')).toBeInTheDocument()
  })
})
