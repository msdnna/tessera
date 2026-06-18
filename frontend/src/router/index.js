import { createRouter, createWebHistory } from 'vue-router'

const routes = [
  { path: '/login', component: () => import('@/views/LoginView.vue'), meta: { public: true } },
  {
    path: '/register',
    component: () => import('@/views/RegisterView.vue'),
    meta: { public: true },
  },
  // Open routes reached from email links — accessible signed in or out, no bounce.
  {
    path: '/forgot-password',
    component: () => import('@/views/ForgotPasswordView.vue'),
    meta: { open: true },
  },
  {
    path: '/reset-password',
    component: () => import('@/views/ResetPasswordView.vue'),
    meta: { open: true },
  },
  {
    path: '/verify-email',
    component: () => import('@/views/VerifyEmailView.vue'),
    meta: { open: true },
  },
  // Accepting an invite needs a signed-in session (the email must match).
  { path: '/invite', component: () => import('@/views/AcceptInviteView.vue') },
  {
    path: '/',
    component: () => import('@/components/AppLayout.vue'),
    children: [
      { path: '', component: () => import('@/views/HomeView.vue') },
      { path: 'project/:projectSlug/board/:boardSlug', component: () => import('@/views/BoardView.vue') },
      // Legacy / deep links by id (UUID or bare slug) — BoardView resolves them
      // and canonicalizes the URL to /project/<slug>/board/<slug>.
      { path: 'board/:id', component: () => import('@/views/BoardView.vue') },
      { path: 'notes', component: () => import('@/views/NotesView.vue') },
      { path: 'reminders', component: () => import('@/views/RemindersView.vue') },
      { path: 'settings', component: () => import('@/views/SettingsView.vue') },
      {
        path: 'admin',
        component: () => import('@/views/AdminView.vue'),
        meta: { admin: true },
      },
    ],
  },
]

const router = createRouter({ history: createWebHistory(), routes })

router.beforeEach((to) => {
  const token = localStorage.getItem('tessera_token')
  if (to.meta.open) return // email-link landing pages: always accessible
  if (!to.meta.public && !token) return { path: '/login', query: { next: to.fullPath } }
  if (to.meta.public && token) return { path: '/' }
  // Admin-only routes: bounce non-admins home (the server also enforces this).
  if (to.meta.admin) {
    const u = JSON.parse(localStorage.getItem('tessera_user') || 'null')
    if (!u?.is_admin) return { path: '/' }
  }
})

export default router
