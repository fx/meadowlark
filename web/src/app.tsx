import { Redirect, Route, Switch } from 'wouter'
import { AppHeader } from '@/components/app-header'
import { ThemeProvider } from '@/components/theme-provider'
import { EndpointsPage } from '@/pages/endpoints'
import { PlaygroundPage } from '@/pages/playground'
import { SettingsPage } from '@/pages/settings'
import { VoicesPage } from '@/pages/voices'

function App() {
  return (
    <ThemeProvider>
      <div className="min-h-screen flex flex-col">
        <AppHeader />
        <Switch>
          {/* One route for both modes, so switching between them does not remount the page. */}
          <Route path="/playground/:aliasId?" component={PlaygroundPage} />
          <Route path="/endpoints" component={EndpointsPage} />
          <Route path="/voices" component={VoicesPage} />
          <Route path="/aliases">
            <Redirect to="/voices" />
          </Route>
          <Route path="/settings" component={SettingsPage} />
          <Route path="/">
            <Redirect to="/playground" />
          </Route>
        </Switch>
      </div>
    </ThemeProvider>
  )
}

export { App }
