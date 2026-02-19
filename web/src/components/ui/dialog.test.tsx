import { render, screen } from '@testing-library/preact'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from './dialog'

describe('Dialog', () => {
  it('renders trigger and opens dialog', async () => {
    const user = userEvent.setup()
    render(
      <Dialog>
        <DialogTrigger>Open</DialogTrigger>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Title</DialogTitle>
            <DialogDescription>Description</DialogDescription>
          </DialogHeader>
          <DialogFooter>Footer</DialogFooter>
        </DialogContent>
      </Dialog>,
    )
    expect(screen.getByText('Open')).toBeInTheDocument()
    expect(screen.queryByText('Title')).not.toBeInTheDocument()

    await user.click(screen.getByText('Open'))
    expect(screen.getByText('Title')).toBeInTheDocument()
    expect(screen.getByText('Description')).toBeInTheDocument()
    expect(screen.getByText('Footer')).toBeInTheDocument()
  })

  it('applies custom className to content', async () => {
    const user = userEvent.setup()
    render(
      <Dialog>
        <DialogTrigger>Open</DialogTrigger>
        <DialogContent className="custom-dialog">
          <DialogTitle>T</DialogTitle>
          <DialogDescription>D</DialogDescription>
        </DialogContent>
      </Dialog>,
    )
    await user.click(screen.getByText('Open'))
    expect(screen.getByRole('dialog')).toHaveClass('custom-dialog')
  })

  it('applies custom className to header and footer', async () => {
    const user = userEvent.setup()
    render(
      <Dialog>
        <DialogTrigger>Open</DialogTrigger>
        <DialogContent>
          <DialogHeader className="hdr-class">
            <DialogTitle className="title-class">T</DialogTitle>
            <DialogDescription className="desc-class">D</DialogDescription>
          </DialogHeader>
          <DialogFooter className="ftr-class">F</DialogFooter>
        </DialogContent>
      </Dialog>,
    )
    await user.click(screen.getByText('Open'))
    expect(screen.getByText('T').parentElement).toHaveClass('hdr-class')
    expect(screen.getByText('T')).toHaveClass('title-class')
    expect(screen.getByText('D')).toHaveClass('desc-class')
    expect(screen.getByText('F')).toHaveClass('ftr-class')
  })
})
