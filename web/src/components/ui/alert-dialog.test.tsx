import { render, screen } from '@testing-library/preact'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from './alert-dialog'

describe('AlertDialog', () => {
  it('renders trigger and opens alert dialog', async () => {
    const user = userEvent.setup()
    render(
      <AlertDialog>
        <AlertDialogTrigger>Delete</AlertDialogTrigger>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Are you sure?</AlertDialogTitle>
            <AlertDialogDescription>This cannot be undone.</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction>Confirm</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>,
    )
    expect(screen.queryByText('Are you sure?')).not.toBeInTheDocument()
    await user.click(screen.getByText('Delete'))
    expect(screen.getByText('Are you sure?')).toBeInTheDocument()
    expect(screen.getByText('This cannot be undone.')).toBeInTheDocument()
    expect(screen.getByText('Cancel')).toBeInTheDocument()
    expect(screen.getByText('Confirm')).toBeInTheDocument()
  })

  it('applies custom classNames', async () => {
    const user = userEvent.setup()
    render(
      <AlertDialog>
        <AlertDialogTrigger>Open</AlertDialogTrigger>
        <AlertDialogContent className="content-class">
          <AlertDialogHeader className="hdr-class">
            <AlertDialogTitle className="title-class">T</AlertDialogTitle>
            <AlertDialogDescription className="desc-class">D</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter className="ftr-class">
            <AlertDialogCancel className="cancel-class">C</AlertDialogCancel>
            <AlertDialogAction className="action-class">A</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>,
    )
    await user.click(screen.getByText('Open'))
    expect(screen.getByRole('alertdialog')).toHaveClass('content-class')
    expect(screen.getByText('T').parentElement).toHaveClass('hdr-class')
    expect(screen.getByText('T')).toHaveClass('title-class')
    expect(screen.getByText('D')).toHaveClass('desc-class')
    expect(screen.getByText('C').parentElement).toHaveClass('ftr-class')
    expect(screen.getByText('C')).toHaveClass('cancel-class')
    expect(screen.getByText('A')).toHaveClass('action-class')
  })
})
