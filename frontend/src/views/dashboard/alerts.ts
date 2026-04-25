export interface DashboardAlert {
  id: string
  tone: 'warning' | 'danger' | 'info'
  title: string
  detail: string
  to?: string
}
