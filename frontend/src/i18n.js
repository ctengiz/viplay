import { ref } from 'vue'
import { GetLocalization } from '../bindings/viplay/app'

export const locale = ref('en')
export const locales = ref([])
const messages = ref({})

export function storedLocale() {
  try { return window.localStorage.getItem('viplay.locale') || 'en' } catch { return 'en' }
}

export async function loadLocale(requestedLocale) {
  const payload = await GetLocalization(requestedLocale)
  locale.value = payload.locale
  locales.value = payload.locales
  messages.value = payload.messages
  document.documentElement.lang = payload.locale
  try { window.localStorage.setItem('viplay.locale', payload.locale) } catch { /* Persistence is optional. */ }
  return payload.locale
}

export function t(key, values = {}) {
  const template = messages.value[key] ?? key
  return template.replace(/\{(\w+)\}/g, (_, name) => values[name] ?? `{${name}}`)
}
