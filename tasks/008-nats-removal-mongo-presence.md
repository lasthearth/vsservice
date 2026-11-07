# 008 — NATS убираем полностью: онлайн, статистика и гейт входа идут через MongoDB

Цель: брокера сообщений в связке «игровой сервер ↔ сайт» больше нет. Всё, что раньше
летело по NATS, пишется игровым модом напрямую в MongoDB и читается `vsservice` из неё.
Попутно чинятся три дефекта текущего трекинга: часы теряются при краше сервера,
`is_online` залипает навсегда, каждый сломанный блок — отдельный round-trip в БД.

Раскатка одним выстрелом, без двойной записи (решение владельца). Инстанс NATS в Dokploy
владелец гасит сам после того, как обе стороны задеплоены.

## Что уходит

| Subject | Было | Стало |
| --- | --- | --- |
| `player.try-join` (RPC, 2 с) | мод спрашивает сайт, сайт отвечает статусом | мод сам читает `verification_requests.status` |
| `player.join` / `player.leave` | сайт ставит `is_online` | мод ставит `is_online` + `last_heartbeat`, сайт гасит протухшее |
| `world.time` (5 с) | сайт пишет `serverinfo.world_time` | мод пишет `serverinfo.world_time` |
| `world.total-online` (5 с) | сайт пишет `serverinfo.total_online` | мод пишет `total_online` + `max_online` + `heartbeat_at` |
| `kit.granted` / `kit.claimed` (JetStream) | паблишер в пустоту, подписчик ни от кого | шина удаляется, домен kit и proto не трогаем |

`kit.granted` публиковался в стрим, который никто не читал: на стороне мода ни
паблишера, ни подписчика нет (старый `Systems/Kit/*` уже удалён, выдача идёт письмом —
ADR-0021, а содержимое китов мод берёт из коллекции `kits` через `MongoKitCaptureBackend`).

## Топология после

```
Игровой сервер (VintageAPI, server-only мод)
  ├─ гейт входа: FindOne verification_requests по user_game_name  (синхронно, 2 с, fail-closed)
  └─ фоновый писатель (один цикл, 15 с):
        verification_requests  ← is_online, last_heartbeat      (bulk)
        player_sessions        ← открыть/продлить/закрыть        (bulk)
        stats                  ← счётчики батчем, часы при закрытии сессии (bulk)
        serverinfo             ← world_time, total_online, max_online, heartbeat_at (5 с)

vsservice (Go)
  ├─ читает serverinfo (только чтение, Update удалён)
  ├─ читает verification_requests.is_online в ростере (запросы не меняются)
  └─ sweeper (60 с): гасит протухший is_online и протухший total_online
```

Единственный писатель `stats`, `deaths`, `player_sessions` и `serverinfo` — мод.
`vsservice` в эти коллекции не пишет ничего, кроме двух полей-флагов, которые гасит
sweeper (`verification_requests.is_online`, `serverinfo.total_online`). Писатели не
пересекаются по времени: мод пишет, пока heartbeat свежий, sweeper — только когда он
протух, так что флаги не «хлопают».

## Контракт MongoDB

База — та же, что у `vsservice` (`VintageApiConfig.MongoDatabase`, по умолчанию `lsp`).
Все времена — **int64 unix-мс**, кроме `created_at`/`updated_at`, которые остаются BSON
datetime: `mongox.Model` декодирует их в `time.Time`, а driver v2 умеет и int64-мс
в `time.Time` (`bson/time_codec.go`, `TypeInt64 → time.Unix(i64/1000, …)`), но смешивать
типы в одном поле нельзя.

### 1. `verification_requests` — документ игрока (владелец: vsservice)

Мод добавляет два поля и больше ничего не трогает:

```js
{ is_online: true, last_heartbeat: NumberLong(1762380000000) }
```

- Ключ матчинга — `user_game_name`, точное совпадение (его уже используют
  `PlayerIdentityResolver` в моде и `GetByUserGameName` в сервисе).
- **Без upsert**: входящий игрок верифицирован, значит документ есть. Нет документа —
  `Warn` в лог, запись не создаём (иначе мод наплодит документов-призраков без `user_id`).
- `is_online = true` пишется на входе и на каждом heartbeat; `false` — на дисконнекте и
  на остановке сервера.
