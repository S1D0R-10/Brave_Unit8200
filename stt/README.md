# Brave STT

Serwis transkrypcji polskich plików MP4. API i panel są napisane w Go, a inferencję wykonuje `whisper.cpp` **na CPU** (obraz Dockera bez CUDA, gotowy pod hosting na Railway). Każde ukończone zadanie zapisuje segmenty jako pojedynczy plik `{nazwa-mp4}-transcription.txt`, gdzie każda linia ma format `[startMs - endMs] tekst` (timestampy w milisekundach).

Jeśli host ma GPU NVIDIA z `nvidia-smi`, kalibracja wykorzysta pomiar VRAM do doboru liczby workerów; bez GPU serwis działa czysto na CPU i honoruje `STT_MAX_WORKERS`.

## Uruchomienie lokalne (Docker, CPU)

Wymagania: Docker Desktop oraz trochę wolnego miejsca (model ~550 MB + artefakty).

1. Skopiuj filmy do `data/inbox/`.
2. Zbuduj i uruchom serwis:

   ```powershell
   docker compose up --build -d
   ```

3. Otwórz [http://localhost:8000](http://localhost:8000), zaznacz filmy i wybierz **Uruchom wybrane**.
4. Wyniki znajdziesz również bezpośrednio w `data/output/<job-id>/`.

Pierwszy wsad rozpoczyna się kalibracją (do 5 minut na pierwszym filmie), która wylicza real-time factor i wstępne ETA.

## Hosting na Railway

Repo zawiera `railway.json` (builder `DOCKERFILE`, healthcheck `/healthz`). Na CPU transkrypcja jest wolna — dwugodzinny film potrafi liczyć się wielokrotnie dłużej niż na GPU.

1. Utwórz serwis z tego repo; Railway zbuduje `Dockerfile`.
2. Podłącz **Volume** zamontowany pod `/data`, żeby `data/output` przetrwał redeploy (statusy w pamięci i tak znikają po restarcie).
3. Aplikacja słucha na `$PORT` wstrzykiwanym przez Railway (fallback `:8000`).
4. Zmienne: ustaw `STT_MAX_WORKERS` wg rozmiaru instancji (domyślnie `1`), ewentualnie `STT_THREADS` i `STT_MIN_FREE_BYTES`.
5. Upload z panelu wymaga większego limitu ciała żądania — przy dużych MP4 preferuj wgranie plików do wolumenu w `/data/inbox` i użycie wsadu.

## API

Kontrakt jest opisany w [`openapi.yaml`](openapi.yaml). Najważniejsze operacje:

- `POST /api/v1/transcriptions` — upload jednego MP4 jako pole `file`.
- `POST /api/v1/batches` — wsad z plików obecnych w `data/inbox`.
- `GET /api/v1/transcriptions` — kolejka, postęp i zbiorcze ETA.
- `GET /api/v1/transcriptions/{id}/result` — wynik JSON (w pamięci) gotowy do późniejszego przekazania do RAG-a.
- `GET /api/v1/transcriptions/{id}/artifacts/txt` — pobranie pliku `{nazwa-mp4}-transcription.txt`.

Statusy zadań są przechowywane w pamięci przez 24 godziny i znikają po restarcie. Gotowe artefakty pozostają na dysku. Pliki z inboxa są zamontowane tylko do odczytu.

## Konfiguracja

| Zmienna | Domyślna wartość | Znaczenie |
|---|---:|---|
| `PORT` | – | Port wstrzykiwany przez Railway; nadpisuje domyślne `:8000` (o ile nie ustawiono `STT_ADDRESS`). |
| `STT_MAX_WORKERS` | `2` (obraz: `1`) | Maksymalna liczba procesów Whisper; dozwolone 1–2. Na CPU kalibracja przyjmuje tę wartość. |
| `STT_INITIAL_WORKERS` | `1` | Liczba workerów przed kalibracją. |
| `STT_THREADS` | `NumCPU/2` (obraz: `4`) | Wątki CPU dla jednego procesu Whisper. |
| `STT_MAX_UPLOAD_BYTES` | `3221225472` | Limit uploadu, czyli 3 GiB. |
| `STT_MIN_FREE_BYTES` | `1073741824` (obraz: `536870912`) | Minimalne wolne miejsce wymagane przez `/readyz`. |
| `STT_GPU_HEADROOM_MIB` | `1024` | Zapas VRAM przy doborze workerów — tylko gdy host ma GPU. |

## Rozwój i testy

```powershell
go test -race ./...
go vet ./...
go build ./cmd/stt
docker compose config --quiet
```

Testy API korzystają z kontrolowanego runnera i nie wymagają GPU. Pełny test transkrypcji wymaga krótkiego polskiego MP4 w `data/inbox` oraz zbudowanego obrazu (`docker compose up --build`).
