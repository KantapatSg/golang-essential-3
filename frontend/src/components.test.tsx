import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { Landing } from './pages'
import { State } from './components'

test('landing explains the system thesis', () => { render(<MemoryRouter><Landing /></MemoryRouter>); expect(screen.getByText(/Trace the work/i)).toBeInTheDocument(); expect(screen.getByText(/LIVE ARCHITECTURE/i)).toBeInTheDocument() })
test('error state is announced accessibly', () => { render(<State kind="error" text="Reader unavailable" />); expect(screen.getByRole('alert')).toHaveTextContent('Reader unavailable') })