- Индекс создаёт `vsservice` рядом с `setupIndexes`
  (`internal/player/internal/repository/player/repository/app.go`), частичный:
  `{ is_online: 1, last_heartbeat: 1 }` c `partialFilterExpression: { is_online: true }` —
  онлайн-игроков десятки, а коллекция большая.

### 2. `player_sessions` — журнал сессий (новая, владелец: мод)

```js
{
  _id: ObjectId,
  user_game_name: "Ветер",
  player_uid: "1234567890",
  seed: 1234567,
  started_at: NumberLong(1762380000000),
  last_heartbeat: NumberLong(1762380015000),
  ended_at: NumberLong(1762383600000),   // null, пока сессия открыта
  end_reason: "disconnect",              // disconnect | shutdown | crash | null
  hours: 1.0                             // null, пока открыта; round((ended-started)/3600000, 2)
}
```

- Открытие — на `PlayerJoin` (insert), продление — на каждом heartbeat
  (`$set last_heartbeat`), закрытие — на `PlayerDisconnect`, на остановке сервера и при
  старте (см. «восстановление после краша»).
- Закрытие — **условный** апдейт `{ _id, ended_at: null }`, поэтому закрыть сессию дважды
  нельзя ни при каком порядке событий.
- Индексы (создаёт мод): `{ user_game_name: 1, started_at: -1 }`, `{ seed: 1, ended_at: 1 }`.

### 3. `stats` — существующая коллекция мода, схема не меняется

- `hours_played` (`$inc`) и `last_online` (`$set`) пишутся **один раз при закрытии сессии**,
  значением `hours` из `player_sessions`. Текущий путь
  `PlayerStatsManager.PlayerDisconnected` (начисление только на дисконнекте) заменяется,
  а не дополняется — иначе часы удвоятся.
- Счётчики (`block_breaks`, `block_placed`, `block_traveled`, `damage_to_players`,
  `death_count`, `players_killed`) копятся в памяти и уходят одним `BulkWrite` раз в 15 с,
  плюс на дисконнекте игрока, на остановке сервера и при старте.
- `deaths` — по-прежнему немедленный `InsertOne` (событие, а не счётчик; их мало).
- Цена батчинга: при краше процесса теряется до 15 с счётчиков блоков. Часы не теряются
  никогда — их источник `player_sessions.last_heartbeat`, а не память.

### 4. `serverinfo` — singleton-документ (владелец документа: vsservice, писатель: мод)

```js
{
  _id: ObjectId,                       // тот, что уже есть; новый не создаём
  world_time: "1 день, 3 часa",        // calendar.PrettyDate()
  total_online: 12,
  max_online: 75,                      // api.Server.Config.MaxClients, больше не хардкод 75
  heartbeat_at: NumberLong(1762380000000),
  created_at: ISODate(...),            // $setOnInsert
  updated_at: ISODate(...)             // BSON datetime, не int64
}
```

- Апсерт с фильтром `{}` и `$setOnInsert created_at`: писатель один (таймер мода),
  второй документ появиться не может, а `GetServerInfo` сервиса
  (`FindOne(bson.M{})`) продолжает работать без изменений.
- Интервал 5 с, как было у `world.time`.
- `max_online` теперь настоящий: сегодня `serverinfo.Repository.Update` при каждом
  апдейте прибивает его к 75 (`repository.go:62-65`), потому что NATS-обработчик
  значение `Max` из события терял.

### 5. Гейт входа

`PreFinalizePlayerIdentification` (существующий Harmony-префикс) читает Mongo напрямую:
`Find({ user_game_name }, projection { status: 1 }, limit 2)`, таймаут 2 с через
`CancellationToken`. Кэш и Change Stream не нужны: попытка входа — редкое событие
(одно на подключение), а свежий статус важнее сэкономленного запроса.

**`limit 2` обязателен, `FindOne` — нет.** На одного пользователя в
`verification_requests` может лежать два документа: об этом прямо написано в
`setupIndexes` сервиса («the check-then-create Submit flow can already have left two
documents for one user»), и ровно поэтому `PlayerIdentityResolver`
(`DailyRewardsManager/PlayerIdentityResolver.cs:70-76`) читает `Limit 2` и при
расхождении отвечает «не знаю» вместо угадывания. Гейт повторяет это правило:
два документа с разными `status` → отказ («не зарегистрированы»), потому что
угадать, какой из них свежее, нельзя, а пустить нельзя тем более.

