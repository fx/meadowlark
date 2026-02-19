import { render, screen } from '@testing-library/preact'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import {
  Menubar,
  MenubarCheckboxItem,
  MenubarContent,
  MenubarItem,
  MenubarLabel,
  MenubarMenu,
  MenubarRadioGroup,
  MenubarRadioItem,
  MenubarSeparator,
  MenubarShortcut,
  MenubarTrigger,
} from './menubar'

describe('Menubar', () => {
  it('renders menubar with triggers', () => {
    render(
      <Menubar className="custom-bar">
        <MenubarMenu>
          <MenubarTrigger className="custom-trigger">File</MenubarTrigger>
          <MenubarContent>
            <MenubarItem>New</MenubarItem>
          </MenubarContent>
        </MenubarMenu>
      </Menubar>,
    )
    expect(screen.getByRole('menubar')).toHaveClass('custom-bar')
    expect(screen.getByText('File')).toHaveClass('custom-trigger')
  })

  it('opens menu on click', async () => {
    const user = userEvent.setup()
    render(
      <Menubar>
        <MenubarMenu>
          <MenubarTrigger>File</MenubarTrigger>
          <MenubarContent>
            <MenubarLabel>Actions</MenubarLabel>
            <MenubarSeparator />
            <MenubarItem>
              Open
              <MenubarShortcut>Ctrl+O</MenubarShortcut>
            </MenubarItem>
          </MenubarContent>
        </MenubarMenu>
      </Menubar>,
    )
    expect(screen.queryByText('Actions')).not.toBeInTheDocument()
    await user.click(screen.getByText('File'))
    expect(screen.getByText('Actions')).toBeInTheDocument()
    expect(screen.getByText('Open')).toBeInTheDocument()
    expect(screen.getByText('Ctrl+O')).toBeInTheDocument()
  })

  it('applies custom classNames to subcomponents', async () => {
    const user = userEvent.setup()
    render(
      <Menubar>
        <MenubarMenu>
          <MenubarTrigger>Edit</MenubarTrigger>
          <MenubarContent className="content-class">
            <MenubarLabel className="label-class">L</MenubarLabel>
            <MenubarSeparator className="sep-class" />
            <MenubarItem className="item-class">I</MenubarItem>
          </MenubarContent>
        </MenubarMenu>
      </Menubar>,
    )
    await user.click(screen.getByText('Edit'))
    expect(screen.getByText('L')).toHaveClass('label-class')
    expect(screen.getByText('I')).toHaveClass('item-class')
  })

  it('renders inset menu item', async () => {
    const user = userEvent.setup()
    render(
      <Menubar>
        <MenubarMenu>
          <MenubarTrigger>File</MenubarTrigger>
          <MenubarContent>
            <MenubarItem inset>Inset</MenubarItem>
          </MenubarContent>
        </MenubarMenu>
      </Menubar>,
    )
    await user.click(screen.getByText('File'))
    expect(screen.getByText('Inset')).toHaveClass('pl-8')
  })

  it('renders inset label', async () => {
    const user = userEvent.setup()
    render(
      <Menubar>
        <MenubarMenu>
          <MenubarTrigger>File</MenubarTrigger>
          <MenubarContent>
            <MenubarLabel inset>Inset Label</MenubarLabel>
          </MenubarContent>
        </MenubarMenu>
      </Menubar>,
    )
    await user.click(screen.getByText('File'))
    expect(screen.getByText('Inset Label')).toHaveClass('pl-8')
  })

  it('renders shortcut with custom className', async () => {
    const user = userEvent.setup()
    render(
      <Menubar>
        <MenubarMenu>
          <MenubarTrigger>File</MenubarTrigger>
          <MenubarContent>
            <MenubarItem>
              <MenubarShortcut className="shortcut-class">K</MenubarShortcut>
            </MenubarItem>
          </MenubarContent>
        </MenubarMenu>
      </Menubar>,
    )
    await user.click(screen.getByText('File'))
    expect(screen.getByText('K')).toHaveClass('shortcut-class')
  })

  it('renders checkbox item', async () => {
    const user = userEvent.setup()
    render(
      <Menubar>
        <MenubarMenu>
          <MenubarTrigger>View</MenubarTrigger>
          <MenubarContent>
            <MenubarCheckboxItem checked className="cb-class">
              Toolbar
            </MenubarCheckboxItem>
          </MenubarContent>
        </MenubarMenu>
      </Menubar>,
    )
    await user.click(screen.getByText('View'))
    expect(screen.getByText('Toolbar')).toHaveClass('cb-class')
  })

  it('renders radio item', async () => {
    const user = userEvent.setup()
    render(
      <Menubar>
        <MenubarMenu>
          <MenubarTrigger>View</MenubarTrigger>
          <MenubarContent>
            <MenubarRadioGroup value="a">
              <MenubarRadioItem value="a" className="radio-class">
                Option A
              </MenubarRadioItem>
            </MenubarRadioGroup>
          </MenubarContent>
        </MenubarMenu>
      </Menubar>,
    )
    await user.click(screen.getByText('View'))
    expect(screen.getByText('Option A')).toHaveClass('radio-class')
  })
})
