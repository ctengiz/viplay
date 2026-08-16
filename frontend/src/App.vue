<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { AlertCircle, Captions, CheckCircle2, ChevronDown, ChevronRight, Film, FolderOpen, Gauge, Images, Keyboard, ListVideo, LoaderCircle, Maximize, Minimize, Pause, Play, RotateCcw, RotateCw, Scissors, SkipBack, SkipForward, Speaker, Trash2, Volume2, VolumeX, X } from '@lucide/vue'
import { Window } from '@wailsio/runtime'
import { DeleteVideo, DirectoryVideos, ExtractContactSheet, MarkPlayed, OpenSubtitle, OpenVideos, ProbeMedia, RecentVideos, SplitVideo } from '../bindings/viplay/app'

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
const sheetFrameCount = ref(12)
const sheetImageWidth = ref(320)
const mediaInfo = ref(null)
const libraryView = ref('folder')
const processing = ref('')
const notice = ref(null)
let noticeTimer
const item = computed(() => queue.value[index.value])
const progress = computed(() => `${duration.value ? current.value / duration.value * 100 : 0}%`)
const volumeProgress = computed(() => `${volume.value * 100}%`)
const speeds = [.5, .75, 1, 1.25, 1.5, 2]
const sheetSizes = [{ label: 'Küçük · 240×135', value: 240 }, { label: 'Orta · 320×180', value: 320 }, { label: 'Büyük · 480×270', value: 480 }, { label: 'Çok büyük · 640×360', value: 640 }]
const sheetSpacing = computed(() => duration.value > 0 ? duration.value / (Number(sheetFrameCount.value) + 1) : 0)

function notify(type, title, detail = '') {
  window.clearTimeout(noticeTimer)
  notice.value = { type, title, detail }
  if (type !== 'progress') noticeTimer = window.setTimeout(() => { notice.value = null }, type === 'error' ? 7000 : 5000)
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
  if (!item.value) { notify('error', 'Video seçilmedi', 'Önce işlem yapılacak bir video açın.'); return }
  if (processing.value) return
  if (video.value && !video.value.paused) video.value.pause()
  const splitAt = video.value?.currentTime ?? current.value
  if (splitAt <= 0 || splitAt >= duration.value) { notify('error', 'Bölme noktası geçersiz', 'İmleci videonun başlangıç ve bitişi arasına getirin.'); return }
  processing.value = 'split'
  notify('progress', 'Video bölünüyor', `${fmt(splitAt)} noktasındaki anahtar kare hazırlanıyor…`)
  await nextTick()
  try {
    const result = await SplitVideo(item.value.path, splitAt)
    notify('success', 'Video başarıyla bölündü', `${fmt(result.splitTime)} · ${result.firstPath} · ${result.secondPath}`)
    if (libraryView.value === 'folder') {
      const activePath = item.value.path
      queue.value = await DirectoryVideos(activePath)
      index.value = queue.value.findIndex(entry => entry.path === activePath)
    }
  } catch (error) { notify('error', 'Video bölünemedi', String(error)) }
  finally { processing.value = '' }
}

function createContactSheet() {
  if (!item.value) { notify('error', 'Video seçilmedi', 'Önce kareleri çıkarılacak bir video açın.'); return }
  if (processing.value) return
  showSheetDialog.value = true
}