Запрос повторяет форму `PlayerIdentityResolver` (тот же ключ, тот же лимит), но
проекция другая: резолверу нужен `user_id` и он фильтрует по `status IN
{verified, approved}`, гейту нужен сам `status` без фильтра — иначе нечем отличить
`pending` от `rejected` в сообщении игроку.

Таблица решений — ровно та, что была в NATS-ответе:

| `status` | Результат |
| --- | --- |
| `verified`, `approved` | пускаем |
| `pending` | кик «Анкета находится на проверке, попробуйте позже» |
| `banned` | кик с сообщением о бане (сайт такого статуса не возвращает — `verification.Status` знает только pending/approved/verified/rejected; ветка оставлена на будущее) |
| `rejected`, пустой, неизвестный, документа нет | кик «Вы не зарегистрированы на сайте! (lasthearth.ru)» |
| ошибка Mongo, таймаут | кик «Ведутся тех. работы, либо наблюдаются временные неполадки.» |

**Fail-closed во всех ветках**, включая ошибку: сервер без гейта — открытая дверь.
По той же причине `ServerAuthSystem` падает на старте, если `VINTAGEAPI_MONGO_URL` не
задан (раньше он падал без `VINTAGEAPI_NATS_URL` ровно по этой причине — см. комментарий
в `MessagingSystem.StartPre`).

## VintageAPI (C#)

Новое, по образцу `MailManager`/`NotifierManager` (ADR-0010: `ILogger` или
`ICoreServerAPI` + `IMongoDatabase` в конструкторе, `[BsonElement]` на полях, никаких
кредов вне env):

```
PresenceManager/PresenceDocs.cs        // SessionDoc, ServerInfoDoc + имена индексов константами
PresenceManager/IPresenceBackend.cs    // шов для тестов: гейт + presence + сессии + serverinfo + flush
PresenceManager/MongoPresenceBackend.cs// весь Mongo I/O
PresenceManager/PresenceWriter.cs      // фоновый цикл: очередь команд, батчи, восстановление после краша
PresenceManager/StatBatcher.cs         // накопитель счётчиков (ConcurrentDictionary + snapshot/reset)
Systems/PresenceSystem.cs              // ModSystem: клиент, конфиг, снимок онлайна, хуки событий
```

Гейт живёт в том же бэкенде (`IVerificationGate`), а `ServerAuthSystem` оставляет себе
только Harmony-патч и разрешает бэкенд лениво внутри `CheckVerification` через
`api.ModLoader.GetModSystem<PresenceSystem>()` — ровно так, как сегодня он достаёт
`MessagingSystem.NatsClient` (`ServerAuthSystem.cs:103-104`). Ленивое разрешение важно:
порядок `StartServerSide` у ModSystem не гарантирован, а патч регистрируется именно там.

Загрузочный предохранитель переезжает, а не исчезает: сегодня `MessagingSystem.StartPre`
бросает `InvalidOperationException` без `VINTAGEAPI_NATS_URL`, потому что сервер без гейта
— открытая дверь (`MessagingSystem.cs:17-27`). Тот же `throw` остаётся в
`PresenceSystem.StartPre`, но про `VINTAGEAPI_MONGO_URL` и только когда
`ServerAuthEnabled == true`.

Соглашения кода, которые в этом моде уже сложились (нарушать нельзя, ревью на них смотрит):

- Каждый POCO — `[BsonIgnoreExtraElements]`, иначе документ сайта с лишними полями не
  десериализуется (`MailDocs.cs:14`, `NotifierDocs.cs:24`, …). У `PlayerStats` этого
  атрибута нет — существующий огрех, не размножать.
- Каждое поле — `[BsonElement("snake_case")]`, зеркалящий Go DTO.
- `[BsonIgnoreIfDefault]` на `_id` ровно там, где писатель — мод (тогда драйвер сам
  генерирует id); у документов, которые мод только читает, его нет.
- Имена индексов — константы на классе документа (`SettlementNotifierDoc.UniqueIndexName`),
  создание в своём `try/catch` (`TryCreateIndex`, `MongoMailBackend.cs:63-71`).
