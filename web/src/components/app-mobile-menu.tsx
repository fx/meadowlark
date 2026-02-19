import { useLocation } from 'wouter'
import { NAV_ITEMS } from '@/components/app-header'
import { Button } from '@/components/ui/button'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'

function AppMobileMenu({
  open,
  onOpenChange,
  currentPath,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  currentPath: string
}) {
  const [, setLocation] = useLocation()

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side="left" aria-label="Navigation menu">
        <SheetHeader>
          <SheetTitle>Meadowlark</SheetTitle>
          <SheetDescription className="sr-only">Navigation menu</SheetDescription>
        </SheetHeader>
        <nav className="mt-4 flex flex-col gap-1" aria-label="Mobile navigation">
          {NAV_ITEMS.map(({ path, label, icon: Icon }) => (
            <Button
              key={path}
              variant={currentPath === path ? 'secondary' : 'ghost'}
              className="justify-start gap-2"
              aria-current={currentPath === path ? 'page' : undefined}
              onClick={() => {
                setLocation(path)
                onOpenChange(false)
              }}
            >
              <Icon className="h-4 w-4" />
              {label}
            </Button>
          ))}
        </nav>
      </SheetContent>
    </Sheet>
  )
}

export { AppMobileMenu }
