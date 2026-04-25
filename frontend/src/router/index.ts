import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore, type PropertyModule } from '@/stores/auth'
import { usePropertyStore } from '@/stores/property'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', component: () => import('@/views/LoginView.vue'), meta: { public: true } },
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