- Фоновый цикл — `_ = Task.Run(() => Loop(_cts.Token))` + `Task.Delay`, отмена в
  `Dispose()`, и сначалаdisposing бэкенда, потом `MongoClient`
  (`NotifierBackendSystem.cs:146-147`, `ShipTradingBackendSystem.cs:74-77`).
  `System.Threading.Timer` и `PeriodicTimer` в моде не используются нигде — не начинать.
- Снимок состояния мира делается на главном треде; всё, что нужно из мира в фоновом
  потоке, оттуда и забирается (обратный hop — `EnqueueMainThreadTask`, нам не нужен).

Правила, без которых мод либо вешает тик, либо теряет данные:

1. **Весь Mongo I/O — вне главного треда.** На главном треде (`api.Event.Timer`, как
   сегодня в `MessagingSystem.cs:63`) только снимок: имена/uid онлайн-игроков,
   `calendar.PrettyDate()`, `AllOnlinePlayers.Length`, `MaxClients`. Снимок уходит в
   очередь, фоновый писатель вычитывает её и делает bulk-записи. Цикл писателя —
   `Task` + `Task.Delay`, как в `MongoNotifierBackend` и `MongoShipTradingBackend`
   (единственные два настоящих фоновых цикла в моде); `PeriodicTimer` не берём, чтобы не
   плодить третий способ. Исключение — гейт входа: он и сегодня блокирует префикс на
   `.Wait()`, остаётся синхронным чтением с таймаутом 2 с.
2. **Один цикл на все записи.** Presence, heartbeat сессий и flush счётчиков едут в одном
   тике: три `BulkWrite` независимо от числа игроков вместо N round-trip'ов.
3. **Порядок команд сохраняется**: открыть сессию → продлить → закрыть. Одна очередь.
4. **Восстановление после краша** — первое, что делает писатель на старте: закрыть все
   `{ seed, ended_at: null }` значением `ended_at = last_heartbeat`, `end_reason = "crash"`,
   начислить часы в `stats`. Это и есть починка «краш съел часы».
5. **Остановка сервера**: закрыть открытые сессии (`end_reason = "shutdown"`), сбросить
   счётчики, снять `is_online` у всех своих игроков.
6. **Ошибка записи не теряет данные и не роняет сервер**: неудачный flush возвращает дельты
   в накопитель, лог `Error`, игра продолжается. Никаких исключений в игровой тред.
7. Конфиг `vintageapi_config.json`: `ServerAuthEnabled` (true), `PresenceEnabled` (true),
   `PresenceHeartbeatSeconds` (15), `StatsFlushSeconds` (15), `ServerInfoIntervalSeconds` (5).
   **Все пять полей обязаны быть объявлены на `VintageApiConfig`**
   (`Systems/PlayerStatsConfig.cs`): `PlayerStatsSystem.cs:24-26` на каждом старте не
   только читает, но и безусловно перезаписывает `vintageapi_config.json`, поэтому ключ,
   которого нет в классе, из файла вычищается. Именно из-за этого `NotifierBackendSystem`
   и `DailyRewardBackendSystem` держат собственные `notifier-backend.json` и
   `daily-rewards.json` — нам отдельный файл не нужен, флаги presence относятся к ядру мода.

Удаляется: `Systems/Messaging/**` (MessagingSystem, EventManager, `Events/Event.cs`,
`PlayerTryJoinEvent.cs`, `PlayerJoinEvent.cs`, `PlayerLeaveEvent.cs`, `WorldTimeEvent.cs`,
`WorldTotalOnlineEvent.cs`), NATS в `ServerAuthSystem` (вместе с мёртвыми
`using System.Net.Http;` и `using Timer = System.Threading.Timer;`),
`Secrets.NatsUrlEnvVar`, `PackageReference NATS.Net 3.0.0-preview.8`
(`VintageAPI.csproj:133`), `using NATS.Net;` в `PlayerStatsSystem.cs:5`.

Заодно правится классовый комментарий `PlayerIdentityResolver`
(`DailyRewardsManager/PlayerIdentityResolver.cs:14-31`): он описывает гейт как
«AuthPatch.CheckVerification спрашивает сайт», а после этой задачи гейт читает ту же
коллекцию сам. Сам резолвер не меняется — его правило «`Limit 2`, расхождение → null»
гейт повторяет, а не переиспользует: резолвер фильтрует по `status IN {verified,approved}`
и проектирует `user_id`, гейту нужен `status` без фильтра.

