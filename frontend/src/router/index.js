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
  // Accepting an invite needs a signed-in session (the email must match), but the
  // page itself is `open`: an invitee with no account has to reach it so it can
  // pass the token on to the register form. Bounced to /login instead, the token
  // was lost and only the email was left to join on — which is exactly what must
  // not grant access.
  {
    path: '/invite',
    component: () => import('@/views/AcceptInviteView.vue'),
    meta: { open: true },
  },
  // GitLab OAuth handoff: parses the session from the URL fragment. `open` so it
  // runs regardless of the (about-to-change) auth state.
  {
    path: '/oauth/callback',
    component: () => import('@/views/OAuthCallbackView.vue'),
    meta: { open: true },
  },
  // Server-side recording template (#2877), opened only by egress's headless
  // Chrome — never a person. `open` because egress carries no Tessera session,
  // only a LiveKit token in the URL; standalone (outside AppLayout) so the
  // recording is the call and nothing else — no sidebar, no topbar.
  {
    path: '/rec/egress',
    component: () => import('@/views/RecorderView.vue'),
    meta: { open: true },
  },
  // The document editor on its own, for the Android app to embed in a WebView
  // (#2894 §4). Standalone like /rec/egress above and for the same reason — no
  // sidebar, no topbar, nothing but the editing surface, because the chrome
  // around it is the app's and is native.
  //
  // `open` because the session does not exist yet when this loads: the host
  // hands over an access token in the URL and the view installs it. The guard
  // would otherwise bounce the WebView to /login before the view ever runs.
  {
    path: '/embed/document/:slug',
    component: () => import('@/views/DocEmbedView.vue'),
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
      // The stage page (#2993): slugs like the board's — the stage slug is
      // unique within its project (milestones.slug).
      {
        path: 'project/:projectSlug/stage/:stageSlug',
        component: () => import('@/views/StageView.vue'),
      },
      // Legacy / deep links by id (UUID or bare slug) — BoardView resolves them
      // and canonicalizes the URL to /project/<slug>/board/<slug>.
      { path: 'board/:id', component: () => import('@/views/BoardView.vue') },
      // The «Заметки» and «Напоминания» sections became the «Входящие» panel
      // (#2999): old links land on the start screen with the panel open on
      // that filter (and on the note a `?note=` named). AppLayout reads the
      // query and drops it.
      {
        path: 'notes',
        redirect: (to) => ({ path: '/', query: { inbox: 'notes', note: to.query.note } }),
      },
      // Lazy on purpose: D2 brings ProseMirror in here, and the board must not
      // carry its weight.
      // One record with an optional param, not two sibling records: the sidebar
      // link points at /documents, and vue-router marks a link active only when
      // the open route shares its record. As two records, opening a document
      // switched the record and the «Документы» item went dark (#2727).
      { path: 'documents/:slug?', component: () => import('@/views/DocumentsView.vue') },
      // Same one-record-with-an-optional-param shape as documents above, and for
      // the same reason (#2727): opening a conference must not take the
      // «Конференции» item in the sidebar dark.
      { path: 'conferences/:id?', component: () => import('@/views/ConferencesView.vue') },
      { path: 'reminders', redirect: () => ({ path: '/', query: { inbox: 'reminders' } }) },
      // No help route (#2792): the help centre is a modal mounted in AppLayout,
      // opened from the sidebar's «Помощь» menu. Cross-links between articles are
      // written as /help/<slug> and intercepted by HelpArticle, so they swap the
      // open article instead of navigating anywhere.
      { path: 'milestones', component: () => import('@/views/MilestonesView.vue') },
      { path: 'settings', component: () => import('@/views/SettingsView.vue') },
      // The phone's «Ещё» tab and the notification feed as a page (#2976 —
      // package G). Both send a wider window elsewhere: «Ещё» to the settings,
      // the feed to the bell's popover.
      { path: 'more', component: () => import('@/views/MoreView.vue') },
      { path: 'notifications', component: () => import('@/views/NotificationsView.vue') },
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
