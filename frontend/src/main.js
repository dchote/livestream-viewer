import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import vuetify from './plugins/vuetify'
import router from './plugins/router'
import { useAuthStore } from './stores/auth'

const app = createApp(App)
const pinia = createPinia()
app.use(pinia)
app.use(vuetify)

const auth = useAuthStore()
auth.restore()

async function start() {
  if (auth.token) {
    await auth.fetchMe()
  }
  app.use(router)
  app.mount('#app')
}

start()