Тесты (`VintageAPI.Tests`, xunit.v3, чистая логика без живой Mongo — 261 кейс сейчас):
таблица решений гейта по статусу (включая «два документа разошлись» и «Mongo
недоступен»); расчёт `hours` из `started_at`/`ended_at`; условное закрытие сессии
(второй вызов — no-op, часы не удваиваются); `StatBatcher` (накопил → snapshot → reset,
повторный snapshot пустой); восстановление после краша закрывает сессию по
`last_heartbeat`, а не по «сейчас». Там, где надо доказать «в Mongo не ходим», в проекте
уже есть приём: заведомо немаршрутизируемая база `127.0.0.1:1` с таймаутом 200 мс
(`UnboundNotifierTests.cs:43-48`).

## vsservice (Go)

Новое. Два sweep'а, по одному в домен — каждый гасит только свою коллекцию, иначе
`internal/serverinfo` полез бы в чужую:

1. **Presence sweeper** (`internal/player`, fx lifecycle hook + `time.Ticker`): раз в
   `PRESENCE_SWEEP_INTERVAL` (60 с)
   ```js
   updateMany(
     { is_online: true,
       $or: [ { last_heartbeat: { $lt: cutoff } }, { last_heartbeat: { $exists: false } } ] },
     { $set: { is_online: false } })
   ```
   Ветка `$exists: false` обязательна: на момент раскатки у всех «онлайн» документов
   heartbeat ещё нет, и без неё они останутся онлайн навсегда. `cutoff = now -
   PRESENCE_STALE_AFTER` (60 с).
2. **Serverinfo sweeper** (`internal/serverinfo`, свой такой же цикл):
   `updateMany({ heartbeat_at: {$lt: cutoff2}, total_online: {$ne: 0} }, {$set: {total_online: 0}})`
   плюс ветка `{heartbeat_at: {$exists: false}}`; `cutoff2 = now - SERVERINFO_STALE_AFTER`
   (30 с). Сервис остаётся чистым читателем: `TotalOnline`/`WorldTime` не меняются,
   proto не трогаем. `world_time` намеренно не затирается — последнее значение полезнее
   пустоты.
3. Логи — со счётчиком погашенных (`presence.sweep.ok` / `serverinfo.sweep.ok`,
   `n` документов), не по одному на игрока; пустой проход — `Debug`, не `Info`
   (обоснование в `internal/event/internal/service/reminder.go:114-119`).

Как писать, чтобы ревью не завернуло (всё уже есть в репозитории):

- Форма цикла — `internal/mod/internal/service/sync.go:154-168` (первый проход сразу,
  дальше ticker, `select` на `ctx.Done()`), а жизненный цикл — строгий вариант из
  `internal/event/fx.go:58-78`: `cancel` + канал `done`, чтобы `OnStop` дождался выхода
  горутины. Каждый проход ограничен своим таймаутом (`remindTimeout`, `modsSyncTimeout`)
  и получает корреляционный id через `logger.WithJob(ctx, "presence.sweep")`
  (`internal/pkg/logger/job.go:18-27`), логгер — `logger.From(ctx, s.log).WithMethod(...)`.
  Ошибка прохода — `Warn` и повтор на следующем тике, старт fx не роняем.
- Индекс — объявленный `[]mongo.IndexModel` + `CreateMany` в `setupIndexes`
  репозитория игрока (канон — `internal/event/internal/repository/app.go:65-79`),
  в существующем бюджете `indexSetupTimeout` (10 с на обе коллекции, fx-старт ждёт 15 с).
- `WithComponent` ставится один раз в конструкторе, `WithMethod` — на каждую операцию.

Удаляется: `internal/pkg/messaging/**` целиком (mnats, mjetstream, correlation, app.go,
`messaging.go` с generic-интерфейсами, `logtest` и все 7 тестовых файлов — `logtest`
импортируют только тесты самого пакета), `internal/player/internal/event/**`,
`internal/serverinfo/internal/event/**`, шина кита
(`internal/kit/internal/service`: `NewEventManager`, `Bus`, `queue.go`, `events.go`,
`Publish` в `service.go:100`), `messaging.SetupConn` и импорт из `main.go:29,87`,
`NatsUrl` и `NatsMaxDeliverLog` из `internal/pkg/config/config.go:85,88`,
`nats.go`/`jetstream` из go.mod (`go mod tidy`), `serverinfo.Repository.Update` вместе с
`SetWorldTime`/`SetTotalOnline` в модели.

