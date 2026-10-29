import { GearSix, PlugsConnected, SpeakerHigh, Waveform } from '@phosphor-icons/react'
import { useLocation } from 'wouter'
import { AppMobileMenu } from '@/components/app-mobile-menu'
import { ThemeToggle } from '@/components/theme-toggle'
import { Button } from '@/components/ui/button'
import { Menubar, MenubarMenu, MenubarTrigger } from '@/components/ui/menubar'

const NAV_ITEMS = [
  { path: '/playground', label: 'Playground', icon: Waveform },
  { path: '/voices', label: 'Voices', icon: SpeakerHigh },
  { path: '/endpoints', label: 'Endpoints', icon: PlugsConnected },
  { path: '/settings', label: 'Settings', icon: GearSix },
] as const

/** A nav item is active on its own path and on any path below it. */
function isActive(location: string, path: string): boolean {
  return location === path || location.startsWith(`${path}/`)
}

function AppHeader({ version }: { version?: string }) {
  const [location, setLocation] = useLocation()

  return (
    <header className="border-b bg-background">
      <Menubar className="border-0 px-4 h-12">
        <MenubarMenu>
          <MenubarTrigger className="font-bold">Meadowlark</MenubarTrigger>
        </MenubarMenu>
        <nav className="flex items-center gap-1 ml-2">
          {NAV_ITEMS.map(({ path, label, icon: Icon }) => (
            <Button
              key={path}
              variant={isActive(location, path) ? 'secondary' : 'ghost'}
              size="icon"
              aria-label={label}
              aria-current={isActive(location, path) ? 'page' : undefined}
              onClick={() => setLocation(path)}
            >
              <Icon className="h-4 w-4" />
            </Button>
          ))}
        </nav>

        <div className="flex-1" />

        <div className="hidden md:flex items-center gap-1">
          {version && <span className="text-xs text-muted-foreground font-mono">{version}</span>}
          <ThemeToggle />
        </div>

        <div className="md:hidden">
          <AppMobileMenu />
        </div>
      </Menubar>
    </header>
  )
}

export { AppHeader, NAV_ITEMS }
