import { render, screen } from '@testing-library/preact'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { ComboBox } from './combo-box'

const options = [
  { value: 'tts-1', label: 'TTS-1' },
  { value: 'tts-1-hd', label: 'TTS-1 HD' },
]

describe('ComboBox', () => {
  it('renders an input with datalist', () => {
    render(
      <ComboBox id="test" value="" onChange={vi.fn()} options={options} placeholder="Pick one" />,
    )
    const input = screen.getByPlaceholderText('Pick one')
    expect(input).toBeInTheDocument()
    expect(input).toHaveAttribute('list', 'test-list')
    const datalist = document.getElementById('test-list')
    expect(datalist).not.toBeNull()
    expect(datalist?.querySelectorAll('option')).toHaveLength(2)
  })

  it('calls onChange when user types', async () => {
    const onChange = vi.fn()
    const user = userEvent.setup()
    render(<ComboBox id="test" value="" onChange={onChange} options={[]} placeholder="Type here" />)
    await user.type(screen.getByPlaceholderText('Type here'), 'hello')
    expect(onChange).toHaveBeenCalled()
  })

  it('shows loading placeholder when loading', () => {
    render(
      <ComboBox
        id="test"
        value=""
        onChange={vi.fn()}
        options={[]}
        loading={true}
        placeholder="Pick one"
      />,
    )
    expect(screen.getByPlaceholderText('Loading...')).toBeInTheDocument()
    expect(screen.getByLabelText('Loading options')).toBeInTheDocument()
  })

  it('uses normal placeholder when not loading', () => {
    render(
      <ComboBox
        id="test"
        value=""
        onChange={vi.fn()}
        options={[]}
        loading={false}
        placeholder="Pick one"
      />,
    )
    expect(screen.getByPlaceholderText('Pick one')).toBeInTheDocument()
  })

  it('disables input when disabled is true', () => {
    render(
      <ComboBox
        id="test"
        value=""
        onChange={vi.fn()}
        options={[]}
        disabled={true}
        placeholder="test"
      />,
    )
    expect(screen.getByPlaceholderText('test')).toBeDisabled()
  })

  it('sets required attribute', () => {
    render(
      <ComboBox
        id="test"
        value=""
        onChange={vi.fn()}
        options={[]}
        required={true}
        placeholder="test"
      />,
    )
    expect(screen.getByPlaceholderText('test')).toBeRequired()
  })

  it('renders current value', () => {
    render(
      <ComboBox id="test" value="tts-1" onChange={vi.fn()} options={options} placeholder="test" />,
    )
    expect(screen.getByPlaceholderText('test')).toHaveValue('tts-1')
  })
})