За пределами пакета `messaging` его используют ровно четыре файла: `main.go`,
`internal/player/internal/event/app.go`, `internal/serverinfo/internal/event/app.go`,
`internal/kit/internal/service/app.go`. Ни один тест вне пакета NATS не упоминает.

**Про kit.** Домен `internal/kit` мёртв: его не импортирует никто и в fx-композиции
`main.go` его нет (в :111 подключён другой домен — `kitdef`). Шину из него убрать
придётся в любом случае, иначе пакет не компилируется после удаления `messaging`.
Домен и `proto/kit/v1` по решению владельца остаются на месте; отдельно сообщаем, что
всю папку `internal/kit` можно снести целиком — это уже за рамками этой задачи.

**Про `banned`.** Такого статуса в сервисе нет: `verification.Status` — это ровно
pending/approved/verified/rejected. Ветка `banned` в гейте недостижима и оставлена только
как место под будущий бан-лист; выдумывать `VerificationStatusBanned` в этой задаче не надо.

`NATS_URL` в `.env` правит владелец (файл закрыт для агента политикой безопасности).
`NatsUrl` не помечен как `required`, так что исчезновение переменной не ломает парсинг
конфига.

Конфиг: `PRESENCE_SWEEP_INTERVAL` (60s), `PRESENCE_STALE_AFTER` (60s),
`SERVERINFO_STALE_AFTER` (30s).

Документация: `AGENTS.md` (строки про `event/`, NATS в схеме запроса, `NATS_URL`,
«Messaging: NATS/JetStream», `internal/pkg/mnats`), `docs/logging.md` (события
`nats.*`, `NATS_MAX_DELIVER_LOG`, панель NATS в Grafana), комментарий про `fatcontext`
в `.golangci.yml`. Исторические `docs/plans/2026-10-27-logging-replay.md` и
`specs/001-admin-kit-distribution/**` не переписываем — это запись о прошлом.

Тесты: фильтр sweeper'а собирается с обеими ветками `$or` (отдельный кейс на
`$exists: false` — он и есть защита от залипания); `serverinfo` больше не имеет
`Update` в интерфейсе репозитория; `AssignKitToUser` сохраняет назначение без шины;
в репозитории не осталось ни одного импорта `nats.go`.

## Проверка

VintageAPI (из `C:\Users\ripls\RiderProjects\VintageAPI\VintageAPI`):

- `.\build.ps1` — Cake Frosting: `ValidateJson` → `Build` (`DotNetPublish` в Release) →
  `Package` (zip в `..\Releases\vintageapi_2.0.0.zip`). Тесты Cake **не запускает**.
  Нужны env `VINTAGE_STORY` (задан: `C:\Games\Vintagestoryorig`) и MSBuild-свойство
  `LhguiAssembly` (дефолт в `VintageAPI.csproj:11` существует на диске).
- `dotnet test VintageAPI\VintageAPI.Tests\VintageAPI.Tests.csproj` — xunit.v3, 261 кейс
  до наших изменений; решения Cake и CI-скрипта для тестов в репозитории нет,
  `VintageAPI\VintageAPI.sln` тестовый проект не включает, корневой
  `C:\Users\ripls\RiderProjects\VintageAPI\VintageAPI.sln` — включает.

vsservice: `go build ./...`, `go test ./...`, `go -C tools/modelguard test ./...`,
`.\custom-gcl.exe run ./...` (линтер с локальным плагином `modelguard`, который следит за
правилом «модель меняется только своими методами»). `make build` на Windows не годится:
`BIN ?= /bin/vsservice`, `SHELL := /bin/sh`.

Ни одна из двух проверок не покрывает живой Mongo и живой игровой сервер, поэтому
критерий «готово» — собранные обе стороны плюс перечисленные ниже ручные проверки.

