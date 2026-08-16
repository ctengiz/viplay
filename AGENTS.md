# ViPlay agent guide

Bu dosya depo genelinde geçerlidir. Kısa tut; ayrıntıyı bağlantılı dosyalarda sakla.

## Oturum başlangıcı

1. `.agents/CONTEXT.md` dosyasını bir kez oku.
2. Yalnız mimari, medya işleme veya bağımlılık kararı gerekiyorsa `.agents/DECISIONS.md` dosyasını oku.
3. İlgili dosyaları `rg` ile hedefli ara; tüm depoyu tekrar tekrar tarama.
4. Mevcut kullanıcı değişikliklerini koru ve ilgisiz dosyalara dokunma.

## Çalışma biçimi

- Kullanıcıyla Türkçe ve sonuç odaklı iletişim kur.
- Değişiklik istenirse makul varsayımlarla uygula; yalnız anlamlı ürün kararı eksikse sor.
- Tanılama istenirse kullanıcı ayrıca istemedikçe kodu değiştirme.
- Aynı bilgiyi yorumlarda, belgelerde ve yanıtta tekrarlama.
- Alt ajanları yalnız kullanıcı açıkça isterse kullan.
- Kalıcı bir teknik karar değişirse `.agents/DECISIONS.md` dosyasını aynı görevde güncelle.
- Dosya haritası veya doğrulama komutları değişirse `.agents/CONTEXT.md` dosyasını güncelle.

## Değişmez ürün kararları

- FFmpeg/ffprobe uygulamayla bundle edilmez; sistem kurulumundan kullanılır.
- Video split, FFmpeg stream-copy ile en yakın anahtar kareden yapılır.
- Contact sheet FFmpeg ile üretilir; `drawtext` yoksa zaman damgası atlanır, işlem başarısız olmaz.
- Kullanıcıya gösterilen uzun hatalar kırpılmadan, satır sararak ve kaydırılabilir biçimde sunulur.
- Pure Go H.264 kodu yalnız hafif thumbnail önizlemesi için korunur.

## Zorunlu doğrulama

Her tamamlanan kod değişikliğinden sonra testleri ve tam uygulama build'ini çalıştır:

```bash
env GOCACHE=/private/tmp/viplay-go-build go test ./...
env GOCACHE=/private/tmp/viplay-go-build PATH=/Users/ct/go/bin:/Users/ct/.local/bin:/opt/homebrew/bin:/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin /Users/ct/go/bin/wails3 task build
git diff --check
```

- macOS linker sürüm uyarıları test/build başarılıysa bilinen ortam uyarısıdır.
- FFmpeg davranışı değişirse sistem FFmpeg'iyle sentetik medya entegrasyon testini çalıştır.
- Frontend davranışı değişirse mümkünse render edilmiş arayüzü doğrula; araç yoksa bunu sonuçta açıkça belirt.

## Teslim

Son yanıtta yalnız sonucu, değişen davranışı, doğrulama durumunu ve kalan gerçek riski belirt.

