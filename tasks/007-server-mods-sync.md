# 007 — Моды сервера: автоматический список и файлы из MinIO

Цель: страница `/mods` и архив «Скачать всё» больше не зависят от ручного списка ModDB.
Список и файлы берутся с игрового сервера автоматически: `VintageAPI` при старте читает
загруженные моды, отбирает только клиентские и складывает их в MinIO; `vsservice` читает
манифест из MinIO и отдаёт сайту. NATS не используется.

## Топология

- Игровой сервер: `root@play.lasthearth.ru`, моды в `/root/vs/vsserver-data/Mods`, внутри контейнера.
- `VintageAPI` — серверный мод (`side: Server`, под NDA), уже имеет MinIO-креды из env.
- `vsservice` уже имеет тот же MinIO и `CDN_URL`.
- Bucket: `server-mods`, публичный read (CDN отдаёт файлы).
- Ключ манифеста: `manifest.json`.

## Критерий отбора (главное требование)

Отправлять на сайт можно только мод, который клиент обязан поставить:

```
Info.RequiredOnClient == true && Info.Side.IsUniversal()
```

Нельзя отправлять:

- моды с `RequiredOnClient == false`;
- моды со стороной `Server` (например `vintageapi` — NDA-код, не должен утекать клиенту);
- ядро игры (`game`, `creative`, `survival`) и сам `lhservermods`, если у него нет zip;
- моды без файла на диске.

Серверные (скрытые) моды не попадают ни в манифест, ни в bucket — даже их имена.

Отдельно: «разрешённые клиентские моды» (необязательные, которые игрок может поставить)
автоматически не собираются: их добавляет админ через сайт как `OPTIONAL`.

## Манифест `server-mods/manifest.json`

Пишется `VintageAPI` после загрузки файлов. Читается `vsservice`.

```json
{
  "version": 1,
  "generated_at": "2026-10-29T14:00:00Z",
  "game_version": "1.22.7",
  "mods": [
    {
      "modid": "carryon",
      "name": "Carry On",
      "authors": ["xeth"],
      "description": "Plain text from modinfo.",
      "version": "2.0.0",
      "side": "universal",
      "required_on_client": true,
      "filename": "CarryOn-1.21_v2.0.0.zip",
      "size": 123456,
      "sha256": "0123abcd...",
      "file_key": "carryon/CarryOn-1.21_v2.0.0.zip",
      "icon_key": "carryon/modicon.png",
      "uploaded_at": "2026-10-29T13:59:00Z"
    }
  ]
}
```

- `side`: `universal` | `client` | `server` (только `universal` попадает в манифест).
- `icon_key`: пустая строка, если в zip нет `modicon.png`.
- Порядок `mods` — по `modid`, стабильный.
- `version` манифеста = 1; `vsservice` отвергает неизвестную версию и не ломает текущий список.
- `"mods": []` — валидный манифест: сервер не требует ни одного клиентского мода, сайт
  снимает обязательные моды и архив. Отсутствующий или битый манифест, наоборот, ничего
  не меняет.
- Манифест — точка фиксации: `VintageAPI` пишет его только после того, как загружены все
  выбранные моды. Один сбой (после 3 повторов) или коллизия ключей — манифест не
  перезаписывается, старые файлы и список остаются, очистка не запускается.

## VintageAPI (C#)

Новый `ModSystem`, только сервер (`StartServerSide`, после загрузки модов):

1. Собрать загруженные моды из `api.ModLoader.Mods`, взять `Mod.Info` и `Mod.SourcePath`
   (fallback — скан папок модов, как в `lhgui/lhservermods/src/Server/ServerModCollector.cs`).
2. Отфильтровать по критерию выше.
3. Для каждого: sha256 файла; если объект `file_key` уже есть с тем же `sha256` — не грузить.
4. Загрузить zip в `server-mods/<modid>/<filename>`, метаданные `X-Amz-Meta-Sha256`.
   Иконку `modicon.png` из zip — в `server-mods/<modid>/modicon.png` (если есть).
5. Записать манифест (атомарный PUT, после файлов).
6. Удалить из bucket объекты, которых нет в новом манифесте (старые версии), если включена
   очистка. Не удалять `manifest.json`.
7. Ошибка сети/MinIO не должна мешать старту сервера: лог `Error`, остальные системы работают.
8. Публикация одного мода — до 4 попыток (500 мс, 1 с, 2 с). Если хотя бы один мод не
   опубликован, нет архива на диске или два мода дали один `file_key` — прогон
   останавливается: `manifest.json` не перезаписывается, очистка не запускается.
9. Пустой список выбранных модов — валидный прогон: пишется манифест с `"mods": []`.

Конфиг `vintageapi_config.json`:

```json
{
  "ServerModsEnabled": true,
  "ServerModsBucket": "server-mods"
}
```

Остальное — env `VINTAGEAPI_MINIO_*` (уже есть) и `CdnUrl` не нужен.

Замечания:

- MinIO C# — пакет `Minio` 7.0.0 (builder API).
- Bucket создать, если нет, и поставить публичную политику `s3:GetObject` (как
  `pkg/storage.MakeBucketPublic` в vsservice).
- Имя объекта — только `Path.GetFileName`, без `..`.

## vsservice (Go)

Домен `internal/mod`, без изменения публичного контракта proto.

1. `internal/pkg/storage`: добавить `GetObject(ctx, bucket, key) (io.ReadCloser, error)`
   (в `pkg/storage/storage.go`).