Вручную на тестовом сервере после раскатки:
в `verification_requests` у онлайн-игрока `is_online: true` и `last_heartbeat` свежий;
`kill -9` игрового процесса → через `PRESENCE_STALE_AFTER` все стали `is_online: false`,
`total_online` в `serverinfo` обнулился, а после перезапуска сервера в `player_sessions`
появилась закрытая сессия с `end_reason: "crash"` и часами, и `stats.hours_played`
вырос ровно на них; вход незарегистрированным именем → кик; вход с `status: pending` → кик;
при остановленном Mongo вход → кик «тех. работы», а не пуск; `GET /v1/serverinfo/...`
отдаёт время мира и онлайн без NATS.

## Раскатка (одним выстрелом)

1. Собрать и выложить новый VintageAPI, перезапустить игровой сервер. С этого момента
   гейт читает Mongo, presence/сессии/serverinfo пишутся в Mongo, в NATS не летит ничего.
   Старый `vsservice` при этом просто не получает событий — его подписчики стоят
   бездельем, данные не портятся.
2. Задеплоить новый `vsservice`. Первый же sweep гасит `is_online` у всех, кто остался
   «онлайн» с NATS-эпохи (у них нет `last_heartbeat`), и обнуляет `total_online`, если
   `serverinfo.heartbeat_at` ещё не появился.
3. Убрать `VINTAGEAPI_NATS_URL` из env игрового сервера и `NATS_URL` из env `vsservice`.
4. Владелец гасит инстанс NATS в Dokploy.

Порядок именно такой: сначала мод, потом сервис. Тогда гейт не зависит от того, жив ли
старый NATS-респондер, а окно рассогласования — только presence-флаги на несколько минут.

На что смотреть в логе старта `vsservice` после шага 2: `failed to create indexes` на
`verification_requests`. `setupIndexes` переведён с двух `CreateOne` на один `CreateMany`
(конвенция репозитория), а `createIndexes` атомарен на коллекцию: если новый частичный
индекс не успеет построиться в общем 10-секундном бюджете `indexSetupTimeout`, ошибка
будет одна на все три индекса. Два старых в базе уже есть, так что потеряется только
новый, и sweeper деградирует в скан коллекции — корректно, но медленно. Лечится вручную:
`{is_online: 1, last_heartbeat: 1}` c `partialFilterExpression {is_online: true}`.

Откат: вернуть оба деплоя; брокер жив до шага 4, так что старая схема заработает сразу.

## Вне рамок

- Флаг «сервер оффлайн» в proto (`TotalOnlineResponse.stale`) — добавляется аддитивно
  одним полем + `buf generate`, когда понадобится фронту. Сейчас протухший онлайн
  выражается нулём.
- Перевод компенсаций в `donate`/`progression`/`settlement` на многодокументные
  транзакции (ADR-0022 упоминает их как устаревшие комментарии) — отдельная задача.
- Бан-лист: сайт не возвращает `banned`, ветка в гейте оставлена как есть.

## Чем реализация отличается от этого документа

Всё ниже — сознательные отступления, а не недоделки.

- **Гейт стал строже, чем был.** Старый `AuthPatch` ловил любое исключение, писал его в
  лог и **пускал игрока**: `catch (Exception) { Console.WriteLine(...); }` не мешал префиксу
  завершиться. Теперь непойманное исключение — отказ. Заодно 2-секундное ожидание NATS
  заменено на явный `CancellationTokenSource` с тем же бюджетом.
- **`PlayerStatsManager` остался.** Его не удалили, а сделали фасадом: он по-прежнему
  создаёт уникальный индекс `{user_game_name, seed}` на `stats`, который иначе потерялся
  бы, и по-прежнему пишет счётчики напрямую, если presence выключен. При включённом
  presence он отдаёт счётчики батчеру, а начисление часов в `PlayerDisconnected`
  выключается одной строкой (`if (Batcher != null) return;`) — это и есть защита от
  двойного счёта. Попутно закрыт старый латентный баг: `_playersLoginTime.Add` падал на
  реконнекте с тем же именем.
- **Serverinfo-sweeper тикает по `SERVERINFO_STALE_AFTER`** (30 с), отдельного
  `SERVERINFO_SWEEP_INTERVAL` нет: проверять чаще ширины окна нельзя ничего погасить
  раньше, а счётчик протухает максимум за два окна. Ключ добавить — пять строк.
- **Счётчики при краше теряются до 15 с.** Часы — нет: их источник `player_sessions`,
  а не память.
