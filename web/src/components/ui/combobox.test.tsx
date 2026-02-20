import { render, screen } from '@testing-library/preact'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import type { ComboboxOption } from './combobox'
import { Combobox } from './combobox'

const options: ComboboxOption[] = [
  { value: 'tts-1', label: 'TTS One' },
  { value: 'tts-1-hd', label: 'TTS One HD' },
  { value: 'piper', label: 'Piper' },
]

/** Open the dropdown via the caret toggle button. */
async function openDropdown(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole('button', { name: 'Toggle options' }))
}

describe('Combobox', () => {
  it('renders input with placeholder', () => {
    render(<Combobox value="" onChange={vi.fn()} options={[]} placeholder="Pick a model" />)
    expect(screen.getByPlaceholderText('Pick a model')).toBeInTheDocument()
  })

  it('renders toggle button when options are present', () => {
    render(<Combobox value="" onChange={vi.fn()} options={options} />)
    expect(screen.getByRole('button', { name: 'Toggle options' })).toBeInTheDocument()
  })

  it('does not render toggle button when options are empty', () => {
    render(<Combobox value="" onChange={vi.fn()} options={[]} />)
    expect(screen.queryByRole('button', { name: 'Toggle options' })).not.toBeInTheDocument()
  })

  it('opens dropdown via toggle and shows all options', async () => {
    const user = userEvent.setup()
    render(<Combobox value="" onChange={vi.fn()} options={options} />)
    await openDropdown(user)
    expect(screen.getByText('TTS One')).toBeInTheDocument()
    expect(screen.getByText('TTS One HD')).toBeInTheDocument()
    expect(screen.getByText('Piper')).toBeInTheDocument()
  })

  it('opens dropdown on input focus', async () => {
    const user = userEvent.setup()
    render(<Combobox value="" onChange={vi.fn()} options={options} />)
    await user.tab()
    expect(screen.getByText('TTS One')).toBeInTheDocument()
  })

  it('calls onChange when an option is selected', async () => {
    const onChange = vi.fn()
    const user = userEvent.setup()
    render(<Combobox value="" onChange={onChange} options={options} />)
    await openDropdown(user)
    await user.click(screen.getByText('Piper'))
    expect(onChange).toHaveBeenCalledWith('piper')
  })

  it('calls onChange on free text input', async () => {
    const onChange = vi.fn()
    const user = userEvent.setup()
    render(<Combobox value="" onChange={onChange} options={[]} />)
    await user.type(screen.getByRole('textbox'), 'custom')
    // Component is controlled with value="", so each keystroke produces
    // individual character values as the prop resets between renders
    expect(onChange).toHaveBeenCalledTimes(6)
    expect(onChange).toHaveBeenCalledWith('c')
    expect(onChange).toHaveBeenCalledWith('o')
    expect(onChange).toHaveBeenCalledWith('m')
  })

  it('shows loading spinner and placeholder when loading', () => {
    render(<Combobox value="" onChange={vi.fn()} options={[]} loading />)
    expect(screen.getByPlaceholderText('Loading...')).toBeInTheDocument()
  })

  it('closes dropdown on Escape key', async () => {
    const user = userEvent.setup()
    render(<Combobox value="" onChange={vi.fn()} options={options} />)
    await openDropdown(user)
    expect(screen.getByText('TTS One')).toBeInTheDocument()
    await user.keyboard('{Escape}')
    expect(screen.queryByText('TTS One')).not.toBeInTheDocument()
  })

  it('toggles dropdown via caret button', async () => {
    const user = userEvent.setup()
    render(<Combobox value="" onChange={vi.fn()} options={options} />)
    const toggle = screen.getByRole('button', { name: 'Toggle options' })
    await user.click(toggle)
    expect(screen.getByText('TTS One')).toBeInTheDocument()
    await user.click(toggle)
    expect(screen.queryByText('TTS One')).not.toBeInTheDocument()
  })

  it('shows check icon for currently selected value', async () => {
    const user = userEvent.setup()
    render(<Combobox value="piper" onChange={vi.fn()} options={options} />)
    await openDropdown(user)
    expect(screen.getByTestId('icon-check')).toBeInTheDocument()
  })

  it('shows "No matches" when filter yields nothing', async () => {
    const onChange = vi.fn()
    const user = userEvent.setup()
    render(<Combobox value="" onChange={onChange} options={options} />)
    await user.type(screen.getByRole('textbox'), 'zzz')
    expect(screen.getByText('No matches')).toBeInTheDocument()
  })

  it('applies disabled state', () => {
    render(<Combobox value="" onChange={vi.fn()} options={options} disabled />)
    expect(screen.getByRole('textbox')).toBeDisabled()
  })

  it('passes id and required props', () => {
    render(<Combobox value="" onChange={vi.fn()} options={[]} id="my-combo" required />)
    const input = screen.getByRole('textbox')
    expect(input).toHaveAttribute('id', 'my-combo')
    expect(input).toBeRequired()
  })

  it('applies custom className', () => {
    const { container } = render(
      <Combobox value="" onChange={vi.fn()} options={[]} className="custom-class" />,
    )
    const wrapper = container.querySelector('.custom-class')
    expect(wrapper).toBeInTheDocument()
  })

  it('clears query and closes after selecting an option', async () => {
    const onChange = vi.fn()
    const user = userEvent.setup()
    render(<Combobox value="" onChange={onChange} options={options} />)
    await openDropdown(user)
    await user.click(screen.getByText('TTS One'))
    // Dropdown should close after selection
    expect(screen.queryByText('Piper')).not.toBeInTheDocument()
  })

  it('opens popover on typing when closed', async () => {
    const onChange = vi.fn()
    const user = userEvent.setup()
    render(<Combobox value="" onChange={onChange} options={options} />)
    await user.type(screen.getByRole('textbox'), 't')
    // Typing should open the dropdown since options exist
    expect(screen.getByText('TTS One')).toBeInTheDocument()
  })

  it('highlights matching option in dropdown', async () => {
    const user = userEvent.setup()
    render(<Combobox value="tts-1" onChange={vi.fn()} options={options} />)
    await openDropdown(user)
    // The selected option has bg-accent class applied
    const selectedBtn = screen.getByText('TTS One').closest('button')
    expect(selectedBtn?.className).toContain('bg-accent')
  })
})
