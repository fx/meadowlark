import { render, screen } from '@testing-library/preact'
import { describe, expect, it } from 'vitest'
import { SettingsPage } from './settings'

describe('SettingsPage', () => {
  it('renders', () => {
    render(<SettingsPage />)
    expect(screen.getByText('Settings')).toBeInTheDocument()
  })
})
