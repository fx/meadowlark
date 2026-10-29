// In-memory history of playground takes. It lives at module level so it
// survives navigating between pages and switching aliases; it does not survive
// a reload.
import { useEffect, useState } from 'preact/hooks'
import type { Peaks } from '@/lib/peaks'

export const MAX_TAKES = 20

export const DEFAULT_TEXT = 'Hello, this is a test.'

/** The settings a take was generated with. */
export interface TakeParams {
  endpointId: string
  endpointName: string
  model: string
  voice: string
  speed?: number
  instructions?: string
}

export interface TakeFormat {
  sampleRate: number
  channels: number
  bitsPerSample: number
}

export interface Take {
  id: string
  /** 1-based sequence number, shown as `#n`. */
  n: number
  params: TakeParams
  text: string
  /** The exact WAV bytes the server returned. */
  blob: Blob
  /** Object URL for `blob`; revoked when the take leaves the history. */
  url: string
  format: TakeFormat
  /** Seconds. */
  duration: number
  peaks: Peaks
  miniPeaks: Peaks
  latencyMs: number
  createdAt: Date
}

export type NewTake = Omit<Take, 'id' | 'n'>

export interface TakesState {
  /** Newest first. */
  takes: Take[]
  selectedId: string | null
  /** The playground's text-area content. */
  text: string
}

let state: TakesState = { takes: [], selectedId: null, text: DEFAULT_TEXT }
let counter = 0
const listeners = new Set<() => void>()

function setState(next: TakesState): void {
  state = next
  for (const fn of listeners) fn()
}

function revoke(takes: Take[]): void {
  for (const t of takes) URL.revokeObjectURL(t.url)
}

export function getTakesState(): TakesState {
  return state
}

export function subscribeTakes(fn: () => void): () => void {
  listeners.add(fn)
  return () => {
    listeners.delete(fn)
  }
}

/** Add a take as the newest and select it, evicting the oldest past MAX_TAKES. */
export function addTake(input: NewTake): Take {
  counter++
  const take: Take = { ...input, id: `take-${counter}`, n: counter }
  const takes = [take, ...state.takes]
  revoke(takes.slice(MAX_TAKES))
  setState({ ...state, takes: takes.slice(0, MAX_TAKES), selectedId: take.id })
  return take
}

export function removeTake(id: string): void {
  const removed = state.takes.filter((t) => t.id === id)
  if (removed.length === 0) return
  revoke(removed)
  const takes = state.takes.filter((t) => t.id !== id)
  const selectedId = state.selectedId === id ? (takes[0]?.id ?? null) : state.selectedId
  setState({ ...state, takes, selectedId })
}

export function clearTakes(): void {
  revoke(state.takes)
  setState({ ...state, takes: [], selectedId: null })
}

export function selectTake(id: string): void {
  setState({ ...state, selectedId: id })
}

export function setDraftText(text: string): void {
  setState({ ...state, text })
}

/** Test-only: forget every take and restore the defaults. */
export function resetTakes(): void {
  counter = 0
  setState({ takes: [], selectedId: null, text: DEFAULT_TEXT })
}

export function useTakes(): TakesState {
  const [snapshot, setSnapshot] = useState(state)
  useEffect(() => {
    // Catch up on anything that changed between render and subscribe.
    setSnapshot(state)
    return subscribeTakes(() => setSnapshot(state))
  }, [])
  return snapshot
}
