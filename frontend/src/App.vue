<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { AlertCircle, ArrowUpDown, Captions, CheckCircle2, ChevronDown, ChevronRight, Copy, Film, Flag, FolderOpen, Gauge, Images, Keyboard, Languages, ListVideo, LoaderCircle, Maximize, Minimize, Pause, Play, RefreshCw, RotateCcw, RotateCw, Scissors, SkipBack, SkipForward, Speaker, Trash2, Volume2, VolumeX, X } from '@lucide/vue'
import { Clipboard, Events, Window } from '@wailsio/runtime'
import { CancelTranscode, ClearThumbnailCache, DeleteVideo, DirectoryVideos, ExtractContactSheet, GenerateThumbnails, MarkPlayed, MediaDurations, OpenDroppedFiles, OpenSubtitle, OpenSubtitlePath, OpenVideos, PauseThumbnailGeneration, PendingSystemFiles, PlayerEngine, PreparePlayback, PreparePlaybackForce, ProbeMedia, RecentVideos, ResumeThumbnailGeneration, RevealInFolder, SplitVideo, SplitVideoAtMarkers, StopThumbnailGeneration, ThumbnailGenerationStatus, TranscodeOptions, TranscodeStatus, TranscodeVideo } from '../bindings/viplay/app'
import { loadLocale, locale, locales, t } from './i18n'
import { createNativePlayer } from './nativePlayer'

const video = ref(null)
const stage = ref(null)
const queuePanel = ref(null)
const nativeEngine = ref(false)
let stageObserver = null
const queue = ref([])
const index = ref(0)
const playing = ref(false)
const playbackURL = ref('')
const playbackPreparing = ref(false)
const current = ref(0)
const duration = ref(0)
const volume = ref(.75)
const muted = ref(false)
const speed = ref(1)
const seekStep = ref(10)
const isSeeking = ref(false)
let seekResumeTimer = null
let wasPlayingBeforeSeek = false
let scrubRaf = null
const fullscreenActive = ref(false)
const showQueue = ref(true)
const cursorIdle = ref(false)
let cursorTimer
const subtitle = ref(null)
const showShortcuts = ref(false)
const showSheetDialog = ref(false)
const showDeleteDialog = ref(false)
const showTranscodeDialog = ref(false)
const transcodeOptions = ref([])
const selectedTranscode = ref('')
const transcodeTrashOriginal = ref(localStorage.getItem('viplay.transcode.trashOriginal') === 'true')
watch(transcodeTrashOriginal, val => {
  localStorage.setItem('viplay.transcode.trashOriginal', String(val))
})
const transcodeProgress = ref({ active: false, percent: 0, speed: '', fps: 0, currentTime: 0, totalTime: 0 })
let transcodePollTimer = null
const sheetFrameCount = ref(12)
const sheetImageWidth = ref(320)
const mediaInfo = ref(null)
const libraryView = ref('folder')
const processing = ref('')
const notice = ref(null)
const splitMarkers = ref([])
const thumbnailProgress = ref({ active: false, paused: false, completed: 0, total: 0, current: '', completedPaths: [] })
const thumbnailReady = ref({})
const thumbnailRevision = ref(0)
const durations = ref({})
const sortModes = ['name', 'modified', 'duration']
const sortMode = ref(sortModes.includes(localStorage.getItem('viplay.playlist.sort')) ? localStorage.getItem('viplay.playlist.sort') : 'name')
const queueList = ref(null)
// Paths a finished run already processed; failures (non-H.264) are not retried on every scroll.
const thumbnailAttempted = new Set()
let thumbnailRunPaths = []
let queueObserver = null
let visibleTimer = null
const visiblePaths = new Set()
const fileDragOver = ref(false)
let proxyFallbackAttempted = false
let noticeTimer
let thumbnailPollTimer
let thumbnailRun = 0
let playbackRun = 0
let autoplayOnReady = false
let pausedForPlayback = false
const item = computed(() => queue.value[index.value])
const hasVideos = computed(() => queue.value.some(entry => entry.kind === 'video'))
const progress = computed(() => `${duration.value ? current.value / duration.value * 100 : 0}%`)
const markerPositions = computed(() => splitMarkers.value.map(seconds => ({ seconds, left: `${seconds / duration.value * 100}%` })))
const volumeProgress = computed(() => `${volume.value * 100}%`)
const thumbnailPercent = computed(() => thumbnailProgress.value.total ? `${thumbnailProgress.value.completed / thumbnailProgress.value.total * 100}%` : '0%')
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
  const directory = await folderItems(items[0].path)
  queue.value = directory
  const selectedIndex = directory.findIndex(entry => entry.path === items[0].path)
  index.value = Math.max(0, selectedIndex)
  autoplayOnReady = true
  libraryView.value = 'folder'
}

async function handleDroppedFiles(filePaths) {
  if (!filePaths || !filePaths.length) return
  const vtt = filePaths.find(p => p.toLowerCase().endsWith('.vtt'))
  const mediaPaths = filePaths.filter(p => !p.toLowerCase().endsWith('.vtt'))
  if (mediaPaths.length) {
    try {
      const items = await OpenDroppedFiles(mediaPaths)
      if (items?.length) {
        let directory = []
        try { directory = await folderItems(items[0].path) } catch {}
        queue.value = directory.length ? directory : items
        const selectedIndex = queue.value.findIndex(entry => entry.path === items[0].path)
        index.value = Math.max(0, selectedIndex)
        autoplayOnReady = true
        libraryView.value = 'folder'
      }
    } catch (err) {
      notify('error', t('playback.errorTitle'), String(err))
    }
  }
  if (vtt) {
    try {
      const sub = await OpenSubtitlePath(vtt)
      if (sub?.url) subtitle.value = sub
    } catch {}
  }
}

async function onVideoError() {
  if (!item.value) return
  if (!proxyFallbackAttempted) {
    proxyFallbackAttempted = true
    playbackPreparing.value = true
    notify('progress', t('playback.preparingTitle'), t('playback.preparing', { name: item.value.name }))
    try {
      const url = await PreparePlaybackForce(item.value.path)
      playbackURL.value = url
      playbackPreparing.value = false
      if (notice.value?.type === 'progress') notice.value = null
      await nextTick()
      try { await video.value?.play() } catch {}
    } catch (err) {
      playbackPreparing.value = false
      notify('error', t('playback.errorTitle'), String(err))
    }
  } else {
    playbackPreparing.value = false
    notify('error', t('playback.errorTitle'), t('error.playbackConversionFailed'))
  }
}

async function loadFolder() {
  if (!item.value || libraryView.value === 'folder') return
  const activePath = item.value.path
  queue.value = await folderItems(activePath)
  index.value = Math.max(0, queue.value.findIndex(entry => entry.path === activePath))
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
      queue.value = await folderItems(activePath)
      index.value = queue.value.findIndex(entry => entry.path === activePath)
    }
  } catch (error) { notify('error', t('split.errorTitle'), String(error)) }
  finally { processing.value = '' }
}

