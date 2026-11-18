import { createApp } from 'vue'
import App from './App.vue'
import { router } from './app/router'
import { initializeTheme } from './composables/useTheme'
import { bindAdminUI } from './services/adminUi'
import { reportNavigationFailure } from './services/navigationFailure'
import { captureInitialPage, preserveInitialArticleNodes } from './services/initialPage'
import { captureInitialPortal } from './services/initialPortal'
import { ensurePageFonts } from './services/fonts'
import './styles/fonts.css'
import './styles/tokens.css'
import './styles/wiki.css'

class WikiTimeElement extends HTMLElement {
  connectedCallback() {
    const value = this.getAttribute('datetime')
    if (!value) return
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return
    this.textContent = new Intl.DateTimeFormat(document.documentElement.lang || undefined, {
      dateStyle: 'medium',
      timeStyle: 'short',
    }).format(date)
    this.title = date.toISOString()
  }
}

if (!customElements.get('techpedia-time')) customElements.define('techpedia-time', WikiTimeElement)
initializeTheme()
captureInitialPage()
captureInitialPortal()
ensurePageFonts()

const app = createApp(App)
bindAdminUI(app)
app.use(router)
router.afterEach(() => window.requestAnimationFrame(ensurePageFonts))
const initialRoute = router.resolve(window.location.pathname)
const standalone = initialRoute.path === '/' || initialRoute.name === 'support' || initialRoute.name === 'not-found'
const admin = initialRoute.path === '/admin' || initialRoute.path.startsWith('/admin/')
const initialShell = !standalone && !admin && initialRoute.matched.length
  ? import('./components/WikiShell.vue').catch(reportNavigationFailure)
  : Promise.resolve()
void Promise.all([router.isReady().catch(reportNavigationFailure), initialShell]).then(() => {
  preserveInitialArticleNodes()
  app.mount('#app')
  void import('./services/analytics').then(({ bindPublicAnalytics }) => bindPublicAnalytics(router))
})
