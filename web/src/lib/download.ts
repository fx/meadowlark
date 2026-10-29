// Helpers for saving a take to disk.

function pad(n: number): string {
  return String(n).padStart(2, '0')
}

/** `meadowlark-<voice>-<YYYYMMDD-HHMMSS>.<ext>`, with the voice made filename-safe. */
export function downloadFilename(voice: string, ext: string, date: Date): string {
  const safeVoice = voice.replace(/[^A-Za-z0-9._-]/g, '-')
  const stamp =
    `${date.getFullYear()}${pad(date.getMonth() + 1)}${pad(date.getDate())}-` +
    `${pad(date.getHours())}${pad(date.getMinutes())}${pad(date.getSeconds())}`
  return `meadowlark-${safeVoice}-${stamp}.${ext}`
}

/** How long a temporary object URL outlives the click that started its download. */
const REVOKE_DELAY_MS = 10_000

/** Save a blob through a temporary anchor, then release its object URL. */
export function downloadBlob(blob: Blob, filename: string): void {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  a.remove()
  setTimeout(() => URL.revokeObjectURL(url), REVOKE_DELAY_MS)
}
