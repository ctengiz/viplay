import { readFile } from 'node:fs/promises'

const catalogPath = new URL('../../locales/catalogs.json', import.meta.url)
const catalog = JSON.parse(await readFile(catalogPath, 'utf8'))
const listed = new Set(catalog.locales.map(({ code }) => code))
const catalogCodes = Object.keys(catalog.messages)
const defaultMessages = catalog.messages[catalog.defaultLocale]

if (!defaultMessages) throw new Error(`Default locale "${catalog.defaultLocale}" has no catalog.`)

for (const code of catalogCodes) {
  if (!listed.has(code)) throw new Error(`Locale catalog "${code}" is not listed in the language selector.`)
  const keys = Object.keys(catalog.messages[code])
  if (keys.length !== Object.keys(defaultMessages).length) throw new Error(`Locale "${code}" does not match the default catalog key count.`)
  for (const key of Object.keys(defaultMessages)) {
    if (!catalog.messages[code][key]?.trim()) throw new Error(`Locale "${code}" has no value for "${key}".`)
  }
}

for (const code of listed) {
  if (!catalog.messages[code]) throw new Error(`Language selector locale "${code}" has no message catalog.`)
}

const appSource = await readFile(new URL('../src/App.vue', import.meta.url), 'utf8')
const usedKeys = [...appSource.matchAll(/\bt\('([^']+)'/g)].map((match) => match[1])
for (const key of usedKeys) {
  if (!defaultMessages[key]) throw new Error(`App.vue uses unknown translation key "${key}".`)
}

console.log(`Validated ${catalogCodes.length} locale catalogs with ${Object.keys(defaultMessages).length} keys each and ${new Set(usedKeys).size} UI keys in use.`)
