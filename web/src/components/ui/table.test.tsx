import { render, screen } from '@testing-library/preact'
import { describe, expect, it } from 'vitest'
import {
  Table,
  TableBody,
  TableCaption,
  TableCell,
  TableFooter,
  TableHead,
  TableHeader,
  TableRow,
} from './table'

describe('Table', () => {
  it('renders a table element', () => {
    render(
      <Table data-testid="table">
        <TableBody>
          <TableRow>
            <TableCell>Cell</TableCell>
          </TableRow>
        </TableBody>
      </Table>,
    )
    expect(screen.getByTestId('table')).toBeInTheDocument()
  })

  it('applies custom className', () => {
    render(
      <Table className="custom" data-testid="table">
        <TableBody>
          <TableRow>
            <TableCell>Cell</TableCell>
          </TableRow>
        </TableBody>
      </Table>,
    )
    expect(screen.getByTestId('table')).toHaveClass('custom')
  })
})

describe('TableHeader', () => {
  it('renders with className', () => {
    render(
      <table>
        <TableHeader className="custom" data-testid="thead">
          <tr>
            <th>H</th>
          </tr>
        </TableHeader>
      </table>,
    )
    expect(screen.getByTestId('thead')).toHaveClass('custom')
  })
})

describe('TableBody', () => {
  it('renders with className', () => {
    render(
      <table>
        <TableBody className="custom" data-testid="tbody">
          <tr>
            <td>B</td>
          </tr>
        </TableBody>
      </table>,
    )
    expect(screen.getByTestId('tbody')).toHaveClass('custom')
  })
})

describe('TableFooter', () => {
  it('renders with className', () => {
    render(
      <table>
        <TableFooter className="custom" data-testid="tfoot">
          <tr>
            <td>F</td>
          </tr>
        </TableFooter>
      </table>,
    )
    expect(screen.getByTestId('tfoot')).toHaveClass('custom')
  })
})

describe('TableRow', () => {
  it('renders with className', () => {
    render(
      <table>
        <tbody>
          <TableRow className="custom" data-testid="tr">
            <td>R</td>
          </TableRow>
        </tbody>
      </table>,
    )
    expect(screen.getByTestId('tr')).toHaveClass('custom')
  })
})

describe('TableHead', () => {
  it('renders with className', () => {
    render(
      <table>
        <thead>
          <tr>
            <TableHead className="custom">Head</TableHead>
          </tr>
        </thead>
      </table>,
    )
    expect(screen.getByText('Head')).toHaveClass('custom')
  })
})

describe('TableCell', () => {
  it('renders with className', () => {
    render(
      <table>
        <tbody>
          <tr>
            <TableCell className="custom">Cell</TableCell>
          </tr>
        </tbody>
      </table>,
    )
    expect(screen.getByText('Cell')).toHaveClass('custom')
  })
})

describe('TableCaption', () => {
  it('renders with className', () => {
    render(
      <table>
        <TableCaption className="custom">Caption</TableCaption>
      </table>,
    )
    expect(screen.getByText('Caption')).toHaveClass('custom')
  })
})
