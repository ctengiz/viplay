<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { AlertCircle, Captions, CheckCircle2, ChevronDown, ChevronRight, Film, Flag, FolderOpen, Gauge, Images, Keyboard, Languages, ListVideo, LoaderCircle, Maximize, Minimize, Pause, Play, RotateCcw, RotateCw, Scissors, SkipBack, SkipForward, Speaker, Trash2, Volume2, VolumeX, X } from '@lucide/vue'
import { Window } from '@wailsio/runtime'
import { DeleteVideo, DirectoryVideos, ExtractContactSheet, MarkPlayed, OpenSubtitle, OpenVideos, ProbeMedia, RecentVideos, SplitVideo, SplitVideoAtMarkers } from '../bindings/viplay/app'
import { loadLocale, locale, locales, t } from './i18n'

const video = ref(null)
const queue = ref([])
const index = ref(0)
const playing = ref(false)
const current = ref(0)
const duration = ref(0)
const volume = ref(.75)
const muted = ref(false)
const speed = ref(1)
const seekStep = ref(10)
const fullscreenActive = ref(false)
const showQueue = ref(true)
const subtitle = ref(null)
const showShortcuts = ref(false)
const showSheetDialog = ref(false)
const showDeleteDialog = ref(false)
const sheetFrameCount = ref(12)
const sheetImageWidth = ref(320)
const mediaInfo = ref(null)
const libraryView = ref('folder')
const processing = ref('')
const notice = ref(null)
const splitMarkers = ref([])
let noticeTimer
const item = computed(() => queue.value[index.value])
const progress = computed(() => `${duration.value ? current.value / duration.value * 100 : 0}%`)
const markerPositions = computed(() => splitMarkers.value.map(seconds => ({ seconds, left: `${seconds / duration.value * 100}%` })))
const volumeProgress = computed(() => `${volume.value * 100}%`)
const speeds = [.5, .75, 1, 1.25, 1.5, 2]
const sheetSizes = computed(() => [
  { label: t('sheet.size.small'), value: 240 },
  { label: t('sheet.size.medium'), value: 320 },
  { label: t('sheet.size.large'), value: 480 },
  { label: t('sheet.size.xlarge'), value: 640 },
])
const sheetSpacing = computed(() => duration.value > 0 ? duration.value / (Number(sheetFrameCount.value) + 1) : 0)

function notify(type, title, detail = '') {
  window.clearTimeout(noticeTimer)
  notice.value = { type, title, detail }
  if (type !== 'progress') noticeTimer = window.setTimeout(() => { notice.value = null }, type === 'error' ? 15000 : 5000)
}

