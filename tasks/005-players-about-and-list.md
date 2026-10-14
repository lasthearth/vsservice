# 005 — Игроки: описание «О себе» и публичный список с фильтрами

Цель: две вещи для сайта (фронт — landing tasks/007, макеты — артефакт «Игроки и описание профиля — Last Hearth»).
1. **Описание в профиле.** Игрок сам пишет короткий текст «О себе»; его видят все на `/player/<ник>` и в списке игроков. Модератор может скрыть описание.
2. **Список игроков.** Публичная страница `/players`: поиск по нику, фильтр «последний раз в игре», сортировка, постраничный вывод с номерами страниц и счётчиками для фильтров.

Ветка: agent/005 от master.

Перед началом: AGENTS.md и skill `add-feature` (как добавлять RPC, маппинг goverter, fx, публичные методы, проверка через make).

## Что уже есть и что учесть

- Игрок — документ в коллекции `verification_requests` (`internal/player`): `user_id`, `user_game_name`, `previous_nickname`, `avatar`, `is_online`, статус проверки `status`.
- Игровая статистика лежит **в другой коллекции** — `stats` (`user_game_name`, `hours_played`, `death_count`, `players_killed`, `last_online`), по несколько документов на игрока. Таблица лидеров (`internal/leaderboard/internal/repository/repository.go`) группирует её по `user_game_name` (`$sum` часов, `$max` `last_online`) и подтягивает игрока `$lookup`-ом. Список игроков считает часы и «последний раз в игре» **так же**, иначе цифры на странице игрока и в списке разойдутся.
- `SearchUsers` подставляет запрос в `$regex` без экранирования (в `scope.go` это записано как известная проблема). Новый поиск **обязан** экранировать: готовая функция — `escapeRegex` в `internal/wiki/internal/repository/repository.go`; перенести её в `internal/pkg` (например, `internal/pkg/mongox` или новый `internal/pkg/regexsafe`) и использовать в обоих местах. Сам `SearchUsers` не менять.
- Статусы проверки: `approved` — анкету одобрили, `verified` — игрок ввёл код в игре. В список попадают `verified`, а также `approved`, у кого есть хотя бы одна запись в `stats` (уже играл).

## A. Описание «О себе»

### proto (`proto/user/v1/user.proto`)
1. `message User`:
   ```proto
   // What the player wrote about themselves: plain text, at most 500
   // characters and 8 lines. Empty when there is none or a moderator hid it.
   string about = 7;
   // When the description was last changed; unset when there is none.
   google.protobuf.Timestamp about_updated_at = 8;
   ```
2. Новые RPC в `UserService`:
   ```proto
   // Replace the caller's description. An empty string removes it.
   //
   // The text is normalized before it is stored: line endings become \n,
   // trailing spaces are cut, more than one empty line in a row becomes one.
   //
   // Errors:
   //   - INVALID_ARGUMENT (400): longer than 500 characters or 8 lines, or
   //     contains control characters
   //   - FAILED_PRECONDITION (400): the caller is not a verified player
   //   - UNAUTHENTICATED (401): missing or invalid auth token
   //   - INTERNAL (500): database failure
   rpc UpdateMyAbout(UpdateMyAboutRequest) returns (User) {
     option (google.api.http) = {
       put: "/v1/users/me/about"
       body: "*"
     };
   }

   // Hide a player's description. Requires players:moderate.
   //
   // The description is removed and the player is notified with the reason;
   // they may write a new one.
   //
   // Errors:
   //   - INVALID_ARGUMENT (400): empty reason
   //   - NOT_FOUND (404): player not found
   //   - UNAUTHENTICATED (401): missing or invalid auth token
   //   - PERMISSION_DENIED (403): missing players:moderate
   //   - INTERNAL (500): database failure
   rpc HideAbout(HideAboutRequest) returns (google.protobuf.Empty) {
     option (google.api.http) = {
       post: "/v1/users/{user_id}/about:hide"
       body: "*"
     };
   }
   ```
   ```proto
   message UpdateMyAboutRequest {
     string about = 1;
   }
   message HideAboutRequest {
     string user_id = 1 [(google.api.field_behavior) = REQUIRED];
     // Shown to the player in the notification.
     string reason = 2 [(google.api.field_behavior) = REQUIRED];
   }
   ```

