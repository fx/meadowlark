import { GearSix, List, PlugsConnected, SpeakerHigh, TagSimple } from '@phosphor-icons/react'
import { useState } from 'preact/hooks'
import { useLocation } from 'wouter'
import { useTheme } from '@/components/theme-provider'
import { Button } from '@/components/ui/button'
import { Sheet, SheetContent, SheetHeader, SheetTitle, SheetTrigger } from '@/components/ui/sheet'

function AppMobileMenu() {
  const [open, setOpen] = useState(false)
  const [, setLocation] = useLocation()
  const { theme, setTheme } = useTheme()

  return (
    <Sheet open={open} onOpenChange={setOpen}>
      <SheetTrigger asChild>
        <Button variant="ghost" size="icon" aria-label="Menu">
          <List className="h-4 w-4" />
        </Button>
      </SheetTrigger>
      <SheetContent side="right">
        <SheetHeader>
          <SheetTitle>Menu</SheetTitle>
        </SheetHeader>
        <nav className="flex flex-col gap-2 px-4 pt-4">
          {[
            { path: '/endpoints', label: 'Endpoints', icon: PlugsConnected },
            { path: '/voices', label: 'Voices', icon: SpeakerHigh },
            { path: '/aliases', label: 'Aliases', icon: TagSimple },
            { path: '/settings', label: 'Settings', icon: GearSix },
          ].map(({ path, label, icon: Icon }) => (
            <Button
              key={path}
              variant="ghost"
              className="w-full justify-start gap-2"
              onClick={() => {
                setLocation(path)
                setOpen(false)
              }}
            >
              <Icon className="h-4 w-4" />
              {label}
            </Button>
          ))}
        </nav>
        <div className="px-4 pt-6">
          <p className="text-sm text-muted-foreground mb-2">Theme</p>
          <div className="flex gap-1">
            <Button
              variant={theme === 'light' ? 'secondary' : 'ghost'}
              size="sm"
              onClick={() => setTheme('light')}
            >
              Light
            </Button>
            <Button
              variant={theme === 'dark' ? 'secondary' : 'ghost'}
              size="sm"
              onClick={() => setTheme('dark')}
            >
              Dark
            </Button>
            <Button
              variant={theme === 'system' ? 'secondary' : 'ghost'}
              size="sm"
              onClick={() => setTheme('system')}
            >
              System
            </Button>
          </div>
        </div>
      </SheetContent>
    </Sheet>
  )
}

export { AppMobileMenu }