2. Конфиг:
   - `MODS_BUCKET` default `server-mods`;
   - `MODS_MANIFEST_KEY` default `manifest.json`;
   - `MODS_SYNC_INTERVAL` default `1m`.
   Также `CDN_URL` уже есть — из него строятся ссылки:
   `file_url = {CDN_URL}/{bucket}/{file_key}`,
   `logo_url = {CDN_URL}/{bucket}/{icon_key}`.
3. Источник манифеста — новый файл `internal/mod/internal/service/sync.go`:
   - читает и валидирует JSON (версия, уникальные modid, sha256 hex, непустые ключи);
   - кеш с TTL и `singleflight`, чтобы `ListMods` не ходил в MinIO каждый раз;
   - при ошибке MinIO отдаёт последний сохранённый список из Mongo (лог Warn), не 500.
4. Синхронизация в Mongo:
   - upsert по `id` для каждого мода манифеста, `source = "server"`, `audience = REQUIRED`;
   - сохранять `note` и `position` существующих записей;
   - удалять документы `source = "server"` (и legacy REQUIRED без source), которых нет в манифесте;
   - `position` новых — в конец списка.
5. Фоновый запуск: первый sync в `fx` lifecycle `OnStart` (best effort), далее ticker
   `MODS_SYNC_INTERVAL`. `ListMods` при устаревшем кеше запускает sync в фоне и сразу
   отвечает из Mongo, поэтому запрос не ждёт MinIO. Все проходы (boot, ticker, lazy)
   идут через `singleflight`: одновременно применяется только один merge.
6. Архив: остаётся ZIP из `REQUIRED`-модов. Файл качается по `release.file_url`
   обычным HTTP-клиентом (таймаут, лимит размера, allow-list: CDN или хосты файлов ModDB);
   при ошибке HTTP файл читается из MinIO по `file_key`. Ключ кеша — как раньше
   `(id, version, file_url, file_name)`.
7. Права админа (тот же scope `news:create`):
   - `AddMod` принимает только `MOD_AUDIENCE_OPTIONAL`; `REQUIRED`/`HIDDEN` → 400
     «required mods come from the server»;
   - `UpdateMod` для серверного мода меняет только `note`; версию/audience — 400;
   - `DeleteMod` серверного мода — 400 (он вернётся при следующем sync);
   - `RefreshMods` обновляет только `source = "admin"` (OPTIONAL) моды;
   - `ReorderMods` работает для всех, sync не сбрасывает позиции и не переписывает
     неизменившиеся документы (иначе `updated_at` архива дёргался бы каждую минуту).
8. `ListMods` не меняется: `REQUIRED + OPTIONAL`, архив по `REQUIRED`.
9. Bucket `server-mods` создаётся публичным при старте (как в `internal/media/fx.go`),
   с таймаутом: недоступный MinIO не должен задерживать старт сервиса.

## Проверка

VintageAPI:

- `dotnet build -c Release` (env `VINTAGE_STORY`, `LhguiAssembly` уже настроены).
- Юнит-тест отбора: server-only мод не попадает, required universal попадает, ядро не попадает.
- Локально на тестовом сервере: в логе `[server-mods]` — сколько собрано/загружено/пропущено,
  в MinIO появился `manifest.json`, в нём нет `vintageapi`.

vsservice:

- `make generate && make lint && make test && make build`.
- Тесты: парсинг манифеста (битый JSON, версия 2, дубликат id), merge (новый мод добавлен,
  пропавший удалён, note/position сохранены, admin-моды не тронуты), архив по HTTP.
- Ручная проверка: `/v1/mods` содержит моды из манифеста, `archive.zip` открывается,
  прямые ссылки `file_url` качаются с CDN.

## Раскатка

1. MinIO: завести bucket `server-mods` (или дать vsservice право создать его) и
   убедиться, что CDN отдаёт `{CDN_URL}/server-mods/*`.
2. vsservice: env по умолчанию подходят (`MODS_BUCKET=server-mods`,
   `MODS_MANIFEST_KEY=manifest.json`, `MODS_SYNC_INTERVAL=1m`); `CDN_URL` обязателен для
   прямых ссылок. Права MinIO-кредов должны включать `s3:PutBucketPolicy`.
3. VintageAPI: `ServerModsEnabled` по умолчанию `true`; MinIO-креды берутся из уже
   существующих `VINTAGEAPI_MINIO_*`. Перезапуск игрового сервера запускает первый sync.
4. Порядок: сначала VintageAPI (запишет полный манифест), затем vsservice. Если наоборот,
   сайт до первого манифеста показывает сохранённый список. Старый пустой/неполный
   `manifest.json` в bucket надо удалить или перезаписать полным до включения нового
   vsservice, иначе сайт примет его за «сервер не требует клиентских модов».
5. Проверить в логе игрового сервера `[server-mods] done: ... uploaded/unchanged/failed`,
   в MinIO — `manifest.json` и файлы только клиентских модов; `vintageapi` в манифесте
   быть не должно.

## Безопасность логов

Серверные (NDA) моды не попадают ни в манифест, ни в bucket, ни в CDN, ни в архив.
Дополнительно ни `VintageAPI`, ни `vsservice` не пишут в лог id, имя или ключ
отброшенного серверного мода: причины отбраковки логируются счётчиками, без
идентификаторов.