### Модель и хранение
3. `model.Player`: поля `About string`, `AboutUpdatedAt *time.Time`. DTO (`internal/player/internal/dto/mongo/player.go`): `About string \`bson:"about,omitempty"\``, `AboutUpdatedAt *time.Time \`bson:"about_updated_at,omitempty"\``. `PlayerUpdate` + `buildUpdateSet` — те же два поля; пустое `About` при скрытии/удалении — `$unset` обоих полей, а не запись пустой строки.
4. Нормализация и проверка — чистая функция в `model` (новый файл `about.go`), с ней работают и сервис, и тесты:
   ```go
   // MaxAboutRunes and MaxAboutLines bound a player's description.
   const (
   	MaxAboutRunes = 500
   	MaxAboutLines = 8
   )

   // NormalizeAbout prepares a description for storage, or reports why it
   // cannot be stored. An empty result means "no description".
   func NormalizeAbout(raw string) (string, error)
   ```
   Порядок: `\r\n` и `\r` → `\n`; `\t` → пробел; любой другой управляющий символ (`unicode.IsControl`, кроме `\n`) и символы направления текста U+202A–U+202E, U+2066–U+2069 → ошибка `INVALID_ARGUMENT`; обрезать пробелы в конце каждой строки; три и более `\n` подряд → два; `strings.TrimSpace` всего текста; затем проверка длины в **рунах** (не байтах) и числа строк. Ошибки — `ierror.InvalidArgument` с текстами `about is longer than 500 characters`, `about has more than 8 lines`, `about contains control characters`.
   HTML и ссылки **не** вырезать и не экранировать: это обычный текст, сайт выводит его интерполяцией.
5. `UpdateMyAbout`: пользователь — из JWT (`interceptor.GetUserID`); игрок не найден или статус не `approved`/`verified` → `FAILED_PRECONDITION` (`player is not verified`). Пустой результат нормализации → удалить описание. Ответ — `User` после записи (как `GetUser`).
6. `HideAbout`: пустая `reason` (после `TrimSpace`) → `INVALID_ARGUMENT`; описание удаляется, игроку уходит уведомление через `notificationuc.Create` + `WithUserId` (образец — `internal/settlement/internal/service/notifier-service.go`): заголовок `Описание профиля скрыто`, текст `Модератор скрыл ваше описание: {reason}. Вы можете написать новое.` Ошибка уведомления — `Warn` в лог, RPC успешен. В лог (Info) — кто скрыл, у кого, причина.
7. Scope: `UpdateMyAbout` — `interceptor.ScopeAuthenticated`; `HideAbout` — новый scope `players:moderate` (в `scope.go` игрока, с комментарием). В отчёте написать, что scope нужно выдать роли администратора в Logto.
8. `GetUser`, `BatchGetUsers`, `SearchUsers` начинают отдавать `about` и `about_updated_at` (маппер + проекция в `SearchUsers`, если она перечисляет поля).

## B. Список игроков

### proto
9. Новый RPC в `UserService`, **публичный** (добавить в `publicMethods` в `internal/server/interceptor/matcher.go`; таблица лидеров уже открыто отдаёт всех игроков с `last_online`, так что новых данных наружу не уходит):
   ```proto
   // Players for the public roster: search by nickname, filter by when they
   // were last in the game, sort and page through.
   //
   // Errors:
   //   - INVALID_ARGUMENT (400): query longer than 32 characters
   //   - INTERNAL (500): database failure
   rpc ListPlayers(ListPlayersRequest) returns (ListPlayersResponse) {
     option (google.api.http) = {get: "/v1/players"};
   }
   ```
   ```proto
   // When a player was last in the game. Each window includes the shorter
   // ones: DAY also counts players who are online now.
   enum LastSeen {
     LAST_SEEN_UNSPECIFIED = 0; // every player
     LAST_SEEN_ONLINE = 1;      // in the game now
     LAST_SEEN_DAY = 2;         // online now or within 24 hours
     LAST_SEEN_WEEK = 3;        // within 7 days
     LAST_SEEN_MONTH = 4;       // within 30 days
     LAST_SEEN_INACTIVE = 5;    // more than 30 days ago, or never recorded
   }

   enum PlayerSort {
     PLAYER_SORT_UNSPECIFIED = 0; // same as LAST_SEEN
     PLAYER_SORT_LAST_SEEN = 1;   // online first, then most recently seen
     PLAYER_SORT_HOURS = 2;       // most hours first
     PLAYER_SORT_NICKNAME = 3;    // alphabetical, case-insensitive
   }

   message ListPlayersRequest {
     // Part of the nickname (current or previous), case-insensitive.
     // Shorter than 2 characters after trimming: ignored.
     string query = 1;
     LastSeen last_seen = 2;
     PlayerSort sort = 3;
     // 1-based page number; 0 means 1. A page past the end returns no players.
     int32 page = 4;
     // Default 24, at most 48.
     int32 page_size = 5;
   }

   message PlayerListItem {
     string user_id = 1;
     string nickname = 2;
     User.Image avatar = 3;
     bool is_online = 4;
     // Unset when the game has never recorded the player.
     google.protobuf.Timestamp last_online = 5;
     double hours_played = 6;
     string about = 7;
   }

   // How many players each window holds for the same query, so the site can
   // show a count on every filter.
   message LastSeenCounts {
     int32 all = 1;
     int32 online = 2;
     int32 day = 3;
     int32 week = 4;
     int32 month = 5;
     int32 inactive = 6;
   }

   message ListPlayersResponse {
     repeated PlayerListItem players = 1;
     // Players matching query and last_seen, ignoring pagination.
     int32 total_size = 2;
     int32 page = 3;
     int32 page_size = 4;
     LastSeenCounts counts = 5;
   }
   ```
   Нумерованные страницы — сознательный выбор (макет показывает «1 … 4 5 6 … 12» и «страница N из M»), поэтому здесь `page` + `total_size`, а не курсор, как в вики. Игроков сотни, `$skip` дешёвый; если станет больше 10 тыс. — переносить `last_online` и часы в документ игрока (написать это комментарием над репозиторием).

