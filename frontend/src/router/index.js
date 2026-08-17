import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

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
    // Neutral path: the «reset-password» URL tripped a recipient spam filter
    // (Yandex) on reset emails. /reset-password kept as an alias for old links.
    path: '/recover',
    alias: '/reset-password',
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
  // GitLab OAuth handoff: parses the session from the URL fragment. `open` so it
  // runs regardless of the (about-to-change) auth state.
  {
    path: '/oauth/callback',
    component: () => import('@/views/OAuthCallbackView.vue'),
    meta: { open: true },
  },
  {
    path: '/',
    component: () => import('@/components/AppLayout.vue'),
    children: [
      { path: '', component: () => import('@/views/HomeView.vue') },
      {
        path: 'project/:projectSlug/board/:boardSlug',
        component: () => import('@/views/BoardView.vue'),
      },
      // Legacy / deep links by id (UUID or bare slug) — BoardView resolves them
      // and canonicalizes the URL to /project/<slug>/board/<slug>.
      { path: 'board/:id', component: () => import('@/views/BoardView.vue') },
      { path: 'notes', component: () => import('@/views/NotesView.vue') },
      // Lazy on purpose: D2 brings ProseMirror in here, and the board must not
      // carry its weight.
      // One record with an optional param, not two sibling records: the sidebar
      // link points at /documents, and vue-router marks a link active only when
      // the open route shares its record. As two records, opening a document
      // switched the record and the «Документы» item went dark (#2727).
      { path: 'documents/:slug?', component: () => import('@/views/DocumentsView.vue') },
      { path: 'reminders', component: () => import('@/views/RemindersView.vue') },
      { path: 'milestones', component: () => import('@/views/MilestonesView.vue') },
      { path: 'settings', component: () => import('@/views/SettingsView.vue') },
      {
        path: 'admin',
        component: () => import('@/views/AdminView.vue'),
        meta: { admin: true },
      },
    ],
  },
  // Catch-all 404 — branded page instead of a blank screen. Open so the guard
  // never bounces it to /login.
  {
    path: '/:pathMatch(.*)*',
    component: () => import('@/views/NotFoundView.vue'),
    meta: { open: true },
  },
]

const router = createRouter({ history: createWebHistory(), routes })

// The guard reads the auth store, not localStorage: since #2684 the access token
// lives in memory only, so the store is the single source of truth for "signed
// in". main.js restores the session before installing the router, so the very
// first navigation already sees the real state.
router.beforeEach((to) => {
  const auth = useAuthStore()
  if (to.meta.open) return // email-link landing pages: always accessible
  if (!to.meta.public && !auth.isAuthenticated)
    return { path: '/login', query: { next: to.fullPath } }
  if (to.meta.public && auth.isAuthenticated) return { path: '/' }
  // Admin-only routes: bounce non-admins home (the server also enforces this).
  if (to.meta.admin && !auth.isAdmin) return { path: '/' }
})

export default router
