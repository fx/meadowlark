import { type ComponentChildren, createContext } from 'preact'
import { useCallback, useContext, useEffect, useState } from 'preact/hooks'

type Theme = 'dark' | 'light' | 'system'

type ThemeProviderState = {
  theme: Theme
  setTheme: (theme: Theme) => void
}

const STORAGE_KEY = 'meadowlark-theme'

const ThemeProviderContext = createContext<ThemeProviderState>({
  theme: 'system',
  setTheme: () => {},
})

type ThemeProviderProps = {
  children: ComponentChildren
  defaultTheme?: Theme
  storageKey?: string
}

function applyTheme(theme: Theme) {
  const root = window.document.documentElement
  const systemTheme = window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
  const resolved = theme === 'system' ? systemTheme : theme

  root.classList.remove('light', 'dark')
  root.classList.add(resolved)
}

function ThemeProvider({
  children,
  defaultTheme = 'system',
  storageKey = STORAGE_KEY,
}: ThemeProviderProps) {
  const [theme, setThemeState] = useState<Theme>(() => {
    const stored = localStorage.getItem(storageKey)
    return (stored as Theme) || defaultTheme
  })

  useEffect(() => {
    applyTheme(theme)
  }, [theme])

  useEffect(() => {
    if (theme !== 'system') return

    const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)')
    const handler = () => applyTheme('system')
    mediaQuery.addEventListener('change', handler)
    return () => mediaQuery.removeEventListener('change', handler)
  }, [theme])

  const setTheme = useCallback(
    (newTheme: Theme) => {
      localStorage.setItem(storageKey, newTheme)
      setThemeState(newTheme)
    },
    [storageKey],
  )

  return (
    <ThemeProviderContext.Provider value={{ theme, setTheme }}>
      {children}
    </ThemeProviderContext.Provider>
  )
}

function useTheme() {
  const context = useContext(ThemeProviderContext)
  return context
}

export { ThemeProvider, useTheme }
export type { Theme, ThemeProviderProps }
