import { createApp } from 'vue'
import App from './App.vue'
import './styles.css'
import { loadLocale, storedLocale } from './i18n'

if (navigator.userAgent.includes('Mac')) document.documentElement.classList.add('mac')
await loadLocale(storedLocale())
createApp(App).mount('#root')
