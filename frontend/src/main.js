import { createApp } from 'vue'
import App from './App.vue'
import './styles.css'
import { loadLocale, storedLocale } from './i18n'

await loadLocale(storedLocale())
createApp(App).mount('#root')
