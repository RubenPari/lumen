# Lumen

Media server self-hosted ispirato a [Jellyfin](https://github.com/jellyfin/jellyfin), riscritto in **Go** con una
web app nuova in **React + Tailwind**. Un unico binario statico (la UI è embeddata), database SQLite, nessuna
dipendenza esterna obbligatoria (ffmpeg è opzionale ma consigliato).

## Avvio rapido

```bash
make build           # compila la web app e il server → bin/lumen
./bin/lumen          # http://localhost:8096
```

Al primo avvio si apre il wizard: nome del server → account amministratore → librerie → chiave TMDB (facoltativa).

Con Docker:

```bash
docker build -t lumen .
docker run -d -p 8096:8096 -v lumen-config:/config -v /percorso/media:/media:ro lumen
```

### Opzioni

| Flag        | Variabile         | Default                    |
|-------------|-------------------|----------------------------|
| `-addr`     | `LUMEN_ADDR`      | `:8096`                    |
| `-data`     | `LUMEN_DATA_DIR`  | `~/.local/share/lumen`     |
| `-ffmpeg`   | `LUMEN_FFMPEG`    | cercato nel `PATH` (`none` per disattivare) |
| `-ffprobe`  | `LUMEN_FFPROBE`   | cercato nel `PATH`         |
| `-debug`    | `LUMEN_DEBUG`     | off                        |

Senza ffmpeg/ffprobe funziona solo la riproduzione diretta dei formati che il browser supporta
(MP4/WebM, MKV su Chrome/Firefox) e non vengono letti codec/tracce. Su Fedora: `sudo dnf install ffmpeg`
(RPM Fusion), su Debian/Ubuntu: `sudo apt install ffmpeg`.

## Funzionalità

- **Librerie Film e Serie TV** con lo stesso parsing dei nomi file di Jellyfin (porting di `Emby.Naming`:
  `S01E02`, `1x02`, multi-episodio, anime `[Group] Nome - 01`, cartelle stagione in più lingue,
  "Specials", anno nel nome, pulizia dei tag di release, file multi-parte `cd1/cd2`, extra e trailer).
- **Metadati locali**: NFO in stile Kodi (`movie.nfo`, `tvshow.nfo`, `<file>.nfo`), immagini locali
  (`poster`, `folder`, `fanart`, `logo`, `<file>-thumb`, `season01-poster`…), sottotitoli esterni con
  lingua/forced/SDH dal nome file.
- **TMDB** (facoltativo): titoli e trame localizzati, poster, sfondi, loghi, cast, valutazioni,
  classificazione per età, stagioni ed episodi. Metadati modificabili e bloccabili dall'admin.
- **Riproduzione**: direct play con Range request; se il browser non supporta il file, transcodifica
  **HLS** con ffmpeg (copia del video H.264 quando possibile, accelerazione hardware VA-API/QSV/NVENC/
  VideoToolbox, tone-mapping HDR→SDR, scelta qualità e traccia audio).
- **Sottotitoli** SRT/ASS/SSA/VTT esterni e incorporati (testuali), convertiti in WebVTT e resi in
  overlay con dimensione configurabile; sottotitoli a immagine **PGS/VobSub** impressi nel video durante la transcodifica.
- **DVD e Blu-ray** in cartella (`VIDEO_TS`, `BDMV`), riprodotti tramite ffmpeg.
- **Monitoraggio in tempo reale** delle cartelle (aggiunte/rimozioni rilevate in pochi secondi) e aggiornamento
  istantaneo della web app via Server-Sent Events.
- **Stato per utente**: posizione di ripresa, visto/non visto (anche per stagione o serie intera),
  preferiti, "Continua a guardare" e "Prossimi episodi" (stessa logica del `TVSeriesManager`).
- **Musica**: artisti, album (anche multi-disco "CD 1/CD 2"), brani; tag ID3/MP4/FLAC/OGG e copertine incorporate
  letti in Go puro; player audio persistente con coda, shuffle/repeat, controlli da tastiera/lock screen (Media Session);
  transcodifica al volo in MP3 per i formati non supportati dal browser.
- **Libri e fumetti**: EPUB (metadati OPF, copertina, serie Calibre/EPUB3), PDF, CBZ/CBR con lettore integrato
  (temi chiaro/scuro/seppia, dimensione testo, doppia pagina, modalità manga) e posizione di lettura sincronizzata.
- **Audiolibri**: cartelle di file audio o singoli file (m4b/mp3) con capitoli e ripresa dal punto esatto.
- **Foto**: album per cartella, cronologia per mese con data di scatto EXIF, rotazione automatica, lightbox
  con zoom/swipe e presentazione.
- **Utenti** multipli con password PBKDF2-SHA512 (come Jellyfin), ruoli admin, sessioni via cookie.
- **Dashboard admin**: statistiche, chi sta guardando cosa (direct/transcode), stato scansione,
  gestione librerie con selettore di cartelle, utenti, impostazioni.

## App Jellyfin

Lumen espone anche un sottoinsieme dell'API di Jellyfin (versione dichiarata 10.11), quindi puoi usare le app
esistenti puntandole a `http://<ip-del-server>:8096`: Jellyfin per Android TV, Findroid, Swiftfin, Infuse,
Streamyfin, Finamp (musica). Il server viene trovato anche in automatico (discovery UDP 7359).

Supportati: login, librerie, navigazione, ricerca, immagini, "Continua a guardare"/"Prossimi episodi",
PlaybackInfo con negoziazione dal DeviceProfile del client, riproduzione diretta, transcodifica HLS con
playlist VOD e salto in qualunque punto, sottotitoli esterni/incorporati, posizione e stato visto/preferito,
keep-alive WebSocket. Non supportati: SyncPlay, controllo remoto, QuickConnect, plugin, gestione del server
dalle app. Le app che incapsulano la web app di Jellyfin (Android "mobile", Jellyfin Media Player) mostrano
la web app di Lumen.

## UI/UX

- Home con hero cinematografico a rotazione (sfondo con effetto Ken Burns, logo del titolo, "Riprendi").
- Righe scorrevoli con snap, card con progresso, badge visto/preferito/episodi mancanti.
- Ricerca istantanea a palette (`Ctrl/⌘ K` o `/`) su film, serie, episodi e persone.
- Pagina dettaglio a tutto schermo con stagioni a schede, episodi con miniature, "finisce alle…",
  badge tecnici (4K/HDR/codec/canali), cast, titoli simili.
- Player custom: scorciatoie da tastiera (spazio/K, ←/→, J/L, F, M, C, N), anteprima del tempo e capitoli
  sulla barra, menu audio/qualità/velocità, Picture-in-Picture, "prossimo episodio" con conto alla rovescia.
- Libreria con filtri (da vedere, in corso, preferiti, genere), ordinamenti, indice A–Z, scroll infinito,
  pulsante "casuale".
- Responsive con tab bar su mobile, interfaccia in italiano o inglese (automatica dal browser).

## Struttura e corrispondenza con Jellyfin

| Lumen (Go)                 | Jellyfin (C#)                                                    |
|----------------------------|------------------------------------------------------------------|
| `internal/naming`          | `Emby.Naming` (regex identiche, valutate con `regexp2`; test portati da `Jellyfin.Naming.Tests`, inclusi musica, libri e audiolibri) |
| `internal/scanner`         | `Emby.Server.Implementations/Library/Resolvers`, `LibraryManager` |
| `internal/metadata/nfo.go` | `MediaBrowser.XbmcMetadata`                                      |
| `internal/metadata/images.go` | `MediaBrowser.LocalMetadata/Images/LocalImageProvider`        |
| `internal/metadata/tmdb.go`| `MediaBrowser.Providers/Plugins/Tmdb`                             |
| `internal/metadata/probe.go` | `MediaBrowser.MediaEncoding/Probing`                           |
| `internal/stream`          | `MediaBrowser.MediaEncoding`, `Jellyfin.Api` (DynamicHls, Subtitles) |
| `internal/api`             | `Jellyfin.Api` (API JSON propria + `jf_*.go`: sottoinsieme compatibile con Jellyfin) |
| `internal/db`              | `Jellyfin.Database` (SQLite, schema compatto)                     |
| `web/`                     | `jellyfin-web` (riscritto da zero)                                |

## Sviluppo

```bash
go run ./cmd/lumen -addr :8097 -data ./.dev/data   # backend
make dev                                          # frontend con hot reload su :5173
make test                                         # go vet + test + typecheck
```

## Limiti attuali

- Compatibilità Jellyfin parziale (vedi sopra); testata con le sequenze di chiamate dei client, non ancora
  su ogni app reale.
- Niente Live TV (in arrivo).
- I formati `.mobi`/`.azw3`/`.fb2` sono catalogati ma solo scaricabili, non leggibili nel browser; le foto HEIC
  si vedono solo nei browser che le supportano.
- Le immagini disco `.iso` non sono supportate (le cartelle `VIDEO_TS`/`BDMV` sì).
- Il monitoraggio in tempo reale usa inotify: su cartelle di rete (NFS/SMB) e su `/mnt/c` in WSL non riceve eventi,
  lì restano le scansioni periodiche.

## Licenza

GPL-2.0, come Jellyfin, da cui derivano la logica di parsing dei nomi (`internal/naming`) e i relativi casi di test.
