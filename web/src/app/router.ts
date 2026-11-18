import { createRouter, createWebHistory } from 'vue-router'
import { isMobileViewerViewport } from '../composables/useMobileViewer'
import { ensureAdminUI } from '../services/adminUi'
import { parallelView } from '../services/routeLoading'
import { navigationFailed, reportNavigationFailure } from '../services/navigationFailure'
import type { Permission } from '../types/api'

function adminView<T>(loader: () => Promise<T>) {
  return parallelView(loader, () => Promise.all([ensureAdminUI(), import('../components/admin/AdminShell.vue')]))
}

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/',
      name: 'language-portal',
      component: () => import('../views/LanguagePortalView.vue'),
    },
    {
      path: '/support',
      name: 'support',
      component: () => import('../views/SupportView.vue'),
    },
    {
      path: '/changelog',
      name: 'changelog',
      component: () => import('../views/ChangelogView.vue'),
    },
    {
      path: '/graph',
      name: 'graph-all',
      component: () => import('../views/GraphView.vue'),
    },
    {
      path: '/:locale/graph',
      name: 'graph-language',
      component: () => import('../views/GraphView.vue'),
    },
    {
      path: '/:locale/search',
      name: 'search',
      component: () => import('../views/SearchView.vue'),
    },
    {
      path: '/:locale/wiki/:name+',
      name: 'article',
      component: () => import('../views/ArticleView.vue'),
    },
    {
      path: '/login',
      name: 'login',
      component: () => import('../views/LoginView.vue'),
      meta: { mobileBlocked: true },
    },
    {
      path: '/signup',
      name: 'signup',
      component: () => import('../views/SignupView.vue'),
      meta: { mobileBlocked: true },
    },
    {
      path: '/account',
      name: 'account',
      component: () => import('../views/AccountView.vue'),
      meta: { requiresAuth: true, mobileBlocked: true },
    },
    {
      path: '/user/:username',
      name: 'user-profile',
      component: () => import('../views/UserProfileView.vue'),
    },
    {
      path: '/history/:locale/:name+',
      name: 'history',
      component: () => import('../views/HistoryView.vue'),
    },
    {
      path: '/edit/:locale/:name+',
      name: 'editor',
      component: () => import('../views/EditorView.vue'),
      meta: { permission: 'edit', mobileBlocked: true },
    },
    {
      path: '/languages',
      name: 'languages',
      component: () => import('../views/LanguagesView.vue'),
    },
    {
      path: '/admin',
      name: 'admin-dashboard',
      component: adminView(() => import('../views/admin/AdminDashboardView.vue')),
      meta: { permission: 'admin', mobileBlocked: true },
    },
    {
      path: '/admin/pages',
      name: 'admin-pages',
      component: adminView(() => import('../views/admin/AdminPagesView.vue')),
      meta: { permission: 'admin', mobileBlocked: true },
    },
    {
      path: '/admin/boxes',
      name: 'admin-boxes',
      component: adminView(() => import('../views/admin/AdminBoxesView.vue')),
      meta: { permission: 'admin', mobileBlocked: true },
    },
    {
      path: '/admin/proposals',
      name: 'admin-proposals',
      component: adminView(() => import('../views/admin/AdminProposalsView.vue')),
      meta: { permission: 'admin', mobileBlocked: true },
    },
    {
      path: '/admin/trash',
      name: 'admin-trash',
      component: adminView(() => import('../views/admin/AdminTrashView.vue')),
      meta: { permission: 'admin', mobileBlocked: true },
    },
    {
      path: '/admin/audit',
      name: 'admin-audit',
      component: adminView(() => import('../views/admin/AdminAuditView.vue')),
      meta: { permission: 'admin', mobileBlocked: true },
    },
    {
      path: '/admin/techdata-audit',
      name: 'admin-techdata-audit',
      component: adminView(() => import('../views/admin/AdminTechdataAuditView.vue')),
      meta: { permission: 'admin', mobileBlocked: true },
    },
    {
      path: '/admin/support',
      name: 'admin-support',
      component: adminView(() => import('../views/admin/AdminSupportView.vue')),
      meta: { permission: 'admin', mobileBlocked: true },
    },
    {
      path: '/admin/server',
      name: 'admin-server',
      component: adminView(() => import('../views/admin/AdminServerView.vue')),
      meta: { permission: 'admin', mobileBlocked: true },
    },
    {
      path: '/admin/analytics',
      name: 'admin-analytics',
      component: adminView(() => import('../views/admin/AdminAnalyticsView.vue')),
      meta: { permission: 'admin', mobileBlocked: true },
    },
    {
      path: '/admin/languages',
      name: 'admin-languages',
      component: adminView(() => import('../views/admin/AdminLanguagesView.vue')),
      meta: { permission: 'admin', mobileBlocked: true },
    },
    {
      path: '/admin/discussions',
      name: 'admin-discussions',
      component: adminView(() => import('../views/admin/AdminDiscussionsView.vue')),
      meta: { permission: 'admin', mobileBlocked: true },
    },
    {
      path: '/admin/bots',
      name: 'admin-bots',
      component: adminView(() => import('../views/admin/AdminBotsView.vue')),
      meta: { permission: 'admin', mobileBlocked: true },
    },
    {
      path: '/admin/techdata/:tid(T[1-9][0-9]*)',
      name: 'admin-techdata-item',
      component: adminView(() => import('../views/TechdataItemView.vue')),
      props: true,
      meta: { permission: 'techdata', mobileBlocked: true },
    },
    {
      path: '/admin/connections',
      name: 'admin-connections',
      component: adminView(() => import('../views/admin/AdminConnectionsView.vue')),
      meta: { permission: 'admin', mobileBlocked: true },
    },
    {
      path: '/admin/users',
      name: 'admin-users',
      component: adminView(() => import('../views/admin/AdminUsersView.vue')),
      meta: { permission: 'admin', mobileBlocked: true },
    },
    {
      path: '/:pathMatch(.*)*',
      name: 'not-found',
      component: () => import('../views/NotFoundView.vue'),
    },
  ],
  scrollBehavior: () => ({ top: 0 }),
})

router.onError(reportNavigationFailure)
router.afterEach((_to, _from, failure) => { if (!failure) navigationFailed.value = false })

router.beforeEach(async (to) => {
  if (to.params.locale) {
    try { Intl.getCanonicalLocales(String(to.params.locale)) } catch { return { name: 'language-portal' } }
  }
  if (isMobileViewerViewport()) {
    if (to.name === 'article' && to.query.tab === 'discussion') {
      const query = { ...to.query }
      delete query.tab
      return { name: 'article', params: to.params, query, hash: to.hash }
    }
    if (to.meta.mobileBlocked) {
      if (to.name === 'editor' && to.params.locale && to.params.name) {
        return { name: 'article', params: { locale: to.params.locale, name: to.params.name } }
      }
      return { name: 'language-portal' }
    }
  }
  const permission = to.meta.permission as Permission | undefined
  if (!to.meta.requiresAuth && !permission) return true

  const { hasPermission, loadSession } = await import('../services/auth')
  const user = await loadSession()
  if (to.meta.requiresAuth && !user) {
    return {
      name: 'login',
      query: { redirect: to.fullPath },
    }
  }
  if (permission && (!user || !hasPermission(permission))) {
    if (!user) {
      return { name: 'login', query: { redirect: to.fullPath } }
    }
    return { name: 'language-portal' }
  }
  return true
})
