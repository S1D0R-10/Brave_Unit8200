# Pipeline

Cztery serwisy, jeden bucket, jedno pole w każdej wiadomości: **klucz obiektu**.
Każdy etap wywodzi z niego resztę nazw sam, więc nikt nikomu nie przekazuje
metadanych, ścieżek ani treści.

```
przeglądarka
    │  1. POST /api/upload            → presigned PUT + klucz
    │  2. PUT <presigned url>         → plik ląduje w buckecie
    │  3. POST /api/ingest {key}
    ▼
  web ──────────── .mp4 ──────────▶  stt   POST /api/v1/ingest {key}   → 202
    │                                  │   pobiera film z bucketa
    │                                  │   transkrybuje (whisper.cpp)
    │                                  │   PUT {base}-transcription.txt
    │                                  ▼
    └── .pdf .txt .md .html ──────▶ embedder   POST /verity {key}
                                        │   .mp4  → czyta {base}-transcription.txt
                                        │   .pdf  → ekstrahuje tekst, PUT {base}-extracted.txt
                                        │   .txt  → czyta wprost
                                        │   tnie na chunki, embeduje
                                        ▼
                                     Qdrant (kolekcja `citations`)
                                     wektor + offsety bajtowe, ZERO tekstu
```

Zapytanie idzie z drugiej strony i nigdy nie dotyka powyższego:

```
rag-search  POST /draft {question}
    │  embeduje pytanie → szuka w Qdrancie
    │  dla trafienia: GET <file_key>  Range: bytes=start-end   ← tekst cytatu
    │  buduje prompt z cytatami → OpenRouter
    ▼  {status, sentences[], sources[]}
```

## Konwencja nazw w buckecie

Wszystko wisi na jednym kluczu i deterministycznych sufiksach:

| Wgrany plik | Obiekt z tekstem (`file_key`) | Kto go tworzy |
|---|---|---|
| `film.mp4` | `film-transcription.txt` | stt |
| `raport.pdf` | `raport-extracted.txt` | embedder |
| `notatki.txt` | `notatki.txt` | nikt, jest od razu |

Dlatego stt oddaje embedderowi klucz **filmu**, a nie transkrypcji — embedder
sam wie, jak się nazywa jej plik.

## Dlaczego offsety bajtowe

`chunk_id` to zawsze `{startByte}-{endByte}` w obiekcie wskazanym przez
`file_key`. Dzięki temu trafienie w Qdrancie zamienia się wprost w nagłówek
`Range: bytes=…`, a tekst nie musi leżeć zduplikowany w bazie wektorowej.
Ruch bucket → backend jest darmowy, indeks zostaje mały, a cytat nie może się
rozjechać ze źródłem.

Konsekwencja: **PDF nie da się czytać po bajtach**, bo surowe bajty PDF-a to nie
tekst. Dlatego embedder raz przy indeksowaniu wyciąga z niego tekst i odkłada
obok jako `-extracted.txt`. To jedyny push do bucketa poza uploadem pliku i
uploadem transkrypcji.

Payload punktu w Qdrancie:

```
file_hash    sha256 obiektu z tekstem — offsety są ważne tylko względem niego
file_key     co czytać Range'em
source_key   co użytkownik faktycznie wgrał (tytuł cytatu)
file_ext     rozszerzenie source_key
chunk_id     "{startByte}-{endByte}"
start_ms     tylko transkrypcje — moment w nagraniu
end_ms       tylko transkrypcje
indexed_at   RFC3339, na potrzeby /kb/stats
```

Ponowne zaindeksowanie tego samego klucza kasuje wcześniejsze punkty, bo
zmieniona treść unieważnia wszystkie offsety.

## Zmienne środowiskowe

Poświadczenia bucketa trzyma `embedder`, reszta bierze je przez referencje
Railway (`${{embedder.RAG_S3_*}}`). Różnią się tylko nazwy: serwisy w Go czytają
`RAG_S3_*`, a AWS SDK w Next `S3_*`.

| Serwis | Zmienne |
|---|---|
| `web` | `S3_ENDPOINT`, `S3_BUCKET_NAME`, `S3_REGION`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`, `EMBEDDER_URL`, `STT_URL`, `RAG_SEARCH_URL` |
| `embedder` | `RAG_S3_*`, `RAG_EMBED_*`, `RAG_QDRANT_*`, `RAG_CHUNK_WORDS`, `RAG_CHUNK_SECS` |
| `rag-search` | `RAG_S3_*`, `RAG_EMBED_*`, `RAG_CHAT_*`, `RAG_QDRANT_*`, `RAG_MAX_QUOTE_BYTES` |
| `stt` | `RAG_S3_*`, `EMBEDDER_URL`, `STT_*` |

Bez poświadczeń nic nie wybucha: `stt` zwraca 501 na `/api/v1/ingest` i dalej
działa jako samodzielny panel, a `rag-search` bez `RAG_CHAT_*` odda same cytaty.

## Co pęka i jak to widać

- **Film wgrany, ale nie ma transkrypcji** — embedder odpowie
  `transcript "…" for "…" not found — did stt run?`. Znaczy, że stt nie
  dokończył albo nie miał `EMBEDDER_URL`.
- **Trafienia bez `file_key`** — punkty z czasów sprzed adresowania bajtowego.
  rag-search użyje ich starego `text` z payloadu, jeśli tam jest, i zaloguje
  ostrzeżenie. Lekarstwem jest ponowne zaindeksowanie dokumentu.
- **`/api/ingest` zwraca 415** — rozszerzenie spoza listy. Filmy to na razie
  wyłącznie `.mp4`, bo tyle przyjmuje stt.
