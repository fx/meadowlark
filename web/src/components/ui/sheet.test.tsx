import { render, screen } from '@testing-library/preact'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from './sheet'

describe('Sheet', () => {
  it('renders trigger and opens sheet', async () => {
    const user = userEvent.setup()
    render(
      <Sheet>
        <SheetTrigger>Open Sheet</SheetTrigger>
        <SheetContent>
          <SheetHeader>
            <SheetTitle>Title</SheetTitle>
            <SheetDescription>Description</SheetDescription>
          </SheetHeader>
          <SheetFooter>Footer</SheetFooter>
        </SheetContent>
      </Sheet>,
    )
    expect(screen.queryByText('Title')).not.toBeInTheDocument()
    await user.click(screen.getByText('Open Sheet'))
    expect(screen.getByText('Title')).toBeInTheDocument()
    expect(screen.getByText('Description')).toBeInTheDocument()
    expect(screen.getByText('Footer')).toBeInTheDocument()
  })

  it('applies custom className to content', async () => {
    const user = userEvent.setup()
    render(
      <Sheet>
        <SheetTrigger>Open</SheetTrigger>
        <SheetContent className="custom-sheet">
          <SheetTitle>T</SheetTitle>
          <SheetDescription>D</SheetDescription>
        </SheetContent>
      </Sheet>,
    )
    await user.click(screen.getByText('Open'))
    expect(screen.getByRole('dialog')).toHaveClass('custom-sheet')
  })

  it('renders left side variant', async () => {
    const user = userEvent.setup()
    render(
      <Sheet>
        <SheetTrigger>Open</SheetTrigger>
        <SheetContent side="left">
          <SheetTitle>Left</SheetTitle>
          <SheetDescription>D</SheetDescription>
        </SheetContent>
      </Sheet>,
    )
    await user.click(screen.getByText('Open'))
    expect(screen.getByRole('dialog').className).toContain('left-0')
  })

  it('applies custom classNames to subcomponents', async () => {
    const user = userEvent.setup()
    render(
      <Sheet>
        <SheetTrigger>Open</SheetTrigger>
        <SheetContent>
          <SheetHeader className="hdr-class">
            <SheetTitle className="title-class">T</SheetTitle>
            <SheetDescription className="desc-class">D</SheetDescription>
          </SheetHeader>
          <SheetFooter className="ftr-class">F</SheetFooter>
        </SheetContent>
      </Sheet>,
    )
    await user.click(screen.getByText('Open'))
    expect(screen.getByText('T').parentElement).toHaveClass('hdr-class')
    expect(screen.getByText('T')).toHaveClass('title-class')
    expect(screen.getByText('D')).toHaveClass('desc-class')
    expect(screen.getByText('F')).toHaveClass('ftr-class')
  })
})
