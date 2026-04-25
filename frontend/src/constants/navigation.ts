import type { PropertyModule } from '@/stores/auth'

export interface AppNavItem {
  to: string
  label: string
  module?: PropertyModule
}

export const APP_NAV_ITEMS: AppNavItem[] = [
  { to: '/', label: 'Dashboard' },
  { to: '/properties', label: 'Properties' },
  { to: '/occupancy', label: 'Occupancy', module: 'occupancy' },
  { to: '/nuki', label: 'Nuki Access', module: 'nuki_access' },
  { to: '/cleaning', label: 'Cleaning', module: 'cleaning_log' },
  { to: '/finance', label: 'Finance', module: 'finance' },
  { to: '/finance/booking-payouts', label: 'Booking Payouts', module: 'finance' },
  { to: '/invoices', label: 'Invoices', module: 'invoices' },
  { to: '/messages', label: 'Messages', module: 'messages' },
  { to: '/analytics', label: 'Analytics', module: 'analytics' },
]
