import { Redirect, Route, Switch } from 'wouter'
import { AppHeader } from '@/components/app-header'
import { ThemeProvider } from '@/components/theme-provider'
import { AliasesPage } from '@/pages/aliases'
import { EndpointsPage } from '@/pages/endpoints'
import { SettingsPage } from '@/pages/settings'
import { VoicesPage } from '@/pages/voices'

function App() {
  return (
    <ThemeProvider>
      <div className="min-h-screen bg-background text-foreground">
        <AppHeader />
        <main className="container mx-auto px-4 py-6">
          <Switch>
            <Route path="/endpoints" component={EndpointsPage} />
            <Route path="/voices" component={VoicesPage} />
            <Route path="/aliases" component={AliasesPage} />
            <Route path="/settings" component={SettingsPage} />
            <Route path="/">
              <Redirect to="/endpoints" />
            </Route>
          </Switch>
        </main>
      </div>
    </ThemeProvider>
  )
}

export { App }
