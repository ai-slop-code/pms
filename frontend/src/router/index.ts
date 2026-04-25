import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore, type PropertyModule } from '@/stores/auth'
import { usePropertyStore } from '@/stores/property'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', component: () => import('@/views/LoginView.vue'), meta: { public: true } },
    {
      path: '/provisioning',
      name: 'provisioning',
      component: () => import('@/views/ProvisioningView.vue'),
      // Reachable as soon as the user is authenticated. The guard below
      // makes sure non-gated users don't end up here, and that gated users
      // can't navigate anywhere else.
      meta: { provisioning: true },
    },
    {
      path: '/',
      component: () => import('@/views/ShellView.vue'),
      meta: { requiresAuth: true },
      children: [
        { path: '', name: 'dashboard', component: () => import('@/views/DashboardView.vue') },
        { path: 'properties', name: 'properties', component: () => import('@/views/PropertiesView.vue') },
        { path: 'properties/new', name: 'property-new', component: () => import('@/views/PropertyFormView.vue') },
        { path: 'properties/:id', name: 'property-detail', component: () => import('@/views/PropertyDetailView.vue') },
        {
          path: 'occupancy',
          name: 'occupancy',
          component: () => import('@/views/OccupancyView.vue'),
          meta: { module: 'occupancy' },
        },
        {
          path: 'nuki',
          name: 'nuki',
          component: () => import('@/views/NukiView.vue'),
          meta: { module: 'nuki_access' },
        },
        {
          path: 'cleaning',
          name: 'cleaning',
          component: () => import('@/views/CleaningView.vue'),
          meta: { module: 'cleaning_log' },
        },
        {
          path: 'finance',
          name: 'finance',
          component: () => import('@/views/FinanceView.vue'),
          meta: { module: 'finance' },
        },
        {
          path: 'finance/booking-payouts',
          name: 'finance-booking-payouts',
          component: () => import('@/views/BookingPayoutsView.vue'),
          meta: { module: 'finance' },
        },
        {
          path: 'invoices',
          name: 'invoices',
          component: () => import('@/views/InvoicesView.vue'),
          meta: { module: 'invoices' },
        },
        {
          path: 'messages',
          name: 'messages',
          component: () => import('@/views/MessagesView.vue'),
          meta: { module: 'messages' },
        },
        {
          path: 'analytics',
          name: 'analytics',
          component: () => import('@/views/AnalyticsView.vue'),
          meta: { module: 'analytics' },
        },
        { path: 'users', name: 'users', component: () => import('@/views/UsersView.vue'), meta: { superAdmin: true } },
        { path: 'users/:id', name: 'user-detail', component: () => import('@/views/UserDetailView.vue'), meta: { superAdmin: true } },
        ...(import.meta.env.DEV
          ? [
              {
                path: 'ui-kit',
                name: 'ui-kit',
                component: () => import('@/views/UiKitView.vue'),
                meta: { dev: true },
              },
            ]
          : []),
      ],
    },
  ],
})

router.beforeEach(async (to) => {
  const auth = useAuthStore()
  const props = usePropertyStore()
  if (!auth.loaded) {
    await auth.refreshMe()
  }
  if (to.meta.public) {
    if (auth.user && to.path === '/login') return { path: '/' }
    return true
  }
  // Provisioning gate: an authenticated user with an unrotated bootstrap
  // password (or a super-admin without TOTP) must finish setup before
  // anything else. Funnel them to /provisioning; bounce them away once
  // there's nothing left to do so the screen doesn't get sticky.
  if (to.meta.provisioning) {
    if (!auth.user) return { path: '/login' }
    if (!auth.provisioningRequired) return { path: '/' }
    return true
  }
  if (auth.user && auth.provisioningRequired) {
    return { path: '/provisioning' }
  }
  if (to.meta.requiresAuth && !auth.user) {
    return { path: '/login', query: { redirect: to.fullPath } }
  }
  if (to.meta.superAdmin && auth.user?.role !== 'super_admin') {
    return { path: '/' }
  }
  if (typeof to.meta.module === 'string') {
    props.loadStored()
    if (!props.list.length) {
      await props.fetchList()
    }
    const property = props.list.find((p) => p.id === props.currentId) ?? null
    if (!property) {
      return { path: '/properties' }
    }
    if (!auth.canAccessPropertyModule(property, to.meta.module as PropertyModule)) {
      return { path: '/' }
    }
  }
  return true
})

export default router
