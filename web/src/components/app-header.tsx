import { GearSix, List, PlugsConnected, SpeakerHigh, TagSimple } from '@phosphor-icons/react'
import type { ComponentChildren } from 'preact'
import { useState } from 'preact/hooks'
import { useLocation } from 'wouter'
import { ThemeToggle } from '@/components/theme-toggle'
import { Button } from '@/components/ui/button'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import { AppMobileMenu } from './app-mobile-menu'

const NAV_ITEMS = [
  { path: '/endpoints', label: 'Endpoints', icon: PlugsConnected },
  { path: '/voices', label: 'Voices', icon: SpeakerHigh },
  { path: '/aliases', label: 'Aliases', icon: TagSimple },
  { path: '/settings', label: 'Settings', icon: GearSix },
] as const

function NavButton({
  href,
  active,
  label,
  children,
}: {
  href: string
  active: boolean
  label: string
  children: ComponentChildren
}) {
  const [, setLocation] = useLocation()
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          variant={active ? 'secondary' : 'ghost'}
          size="icon"
          aria-label={label}
          aria-current={active ? 'page' : undefined}
          onClick={() => setLocation(href)}
        >
          {children}
        </Button>
      </TooltipTrigger>
      <TooltipContent>
        <p>{label}</p>
      </TooltipContent>
    </Tooltip>
  )
}

function AppHeader({ version }: { version?: string }) {
  const [location] = useLocation()
  const [mobileOpen, setMobileOpen] = useState(false)

  return (
    <header className="sticky top-0 z-40 flex h-12 items-center justify-between border-b bg-background px-4">
      <div className="flex items-center gap-2">
        <span className="text-sm font-bold">Meadowlark</span>
      </div>

      <TooltipProvider>
        <nav className="hidden items-center gap-1 md:flex" aria-label="Main navigation">
          {NAV_ITEMS.map(({ path, label, icon: Icon }) => (
            <NavButton key={path} href={path} active={location === path} label={label}>
              <Icon className="h-4 w-4" />
            </NavButton>
          ))}
        </nav>
      </TooltipProvider>

      <div className="flex items-center gap-2">
        {version && (
          <span className="hidden text-xs text-muted-foreground sm:inline">{version}</span>
        )}
        <ThemeToggle />
        <Button
          variant="ghost"
          size="icon"
          className="md:hidden"
          aria-label="Open menu"
          onClick={() => setMobileOpen(true)}
        >
          <List className="h-4 w-4" />
        </Button>
      </div>

      <AppMobileMenu open={mobileOpen} onOpenChange={setMobileOpen} currentPath={location} />
    </header>
  )
}

export { AppHeader, NAV_ITEMS }
