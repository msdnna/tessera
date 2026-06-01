import { createRouter, createWebHistory } from 'vue-router'

const routes = [
  { path: '/login', component: () => import('@/views/LoginView.vue'), meta: { public: true } },
  {
    path: '/register',
    component: () => import('@/views/RegisterView.vue'),
    meta: { public: true },
  },
  {
    path: '/',
    component: () => import('@/components/AppLayout.vue'),
    children: [
      { path: '', component: () => import('@/views/HomeView.vue') },
      { path: 'board/:id', component: () => import('@/views/BoardView.vue') },
      { path: 'notes', component: () => import('@/views/NotesView.vue') },
      { path: 'reminders', component: () => import('@/views/RemindersView.vue') },
    ],
  },
]

const router = createRouter({ history: createWebHistory(), routes })

router.beforeEach((to) => {
  const token = localStorage.getItem('tessera_token')
  if (!to.meta.public && !token) return { path: '/login' }
  if (to.meta.public && token) return { path: '/' }
})

export default router