### Реализация
10. Репозиторий `internal/player/internal/repository/player/repository/` — метод `ListPlayers(ctx, filter model.PlayerListFilter, page, size int, now time.Time) (*model.PlayerListPage, error)`; `now` — параметром, чтобы тесты не зависели от часов. Один `Aggregate` по `verification_requests`:
    1. `$match`: `user_game_name` не пустой; `status` ∈ {`verified`, `approved`}; если есть запрос — `$or` по `user_game_name` и `previous_nickname` с `$regex` = экранированный запрос, `$options: "i"`;
    2. `$lookup` в `stats` по `user_game_name` с внутренним `$group` (`$sum` `hours_played`, `$max` `last_online`) — тот же расчёт, что у таблицы лидеров;
    3. `$addFields`: `hours`, `last_online`, `has_stats`; затем отбросить `approved` без статистики;
    4. `$facet`:
       - `counts` — один `$group` с `$sum: {$cond: …}` на каждое окно (окна вложены, см. комментарии к `LastSeen`);
       - `total` — `$match` по окну + `$count`;
       - `items` — `$match` по окну, `$sort`, `$skip`, `$limit`, `$project` нужных полей.
    Сортировка: LAST_SEEN — `is_online` desc, `last_online` desc (без даты — в конце), `user_game_name` asc; HOURS — `hours` desc, `user_game_name` asc; NICKNAME — `user_game_name` asc. На `Aggregate` — `Collation{Locale: "ru", Strength: 2}` (регистр не важен, кириллица по алфавиту).
11. Аватар — как в `GetUsersByIds` (префикс `getAvatarPrefix()` через маппер), чтобы адреса совпадали.
12. Сервис: `query` → `TrimSpace`; длиннее 32 рун → `INVALID_ARGUMENT`; короче 2 → игнорировать; `page_size` ≤ 0 → 24, > 48 → 48; `page` ≤ 0 → 1. В ответе — `page` и `page_size`, которые реально применились.
13. Индексы: проверить, есть ли индекс на `stats.user_game_name` и `verification_requests.user_game_name`; если нет — создать там же, где создаются остальные индексы этих коллекций (найти по `CreateMany`/`Indexes()`), без отдельной миграции, если проект так делает.

## Тесты (в стиле существующих)
- `model`: таблица для `NormalizeAbout` — CRLF, табы, хвостовые пробелы, 4 пустых строки подряд, ровно 500 рун кириллицей (проходит) и 501 (нет), 8 и 9 строк, управляющий символ, символ направления текста, только пробелы → пустая строка.
- сервис: `UpdateMyAbout` пишет только вызывающему; неверифицированный → `FAILED_PRECONDITION`; пустая строка удаляет; `HideAbout` без причины → `INVALID_ARGUMENT`; `HideAbout` шлёт уведомление; ошибка уведомления не роняет RPC.
- scope: `HideAbout` требует `players:moderate`, `ListPlayers` публичный — проверка покрытия методов в `interceptor` должна проходить.
- репозиторий/сервис `ListPlayers`: если в проекте нет тестов на живом Mongo — вынести сборку пайплайна в чистую функцию (`buildListPlayersPipeline(filter, page, size, now) bson.A`) и проверить её: экранирование запроса, границы окон относительно `now`, сортировка, `$skip` = `(page-1)*size`, ограничения `page_size`.

## Проверка исполнителем
`make proto` (или `make proto-offline` — и тогда отдельно `docs/v1/openapi.yaml`, см. Makefile) `&& make generate && make lint && make test && make build` — всё зелёное. `openapi.yaml` должен содержать новые методы.

## Готово, когда
- `PUT /v1/users/me/about` сохраняет и нормализует текст, `GET /v1/users/{id}` его отдаёт;
- `POST /v1/users/{id}/about:hide` без `players:moderate` → 403, с ним — описание удалено, уведомление создано;
- `GET /v1/players?query=вет&last_seen=LAST_SEEN_WEEK&sort=PLAYER_SORT_HOURS&page=2` работает без токена, `counts` и `total_size` согласованы с `players`;
- тесты выше есть и проходят.

## Не делать
- жалобы игроков на описания (нужна отдельная модель) — следующая задача, если понадобится;
- не менять `SearchUsers`, таблицу лидеров и формат `stats`;
- не переносить статистику в документ игрока.

## Отчёт
≤10 строк: что сделано, что нет, вывод `make lint/test/build`, что нужно настроить в Logto (`players:moderate`). `git commit` + `git push -u origin agent/005`.
