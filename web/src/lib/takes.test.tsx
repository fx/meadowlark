import { act, render, screen } from '@testing-library/preact'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { makeNewTake } from '@/test-audio'
import {
  addTake,
  clearTakes,
  DEFAULT_TEXT,
  getTakesState,
  MAX_TAKES,
  removeTake,
  resetTakes,
  selectTake,
  setDraftText,
  subscribeTakes,
  useTakes,
} from './takes'

beforeEach(() => {
  resetTakes()
})

describe('take store', () => {
  it('starts empty with the default text', () => {
    expect(getTakesState()).toEqual({ takes: [], selectedId: null, text: DEFAULT_TEXT })
  })

  it('adds takes newest first, numbers them, and selects the newest', () => {
    const a = addTake(makeNewTake())
    const b = addTake(makeNewTake())
    expect(a.n).toBe(1)
    expect(b.n).toBe(2)
    expect(getTakesState().takes.map((t) => t.id)).toEqual([b.id, a.id])
    expect(getTakesState().selectedId).toBe(b.id)
  })

  it('evicts the oldest take past the limit and revokes its URL', () => {
    const first = addTake(makeNewTake())
    for (let i = 1; i < MAX_TAKES; i++) addTake(makeNewTake())
    expect(getTakesState().takes).toHaveLength(MAX_TAKES)
    expect(URL.revokeObjectURL).not.toHaveBeenCalled()

    addTake(makeNewTake())
    expect(getTakesState().takes).toHaveLength(MAX_TAKES)
    expect(getTakesState().takes.some((t) => t.id === first.id)).toBe(false)
    expect(URL.revokeObjectURL).toHaveBeenCalledExactlyOnceWith(first.url)
  })

  it('removes a take and revokes its URL', () => {
    const a = addTake(makeNewTake())
    const b = addTake(makeNewTake())
    selectTake(a.id)
    removeTake(b.id)
    expect(getTakesState().takes.map((t) => t.id)).toEqual([a.id])
    expect(getTakesState().selectedId).toBe(a.id)
    expect(URL.revokeObjectURL).toHaveBeenCalledWith(b.url)
  })

  it('selects the newest remaining take when the selected one is removed', () => {
    const a = addTake(makeNewTake())
    const b = addTake(makeNewTake())
    const c = addTake(makeNewTake())
    removeTake(c.id)
    expect(getTakesState().selectedId).toBe(b.id)
    removeTake(b.id)
    removeTake(a.id)
    expect(getTakesState().selectedId).toBeNull()
  })

  it('ignores removing an unknown take', () => {
    addTake(makeNewTake())
    const before = getTakesState()
    removeTake('nope')
    expect(getTakesState()).toBe(before)
    expect(URL.revokeObjectURL).not.toHaveBeenCalled()
  })

  it('clears every take and revokes every URL', () => {
    const a = addTake(makeNewTake())
    const b = addTake(makeNewTake())
    clearTakes()
    expect(getTakesState().takes).toEqual([])
    expect(getTakesState().selectedId).toBeNull()
    expect(URL.revokeObjectURL).toHaveBeenCalledWith(a.url)
    expect(URL.revokeObjectURL).toHaveBeenCalledWith(b.url)
  })

  it('keeps the draft text', () => {
    setDraftText('Something else')
    expect(getTakesState().text).toBe('Something else')
    addTake(makeNewTake())
    expect(getTakesState().text).toBe('Something else')
  })

  it('notifies subscribers until they unsubscribe', () => {
    const fn = vi.fn()
    const unsubscribe = subscribeTakes(fn)
    setDraftText('a')
    expect(fn).toHaveBeenCalledTimes(1)
    unsubscribe()
    setDraftText('b')
    expect(fn).toHaveBeenCalledTimes(1)
  })

  it('restarts numbering on reset', () => {
    addTake(makeNewTake())
    resetTakes()
    expect(addTake(makeNewTake()).n).toBe(1)
  })
})

describe('useTakes', () => {
  function Probe() {
    const { takes, text } = useTakes()
    return (
      <p>
        {takes.length} takes, {text}
      </p>
    )
  }

  it('re-renders on store changes and stops listening on unmount', () => {
    const { unmount } = render(<Probe />)
    expect(screen.getByText(`0 takes, ${DEFAULT_TEXT}`)).toBeInTheDocument()
    act(() => {
      addTake(makeNewTake())
    })
    expect(screen.getByText(`1 takes, ${DEFAULT_TEXT}`)).toBeInTheDocument()
    unmount()
    const fn = vi.fn()
    subscribeTakes(fn)
    setDraftText('x')
    expect(fn).toHaveBeenCalledOnce()
  })
})
