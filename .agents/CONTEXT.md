# ViPlay hızlı bağlam

## Teknoloji

- Go 1.25, Wails v3 beta.8
- Vue 3, Vite, Lucide
- Masaüstü hedefleri: macOS, Windows, Linux
- Harici çalışma zamanı araçları: `ffmpeg`, `ffprobe`

## Dosya haritası

- `app.go`: Wails API modelleri ve kullanıcı işlemleri
- `main.go`: uygulama başlangıcı ve yetkili yerel medya sunucusu
- `media_analysis.go`: MP4 analizi, thumbnail ve recent-store
- `media_tools.go`: FFmpeg split/contact-sheet ve H.264 thumbnail decoder
- `frontend/src/App.vue`: uygulama durumu, kullanıcı akışları ve template
- `frontend/src/styles.css`: tüm görünüm ve bildirim stilleri
- `main_test.go`: backend, FFmpeg hata mesajı ve medya entegrasyon testleri
- `Taskfile.yml`, `build/`: Wails build/package görevleri

## Hızlı yönlendirme

- Split veya contact sheet: önce `media_tools.go`, sonra ilgili `App.vue` handler'ı.
- Hata bildirimi: `notify()` ve `.operation-notice`.
- Medya yetkilendirmesi: `mediaServer.isAllowed`; yeni dosya işlemlerinde atlama.
- Çıktılar kaynak videonun yanında, çakışmayan adla ve geçici dosyadan atomik taşıma ile oluşturulur.
- `ffmpeg` komutlarını shell string'iyle değil `exec.Command` argümanlarıyla çalıştır.

## Güncel davranış

- Split yeniden kodlamaz; gerçek kesim zamanı ilk parçadan `ffprobe` ile ölçülür.
- Contact sheet eşit aralıklı kareleri 4 sütunlu JPEG grid'e dönüştürür.
- FFmpeg filtresi çalışma anında sorgulanır; Homebrew FFmpeg'de eksik olabilen `drawtext` opsiyoneldir.
- FFmpeg bulunamazsa platforma göre kurulum komutları içeren Türkçe hata gösterilir.
- Hata toast'ı 15 saniye kalır; ayrıntı sarılır, kaydırılır ve seçilebilir.

## Çalışma ağacı

Depo kirli olabilir. `git status --short` ile kontrol et; kullanıcı değişikliklerini geri alma. Build çıktıları `bin/` ve `frontend/dist/` altında üretilir.

