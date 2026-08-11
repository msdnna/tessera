import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'
import { useAuthStore } from './stores/auth'
import './styles/main.css'

const app = createApp(App)
app.use(createPinia())

// #2684: the access token lives in memory, so every page load starts signed out
// and the session is restored from the httpOnly refresh cookie. This has to
// finish BEFORE the router is installed — the first navigation guard would
// otherwise run against an empty store and bounce a signed-in user to /login on
// every reload. bootstrap() never rejects and its request is time-bounded, so a
// dead backend delays the first paint rather than blocking it forever.
useAuthStore()
  .bootstrap()
  .finally(() => {
    app.use(router)
    app.mount('#app')
  })
