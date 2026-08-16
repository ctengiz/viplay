# ViPlay

ViPlay, Go + Wails v3 + Vue 3 ile geliştirilmiş macOS, Linux ve Windows masaüstü video oynatıcısıdır.

## Gereksinimler

- Go 1.24+
- Node.js 20+
- Wails platform bağımlılıkları: https://wails.io/docs/gettingstarted/installation

## Geliştirme

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.8
cd frontend && npm install && cd ..
wails3 dev
```

## Derleme

Aktif işletim sistemi için:

```bash
wails3 task package
```

Dağıtım çıktısı `bin` altında oluşur. Windows derlemesi Windows'ta, macOS uygulama paketi macOS'ta ve Linux paketi Linux'ta üretilmelidir; CI matrisi üç platformu da kapsar.

## Özellikler

- Birden fazla yerel video seçme ve oynatma listesi
- Oynat/duraklat, ileri/geri sarma, önceki/sonraki video
- Ses, sessize alma, oynatma hızı ve tam ekran
- WebVTT altyazı dosyası ekleme
- Klavye kısayolları: `Space`, `←`, `→`, `F`, `M`, klasörde gezinmek için `⌘←` / `⌘→`, videoyu diskten silmek için `⌘⌫`
- İsteğe bağlı `ffprobe` entegrasyonuyla video/ses codec, kapsayıcı, çözünürlük, FPS ve dosya boyutu bilgileri
- Range istekleriyle güvenli yerel medya akışı

Codec desteği işletim sisteminin WebView medya motoruna bağlıdır. En geniş ortak destek için H.264/AAC içeren MP4 önerilir.