async function runContactSheet() {
  const count = Math.round(Number(sheetFrameCount.value))
  const imageWidth = Number(sheetImageWidth.value)
  if (!Number.isFinite(count) || count < 1 || count > 60) { notify('error', 'Kare sayısı geçersiz', '1 ile 60 arasında bir kare sayısı girin.'); return }
  showSheetDialog.value = false
  processing.value = 'sheet'
  notify('progress', 'Contact sheet hazırlanıyor', `${count} kare · yaklaşık ${sheetSpacing.value.toFixed(1)} saniye aralıkla…`)
  await nextTick()
  try {
    const output = await ExtractContactSheet(item.value.path, count, imageWidth)
    notify('success', 'Contact sheet oluşturuldu', output)
  } catch (error) { notify('error', 'Contact sheet oluşturulamadı', String(error)) }
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
    try { await video.value?.play() } catch { /* Oynatma kontrolü kullanıcıya açık kalır. */ }
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

async function deleteCurrent() {
  if (!item.value) return
  const deleting = item.value
  if (!window.confirm(`“${deleting.name}” diskinizden kalıcı olarak silinsin mi? Bu işlem geri alınamaz.`)) return
  await DeleteVideo(deleting.path)
  queue.value.splice(index.value, 1)
  if (index.value >= queue.value.length) index.value = Math.max(0, queue.value.length - 1)
  current.value = 0
  playing.value = false
  subtitle.value = null
  mediaInfo.value = null
}

function loaded(event) {
  duration.value = event.currentTarget.duration
  event.currentTarget.volume = volume.value
  event.currentTarget.playbackRate = speed.value
  mediaInfo.value = { ...mediaInfo.value, width: mediaInfo.value?.width || event.currentTarget.videoWidth, height: mediaInfo.value?.height || event.currentTarget.videoHeight, duration: mediaInfo.value?.duration || event.currentTarget.duration }
}

function onKey(event) {
  if (event.key === 'Escape' && showSheetDialog.value) { showSheetDialog.value = false; return }
  if (event.key === 'Escape' && showShortcuts.value) { showShortcuts.value = false; return }
  if (showShortcuts.value) return
  if (['INPUT', 'BUTTON'].includes(document.activeElement?.tagName) && event.code === 'Space') return
  const player = video.value
  if (event.metaKey && event.code === 'ArrowRight') { event.preventDefault(); navigateDirectory(1); return }
  if (event.metaKey && event.code === 'ArrowLeft') { event.preventDefault(); navigateDirectory(-1); return }
  if (event.metaKey && event.code === 'Backspace') { event.preventDefault(); deleteCurrent(); return }
  if (event.code === 'Space' && player) { event.preventDefault(); player.paused ? player.play() : player.pause() }
  if (event.code === 'ArrowRight' && player) player.currentTime = Math.min(player.duration || 0, player.currentTime + seekStep.value)
  if (event.code === 'ArrowLeft' && player) player.currentTime = Math.max(0, player.currentTime - seekStep.value)
  if (event.key.toLowerCase() === 'f') fullscreen()
  if (event.key.toLowerCase() === 'm') muted.value = !muted.value
}

watch(speed, value => { if (video.value) video.value.playbackRate = value })
watch(volume, value => { if (video.value) video.value.volume = value })
watch(item, async value => { mediaInfo.value = value ? await ProbeMedia(value.path) : null }, { immediate: true })
onMounted(() => window.addEventListener('keydown', onKey))
onBeforeUnmount(() => { window.removeEventListener('keydown', onKey); window.clearTimeout(noticeTimer) })
</script>

<template>
  <main class="app" :class="{ 'queue-closed': !showQueue }">
    <aside class="sidebar">
      <div class="brand"><span class="brand-mark"><Play :size="14" fill="currentColor" /></span>ViPlay</div>
      <nav>
        <span class="nav-title">Kütüphane</span>
        <button class="open-button" @click="openVideos"><FolderOpen :size="19" />Video aç</button>
        <button class="nav-item" :class="{ selected: libraryView === 'folder' }"><Film :size="18" />Tüm videolar<span>{{ libraryView === 'folder' ? queue.length : '' }}</span></button>
        <button class="nav-item" :class="{ selected: libraryView === 'recent' }" @click="loadRecent"><RotateCcw :size="18" />Son oynatılanlar</button>
      </nav>
      <div class="media-sidebar">
        <span>Video bilgisi</span>
        <dl v-if="item">
          <div><dt>Video</dt><dd>{{ mediaInfo?.videoCodec?.toUpperCase() || 'Bilinmiyor' }}</dd></div>
          <div><dt>Ses</dt><dd>{{ mediaInfo?.audioCodec?.toUpperCase() || 'Bilinmiyor' }}</dd></div>
          <div><dt>Boyut</dt><dd>{{ mediaInfo?.width ? `${mediaInfo.width}×${mediaInfo.height}` : '—' }}</dd></div>
          <div><dt>Format</dt><dd>{{ mediaInfo?.container?.toUpperCase() || '—' }}</dd></div>
          <div><dt>FPS</dt><dd>{{ mediaInfo?.fps || '—' }}</dd></div>
          <div><dt>Dosya</dt><dd>{{ fileSize(mediaInfo?.size) }}</dd></div>
        </dl>
        <p v-else>Video seçildiğinde teknik bilgiler burada görünür.</p>
      </div>
      <button class="shortcut-trigger" @click="showShortcuts = true"><Keyboard :size="18" /><span>Klavye kısayolları</span></button>
    </aside>

    <section class="player-shell">
      <header class="topbar">
        <div><span class="eyebrow">ŞİMDİ OYNATILIYOR</span><strong :title="item?.name">{{ item?.name || 'Bir video seçin' }}</strong></div>
        <div class="topbar-actions">
          <button class="icon-btn operation-btn" :class="{ processing: processing === 'split' }" aria-label="Geçerli noktadan H.264 videoyu böl" title="H.264 MP4/MOV videoyu geçerli noktadan ikiye böl" @click="splitCurrent"><LoaderCircle v-if="processing === 'split'" class="spin" :size="20" /><Scissors v-else :size="19" /></button>
          <button class="icon-btn operation-btn" :class="{ processing: processing === 'sheet' }" aria-label="Contact sheet oluştur" title="Belirli aralıklarla contact sheet oluştur" @click="createContactSheet"><LoaderCircle v-if="processing === 'sheet'" class="spin" :size="20" /><Images v-else :size="19" /></button>
          <button class="icon-btn danger" :disabled="!item" aria-label="Videoyu diskten sil" title="Videoyu sil (⌘⌫)" @click="deleteCurrent"><Trash2 :size="19" /></button>
          <button class="icon-btn" :class="{ active: showQueue }" aria-label="Oynatma listesini göster" title="Oynatma listesini göster" @click="showQueue = !showQueue"><ListVideo :size="20" /></button>
        </div>
      </header>

      <div class="stage" @dblclick="fullscreen">
        <video v-if="item" :key="item.url" ref="video" :src="item.url" :muted="muted" @click="toggle" @play="markPlayed" @pause="playing = false" @timeupdate="current = $event.currentTarget.currentTime" @loadedmetadata="loaded" @ended="next">
          <track v-if="subtitle" default kind="subtitles" :src="subtitle.url" srcLang="tr" :label="subtitle.name">
        </video>
        <div v-else class="empty-state">
          <div class="empty-icon"><Film :size="42" /></div>
          <h1>Perde hazır.</h1><p>İzlemeye başlamak için bilgisayarınızdan bir video seçin.</p>
          <button @click="openVideos"><FolderOpen :size="18" />Video aç</button>
          <small>MP4, MOV, WebM ve sisteminizin desteklediği formatlar</small>
        </div>
        <button v-if="item" class="center-play" :aria-label="playing ? 'Duraklat' : 'Oynat'" @click="toggle">
          <Pause v-if="playing" :size="34" fill="currentColor" /><Play v-else :size="36" fill="currentColor" />
        </button>
        <div class="bottom-fade" />
      </div>

      <div class="controls">
        <div class="timeline-row"><span>{{ fmt(current) }}</span><input aria-label="İlerleme" type="range" min="0" :max="duration || 0" step=".1" :value="current" :style="{ '--progress': progress }" @input="seek"><span>{{ fmt(duration) }}</span></div>
        <div class="control-row">
          <div class="volume-group">
            <button class="icon-btn" :aria-label="muted ? 'Sesi aç' : 'Sessize al'" @click="muted = !muted"><VolumeX v-if="muted || volume === 0" :size="20" /><Volume2 v-else :size="20" /></button>
            <input v-model.number="volume" aria-label="Ses" type="range" min="0" max="1" step=".01" :style="{ '--progress': volumeProgress }">
          </div>
          <div class="transport">
            <button class="icon-btn" :aria-label="`${seekStep} saniye geri`" @click="seekBy(-seekStep)"><RotateCcw :size="19" /></button>
            <button class="icon-btn" aria-label="Önceki" @click="previous"><SkipBack :size="21" fill="currentColor" /></button>
            <button class="primary-play" :aria-label="playing ? 'Duraklat' : 'Oynat'" @click="toggle"><Pause v-if="playing" fill="currentColor" /><Play v-else fill="currentColor" /></button>
            <button class="icon-btn" aria-label="Sonraki" @click="next"><SkipForward :size="21" fill="currentColor" /></button>
            <button class="icon-btn" :aria-label="`${seekStep} saniye ileri`" @click="seekBy(seekStep)"><RotateCw :size="19" /></button>
          </div>
          <div class="tools">
            <label class="seek-step" title="İleri/geri sarma süresi"><input v-model.number="seekStep" aria-label="Sarma saniyesi" type="number" min="1" max="300" step="1" @change="normaliseSeekStep"><span>sn</span></label>
            <button class="tool" :class="{ active: subtitle }" @click="openSubtitle"><Captions :size="20" /><span>Altyazı</span></button>
            <label class="speed"><Gauge :size="19" /><select v-model.number="speed" aria-label="Oynatma hızı"><option v-for="value in speeds" :key="value" :value="value">{{ value }}×</option></select><ChevronDown :size="14" /></label>
            <button class="icon-btn" :class="{ active: fullscreenActive }" :aria-label="fullscreenActive ? 'Tam ekrandan çık' : 'Tam ekran'" @click="fullscreen"><Minimize v-if="fullscreenActive" :size="20" /><Maximize v-else :size="20" /></button>
          </div>
        </div>
      </div>
    </section>

    <aside v-if="showQueue" class="queue">
      <div class="queue-head"><div><span>Oynatma listesi</span><strong>Sıradaki</strong></div><button class="icon-btn" aria-label="Listeyi kapat" @click="showQueue = false"><X :size="20" /></button></div>
      <div class="queue-list">
        <template v-if="queue.length">
          <button v-for="(entry, i) in queue" :key="entry.path" class="queue-item" :class="{ current: i === index }" @click="select(i, true)">
            <span class="queue-number"><Speaker v-if="i === index" :size="15" /><template v-else>{{ String(i + 1).padStart(2, '0') }}</template></span>
            <span class="thumb"><img v-if="entry.kind === 'video'" :src="entry.thumbnailUrl" alt="" loading="lazy" @error="$event.currentTarget.style.display = 'none'"><Film :size="22" /><i v-if="i === index && playing"><span /><span /><span /></i></span>
            <span class="queue-copy"><strong>{{ title(entry.name) }}</strong><small>{{ entry.kind === 'audio' ? 'Ses' : 'Yerel video' }}</small></span><ChevronRight :size="16" />
          </button>
        </template>
        <div v-else class="queue-empty"><ListVideo :size="28" /><p>Listeniz boş</p><button @click="openVideos">Video ekle</button></div>
      </div>
      <button class="add-more" @click="openVideos"><FolderOpen :size="17" />Listeye video ekle</button>
    </aside>

    <Transition name="notice">
      <button v-if="notice" class="operation-notice" :class="notice.type" role="status" aria-live="polite" @click="notice = null">
        <LoaderCircle v-if="notice.type === 'progress'" class="spin" :size="22" />
        <CheckCircle2 v-else-if="notice.type === 'success'" :size="22" />
        <AlertCircle v-else :size="22" />
        <span><strong>{{ notice.title }}</strong><small>{{ notice.detail }}</small></span><X :size="15" />
      </button>
    </Transition>

    <div v-if="showSheetDialog" class="shortcut-modal sheet-modal" role="dialog" aria-modal="true" aria-label="Contact sheet ayarları" @click.self="showSheetDialog = false">
      <section>
        <header><div><span>Görsel çıkarıcı</span><strong>Contact sheet oluştur</strong></div><button class="icon-btn" aria-label="Kapat" @click="showSheetDialog = false"><X :size="20" /></button></header>
        <p>Kareler video süresine eşit aralıklarla dağıtılır ve her görselin üzerine zaman bilgisi eklenir.</p>
        <div class="sheet-options">
          <label><span>Kare sayısı<small>En fazla 60</small></span><span class="sheet-input"><input v-model.number="sheetFrameCount" type="number" min="1" max="60" step="1" autofocus><small>kare</small></span></label>
          <label><span>Kare aralığı<small>Otomatik hesaplanır</small></span><output>{{ sheetSpacing ? `${sheetSpacing.toFixed(1)} saniye` : '—' }}</output></label>
          <label><span>Görsel boyutu<small>Her bir karenin ölçüsü</small></span><select v-model.number="sheetImageWidth"><option v-for="size in sheetSizes" :key="size.value" :value="size.value">{{ size.label }}</option></select></label>
        </div>
        <footer><button class="cancel" @click="showSheetDialog = false">Vazgeç</button><button class="confirm" @click="runContactSheet"><Images :size="17" />Oluştur</button></footer>
      </section>
    </div>

    <div v-if="showShortcuts" class="shortcut-modal" role="dialog" aria-modal="true" aria-label="Klavye kısayolları" @click.self="showShortcuts = false">
      <section>
        <header><div><span>Klavye</span><strong>Kısayollar</strong></div><button class="icon-btn" aria-label="Kapat" @click="showShortcuts = false"><X :size="20" /></button></header>
        <dl>
          <div><dt>Oynat / duraklat</dt><dd><kbd>Space</kbd></dd></div>
          <div><dt>{{ seekStep }} saniye sar</dt><dd><kbd>←</kbd><kbd>→</kbd></dd></div>
          <div><dt>Önceki / sonraki video</dt><dd><kbd>⌘←</kbd><kbd>⌘→</kbd></dd></div>
          <div><dt>Videoyu diskten sil</dt><dd><kbd>⌘⌫</kbd></dd></div>
          <div><dt>Tam ekran</dt><dd><kbd>F</kbd></dd></div>
          <div><dt>Sessize al</dt><dd><kbd>M</kbd></dd></div>
        </dl>
      </section>
    </div>
  </main>
</template>