function fmt(value) {
  if (!Number.isFinite(value)) return '00:00'
  const h = Math.floor(value / 3600)
  const m = Math.floor((value % 3600) / 60)
  const s = Math.floor(value % 60)
  return h ? `${h}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}` : `${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
}

async function openVideos() {
  const items = await OpenVideos()
  if (!items?.length) return
  const directory = await DirectoryVideos(items[0].path)
  queue.value = directory
  const selectedIndex = directory.findIndex(entry => entry.path === items[0].path)
  index.value = Math.max(0, selectedIndex)
  libraryView.value = 'folder'
}

async function loadRecent() {
  const items = await RecentVideos()
  queue.value = items || []
  index.value = 0
  libraryView.value = 'recent'
}

async function markPlayed() {
  playing.value = true
  if (item.value?.path) await MarkPlayed(item.value.path)
}

async function splitCurrent() {
  if (!item.value) { notify('error', t('error.noVideoTitle'), t('error.noVideoSplit')); return }
  if (processing.value) return
  if (video.value && !video.value.paused) video.value.pause()
  const splitAt = video.value?.currentTime ?? current.value
  if (splitAt <= 0 || splitAt >= duration.value) { notify('error', t('error.invalidSplitTitle'), t('error.invalidSplit')); return }
  processing.value = 'split'
  notify('progress', t('split.progressTitle'), t('split.progress', { time: fmt(splitAt) }))
  await nextTick()
  try {
    const result = await SplitVideo(item.value.path, splitAt)
    notify('success', t('split.successTitle'), `${fmt(result.splitTime)} · ${result.firstPath} · ${result.secondPath}`)
    if (libraryView.value === 'folder') {
      const activePath = item.value.path
      queue.value = await DirectoryVideos(activePath)
      index.value = queue.value.findIndex(entry => entry.path === activePath)
    }
  } catch (error) { notify('error', t('split.errorTitle'), String(error)) }
  finally { processing.value = '' }
}

function toggleSplitMarker() {
  if (!item.value || !duration.value) { notify('error', t('error.noVideoTitle'), t('error.noVideoSplit')); return }
  if (processing.value) return
  const seconds = video.value?.currentTime ?? current.value
  if (seconds <= .05 || seconds >= duration.value - .05) { notify('error', t('error.invalidSplitTitle'), t('error.invalidSplit')); return }
  const existing = splitMarkers.value.findIndex(marker => Math.abs(marker - seconds) < .25)
  if (existing >= 0) {
    splitMarkers.value.splice(existing, 1)
    return
  }
  if (splitMarkers.value.length >= 100) { notify('error', t('markers.limitTitle'), t('markers.limit')); return }
  splitMarkers.value = [...splitMarkers.value, seconds].sort((a, b) => a - b)
}

function removeSplitMarker(seconds) {
  splitMarkers.value = splitMarkers.value.filter(marker => marker !== seconds)
}

async function splitAtMarkers() {
  if (!splitMarkers.value.length) return splitCurrent()
  if (!item.value || processing.value) return
  if (video.value && !video.value.paused) video.value.pause()
  const markers = [...splitMarkers.value]
  processing.value = 'split'
  notify('progress', t('multiSplit.progressTitle'), t('multiSplit.progress', { count: markers.length, parts: markers.length + 1 }))
  await nextTick()
  try {
    const result = await SplitVideoAtMarkers(item.value.path, markers)
    splitMarkers.value = []
    notify('success', t('multiSplit.successTitle'), t('multiSplit.success', { count: result.paths.length }))
    if (libraryView.value === 'folder') {
      const activePath = item.value.path
      queue.value = await DirectoryVideos(activePath)
      index.value = queue.value.findIndex(entry => entry.path === activePath)
    }
  } catch (error) { notify('error', t('split.errorTitle'), String(error)) }
  finally { processing.value = '' }
}

function createContactSheet() {
  if (!item.value) { notify('error', t('error.noVideoTitle'), t('error.noVideoSheet')); return }
  if (processing.value) return
  showSheetDialog.value = true
}

async function runContactSheet() {
  const count = Math.round(Number(sheetFrameCount.value))
  const imageWidth = Number(sheetImageWidth.value)
  if (!Number.isFinite(count) || count < 1 || count > 60) { notify('error', t('error.frameCountTitle'), t('error.frameCount')); return }
  showSheetDialog.value = false
  processing.value = 'sheet'
  notify('progress', t('sheet.progressTitle'), t('sheet.progress', { count, seconds: sheetSpacing.value.toFixed(1) }))
  await nextTick()
  try {
    const output = await ExtractContactSheet(item.value.path, count, imageWidth)
    notify('success', t('sheet.successTitle'), output)
  } catch (error) { notify('error', t('sheet.errorTitle'), String(error)) }
  finally { processing.value = '' }
}

async function openSubtitle() {
  const selected = await OpenSubtitle()
  if (selected?.url) subtitle.value = selected
}

function toggle() {
  if (!video.value || !item.value) return openVideos()
  video.value.paused ? video.value.play() : video.value.pause()
}

function seekBy(delta) {
  if (video.value) video.value.currentTime = Math.max(0, Math.min(duration.value, video.value.currentTime + delta))
}

function normaliseSeekStep() {
  seekStep.value = Math.min(300, Math.max(1, Math.round(Number(seekStep.value) || 10)))
}

function seek(event) {
  if (video.value) video.value.currentTime = Number(event.target.value)
}

async function fullscreen() {
  await Window.ToggleFullscreen()
  fullscreenActive.value = await Window.IsFullscreen()
}
async function select(i, autoplay = false) {
  index.value = i
  playing.value = false
  current.value = 0
  subtitle.value = null
  if (autoplay) {
    await nextTick()
    try { await video.value?.play() } catch { /* Playback remains available through the user controls. */ }
  }
}
function next() { if (index.value < queue.value.length - 1) select(index.value + 1, true) }
function previous() { current.value > 3 ? seekBy(-current.value) : index.value > 0 && select(index.value - 1, true) }
function title(name) { return name.replace(/\.[^/.]+$/, '') }
function fileSize(value) { return value ? `${(value / 1024 / 1024).toFixed(1)} MB` : '—' }

async function navigateDirectory(direction) {
  if (!item.value) return
  const activePath = item.value.path
  const items = await DirectoryVideos(activePath)
  const activeIndex = items.findIndex(entry => entry.path === activePath)
  const target = activeIndex + direction
  if (target < 0 || target >= items.length) return
  queue.value = items
  select(target, true)
}

function requestDelete() {
  if (!item.value) { notify('error', t('error.noVideoTitle'), t('delete.noVideo')); return }
  if (processing.value) { notify('error', t('delete.busyTitle'), t('delete.busy')); return }
  showDeleteDialog.value = true
}

async function deleteCurrent() {
  if (!item.value || processing.value) return
  const deleting = item.value
  showDeleteDialog.value = false
  processing.value = 'delete'
  try {
    await DeleteVideo(deleting.path)
    queue.value.splice(index.value, 1)
    if (index.value >= queue.value.length) index.value = Math.max(0, queue.value.length - 1)
    current.value = 0
    playing.value = false
    subtitle.value = null
    mediaInfo.value = null
    notify('success', t('delete.successTitle'), deleting.name)
  } catch (error) { notify('error', t('delete.errorTitle'), String(error)) }
  finally { processing.value = '' }
}

function loaded(event) {
  duration.value = event.currentTarget.duration
  event.currentTarget.volume = volume.value
  event.currentTarget.playbackRate = speed.value
  mediaInfo.value = { ...mediaInfo.value, width: mediaInfo.value?.width || event.currentTarget.videoWidth, height: mediaInfo.value?.height || event.currentTarget.videoHeight, duration: mediaInfo.value?.duration || event.currentTarget.duration }
}

function onKey(event) {
  if (event.key === 'Escape' && showDeleteDialog.value) { showDeleteDialog.value = false; return }
  if (!event.repeat && event.key === 'Enter' && showDeleteDialog.value) { event.preventDefault(); deleteCurrent(); return }
  if (event.key === 'Escape' && showSheetDialog.value) { showSheetDialog.value = false; return }
  if (event.key === 'Escape' && showShortcuts.value) { showShortcuts.value = false; return }
  if (showDeleteDialog.value || showShortcuts.value) return
  if (['INPUT', 'BUTTON'].includes(document.activeElement?.tagName) && event.code === 'Space') return
  const player = video.value
  if (event.metaKey && event.code === 'ArrowRight') { event.preventDefault(); navigateDirectory(1); return }
  if (event.metaKey && event.code === 'ArrowLeft') { event.preventDefault(); navigateDirectory(-1); return }
  if (!event.repeat && (event.code === 'Delete' || (event.metaKey && event.code === 'Backspace'))) { event.preventDefault(); requestDelete(); return }
  if (!event.repeat && event.code === 'KeyB' && player) { event.preventDefault(); toggleSplitMarker(); return }
  if (event.code === 'Space' && player) { event.preventDefault(); player.paused ? player.play() : player.pause() }
  if (event.code === 'ArrowRight' && player) player.currentTime = Math.min(player.duration || 0, player.currentTime + seekStep.value)
  if (event.code === 'ArrowLeft' && player) player.currentTime = Math.max(0, player.currentTime - seekStep.value)
  if (event.key.toLowerCase() === 'f') fullscreen()
  if (event.key.toLowerCase() === 'm') muted.value = !muted.value
}

watch(speed, value => { if (video.value) video.value.playbackRate = value })
watch(volume, value => { if (video.value) video.value.volume = value })
watch(item, async value => {
  splitMarkers.value = []
  mediaInfo.value = value ? await ProbeMedia(value.path) : null
}, { immediate: true })
async function changeLanguage(event) {
  await loadLocale(event.target.value)
}
onMounted(() => {
  window.addEventListener('keydown', onKey)
})
onBeforeUnmount(() => { window.removeEventListener('keydown', onKey); window.clearTimeout(noticeTimer) })
</script>

<template>
  <main class="app" :class="{ 'queue-closed': !showQueue }">
    <aside class="sidebar">
      <div class="brand"><span class="brand-mark"><Play :size="14" fill="currentColor" /></span>ViPlay</div>
      <nav>
        <span class="nav-title">{{ t('library.title') }}</span>
        <button class="open-button" @click="openVideos"><FolderOpen :size="19" />{{ t('library.openVideo') }}</button>
        <button class="nav-item" :class="{ selected: libraryView === 'folder' }"><Film :size="18" />{{ t('library.allVideos') }}<span>{{ libraryView === 'folder' ? queue.length : '' }}</span></button>
        <button class="nav-item" :class="{ selected: libraryView === 'recent' }" @click="loadRecent"><RotateCcw :size="18" />{{ t('library.recent') }}</button>
      </nav>
      <div class="media-sidebar">
        <span>{{ t('mediaInfo.title') }}</span>
        <dl v-if="item">
          <div><dt>{{ t('mediaInfo.video') }}</dt><dd>{{ mediaInfo?.videoCodec?.toUpperCase() || t('mediaInfo.unknown') }}</dd></div>
          <div><dt>{{ t('mediaInfo.audio') }}</dt><dd>{{ mediaInfo?.audioCodec?.toUpperCase() || t('mediaInfo.unknown') }}</dd></div>
          <div><dt>{{ t('mediaInfo.dimensions') }}</dt><dd>{{ mediaInfo?.width ? `${mediaInfo.width}×${mediaInfo.height}` : '—' }}</dd></div>
          <div><dt>{{ t('mediaInfo.format') }}</dt><dd>{{ mediaInfo?.container?.toUpperCase() || '—' }}</dd></div>
          <div><dt>FPS</dt><dd>{{ mediaInfo?.fps || '—' }}</dd></div>
          <div><dt>{{ t('mediaInfo.file') }}</dt><dd>{{ fileSize(mediaInfo?.size) }}</dd></div>
        </dl>
        <p v-else>{{ t('mediaInfo.empty') }}</p>
      </div>
      <button class="shortcut-trigger" @click="showShortcuts = true"><Keyboard :size="18" /><span>{{ t('shortcuts.open') }}</span></button>
    </aside>

    <section class="player-shell">
      <header class="topbar">
        <div><span class="eyebrow">{{ t('nowPlaying') }}</span><strong :title="item?.name">{{ item?.name || t('selectVideo') }}</strong></div>
        <div class="topbar-actions">
          <label class="language-picker" :title="t('language.label')"><Languages :size="16" /><select :value="locale" :aria-label="t('language.label')" @change="changeLanguage"><option v-for="language in locales" :key="language.code" :value="language.code">{{ language.label }}</option></select><ChevronDown :size="12" /></label>
          <button class="icon-btn operation-btn" :class="{ processing: processing === 'split', marked: splitMarkers.length }" :aria-label="splitMarkers.length ? t('actions.multiSplit', { count: splitMarkers.length }) : t('actions.split')" :title="splitMarkers.length ? t('actions.multiSplitTitle', { count: splitMarkers.length }) : t('actions.splitTitle')" @click="splitAtMarkers"><LoaderCircle v-if="processing === 'split'" class="spin" :size="20" /><Scissors v-else :size="19" /><span v-if="splitMarkers.length" class="marker-count">{{ splitMarkers.length }}</span></button>
          <button class="icon-btn operation-btn" :class="{ processing: processing === 'sheet' }" :aria-label="t('actions.contactSheet')" :title="t('actions.contactSheetTitle')" @click="createContactSheet"><LoaderCircle v-if="processing === 'sheet'" class="spin" :size="20" /><Images v-else :size="19" /></button>
          <button class="icon-btn danger" :disabled="processing === 'delete'" :aria-label="t('actions.delete')" :title="t('actions.deleteTitle')" @click="requestDelete"><Trash2 :size="19" /></button>
          <button class="icon-btn" :class="{ active: showQueue }" :aria-label="t('actions.showQueue')" :title="t('actions.showQueue')" @click="showQueue = !showQueue"><ListVideo :size="20" /></button>
        </div>
      </header>

      <div class="stage" @dblclick="fullscreen">
        <video v-if="item" :key="item.url" ref="video" :src="item.url" :muted="muted" @click="toggle" @play="markPlayed" @pause="playing = false" @timeupdate="current = $event.currentTarget.currentTime" @loadedmetadata="loaded" @ended="next">
          <track v-if="subtitle" default kind="subtitles" :src="subtitle.url" srcLang="tr" :label="subtitle.name">
        </video>
        <div v-else class="empty-state">
          <div class="empty-icon"><Film :size="42" /></div>
          <h1>{{ t('empty.title') }}</h1><p>{{ t('empty.description') }}</p>
          <button @click="openVideos"><FolderOpen :size="18" />{{ t('library.openVideo') }}</button>
          <small>{{ t('empty.formats') }}</small>
        </div>
        <button v-if="item" class="center-play" :aria-label="playing ? t('player.pause') : t('player.play')" @click="toggle">
          <Pause v-if="playing" :size="34" fill="currentColor" /><Play v-else :size="36" fill="currentColor" />
        </button>
        <div class="bottom-fade" />
      </div>

      <div class="controls">
        <div class="timeline-row"><span>{{ fmt(current) }}</span><div class="timeline-track"><input :aria-label="t('player.progress')" type="range" min="0" :max="duration || 0" step=".1" :value="current" :style="{ '--progress': progress }" @input="seek"><button v-for="marker in markerPositions" :key="marker.seconds" class="split-marker" :style="{ left: marker.left }" :aria-label="t('markers.removeAt', { time: fmt(marker.seconds) })" :title="t('markers.removeAt', { time: fmt(marker.seconds) })" @click="removeSplitMarker(marker.seconds)"><span>{{ fmt(marker.seconds) }}</span></button></div><span>{{ fmt(duration) }}</span></div>
        <div class="control-row">
          <div class="volume-group">
            <button class="icon-btn" :aria-label="muted ? t('player.unmute') : t('player.mute')" @click="muted = !muted"><VolumeX v-if="muted || volume === 0" :size="20" /><Volume2 v-else :size="20" /></button>
            <input v-model.number="volume" :aria-label="t('player.volume')" type="range" min="0" max="1" step=".01" :style="{ '--progress': volumeProgress }">
          </div>
          <div class="transport">
            <button class="icon-btn" :aria-label="t('player.seekBack', { seconds: seekStep })" @click="seekBy(-seekStep)"><RotateCcw :size="19" /></button>
            <button class="icon-btn" :aria-label="t('player.previous')" @click="previous"><SkipBack :size="21" fill="currentColor" /></button>
            <button class="primary-play" :aria-label="playing ? t('player.pause') : t('player.play')" @click="toggle"><Pause v-if="playing" fill="currentColor" /><Play v-else fill="currentColor" /></button>
            <button class="icon-btn" :aria-label="t('player.next')" @click="next"><SkipForward :size="21" fill="currentColor" /></button>
            <button class="icon-btn" :aria-label="t('player.seekForward', { seconds: seekStep })" @click="seekBy(seekStep)"><RotateCw :size="19" /></button>
          </div>
          <div class="tools">
            <button class="tool marker-tool" :class="{ active: splitMarkers.length }" :disabled="!item || processing" :title="t('markers.addTitle')" @click="toggleSplitMarker"><Flag :size="18" /><span>{{ splitMarkers.length ? t('markers.count', { count: splitMarkers.length }) : t('markers.add') }}</span><kbd>B</kbd></button>
            <label class="seek-step" :title="t('player.seekDuration')"><input v-model.number="seekStep" :aria-label="t('player.seekSeconds')" type="number" min="1" max="300" step="1" @change="normaliseSeekStep"><span>{{ t('player.secondsShort') }}</span></label>
            <button class="tool" :class="{ active: subtitle }" @click="openSubtitle"><Captions :size="20" /><span>{{ t('player.subtitles') }}</span></button>
            <label class="speed"><Gauge :size="19" /><select v-model.number="speed" :aria-label="t('player.speed')"><option v-for="value in speeds" :key="value" :value="value">{{ value }}×</option></select><ChevronDown :size="14" /></label>
            <button class="icon-btn" :class="{ active: fullscreenActive }" :aria-label="fullscreenActive ? t('player.exitFullscreen') : t('player.fullscreen')" @click="fullscreen"><Minimize v-if="fullscreenActive" :size="20" /><Maximize v-else :size="20" /></button>
          </div>
        </div>
      </div>
    </section>

    <aside v-if="showQueue" class="queue">
      <div class="queue-head"><div><span>{{ t('queue.playlist') }}</span><strong>{{ t('queue.upNext') }}</strong></div><button class="icon-btn" :aria-label="t('queue.close')" @click="showQueue = false"><X :size="20" /></button></div>
      <div class="queue-list">
        <template v-if="queue.length">
          <button v-for="(entry, i) in queue" :key="entry.path" class="queue-item" :class="{ current: i === index }" @click="select(i, true)">
            <span class="queue-number"><Speaker v-if="i === index" :size="15" /><template v-else>{{ String(i + 1).padStart(2, '0') }}</template></span>
            <span class="thumb"><img v-if="entry.kind === 'video'" :src="entry.thumbnailUrl" alt="" loading="lazy" @error="$event.currentTarget.style.display = 'none'"><Film :size="22" /><i v-if="i === index && playing"><span /><span /><span /></i></span>
            <span class="queue-copy"><strong>{{ title(entry.name) }}</strong><small>{{ entry.kind === 'audio' ? t('mediaInfo.audio') : t('queue.localVideo') }}</small></span><ChevronRight :size="16" />
          </button>
        </template>
        <div v-else class="queue-empty"><ListVideo :size="28" /><p>{{ t('queue.empty') }}</p><button @click="openVideos">{{ t('queue.addVideo') }}</button></div>
      </div>
      <button class="add-more" @click="openVideos"><FolderOpen :size="17" />{{ t('queue.addMore') }}</button>
    </aside>

    <Transition name="notice">
      <div v-if="notice" class="operation-notice" :class="notice.type" role="status" aria-live="polite">
        <LoaderCircle v-if="notice.type === 'progress'" class="spin" :size="22" />
        <CheckCircle2 v-else-if="notice.type === 'success'" :size="22" />
        <AlertCircle v-else :size="22" />
        <span><strong>{{ notice.title }}</strong><small>{{ notice.detail }}</small></span><button class="notice-close" :aria-label="t('notice.close')" @click="notice = null"><X :size="15" /></button>
      </div>
    </Transition>

    <div v-if="showSheetDialog" class="shortcut-modal sheet-modal" role="dialog" aria-modal="true" :aria-label="t('sheet.settings')" @click.self="showSheetDialog = false">
      <section>
        <header><div><span>{{ t('sheet.extractor') }}</span><strong>{{ t('sheet.create') }}</strong></div><button class="icon-btn" :aria-label="t('common.close')" @click="showSheetDialog = false"><X :size="20" /></button></header>
        <p>{{ t('sheet.description') }}</p>
        <div class="sheet-options">
          <label><span>{{ t('sheet.frameCount') }}<small>{{ t('sheet.maxFrames') }}</small></span><span class="sheet-input"><input v-model.number="sheetFrameCount" type="number" min="1" max="60" step="1" autofocus><small>{{ t('sheet.frames') }}</small></span></label>
          <label><span>{{ t('sheet.interval') }}<small>{{ t('sheet.automatic') }}</small></span><output>{{ sheetSpacing ? t('sheet.seconds', { seconds: sheetSpacing.toFixed(1) }) : '—' }}</output></label>
          <label><span>{{ t('sheet.imageSize') }}<small>{{ t('sheet.frameDimensions') }}</small></span><select v-model.number="sheetImageWidth"><option v-for="size in sheetSizes" :key="size.value" :value="size.value">{{ size.label }}</option></select></label>
        </div>
        <footer><button class="cancel" @click="showSheetDialog = false">{{ t('sheet.cancel') }}</button><button class="confirm" @click="runContactSheet"><Images :size="17" />{{ t('sheet.confirm') }}</button></footer>
      </section>
    </div>

    <div v-if="showDeleteDialog" class="shortcut-modal sheet-modal delete-modal" role="dialog" aria-modal="true" :aria-label="t('delete.title')" @click.self="showDeleteDialog = false">
      <section>
        <header><div><span>{{ t('delete.label') }}</span><strong>{{ t('delete.title') }}</strong></div><button class="icon-btn" :aria-label="t('common.close')" @click="showDeleteDialog = false"><X :size="20" /></button></header>
        <p>{{ t('delete.confirm', { name: item?.name }) }}</p>
        <footer><button class="cancel" @click="showDeleteDialog = false">{{ t('delete.cancel') }}</button><button class="confirm danger" autofocus @click="deleteCurrent"><Trash2 :size="17" />{{ t('delete.action') }}</button></footer>
      </section>
    </div>

    <div v-if="showShortcuts" class="shortcut-modal" role="dialog" aria-modal="true" :aria-label="t('shortcuts.open')" @click.self="showShortcuts = false">
      <section>
        <header><div><span>{{ t('shortcuts.keyboard') }}</span><strong>{{ t('shortcuts.title') }}</strong></div><button class="icon-btn" :aria-label="t('common.close')" @click="showShortcuts = false"><X :size="20" /></button></header>
        <dl>
          <div><dt>{{ t('shortcuts.playPause') }}</dt><dd><kbd>Space</kbd></dd></div>
          <div><dt>{{ t('shortcuts.seek', { seconds: seekStep }) }}</dt><dd><kbd>←</kbd><kbd>→</kbd></dd></div>
          <div><dt>{{ t('shortcuts.splitMarker') }}</dt><dd><kbd>B</kbd></dd></div>
          <div><dt>{{ t('shortcuts.previousNext') }}</dt><dd><kbd>⌘←</kbd><kbd>⌘→</kbd></dd></div>
          <div><dt>{{ t('shortcuts.delete') }}</dt><dd><kbd>Del</kbd><kbd>⌘⌫</kbd></dd></div>
          <div><dt>{{ t('shortcuts.fullscreen') }}</dt><dd><kbd>F</kbd></dd></div>
          <div><dt>{{ t('shortcuts.mute') }}</dt><dd><kbd>M</kbd></dd></div>
        </dl>
      </section>
    </div>
  </main>
</template>