- **Три wiring-теста** (`internal/player`, `internal/serverinfo`, `internal/kit`) появились
  потому, что `go build`/`go test` не видят fx-граф, который перестал резолвиться; падение
  вылезло бы только при старте процесса. Образец — `internal/mod/wiring_test.go`.
- **`playerevfx` удалён целиком**, а не вырезан по строчкам: после удаления шины он
  предоставлял второй `repository.Mapper` и второй `Repository`, из-за чего `setupIndexes`
  выполнялся бы дважды за старт.
- **Приватный `presence.sweep` идёт в `internal/player/internal/service/presence`,**
  репозиторный интерфейс объявлен там же, а `playerfx` прокидывает один и тот же
  `*Repository` двумя `fx.As` в одном `fx.Annotate` — чтобы индекс-сетап остался однократным.
- **Цикл писателя в моде ждёт на `SemaphoreSlim.WaitAsync(_tickMs, ct)`, а не на голом
  `Task.Delay`.** `Task.Run` и отмена в `Dispose` — как написано выше, `Task.Delay`
  остался только в бэкоффе восстановления. Причина: голый delay заставляет вход ждать
  до полного тика перед появлением в ростере, а `Dispose` — до полного тика на
  остановке. Семафор — тот же отменяемый delay, который можно ещё и прервать досрочно.
- **`ServerAuthEnabled=false` не регистрирует Harmony-патч вообще** (`PatchAll` не
  вызывается, никто не проверяется). В контракте от флага зависел только загрузочный
  предохранитель, смысл «false» из него и выведен. Дефолт `true`, так что текущее
  поведение не меняется. `PresenceEnabled=false` при этом гейт оставляет, а писателя не
  запускает — и `PlayerStatsManager` тогда продолжает писать счётчики по событию и часы
  по-старому, то есть флаг выключает presence, а не статистику.
- **`heartbeat_at` штампуется в момент записи**, а не берётся из снимка главного треда:
  это ключ живости для sweeper'а, он обязан говорить, когда запись случилась.
  `world_time` / `total_online` / `max_online` — из снимка.
- **Имена индексов `player_sessions`** в контракте не задавались; взяты дефолтные для
  Mongo (`user_game_name_started_at`, `seed_ended_at`) константами на `SessionDoc`, чтобы
  индекс той же формы, созданный со стороны Go, столкнулся по имени, а не задублировался.
- **Polly 8.7.0 удалён из `VintageAPI.csproj`.** Его единственным потребителем был
  retry в NATS-гейте; после выпила брокера пакет не использует ни один файл (проверено
  grep'ом по всему дереву). Сборка и тесты после удаления зелёные.
- **Число тестов в C#: 412 passed / 0 failed** (107 новых), а не 261, как было написано
  в разделе «Проверка»: 261 — это состояние HEAD, а в рабочем дереве уже лежали чужие
  незакоммиченные тестовые файлы, так что базой были 305. Команда —
  `dotnet test VintageAPI.Tests\VintageAPI.Tests.csproj` из `VintageAPI\VintageAPI`
  (путь `VintageAPI\VintageAPI.Tests\...` оттуда не резолвится).

Известные дефекты, замеченные и намеренно не тронутые: `internal/kit` — мёртвый код, и его
`service.NewFx` передаёт `nil` как `KitRepository` (упал бы при монтировании в `main.go`);
`model.PlayerUpdate.IsOnline` остался без продакшн-писателя; `dto/mongo/stats` (пакет
`statsdto`) не импортируется нигде; инлайновый `stats.Stats` в документе игрока вообще без
bson-тегов, поэтому читается как `hoursplayed`/`lastonline`; ростерная сортировка
`{is_online:-1, last_online:-1, nickname_key:1, _id:1}` не покрыта ни одним объявленным
индексом.

В моде, тоже не тронуто: `PlayerDeath` возвращается до `_deathColl.InsertOneAsync`, когда
источник урона — `EnumDamageSource.Player`, но сущность не `EntityPlayer`: `death_count`
растёт, а документа в `deaths` нет. `PlayerStats` — единственный POCO без
`[BsonIgnoreExtraElements]` и без `[BsonElement]`, держится на совпадении camelCase с
колонками сайта. В `VintageAPI.csproj` остались закомментированные блоки
`<Reference Include="NATS.Net">` — это комментарии в общем закомментированном списке старых
референсов, а не зависимости.
