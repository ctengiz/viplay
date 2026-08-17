import { locales, messages } from '../src/i18n.js'
import { readFile } from 'node:fs/promises'

const listed = new Set(locales.map(({ code }) => code))
const catalogCodes = Object.keys(messages)

for (const code of catalogCodes) {
  if (!listed.has(code)) throw new Error(`Locale catalog "${code}" is not listed in the language selector.`)
  for (const [key, value] of Object.entries(messages[code])) {
    if (!value.trim()) throw new Error(`Locale "${code}" has an empty value for "${key}".`)
  }
}

for (const code of listed) {
  if (!messages[code]) throw new Error(`Language selector locale "${code}" has no message catalog.`)
}

const appSource = await readFile(new URL('../src/App.vue', import.meta.url), 'utf8')
const usedKeys = [...appSource.matchAll(/\bt\('([^']+)'/g)].map((match) => match[1])
for (const key of usedKeys) {
  if (!messages.en[key]) throw new Error(`App.vue uses unknown translation key "${key}".`)
}

console.log(`Validated ${catalogCodes.length} locale catalogs with ${Object.keys(messages.en).length} keys each and ${new Set(usedKeys).size} UI keys in use.`)
