# ViPlay kalıcı kararlar

Yalnız karar değiştiğinde güncelle. Yeni kayıt biçimi: `DNN · YYYY-MM-DD · karar — gerekçe`.

## Aktif kararlar

- **D01 · 2026-08-17 · FFmpeg bundle edilmeyecek.** Kurulum ve güncelleme kullanıcı/sistem tarafından yönetilir; uygulama `PATH`, macOS'ta ayrıca `/opt/homebrew/bin` ve `/usr/local/bin` üzerinden arar.
- **D02 · 2026-08-17 · Split stream-copy kullanacak.** Kalite kaybı ve uzun encode süresi önlenir; kesim noktası en yakın uygun anahtar kareye kayabilir.
- **D03 · 2026-08-17 · Split çıktısı kaynak container uzantısını koruyacak.** İki parça önce kaynak klasörde geçici dizine yazılır, başarıdan sonra nihai adlara taşınır.
- **D04 · 2026-08-17 · Contact sheet FFmpeg filtre zinciriyle üretilecek.** `ffprobe` süreyi sağlar; `fps`, `scale`, `pad` ve `tile` temel zorunlu filtrelerdir.
- **D05 · 2026-08-17 · `drawtext` opsiyoneldir.** Bazı FFmpeg dağıtımları filtreyi içermez; yokluğunda timestamp atlanır ve contact sheet yine üretilir.
- **D06 · 2026-08-17 · FFmpeg hataları kullanıcıya açıklayıcı gösterilecek.** Kurulum eksikliği platform komutlarıyla anlatılır; işlem stderr'i en fazla 800 karaktere indirgenir, UI bu ayrıntıyı satır sararak gösterir.
- **D07 · 2026-08-17 · Pure Go decoder yalnız thumbnail için kalacak.** Kullanıcı tetiklemeli split/contact-sheet işlemleri FFmpeg'e aittir.
- **D08 · 2026-08-17 · Her tamamlanan kod değişikliğinden sonra test + Wails build zorunludur.** Sandbox ortamında Go cache `/private/tmp/viplay-go-build` kullanılmalıdır.

## Karar değiştirme kuralı

Eski kaydı silme. Kararı `Değiştirildi: DNN` notuyla işaretle ve yeni numaralı kararı ekle; böylece sonraki oturumlar gerekçeyi kaybetmez.

