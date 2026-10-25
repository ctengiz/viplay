import { Events } from '@wailsio/runtime'
import { PlayerLoad, PlayerSeek, PlayerSetMuted, PlayerSetPause, PlayerSetSpeed, PlayerSetSubtitle, PlayerSetVideoArea, PlayerSetVolume, PlayerStop } from '../bindings/viplay/app'

// Mirrors the subset of HTMLVideoElement used by App.vue so both engines share one code path.
export function createNativePlayer(handlers) {
  const state = { currentTime: 0, duration: 0, paused: true, videoWidth: 0, videoHeight: 0, volume: 1, playbackRate: 1 }
  const player = {
    get currentTime() { return state.currentTime },
    set currentTime(value) {
      state.currentTime = value
      PlayerSeek(value).catch(() => {})
    },
    get duration() { return state.duration },
    get paused() { return state.paused },
    get videoWidth() { return state.videoWidth },
    get videoHeight() { return state.videoHeight },
    get volume() { return state.volume },
    set volume(value) {
      state.volume = value
      PlayerSetVolume(value).catch(() => {})
    },
    get playbackRate() { return state.playbackRate },
    set playbackRate(value) {
      state.playbackRate = value
      PlayerSetSpeed(value).catch(() => {})
    },
    play() {
      state.paused = false
      return PlayerSetPause(false)
    },
    pause() {
      state.paused = true
      return PlayerSetPause(true).catch(() => {})
    },
    load(path, autoplay) {
      Object.assign(state, { currentTime: 0, duration: 0, paused: !autoplay, videoWidth: 0, videoHeight: 0 })
      return PlayerLoad(path, autoplay)
    },
    stop() { return PlayerStop().catch(() => {}) },
    setMuted(muted) { return PlayerSetMuted(muted).catch(() => {}) },
    setSubtitle(path) { return PlayerSetSubtitle(path || '').catch(() => {}) },
    setVideoArea(left, top, right, bottom) { return PlayerSetVideoArea(left, top, right, bottom).catch(() => {}) },
  }
  const event = { currentTarget: player }
  Events.On('player:event', ({ data }) => {
    switch (data.type) {
      case 'time-pos':
        state.currentTime = data.value
        handlers.timeupdate?.(event)
        break
      case 'duration':
        state.duration = data.value
        handlers.loadedmetadata?.(event)
        break
      case 'dwidth': state.videoWidth = data.value; break
      case 'dheight':
        state.videoHeight = data.value
        handlers.loadedmetadata?.(event)
        break
      case 'loaded':
        if (!state.paused) handlers.play?.(event)
        break
      case 'pause':
        state.paused = data.flag
        data.flag ? handlers.pause?.(event) : handlers.play?.(event)
        break
      case 'eof-reached':
        if (data.flag) handlers.ended?.(event)
        break
      case 'error':
        handlers.error?.(data.message)
        break
    }
  })
  return player
}