function toggleSplitMarker(seconds = video.value?.currentTime ?? current.value) {
  if (!item.value || !duration.value) { notify('error', t('error.noVideoTitle'), t('error.noVideoSplit')); return }
  if (processing.value) return
  if (seconds <= .05 || seconds >= duration.value - .05) { notify('error', t('error.invalidSplitTitle'), t('error.invalidSplit')); return }
  const existing = splitMarkers.value.findIndex(marker => Math.abs(marker - seconds) < .25)
  if (existing >= 0) {
    splitMarkers.value.splice(existing, 1)
    return
  }
  if (splitMarkers.value.length >= 100) { notify('error', t('markers.limitTitle'), t('markers.limit')); return }
  splitMarkers.value = [...splitMarkers.value, seconds].sort((a, b) => a - b)
}

// Right-clicking the timeline places a marker under the cursor without seeking there.
function toggleMarkerAtPointer(event) {
  const rect = event.currentTarget.getBoundingClientRect()
  toggleSplitMarker(Math.min(1, Math.max(0, (event.clientX - rect.left) / rect.width)) * duration.value)
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
      queue.value = await folderItems(activePath)
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

async function requestTranscode() {
  if (!item.value || item.value.kind !== 'video') { notify('error', t('error.noVideoTitle'), t('transcode.noVideo')); return }
  if (processing.value) { notify('error', t('delete.busyTitle'), t('delete.busy')); return }
  processing.value = 'options'
  try {
    transcodeOptions.value = await TranscodeOptions(item.value.path)
    if (!transcodeOptions.value.length) { notify('error', t('transcode.noOptionsTitle'), t('transcode.noOptions')); return }
    selectedTranscode.value = transcodeOptions.value[0].id
    showTranscodeDialog.value = true
  } catch (error) { notify('error', t('transcode.errorTitle'), String(error)) }
  finally { processing.value = '' }
}

async function runTranscode() {
  if (!item.value || !selectedTranscode.value || processing.value) return
  const source = item.value
  const option = transcodeOptions.value.find(candidate => candidate.id === selectedTranscode.value)
  if (!option) return
  const shouldTrash = transcodeTrashOriginal.value
  showTranscodeDialog.value = false
  if (video.value && !video.value.paused) video.value.pause()
  processing.value = 'transcode'
  transcodeProgress.value = { active: true, percent: 0, speed: '', fps: 0, currentTime: 0, totalTime: 0 }
  notify('progress', t('transcode.progressTitle'), t('transcode.progress', { codec: option.codec }))
  await nextTick()
  if (transcodePollTimer) window.clearInterval(transcodePollTimer)
  transcodePollTimer = window.setInterval(async () => {
    try {
      const status = await TranscodeStatus()
      if (status && status.active) {
        transcodeProgress.value = status
      }
    } catch {}
  }, 350)
  try {
    const result = await TranscodeVideo(source.path, option.id, shouldTrash)
    if (result.deleted) {
      queue.value.splice(index.value, 1, result.item)
      current.value = 0
      playing.value = false
      subtitle.value = null
      notify('success', t('transcode.successTitle'), t('transcode.success', { name: result.item.name, saved: fileSize(result.originalSize - result.outputSize) }))
    } else {
      queue.value.splice(index.value + 1, 0, result.item)
      const savedDiff = result.originalSize - result.outputSize
      if (savedDiff > 0) {
        notify('success', t('transcode.successTitle'), t('transcode.successKept', { name: result.item.name, saved: fileSize(savedDiff) }))
      } else {
        notify('success', t('transcode.successTitle'), t('transcode.successKeptNoSaving', { name: result.item.name }))
      }
    }
  } catch (error) {
    const errStr = String(error)
    if (errStr.includes(t('error.transcodeCancelled')) || errStr.toLowerCase().includes('cancel') || errStr.toLowerCase().includes('iptal')) {
      notify('info', t('transcode.cancelledTitle'), t('transcode.cancelled'))
    } else {
      notify('error', t('transcode.errorTitle'), errStr)
    }
  } finally {
    if (transcodePollTimer) {
      window.clearInterval(transcodePollTimer)
      transcodePollTimer = null
    }
    transcodeProgress.value = { active: false, percent: 0, speed: '', fps: 0, currentTime: 0, totalTime: 0 }
    processing.value = ''
  }
}

async function cancelTranscode() {
  try {
    await CancelTranscode()
  } catch {}
}

async function openSubtitle() {
  const selected = await OpenSubtitle()
  if (selected?.url) subtitle.value = selected
}

function beginSeeking() {
  if (!isSeeking.value) {
    isSeeking.value = true
    wasPlayingBeforeSeek = !!(video.value && !video.value.paused)
    if (video.value && !video.value.paused) {
      video.value.pause()
    }
  }
  if (seekResumeTimer) {
    window.clearTimeout(seekResumeTimer)
    seekResumeTimer = null
  }
}

function finishSeeking(delay = 200) {
  if (seekResumeTimer) window.clearTimeout(seekResumeTimer)
  seekResumeTimer = window.setTimeout(() => {
    seekResumeTimer = null
    const shouldResume = wasPlayingBeforeSeek
    isSeeking.value = false
    wasPlayingBeforeSeek = false
    if (video.value) current.value = video.value.currentTime
    if (shouldResume && video.value && video.value.paused) {
      video.value.play().catch(() => {})
    } else if (pausedForPlayback && thumbnailProgress.value.paused && (!video.value || video.value.paused)) {
      pausedForPlayback = false
      ResumeThumbnailGeneration().then(refreshThumbnailProgress).catch(() => {})
    }
  }, delay)
}

function toggle() {
  if (!video.value || !item.value) return openVideos()
  const hadActiveSeek = isSeeking.value
  const previouslyPlaying = wasPlayingBeforeSeek
  if (seekResumeTimer) {
    window.clearTimeout(seekResumeTimer)
    seekResumeTimer = null
  }
  isSeeking.value = false
  wasPlayingBeforeSeek = false
  if (hadActiveSeek && previouslyPlaying) {
    video.value.pause()
    return
  }
  video.value.paused ? video.value.play() : video.value.pause()
}

function seekBy(delta) {
  if (!video.value) return
  beginSeeking()
  const baseTime = isSeeking.value && current.value !== undefined ? current.value : video.value.currentTime
  const nextTime = Math.max(0, Math.min(duration.value || 0, baseTime + delta))
  current.value = nextTime
  video.value.currentTime = nextTime
  finishSeeking(200)
}

function normaliseSeekStep() {
  seekStep.value = Math.min(300, Math.max(1, Math.round(Number(seekStep.value) || 10)))
}

function startScrub() {
  beginSeeking()
  window.addEventListener('pointerup', endScrub, { once: true })
  window.addEventListener('pointercancel', endScrub, { once: true })
}

function onScrub(event) {
  beginSeeking()
  const targetTime = Number(event.target.value)
  current.value = targetTime
  if (scrubRaf) cancelAnimationFrame(scrubRaf)
  scrubRaf = requestAnimationFrame(() => {
    scrubRaf = null
    if (video.value) video.value.currentTime = targetTime
  })
}

function endScrub(event) {
  window.removeEventListener('pointerup', endScrub)
  window.removeEventListener('pointercancel', endScrub)
  if (scrubRaf) {
    cancelAnimationFrame(scrubRaf)
    scrubRaf = null
  }
  if (!isSeeking.value) return
  if (event?.target && 'value' in event.target) {
    const targetTime = Number(event.target.value)
    current.value = targetTime
    if (video.value) video.value.currentTime = targetTime
  }
  finishSeeking(60)
}

function onTimeUpdate(event) {
  if (!isSeeking.value) current.value = event.currentTarget.currentTime
}

async function fullscreen() {
  await Window.ToggleFullscreen()
  await syncFullscreen()
}
async function syncFullscreen() {
  fullscreenActive.value = await Window.IsFullscreen()
}
function wakeCursor() {
  cursorIdle.value = false
  window.clearTimeout(cursorTimer)
  if (fullscreenActive.value) cursorTimer = window.setTimeout(() => { cursorIdle.value = true }, 2500)
}
watch(fullscreenActive, wakeCursor)
async function select(i, autoplay = false) {
  if (seekResumeTimer) {
    window.clearTimeout(seekResumeTimer)
    seekResumeTimer = null
  }
  isSeeking.value = false
  wasPlayingBeforeSeek = false
  index.value = i
  playing.value = false
  playbackURL.value = ''
  autoplayOnReady = autoplay
  current.value = 0
  subtitle.value = null
}
function next() { if (index.value < queue.value.length - 1) select(index.value + 1, true) }
function previous() { current.value > 3 ? seekBy(-current.value) : index.value > 0 && select(index.value - 1, true) }
function title(name) { return name.replace(/\.[^/.]+$/, '') }
function fileType(name) { return name.split('.').pop().toUpperCase() }
const mediaDetails = computed(() => {
  const info = mediaInfo.value
  if (!info) return []
  return [
    { label: t('mediaInfo.video'), value: info.videoCodec?.toUpperCase() },
    { label: t('mediaInfo.audio'), value: info.audioCodec?.toUpperCase() },
    { label: t('mediaInfo.dimensions'), value: info.width ? `${info.width}×${info.height}` : '' },
    { label: t('mediaInfo.format'), value: info.container?.toUpperCase() },
    { label: 'FPS', value: info.fps ? `${info.fps} FPS` : '' },
    { label: t('mediaInfo.file'), value: info.size ? fileSize(info.size) : '' },
  ].filter(detail => detail.value)
})
function fileSize(value) {
  if (!value) return '—'
  const megabytes = value / 1024 / 1024
  if (megabytes < 1) return `${Math.max(1, Math.round(value / 1024))} KB`
  return megabytes >= 1024 ? `${(megabytes / 1024).toFixed(2)} GB` : `${megabytes.toFixed(1)} MB`
}

function sortItems(items) {
  const byName = (a, b) => a.name.localeCompare(b.name, undefined, { numeric: true, sensitivity: 'base' })
  // Longest first; files whose duration could not be read go last.
  const byDuration = (a, b) => (durations.value[b.path] || 0) - (durations.value[a.path] || 0) || byName(a, b)
  const compare = { modified: (a, b) => b.modTime - a.modTime || byName(a, b), duration: byDuration }[sortMode.value] ?? byName
  return [...items].sort(compare)
}

// Duration sorting needs every row's duration, not only the rows the viewport has loaded.
async function loadAllDurations(items) {
  const missing = items.filter(entry => !durations.value[entry.path]).map(entry => entry.path)
  if (missing.length) {
    try { Object.assign(durations.value, await MediaDurations(missing)) } catch {}
  }
}

async function folderItems(path) {
  const items = await DirectoryVideos(path)
  if (sortMode.value === 'duration') await loadAllDurations(items)
  return sortItems(items)
}

watch(sortMode, async value => {
  try { localStorage.setItem('viplay.playlist.sort', value) } catch {}
  if (libraryView.value !== 'folder' || !queue.value.length) return
  if (value === 'duration') await loadAllDurations(queue.value)
  if (sortMode.value !== value) return
  const activePath = item.value?.path
  queue.value = sortItems(queue.value)
  index.value = Math.max(0, queue.value.findIndex(entry => entry.path === activePath))
})

// Rows entering the playlist viewport get their durations and thumbnails on demand.
function observeQueueRows() {
  queueObserver?.disconnect()
  visiblePaths.clear()
  if (!queueList.value) return
  // A new observer each time: the list element is recreated when the playlist is closed and reopened.
  queueObserver = new IntersectionObserver(entries => {
    for (const entry of entries) {
      const path = entry.target.dataset.path
      entry.isIntersecting ? visiblePaths.add(path) : visiblePaths.delete(path)
    }
    window.clearTimeout(visibleTimer)
    visibleTimer = window.setTimeout(loadVisibleRows, 150)
  }, { root: queueList.value, rootMargin: '200px 0px' })
  queueList.value.querySelectorAll('.queue-item').forEach(row => queueObserver.observe(row))
}

async function loadVisibleRows() {
  const visible = queue.value.filter(entry => visiblePaths.has(entry.path))
  const missingDurations = visible.filter(entry => durations.value[entry.path] === undefined).map(entry => entry.path)
  if (missingDurations.length) {
    for (const path of missingDurations) durations.value[path] = null
    try { Object.assign(durations.value, await MediaDurations(missingDurations)) } catch {}
  }
  const missingThumbnails = visible.filter(entry => entry.kind === 'video' && !thumbnailReady.value[entry.path] && !thumbnailAttempted.has(entry.path)).map(entry => entry.path)
  const coveredByActiveRun = thumbnailProgress.value.active && !thumbnailProgress.value.paused && missingThumbnails.every(path => thumbnailRunPaths.includes(path))
  if (missingThumbnails.length && !coveredByActiveRun) startThumbnailQueue(missingThumbnails)
}

function scrollToCurrent() {
  queueList.value?.querySelector('.queue-item.current')?.scrollIntoView({ block: 'nearest' })
}

function prioritizeVideoPaths(items, activeIndex, limit = 0) {
  const videoEntries = []
  items.forEach((entry, i) => {
    if (entry.kind === 'video') {
      videoEntries.push({ path: entry.path, dist: Math.abs(i - activeIndex), index: i })
    }
  })
  videoEntries.sort((a, b) => a.dist - b.dist || a.index - b.index)
  const selected = (limit > 0 && videoEntries.length > limit) ? videoEntries.slice(0, limit) : videoEntries
  return selected.map(e => e.path)
}

function applyThumbnailProgress(status) {
  thumbnailProgress.value = status
  if (!status.active && status.total && status.completed >= status.total) thumbnailRunPaths.forEach(path => thumbnailAttempted.add(path))
  if (status.completedPaths?.length) {
    let changed = false
    for (const path of status.completedPaths) {
      if (!thumbnailReady.value[path]) {
        thumbnailReady.value[path] = true
        changed = true
      }
    }
    if (changed) {
      thumbnailReady.value = { ...thumbnailReady.value }
    }
  }
}

async function refreshThumbnailProgress() {
  applyThumbnailProgress(await ThumbnailGenerationStatus())
  if (!thumbnailProgress.value.active) window.clearInterval(thumbnailPollTimer)
}

// Without explicit paths, only the 30 videos around the current one are generated (D19); scrolling covers the rest.
function startThumbnailQueue(visibleOnly = null) {
  const limit = queue.value.length > 30 ? 30 : 0
  const paths = visibleOnly ?? prioritizeVideoPaths(queue.value, index.value, limit)
  thumbnailRunPaths = paths
  // Rows the user scrolled to are a small batch they asked to see, so they are not held back during playback.
  const holdForPlayback = playing.value && !visibleOnly
  const run = ++thumbnailRun
  window.clearInterval(thumbnailPollTimer)
  pausedForPlayback = false
  if (!paths.length) {
    thumbnailProgress.value = { active: false, paused: false, completed: 0, total: 0, current: '', completedPaths: [] }
    return
  }
  thumbnailProgress.value = { active: true, paused: false, completed: 0, total: paths.length, current: '', completedPaths: [] }
  thumbnailPollTimer = window.setInterval(() => { if (run === thumbnailRun) refreshThumbnailProgress() }, 350)

  if (holdForPlayback) {
    pausedForPlayback = true
  }

  GenerateThumbnails(paths)
    .catch(error => notify('error', t('thumbnails.errorTitle'), String(error)))
    .finally(() => { if (run === thumbnailRun) refreshThumbnailProgress() })

  if (holdForPlayback) {
    PauseThumbnailGeneration().then(refreshThumbnailProgress).catch(() => {})
  }
}

async function toggleThumbnailPause() {
  pausedForPlayback = false
  if (thumbnailProgress.value.paused) await ResumeThumbnailGeneration()
  else await PauseThumbnailGeneration()
  await refreshThumbnailProgress()
}

async function stopThumbnailQueue() {
  pausedForPlayback = false
  ++thumbnailRun
  window.clearInterval(thumbnailPollTimer)
  await StopThumbnailGeneration()
  await refreshThumbnailProgress()
}

async function clearThumbnails() {
  pausedForPlayback = false
  ++thumbnailRun
  window.clearInterval(thumbnailPollTimer)
  try {
    await ClearThumbnailCache()
    thumbnailRevision.value++
    thumbnailReady.value = {}
    thumbnailAttempted.clear()
    loadVisibleRows()
    thumbnailProgress.value = { active: false, paused: false, completed: 0, total: 0, current: '', completedPaths: [] }
    notify('success', t('thumbnails.clearedTitle'), t('thumbnails.cleared'))
  } catch (error) { notify('error', t('thumbnails.clearErrorTitle'), String(error)) }
}

// Actions from the native playlist row menu (built in playlist_menu.go).
async function handlePlaylistAction(action, path) {
  const entryIndex = queue.value.findIndex(entry => entry.path === path)
  if (entryIndex < 0) return
  const entry = queue.value[entryIndex]
  if (action === 'play') select(entryIndex, true)
  else if (action === 'reveal') RevealInFolder(path).catch(error => notify('error', t('playlistMenu.revealErrorTitle'), String(error)))
  else if (action === 'copy') copyPath(path)
  else if (action === 'delete') requestDelete(entry)
}

async function copyPath(path) {
  try {
    await Clipboard.SetText(path)
    notify('success', t('pathCopy.successTitle'), path)
  } catch (error) { notify('error', t('pathCopy.errorTitle'), String(error)) }
}

function copyFullPath() {
  if (item.value?.path) copyPath(item.value.path)
}

async function navigateDirectory(direction) {
  if (!item.value) return
  const activePath = item.value.path
  const items = await folderItems(activePath)
  const activeIndex = items.findIndex(entry => entry.path === activePath)
  const target = activeIndex + direction
  if (target < 0 || target >= items.length) return
  queue.value = items
  select(target, true)
}

let wasPlayingBeforeDelete = false
const deleteTarget = ref(null)

// Deletes the playing video by default; the playlist context menu passes any other row.
function requestDelete(target = item.value) {
  if (!target) { notify('error', t('error.noVideoTitle'), t('delete.noVideo')); return }
  if (processing.value) { notify('error', t('delete.busyTitle'), t('delete.busy')); return }
  deleteTarget.value = target
  wasPlayingBeforeDelete = playing.value || !!(video.value && !video.value.paused)
  showDeleteDialog.value = true
}

function closeDeleteDialog() {
  showDeleteDialog.value = false
  wasPlayingBeforeDelete = false
  deleteTarget.value = null
}

function forgetDeletedThumbnail(path) {
  if (thumbnailReady.value[path]) {
    delete thumbnailReady.value[path]
    thumbnailReady.value = { ...thumbnailReady.value }
  }
  if (thumbnailProgress.value.completedPaths?.includes(path)) {
    thumbnailProgress.value.completedPaths = thumbnailProgress.value.completedPaths.filter(p => p !== path)
    thumbnailProgress.value.completed = Math.max(0, thumbnailProgress.value.completed - 1)
    thumbnailProgress.value.total = Math.max(0, thumbnailProgress.value.total - 1)
  }
}

async function confirmDelete() {
  const deleting = deleteTarget.value ?? item.value
  if (!deleting || processing.value) return
  if (deleting.path !== item.value?.path) return deleteOtherRow(deleting)
  const wasPlaying = wasPlayingBeforeDelete || playing.value || !!(video.value && !video.value.paused)
  wasPlayingBeforeDelete = false
  showDeleteDialog.value = false
  processing.value = 'delete'
  if (seekResumeTimer) {
    window.clearTimeout(seekResumeTimer)
    seekResumeTimer = null
  }
  isSeeking.value = false
  wasPlayingBeforeSeek = false
  if (video.value && !video.value.paused) {
    video.value.pause()
  }
  if (video.value) {
    video.value.currentTime = 0
  }
  current.value = 0
  duration.value = 0
  playing.value = false
  try {
    await DeleteVideo(deleting.path)
    forgetDeletedThumbnail(deleting.path)
    const currentIndex = index.value
    const hasNext = queue.value.length > 1
    const nextIndex = currentIndex >= queue.value.length - 1 ? Math.max(0, queue.value.length - 2) : currentIndex
    autoplayOnReady = wasPlaying && hasNext
    current.value = 0
    duration.value = 0
    playing.value = false
    playbackURL.value = ''
    subtitle.value = null
    mediaInfo.value = null
    queue.value.splice(currentIndex, 1)
    index.value = nextIndex
    notify('success', t('delete.successTitle'), deleting.name)
  } catch (error) { notify('error', t('delete.errorTitle'), String(error)) }
  finally { processing.value = ''; deleteTarget.value = null }
}

// A row other than the playing one is removed without interrupting playback.
async function deleteOtherRow(deleting) {
  showDeleteDialog.value = false
  wasPlayingBeforeDelete = false
  processing.value = 'delete'
  try {
    await DeleteVideo(deleting.path)
    forgetDeletedThumbnail(deleting.path)
    const playingPath = item.value?.path
    queue.value = queue.value.filter(entry => entry.path !== deleting.path)
    index.value = Math.max(0, queue.value.findIndex(entry => entry.path === playingPath))
    notify('success', t('delete.successTitle'), deleting.name)
  } catch (error) { notify('error', t('delete.errorTitle'), String(error)) }
  finally { processing.value = ''; deleteTarget.value = null }
}

function loaded(event) {
  duration.value = event.currentTarget.duration
  if (item.value && Number.isFinite(duration.value) && duration.value > 0) durations.value[item.value.path] = duration.value
  event.currentTarget.volume = volume.value
  event.currentTarget.playbackRate = speed.value
  mediaInfo.value = { ...mediaInfo.value, width: mediaInfo.value?.width || event.currentTarget.videoWidth, height: mediaInfo.value?.height || event.currentTarget.videoHeight, duration: mediaInfo.value?.duration || event.currentTarget.duration }
}

function onKey(event) {
  if (event.key === 'Escape' && showTranscodeDialog.value) { showTranscodeDialog.value = false; return }
  if (event.key === 'Escape' && showDeleteDialog.value) { closeDeleteDialog(); return }
  if (!event.repeat && event.key === 'Enter' && showDeleteDialog.value) { event.preventDefault(); confirmDelete(); return }
  if (event.key === 'Escape' && showSheetDialog.value) { showSheetDialog.value = false; return }
  if (event.key === 'Escape' && showShortcuts.value) { showShortcuts.value = false; return }
  if (showDeleteDialog.value || showTranscodeDialog.value || showShortcuts.value) return
  if (event.key === 'Escape' && fullscreenActive.value) { fullscreen(); return }
  if (['INPUT', 'BUTTON'].includes(document.activeElement?.tagName) && event.code === 'Space') return
  if (document.activeElement?.tagName === 'INPUT' && document.activeElement.type !== 'range' && ['ArrowRight', 'ArrowLeft'].includes(event.code)) return
  const player = video.value
  if (event.metaKey && event.code === 'ArrowRight') { event.preventDefault(); navigateDirectory(1); return }
  if (event.metaKey && event.code === 'ArrowLeft') { event.preventDefault(); navigateDirectory(-1); return }
  if (!event.repeat && (event.code === 'Delete' || (event.metaKey && event.code === 'Backspace'))) { event.preventDefault(); requestDelete(); return }
  if (!event.repeat && event.code === 'KeyB' && player) { event.preventDefault(); toggleSplitMarker(); return }
  if (event.code === 'Space' && player) { event.preventDefault(); toggle(); return }
  if (event.code === 'ArrowRight' && player) { event.preventDefault(); seekBy(seekStep.value); return }
  if (event.code === 'ArrowLeft' && player) { event.preventDefault(); seekBy(-seekStep.value); return }
  if (event.key.toLowerCase() === 'f') fullscreen()
  if (event.key.toLowerCase() === 'm') muted.value = !muted.value
}

function onKeyUp(event) {
  if (['ArrowRight', 'ArrowLeft'].includes(event.code) && isSeeking.value) {
    finishSeeking(50)
  }
}

watch(speed, value => { if (video.value) video.value.playbackRate = value })
watch(volume, value => { if (video.value) video.value.volume = value })
watch(playing, isPlaying => {
  if (thumbnailProgress.value.active) {
    if (isPlaying && !thumbnailProgress.value.paused) {
      pausedForPlayback = true
      PauseThumbnailGeneration().then(refreshThumbnailProgress).catch(() => {})
    } else if (!isPlaying && pausedForPlayback && thumbnailProgress.value.paused) {
      if (isSeeking.value) return
      pausedForPlayback = false
      ResumeThumbnailGeneration().then(refreshThumbnailProgress).catch(() => {})
    }
  }
})
watch([() => queue.value.map(entry => entry.path).join('\n'), showQueue], () => nextTick(observeQueueRows))
watch(() => item.value?.path, () => nextTick(scrollToCurrent))
watch(() => queue.value.map(entry => entry.path).join('\n'), (newPathsStr, oldPathsStr) => {
  if (!newPathsStr) return
  if (oldPathsStr) {
    const oldSet = new Set(oldPathsStr.split('\n'))
    const hasNewPaths = newPathsStr.split('\n').some(path => !oldSet.has(path))
    if (!hasNewPaths) return
  }
  startThumbnailQueue()
}, { immediate: true })
// Keyed by path so reloading the folder or playlist does not restart the video that is already playing.
watch(() => item.value?.path, async () => {
  const value = item.value
  splitMarkers.value = []
  proxyFallbackAttempted = false
  const run = ++playbackRun
  const shouldAutoplay = autoplayOnReady
  autoplayOnReady = false
  playbackURL.value = ''
  playbackPreparing.value = !!value
  current.value = 0
  duration.value = 0
  if (!value) { mediaInfo.value = null; playbackPreparing.value = false; if (nativeEngine.value) video.value.stop(); return }
  if (nativeEngine.value) {
    // libmpv opens every supported container directly, so no playback proxy is prepared.
    playbackPreparing.value = false
    video.value.load(value.path, shouldAutoplay).catch(error => { if (run === playbackRun) notify('error', t('playback.errorTitle'), String(error)) })
    try {
      const info = await ProbeMedia(value.path)
      if (run === playbackRun) mediaInfo.value = info
    } catch { if (run === playbackRun) mediaInfo.value = null }
    return
  }
  const legacyFormat = /\.(mpg|mpeg|flv|avi|wmv|asf|mkv)$/i.test(value.path)
  if (legacyFormat) notify('progress', t('playback.preparingTitle'), t('playback.preparing', { name: value.name }))
  // Probing and proxy preparation are independent, so neither waits for the other.
  const playbackRequest = PreparePlayback(value.path)
  playbackRequest.catch(() => {})
  try {
    const info = await ProbeMedia(value.path)
    if (run === playbackRun) mediaInfo.value = info
  } catch { if (run === playbackRun) mediaInfo.value = null }
  if (run !== playbackRun) return
  try {
    const url = await playbackRequest
    if (run !== playbackRun) return
    playbackURL.value = url
    playbackPreparing.value = false
    if (legacyFormat && notice.value?.type === 'progress') notice.value = null
    if (shouldAutoplay) {
      await nextTick()
      try { await video.value?.play() } catch { /* Playback remains available through the user controls. */ }
    }
  } catch (error) {
    if (run !== playbackRun) return
    playbackPreparing.value = false
    notify('error', t('playback.errorTitle'), String(error))
  }
}, { immediate: true })
async function changeLanguage(event) {
  await loadLocale(event.target.value)
}

function onWindowDragOver(e) {
  e.preventDefault()
  if (e.dataTransfer?.types?.includes('Files')) {
    fileDragOver.value = true
  }
}
function onWindowDragLeave(e) {
  e.preventDefault()
  if (e.clientX <= 0 || e.clientY <= 0 || e.clientX >= window.innerWidth || e.clientY >= window.innerHeight) {
    fileDragOver.value = false
  }
}
function onWindowDrop(e) {
  e.preventDefault()
  fileDragOver.value = false
  if (e.dataTransfer?.files?.length) {
    const paths = Array.from(e.dataTransfer.files).map(f => f.path || f.name).filter(Boolean)
    if (paths.length) handleDroppedFiles(paths)
  }
}
function onWailsFilesDropped(event) {
  fileDragOver.value = false
  const files = event.data?.files || []
  if (files.length) handleDroppedFiles(files)
}

function updateVideoArea() {
  const rect = stage.value?.getBoundingClientRect()
  if (!rect || !nativeEngine.value) return
  const width = window.innerWidth
  const height = window.innerHeight
  // The playlist overlays the stage, so the picture is kept to the uncovered part.
  const queueLeft = queuePanel.value?.getBoundingClientRect().left ?? width
  const right = Math.min(rect.right, queueLeft)
  video.value.setVideoArea(rect.left / width, rect.top / height, (width - right) / width, (height - rect.bottom) / height)
}
watch([showQueue, fullscreenActive], () => nextTick(updateVideoArea))

async function initNativeEngine() {
  try {
    if (await PlayerEngine() !== 'mpv') return
  } catch { return }
  video.value = createNativePlayer({
    play: markPlayed,
    pause: () => { playing.value = false },
    timeupdate: onTimeUpdate,
    loadedmetadata: loaded,
    ended: next,
    error: message => notify('error', t('playback.errorTitle'), message),
  })
  video.value.volume = volume.value
  video.value.playbackRate = speed.value
  video.value.setMuted(muted.value)
  nativeEngine.value = true
  document.documentElement.classList.add('native-engine')
  stageObserver = new ResizeObserver(updateVideoArea)
  stageObserver.observe(stage.value)
  window.addEventListener('resize', updateVideoArea)
  updateVideoArea()
  if (item.value) video.value.load(item.value.path, autoplayOnReady).catch(() => {})
}

watch(muted, value => { if (nativeEngine.value) video.value.setMuted(value) })
watch(subtitle, value => { if (nativeEngine.value) video.value.setSubtitle(value?.path) })

onMounted(() => {
  // Files opened from Finder before the UI loaded are queued in the backend until the engine is chosen.
  initNativeEngine().then(PendingSystemFiles).then(files => handleDroppedFiles(files)).catch(() => {})
  window.addEventListener('keydown', onKey)
  window.addEventListener('keyup', onKeyUp)
  window.addEventListener('dragenter', onWindowDragOver)
  window.addEventListener('dragover', onWindowDragOver)
  // The OS can leave fullscreen without going through fullscreen(), e.g. the macOS window button.
  window.addEventListener('resize', syncFullscreen)
  window.addEventListener('mousemove', wakeCursor)
  window.addEventListener('dragleave', onWindowDragLeave)
  window.addEventListener('drop', onWindowDrop)
  Events.On('files-dropped', onWailsFilesDropped)
  Events.On('playlist:action', ({ data }) => handlePlaylistAction(data.action, decodeURIComponent(String(data.data).trim())))
})
onBeforeUnmount(() => {
  stageObserver?.disconnect()
  queueObserver?.disconnect()
  window.clearTimeout(visibleTimer)
  window.removeEventListener('resize', updateVideoArea)
  window.removeEventListener('resize', syncFullscreen)
  window.removeEventListener('mousemove', wakeCursor)
  window.clearTimeout(cursorTimer)
  window.removeEventListener('keydown', onKey)
  window.removeEventListener('keyup', onKeyUp)
  window.removeEventListener('dragenter', onWindowDragOver)
  window.removeEventListener('dragover', onWindowDragOver)
  window.removeEventListener('dragleave', onWindowDragLeave)
  window.removeEventListener('drop', onWindowDrop)
  window.removeEventListener('pointerup', endScrub)
  window.removeEventListener('pointercancel', endScrub)
  if (scrubRaf) cancelAnimationFrame(scrubRaf)
  if (seekResumeTimer) window.clearTimeout(seekResumeTimer)
  window.clearTimeout(noticeTimer)
  window.clearInterval(thumbnailPollTimer)
  if (transcodePollTimer) window.clearInterval(transcodePollTimer)
  CancelTranscode().catch(() => {})
  StopThumbnailGeneration().catch(() => {})
})
</script>

<template>
  <main class="app" data-file-drop-target :class="{ 'queue-closed': !showQueue || fullscreenActive, fullscreen: fullscreenActive, 'cursor-idle': fullscreenActive && cursorIdle }">
    <div v-if="fileDragOver" class="drop-overlay">
      <div class="drop-overlay-icon"><Film :size="36" /></div>
      <h2>{{ t('drop.title') }}</h2>
      <p>{{ t('drop.description') }}</p>
    </div>
    <section class="player-shell">
      <header class="topbar">
        <div class="topbar-left">
          <div v-if="item" class="topbar-meta">
            <span class="topbar-file"><strong :title="item.path">{{ item.name }}</strong><button class="copy-path" :aria-label="t('pathCopy.action')" :title="t('pathCopy.action')" @click="copyFullPath"><Copy :size="14" /></button></span>
            <small v-if="mediaDetails.length" class="media-details" :aria-label="t('mediaInfo.title')"><span v-for="detail in mediaDetails" :key="detail.label" :title="detail.label">{{ detail.value }}</span></small>
          </div>
        </div>
        <div class="topbar-actions">
          <button class="icon-btn" :aria-label="t('library.openVideo')" :title="t('library.openVideo')" @click="openVideos"><FolderOpen :size="19" /></button>
          <label class="language-picker" :title="t('language.label')"><Languages :size="16" /><select :value="locale" :aria-label="t('language.label')" @change="changeLanguage"><option v-for="language in locales" :key="language.code" :value="language.code">{{ language.label }}</option></select><ChevronDown :size="12" /></label>
          <button class="icon-btn operation-btn" :class="{ processing: processing === 'split', marked: splitMarkers.length }" :aria-label="splitMarkers.length ? t('actions.multiSplit', { count: splitMarkers.length }) : t('actions.split')" :title="splitMarkers.length ? t('actions.multiSplitTitle', { count: splitMarkers.length }) : t('actions.splitTitle')" @click="splitAtMarkers"><LoaderCircle v-if="processing === 'split'" class="spin" :size="20" /><Scissors v-else :size="19" /><span v-if="splitMarkers.length" class="marker-count">{{ splitMarkers.length }}</span></button>
          <button class="icon-btn operation-btn" :class="{ processing: processing === 'sheet' }" :aria-label="t('actions.contactSheet')" :title="t('actions.contactSheetTitle')" @click="createContactSheet"><LoaderCircle v-if="processing === 'sheet'" class="spin" :size="20" /><Images v-else :size="19" /></button>
          <button class="icon-btn operation-btn" :class="{ processing: processing === 'transcode' || processing === 'options', 'can-cancel': processing === 'transcode' }" :disabled="!item || item.kind !== 'video' || (!!processing && processing !== 'transcode')" :aria-label="processing === 'transcode' ? t('transcode.cancelAction') : t('actions.transcode')" :title="processing === 'transcode' ? t('transcode.cancelAction') : t('actions.transcodeTitle')" @click="processing === 'transcode' ? cancelTranscode() : requestTranscode()"><X v-if="processing === 'transcode'" :size="19" /><LoaderCircle v-else-if="processing === 'options'" class="spin" :size="20" /><RefreshCw v-else :size="19" /></button>
          <button class="icon-btn danger" :disabled="processing === 'delete'" :aria-label="t('actions.delete')" :title="t('actions.deleteTitle')" @click="requestDelete()"><Trash2 :size="19" /></button>
          <button class="icon-btn" :aria-label="t('shortcuts.open')" :title="t('shortcuts.open')" @click="showShortcuts = true"><Keyboard :size="19" /></button>
          <button class="icon-btn" :class="{ active: showQueue }" :aria-label="t('actions.showQueue')" :title="t('actions.showQueue')" @click="showQueue = !showQueue"><ListVideo :size="20" /></button>
        </div>
      </header>

      <div ref="stage" class="stage" :class="{ 'has-media': !!item }" @dblclick="fullscreen">
        <div v-if="item && nativeEngine" class="native-surface" @click="toggle" />
        <video v-else-if="item" :key="playbackURL" ref="video" :src="playbackURL || undefined" :muted="muted" @click="toggle" @play="markPlayed" @pause="playing = false" @timeupdate="onTimeUpdate" @loadedmetadata="loaded" @ended="next" @error="onVideoError">
          <track v-if="subtitle" default kind="subtitles" :src="subtitle.url" srcLang="tr" :label="subtitle.name">
        </video>
        <div v-if="item && playbackPreparing" class="playback-preparing"><LoaderCircle class="spin" :size="30" /><span>{{ t('playback.preparingTitle') }}</span></div>
        <div v-if="!item" class="empty-state">
          <div class="empty-icon"><Film :size="42" /></div>
          <h1>{{ t('empty.title') }}</h1><p>{{ t('empty.description') }}</p>
          <button @click="openVideos"><FolderOpen :size="18" />{{ t('library.openVideo') }}</button>
        </div>
        <div class="bottom-fade" />
        <div v-if="item && splitMarkers.length" class="marker-bar">
          <Flag :size="15" /><span>{{ t('markers.count', { count: splitMarkers.length }) }}</span>
          <button class="marker-bar-primary" :disabled="!!processing" @click="splitAtMarkers"><Scissors :size="15" />{{ t('markers.splitInto', { parts: splitMarkers.length + 1 }) }}</button>
          <button :disabled="!!processing" @click="splitMarkers = []">{{ t('markers.clear') }}</button>
        </div>
      </div>

      <div class="controls" :class="{ idle: !item }">
        <div class="timeline-row"><span>{{ fmt(current) }}</span><div class="timeline-track" :title="item ? t('markers.timelineHint') : ''" @contextmenu.prevent="toggleMarkerAtPointer"><input :aria-label="t('player.progress')" type="range" min="0" :max="duration || 0" step=".1" :value="current" :style="{ '--progress': progress }" @pointerdown="event => event.button === 0 && startScrub()" @input="onScrub" @change="endScrub" @pointerup="endScrub" @pointercancel="endScrub"><button v-for="marker in markerPositions" :key="marker.seconds" class="split-marker" :style="{ left: marker.left }" :aria-label="t('markers.removeAt', { time: fmt(marker.seconds) })" :title="t('markers.removeAt', { time: fmt(marker.seconds) })" @click="removeSplitMarker(marker.seconds)" @contextmenu.prevent.stop="removeSplitMarker(marker.seconds)"><span>{{ fmt(marker.seconds) }}</span></button></div><span>{{ fmt(duration) }}</span></div>
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
            <button class="tool marker-tool" :class="{ active: splitMarkers.length }" :disabled="!item || processing" :title="t('markers.addTitle')" @click="toggleSplitMarker()"><Flag :size="18" /><span>{{ t('markers.add') }}</span><kbd>B</kbd></button>
            <label class="seek-step" :title="t('player.seekDuration')"><input v-model.number="seekStep" :aria-label="t('player.seekSeconds')" type="number" min="1" max="300" step="1" @change="normaliseSeekStep"><span>{{ t('player.secondsShort') }}</span></label>
            <button class="tool" :class="{ active: subtitle }" @click="openSubtitle"><Captions :size="20" /><span>{{ t('player.subtitles') }}</span></button>
            <label class="speed"><Gauge :size="19" /><select v-model.number="speed" :aria-label="t('player.speed')"><option v-for="value in speeds" :key="value" :value="value">{{ value }}×</option></select><ChevronDown :size="14" /></label>
            <button class="icon-btn" :class="{ active: fullscreenActive }" :aria-label="fullscreenActive ? t('player.exitFullscreen') : t('player.fullscreen')" @click="fullscreen"><Minimize v-if="fullscreenActive" :size="20" /><Maximize v-else :size="20" /></button>
          </div>
        </div>
      </div>
    </section>

    <aside v-if="showQueue && !fullscreenActive" ref="queuePanel" class="queue">
      <div class="queue-head">
        <strong>{{ t('queue.playlist') }}<span v-if="queue.length">{{ queue.length }}</span></strong>
        <div>
          <label v-if="libraryView === 'folder' && queue.length > 1" class="queue-sort" :title="t('queue.sortBy')"><ArrowUpDown :size="14" /><select v-model="sortMode" :aria-label="t('queue.sortBy')"><option value="name">{{ t('queue.sortName') }}</option><option value="modified">{{ t('queue.sortModified') }}</option><option value="duration">{{ t('queue.sortDuration') }}</option></select></label>
          <button v-if="hasVideos" class="icon-btn small" :aria-label="t('thumbnails.clear')" :title="t('thumbnails.clear')" @click="clearThumbnails"><Trash2 :size="15" /></button>
          <button class="icon-btn" :aria-label="t('queue.close')" :title="t('queue.close')" @click="showQueue = false"><X :size="20" /></button>
        </div>
      </div>
      <div class="queue-tabs">
        <button :class="{ selected: item && libraryView === 'folder' }" :disabled="!item" @click="loadFolder"><Film :size="15" />{{ t('library.folder') }}</button>
        <button :class="{ selected: libraryView === 'recent' }" @click="loadRecent"><RotateCcw :size="15" />{{ t('library.recent') }}</button>
      </div>
      <div v-if="thumbnailProgress.active" class="thumbnail-progress">
        <div class="thumbnail-progress-head"><small>{{ thumbnailProgress.paused ? t('thumbnails.paused', { completed: thumbnailProgress.completed, total: thumbnailProgress.total }) : t('thumbnails.generating', { completed: thumbnailProgress.completed, total: thumbnailProgress.total }) }}</small><div>
          <button :aria-label="thumbnailProgress.paused ? t('thumbnails.resume') : t('thumbnails.pause')" :title="thumbnailProgress.paused ? t('thumbnails.resume') : t('thumbnails.pause')" @click="toggleThumbnailPause"><Play v-if="thumbnailProgress.paused" :size="14" /><Pause v-else :size="14" /></button>
          <button :aria-label="t('thumbnails.stop')" :title="t('thumbnails.stop')" @click="stopThumbnailQueue"><X :size="15" /></button>
        </div></div>
        <div class="thumbnail-progress-track"><i :style="{ width: thumbnailPercent }" /></div>
      </div>
      <div ref="queueList" class="queue-list">
        <template v-if="queue.length">
          <button v-for="(entry, i) in queue" :key="entry.path" class="queue-item" :class="{ current: i === index }" :data-path="entry.path" :style="{ '--custom-contextmenu': 'playlist-item', '--custom-contextmenu-data': encodeURIComponent(entry.path) }" @click="select(i, true)">
            <span class="queue-number"><Speaker v-if="i === index" :size="15" /><template v-else>{{ String(i + 1).padStart(2, '0') }}</template></span>
            <span class="thumb"><img v-if="entry.kind === 'video' && thumbnailReady[entry.path]" :src="`${entry.thumbnailUrl}&v=${thumbnailRevision}`" alt="" loading="lazy" @error="$event.currentTarget.style.display = 'none'"><LoaderCircle v-else-if="entry.kind === 'video' && thumbnailProgress.active && thumbnailProgress.current === entry.name" class="spin" :size="18" /><Film v-else :size="22" /><i v-if="i === index && playing"><span /><span /><span /></i></span>
            <span class="queue-copy"><strong>{{ title(entry.name) }}</strong><small>{{ [fileType(entry.name), durations[entry.path] ? fmt(durations[entry.path]) : '', fileSize(entry.size)].filter(Boolean).join(' · ') }}</small></span><ChevronRight :size="16" />
          </button>
        </template>
        <div v-else class="queue-empty"><ListVideo :size="28" /><p>{{ t('queue.empty') }}</p><button @click="openVideos">{{ t('queue.addVideo') }}</button></div>
      </div>
    </aside>

    <Transition name="notice">
      <div v-if="notice" class="operation-notice" :class="[notice.type, { 'transcode-notice': transcodeProgress.active }]" role="status" aria-live="polite">
        <LoaderCircle v-if="notice.type === 'progress'" class="spin" :size="22" />
        <CheckCircle2 v-else-if="notice.type === 'success'" :size="22" />
        <AlertCircle v-else :size="22" />
        <span>
          <strong>{{ notice.title }}</strong>
          <small>{{ notice.detail }}</small>
          <template v-if="transcodeProgress.active">
            <div class="transcode-progress-track">
              <div class="transcode-progress-fill" :style="{ width: `${transcodeProgress.percent}%` }"></div>
            </div>
            <div class="transcode-progress-meta">
              <span>{{ transcodeProgress.percent.toFixed(0) }}%</span>
              <span v-if="transcodeProgress.speed">{{ transcodeProgress.speed }}</span>
              <span v-if="transcodeProgress.fps > 0">{{ Math.round(transcodeProgress.fps) }} fps</span>
              <span v-if="transcodeProgress.totalTime > 0">{{ fmt(transcodeProgress.currentTime) }} / {{ fmt(transcodeProgress.totalTime) }}</span>
            </div>
          </template>
        </span>
        <button v-if="transcodeProgress.active" class="notice-stop" :title="t('transcode.cancelAction')" :aria-label="t('transcode.cancelAction')" @click="cancelTranscode"><X :size="14" /></button>
        <button class="notice-close" :aria-label="t('notice.close')" @click="notice = null"><X :size="15" /></button>
      </div>
    </Transition>

    <div v-if="showSheetDialog" class="shortcut-modal sheet-modal" role="dialog" aria-modal="true" :aria-label="t('sheet.settings')" @click.self="showSheetDialog = false">
      <section>
        <header><div><strong>{{ t('sheet.create') }}</strong></div><button class="icon-btn" :aria-label="t('common.close')" @click="showSheetDialog = false"><X :size="20" /></button></header>
        <p>{{ t('sheet.description') }}</p>
        <div class="sheet-options">
          <label><span>{{ t('sheet.frameCount') }}<small>{{ t('sheet.maxFrames') }}</small></span><span class="sheet-input"><input v-model.number="sheetFrameCount" type="number" min="1" max="60" step="1" autofocus><small>{{ t('sheet.frames') }}</small></span></label>
          <label><span>{{ t('sheet.interval') }}<small>{{ t('sheet.automatic') }}</small></span><output>{{ sheetSpacing ? t('sheet.seconds', { seconds: sheetSpacing.toFixed(1) }) : '—' }}</output></label>
          <label><span>{{ t('sheet.imageSize') }}<small>{{ t('sheet.frameDimensions') }}</small></span><select v-model.number="sheetImageWidth"><option v-for="size in sheetSizes" :key="size.value" :value="size.value">{{ size.label }}</option></select></label>
        </div>
        <footer><button class="cancel" @click="showSheetDialog = false">{{ t('sheet.cancel') }}</button><button class="confirm" @click="runContactSheet"><Images :size="17" />{{ t('sheet.confirm') }}</button></footer>
      </section>
    </div>

    <div v-if="showDeleteDialog" class="shortcut-modal sheet-modal delete-modal" role="dialog" aria-modal="true" :aria-label="t('delete.title')" @click.self="closeDeleteDialog">
      <section>
        <header><div><strong>{{ t('delete.title') }}</strong></div><button class="icon-btn" :aria-label="t('common.close')" @click="closeDeleteDialog"><X :size="20" /></button></header>
        <p>{{ t('delete.confirm', { name: (deleteTarget ?? item)?.name }) }}</p>
        <footer><button class="cancel" @click="closeDeleteDialog">{{ t('delete.cancel') }}</button><button class="confirm danger" autofocus @click="confirmDelete"><Trash2 :size="17" />{{ t('delete.action') }}</button></footer>
      </section>
    </div>

    <div v-if="showTranscodeDialog" class="shortcut-modal sheet-modal transcode-modal" role="dialog" aria-modal="true" :aria-label="t('transcode.title')" @click.self="showTranscodeDialog = false">
      <section>
        <header><div><strong>{{ t('transcode.title') }}</strong></div><button class="icon-btn" :aria-label="t('common.close')" @click="showTranscodeDialog = false"><X :size="20" /></button></header>
        <p>{{ t('transcode.description', { name: item?.name }) }}</p>
        <div class="transcode-options">
          <label v-for="option in transcodeOptions" :key="option.id" :class="{ selected: selectedTranscode === option.id }">
            <input v-model="selectedTranscode" type="radio" name="transcode-codec" :value="option.id">
            <span><strong>{{ option.codec }}</strong><small>{{ t(`transcode.${option.id}.description`) }}</small></span>
            <output>{{ t('transcode.estimatedSaving', { percent: option.estimatedSaving }) }}</output>
          </label>
        </div>
        <div class="transcode-trash-option">
          <label>
            <input v-model="transcodeTrashOriginal" type="checkbox">
            <span>{{ t('transcode.trashOriginal') }}</span>
          </label>
        </div>
        <div class="transcode-warning"><AlertCircle :size="17" /><span>{{ transcodeTrashOriginal ? t('transcode.warning') : t('transcode.warningKept') }}</span></div>
        <footer><button class="cancel" @click="showTranscodeDialog = false">{{ t('transcode.cancel') }}</button><button class="confirm" autofocus @click="runTranscode"><RefreshCw :size="17" />{{ t('transcode.confirm') }}</button></footer>
      </section>
    </div>

    <div v-if="showShortcuts" class="shortcut-modal" role="dialog" aria-modal="true" :aria-label="t('shortcuts.open')" @click.self="showShortcuts = false">
      <section>
        <header><div><strong>{{ t('shortcuts.title') }}</strong></div><button class="icon-btn" :aria-label="t('common.close')" @click="showShortcuts = false"><X :size="20" /></button></header>
        <dl>
          <div><dt>{{ t('shortcuts.playPause') }}</dt><dd><kbd>Space</kbd></dd></div>
          <div><dt>{{ t('shortcuts.seek', { seconds: seekStep }) }}</dt><dd><kbd>←</kbd><kbd>→</kbd></dd></div>
          <div><dt>{{ t('shortcuts.splitMarker') }}</dt><dd><kbd>B</kbd></dd></div>
          <div><dt>{{ t('shortcuts.markerAtPointer') }}</dt><dd><kbd>{{ t('shortcuts.rightClickTimeline') }}</kbd></dd></div>
          <div><dt>{{ t('shortcuts.previousNext') }}</dt><dd><kbd>⌘←</kbd><kbd>⌘→</kbd></dd></div>
          <div><dt>{{ t('shortcuts.delete') }}</dt><dd><kbd>Del</kbd><kbd>⌘⌫</kbd></dd></div>
          <div><dt>{{ t('shortcuts.fullscreen') }}</dt><dd><kbd>F</kbd></dd></div>
          <div><dt>{{ t('shortcuts.mute') }}</dt><dd><kbd>M</kbd></dd></div>
        </dl>
      </section>
    </div>
  </main>
</template>
